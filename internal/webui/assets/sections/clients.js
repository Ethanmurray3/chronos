// The clients section: compact list + add/import, with the detail view in
// the slide-over panel.

import { $, esc, hours, toast, GET, POST } from "../lib/api.js";
import { state, reloadRefs, clientLabel, onRefsChanged } from "../lib/state.js";
import { claimPanel, openPanel, panelClaimIsCurrent } from "../lib/panel.js";

export function renderClients() {
  const list = $("#client-list");
  if (state.clients.length === 0) {
    list.innerHTML = `<p class="empty">No clients yet. Add one to start attributing time.</p>`;
    return;
  }
  list.innerHTML = state.clients.map((c) => `
    <button class="client-row" data-id="${c.id}">
      <span class="client-name">${esc(c.name)}</span>
      <span class="code">${esc(c.code || "")}</span>
    </button>`).join("");
}

export async function openClientPanel(id) {
  const claim = claimPanel();
  const c = state.clients.find((x) => x.id === id);
  openPanel(c ? clientLabel(c) : "Client", (body) => {
    body.innerHTML = `<p class="empty small">Loading…</p>`;
  }, { claim });
  let entries;
  try {
    ({ entries } = await GET("/api/v1/time-entries?client_id=" + id));
  } catch (err) {
    if (panelClaimIsCurrent(claim)) toast(err.message, true);
    return;
  }
  if (!panelClaimIsCurrent(claim)) return;
  const body = $("#panel-body");
  const done = entries.filter((e) => !e.running);
  if (done.length === 0) {
    body.innerHTML = `<p class="empty">No time recorded for this client yet.</p>`;
    return;
  }
  const total = done.reduce((s, e) => s + e.effective_min, 0);
  body.innerHTML =
    `<p class="panel-lede">Total tracked <span class="tot">${hours(total)}</span> across ${done.length} entries</p>` +
    `<div class="entries panel-entries">` +
    done.slice(0, 50).map((e) => `
      <div class="entry">
        <div class="when">${new Date(e.started_at * 1000).toLocaleDateString([], { month: "short", day: "numeric" })}</div>
        <div class="desc"><span class="what">${e.work_type_name ? `<span class="badge">${esc(e.work_type_name)}</span>` : ""}${esc(e.description || "")}</span></div>
        <div class="dur">${hours(e.effective_min)}</div>
      </div>`).join("") + `</div>`;
}

async function submitClient(e) {
  e.preventDefault();
  const f = e.target;
  try {
    await POST("/api/v1/clients", { name: f.name.value.trim(), code: f.code.value.trim() });
    f.reset();
    await reloadRefs(); // re-renders every ref consumer, this list included
    toast("Client added");
  } catch (err) {
    toast(err.message, true);
  }
}

export function initClients() {
  onRefsChanged(renderClients);
  $("#client-form").addEventListener("submit", submitClient);
  $("#client-list").addEventListener("click", (e) => {
    const row = e.target.closest(".client-row");
    if (row) openClientPanel(parseInt(row.dataset.id, 10));
  });
}
