// Package api exposes the Chronos REST surface over a store.Store. It is the
// single contract the web UI (and, later, an MCP server) speak to.
//
// Auth is deliberately a stub for now: currentOwner always returns the seeded
// single user. When real accounts land, only that function and a login flow
// change — every handler already scopes its work to an owner id.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"chronos/internal/store"
)

// Server holds the dependencies shared by the handlers.
type Server struct {
	st *store.Store
}

// New builds a Server over the given store.
func New(st *store.Store) *Server { return &Server{st: st} }

// Routes returns a mux with every /api/v1 endpoint mounted. Static file
// serving for the UI is wired up by the caller (main).
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.putSettings)

	mux.HandleFunc("GET /api/v1/work-types", s.listWorkTypes)
	mux.HandleFunc("POST /api/v1/work-types", s.createWorkType)

	mux.HandleFunc("GET /api/v1/clients", s.listClients)
	mux.HandleFunc("POST /api/v1/clients", s.createClient)
	mux.HandleFunc("GET /api/v1/clients/{id}", s.getClient)
	mux.HandleFunc("PUT /api/v1/clients/{id}", s.updateClient)

	mux.HandleFunc("GET /api/v1/time-entries", s.listEntries)
	mux.HandleFunc("POST /api/v1/time-entries", s.createEntry)
	mux.HandleFunc("PUT /api/v1/time-entries/{id}", s.updateEntry)
	mux.HandleFunc("DELETE /api/v1/time-entries/{id}", s.deleteEntry)

	mux.HandleFunc("GET /api/v1/attention", s.attention)

	mux.HandleFunc("GET /api/v1/todos", s.listTodos)
	mux.HandleFunc("POST /api/v1/todos", s.createTodo)
	mux.HandleFunc("PUT /api/v1/todos/{id}", s.updateTodo)
	mux.HandleFunc("DELETE /api/v1/todos/{id}", s.deleteTodo)
	mux.HandleFunc("POST /api/v1/todos/{id}/done", s.setTodoDone)
	mux.HandleFunc("POST /api/v1/todos/{id}/start", s.startFromTodo)

	mux.HandleFunc("GET /api/v1/timer", s.getTimer)
	mux.HandleFunc("POST /api/v1/timer/start", s.startTimer)
	mux.HandleFunc("POST /api/v1/timer/stop", s.stopTimer)

	mux.HandleFunc("GET /api/v1/reports/day", s.reportDay)
	mux.HandleFunc("GET /api/v1/reports/summary", s.reportSummary)

	mux.HandleFunc("GET /api/v1/export.csv", s.exportCSV)
	mux.HandleFunc("POST /api/v1/import", s.importCSV)

	return mux
}

// currentOwner is the single-user auth stub.
func currentOwner(_ *http.Request) int64 { return store.SeedOwner }

// ---- settings ----

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.st.Settings(currentOwner(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.st.SaveSettings(currentOwner(r), in); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	set, _ := s.st.Settings(currentOwner(r))
	writeJSON(w, http.StatusOK, set)
}

// ---- work types ----

func (s *Server) listWorkTypes(w http.ResponseWriter, r *http.Request) {
	wts, err := s.st.WorkTypes(currentOwner(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work_types": wts})
}

func (s *Server) createWorkType(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Billable bool   `json:"billable_default"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	wt, err := s.st.CreateWorkType(currentOwner(r), in.Name, in.Category, in.Billable)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, wt)
}

// ---- clients ----

func (s *Server) listClients(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("all") != "1"
	cs, err := s.st.Clients(currentOwner(r), activeOnly)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": cs})
}

func (s *Server) createClient(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Code  string `json:"code"`
		Notes string `json:"notes"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	c, err := s.st.CreateClient(currentOwner(r), in.Name, in.Code, in.Notes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := s.st.Client(currentOwner(r), id)
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) updateClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name   string `json:"name"`
		Code   string `json:"code"`
		Notes  string `json:"notes"`
		Status string `json:"status"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c, err := s.st.UpdateClient(currentOwner(r), id, in.Name, in.Code, in.Notes, in.Status)
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ---- time entries ----

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := queryInt(q.Get("from"))
	to := queryInt(q.Get("to"))
	clientID := queryInt(q.Get("client_id"))
	es, err := s.st.ListEntries(currentOwner(r), from, to, clientID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": es})
}

// entryBody is the JSON shape accepted for creating/updating an entry.
type entryBody struct {
	ClientID    *int64 `json:"client_id"`
	MatterID    *int64 `json:"matter_id"`
	WorkTypeID  *int64 `json:"work_type_id"`
	Description string `json:"description"`
	StartedAt   int64  `json:"started_at"`
	DurationMin int64  `json:"duration_min"`
	Billable    *bool  `json:"billable"`
}

func (b entryBody) input() store.EntryInput {
	return store.EntryInput{
		ClientID:    b.ClientID,
		MatterID:    b.MatterID,
		WorkTypeID:  b.WorkTypeID,
		Description: b.Description,
		StartedAt:   b.StartedAt,
		DurationMin: b.DurationMin,
		Billable:    b.Billable,
	}
}

func (s *Server) createEntry(w http.ResponseWriter, r *http.Request) {
	var b entryBody
	if !readJSON(w, r, &b) {
		return
	}
	e, err := s.st.CreateEntry(currentOwner(r), b.input())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) updateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b entryBody
	if !readJSON(w, r, &b) {
		return
	}
	e, err := s.st.UpdateEntry(currentOwner(r), id, b.input())
	if handleLookupErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if handleLookupErr(w, s.st.DeleteEntry(currentOwner(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- timer ----

func (s *Server) getTimer(w http.ResponseWriter, r *http.Request) {
	e, err := s.st.CurrentTimer(currentOwner(r))
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"running": false})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": true, "entry": e})
}

func (s *Server) startTimer(w http.ResponseWriter, r *http.Request) {
	var b entryBody
	if !readJSON(w, r, &b) {
		return
	}
	e, err := s.st.StartTimer(currentOwner(r), b.ClientID, b.MatterID, b.WorkTypeID, b.Description, b.Billable)
	if errors.Is(err, store.ErrTimerRunning) {
		writeErr(w, http.StatusConflict, "a timer is already running")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) stopTimer(w http.ResponseWriter, r *http.Request) {
	e, err := s.st.StopTimer(currentOwner(r))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusConflict, "no timer is running")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// ---- reports ----

func (s *Server) reportDay(w http.ResponseWriter, r *http.Request) {
	rep, err := s.st.DayReport(currentOwner(r), r.URL.Query().Get("date"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) reportSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, err := s.st.SummaryReport(currentOwner(r), q.Get("from"), q.Get("to"), q.Get("group_by"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// ---- shared helpers ----

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Response is already committed; nothing useful to do but note it.
		return
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// readJSON decodes the request body into v, writing a 400 and returning false
// on failure.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// pathID parses the {id} path segment, writing a 400 on failure.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// queryInt parses an optional integer query param, returning nil when empty
// or unparseable.
func queryInt(s string) *int64 {
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// handleLookupErr writes 404 for ErrNotFound, 500 for other errors, and
// returns true when it wrote a response (so the caller should stop).
func handleLookupErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
	return true
}
