package store

import (
	"fmt"
	"time"
)

// DayReport is the live snapshot the Today view is built on. All figures are
// in minutes.
type DayReport struct {
	Date        string  `json:"date"`         // YYYY-MM-DD
	WorkedMin   int64   `json:"worked_min"`   // all time logged (incl. running)
	BillableMin int64   `json:"billable_min"` // the billable share, unrounded
	BilledMin   int64   `json:"billed_min"`   // billable share rounded per entry
	TargetMin   int64   `json:"target_min"`   // billable target for the day
	RemainingMin int64  `json:"remaining_min"` // max(0, target - billable)
	OvertimeMin int64   `json:"overtime_min"` // max(0, worked - workday)
	Entries     []Entry `json:"entries"`
}

// DayReport aggregates a single calendar day (in the owner's timezone). An
// empty date means today.
func (s *Store) DayReport(owner int64, date string) (DayReport, error) {
	set, err := s.Settings(owner)
	if err != nil {
		return DayReport{}, err
	}
	loc := set.location()
	start, end, isoDate, err := dayBounds(date, loc)
	if err != nil {
		return DayReport{}, err
	}
	entries, err := s.ListEntries(owner, &start, &end, nil)
	if err != nil {
		return DayReport{}, err
	}

	rep := DayReport{Date: isoDate, TargetMin: int64(set.DailyTargetMin), Entries: entries}
	for _, e := range entries {
		rep.WorkedMin += e.EffectiveMin
		if e.Billable {
			rep.BillableMin += e.EffectiveMin
			rep.BilledMin += roundUpTo(e.EffectiveMin, int64(set.RoundingMin))
		}
	}
	rep.RemainingMin = max64(0, rep.TargetMin-rep.BillableMin)
	rep.OvertimeMin = max64(0, rep.WorkedMin-int64(set.WorkdayMin))
	return rep, nil
}

// SummaryRow is one grouped bucket in a range report.
type SummaryRow struct {
	Key         string `json:"key"`   // id or date, as a string
	Label       string `json:"label"` // human name
	WorkedMin   int64  `json:"worked_min"`
	BillableMin int64  `json:"billable_min"`
	BilledMin   int64  `json:"billed_min"`
	Count       int    `json:"count"`
}

// SummaryReport rolls entries in [fromDate, toDate] (inclusive days, owner tz)
// up by client, work_type, or day. Rows are sorted by worked time descending.
func (s *Store) SummaryReport(owner int64, fromDate, toDate, groupBy string) ([]SummaryRow, error) {
	set, err := s.Settings(owner)
	if err != nil {
		return nil, err
	}
	loc := set.location()
	start, _, _, err := dayBounds(fromDate, loc)
	if err != nil {
		return nil, err
	}
	_, end, _, err := dayBounds(toDate, loc)
	if err != nil {
		return nil, err
	}
	entries, err := s.ListEntries(owner, &start, &end, nil)
	if err != nil {
		return nil, err
	}

	rounding := int64(set.RoundingMin)
	buckets := map[string]*SummaryRow{}
	order := []string{}
	get := func(key, label string) *SummaryRow {
		r, ok := buckets[key]
		if !ok {
			r = &SummaryRow{Key: key, Label: label}
			buckets[key] = r
			order = append(order, key)
		}
		return r
	}

	for _, e := range entries {
		var key, label string
		switch groupBy {
		case "work_type":
			if e.WorkTypeID != nil {
				key = fmt.Sprintf("%d", *e.WorkTypeID)
				label = orDash(e.WorkTypeName)
			} else {
				key, label = "0", "(no work type)"
			}
		case "day":
			d := time.Unix(e.StartedAt, 0).In(loc).Format("2006-01-02")
			key, label = d, d
		default: // client
			if e.ClientID != nil {
				key = fmt.Sprintf("%d", *e.ClientID)
				label = orDash(e.ClientName)
			} else {
				key, label = "0", "(no client)"
			}
		}
		r := get(key, label)
		r.WorkedMin += e.EffectiveMin
		r.Count++
		if e.Billable {
			r.BillableMin += e.EffectiveMin
			r.BilledMin += roundUpTo(e.EffectiveMin, rounding)
		}
	}

	out := make([]SummaryRow, 0, len(order))
	for _, k := range order {
		out = append(out, *buckets[k])
	}
	// Simple insertion sort by WorkedMin desc — row counts are tiny.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].WorkedMin > out[j-1].WorkedMin; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// DateBounds returns [start, end) unix seconds spanning the inclusive day
// range [fromDate, toDate] in the owner's timezone. Empty dates mean today.
func (s *Store) DateBounds(owner int64, fromDate, toDate string) (start, end int64, err error) {
	set, err := s.Settings(owner)
	if err != nil {
		return 0, 0, err
	}
	loc := set.location()
	start, _, _, err = dayBounds(fromDate, loc)
	if err != nil {
		return 0, 0, err
	}
	_, end, _, err = dayBounds(toDate, loc)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// dayBounds returns [start, end) unix seconds for the given YYYY-MM-DD in loc,
// plus the normalized ISO date. Empty date means today.
func dayBounds(date string, loc *time.Location) (start, end int64, iso string, err error) {
	var day time.Time
	if date == "" {
		now := time.Now().In(loc)
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	} else {
		day, err = time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid date %q: %w", date, err)
		}
	}
	start = day.Unix()
	end = day.AddDate(0, 0, 1).Unix()
	return start, end, day.Format("2006-01-02"), nil
}

// roundUpTo rounds min up to the nearest multiple of step (the billing
// increment). A step of 0 or less is a no-op.
func roundUpTo(min, step int64) int64 {
	if step <= 0 || min <= 0 {
		return min
	}
	return ((min + step - 1) / step) * step
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
