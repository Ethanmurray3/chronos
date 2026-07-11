# Chronos

A single-binary time & client-work tracker for tax and accounting
practitioners. Track your day by client and work type, watch your billable
hours against a daily target (with remaining and overtime), keep a searchable
record of what you did for whom, and export/import CSV so it can replace a
timesheet spreadsheet.

Everything ships as **one Go binary** with an **embedded web UI** and a local
**SQLite** file — no runtime, no database server, no build step. Copy the
binary to a machine, run it, open the browser.

## Quick start

```sh
go build -o chronos ./cmd/chronos
./chronos --db ./chronos.db        # serves http://localhost:8787
```

Flags / environment:

| Flag     | Env            | Default        | Meaning                        |
|----------|----------------|----------------|--------------------------------|
| `--addr` | `CHRONOS_ADDR` | `:8787`        | Listen address (`PORT` honored)|
| `--db`   | `CHRONOS_DB`   | `./chronos.db` | Path to the SQLite file        |

On first run Chronos seeds one user, a starter tax work-type catalog (CRA
correspondence, T1, T2, reorganizations, form filing, bookkeeping, General, …),
and sensible defaults (6-minute billing increment and a **seasonal daily
standard**: 8 h/day in busy season Jan–Apr, 7.5 h/day off season May–Dec).
Adjust these under the ⚙ settings dialog.

The **daily standard** is the number of hours you need to code that day, and it
doubles as the overtime line: **Remaining** counts your total coded time down
toward it, and anything past it is **Overtime**.

Nearly everything is configurable per user: seasonal targets, billing
increment, timezone, the "due soon" window, and the stale-todo threshold live
in settings; work types (billing types) are fully editable in the ⚙ dialog and
importable from CSV.

## Using it

The app is **two pages** — Today (the daily workspace) and Templates — with
Clients, Reports, and Settings as dialogs off the top bar.

- **Today** — start/stop a timer against a client and work type, or add a past
  entry. Pick an open **task** in either form and it fills in the client and
  description. Manual entries take real **start/end clock times** (3:00 →
  4:30 becomes 1.5h at 3:00) or a bare duration. The header tracks Worked ·
  Billable · **Non-billable** · **Remaining** (of the seasonal target) ·
  **Overtime** live, and the **Coded today** ledger sits front and centre with
  the day's total. Arrow through days or jump to any date and the day bar
  recomputes with that day's seasonal standard.
- **Needs attention** — a panel on Today that surfaces what matters from your
  list: overdue items, due today/soon, high priority, and anything sitting
  untouched for 14+ days. Served by `GET /api/v1/attention`, so agents can ask
  the same question.
- **To-do** — a task list optionally tied to a client, with due dates and a
  high-priority flag. Tasks feed the time-coding forms; rows show how long
  they've been on your list.
- **Templates** — the standalone deliverable library, decoupled from time
  coding. When you finish a letter or worksheet worth reusing, save it here:
  attach the real file (Word, PDF), write notes about what it contains and
  when to reach for it, file it under a category ("Reorg letters", "CRA
  responses"), and optionally label which client it was originally for.
  Grouped by category with its own full-text search; files live on disk next
  to the database in `chronos-files/`, so backup means copying one folder.
- **Clients** (dialog) — add clients (name + your firm's client number) and
  browse everything you've logged for each. Bulk-load from a CSV export of
  your firm system (`name` + `code`/`number` columns); dropdowns show the
  number everywhere.
- **Reports** (overlay) — a mini dashboard for any range: quick chips (this
  month, last month, this year, last year) or custom dates, summary tiles for
  worked / billable / non-billable / **overtime** / **vacation**, and a
  breakdown grouped by client, work type, or day with client/work-type
  filters. **Export to Excel** produces a real .xlsx (Summary + Breakdown
  sheets). Overtime uses the weekend rule — Mon–Fri it's time beyond the
  seasonal daily standard, Sat/Sun every coded minute counts — and vacation
  (time on the seeded non-billable **Vacation** work type) is totalled
  separately, never as overtime. CSV export/import of raw entries lives on in
  the API (`/api/v1/export.csv`, `/api/v1/import`).

## Agent access (MCP)

`chronos-mcp` is a **separate, optional binary** — an [MCP](https://modelcontextprotocol.io)
server that lets an AI agent work your Chronos data. One person on a team can
use it; the Chronos server needs no changes and nobody else ever sees it.

```sh
go build -o chronos-mcp ./cmd/chronos-mcp
claude mcp add chronos -- /path/to/chronos-mcp --url http://localhost:8787
```

Configuration: `--url` flag or `CHRONOS_URL` env; if `CHRONOS_TOKEN` is set it
is sent as a Bearer token (ready for when the API grows authentication).

Tools: `day_summary`, `attention`, `search_work`, `search_templates`,
`get_template`, `log_time`, `start_timer`, `stop_timer`, `add_todo`,
`list_todos`, `summary_report` (filterable by client/work type),
`period_report` (overtime + vacation for a range), `list_clients`,
`list_work_types`. Clients and work types are matched by name or client
number against your live catalogs — never invented; a miss returns the list
of real options so the agent can correct itself.

## Design notes

- **Single-user today, multi-tenant-ready.** Every row carries an `owner_id`
  and the API scopes every call to one. Adding real accounts later is a login
  flow plus resolving a real owner id — no schema migration.
- **A running timer is just an entry with no end time.** At most one runs at
  once, enforced by a unique partial index (race-free).
- **The API is the contract.** The web UI is one client of `/api/v1`; a future
  MCP server will be another, letting an assistant log time and search past
  work directly.

## Layout

```
cmd/chronos      entry point (flags, wiring, graceful shutdown)
internal/store   SQLite layer — one file per concern; math is unit-tested
internal/api     /api/v1 REST handlers + CSV import/export
internal/webui   embedded HTML/CSS/JS frontend
internal/config  flag/env resolution
```

## Development

```sh
go vet ./...
go test ./...     # store math: rounding, overtime, timer invariant, tz days
go run ./cmd/chronos --db /tmp/chronos.db
```

## Status

M1 covers the daily-use core: clients, work-type catalog, timer + manual
entries, the live day report, range reports, and CSV import/export. M2 adds the
to-do list (with start-timer-from-task) and seasonal daily targets with a
non-billable breakdown. Planned next: a searchable template/work library, an MCP
server, real multi-user auth, and self-hosting behind Authentik on CloudOlympus.
