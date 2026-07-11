// The slide-over panel: one reusable right-hand surface for client detail
// and the template editor. Focus-trapped, Escape/scrim closes, body scroll
// locked while open.

import { $ } from "./api.js";

let lastFocus = null;
let onCloseCb = null;
let activeClaim = 0;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// Async panel users claim the shared surface before loading. Any later claim,
// open, or close supersedes that work so a stale response cannot replace the
// panel the user most recently requested.
export function claimPanel() {
  return ++activeClaim;
}

export const panelClaimIsCurrent = (claim) => claim === activeClaim;

export function openPanel(title, render, { onClose, claim } = {}) {
  if (claim === undefined) claim = claimPanel();
  if (!panelClaimIsCurrent(claim)) return null;
  const panel = $("#panel");
  const scrim = $("#scrim");
  $("#panel-title").textContent = title;
  const body = $("#panel-body");
  body.replaceChildren();
  render(body);
  onCloseCb = onClose ?? null;
  lastFocus = document.activeElement;
  document.body.classList.add("locked");
  scrim.classList.add("open");
  panel.classList.add("open");
  const target = body.querySelector(FOCUSABLE) || $("#panel-close");
  // let the slide transition start before moving focus
  requestAnimationFrame(() => target.focus({ preventScroll: true }));
  return claim;
}

export function closePanel() {
  claimPanel(); // invalidate requests that were loading content for this panel
  const panel = $("#panel");
  if (!panel.classList.contains("open")) return;
  panel.classList.remove("open");
  $("#scrim").classList.remove("open");
  document.body.classList.remove("locked");
  if (onCloseCb) onCloseCb();
  onCloseCb = null;
  if (lastFocus && document.contains(lastFocus)) lastFocus.focus({ preventScroll: true });
  lastFocus = null;
}

export const panelIsOpen = () => $("#panel").classList.contains("open");

export function initPanel() {
  $("#panel-close").addEventListener("click", closePanel);
  $("#scrim").addEventListener("click", closePanel);
  document.addEventListener("keydown", (e) => {
    if (!panelIsOpen()) return;
    if (e.key === "Escape") {
      e.preventDefault();
      closePanel();
      return;
    }
    if (e.key !== "Tab") return;
    // keep Tab cycling inside the panel
    const items = [...$("#panel").querySelectorAll(FOCUSABLE)].filter(
      (el) => el.offsetParent !== null);
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    } else if (!$("#panel").contains(document.activeElement)) {
      e.preventDefault();
      first.focus();
    }
  });
}
