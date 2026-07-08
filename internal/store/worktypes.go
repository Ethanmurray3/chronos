package store

import "time"

// WorkType is a category of work — the "what I did" taxonomy that makes the
// history searchable and reportable.
type WorkType struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	BillableDefault bool   `json:"billable_default"`
	Sort            int    `json:"sort"`
}

// WorkTypes lists the owner's work types in display order.
func (s *Store) WorkTypes(owner int64) ([]WorkType, error) {
	rows, err := s.db.Query(
		`SELECT id, name, category, billable_default, sort
		   FROM work_types WHERE owner_id = ?
		  ORDER BY sort, name COLLATE NOCASE`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkType{}
	for rows.Next() {
		var w WorkType
		var billable int
		if err := rows.Scan(&w.ID, &w.Name, &w.Category, &billable, &w.Sort); err != nil {
			return nil, err
		}
		w.BillableDefault = billable == 1
		out = append(out, w)
	}
	return out, rows.Err()
}

// CreateWorkType adds a new work type at the end of the sort order.
func (s *Store) CreateWorkType(owner int64, name, category string, billable bool) (WorkType, error) {
	b := 0
	if billable {
		b = 1
	}
	var maxSort int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(sort)+1, 0) FROM work_types WHERE owner_id = ?`, owner).Scan(&maxSort)
	res, err := s.db.Exec(
		`INSERT INTO work_types (owner_id, name, category, billable_default, sort)
		 VALUES (?, ?, ?, ?, ?)`,
		owner, name, category, b, maxSort)
	if err != nil {
		return WorkType{}, err
	}
	id, _ := res.LastInsertId()
	return WorkType{ID: id, Name: name, Category: category, BillableDefault: billable, Sort: maxSort}, nil
}

// UpdateWorkType renames/recategorizes a work type and sets its billable
// default.
func (s *Store) UpdateWorkType(owner, id int64, name, category string, billable bool) (WorkType, error) {
	res, err := s.db.Exec(
		`UPDATE work_types SET name = ?, category = ?, billable_default = ?
		  WHERE owner_id = ? AND id = ?`,
		name, category, boolToInt(billable), owner, id)
	if err != nil {
		return WorkType{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return WorkType{}, ErrNotFound
	}
	var w WorkType
	var b int
	err = s.db.QueryRow(
		`SELECT id, name, category, billable_default, sort FROM work_types
		  WHERE owner_id = ? AND id = ?`, owner, id).
		Scan(&w.ID, &w.Name, &w.Category, &b, &w.Sort)
	w.BillableDefault = b == 1
	return w, err
}

// DeleteWorkType removes a work type, refusing (ErrInUse) while time entries
// or templates still reference it — history must stay attributable.
func (s *Store) DeleteWorkType(owner, id int64) error {
	var n int
	if err := s.db.QueryRow(
		`SELECT (SELECT COUNT(*) FROM time_entries WHERE owner_id = ?1 AND work_type_id = ?2)
		      + (SELECT COUNT(*) FROM templates    WHERE owner_id = ?1 AND work_type_id = ?2)`,
		owner, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrInUse
	}
	res, err := s.db.Exec(`DELETE FROM work_types WHERE owner_id = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertWorkType matches by name (case-insensitive) and updates category and
// billable default, or creates the work type. Used by CSV import. Returns
// true when a new row was created.
func (s *Store) UpsertWorkType(owner int64, name, category string, billable bool) (bool, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM work_types WHERE owner_id = ? AND name = ? COLLATE NOCASE`,
		owner, name).Scan(&id)
	if err == nil {
		_, err = s.UpdateWorkType(owner, id, name, category, billable)
		return false, err
	}
	if _, err := s.CreateWorkType(owner, name, category, billable); err != nil {
		return false, err
	}
	return true, nil
}

// workTypeBillable reports the billable default for a work type, used when an
// entry does not specify billable explicitly. Missing types default to true.
func (s *Store) workTypeBillable(owner, id int64) bool {
	var b int
	if err := s.db.QueryRow(
		`SELECT billable_default FROM work_types WHERE owner_id = ? AND id = ?`,
		owner, id).Scan(&b); err != nil {
		return true
	}
	return b == 1
}

// WorkTypeIDByName returns the id of the owner's work type matching name
// (case-insensitive), or nil if none matches. Used by CSV import.
func (s *Store) WorkTypeIDByName(owner int64, name string) *int64 {
	if name == "" {
		return nil
	}
	var id int64
	if err := s.db.QueryRow(
		`SELECT id FROM work_types WHERE owner_id = ? AND name = ? COLLATE NOCASE`,
		owner, name).Scan(&id); err != nil {
		return nil
	}
	return &id
}

// touchNow is a tiny shared helper for updated_at stamps.
func touchNow() int64 { return time.Now().Unix() }
