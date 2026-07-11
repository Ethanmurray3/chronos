// The Today workspace: day navigation, the figures strip, timer hero,
// day timeline, manual entry, and the entries ledger.

import { $, esc, hours, clock, todayISO, shiftISO, intOrNull, toast, GET, POST, DEL, ic } from "../lib/api.js";
import { state, clientOptions, workTypeOptions, onRefsChanged } from "../lib/state.js";
import { refreshRail } from "./todos.js";

export async function refreshToday() {
  const [day, timer] = await Promise.all([
    GET("/api/v1/reports/day?date=" + state.viewDate),
    GET("/api/v1/timer"),
  ]);
  state.day = day;
  state.timer = timer.running ? timer.entry : null;
  renderDayNav();
  renderDaystrip();
  renderTimer();
  renderTimeline();
  renderManualForm();
  renderEntries();
  refreshRail().catch(() => {}); // the rail is a nicety; never block the day
}

// ---------- day navigation ----------
function dayLabel() {
  const today = todayISO();
  if (state.viewDate === today) return "Today";
  if (state.viewDate === shiftISO(today, -1)) return "Yesterday";
  return new Date(state.viewDate + "T12:00:00").toLocaleDateString([], {
    weekday: "short", year: "numeric", month: "short", day: "numeric",
  });
}

function renderDayNav() {
  $("#day-label").textContent = dayLabel();
  $("#day-pick").value = state.viewDate;
  $("#day-today").hidden = state.viewDate === todayISO();
}

function gotoDate(iso) {
  state.viewDate = iso;
  refreshToday()
    .then(() => {
      // Day navigation is intentional, so move the manual-entry default to
      // the selected day. Ordinary background refreshes leave it untouched.
      const form = $("#manual-form");
      if (form?.date) form.date.value = iso;
    })
    .catch((e) => toast(e.message, true));
}

// ---------- the figures strip ----------
function renderDaystrip() {
  const d = state.day;
  const pct = d.target_min > 0 ? Math.min(100, (d.worked_min / d.target_min) * 100) : 0;
  const tail =
    d.overtime_min > 0
      ? `<div class="fig warn"><span class="fig-val">${hours(d.overtime_min)}</span><span class="fig-label">overtime</span></div>`
      : `<div class="fig good"><span class="fig-val">${hours(d.remaining_min)}</span><span class="fig-label">to target of ${hours(d.target_min)}</span></div>`;
  $("#daybar").innerHTML = `
    <div class="fig accent">
      <span class="fig-val">${hours(d.worked_min)}</span><span class="fig-label">worked</span>
      <div class="progress"><span style="width:${pct}%"></span></div>
    </div>
    <div class="fig"><span class="fig-val">${hours(d.billable_min)}</span><span class="fig-label">billable</span></div>
    <div class="fig"><span class="fig-val">${hours(d.non_billable_min)}</span><span class="fig-label">non-billable</span></div>
    ${tail}`;
}

// ---------- timer hero ----------
function renderTimer() {
  const card = $("#timer-card");
  const renderKey = state.timer
    ? `running:${state.timer.id ?? state.timer.started_at}`
    : "idle";

  // Keep an unchanged form mounted. Replacing its innerHTML on every polling
  // refresh discards whatever the user is currently typing.
  if (card.dataset.renderKey === renderKey) {
    updateBarTimer();
    return;
  }

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
        <label>Task<select name="task">${taskOptions()}</select></label>
        <label>Client<select name="client">${clientOptions()}</select></label>
        <label>Work type<select name="work_type">${workTypeOptions()}</select></label>
        <label class="grow">Description<input name="description" placeholder="What are you working on?" /></label>
        <button type="submit">Start</button>
      </form>`;
    $("#start-form").addEventListener("submit", startTimer);
    $("#start-form").task.addEventListener("change", onTaskPicked);
  }
  card.dataset.renderKey = renderKey;
  updateBarTimer();
}

// ---------- the task picker ----------
// Coding time usually follows the to-do list: picking an open task seeds the
// client and description (still editable). Completing the task stays manual.
function taskOptions(sel) {
  const open = (state.todos || []).filter((t) => t.status !== "done");
  return `<option value="">— task —</option>` + open.map((t) =>
    `<option value="${t.id}" ${t.id === sel ? "selected" : ""}>${esc(t.title)}</option>`).join("");
}

function onTaskPicked(e) {
  const form = e.target.form;
  const t = (state.todos || []).find((x) => x.id === parseInt(e.target.value, 10));
  if (!t) return;
  if (t.client_id) form.client.value = String(t.client_id);
  form.description.value = t.title;
}

// The topbar chip mirrors the running timer so it stays visible anywhere on the page.
function updateBarTimer() {
  const chip = $("#bar-timer");
  if (!state.timer) {
    chip.hidden = true;
    return;
  }
  chip.hidden = false;
  $("#bar-timer-label").textContent =
    state.timer.client_name || state.timer.description || "Working";
}

export function tickTimer() {
  if (!state.timer) return;
  const secs = Math.max(0, Math.floor(Date.now() / 1000) - state.timer.started_at);
  const h = Math.floor(secs / 3600);
  const m = String(Math.floor((secs % 3600) / 60)).padStart(2, "0");
  const s = String(secs % 60).padStart(2, "0");
  const clockEl = $("#live-clock");
  if (clockEl) clockEl.innerHTML = `<span class="pulse"></span>${h}:${m}:${s}`;
  $("#bar-timer-clock").textContent = `${h}:${m}:${s}`;
  // keep the running block in the timeline growing toward "now"
  const grow = $("#tl-track .tl-block.running");
  if (grow && grow.dataset.axis) {
    const [start, span] = grow.dataset.axis.split(",").map(Number);
    const now = Math.floor(Date.now() / 1000);
    grow.style.width = Math.max(0.4, ((now - Number(grow.dataset.s)) / span) * 100) + "%";
    const marker = $("#tl-track .tl-now");
    if (marker) marker.style.left = Math.min(100, ((now - start) / span) * 100) + "%";
  }
}

export function scrollToTimer(focusForm = true) {
  // make sure the Today view is frontmost first (app.js owns view switching)
  document.dispatchEvent(new CustomEvent("switch-view", { detail: "today" }));
  $("#today").scrollIntoView({ behavior: "smooth", block: "start" });
  if (focusForm) {
    const el = $("#timer-card select, #timer-card button.stop");
    el?.focus({ preventScroll: true });
  }
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

export async function stopTimer() {
  try {
    await POST("/api/v1/timer/stop");
    await refreshToday();
    toast("Timer stopped and logged");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- the day timeline ----------
function renderTimeline() {
  const wrap = $("#timeline");
  const track = $("#tl-track");
  const entries = state.day.entries;
  if (!entries || entries.length === 0) {
    wrap.hidden = true;
    return;
  }
  wrap.hidden = false;

  const isToday = state.viewDate === todayISO();
  const now = Math.floor(Date.now() / 1000);
  const eightAM = Math.floor(new Date(state.viewDate + "T08:00:00").getTime() / 1000);
  const endOf = (e) => (e.running ? now : e.ended_at || e.started_at + e.effective_min * 60);

  let start = Math.min(eightAM, ...entries.map((e) => e.started_at));
  let end = Math.max(...entries.map(endOf), isToday ? now : 0);
  end = Math.max(end, start + 4 * 3600); // never zoom tighter than a 4h window
  const span = end - start;

  const blocks = entries.map((e) => {
    const s = e.started_at;
    const f = endOf(e);
    const left = ((s - start) / span) * 100;
    const width = Math.max(0.4, ((f - s) / span) * 100);
    const label = `${e.client_name || "No client"} · ${clock(s)}–${e.running ? "now" : clock(f)} · ${hours(e.effective_min)}`;
    return `<span class="tl-block ${e.running ? "running" : ""}" data-s="${s}" data-axis="${start},${span}"
      style="left:${left}%;width:${width}%" title="${esc(label)}"></span>`;
  });

  const nowMark =
    isToday && now >= start && now <= end
      ? `<span class="tl-now" style="left:${((now - start) / span) * 100}%"></span>`
      : "";

  track.innerHTML = blocks.join("") + nowMark;
  $("#tl-start").textContent = clock(start);
  $("#tl-end").textContent = isToday ? "now" : clock(end);
}

// ---------- manual entry ----------
function renderManualForm() {
  const form = $("#manual-form");
  // The form has no server-rendered state, so it only needs to be mounted
  // once. Keeping the nodes alive preserves in-progress input while the day
  // report refreshes in the background.
  if (form.elements.length > 0) return;

  form.innerHTML = `
    <label>Date<input type="date" name="date" value="${state.viewDate}" /></label>
    <label>Task<select name="task">${taskOptions()}</select></label>
    <label>Client<select name="client">${clientOptions()}</select></label>
    <label>Work type<select name="work_type">${workTypeOptions()}</select></label>
    <label class="grow">Description<input name="description" placeholder="Notes" /></label>
    <label>Start<input type="time" name="tstart" class="hrs" /></label>
    <label>End<input type="time" name="tend" class="hrs" /></label>
    <label>or Hours<input type="number" name="dhours" step="0.1" min="0" placeholder="1.5" class="hrs" /></label>
    <label class="chk"><input type="checkbox" name="billable" checked /> Billable</label>
    <button type="submit">Add</button>`;
  form.task.addEventListener("change", onTaskPicked);
}

async function submitManual(e) {
  e.preventDefault();
  const f = e.target;
  // Two ways to say how long: real start/end clock times (3:00 → 4:30 is
  // 1.5h at 3:00), or a bare duration with a nominal 09:00 start.
  let started, durationMin;
  if (f.tstart.value && f.tend.value) {
    started = Math.floor(new Date(f.date.value + "T" + f.tstart.value).getTime() / 1000);
    const ended = Math.floor(new Date(f.date.value + "T" + f.tend.value).getTime() / 1000);
    durationMin = Math.round((ended - started) / 60);
    if (durationMin <= 0) return toast("End time must be after start", true);
  } else if (f.tstart.value || f.tend.value) {
    return toast("Enter both start and end times (or just hours)", true);
  } else {
    const dh = parseFloat(f.dhours.value);
    if (!dh || dh <= 0) return toast("Enter start/end times or hours", true);
    started = Math.floor(new Date(f.date.value + "T09:00:00").getTime() / 1000);
    durationMin = Math.round(dh * 60);
  }
  try {
    await POST("/api/v1/time-entries", {
      client_id: intOrNull(f.client.value),
      work_type_id: intOrNull(f.work_type.value),
      description: f.description.value.trim(),
      started_at: started,
      duration_min: durationMin,
      billable: f.billable.checked,
    });
    f.reset();
    await refreshToday();
    toast("Entry added");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- entries ledger ----------
function renderEntries() {
  const box = $("#entries");
  const viewingToday = state.viewDate === todayISO();
  $("#entries-title").textContent = viewingToday ? "Coded today" : `Coded — ${dayLabel()}`;
  const entries = state.day.entries.filter((e) => !e.running);
  $("#entries-total").textContent = entries.length
    ? `${hours(state.day.worked_min)} · ${entries.length} ${entries.length === 1 ? "entry" : "entries"}`
    : "";
  if (entries.length === 0) {
    box.innerHTML = viewingToday
      ? `<p class="empty">No entries yet today. Start a timer or add one below.</p>`
      : `<p class="empty">Nothing coded on this day.</p>`;
    return;
  }
  box.innerHTML = entries.map((e) => `
    <div class="entry" data-id="${e.id}">
      <div class="when">${clock(e.started_at)}${e.ended_at ? "–" + clock(e.ended_at) : ""}</div>
      <div class="desc">
        <span class="who">${esc(e.client_name || "No client")}</span>
        <span class="what">${e.work_type_name ? `<span class="badge ${e.billable ? "" : "nb"}">${esc(e.work_type_name)}</span>` : ""}${esc(e.description || "")}</span>
      </div>
      <div class="dur">${hours(e.effective_min)}</div>
      <div class="actions">
        <button data-act="del" title="Delete">${ic.x}</button>
      </div>
    </div>`).join("");
}

async function onEntriesClick(e) {
  const btn = e.target.closest("button[data-act]");
  if (!btn) return;
  const id = btn.closest(".entry").dataset.id;
  try {
    if (btn.dataset.act === "del") {
      if (!confirm("Delete this entry?")) return;
      await DEL("/api/v1/time-entries/" + id);
      await refreshToday();
    }
  } catch (err) {
    toast(err.message, true);
  }
}

// keep the form dropdowns current after catalog changes without
// clobbering anything the user has typed or picked
function refreshSelect(sel, optionsHTML) {
  if (!sel) return;
  const v = sel.value;
  sel.innerHTML = optionsHTML;
  sel.value = v;
  if (sel.value !== v) sel.selectedIndex = 0; // selection was deleted
}

function refreshFormOptions() {
  for (const f of [$("#start-form"), $("#manual-form")]) {
    if (!f || !f.client) continue;
    refreshSelect(f.client, clientOptions());
    refreshSelect(f.work_type, workTypeOptions());
  }
}

function refreshTaskPickers() {
  for (const f of [$("#start-form"), $("#manual-form")]) {
    if (f?.task) refreshSelect(f.task, taskOptions());
  }
}

// ---------- wiring ----------
export function initToday() {
  onRefsChanged(refreshFormOptions);
  document.addEventListener("todos-changed", refreshTaskPickers);
  $("#day-prev").addEventListener("click", () => gotoDate(shiftISO(state.viewDate, -1)));
  $("#day-next").addEventListener("click", () => gotoDate(shiftISO(state.viewDate, 1)));
  $("#day-today").addEventListener("click", () => gotoDate(todayISO()));
  $("#day-pick").addEventListener("change", (e) => {
    if (e.target.value) gotoDate(e.target.value);
  });
  $("#manual-form").addEventListener("submit", submitManual);
  $("#entries").addEventListener("click", onEntriesClick);
}
