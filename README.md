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

- **Today** — start/stop a timer against a client and work type, or add a past
  entry. The header tracks Worked · Billable · **Non-billable** · **Remaining**
  (of the seasonal target) · **Overtime**, live. Arrow through days or jump to
  any date — yesterday or a random Wednesday two years ago — and the day bar
  recomputes with that day's seasonal standard.
- **Needs attention** — a panel on Today that surfaces what matters from your
  list: overdue items, due today/soon, high priority, and anything sitting
  untouched for 14+ days. Served by `GET /api/v1/attention`, so agents can ask
  the same question.
- **To-do** — a task list optionally tied to a client, with due dates and a
  high-priority flag. One click (▶) starts a timer seeded from the task; rows
  show how long they've been on your list.
- **Clients** — add clients (name + your firm's client number) and browse
  everything you've logged for each. Bulk-load from a CSV export of your firm
  system (`name` + `code`/`number` columns); dropdowns show the number
  everywhere.
- **Library** — reusable templates (letter skeletons, reorg step lists) plus
  full-text search over *everything you've ever coded*. Save any past entry as
  a template with one click, then polish it. Attach the real deliverables —
  Word letters, PDFs, worksheets — to any template; files live on disk next to
  the database in `chronos-files/`, so backup means copying one folder.
- **Reports** — total any date range grouped by client, work type, or day, and
  export it as CSV. The same CSV shape imports back in (only `date` and
  `minutes` columns are required).

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

Tools: `day_summary`, `attention`, `search_work`, `get_template`, `log_time`,
`start_timer`, `stop_timer`, `add_todo`, `list_todos`, `summary_report`,
`list_clients`, `list_work_types`. Clients and work types are matched by name
or client number against your live catalogs — never invented; a miss returns
the list of real options so the agent can correct itself.

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
