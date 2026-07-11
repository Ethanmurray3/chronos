package api

import (
	"fmt"
	"net/http"

	"github.com/xuri/excelize/v2"
)

// exportXLSX writes the current report as a real Excel workbook: a Summary
// sheet with the period totals (worked, billable, overtime, vacation) and a
// Breakdown sheet with the grouped rows. Query params mirror the reports
// dialog: from, to, group_by, client_id, work_type_id.
func (s *Server) exportXLSX(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	owner := currentOwner(r)
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "client"
	}

	rng, err := s.st.RangeReport(owner, q.Get("from"), q.Get("to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := s.st.SummaryReport(owner, q.Get("from"), q.Get("to"), groupBy,
		queryInt(q.Get("client_id")), queryInt(q.Get("work_type_id")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	hours := func(min int64) float64 { return float64(min) / 60 }

	// ---- Summary sheet ----
	const sum = "Summary"
	f.SetSheetName("Sheet1", sum)
	set := func(sheet, cell string, v any) { _ = f.SetCellValue(sheet, cell, v) }
	set(sum, "A1", "Chronos report")
	set(sum, "A2", "From")
	set(sum, "B2", rng.From)
	set(sum, "A3", "To")
	set(sum, "B3", rng.To)
	for i, kv := range []struct {
		label string
		min   int64
	}{
		{"Worked (h)", rng.WorkedMin},
		{"Billable (h)", rng.BillableMin},
		{"Non-billable (h)", rng.NonBillableMin},
		{"Billed (h)", rng.BilledMin},
		{"Overtime (h)", rng.OvertimeMin},
		{"Vacation (h)", rng.VacationMin},
	} {
		row := 5 + i
		set(sum, fmt.Sprintf("A%d", row), kv.label)
		set(sum, fmt.Sprintf("B%d", row), hours(kv.min))
	}

	// ---- Breakdown sheet ----
	const brk = "Breakdown"
	if _, err := f.NewSheet(brk); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for col, h := range []string{"Group (" + groupBy + ")", "Worked (h)", "Billable (h)", "Billed (h)", "Entries"} {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		set(brk, cell, h)
	}
	for i, row := range rows {
		set(brk, fmt.Sprintf("A%d", i+2), row.Label)
		set(brk, fmt.Sprintf("B%d", i+2), hours(row.WorkedMin))
		set(brk, fmt.Sprintf("C%d", i+2), hours(row.BillableMin))
		set(brk, fmt.Sprintf("D%d", i+2), hours(row.BilledMin))
		set(brk, fmt.Sprintf("E%d", i+2), row.Count)
	}
	_ = f.SetColWidth(sum, "A", "A", 18)
	_ = f.SetColWidth(brk, "A", "A", 28)

	name := fmt.Sprintf("chronos-%s-to-%s.xlsx", rng.From, rng.To)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := f.Write(w); err != nil {
		// headers are gone; nothing useful to send
		return
	}
}
