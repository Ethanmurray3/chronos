package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Tool is one MCP tool: its wire definition plus the handler that serves it.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`

	handler func(*APIClient, map[string]any) (string, error) `json:"-"`
}

// ---- schema helpers ----

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any  { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any  { return map[string]any{"type": "number", "description": desc} }
func boolp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// ---- argument helpers ----

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func argNum(args map[string]any, key string) (float64, bool) {
	v, ok := args[key].(float64)
	return v, ok
}

func argBool(args map[string]any, key string) (bool, bool) {
	v, ok := args[key].(bool)
	return v, ok
}

func pretty(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}

// ---- name resolution against the live catalogs ----

type namedRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// resolveClient matches by client code first (the firm's number is exact),
// then case-insensitive name, then unique substring.
func resolveClient(c *APIClient, q string) (*int64, string, error) {
	if q == "" {
		return nil, "", nil
	}
	var payload struct {
		Clients []namedRef `json:"clients"`
	}
	if err := c.get("/api/v1/clients", &payload); err != nil {
		return nil, "", err
	}
	lower := strings.ToLower(q)
	var sub []namedRef
	for _, cl := range payload.Clients {
		if strings.EqualFold(cl.Code, q) || strings.EqualFold(cl.Name, q) {
			return &cl.ID, cl.Name, nil
		}
		if strings.Contains(strings.ToLower(cl.Name), lower) {
			sub = append(sub, cl)
		}
	}
	if len(sub) == 1 {
		return &sub[0].ID, sub[0].Name, nil
	}
	names := make([]string, 0, len(payload.Clients))
	for _, cl := range payload.Clients {
		names = append(names, cl.Name)
	}
	sort.Strings(names)
	if len(sub) > 1 {
		return nil, "", fmt.Errorf("client %q is ambiguous; candidates: %s", q, joinNames(sub))
	}
	return nil, "", fmt.Errorf("no client matches %q; known clients: %s", q, strings.Join(names, ", "))
}

func resolveWorkType(c *APIClient, q string) (*int64, string, error) {
	if q == "" {
		return nil, "", nil
	}
	var payload struct {
		WorkTypes []namedRef `json:"work_types"`
	}
	if err := c.get("/api/v1/work-types", &payload); err != nil {
		return nil, "", err
	}
	lower := strings.ToLower(q)
	var sub []namedRef
	for _, wt := range payload.WorkTypes {
		if strings.EqualFold(wt.Name, q) {
			return &wt.ID, wt.Name, nil
		}
		if strings.Contains(strings.ToLower(wt.Name), lower) {
			sub = append(sub, wt)
		}
	}
	if len(sub) == 1 {
		return &sub[0].ID, sub[0].Name, nil
	}
	names := make([]string, 0, len(payload.WorkTypes))
	for _, wt := range payload.WorkTypes {
		names = append(names, wt.Name)
	}
	if len(sub) > 1 {
		return nil, "", fmt.Errorf("work type %q is ambiguous; candidates: %s", q, joinNames(sub))
	}
	return nil, "", fmt.Errorf("no work type matches %q; known types: %s", q, strings.Join(names, ", "))
}

func joinNames(refs []namedRef) string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

// ---- the tool table ----

func toolTable() []Tool {
	return []Tool{
		{
			Name: "day_summary",
			Description: "The day's coded time: worked, billable, non-billable, remaining toward the " +
				"seasonal daily standard, overtime, and every entry. Defaults to today.",
			InputSchema: obj(map[string]any{
				"date": str("Day to report, YYYY-MM-DD. Omit for today."),
			}),
			handler: daySummary,
		},
		{
			Name: "attention",
			Description: "Open to-dos that need attention, sorted by severity: overdue, due today, " +
				"due soon, high priority, and stale items sitting on the list too long.",
			InputSchema: obj(map[string]any{}),
			handler: func(c *APIClient, _ map[string]any) (string, error) {
				var out any
				if err := c.get("/api/v1/attention", &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name: "search_work",
			Description: "Full-text search across every past time entry and library template — " +
				"'what did I do for that reorg' answered from the record.",
			InputSchema: obj(map[string]any{
				"query": str("Search terms, e.g. 'rollover instruction letter'."),
			}, "query"),
			handler: func(c *APIClient, args map[string]any) (string, error) {
				q := argStr(args, "query")
				if q == "" {
					return "", fmt.Errorf("query is required")
				}
				var out any
				if err := c.get("/api/v1/search?q="+url.QueryEscape(q), &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name: "get_template",
			Description: "Fetch one library template in full: title, body text, tags, and the names " +
				"of any attached files (Word letters, PDFs). Use search_work to find its id.",
			InputSchema: obj(map[string]any{
				"id": num("Template id from search_work results."),
			}, "id"),
			handler: func(c *APIClient, args map[string]any) (string, error) {
				id, ok := argNum(args, "id")
				if !ok {
					return "", fmt.Errorf("id is required")
				}
				var out any
				if err := c.get(fmt.Sprintf("/api/v1/templates/%d", int64(id)), &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name: "log_time",
			Description: "Record a completed block of time. Client and work type are matched by name " +
				"or client number against the live catalogs (never created). Without a date the " +
				"entry ends now; with a date it starts at 09:00 local.",
			InputSchema: obj(map[string]any{
				"hours":       num("Duration in hours, e.g. 1.5."),
				"description": str("What was done."),
				"client":      str("Client name or number (optional)."),
				"work_type":   str("Work type name, e.g. 'T2 corporate' (optional)."),
				"date":        str("YYYY-MM-DD (optional; omit for 'just finished')."),
				"billable":    boolp("Override the work type's billable default (optional)."),
			}, "hours"),
			handler: logTime,
		},
		{
			Name: "start_timer",
			Description: "Start the live timer (fails politely if one is already running). " +
				"Client and work type resolve by name.",
			InputSchema: obj(map[string]any{
				"description": str("What you're starting."),
				"client":      str("Client name or number (optional)."),
				"work_type":   str("Work type name (optional)."),
			}),
			handler: startTimer,
		},
		{
			Name:        "stop_timer",
			Description: "Stop the running timer and log the entry.",
			InputSchema: obj(map[string]any{}),
			handler: func(c *APIClient, _ map[string]any) (string, error) {
				var out any
				if err := c.post("/api/v1/timer/stop", nil, &out); err != nil {
					return "", err
				}
				return "Timer stopped.\n" + pretty(out), nil
			},
		},
		{
			Name: "add_todo",
			Description: "Add a task to the to-do list, optionally tied to a client, with a due date " +
				"and a high-priority flag.",
			InputSchema: obj(map[string]any{
				"title":    str("The task."),
				"client":   str("Client name or number (optional)."),
				"due_date": str("YYYY-MM-DD (optional)."),
				"priority": boolp("Mark high priority (optional)."),
			}, "title"),
			handler: addTodo,
		},
		{
			Name:        "list_todos",
			Description: "The to-do list, open items first. Set include_done to also see completed tasks.",
			InputSchema: obj(map[string]any{
				"include_done": boolp("Include completed items (default false)."),
			}),
			handler: func(c *APIClient, args map[string]any) (string, error) {
				path := "/api/v1/todos"
				if done, _ := argBool(args, "include_done"); done {
					path += "?include_done=1"
				}
				var out any
				if err := c.get(path, &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name: "summary_report",
			Description: "Totals for a date range grouped by client, work_type, or day — worked, " +
				"billable, and billed (rounded to the firm's increment) minutes per bucket.",
			InputSchema: obj(map[string]any{
				"from":     str("Start date, YYYY-MM-DD."),
				"to":       str("End date, YYYY-MM-DD (inclusive)."),
				"group_by": str("client | work_type | day (default client)."),
			}, "from", "to"),
			handler: func(c *APIClient, args map[string]any) (string, error) {
				q := url.Values{}
				q.Set("from", argStr(args, "from"))
				q.Set("to", argStr(args, "to"))
				if g := argStr(args, "group_by"); g != "" {
					q.Set("group_by", g)
				}
				var out any
				if err := c.get("/api/v1/reports/summary?"+q.Encode(), &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name:        "list_clients",
			Description: "All active clients with their firm client numbers.",
			InputSchema: obj(map[string]any{}),
			handler: func(c *APIClient, _ map[string]any) (string, error) {
				var out any
				if err := c.get("/api/v1/clients", &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
		{
			Name:        "list_work_types",
			Description: "The work-type (billing-type) catalog with billable defaults.",
			InputSchema: obj(map[string]any{}),
			handler: func(c *APIClient, _ map[string]any) (string, error) {
				var out any
				if err := c.get("/api/v1/work-types", &out); err != nil {
					return "", err
				}
				return pretty(out), nil
			},
		},
	}
}

// ---- multi-step handlers ----

func daySummary(c *APIClient, args map[string]any) (string, error) {
	path := "/api/v1/reports/day"
	if d := argStr(args, "date"); d != "" {
		path += "?date=" + url.QueryEscape(d)
	}
	var out any
	if err := c.get(path, &out); err != nil {
		return "", err
	}
	return pretty(out), nil
}

func logTime(c *APIClient, args map[string]any) (string, error) {
	hours, ok := argNum(args, "hours")
	if !ok || hours <= 0 {
		return "", fmt.Errorf("hours must be a positive number")
	}
	minutes := int64(math.Round(hours * 60))

	clientID, clientName, err := resolveClient(c, argStr(args, "client"))
	if err != nil {
		return "", err
	}
	workTypeID, workTypeName, err := resolveWorkType(c, argStr(args, "work_type"))
	if err != nil {
		return "", err
	}

	var startedAt int64
	if d := argStr(args, "date"); d != "" {
		day, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err != nil {
			return "", fmt.Errorf("invalid date %q (want YYYY-MM-DD)", d)
		}
		startedAt = day.Add(9 * time.Hour).Unix()
	} else {
		startedAt = time.Now().Unix() - minutes*60 // ends now
	}

	body := map[string]any{
		"client_id":    clientID,
		"work_type_id": workTypeID,
		"description":  argStr(args, "description"),
		"started_at":   startedAt,
		"duration_min": minutes,
	}
	if b, ok := argBool(args, "billable"); ok {
		body["billable"] = b
	}
	var out any
	if err := c.post("/api/v1/time-entries", body, &out); err != nil {
		return "", err
	}
	label := strings.TrimSpace(clientName + " / " + workTypeName)
	return fmt.Sprintf("Logged %.1fh (%s).\n%s", hours, strings.Trim(label, "/ "), pretty(out)), nil
}

func startTimer(c *APIClient, args map[string]any) (string, error) {
	clientID, _, err := resolveClient(c, argStr(args, "client"))
	if err != nil {
		return "", err
	}
	workTypeID, _, err := resolveWorkType(c, argStr(args, "work_type"))
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"client_id":    clientID,
		"work_type_id": workTypeID,
		"description":  argStr(args, "description"),
	}
	var out any
	if err := c.post("/api/v1/timer/start", body, &out); err != nil {
		return "", err
	}
	return "Timer started.\n" + pretty(out), nil
}

func addTodo(c *APIClient, args map[string]any) (string, error) {
	title := argStr(args, "title")
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	clientID, _, err := resolveClient(c, argStr(args, "client"))
	if err != nil {
		return "", err
	}
	body := map[string]any{"title": title, "client_id": clientID}
	if d := argStr(args, "due_date"); d != "" {
		body["due_date"] = d
	}
	if hi, ok := argBool(args, "priority"); ok && hi {
		body["priority"] = 1
	}
	var out any
	if err := c.post("/api/v1/todos", body, &out); err != nil {
		return "", err
	}
	return "Added to the list.\n" + pretty(out), nil
}
