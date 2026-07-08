package store

import (
	"database/sql"
	"errors"
	"time"
)

// Client is a person or company you do work for.
type Client struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code"`
	Notes      string `json:"notes"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	ArchivedAt *int64 `json:"archived_at,omitempty"`
}

// Clients lists the owner's clients. When activeOnly is true, archived
// clients are excluded. Ordered by name.
func (s *Store) Clients(owner int64, activeOnly bool) ([]Client, error) {
	q := `SELECT id, name, code, notes, status, created_at, archived_at
	        FROM clients WHERE owner_id = ?`
	if activeOnly {
		q += ` AND status = 'active'`
	}
	q += ` ORDER BY name COLLATE NOCASE`
	rows, err := s.db.Query(q, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Client fetches one client by id, scoped to the owner.
func (s *Store) Client(owner, id int64) (Client, error) {
	row := s.db.QueryRow(
		`SELECT id, name, code, notes, status, created_at, archived_at
		   FROM clients WHERE owner_id = ? AND id = ?`, owner, id)
	c, err := scanClient(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	return c, err
}

// CreateClient inserts a new active client and returns it.
func (s *Store) CreateClient(owner int64, name, code, notes string) (Client, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO clients (owner_id, name, code, notes, status, created_at)
		 VALUES (?, ?, ?, ?, 'active', ?)`,
		owner, name, code, notes, now)
	if err != nil {
		return Client{}, err
	}
	id, _ := res.LastInsertId()
	return s.Client(owner, id)
}

// UpdateClient updates the editable fields (name, code, notes, status). When
// status transitions to archived the archived_at stamp is set.
func (s *Store) UpdateClient(owner, id int64, name, code, notes, status string) (Client, error) {
	if status != "active" && status != "archived" {
		status = "active"
	}
	var archivedAt any
	if status == "archived" {
		archivedAt = time.Now().Unix()
	}
	res, err := s.db.Exec(
		`UPDATE clients
		    SET name = ?, code = ?, notes = ?, status = ?,
		        archived_at = CASE WHEN ? = 'archived' THEN ? ELSE NULL END
		  WHERE owner_id = ? AND id = ?`,
		name, code, notes, status, status, archivedAt, owner, id)
	if err != nil {
		return Client{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Client{}, ErrNotFound
	}
	return s.Client(owner, id)
}

// EnsureClient returns the id of the owner's active client with the given
// name (case-insensitive), creating it if none exists. Used by CSV import.
func (s *Store) EnsureClient(owner int64, name string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM clients WHERE owner_id = ? AND name = ? COLLATE NOCASE`,
		owner, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	c, err := s.CreateClient(owner, name, "", "")
	if err != nil {
		return 0, err
	}
	return c.ID, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanClient(sc scannable) (Client, error) {
	var c Client
	var archived sql.NullInt64
	if err := sc.Scan(&c.ID, &c.Name, &c.Code, &c.Notes, &c.Status, &c.CreatedAt, &archived); err != nil {
		return Client{}, err
	}
	if archived.Valid {
		c.ArchivedAt = &archived.Int64
	}
	return c, nil
}
