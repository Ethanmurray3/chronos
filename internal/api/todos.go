package api

import (
	"net/http"

	"chronos/internal/store"
)

// todoBody is the JSON shape for creating/updating a todo.
type todoBody struct {
	ClientID *int64  `json:"client_id"`
	MatterID *int64  `json:"matter_id"`
	Title    string  `json:"title"`
	Detail   string  `json:"detail"`
	DueDate  *string `json:"due_date"`
	Priority int     `json:"priority"`
}

func (b todoBody) input() store.TodoInput {
	due := b.DueDate
	if due != nil && *due == "" {
		due = nil // treat an empty string as "no due date"
	}
	return store.TodoInput{
		ClientID: b.ClientID,
		MatterID: b.MatterID,
		Title:    b.Title,
		Detail:   b.Detail,
		DueDate:  due,
		Priority: b.Priority,
	}
}

func (s *Server) listTodos(w http.ResponseWriter, r *http.Request) {
	includeDone := r.URL.Query().Get("include_done") == "1"
	todos, err := s.st.ListTodos(currentOwner(r), includeDone)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todos": todos})
}

func (s *Server) createTodo(w http.ResponseWriter, r *http.Request) {
	var b todoBody
	if !readJSON(w, r, &b) {
		return
	}
	if b.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	td, err := s.st.CreateTodo(currentOwner(r), b.input())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, td)
}

func (s *Server) updateTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b todoBody
	if !readJSON(w, r, &b) {
		return
	}
	td, err := s.st.UpdateTodo(currentOwner(r), id, b.input())
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, td)
}

func (s *Server) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if handleLookupErr(w, s.st.DeleteTodo(currentOwner(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setTodoDone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Body optional: {"done": true|false}. Absent body means "mark done".
	body := struct {
		Done *bool `json:"done"`
	}{}
	if r.ContentLength != 0 {
		if !readJSON(w, r, &body) {
			return
		}
	}
	done := true
	if body.Done != nil {
		done = *body.Done
	}
	td, err := s.st.SetTodoDone(currentOwner(r), id, done)
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, td)
}

// startFromTodo starts a timer seeded from a todo (its client and title). If a
// timer is already running, it is stopped and logged before the new one starts.
func (s *Server) startFromTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	owner := currentOwner(r)
	td, err := s.st.Todo(owner, id)
	if handleLookupErr(w, err) {
		return
	}
	e, err := s.st.SwitchTimer(owner, td.ClientID, td.MatterID, nil, td.Title, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}
