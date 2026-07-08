// Chronos frontend — vanilla ES modules, no build step. Talks to /api/v1.

// ---------- tiny helpers ----------
const $ = (sel, root = document) => root.querySelector(sel);
const esc = (s) =>
  String(s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
const GET = (p) => api("GET", p);
const POST = (p, b) => api("POST", p, b);
const PUT = (p, b) => api("PUT", p, b);
const DEL = (p) => api("DELETE", p);

const hours = (min) => (min / 60).toFixed(1) + "h";
const clock = (unix) =>
  new Date(unix * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
const todayISO = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

function toast(msg, isErr = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (isErr ? " err" : "");
  t.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => (t.hidden = true), 2600);
}

// ---------- state ----------
const state = { settings: null, clients: [], workTypes: [], timer: null, day: null };

// ---------- boot ----------
async function boot() {
  wireNav();
  wireSettings();
  wireForms();
  try {
    await reloadRefs();
    await refreshToday();
  } catch (e) {
    toast("Load failed: " + e.message, true);
  }
  setInterval(tickTimer, 1000);
  setInterval(() => { if (currentView === "today") refreshToday(); }, 30000);
}

async function reloadRefs() {
  const [s, c, w] = await Promise.all([
    GET("/api/v1/settings"),
    GET("/api/v1/clients"),
    GET("/api/v1/work-types"),
  ]);
  state.settings = s;
  state.clients = c.clients;
  state.workTypes = w.work_types;
}

async function refreshToday() {
  const [day, timer] = await Promise.all([
    GET("/api/v1/reports/day"),
    GET("/api/v1/timer"),
  ]);
  state.day = day;
  state.timer = timer.running ? timer.entry : null;
  renderDaybar();
  renderTimer();
  renderManualForm();
  renderEntries();
}

// ---------- navigation ----------
let currentView = "today";
function wireNav() {
  $("#tabs").addEventListener("click", (e) => {
    const btn = e.target.closest("button[data-view]");
    if (!btn) return;
    currentView = btn.dataset.view;
    for (const b of $("#tabs").children) b.classList.toggle("active", b === btn);
    for (const v of document.querySelectorAll(".view")) v.hidden = v.id !== "view-" + currentView;
    if (currentView === "clients") renderClients();
    if (currentView === "reports") initReports();
  });
}

// ---------- today: day bar ----------
function renderDaybar() {
  const d = state.day;
  const pct = d.target_min > 0 ? Math.min(100, (d.billable_min / d.target_min) * 100) : 0;
  $("#daybar").innerHTML = `
    <div class="stat"><div class="label">Worked</div><div class="value">${hours(d.worked_min)}</div></div>
    <div class="stat accent"><div class="label">Billable</div><div class="value">${hours(d.billable_min)}</div>
      <div class="progress"><span style="width:${pct}%"></span></div></div>
    <div class="stat"><div class="label">Target</div><div class="value small">${hours(d.target_min)}</div></div>
    <div class="stat good"><div class="label">Remaining</div><div class="value small">${hours(d.remaining_min)}</div></div>
    <div class="stat ${d.overtime_min > 0 ? "warn" : ""}"><div class="label">Overtime</div><div class="value small">${hours(d.overtime_min)}</div></div>`;
}

// ---------- today: timer ----------
function clientOptions(sel) {
  return `<option value="">— client —</option>` +
    state.clients.map((c) => `<option value="${c.id}" ${c.id === sel ? "selected" : ""}>${esc(c.name)}</option>`).join("");
}
function workTypeOptions(sel) {
  return `<option value="">— work type —</option>` +
    state.workTypes.map((w) => `<option value="${w.id}" ${w.id === sel ? "selected" : ""}>${esc(w.name)}</option>`).join("");
}

function renderTimer() {
  const card = $("#timer-card");
  if (state.timer) {
    const t = state.timer;
    card.innerHTML = `
      <div class="timer-running">
        <div class="timer-clock" id="live-clock"><span class="pulse"></span>0:00:00</div>
        <div class="timer-meta">
          <div class="who">${esc(t.client_name || "No client")}</div>
          <div class="what">${t.work_type_name ? `<span class="badge">${esc(t.work_type_name)}</span>` : ""}${esc(t.description || "…")}</div>
        </div>
        <button class="stop" id="stop-btn">Stop</button>
      </div>`;
    $("#stop-btn").addEventListener("click", stopTimer);
    tickTimer();
  } else {
    card.innerHTML = `
      <form class="start-form" id="start-form">
        <label>Client<select name="client">${clientOptions()}</select></label>
        <label>Work type<select name="work_type">${workTypeOptions()}</select></label>
        <label class="grow">Description<input name="description" placeholder="What are you working on?" /></label>
        <button type="submit">Start</button>
      </form>`;
    $("#start-form").addEventListener("submit", startTimer);
  }
}

function tickTimer() {
  if (!state.timer) return;
  const clockEl = $("#live-clock");
  if (!clockEl) return;
  const secs = Math.max(0, Math.floor(Date.now() / 1000) - state.timer.started_at);
  const h = Math.floor(secs / 3600);
  const m = String(Math.floor((secs % 3600) / 60)).padStart(2, "0");
  const s = String(secs % 60).padStart(2, "0");
  clockEl.innerHTML = `<span class="pulse"></span>${h}:${m}:${s}`;
}

async function startTimer(e) {
  e.preventDefault();
  const f = e.target;
  try {
    await POST("/api/v1/timer/start", {
      client_id: intOrNull(f.client.value),
      work_type_id: intOrNull(f.work_type.value),
      description: f.description.value.trim(),
    });
    await refreshToday();
  } catch (err) {
    toast(err.message, true);
  }
}

async function stopTimer() {
  try {
    await POST("/api/v1/timer/stop");
    await refreshToday();
    toast("Timer stopped and logged");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- today: manual entry ----------
function renderManualForm() {
  $("#manual-form").innerHTML = `
    <label>Date<input type="date" name="date" value="${todayISO()}" /></label>
    <label>Client<select name="client">${clientOptions()}</select></label>
    <label>Work type<select name="work_type">${workTypeOptions()}</select></label>
    <label class="grow">Description<input name="description" placeholder="Notes" /></label>
    <label>Hours<input type="number" name="dhours" step="0.1" min="0" placeholder="1.5" style="max-width:90px" /></label>
    <label style="flex-direction:row;align-items:center;gap:.4rem"><input type="checkbox" name="billable" checked /> Billable</label>
    <button type="submit">Add</button>`;
}

async function submitManual(e) {
  e.preventDefault();
  const f = e.target;
  const dh = parseFloat(f.dhours.value);
  if (!dh || dh <= 0) return toast("Enter hours", true);
  // Nominal start: 09:00 local on the chosen date.
  const started = Math.floor(new Date(f.date.value + "T09:00:00").getTime() / 1000);
  try {
    await POST("/api/v1/time-entries", {
      client_id: intOrNull(f.client.value),
      work_type_id: intOrNull(f.work_type.value),
      description: f.description.value.trim(),
      started_at: started,
      duration_min: Math.round(dh * 60),
      billable: f.billable.checked,
    });
    f.reset();
    renderManualForm();
    await refreshToday();
    toast("Entry added");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- today: entries list ----------
function renderEntries() {
  const box = $("#entries");
  const entries = state.day.entries.filter((e) => !e.running);
  if (entries.length === 0) {
    box.innerHTML = `<p class="empty">No entries yet today. Start a timer or add one above.</p>`;
    return;
  }
  box.innerHTML = entries.map((e) => `
    <div class="entry" data-id="${e.id}">
      <div class="when">${clock(e.started_at)}${e.ended_at ? "–" + clock(e.ended_at) : ""}</div>
      <div class="desc">
        <span class="who">${esc(e.client_name || "No client")}</span>
        <div class="what">${e.work_type_name ? `<span class="badge ${e.billable ? "" : "nb"}">${esc(e.work_type_name)}</span>` : ""}${esc(e.description || "")}</div>
      </div>
      <div class="dur">${hours(e.effective_min)}</div>
      <div class="actions"><button data-act="del" title="Delete">✕</button></div>
    </div>`).join("");
}

async function onEntriesClick(e) {
  const btn = e.target.closest("button[data-act='del']");
  if (!btn) return;
  const id = btn.closest(".entry").dataset.id;
  if (!confirm("Delete this entry?")) return;
  try {
    await DEL("/api/v1/time-entries/" + id);
    await refreshToday();
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- clients ----------
let selectedClient = null;
function renderClients() {
  const list = $("#client-list");
  if (state.clients.length === 0) {
    list.innerHTML = `<p class="empty">No clients yet. Add one to start attributing time.</p>`;
    return;
  }
  list.innerHTML = state.clients.map((c) => `
    <div class="client-row ${c.id === selectedClient ? "sel" : ""}" data-id="${c.id}">
      <span>${esc(c.name)}</span><span class="code">${esc(c.code || "")}</span>
    </div>`).join("");
}

async function openClient(id) {
  selectedClient = id;
  renderClients();
  const c = state.clients.find((x) => x.id === id);
  $("#client-detail-title").textContent = c ? c.name : "Client";
  const { entries } = await GET("/api/v1/time-entries?client_id=" + id);
  const done = entries.filter((e) => !e.running);
  const total = done.reduce((s, e) => s + e.effective_min, 0);
  const detail = $("#client-detail");
  if (done.length === 0) {
    detail.innerHTML = `<p class="empty">No time recorded for this client yet.</p>`;
    return;
  }
  detail.innerHTML =
    `<p>Total tracked: <span class="tot">${hours(total)}</span> across ${done.length} entries</p>` +
    `<div class="entries">` +
    done.slice(0, 50).map((e) => `
      <div class="entry">
        <div class="when">${new Date(e.started_at * 1000).toLocaleDateString([], { month: "short", day: "numeric" })}</div>
        <div class="desc"><div class="what">${e.work_type_name ? `<span class="badge">${esc(e.work_type_name)}</span>` : ""}${esc(e.description || "")}</div></div>
        <div class="dur">${hours(e.effective_min)}</div><div></div>
      </div>`).join("") + `</div>`;
}

async function submitClient(e) {
  e.preventDefault();
  const f = e.target;
  try {
    await POST("/api/v1/clients", { name: f.name.value.trim(), code: f.code.value.trim() });
    f.reset();
    await reloadRefs();
    renderClients();
    toast("Client added");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- reports ----------
function initReports() {
  const f = $("#report-form");
  if (!f.from.value) {
    const d = new Date();
    const first = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-01`;
    f.from.value = first;
    f.to.value = todayISO();
  }
  updateExportLink();
}

function updateExportLink() {
  const f = $("#report-form");
  $("#export-link").href = `/api/v1/export.csv?from=${f.from.value}&to=${f.to.value}`;
}

async function runReport(e) {
  e.preventDefault();
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

// ---------- settings ----------
function wireSettings() {
  const dlg = $("#settings-dialog");
  $("#settings-btn").addEventListener("click", () => {
    const s = state.settings;
    const f = $("#settings-form");
    f.target.value = (s.daily_target_min / 60).toFixed(1);
    f.workday.value = (s.workday_min / 60).toFixed(1);
    f.rounding.value = s.rounding_min;
    f.timezone.value = s.timezone || "";
    dlg.showModal();
  });
  $("#settings-form").addEventListener("submit", async (e) => {
    if (e.submitter && e.submitter.value !== "save") return; // cancel
    e.preventDefault();
    const f = e.target;
    try {
      state.settings = await PUT("/api/v1/settings", {
        daily_target_min: Math.round(parseFloat(f.target.value) * 60) || 0,
        workday_min: Math.round(parseFloat(f.workday.value) * 60) || 0,
        rounding_min: parseInt(f.rounding.value, 10) || 0,
        timezone: f.timezone.value.trim(),
      });
      dlg.close();
      await refreshToday();
      toast("Settings saved");
    } catch (err) {
      toast(err.message, true);
    }
  });
}

// ---------- form wiring ----------
function wireForms() {
  $("#manual-form").addEventListener("submit", submitManual);
  $("#entries").addEventListener("click", onEntriesClick);
  $("#client-form").addEventListener("submit", submitClient);
  $("#client-list").addEventListener("click", (e) => {
    const row = e.target.closest(".client-row");
    if (row) openClient(parseInt(row.dataset.id, 10));
  });
  $("#report-form").addEventListener("submit", runReport);
  $("#report-form").addEventListener("change", updateExportLink);
}

const intOrNull = (v) => (v ? parseInt(v, 10) : null);

boot();
