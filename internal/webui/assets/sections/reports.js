// The reports overlay: a mini dashboard for any date range. Summary tiles
// (worked, billable, overtime, vacation) from /reports/range on top, the
// grouped/filterable breakdown from /reports/summary below, and the whole
// thing exportable as a real Excel workbook.

import { $, esc, hours, toast, GET } from "../lib/api.js";
import { clientOptions, workTypeOptions, onRefsChanged } from "../lib/state.js";

const iso = (d) => d.toISOString().slice(0, 10);

function rangeFor(chip) {
  const now = new Date();
  const y = now.getFullYear(), m = now.getMonth();
  switch (chip) {
    case "last-month": return [new Date(y, m - 1, 1), new Date(y, m, 0)];
    case "this-year":  return [new Date(y, 0, 1), now];
    case "last-year":  return [new Date(y - 1, 0, 1), new Date(y - 1, 11, 31)];
    default:           return [new Date(y, m, 1), now]; // this month
  }
}

function params() {
  const q = new URLSearchParams({ from: $("#rep-from").value, to: $("#rep-to").value });
  q.set("group_by", $("#rep-group").value);
  if ($("#rep-client").value) q.set("client_id", $("#rep-client").value);
  if ($("#rep-worktype").value) q.set("work_type_id", $("#rep-worktype").value);
  return q;
}

export function openReports() {
  if (!$("#rep-from").value) setRange("this-month");
  $("#reports-dialog").showModal();
  runReport();
}

function setRange(chip) {
  const [from, to] = rangeFor(chip);
  $("#rep-from").value = iso(from);
  $("#rep-to").value = iso(to);
  for (const b of $("#range-chips").children) b.classList.toggle("active", b.dataset.range === chip);
}

async function runReport() {
  const from = $("#rep-from").value, to = $("#rep-to").value;
  if (!from || !to) return;
  const q = params();
  $("#rep-export").href = "/api/v1/reports/export.xlsx?" + q.toString();
  try {
    const [rng, { rows }] = await Promise.all([
      GET(`/api/v1/reports/range?from=${from}&to=${to}`),
      GET("/api/v1/reports/summary?" + q.toString()),
    ]);
    renderTiles(rng);
    renderTable(rows, $("#rep-group").value);
  } catch (err) {
    toast(err.message, true);
  }
}

function renderTiles(r) {
  const tile = (label, min, cls = "") =>
    `<div class="fig ${cls}"><span class="fig-val">${hours(min)}</span><span class="fig-label">${label}</span></div>`;
  $("#report-tiles").innerHTML =
    tile("worked", r.worked_min, "accent") +
    tile("billable", r.billable_min) +
    tile("non-billable", r.non_billable_min) +
    tile("overtime", r.overtime_min, r.overtime_min > 0 ? "warn" : "") +
    tile("vacation", r.vacation_min, "good");
}

function renderTable(rows, groupBy) {
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

function renderFilters() {
  $("#rep-client").innerHTML = clientOptions().replace("— client —", "All clients");
  $("#rep-worktype").innerHTML = workTypeOptions().replace("— work type —", "All work types");
}

export function initReports() {
  onRefsChanged(renderFilters);
  $("#reports-btn").addEventListener("click", openReports);
  $("#range-chips").addEventListener("click", (e) => {
    const b = e.target.closest("button[data-range]");
    if (!b) return;
    setRange(b.dataset.range);
    runReport();
  });
  for (const id of ["#rep-from", "#rep-to", "#rep-group", "#rep-client", "#rep-worktype"]) {
    $(id).addEventListener("change", () => {
      // custom dates clear the chip highlight
      if (id === "#rep-from" || id === "#rep-to")
        for (const b of $("#range-chips").children) b.classList.remove("active");
      runReport();
    });
  }
}
