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

## Using it

- **Today** — start/stop a timer against a client and work type, or add a past
  entry. The header tracks Worked · Billable · **Non-billable** · **Remaining**
  (of the seasonal target) · **Overtime**, live.
- **To-do** — a task list optionally tied to a client, with due dates and a
  high-priority flag. One click (▶) starts a timer seeded from the task.
- **Clients** — add clients and browse everything you've logged for each.
- **Reports** — total any date range grouped by client, work type, or day, and
  export it as CSV. The same CSV shape imports back in (only `date` and
  `minutes` columns are required).

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
