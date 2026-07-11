# Chronos UI redesign — one-page workspace

A frontend-only redesign brief. Two goals, in order:

1. **One page instead of tabs.** The sidebar's five views (Today / To-do /
   Clients / Library / Reports) collapse into a single scrolling page. The
   working day is the centerpiece; reference material lives below it; detail
   editing happens in a slide-over panel instead of split columns.
2. **Feel designed, not templated.** The current UI is a competent
   "modern-SaaS admin" look — white cards, soft shadows, rounded corners —
   which reads as generic. Replace it with a sharper identity: a
   **ledger-modern** aesthetic (this is a tool for a CPA) — numeric-heavy,
   hairline rules, one gold accent, generous whitespace, oversized tabular
   numerals.

Everything here is in `internal/webui/assets/` (`index.html`, `style.css`,
`app.js`). The Go side needs **zero changes**: `//go:embed all:assets` picks up
any new files automatically, and the API is untouched.

## Hard constraints (do not break these)

- **No build step.** Vanilla ES modules only. No npm, no framework, no
  bundler. New JS files are fine (native `import`), served from `assets/`.
- **API is the contract.** Frontend-only change; keep every `/api/v1` call
  shape exactly as it is today.
- **Both themes stay.** Light and dark via the existing CSS custom-property
  tokens, OS-following by default, `[data-theme]` override, and the
  before-first-paint flash guard in `index.html` must survive.
- **Security discipline stays.** All user data through `esc()` before
  `innerHTML` (see the `safeSnippet` pattern in `app.js` for the one
  server-marked exception).
- **Accessibility stays or improves.** `prefers-reduced-motion` kill switch,
  focus-visible rings, aria-labels on icon buttons, keyboard-reachable
  everything. New: the slide-over panel and command palette must trap focus
  and close on Escape.
- **Responsive to phone width.** The one-page layout must degrade to a single
  column; the top bar must not overflow.
- **System fonts only** (self-contained binary). Sharpen usage rather than
  adding webfonts: bigger sizes, `font-variant-numeric: tabular-nums`
  everywhere numbers appear, tighter letter-spacing on display sizes.

## Information architecture

### Top bar (replaces the sidebar entirely)

Sticky, slim (~56px), full width:

- **Brand** (mark + "Chronos") on the left.
- **Section nav**: `Today · Clients · Library · Reports` — anchor links that
  smooth-scroll, with a scroll-spy highlight (IntersectionObserver on the
  sections; the existing observer pattern in the codebase is a good template).
  To-do is *not* a nav item — it lives inside the Today workspace.
- **Live timer chip** (the current `#side-timer`): visible whenever a timer
  runs, click scrolls to the timer hero. Keep the pulse dot and mono clock.
- **Right cluster**: search button (opens command palette, shows `⌘K` hint),
  theme toggle, settings.

### Section 1 — `#today` (the workspace)

The hero of the page. Desktop is a two-column grid; mobile stacks.

```
┌──────────────────────────────────────────────────────────┐
│  Today ‹ › [date]                     Worked 5.2h ▂▂▂▂▂  │  ← day header + strip
│                                       Billable 4.6h …    │
├──────────────────────────────────────────────────────────┤
│  ●  2:41:07   Acme Corp · T2 prep            [ Stop ]    │  ← timer hero
├──────────────────────────────────────────────────────────┤
│  09:00 ─▇▇▇▇──▇▇──────▇▇▇▇▇▇──── now                     │  ← day timeline
├───────────────────────────────┬──────────────────────────┤
│  ENTRIES (main column)        │  TO-DO (rail)            │
│  9:00–9:45  Acme · CRA corr.  │  ⚠ Needs attention (3)   │
│  9:45–11:10 Jones · T1        │  ○ File T2 for Acme  ▶   │
│  + Add an entry               │  ○ Call CRA re: Jones ▶  │
│                               │  + quick-add             │
└───────────────────────────────┴──────────────────────────┘
```

- **Day header**: `Today` / `Yesterday` / date label with the existing ‹ ›
  date navigation, compact and left-aligned. The **day strip** moves up beside
  it: instead of five bordered stat boxes, render a single row of figures —
  `Worked 5.2h · Billable 4.6h · Non-billable 0.6h · 2.3h to target` — with a
  slim gold progress bar under Worked. **Overtime appears only when > 0**
  (styled `--warn`); don't reserve a box for a usually-zero number.
- **Timer hero**: one card, but the *only* elevated card on the page. When
  running: very large mono clock (`clamp(2.6rem, 6vw, 4.2rem)`, tabular nums,
  gold), client + work-type + description beside it, Stop button. When idle:
  the start form on one line — client select, work-type select, description
  input, gold Start button. This is the identity moment of the app; spend the
  polish here (e.g. a soft gold wash gradient while running, as now).
- **Day timeline** (new, signature piece): a horizontal band (~28px) under
  the hero mapping the day's entries as gold blocks positioned by
  `started_at`/`ended_at` percentages along an axis from the day's first entry
  (or 8:00, whichever is earlier) to `max(now, last end)`. Running entry
  renders as a pulsing block growing toward "now". Pure CSS + JS percent
  widths; `title` tooltips with client + range; hide under ~520px. Data is
  already in `state.day.entries` — no new API call.
- **Entries list**: drop the card-per-row borders. Render as a ruled ledger:
  hairline dividers between rows, time range in a fixed mono column,
  client bold + description muted, work-type as a small tag, duration
  right-aligned mono. Row actions (save-as-template, delete) appear on
  hover/focus-within as now. "Add an entry" keeps the `<details>` disclosure
  but restyled as a ghost row at the bottom of the ledger.
- **To-do rail** (right column, ~320px): merges the current attention panel
  and to-do view. Order: attention items first (with their reason badges),
  then open todos, then a collapsed "Completed (n)". Quick-add input at the
  top (title + client select + due date behind a disclosure). Keep the ▶
  start-timer-from-task button — it should scroll to the timer hero after
  starting. On mobile the rail stacks below the entries.

### Section 2 — `#clients`

Full-width section. The current split-column layout goes away:

- One compact table/list of clients (name, code, and — nice to have — total
  tracked hours if cheap to fetch lazily per open, not upfront).
- The add form + Import CSV stay above the list, one line.
- **Clicking a client opens the slide-over panel** (see below) with the
  detail view: total tracked, recent entries ledger (same row style as Today).

### Section 3 — `#library`

- Search bar + "New template" button, then results (grouped Templates / Past
  work exactly as the current logic does).
- **Clicking a template / New template opens the slide-over panel** with the
  editor form + attachments block (all existing behavior: save, delete,
  attach, download).

### Section 4 — `#reports`

- Range + group-by form and the results table, essentially as today but
  restyled: the table loses its box shadow, keeps the double-rule total row
  (the accountant's total rule — keep that detail, it's good).
- Consider defaulting to running the current-month report on first scroll
  into view (IntersectionObserver, run once) so the section is never empty.

### Slide-over panel (new shared component)

One reusable right-hand panel (`<aside id="panel">`, ~440px, full-height,
translate-in over a scrim) that hosts: client detail, template editor, and
settings (retire the `<dialog>` or keep dialog only for settings — implementer's
choice, but client detail and template editor must move to the panel).
Requirements: focus trap, Escape closes, scrim click closes, body scroll
locked while open, reduced-motion = no slide animation.

### Command palette (new, `⌘K` / `Ctrl+K`)

A small `<dialog>` with a single input + result list. Sources, all local or
one fetch:

- Actions: "Start timer", "Stop timer", "Add to-do", "Add entry", "Go to
  Clients/Library/Reports", "Settings", "Export CSV".
- Clients by name/code (from `state.clients`) → opens client panel.
- Library search (debounced `/api/v1/search`) → opens template panel or shows
  the entry hit.

Arrow keys + Enter to select, Escape closes. This is the second signature
piece; keep it under ~150 lines.

## Visual system

Keep the existing token names (`--bg`, `--surface`, `--ink*`, `--gold*`,
`--line*`, semantic colors) and both palettes — the CloudOlympus gold/night
identity is right. **"Ledger" here means structure — rules, columns, tabular
numerals — never styling: no warm browns, no parchment/vintage/sepia
anything, no serif body text.** The owner explicitly rejected that direction;
the feel should stay modern (think QuickBooks-clean, sharpened). Change how
the existing tokens are used:

- **Fewer boxes.** Only the timer hero (and open panel/palette) get
  `--surface` + shadow. Everything else sits directly on `--bg` with hairline
  `--line` rules. This alone kills most of the "template" feel.
- **Numbers are the decoration.** Every numeric value mono + tabular;
  stat figures large; durations right-aligned. Section titles stay the small
  uppercase letterspaced style with the rule-line after — it works.
- **Gold is the only accent.** Semantic green/red/orange only for status
  (overdue, running pulse, stop button). No blue anywhere.
- **Motion**: 150–250ms ease-out; the existing staggered `rise` on first
  paint is good — keep it per-section on scroll-into-view rather than
  per-view-switch. Everything behind the reduced-motion guard.
- **Empty states** get one short human line, no icons or illustrations.

## Code organization

`app.js` (948 lines) should split into native ES modules — no build step
needed, `<script type="module">` already in place:

```
assets/
  app.js            boot + wiring only
  lib/api.js        fetch helpers (GET/POST/PUT/DEL), esc, toast, fmt helpers
  lib/panel.js      slide-over component
  lib/palette.js    command palette
  sections/today.js timer, day strip, timeline, entries, day nav
  sections/todos.js rail: attention + todos
  sections/clients.js
  sections/library.js
  sections/reports.js
  sections/settings.js
```

Keep the module boundaries aligned with the current function groups (they're
already well separated). Keep the `state` object as a shared module export.
`//go:embed all:assets` embeds new files automatically.

Since all sections are now mounted at once: load Today + refs at boot (as
now), lazy-init Clients/Library/Reports on first scroll-into-view or first
palette use, keep the existing 30s Today refresh + 1s timer tick.

## Phases (each is a reviewable commit that builds and works)

1. **Layout swap.** New `index.html` skeleton: top bar, four sections,
   scroll-spy nav, slide-over panel shell. Move todos into the rail, client
   detail + template editor into the panel. All existing behavior preserved;
   old CSS adjusted minimally to keep it usable. `app.js` split into modules.
2. **Visual system.** Restyle everything to the ledger-modern direction:
   de-box the lists, restyle day strip, tables, forms, buttons. Both themes,
   both breakpoints.
3. **Signature pieces.** Timer hero treatment, day timeline band, command
   palette, keyboard shortcuts (`⌘K`; optional: `t` start/stop when no input
   focused).
4. **Polish pass.** Empty states, focus order, scroll-margin on anchored
   sections (account for sticky bar), mobile audit, reduced-motion audit.

## Acceptance checklist

- `go build ./... && go vet ./...` pass; `go run ./cmd/chronos --db /tmp/c.db`
  serves the new UI with no console errors.
- Every current capability still works: start/stop timer, manual entry, day
  navigation to arbitrary dates, todo add/done/delete/start-timer, attention
  panel, client add/import/detail, library search/template CRUD/attachments,
  reports run/export CSV, settings + work-type manager + CSV import, theme
  cycling with no flash, toasts.
- One page: no `hidden` view sections; nav highlights follow scroll; timer
  chip visible in the bar wherever you are on the page.
- Light + dark verified; 375px-wide phone verified; keyboard-only walk of
  timer → todo → panel → palette verified; reduced-motion shows no animation.
- No changes outside `internal/webui/assets/`.

### Post-redesign note: asset caching (2026-07-09)

Embedded files have zero modtime, so plain `http.FileServerFS` sends no cache
validators and a browser can heuristically cache some ES modules but not
others — after a deploy a page may run a stale mix of old and new modules
(this happened during development). `webui.Handler()` now sets
`Cache-Control: no-cache` plus a build-wide `ETag` (hash of the embedded
tree) on every asset response, so browsers revalidate each load, unchanged
builds answer 304, and any rebuild invalidates all modules atomically.
Verify: `curl -sI http://localhost:8787/app.js` shows both headers.
