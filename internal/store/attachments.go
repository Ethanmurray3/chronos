package store

import (
	"database/sql"
	"errors"
	"time"
)

// Attachment is a file (Word letter, PDF, worksheet) attached to a template.
// The store keeps metadata only; the API layer owns the bytes on disk.
type Attachment struct {
	ID         int64  `json:"id"`
	TemplateID int64  `json:"template_id"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	SizeBytes  int64  `json:"size_bytes"`
	CreatedAt  int64  `json:"created_at"`
}

// CreateAttachment records an attachment row for one of the owner's
// templates. The caller writes the bytes keyed by the returned id.
func (s *Store) CreateAttachment(owner, templateID int64, filename, mime string, size int64) (Attachment, error) {
	// The template must exist and belong to the owner.
	if _, err := s.Template(owner, templateID); err != nil {
		return Attachment{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO attachments (owner_id, template_id, filename, mime, size_bytes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		owner, templateID, filename, mime, size, now)
	if err != nil {
		return Attachment{}, err
	}
	id, _ := res.LastInsertId()
	return Attachment{ID: id, TemplateID: templateID, Filename: filename, Mime: mime, SizeBytes: size, CreatedAt: now}, nil
}

// Attachment fetches one attachment's metadata, scoped to the owner.
func (s *Store) Attachment(owner, id int64) (Attachment, error) {
	var a Attachment
	err := s.db.QueryRow(
		`SELECT id, template_id, filename, mime, size_bytes, created_at
		   FROM attachments WHERE owner_id = ? AND id = ?`, owner, id).
		Scan(&a.ID, &a.TemplateID, &a.Filename, &a.Mime, &a.SizeBytes, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return a, err
}

// AttachmentsForTemplate lists a template's attachments, oldest first.
func (s *Store) AttachmentsForTemplate(owner, templateID int64) ([]Attachment, error) {
	rows, err := s.db.Query(
		`SELECT id, template_id, filename, mime, size_bytes, created_at
		   FROM attachments WHERE owner_id = ? AND template_id = ? ORDER BY id`,
		owner, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.TemplateID, &a.Filename, &a.Mime, &a.SizeBytes, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAttachment removes the metadata row. The caller removes the bytes.
func (s *Store) DeleteAttachment(owner, id int64) error {
	res, err := s.db.Exec(`DELETE FROM attachments WHERE owner_id = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
