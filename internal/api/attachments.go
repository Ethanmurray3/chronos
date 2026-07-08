package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxAttachmentBytes caps uploads (25 MB is generous for letters and PDFs).
const maxAttachmentBytes = 25 << 20

// allowedAttachmentExts is the professional-documents safelist.
var allowedAttachmentExts = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".txt": true, ".md": true, ".rtf": true,
	".csv": true, ".eml": true, ".msg": true,
}

// diskPath is where an attachment's bytes live: the id alone names the file,
// so user-supplied filenames never touch the filesystem.
func (s *Server) diskPath(id int64) string {
	return filepath.Join(s.filesDir, strconv.FormatInt(id, 10))
}

// uploadAttachment handles multipart POST /templates/{id}/attachments.
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	templateID, ok := pathID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes)
	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "upload too large or malformed: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedAttachmentExts[ext] {
		writeErr(w, http.StatusBadRequest, "file type not allowed: "+ext)
		return
	}
	mime := header.Header.Get("Content-Type")

	att, err := s.st.CreateAttachment(currentOwner(r), templateID, name, mime, header.Size)
	if handleLookupErr(w, err) {
		return
	}
	dst, err := os.Create(s.diskPath(att.ID))
	if err == nil {
		_, err = io.Copy(dst, file)
		if cerr := dst.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		// Keep DB and disk consistent: no bytes, no row.
		_ = s.st.DeleteAttachment(currentOwner(r), att.ID)
		_ = os.Remove(s.diskPath(att.ID))
		writeErr(w, http.StatusInternalServerError, "storing file failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, att)
}

// downloadAttachment streams the bytes with the original filename.
func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	att, err := s.st.Attachment(currentOwner(r), id)
	if handleLookupErr(w, err) {
		return
	}
	f, err := os.Open(s.diskPath(att.ID))
	if err != nil {
		writeErr(w, http.StatusNotFound, "file missing on disk")
		return
	}
	defer f.Close()

	mime := att.Mime
	if mime == "" {
		mime = "application/octet-stream"
	}
	// Sanitize the filename for the header; the disk path never used it.
	safe := strings.Map(func(c rune) rune {
		if c == '"' || c == '\\' || c < 32 {
			return '_'
		}
		return c
	}, att.Filename)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
	_, _ = io.Copy(w, f)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if handleLookupErr(w, s.st.DeleteAttachment(currentOwner(r), id)) {
		return
	}
	_ = os.Remove(s.diskPath(id)) // best-effort; the row is gone either way
	w.WriteHeader(http.StatusNoContent)
}
