// The reports section: range + group-by summary table with CSV export.
// Auto-runs the current month the first time it scrolls into view.

import { $, esc, hours, todayISO, toast, GET } from "../lib/api.js";

let ran = false;

export function initReports() {
  const f = $("#report-form");
  const d = new Date();
  f.from.value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-01`;
  f.to.value = todayISO();
  updateExportLink();
  f.addEventListener("submit", runReport);
  f.addEventListener("change", updateExportLink);
}

// first-scroll-into-view hook so the section is never empty
export function autoRunReports() {
  if (ran) return;
  $("#report-form").requestSubmit();
}

function updateExportLink() {
  $("#export-link").href = exportURL();
}

export function exportURL() {
  const f = $("#report-form");
  return `/api/v1/export.csv?from=${f.from.value}&to=${f.to.value}`;
}

async function runReport(e) {
  e.preventDefault();
  ran = true;
  const f = e.target;
  updateExportLink();
  try {
    const { rows } = await GET(
      `/api/v1/reports/summary?from=${f.from.value}&to=${f.to.value}&group_by=${f.group_by.value}`);
    renderReport(rows, f.group_by.value);
  } catch (err) {
    toast(err.message, true);
  }
}

function renderReport(rows, groupBy) {
  const out = $("#report-out");
  if (!rows || rows.length === 0) {
    out.innerHTML = `<p class="empty">No time in this range.</p>`;
    return;
  }
  const head = { client: "Client", work_type: "Work type", day: "Day" }[groupBy] || "Group";
  let tWorked = 0, tBill = 0, tBilled = 0, tCount = 0;
  const body = rows.map((r) => {
    tWorked += r.worked_min; tBill += r.billable_min; tBilled += r.billed_min; tCount += r.count;
    return `<tr><td>${esc(r.label)}</td>
      <td class="num">${hours(r.worked_min)}</td>
      <td class="num">${hours(r.billable_min)}</td>
      <td class="num">${hours(r.billed_min)}</td>
      <td class="num">${r.count}</td></tr>`;
  }).join("");
  out.innerHTML = `<table>
    <thead><tr><th>${head}</th><th class="num">Worked</th><th class="num">Billable</th><th class="num">Billed</th><th class="num">Entries</th></tr></thead>
    <tbody>${body}</tbody>
    <tfoot><tr><td>Total</td><td class="num">${hours(tWorked)}</td><td class="num">${hours(tBill)}</td><td class="num">${hours(tBilled)}</td><td class="num">${tCount}</td></tr></tfoot>
  </table>`;
}
