package store

import (
	"strconv"
	"time"
)

// busySeasonStartMonth / busySeasonEndMonth define the tax "busy season"
// (January–April). Days in this window use the busy-season target; the rest of
// the year uses the off-season target. Hardcoded for now — a common Canadian
// tax calendar — but easy to promote to a setting later.
const (
	busySeasonStartMonth = 1
	busySeasonEndMonth   = 4
)

// Settings are the per-owner knobs that drive the day math. The daily standard
// (target) is seasonal and doubles as the overtime threshold: hours coded past
// the standard for that day are overtime.
type Settings struct {
	BusySeasonTargetMin int    `json:"busy_season_target_min"` // Jan–Apr, default 480 (8h)
	OffSeasonTargetMin  int    `json:"off_season_target_min"`  // May–Dec, default 450 (7.5h)
	RoundingMin         int    `json:"rounding_min"`           // billing increment (e.g. 6 = 0.1h)
	StaleDays           int    `json:"stale_days"`             // open todo age before "stale"
	DueSoonDays         int    `json:"due_soon_days"`          // days ahead that count as "due soon"
	Timezone            string `json:"timezone"`               // IANA name, "" = server local
}

// Settings returns the owner's settings, falling back to sane defaults for any
// key that is missing.
func (s *Store) Settings(owner int64) (Settings, error) {
	out := Settings{BusySeasonTargetMin: 480, OffSeasonTargetMin: 450, RoundingMin: 6, StaleDays: 14, DueSoonDays: 2}
	rows, err := s.db.Query(`SELECT k, v FROM settings WHERE owner_id = ?`, owner)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "busy_season_target_min":
			out.BusySeasonTargetMin = atoiOr(v, out.BusySeasonTargetMin)
		case "off_season_target_min":
			out.OffSeasonTargetMin = atoiOr(v, out.OffSeasonTargetMin)
		case "rounding_min":
			out.RoundingMin = atoiOr(v, out.RoundingMin)
		case "stale_days":
			out.StaleDays = atoiOr(v, out.StaleDays)
		case "due_soon_days":
			out.DueSoonDays = atoiOr(v, out.DueSoonDays)
		case "timezone":
			out.Timezone = v
		}
	}
	return out, rows.Err()
}

// SaveSettings upserts every field of the owner's settings.
func (s *Store) SaveSettings(owner int64, in Settings) error {
	pairs := map[string]string{
		"busy_season_target_min": strconv.Itoa(in.BusySeasonTargetMin),
		"off_season_target_min":  strconv.Itoa(in.OffSeasonTargetMin),
		"rounding_min":           strconv.Itoa(in.RoundingMin),
		"stale_days":             strconv.Itoa(in.StaleDays),
		"due_soon_days":          strconv.Itoa(in.DueSoonDays),
		"timezone":               in.Timezone,
	}
	for k, v := range pairs {
		if _, err := s.db.Exec(
			`INSERT INTO settings (owner_id, k, v) VALUES (?, ?, ?)
			 ON CONFLICT (owner_id, k) DO UPDATE SET v = excluded.v`,
			owner, k, v); err != nil {
			return err
		}
	}
	return nil
}

// TargetMinForMonth returns the daily standard (in minutes) for a calendar
// month: the busy-season target during Jan–Apr, otherwise the off-season one.
func (s Settings) TargetMinForMonth(month time.Month) int {
	if int(month) >= busySeasonStartMonth && int(month) <= busySeasonEndMonth {
		return s.BusySeasonTargetMin
	}
	return s.OffSeasonTargetMin
}

// TodayISO returns today's date (YYYY-MM-DD) in the owner's timezone — the
// reference point for due-date comparisons.
func (s *Store) TodayISO(owner int64) (string, error) {
	set, err := s.Settings(owner)
	if err != nil {
		return "", err
	}
	return time.Now().In(set.location()).Format("2006-01-02"), nil
}

// location resolves the owner's configured timezone, defaulting to the server's
// local time when unset or invalid.
func (s Settings) location() *time.Location {
	if s.Timezone == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	return time.Local
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
