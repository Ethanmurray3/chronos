package api

import (
	"net/http"

	"chronos/internal/store"
)

type templateBody struct {
	ClientID *int64 `json:"client_id"`
	Title    string `json:"title"`
	Notes    string `json:"notes"`
	Category string `json:"category"`
	Tags     string `json:"tags"`
}

func (b templateBody) input() store.TemplateInput {
	return store.TemplateInput{ClientID: b.ClientID, Title: b.Title, Notes: b.Notes, Category: b.Category, Tags: b.Tags}
}

// listTemplates returns the template library, category-grouped order. With
// ?q= it becomes the Templates page's own full-text search instead.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	if q := r.URL.Query().Get("q"); q != "" {
		hits, err := s.st.SearchTemplates(currentOwner(r), q, 20)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
		return
	}
	ts, err := s.st.Templates(currentOwner(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": ts})
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.st.Template(currentOwner(r), id)
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var b templateBody
	if !readJSON(w, r, &b) {
		return
	}
	if b.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	t, err := s.st.CreateTemplate(currentOwner(r), b.input())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b templateBody
	if !readJSON(w, r, &b) {
		return
	}
	t, err := s.st.UpdateTemplate(currentOwner(r), id, b.input())
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if handleLookupErr(w, s.st.DeleteTemplate(currentOwner(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// search is the full-text search over past coded work (time entries only —
// templates have their own search on GET /api/v1/templates?q=).
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	hits, err := s.st.SearchEntries(currentOwner(r), r.URL.Query().Get("q"), 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}
