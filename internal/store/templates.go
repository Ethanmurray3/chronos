package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Template is a reusable piece of work — a letter skeleton, a reorg step
// list, a checklist — kept in the library and full-text searchable.
type Template struct {
	ID            int64  `json:"id"`
	WorkTypeID    *int64 `json:"work_type_id,omitempty"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Tags          string `json:"tags"`
	SourceEntryID *int64 `json:"source_entry_id,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	WorkTypeName  string `json:"work_type_name,omitempty"`
}

// TemplateInput carries the mutable fields for create/update.
type TemplateInput struct {
	WorkTypeID *int64
	Title      string
	Body       string
	Tags       string
}

const tplCols = `
  t.id, t.work_type_id, t.title, t.body, t.tags, t.source_entry_id,
  t.created_at, t.updated_at, w.name`

const tplFrom = `
  FROM templates t
  LEFT JOIN work_types w ON w.id = t.work_type_id`

// Templates lists the owner's templates, most recently updated first.
func (s *Store) Templates(owner int64) ([]Template, error) {
	rows, err := s.db.Query(
		`SELECT`+tplCols+tplFrom+` WHERE t.owner_id = ? ORDER BY t.updated_at DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Template fetches one template by id, scoped to the owner.
func (s *Store) Template(owner, id int64) (Template, error) {
	row := s.db.QueryRow(`SELECT`+tplCols+tplFrom+` WHERE t.owner_id = ? AND t.id = ?`, owner, id)
	t, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	return t, err
}

// CreateTemplate inserts a new template.
func (s *Store) CreateTemplate(owner int64, in TemplateInput) (Template, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO templates (owner_id, work_type_id, title, body, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		owner, in.WorkTypeID, in.Title, in.Body, in.Tags, now, now)
	if err != nil {
		return Template{}, err
	}
	id, _ := res.LastInsertId()
	return s.Template(owner, id)
}

// UpdateTemplate rewrites a template's editable fields.
func (s *Store) UpdateTemplate(owner, id int64, in TemplateInput) (Template, error) {
	res, err := s.db.Exec(
		`UPDATE templates SET work_type_id = ?, title = ?, body = ?, tags = ?, updated_at = ?
		  WHERE owner_id = ? AND id = ?`,
		in.WorkTypeID, in.Title, in.Body, in.Tags, time.Now().Unix(), owner, id)
	if err != nil {
		return Template{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Template{}, ErrNotFound
	}
	return s.Template(owner, id)
}

// DeleteTemplate removes a template.
func (s *Store) DeleteTemplate(owner, id int64) error {
	res, err := s.db.Exec(`DELETE FROM templates WHERE owner_id = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateTemplateFromEntry seeds a template from a past time entry: the
// entry's description becomes the body, its work type carries over, and the
// client name lands in tags so searches keep finding it.
func (s *Store) CreateTemplateFromEntry(owner, entryID int64) (Template, error) {
	e, err := s.Entry(owner, entryID)
	if err != nil {
		return Template{}, err
	}
	title := e.WorkTypeName
	if title == "" {
		title = "Saved entry"
	}
	if e.ClientName != "" {
		title += " — " + e.ClientName
	}
	title += " (" + time.Unix(e.StartedAt, 0).Format("Jan 2, 2006") + ")"

	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO templates (owner_id, work_type_id, title, body, tags, source_entry_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, e.WorkTypeID, title, e.Description, strings.ToLower(e.ClientName), entryID, now, now)
	if err != nil {
		return Template{}, err
	}
	id, _ := res.LastInsertId()
	return s.Template(owner, id)
}

func scanTemplate(sc scannable) (Template, error) {
	var t Template
	var workTypeID, sourceEntryID sql.NullInt64
	var workTypeName sql.NullString
	if err := sc.Scan(
		&t.ID, &workTypeID, &t.Title, &t.Body, &t.Tags, &sourceEntryID,
		&t.CreatedAt, &t.UpdatedAt, &workTypeName,
	); err != nil {
		return Template{}, err
	}
	t.WorkTypeID = nullInt(workTypeID)
	t.SourceEntryID = nullInt(sourceEntryID)
	t.WorkTypeName = workTypeName.String
	return t, nil
}

// ---- full-text search ----

// SearchHit is one result from the library search — either a template or a
// past time entry.
type SearchHit struct {
	Kind         string `json:"kind"` // template | entry
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Snippet      string `json:"snippet"` // matched text with <b> marks
	ClientName   string `json:"client_name,omitempty"`
	WorkTypeName string `json:"work_type_name,omitempty"`
	Tags         string `json:"tags,omitempty"`
	Date         string `json:"date,omitempty"` // entry start date, YYYY-MM-DD
}

// Search runs a full-text query across templates and entry descriptions,
// templates first, each ranked by FTS5 relevance.
func (s *Store) Search(owner int64, q string, limit int) ([]SearchHit, error) {
	match := ftsQuery(q)
	if match == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	out := []SearchHit{}

	tRows, err := s.db.Query(`
		SELECT t.id, t.title, snippet(templates_fts, 1, '<b>', '</b>', '…', 14), t.tags, COALESCE(w.name, '')
		  FROM templates_fts f
		  JOIN templates t ON t.id = f.rowid
		  LEFT JOIN work_types w ON w.id = t.work_type_id
		 WHERE templates_fts MATCH ? AND t.owner_id = ?
		 ORDER BY rank LIMIT ?`, match, owner, limit)
	if err != nil {
		return nil, fmt.Errorf("template search: %w", err)
	}
	defer tRows.Close()
	for tRows.Next() {
		h := SearchHit{Kind: "template"}
		if err := tRows.Scan(&h.ID, &h.Title, &h.Snippet, &h.Tags, &h.WorkTypeName); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := tRows.Err(); err != nil {
		return nil, err
	}

	eRows, err := s.db.Query(`
		SELECT e.id, snippet(entries_fts, 0, '<b>', '</b>', '…', 14),
		       COALESCE(c.name, ''), COALESCE(w.name, ''), e.started_at
		  FROM entries_fts f
		  JOIN time_entries e ON e.id = f.rowid
		  LEFT JOIN clients c ON c.id = e.client_id
		  LEFT JOIN work_types w ON w.id = e.work_type_id
		 WHERE entries_fts MATCH ? AND e.owner_id = ?
		 ORDER BY rank LIMIT ?`, match, owner, limit)
	if err != nil {
		return nil, fmt.Errorf("entry search: %w", err)
	}
	defer eRows.Close()
	for eRows.Next() {
		h := SearchHit{Kind: "entry"}
		var startedAt int64
		if err := eRows.Scan(&h.ID, &h.Snippet, &h.ClientName, &h.WorkTypeName, &startedAt); err != nil {
			return nil, err
		}
		h.Title = h.WorkTypeName
		if h.Title == "" {
			h.Title = "Time entry"
		}
		h.Date = time.Unix(startedAt, 0).Format("2006-01-02")
		out = append(out, h)
	}
	return out, eRows.Err()
}

// ftsQuery turns free text into a safe FTS5 MATCH expression: each term is
// quoted (so FTS syntax characters can't break the query) and the last term
// matches as a prefix, which makes search-as-you-type feel right.
func ftsQuery(q string) string {
	fields := strings.Fields(q)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		if f != "" {
			parts = append(parts, `"`+f+`"`)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	parts[len(parts)-1] += "*"
	return strings.Join(parts, " ")
}
