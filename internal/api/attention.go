package api

import (
	"net/http"
	"sort"
	"time"

	"chronos/internal/store"
)

// attentionItem is an open todo plus the reasons it deserves attention.
// Reasons, most severe first: overdue, due-today, due-soon, high-priority,
// stale. An item can carry several.
type attentionItem struct {
	store.Todo
	AgeDays int64    `json:"age_days"`
	Reasons []string `json:"reasons"`
}

var reasonRank = map[string]int{
	"overdue":       0,
	"due-today":     1,
	"due-soon":      2,
	"high-priority": 3,
	"stale":         4,
}

// attention returns the owner's open todos that need attention, sorted by
// severity. This is the "what should I be worried about?" endpoint — the same
// view the UI panel shows and a future MCP tool will expose.
func (s *Server) attention(w http.ResponseWriter, r *http.Request) {
	owner := currentOwner(r)
	todos, err := s.st.ListTodos(owner, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	today, err := s.st.TodayISO(owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Thresholds are per-owner settings (stale_days, due_soon_days).
	set, err := s.st.Settings(owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	soonEnd := today
	if t, err := time.Parse("2006-01-02", today); err == nil {
		soonEnd = t.AddDate(0, 0, set.DueSoonDays).Format("2006-01-02")
	}

	now := time.Now().Unix()
	items := []attentionItem{}
	for _, td := range todos {
		age := (now - td.CreatedAt) / 86400
		var reasons []string
		if td.DueDate != nil {
			switch {
			case *td.DueDate < today:
				reasons = append(reasons, "overdue")
			case *td.DueDate == today:
				reasons = append(reasons, "due-today")
			case *td.DueDate <= soonEnd:
				reasons = append(reasons, "due-soon")
			}
		}
		if td.Priority > 0 {
			reasons = append(reasons, "high-priority")
		}
		if age >= int64(set.StaleDays) {
			reasons = append(reasons, "stale")
		}
		if len(reasons) > 0 {
			items = append(items, attentionItem{Todo: td, AgeDays: age, Reasons: reasons})
		}
	}

	// Severity = the best (lowest-ranked) reason; ties broken by due date
	// (soonest first, undated last), then by age (oldest first).
	best := func(it attentionItem) int { return reasonRank[it.Reasons[0]] }
	sort.SliceStable(items, func(i, j int) bool {
		if a, b := best(items[i]), best(items[j]); a != b {
			return a < b
		}
		di, dj := items[i].DueDate, items[j].DueDate
		switch {
		case di != nil && dj != nil && *di != *dj:
			return *di < *dj
		case di != nil && dj == nil:
			return true
		case di == nil && dj != nil:
			return false
		}
		return items[i].AgeDays > items[j].AgeDays
	})

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "today": today})
}
