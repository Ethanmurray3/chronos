package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chronos/internal/store"
)

// csvHeader is the column order for both export and import.
var csvHeader = []string{"date", "start", "end", "client", "work_type", "description", "minutes", "billable", "source"}

// exportCSV streams the owner's entries in [from, to] (inclusive days, default
// today) as CSV — the round-trip format for import.
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	owner := currentOwner(r)
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" && to == "" {
		from, to = "", "" // both empty → DateBounds treats as today..today
	}
	start, end, err := s.st.DateBounds(owner, from, to)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := s.st.ListEntries(owner, &start, &end, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="chronos-export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	for _, e := range entries {
		started := time.Unix(e.StartedAt, 0).Local()
		endStr := ""
		if e.EndedAt != nil {
			endStr = time.Unix(*e.EndedAt, 0).Local().Format("15:04")
		}
		_ = cw.Write([]string{
			started.Format("2006-01-02"),
			started.Format("15:04"),
			endStr,
			e.ClientName,
			e.WorkTypeName,
			e.Description,
			strconv.FormatInt(e.EffectiveMin, 10),
			strconv.FormatBool(e.Billable),
			e.Source,
		})
	}
	cw.Flush()
}

// importCSV ingests a CSV body (the export shape; only date + minutes are
// required). Unknown clients are created; unknown work types are left blank.
// Rows start at 09:00 local on their date unless the file carries a start time.
func (s *Server) importCSV(w http.ResponseWriter, r *http.Request) {
	owner := currentOwner(r)
	cr := csv.NewReader(r.Body)
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not parse CSV: "+err.Error())
		return
	}
	if len(records) < 2 {
		writeErr(w, http.StatusBadRequest, "CSV has no data rows")
		return
	}

	col := map[string]int{}
	for i, name := range records[0] {
		col[strings.ToLower(strings.TrimSpace(name))] = i
	}
	if _, ok := col["date"]; !ok {
		writeErr(w, http.StatusBadRequest, "CSV must have a 'date' column")
		return
	}
	if _, ok := col["minutes"]; !ok {
		writeErr(w, http.StatusBadRequest, "CSV must have a 'minutes' column")
		return
	}

	field := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	imported := 0
	var problems []string
	for n, row := range records[1:] {
		lineNo := n + 2 // 1-based, plus header
		date := field(row, "date")
		minutes, mErr := strconv.ParseInt(field(row, "minutes"), 10, 64)
		if date == "" || mErr != nil || minutes <= 0 {
			problems = append(problems, fmt.Sprintf("line %d: bad date/minutes", lineNo))
			continue
		}
		dayStart, _, err := s.st.DateBounds(owner, date, date)
		if err != nil {
			problems = append(problems, fmt.Sprintf("line %d: %v", lineNo, err))
			continue
		}
		startedAt := dayStart + 9*3600 // nominal 09:00 local start
		if hm := field(row, "start"); hm != "" {
			if t, err := time.Parse("15:04", hm); err == nil {
				startedAt = dayStart + int64(t.Hour())*3600 + int64(t.Minute())*60
			}
		}

		var clientID *int64
		if name := field(row, "client"); name != "" {
			if id, err := s.st.EnsureClient(owner, name); err == nil {
				clientID = &id
			}
		}
		workTypeID := s.st.WorkTypeIDByName(owner, field(row, "work_type"))
		billable := parseBillable(field(row, "billable"))

		if _, err := s.st.CreateEntry(owner, store.EntryInput{
			ClientID:    clientID,
			WorkTypeID:  workTypeID,
			Description: field(row, "description"),
			StartedAt:   startedAt,
			DurationMin: minutes,
			Billable:    billable,
		}); err != nil {
			problems = append(problems, fmt.Sprintf("line %d: %v", lineNo, err))
			continue
		}
		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"problems": problems,
	})
}

// csvTable parses a CSV body into a header-index map and data rows, writing a
// 400 and returning ok=false when it is empty or malformed.
func csvTable(w http.ResponseWriter, r *http.Request) (col map[string]int, rows [][]string, ok bool) {
	cr := csv.NewReader(r.Body)
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not parse CSV: "+err.Error())
		return nil, nil, false
	}
	if len(records) < 2 {
		writeErr(w, http.StatusBadRequest, "CSV has no data rows")
		return nil, nil, false
	}
	col = map[string]int{}
	for i, name := range records[0] {
		col[strings.ToLower(strings.TrimSpace(name))] = i
	}
	return col, records[1:], true
}

func csvField(col map[string]int, row []string, names ...string) string {
	for _, n := range names {
		if i, ok := col[n]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
	}
	return ""
}

// importClientsCSV upserts clients from a firm-system export. Columns: name
// (required) and the client number under any of: code, number, client number,
// client code. Matching prefers the number, then the name.
func (s *Server) importClientsCSV(w http.ResponseWriter, r *http.Request) {
	owner := currentOwner(r)
	col, rows, ok := csvTable(w, r)
	if !ok {
		return
	}
	if _, has := col["name"]; !has {
		writeErr(w, http.StatusBadRequest, "CSV must have a 'name' column")
		return
	}
	created, updated := 0, 0
	var problems []string
	for n, row := range rows {
		name := csvField(col, row, "name", "client name", "client")
		code := csvField(col, row, "code", "number", "client number", "client code", "client no")
		if name == "" {
			problems = append(problems, fmt.Sprintf("line %d: missing name", n+2))
			continue
		}
		isNew, err := s.st.UpsertClient(owner, name, code)
		if err != nil {
			problems = append(problems, fmt.Sprintf("line %d: %v", n+2, err))
			continue
		}
		if isNew {
			created++
		} else {
			updated++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "problems": problems})
}

// importWorkTypesCSV upserts work types (billing types). Columns: name
// (required), category, billable (truthy/falsey; default billable).
func (s *Server) importWorkTypesCSV(w http.ResponseWriter, r *http.Request) {
	owner := currentOwner(r)
	col, rows, ok := csvTable(w, r)
	if !ok {
		return
	}
	if _, has := col["name"]; !has {
		writeErr(w, http.StatusBadRequest, "CSV must have a 'name' column")
		return
	}
	created, updated := 0, 0
	var problems []string
	for n, row := range rows {
		name := csvField(col, row, "name", "work type", "billing type", "type")
		if name == "" {
			problems = append(problems, fmt.Sprintf("line %d: missing name", n+2))
			continue
		}
		billable := true
		if b := parseBillable(csvField(col, row, "billable")); b != nil {
			billable = *b
		}
		isNew, err := s.st.UpsertWorkType(owner, name, csvField(col, row, "category"), billable)
		if err != nil {
			problems = append(problems, fmt.Sprintf("line %d: %v", n+2, err))
			continue
		}
		if isNew {
			created++
		} else {
			updated++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "problems": problems})
}

// parseBillable maps common truthy/falsey spellings; empty → nil (inherit the
// work type default).
func parseBillable(s string) *bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "y", "billable":
		b := true
		return &b
	case "0", "false", "no", "n", "non-billable":
		b := false
		return &b
	}
	return nil
}
