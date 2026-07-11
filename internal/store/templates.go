package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Template is a saved deliverable in the library — a finished letter,
// worksheet, or step list kept for reuse. The attached files are the point;
// Notes describe what they contain and when to reach for them. Templates are
// organized by free-text category and may carry a label-only client
// reference ("originally for Acme Inc.").
type Template struct {
	ID         int64  `json:"id"`
	ClientID   *int64 `json:"client_id,omitempty"`
	Title      string `json:"title"`
	Notes      string `json:"notes"` // stored in the body column (FTS is bound to that name)
	Category   string `json:"category"`
	Tags       string `json:"tags"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	ClientName string `json:"client_name,omitempty"`

	// AttachmentCount lets list views show how many files a template carries
	// without fetching each one.
	AttachmentCount int64 `json:"attachment_count"`

	// Attachments is populated on single-template fetches only.
	Attachments []Attachment `json:"attachments,omitempty"`
}

// TemplateInput carries the mutable fields for create/update.
type TemplateInput struct {
	ClientID *int64
	Title    string
	Notes    string
	Category string
	Tags     string
}

const tplCols = `
  t.id, t.client_id, t.title, t.body, t.category, t.tags,
  t.created_at, t.updated_at, c.name,
  (SELECT COUNT(*) FROM attachments a WHERE a.template_id = t.id)`

const tplFrom = `
  FROM templates t
  LEFT JOIN clients c ON c.id = t.client_id`

// Templates lists the owner's templates grouped for the Templates page:
// alphabetical by category (uncategorized last), newest first within each.
func (s *Store) Templates(owner int64) ([]Template, error) {
	rows, err := s.db.Query(
		`SELECT`+tplCols+tplFrom+` WHERE t.owner_id = ?
		 ORDER BY t.category = '', t.category COLLATE NOCASE, t.updated_at DESC`, owner)
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

// Template fetches one template by id (with its attachments), scoped to the
// owner.
func (s *Store) Template(owner, id int64) (Template, error) {
	row := s.db.QueryRow(`SELECT`+tplCols+tplFrom+` WHERE t.owner_id = ? AND t.id = ?`, owner, id)
	t, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	if t.Attachments, err = s.AttachmentsForTemplate(owner, id); err != nil {
		return Template{}, err
	}
	return t, nil
}

// CreateTemplate inserts a new template.
func (s *Store) CreateTemplate(owner int64, in TemplateInput) (Template, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO templates (owner_id, client_id, title, body, category, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, in.ClientID, in.Title, in.Notes, strings.TrimSpace(in.Category), in.Tags, now, now)
	if err != nil {
		return Template{}, err
	}
	id, _ := res.LastInsertId()
	return s.Template(owner, id)
}

// UpdateTemplate rewrites a template's editable fields.
func (s *Store) UpdateTemplate(owner, id int64, in TemplateInput) (Template, error) {
	res, err := s.db.Exec(
		`UPDATE templates SET client_id = ?, title = ?, body = ?, category = ?, tags = ?, updated_at = ?
		  WHERE owner_id = ? AND id = ?`,
		in.ClientID, in.Title, in.Notes, strings.TrimSpace(in.Category), in.Tags, time.Now().Unix(), owner, id)
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

func scanTemplate(sc scannable) (Template, error) {
	var t Template
	var clientID sql.NullInt64
	var clientName sql.NullString
	if err := sc.Scan(
		&t.ID, &clientID, &t.Title, &t.Notes, &t.Category, &t.Tags,
		&t.CreatedAt, &t.UpdatedAt, &clientName, &t.AttachmentCount,
	); err != nil {
		return Template{}, err
	}
	t.ClientID = nullInt(clientID)
	t.ClientName = clientName.String
	return t, nil
}

// ---- full-text search ----

// SearchHit is one result from a full-text search — a template or a past
// time entry, depending on which search produced it.
type SearchHit struct {
	Kind       string `json:"kind"` // template | entry
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"` // matched text with <b> marks
	ClientName string `json:"client_name,omitempty"`
	Category   string `json:"category,omitempty"`
	Tags       string `json:"tags,omitempty"`
	Date       string `json:"date,omitempty"` // entry start date, YYYY-MM-DD
}

// SearchTemplates runs a full-text query over template titles, notes, and
// tags, ranked by FTS5 relevance.
func (s *Store) SearchTemplates(owner int64, q string, limit int) ([]SearchHit, error) {
	match := ftsQuery(q)
	if match == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT t.id, t.title, snippet(templates_fts, 1, '<b>', '</b>', '…', 14),
		       t.category, t.tags, COALESCE(c.name, '')
		  FROM templates_fts f
		  JOIN templates t ON t.id = f.rowid
		  LEFT JOIN clients c ON c.id = t.client_id
		 WHERE templates_fts MATCH ? AND t.owner_id = ?
		 ORDER BY rank LIMIT ?`, match, owner, limit)
	if err != nil {
		return nil, fmt.Errorf("template search: %w", err)
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		h := SearchHit{Kind: "template"}
		if err := rows.Scan(&h.ID, &h.Title, &h.Snippet, &h.Category, &h.Tags, &h.ClientName); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchEntries runs a full-text query over past time-entry descriptions,
// ranked by FTS5 relevance.
func (s *Store) SearchEntries(owner int64, q string, limit int) ([]SearchHit, error) {
	match := ftsQuery(q)
	if match == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Query(`
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
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		h := SearchHit{Kind: "entry"}
		var workTypeName string
		var startedAt int64
		if err := rows.Scan(&h.ID, &h.Snippet, &h.ClientName, &workTypeName, &startedAt); err != nil {
			return nil, err
		}
		h.Title = workTypeName
		if h.Title == "" {
			h.Title = "Time entry"
		}
		h.Date = time.Unix(startedAt, 0).Format("2006-01-02")
		out = append(out, h)
	}
	return out, rows.Err()
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
