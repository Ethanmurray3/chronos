package store

import (
	"database/sql"
	"errors"
	"time"
)

// Entry is a block of tracked time. A running timer is simply an entry with
// ended_at / duration_min still NULL; there can be at most one per owner
// (enforced by a unique partial index).
type Entry struct {
	ID          int64  `json:"id"`
	ClientID    *int64 `json:"client_id,omitempty"`
	MatterID    *int64 `json:"matter_id,omitempty"`
	WorkTypeID  *int64 `json:"work_type_id,omitempty"`
	Description string `json:"description"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     *int64 `json:"ended_at,omitempty"`
	DurationMin *int64 `json:"duration_min,omitempty"`
	Billable    bool   `json:"billable"`
	Source      string `json:"source"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`

	// Derived, read-only fields for the UI / reports.
	Running      bool   `json:"running"`
	EffectiveMin int64  `json:"effective_min"` // duration if closed, else live elapsed
	ClientName   string `json:"client_name,omitempty"`
	WorkTypeName string `json:"work_type_name,omitempty"`
}

const entryCols = `
  e.id, e.client_id, e.matter_id, e.work_type_id, e.description,
  e.started_at, e.ended_at, e.duration_min, e.billable, e.source,
  e.created_at, e.updated_at, c.name, w.name`

const entryFrom = `
  FROM time_entries e
  LEFT JOIN clients    c ON c.id = e.client_id
  LEFT JOIN work_types w ON w.id = e.work_type_id`

// ListEntries returns the owner's entries whose start falls in [from, to)
// (either bound optional, unix seconds), optionally filtered to one client,
// most recent first. A running timer is always included when it matches.
func (s *Store) ListEntries(owner int64, from, to, clientID *int64) ([]Entry, error) {
	q := `SELECT` + entryCols + entryFrom + ` WHERE e.owner_id = ?`
	args := []any{owner}
	if from != nil {
		q += ` AND e.started_at >= ?`
		args = append(args, *from)
	}
	if to != nil {
		q += ` AND e.started_at < ?`
		args = append(args, *to)
	}
	if clientID != nil {
		q += ` AND e.client_id = ?`
		args = append(args, *clientID)
	}
	q += ` ORDER BY e.started_at DESC, e.id DESC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	now := time.Now().Unix()
	for rows.Next() {
		e, err := scanEntry(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Entry fetches a single entry by id, scoped to the owner.
func (s *Store) Entry(owner, id int64) (Entry, error) {
	row := s.db.QueryRow(`SELECT`+entryCols+entryFrom+` WHERE e.owner_id = ? AND e.id = ?`, owner, id)
	e, err := scanEntry(row, time.Now().Unix())
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

// EntryInput carries the mutable fields shared by create and update.
type EntryInput struct {
	ClientID    *int64
	MatterID    *int64
	WorkTypeID  *int64
	Description string
	StartedAt   int64
	DurationMin int64
	Billable    *bool // nil → inherit the work type's default
}

// CreateEntry records a completed (manual) block of time.
func (s *Store) CreateEntry(owner int64, in EntryInput) (Entry, error) {
	billable := s.resolveBillable(owner, in.WorkTypeID, in.Billable)
	if in.StartedAt == 0 {
		in.StartedAt = time.Now().Unix()
	}
	ended := in.StartedAt + in.DurationMin*60
	now := touchNow()
	res, err := s.db.Exec(
		`INSERT INTO time_entries
		   (owner_id, client_id, matter_id, work_type_id, description,
		    started_at, ended_at, duration_min, billable, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'manual', ?, ?)`,
		owner, in.ClientID, in.MatterID, in.WorkTypeID, in.Description,
		in.StartedAt, ended, in.DurationMin, boolToInt(billable), now, now)
	if err != nil {
		return Entry{}, err
	}
	id, _ := res.LastInsertId()
	return s.Entry(owner, id)
}

// UpdateEntry rewrites a completed entry's fields. It refuses to turn a
// running timer into a fixed block here — stop it first.
func (s *Store) UpdateEntry(owner, id int64, in EntryInput) (Entry, error) {
	existing, err := s.Entry(owner, id)
	if err != nil {
		return Entry{}, err
	}
	if existing.Running {
		return Entry{}, errors.New("cannot edit a running timer; stop it first")
	}
	billable := s.resolveBillable(owner, in.WorkTypeID, in.Billable)
	ended := in.StartedAt + in.DurationMin*60
	res, err := s.db.Exec(
		`UPDATE time_entries
		    SET client_id = ?, matter_id = ?, work_type_id = ?, description = ?,
		        started_at = ?, ended_at = ?, duration_min = ?, billable = ?, updated_at = ?
		  WHERE owner_id = ? AND id = ?`,
		in.ClientID, in.MatterID, in.WorkTypeID, in.Description,
		in.StartedAt, ended, in.DurationMin, boolToInt(billable), touchNow(), owner, id)
	if err != nil {
		return Entry{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Entry{}, ErrNotFound
	}
	return s.Entry(owner, id)
}

// DeleteEntry removes an entry (running or not).
func (s *Store) DeleteEntry(owner, id int64) error {
	res, err := s.db.Exec(`DELETE FROM time_entries WHERE owner_id = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- timer ----

// CurrentTimer returns the running entry, or ErrNotFound if none is running.
func (s *Store) CurrentTimer(owner int64) (Entry, error) {
	row := s.db.QueryRow(`SELECT`+entryCols+entryFrom+
		` WHERE e.owner_id = ? AND e.ended_at IS NULL`, owner)
	e, err := scanEntry(row, time.Now().Unix())
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

// StartTimer begins a new running entry. It returns ErrTimerRunning if one is
// already running (the unique partial index makes this race-free).
func (s *Store) StartTimer(owner int64, clientID, matterID, workTypeID *int64, desc string, billable *bool) (Entry, error) {
	b := s.resolveBillable(owner, workTypeID, billable)
	now := touchNow()
	res, err := s.db.Exec(
		`INSERT INTO time_entries
		   (owner_id, client_id, matter_id, work_type_id, description,
		    started_at, ended_at, duration_min, billable, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, 'timer', ?, ?)`,
		owner, clientID, matterID, workTypeID, desc, now, boolToInt(b), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return Entry{}, ErrTimerRunning
		}
		return Entry{}, err
	}
	id, _ := res.LastInsertId()
	return s.Entry(owner, id)
}

// StopTimer closes the running entry, computing its duration to the nearest
// minute. Returns ErrNotFound if nothing is running.
func (s *Store) StopTimer(owner int64) (Entry, error) {
	running, err := s.CurrentTimer(owner)
	if err != nil {
		return Entry{}, err
	}
	now := time.Now().Unix()
	dur := roundSecondsToMin(now - running.StartedAt)
	if _, err := s.db.Exec(
		`UPDATE time_entries SET ended_at = ?, duration_min = ?, updated_at = ?
		  WHERE owner_id = ? AND id = ?`,
		now, dur, now, owner, running.ID); err != nil {
		return Entry{}, err
	}
	return s.Entry(owner, running.ID)
}

// ---- helpers ----

func (s *Store) resolveBillable(owner int64, workTypeID *int64, override *bool) bool {
	if override != nil {
		return *override
	}
	if workTypeID != nil {
		return s.workTypeBillable(owner, *workTypeID)
	}
	return true
}

func scanEntry(sc scannable, now int64) (Entry, error) {
	var e Entry
	var clientID, matterID, workTypeID, endedAt, durationMin sql.NullInt64
	var billable int
	var clientName, workTypeName sql.NullString
	if err := sc.Scan(
		&e.ID, &clientID, &matterID, &workTypeID, &e.Description,
		&e.StartedAt, &endedAt, &durationMin, &billable, &e.Source,
		&e.CreatedAt, &e.UpdatedAt, &clientName, &workTypeName,
	); err != nil {
		return Entry{}, err
	}
	e.ClientID = nullInt(clientID)
	e.MatterID = nullInt(matterID)
	e.WorkTypeID = nullInt(workTypeID)
	e.EndedAt = nullInt(endedAt)
	e.DurationMin = nullInt(durationMin)
	e.Billable = billable == 1
	e.ClientName = clientName.String
	e.WorkTypeName = workTypeName.String

	if e.EndedAt == nil {
		e.Running = true
		e.EffectiveMin = roundSecondsToMin(now - e.StartedAt)
	} else if e.DurationMin != nil {
		e.EffectiveMin = *e.DurationMin
	}
	return e, nil
}

// roundSecondsToMin converts a duration in seconds to whole minutes, rounded
// to nearest, never negative.
func roundSecondsToMin(sec int64) int64 {
	if sec < 0 {
		sec = 0
	}
	return (sec + 30) / 60
}

func nullInt(n sql.NullInt64) *int64 {
	if n.Valid {
		v := n.Int64
		return &v
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
