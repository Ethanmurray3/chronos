package store

import (
	"strconv"
	"time"
)

// Settings are the per-owner knobs that drive the day math.
type Settings struct {
	DailyTargetMin int    `json:"daily_target_min"` // billable target for a day
	WorkdayMin     int    `json:"workday_min"`      // worked minutes before overtime
	RoundingMin    int    `json:"rounding_min"`     // billing increment (e.g. 6 = 0.1h)
	Timezone       string `json:"timezone"`         // IANA name, "" = server local
}

// Settings returns the owner's settings, falling back to sane defaults for
// any key that is missing.
func (s *Store) Settings(owner int64) (Settings, error) {
	out := Settings{DailyTargetMin: 450, WorkdayMin: 480, RoundingMin: 6}
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
		case "daily_target_min":
			out.DailyTargetMin = atoiOr(v, out.DailyTargetMin)
		case "workday_min":
			out.WorkdayMin = atoiOr(v, out.WorkdayMin)
		case "rounding_min":
			out.RoundingMin = atoiOr(v, out.RoundingMin)
		case "timezone":
			out.Timezone = v
		}
	}
	return out, rows.Err()
}

// SaveSettings upserts every field of the owner's settings.
func (s *Store) SaveSettings(owner int64, in Settings) error {
	pairs := map[string]string{
		"daily_target_min": strconv.Itoa(in.DailyTargetMin),
		"workday_min":      strconv.Itoa(in.WorkdayMin),
		"rounding_min":     strconv.Itoa(in.RoundingMin),
		"timezone":         in.Timezone,
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

// location resolves the owner's configured timezone, defaulting to the
// server's local time when unset or invalid.
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
