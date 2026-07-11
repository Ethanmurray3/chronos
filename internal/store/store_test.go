package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRoundUpTo(t *testing.T) {
	cases := []struct{ min, step, want int64 }{
		{0, 6, 0},
		{1, 6, 6},
		{6, 6, 6},
		{7, 6, 12},
		{100, 6, 102},
		{100, 0, 100}, // step 0 is a no-op
		{-5, 6, -5},   // negative is a no-op
	}
	for _, c := range cases {
		if got := roundUpTo(c.min, c.step); got != c.want {
			t.Errorf("roundUpTo(%d,%d) = %d, want %d", c.min, c.step, got, c.want)
		}
	}
}

func TestRoundSecondsToMin(t *testing.T) {
	cases := []struct{ sec, want int64 }{
		{0, 0}, {29, 0}, {30, 1}, {89, 1}, {90, 2}, {-100, 0},
	}
	for _, c := range cases {
		if got := roundSecondsToMin(c.sec); got != c.want {
			t.Errorf("roundSecondsToMin(%d) = %d, want %d", c.sec, got, c.want)
		}
	}
}

func TestOneRunningTimer(t *testing.T) {
	s := openTest(t)
	if _, err := s.StartTimer(SeedOwner, nil, nil, nil, "first", nil); err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if _, err := s.StartTimer(SeedOwner, nil, nil, nil, "second", nil); err != ErrTimerRunning {
		t.Fatalf("second StartTimer = %v, want ErrTimerRunning", err)
	}

	cur, err := s.CurrentTimer(SeedOwner)
	if err != nil {
		t.Fatalf("CurrentTimer: %v", err)
	}
	if !cur.Running {
		t.Errorf("current timer not marked running")
	}

	stopped, err := s.StopTimer(SeedOwner)
	if err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if stopped.Running || stopped.DurationMin == nil {
		t.Errorf("stopped entry still running or has no duration: %+v", stopped)
	}
	if _, err := s.StopTimer(SeedOwner); err != ErrNotFound {
		t.Errorf("StopTimer with none running = %v, want ErrNotFound", err)
	}
	// After stopping, a fresh timer must be allowed.
	if _, err := s.StartTimer(SeedOwner, nil, nil, nil, "third", nil); err != nil {
		t.Errorf("StartTimer after stop: %v", err)
	}
}

func TestSwitchTimer(t *testing.T) {
	s := openTest(t)
	first, err := s.StartTimer(SeedOwner, nil, nil, nil, "first", nil)
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}

	second, err := s.SwitchTimer(SeedOwner, nil, nil, nil, "from todo", nil)
	if err != nil {
		t.Fatalf("SwitchTimer: %v", err)
	}
	if second.Description != "from todo" || !second.Running {
		t.Fatalf("replacement timer = %+v", second)
	}

	stopped, err := s.Entry(SeedOwner, first.ID)
	if err != nil {
		t.Fatalf("old timer: %v", err)
	}
	if stopped.Running || stopped.EndedAt == nil || stopped.DurationMin == nil {
		t.Errorf("old timer was not logged: %+v", stopped)
	}

	current, err := s.CurrentTimer(SeedOwner)
	if err != nil {
		t.Fatalf("CurrentTimer: %v", err)
	}
	if current.ID != second.ID {
		t.Errorf("current timer ID = %d, want %d", current.ID, second.ID)
	}
}

func TestDayReportMath(t *testing.T) {
	s := openTest(t)
	// 100 min billable, 50 min non-billable, 7 min billable (rounds to 12),
	// all on a fixed off-season day so the run date can't affect the result.
	day := timeUnix(t, "2026-07-07", 9)
	mustEntryAt(t, s, day, 100, boolp(true))
	mustEntryAt(t, s, day, 50, boolp(false))
	mustEntryAt(t, s, day, 7, boolp(true))

	rep, err := s.DayReport(SeedOwner, "2026-07-07") // July → off-season, 450
	if err != nil {
		t.Fatalf("DayReport: %v", err)
	}
	assertEq(t, "worked", rep.WorkedMin, 157)
	assertEq(t, "billable", rep.BillableMin, 107)
	assertEq(t, "non-billable", rep.NonBillableMin, 50)
	assertEq(t, "billed", rep.BilledMin, 114)       // 102 + 12
	assertEq(t, "target", rep.TargetMin, 450)       // off-season
	assertEq(t, "remaining", rep.RemainingMin, 293) // 450 - 157 (total worked)
	assertEq(t, "overtime", rep.OvertimeMin, 0)     // 157 < 450
}

func TestSeasonalTargetAndOvertime(t *testing.T) {
	s := openTest(t)
	// One 500-minute entry, reported on both a busy-season and off-season day.
	// (started_at date drives which season applies.)
	busyStart := timeUnix(t, "2026-02-10", 9)
	offStart := timeUnix(t, "2026-07-10", 9)
	mustEntryAt(t, s, busyStart, 500, boolp(true))
	mustEntryAt(t, s, offStart, 500, boolp(true))

	busy, err := s.DayReport(SeedOwner, "2026-02-10")
	if err != nil {
		t.Fatalf("DayReport busy: %v", err)
	}
	assertEq(t, "busy target", busy.TargetMin, 480)    // Feb → busy season
	assertEq(t, "busy overtime", busy.OvertimeMin, 20) // 500 - 480

	off, err := s.DayReport(SeedOwner, "2026-07-10")
	if err != nil {
		t.Fatalf("DayReport off: %v", err)
	}
	assertEq(t, "off target", off.TargetMin, 450)    // July → off season
	assertEq(t, "off overtime", off.OvertimeMin, 50) // 500 - 450
	assertEq(t, "off remaining", off.RemainingMin, 0)
}

func TestDayBoundaryTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	s := openTest(t)
	if err := s.SaveSettings(SeedOwner, Settings{
		BusySeasonTargetMin: 480, OffSeasonTargetMin: 450, RoundingMin: 6, Timezone: "America/Toronto",
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	// 23:30 local on Jan 15 belongs to Jan 15, not Jan 16.
	start := time.Date(2026, 1, 15, 23, 30, 0, 0, loc).Unix()
	if _, err := s.CreateEntry(SeedOwner, EntryInput{
		Description: "late night", StartedAt: start, DurationMin: 20, Billable: boolp(true),
	}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	on, err := s.DayReport(SeedOwner, "2026-01-15")
	if err != nil {
		t.Fatalf("DayReport 15th: %v", err)
	}
	assertEq(t, "worked on 15th", on.WorkedMin, 20)

	off, err := s.DayReport(SeedOwner, "2026-01-16")
	if err != nil {
		t.Fatalf("DayReport 16th: %v", err)
	}
	assertEq(t, "worked on 16th", off.WorkedMin, 0)
}

func TestClientScopingAndArchive(t *testing.T) {
	s := openTest(t)
	c, err := s.CreateClient(SeedOwner, "Acme Corp", "ACME", "notes")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := s.UpdateClient(SeedOwner, c.ID, "Acme Corp", "ACME", "", "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	active, _ := s.Clients(SeedOwner, true)
	if len(active) != 0 {
		t.Errorf("archived client still listed as active: %d", len(active))
	}
	all, _ := s.Clients(SeedOwner, false)
	if len(all) != 1 || all[0].ArchivedAt == nil {
		t.Errorf("archived client missing or has no archived_at: %+v", all)
	}
}

func TestSearchAndTemplates(t *testing.T) {
	s := openTest(t)
	c, err := s.CreateClient(SeedOwner, "Acme Holdings", "1234567", "")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	e, err := s.CreateEntry(SeedOwner, EntryInput{
		ClientID:    &c.ID,
		Description: "s.85 rollover instruction letter to counsel",
		DurationMin: 60,
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	// Entry text is searchable, with prefix matching on the last term —
	// and templates never leak into the past-work search.
	hits, err := s.SearchEntries(SeedOwner, "rollover instruc", 20)
	if err != nil {
		t.Fatalf("SearchEntries: %v", err)
	}
	if len(hits) != 1 || hits[0].Kind != "entry" || hits[0].ID != e.ID {
		t.Fatalf("entry hits = %+v, want the one entry", hits)
	}

	// Templates are standalone: category, client label, notes.
	tpl, err := s.CreateTemplate(SeedOwner, TemplateInput{
		ClientID: &c.ID,
		Title:    "Reorg instruction letter — holdco freeze",
		Notes:    "s.85 rollover steps, PUC grind, price adjustment clause",
		Category: "Reorg letters",
		Tags:     "rollover, s85",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if tpl.ClientName != "Acme Holdings" || tpl.Category != "Reorg letters" {
		t.Errorf("template missing client/category: %+v", tpl)
	}

	// Template search is its own index: notes match, entries don't show up.
	thits, err := s.SearchTemplates(SeedOwner, "rollover", 20)
	if err != nil {
		t.Fatalf("SearchTemplates: %v", err)
	}
	if len(thits) != 1 || thits[0].Kind != "template" || thits[0].ID != tpl.ID {
		t.Fatalf("template hits = %+v, want the one template", thits)
	}
	ehits, _ := s.SearchEntries(SeedOwner, "rollover", 20)
	if len(ehits) != 1 || ehits[0].Kind != "entry" {
		t.Fatalf("entry search polluted: %+v", ehits)
	}

	// Editing the template keeps the FTS index in sync via triggers.
	if _, err := s.UpdateTemplate(SeedOwner, tpl.ID, TemplateInput{
		Title: "Reorg letter skeleton", Notes: "butterfly reorganization steps", Category: "Reorg letters",
	}); err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	thits, _ = s.SearchTemplates(SeedOwner, "butterfly", 20)
	if len(thits) != 1 {
		t.Errorf("updated notes not searchable: %+v", thits)
	}
	thits, _ = s.SearchTemplates(SeedOwner, "price adjustment", 20)
	if len(thits) != 0 {
		t.Errorf("old template text still indexed: %+v", thits)
	}

	// The list is grouped for the Templates page: categories alphabetical,
	// uncategorized last.
	if _, err := s.CreateTemplate(SeedOwner, TemplateInput{Title: "loose note"}); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if _, err := s.CreateTemplate(SeedOwner, TemplateInput{Title: "CRA response shell", Category: "CRA responses"}); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	list, err := s.Templates(SeedOwner)
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	if len(list) != 3 || list[0].Category != "CRA responses" || list[1].Category != "Reorg letters" || list[2].Category != "" {
		t.Errorf("list order wrong: %+v", list)
	}

	// FTS syntax characters must not break either query.
	if _, err := s.SearchEntries(SeedOwner, `"AND (rollover OR`, 20); err != nil {
		t.Errorf("hostile entry query errored: %v", err)
	}
	if _, err := s.SearchTemplates(SeedOwner, `"AND (rollover OR`, 20); err != nil {
		t.Errorf("hostile template query errored: %v", err)
	}
}

// TestTemplateMigration opens a database shaped like the pre-decoupling
// schema (templates without client_id/category, tied to work types) and
// verifies migrate() adds the columns and turns work-type names into
// categories.
func TestTemplateMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := old.Exec(`
		CREATE TABLE templates (
		  id              INTEGER PRIMARY KEY AUTOINCREMENT,
		  owner_id        INTEGER NOT NULL,
		  work_type_id    INTEGER,
		  title           TEXT    NOT NULL,
		  body            TEXT    NOT NULL DEFAULT '',
		  tags            TEXT    NOT NULL DEFAULT '',
		  source_entry_id INTEGER,
		  created_at      INTEGER NOT NULL,
		  updated_at      INTEGER NOT NULL
		);
		CREATE TABLE work_types (
		  id INTEGER PRIMARY KEY AUTOINCREMENT, owner_id INTEGER NOT NULL,
		  name TEXT NOT NULL, category TEXT NOT NULL DEFAULT '',
		  billable_default INTEGER NOT NULL DEFAULT 1, sort INTEGER NOT NULL DEFAULT 0
		);
		CREATE VIRTUAL TABLE templates_fts USING fts5(
		  title, body, tags, content='templates', content_rowid='id');
		CREATE TRIGGER templates_fts_i AFTER INSERT ON templates BEGIN
		  INSERT INTO templates_fts(rowid, title, body, tags) VALUES (new.id, new.title, new.body, new.tags);
		END;
		INSERT INTO work_types (id, owner_id, name) VALUES (7, 1, 'Reorg step letter');
		INSERT INTO templates (owner_id, work_type_id, title, body, created_at, updated_at)
		  VALUES (1, 7, 'old template', 'the body', 100, 100);
	`); err != nil {
		t.Fatalf("build old schema: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open over old db: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	list, err := s.Templates(SeedOwner)
	if err != nil {
		t.Fatalf("Templates after migrate: %v", err)
	}
	if len(list) != 1 || list[0].Category != "Reorg step letter" || list[0].Notes != "the body" {
		t.Errorf("migrated template = %+v, want work-type name as category", list)
	}
	// Reopening must be a no-op (migration is idempotent).
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s2.Close()
}

// TestRangeReport exercises the period math: weekday OT is time beyond the
// daily standard, every weekend minute is OT, and vacation is totalled
// separately without ever counting as OT.
func TestRangeReport(t *testing.T) {
	s := openTest(t)
	if err := s.SaveSettings(SeedOwner, Settings{
		BusySeasonTargetMin: 480, OffSeasonTargetMin: 450, RoundingMin: 6,
		StaleDays: 14, DueSoonDays: 2, Timezone: "UTC",
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	wts, _ := s.WorkTypes(SeedOwner)
	var vacID int64
	for _, w := range wts {
		if w.Name == "Vacation" {
			vacID = w.ID
		}
	}
	if vacID == 0 {
		t.Fatal("seed catalog is missing the Vacation work type")
	}

	entry := func(day int, min int64, workType *int64) {
		t.Helper()
		started := time.Date(2025, 6, day, 9, 0, 0, 0, time.UTC).Unix()
		if _, err := s.CreateEntry(SeedOwner, EntryInput{
			Description: "x", StartedAt: started, DurationMin: min, WorkTypeID: workType,
		}); err != nil {
			t.Fatalf("CreateEntry day %d: %v", day, err)
		}
	}
	// June 2025 (off season, 450 min/day standard):
	entry(2, 480, nil)    // Mon Jun 2: 30 min over standard → 30 OT
	entry(3, 300, nil)    // Tue Jun 3: under standard → 0 OT
	entry(7, 120, nil)    // Sat Jun 7: weekend → all 120 OT
	entry(9, 450, &vacID) // Mon Jun 9: full vacation day → 0 OT, 450 vacation

	rep, err := s.RangeReport(SeedOwner, "2025-06-01", "2025-06-30")
	if err != nil {
		t.Fatalf("RangeReport: %v", err)
	}
	assertEq(t, "worked", rep.WorkedMin, 480+300+120+450)
	assertEq(t, "overtime", rep.OvertimeMin, 30+0+120+0)
	assertEq(t, "vacation", rep.VacationMin, 450)
	if len(rep.Days) != 4 {
		t.Fatalf("days = %d, want 4", len(rep.Days))
	}
	if !rep.Days[2].Weekend || rep.Days[2].OvertimeMin != 120 {
		t.Errorf("Saturday row wrong: %+v", rep.Days[2])
	}

	// A range that excludes June finds nothing.
	empty, err := s.RangeReport(SeedOwner, "2025-07-01", "2025-07-31")
	if err != nil {
		t.Fatalf("RangeReport empty: %v", err)
	}
	assertEq(t, "empty worked", empty.WorkedMin, 0)
	assertEq(t, "empty overtime", empty.OvertimeMin, 0)
}

func TestSummaryReportFilters(t *testing.T) {
	s := openTest(t)
	a, _ := s.CreateClient(SeedOwner, "Acme", "1", "")
	b, _ := s.CreateClient(SeedOwner, "Bravo", "2", "")
	wts, _ := s.WorkTypes(SeedOwner)
	w1, w2 := wts[0].ID, wts[1].ID
	started := time.Date(2025, 6, 2, 9, 0, 0, 0, time.Local).Unix()
	mk := func(c, w int64, min int64) {
		t.Helper()
		if _, err := s.CreateEntry(SeedOwner, EntryInput{
			ClientID: &c, WorkTypeID: &w, StartedAt: started, DurationMin: min,
		}); err != nil {
			t.Fatalf("CreateEntry: %v", err)
		}
	}
	mk(a.ID, w1, 60)
	mk(a.ID, w2, 30)
	mk(b.ID, w1, 90)

	rows, err := s.SummaryReport(SeedOwner, "2025-06-01", "2025-06-30", "client", &a.ID, nil)
	if err != nil {
		t.Fatalf("SummaryReport client filter: %v", err)
	}
	if len(rows) != 1 || rows[0].WorkedMin != 90 {
		t.Errorf("client filter rows = %+v, want one Acme row of 90", rows)
	}
	rows, err = s.SummaryReport(SeedOwner, "2025-06-01", "2025-06-30", "client", nil, &w1)
	if err != nil {
		t.Fatalf("SummaryReport work-type filter: %v", err)
	}
	var total int64
	for _, r := range rows {
		total += r.WorkedMin
	}
	if len(rows) != 2 || total != 150 {
		t.Errorf("work-type filter rows = %+v, want Acme 60 + Bravo 90", rows)
	}
}

func TestWorkTypeDeleteGuard(t *testing.T) {
	s := openTest(t)
	wts, _ := s.WorkTypes(SeedOwner)
	used := wts[0].ID
	if _, err := s.CreateEntry(SeedOwner, EntryInput{WorkTypeID: &used, DurationMin: 30}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if err := s.DeleteWorkType(SeedOwner, used); err != ErrInUse {
		t.Errorf("delete of used work type = %v, want ErrInUse", err)
	}
	unused := wts[1].ID
	if err := s.DeleteWorkType(SeedOwner, unused); err != nil {
		t.Errorf("delete of unused work type: %v", err)
	}
}

// ---- helpers ----

func mustEntry(t *testing.T, s *Store, dur int64, billable *bool) {
	t.Helper()
	if _, err := s.CreateEntry(SeedOwner, EntryInput{
		Description: "x", DurationMin: dur, Billable: billable,
	}); err != nil {
		t.Fatalf("CreateEntry(%d): %v", dur, err)
	}
}

func mustEntryAt(t *testing.T, s *Store, started, dur int64, billable *bool) {
	t.Helper()
	if _, err := s.CreateEntry(SeedOwner, EntryInput{
		Description: "x", StartedAt: started, DurationMin: dur, Billable: billable,
	}); err != nil {
		t.Fatalf("CreateEntry at %d: %v", started, err)
	}
}

// timeUnix returns the unix seconds for date (YYYY-MM-DD) at the given hour in
// the server's local timezone.
func timeUnix(t *testing.T, date string, hour int) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", date, err)
	}
	return tm.Unix() + int64(hour)*3600
}

func assertEq(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func boolp(b bool) *bool { return &b }
