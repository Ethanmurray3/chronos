// The to-do rail beside the entries ledger: attention-flagged items first
// (with their reason badges), then the rest of the open list, then a
// collapsed Completed group.

import { $, esc, todayISO, intOrNull, toast, GET, POST, DEL, ic } from "../lib/api.js";
import { state, clientOptions, onRefsChanged } from "../lib/state.js";
import { refreshToday, scrollToTimer } from "./today.js";

const REASON_LABEL = {
  overdue: "Overdue",
  "due-today": "Due today",
  "due-soon": "Due soon",
  "high-priority": "High priority",
  stale: "Stale",
};

export async function refreshRail() {
  const [{ todos }, att] = await Promise.all([
    GET("/api/v1/todos?include_done=1"),
    GET("/api/v1/attention").catch(() => ({ items: [] })),
  ]);
  state.todos = todos;
  state.attention = att.items || [];
  renderRail();
}

function renderRail() {
  const attIndex = new Map(state.attention.map((it, i) => [it.id, i]));
  const attById = new Map(state.attention.map((it) => [it.id, it]));

  const open = state.todos.filter((t) => t.status !== "done");
  const done = state.todos.filter((t) => t.status === "done");
  // attention items float to the top in severity order; the rest keep API order
  open.sort((a, b) => (attIndex.get(a.id) ?? Infinity) - (attIndex.get(b.id) ?? Infinity));

  const list = $("#todo-list");
  list.innerHTML = open.length
    ? open.map((t) => todoRow(t, attById.get(t.id))).join("")
    : `<p class="empty">Nothing on your list. Add something above.</p>`;

  const wrap = $("#todo-done-wrap");
  if (done.length) {
    wrap.hidden = false;
    wrap.querySelector("summary").textContent = `Completed (${done.length})`;
    $("#todo-done").innerHTML = done.map((t) => todoRow(t)).join("");
  } else {
    wrap.hidden = true;
  }
}

function todoRow(t, att) {
  const due = t.due_date
    ? `<span class="due ${overdue(t.due_date) && t.status !== "done" ? "over" : ""}">${fmtDue(t.due_date)}</span>`
    : "";
  const ageDays = Math.floor((Date.now() / 1000 - t.created_at) / 86400);
  const age = t.status !== "done" && ageDays >= 1
    ? `<span class="age ${ageDays >= 14 ? "stale" : ""}">${ageDays}d on list</span>`
    : "";
  const badges = att
    ? `<span class="todo-badges">${att.reasons.map((r) =>
        `<span class="rbadge r-${r}">${r === "stale" ? `${att.age_days}d` : REASON_LABEL[r]}</span>`).join("")}</span>`
    : "";
  const main = t.status !== "done"
    ? `<button type="button" class="todo-main" data-act="start" title="Log the current timer and start this task">
        <span class="todo-title">${esc(t.title)}${badges}</span>
        <span class="todo-meta">${t.client_name ? `<span class="who">${esc(t.client_name)}</span>` : ""}${due}${age}</span>
      </button>`
    : `<div class="todo-main">
        <span class="todo-title">${esc(t.title)}${badges}</span>
        <span class="todo-meta">${t.client_name ? `<span class="who">${esc(t.client_name)}</span>` : ""}${due}${age}</span>
      </div>`;
  return `<div class="todo ${t.status === "done" ? "is-done" : ""} ${t.priority ? "hi" : ""}" data-id="${t.id}">
    <button class="check" data-act="toggle" title="${t.status === "done" ? "Reopen" : "Mark done"}">${t.status === "done" ? "✓" : ""}</button>
    ${main}
    <div class="todo-actions">
      ${t.status !== "done" ? `<button data-act="start" title="Start a timer for this">${ic.play}</button>` : ""}
      <button data-act="del" title="Delete">${ic.x}</button>
    </div>
  </div>`;
}

const overdue = (d) => d < todayISO();
const fmtDue = (d) =>
  new Date(d + "T00:00:00").toLocaleDateString([], { month: "short", day: "numeric" });

async function submitTodo(e) {
  e.preventDefault();
  const f = e.target;
  try {
    await POST("/api/v1/todos", {
      title: f.title.value.trim(),
      client_id: intOrNull(f.client.value),
      due_date: f.due.value || null,
      priority: f.priority.checked ? 1 : 0,
    });
    f.reset();
    await refreshRail();
    toast("Added to your list");
  } catch (err) {
    toast(err.message, true);
  }
}

async function onTodoClick(e) {
  const btn = e.target.closest("button[data-act]");
  if (!btn) return;
  const id = Number(btn.closest(".todo").dataset.id);
  const act = btn.dataset.act;
  try {
    if (act === "toggle") {
      const t = state.todos.find((x) => x.id === id);
      await POST(`/api/v1/todos/${id}/done`, { done: t.status !== "done" });
      await refreshRail();
    } else if (act === "del") {
      await DEL("/api/v1/todos/" + id);
      await refreshRail();
    } else if (act === "start") {
      const switched = !!state.timer;
      await POST(`/api/v1/todos/${id}/start`);
      await refreshToday();
      scrollToTimer(false);
      toast(switched ? "Previous timer logged; new timer started" : "Timer started from to-do");
    }
  } catch (err) {
    toast(err.message, true);
  }
}

export function focusTodoForm() {
  $("#today").scrollIntoView({ behavior: "smooth", block: "start" });
  $("#todo-form input[name=title]").focus({ preventScroll: true });
}

export function initTodos() {
  onRefsChanged(refreshTodoClientOptions);
  $("#todo-form").addEventListener("submit", submitTodo);
  $("#todo-client").innerHTML = clientOptions();
  $("#todo-list").addEventListener("click", onTodoClick);
  $("#todo-done").addEventListener("click", onTodoClick);
}

// refreshed via onRefsChanged whenever the client list changes
function refreshTodoClientOptions() {
  const sel = $("#todo-client");
  const v = sel.value;
  sel.innerHTML = clientOptions();
  sel.value = v;
  if (sel.value !== v) sel.selectedIndex = 0;
}
