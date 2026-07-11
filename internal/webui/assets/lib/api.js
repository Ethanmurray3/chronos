// Low-level helpers shared by every module: DOM shorthand, escaping,
// fetch wrappers for /api/v1, formatting, icons, and the toast.

export const $ = (sel, root = document) => root.querySelector(sel);

export const esc = (s) =>
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
export const GET = (p) => api("GET", p);
export const POST = (p, b) => api("POST", p, b);
export const PUT = (p, b) => api("PUT", p, b);
export const DEL = (p) => api("DELETE", p);

export const hours = (min) => (min / 60).toFixed(1) + "h";
export const clock = (unix) =>
  new Date(unix * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
export const fmtISO = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
export const todayISO = () => fmtISO(new Date());
// shiftISO moves an ISO date by n days; noon anchor dodges DST edges.
export const shiftISO = (iso, days) => {
  const d = new Date(iso + "T12:00:00");
  d.setDate(d.getDate() + days);
  return fmtISO(d);
};
export const intOrNull = (v) => (v ? parseInt(v, 10) : null);
export const fmtSize = (b) =>
  b >= 1048576 ? (b / 1048576).toFixed(1) + " MB" : Math.max(1, Math.round(b / 1024)) + " KB";

// Inline icons (stroke inherits currentColor) for row actions.
export const ic = {
  x: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  play: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M7.5 5.4v13.2a.6.6 0 0 0 .92.5l10.3-6.6a.6.6 0 0 0 0-1L8.42 4.9a.6.6 0 0 0-.92.5z"/></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  file: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M6 2h8l6 6v14H6z"/><path d="M13 2v7h7"/></svg>',
};

export function toast(msg, isErr = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (isErr ? " err" : "");
  t.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => (t.hidden = true), 2600);
}
