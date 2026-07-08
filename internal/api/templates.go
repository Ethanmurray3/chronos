package api

import (
	"net/http"

	"chronos/internal/store"
)

type templateBody struct {
	WorkTypeID *int64 `json:"work_type_id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Tags       string `json:"tags"`
}

func (b templateBody) input() store.TemplateInput {
	return store.TemplateInput{WorkTypeID: b.WorkTypeID, Title: b.Title, Body: b.Body, Tags: b.Tags}
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
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

// templateFromEntry turns a past time entry into a library template.
func (s *Server) templateFromEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.st.CreateTemplateFromEntry(currentOwner(r), id)
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// search is the library's full-text search across templates and entries.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	hits, err := s.st.Search(currentOwner(r), r.URL.Query().Get("q"), 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}
