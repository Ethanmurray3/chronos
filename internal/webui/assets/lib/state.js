// Shared app state plus the reference data (settings, clients, work types)
// that several sections render from.

import { GET, esc, todayISO } from "./api.js";

export const state = {
  settings: null,
  clients: [],
  workTypes: [],
  timer: null, // the running entry, or null
  day: null, // /reports/day payload for viewDate
  todos: [],
  attention: [], // /attention items, severity-sorted
  viewDate: todayISO(), // the day the Today workspace is showing
};

// Sections that render from the reference data (dropdowns, the client
// list) register here; reloadRefs re-renders them all so no consumer
// goes stale after a catalog change.
const refListeners = [];
export function onRefsChanged(cb) {
  refListeners.push(cb);
}

export async function reloadRefs() {
  const [s, c, w] = await Promise.all([
    GET("/api/v1/settings"),
    GET("/api/v1/clients"),
    GET("/api/v1/work-types"),
  ]);
  state.settings = s;
  state.clients = c.clients;
  state.workTypes = w.work_types;
  for (const cb of refListeners) cb();
}

// clientLabel shows the firm's client number alongside the name everywhere.
export const clientLabel = (c) => (c.code ? c.code + " · " : "") + c.name;

export function clientOptions(sel) {
  return `<option value="">— client —</option>` +
    state.clients.map((c) =>
      `<option value="${c.id}" ${c.id === sel ? "selected" : ""}>${esc(clientLabel(c))}</option>`).join("");
}

export function workTypeOptions(sel) {
  return `<option value="">— work type —</option>` +
    state.workTypes.map((w) =>
      `<option value="${w.id}" ${w.id === sel ? "selected" : ""}>${esc(w.name)}</option>`).join("");
}
