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
