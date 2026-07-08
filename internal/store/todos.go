package store

import (
	"database/sql"
	"errors"
	"time"
)

// Todo is a task on the owner's list, optionally tied to a client/matter.
type Todo struct {
	ID          int64   `json:"id"`
	ClientID    *int64  `json:"client_id,omitempty"`
	MatterID    *int64  `json:"matter_id,omitempty"`
	Title       string  `json:"title"`
	Detail      string  `json:"detail"`
	DueDate     *string `json:"due_date,omitempty"` // YYYY-MM-DD
	Priority    int     `json:"priority"`           // 0 normal, 1 high
	Status      string  `json:"status"`             // open | done
	CreatedAt   int64   `json:"created_at"`
	CompletedAt *int64  `json:"completed_at,omitempty"`
	ClientName  string  `json:"client_name,omitempty"`
}

// TodoInput carries the mutable fields for create/update.
type TodoInput struct {
	ClientID *int64
	MatterID *int64
	Title    string
	Detail   string
	DueDate  *string
	Priority int
}

const todoCols = `
  t.id, t.client_id, t.matter_id, t.title, t.detail, t.due_date,
  t.priority, t.status, t.created_at, t.completed_at, c.name`

const todoFrom = `
  FROM todos t
  LEFT JOIN clients c ON c.id = t.client_id`

// ListTodos returns the owner's todos. When includeDone is false only open
// items are returned. Ordering: open before done, high priority first, then
// soonest due date (undated last), then oldest first.
func (s *Store) ListTodos(owner int64, includeDone bool) ([]Todo, error) {
	q := `SELECT` + todoCols + todoFrom + ` WHERE t.owner_id = ?`
	if !includeDone {
		q += ` AND t.status = 'open'`
	}
	q += ` ORDER BY (t.status = 'done'), t.priority DESC,
	         t.due_date IS NULL, t.due_date ASC, t.created_at ASC`
	rows, err := s.db.Query(q, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Todo{}
	for rows.Next() {
		td, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, td)
	}
	return out, rows.Err()
}

// Todo fetches one todo by id, scoped to the owner.
func (s *Store) Todo(owner, id int64) (Todo, error) {
	row := s.db.QueryRow(`SELECT`+todoCols+todoFrom+` WHERE t.owner_id = ? AND t.id = ?`, owner, id)
	td, err := scanTodo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	return td, err
}

// CreateTodo inserts a new open todo.
func (s *Store) CreateTodo(owner int64, in TodoInput) (Todo, error) {
	res, err := s.db.Exec(
		`INSERT INTO todos (owner_id, client_id, matter_id, title, detail, due_date, priority, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?)`,
		owner, in.ClientID, in.MatterID, in.Title, in.Detail, in.DueDate, in.Priority, time.Now().Unix())
	if err != nil {
		return Todo{}, err
	}
	id, _ := res.LastInsertId()
	return s.Todo(owner, id)
}

// UpdateTodo rewrites a todo's editable fields (not its status).
func (s *Store) UpdateTodo(owner, id int64, in TodoInput) (Todo, error) {
	res, err := s.db.Exec(
		`UPDATE todos SET client_id = ?, matter_id = ?, title = ?, detail = ?, due_date = ?, priority = ?
		  WHERE owner_id = ? AND id = ?`,
		in.ClientID, in.MatterID, in.Title, in.Detail, in.DueDate, in.Priority, owner, id)
	if err != nil {
		return Todo{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, ErrNotFound
	}
	return s.Todo(owner, id)
}

// SetTodoDone marks a todo done (stamping completed_at) or reopens it.
func (s *Store) SetTodoDone(owner, id int64, done bool) (Todo, error) {
	var status string
	var completed any
	if done {
		status, completed = "done", time.Now().Unix()
	} else {
		status, completed = "open", nil
	}
	res, err := s.db.Exec(
		`UPDATE todos SET status = ?, completed_at = ? WHERE owner_id = ? AND id = ?`,
		status, completed, owner, id)
	if err != nil {
		return Todo{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, ErrNotFound
	}
	return s.Todo(owner, id)
}

// DeleteTodo removes a todo.
func (s *Store) DeleteTodo(owner, id int64) error {
	res, err := s.db.Exec(`DELETE FROM todos WHERE owner_id = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanTodo(sc scannable) (Todo, error) {
	var t Todo
	var clientID, matterID, completedAt sql.NullInt64
	var dueDate, clientName sql.NullString
	if err := sc.Scan(
		&t.ID, &clientID, &matterID, &t.Title, &t.Detail, &dueDate,
		&t.Priority, &t.Status, &t.CreatedAt, &completedAt, &clientName,
	); err != nil {
		return Todo{}, err
	}
	t.ClientID = nullInt(clientID)
	t.MatterID = nullInt(matterID)
	t.CompletedAt = nullInt(completedAt)
	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	t.ClientName = clientName.String
	return t, nil
}
