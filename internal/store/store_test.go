package store

import (
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

func TestDayReportMath(t *testing.T) {
	s := openTest(t)
	// 100 min billable, 50 min non-billable, 7 min billable (rounds to 12).
	mustEntry(t, s, 100, boolp(true))
	mustEntry(t, s, 50, boolp(false))
	mustEntry(t, s, 7, boolp(true))

	rep, err := s.DayReport(SeedOwner, "")
	if err != nil {
		t.Fatalf("DayReport: %v", err)
	}
	assertEq(t, "worked", rep.WorkedMin, 157)
	assertEq(t, "billable", rep.BillableMin, 107)
	assertEq(t, "billed", rep.BilledMin, 114) // 102 + 12
	assertEq(t, "target", rep.TargetMin, 450)
	assertEq(t, "remaining", rep.RemainingMin, 343) // 450 - 107
	assertEq(t, "overtime", rep.OvertimeMin, 0)     // 157 < 480
}

func TestDayReportOvertime(t *testing.T) {
	s := openTest(t)
	mustEntry(t, s, 500, boolp(true)) // > 480 workday
	rep, err := s.DayReport(SeedOwner, "")
	if err != nil {
		t.Fatalf("DayReport: %v", err)
	}
	assertEq(t, "overtime", rep.OvertimeMin, 20)   // 500 - 480
	assertEq(t, "remaining", rep.RemainingMin, 0)  // target exceeded → clamped
}

func TestDayBoundaryTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	s := openTest(t)
	if err := s.SaveSettings(SeedOwner, Settings{
		DailyTargetMin: 450, WorkdayMin: 480, RoundingMin: 6, Timezone: "America/Toronto",
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

// ---- helpers ----

func mustEntry(t *testing.T, s *Store, dur int64, billable *bool) {
	t.Helper()
	if _, err := s.CreateEntry(SeedOwner, EntryInput{
		Description: "x", DurationMin: dur, Billable: billable,
	}); err != nil {
		t.Fatalf("CreateEntry(%d): %v", dur, err)
	}
}

func assertEq(t *testing.T, name string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func boolp(b bool) *bool { return &b }
