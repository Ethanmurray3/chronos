// Chronos frontend — vanilla ES modules, no build step. Talks to /api/v1.
// This module only boots and wires; the work lives in lib/ and sections/.

import { $, toast, todayISO } from "./lib/api.js";
import { state, reloadRefs } from "./lib/state.js";
import { initPanel } from "./lib/panel.js";
import { initPalette } from "./lib/palette.js";
import { initToday, refreshToday, tickTimer, scrollToTimer, stopTimer } from "./sections/today.js";
import { initTodos, focusTodoForm } from "./sections/todos.js";
import { initClients, openClients, openClientPanel } from "./sections/clients.js";
import { initTemplates, loadTemplates, templatesLoaded, openTplEditor, openTplEditorById } from "./sections/templates.js";
import { initReports, openReports } from "./sections/reports.js";
import { initSettings, openSettings } from "./sections/settings.js";

// ---------- theme ----------
const THEME_KEY = "chronos-theme";
const THEME_LABEL = { light: "Light", dark: "Dark", system: "System" };

function applyTheme(mode) {
  const root = document.documentElement;
  if (mode === "light" || mode === "dark") {
    root.dataset.theme = mode;
  } else {
    delete root.dataset.theme;
  }
  $("#theme-btn").title = "Theme: " + (THEME_LABEL[mode] || "System");
}

function wireTheme() {
  applyTheme(localStorage.getItem(THEME_KEY) || "system");
  $("#theme-btn").addEventListener("click", () => {
    const cur = localStorage.getItem(THEME_KEY) || "system";
    const next = cur === "system" ? "light" : cur === "light" ? "dark" : "system";
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
    toast("Theme: " + THEME_LABEL[next]);
  });
}

// ---------- the two pages: Today | Templates ----------
export function switchView(id) {
  if (!document.getElementById(id)) id = "today";
  for (const sec of document.querySelectorAll("main .view"))
    sec.classList.toggle("active", sec.id === id);
  for (const a of document.querySelectorAll("#topnav a"))
    a.classList.toggle("active", a.getAttribute("href") === "#" + id);
  if (id === "templates" && !templatesLoaded()) loadTemplates();
  if (location.hash !== "#" + id) history.replaceState(null, "", "#" + id);
  window.scrollTo({ top: 0 });
}

function wireTopbar() {
  document.querySelector(".topbar").addEventListener("click", (e) => {
    const a = e.target.closest('a[href^="#"]');
    if (!a) return;
    e.preventDefault();
    switchView(a.getAttribute("href").slice(1));
  });
  window.addEventListener("hashchange", () => switchView(location.hash.slice(1)));
  // sections (scrollToTimer) ask for a view change via this event so they
  // don't have to import app.js back
  document.addEventListener("switch-view", (e) => switchView(e.detail));
  // generic dialog close buttons
  document.addEventListener("click", (e) => {
    const btn = e.target.closest("button[data-close]");
    if (btn) $("#" + btn.dataset.close).close();
  });
  $("#bar-timer").addEventListener("click", () => scrollToTimer(false));
  switchView(location.hash.slice(1) || "today");
}

// ---------- command palette actions ----------
const paletteActions = [
  { label: "Start a timer", hint: "Today", when: () => !state.timer, run: () => scrollToTimer(true) },
  { label: "Stop the timer", hint: "Today", when: () => !!state.timer, run: stopTimer },
  { label: "Add a to-do", hint: "Today", run: focusTodoForm },
  {
    label: "Add a manual entry",
    hint: "Today",
    run: () => {
      $("#manual").open = true;
      scrollToTimer(false);
      $("#manual-form input[name=description]")?.focus({ preventScroll: true });
    },
  },
  { label: "New template", hint: "Templates", run: () => openTplEditor(null) },
  { label: "Go to Today", run: () => switchView("today") },
  { label: "Go to Templates", run: () => switchView("templates") },
  { label: "Open Clients", run: openClients },
  { label: "Open Reports", run: openReports },
  { label: "Settings", run: openSettings },
];

// ---------- boot ----------
window.__CHRONOS_UI = "r2"; // build marker: confirms which app.js the page runs

async function boot() {
  wireTheme();
  initPanel();
  initPalette(paletteActions, {
    openClient: openClientPanel,
    openTemplate: openTplEditorById,
  });
  initToday();
  initTodos();
  initClients();
  initTemplates();
  initReports();
  initSettings();
  wireTopbar();

  try {
    await reloadRefs(); // renders every ref consumer via onRefsChanged
    await refreshToday();
  } catch (e) {
    toast("Load failed: " + e.message, true);
  }

  setInterval(tickTimer, 1000);
  setInterval(() => {
    // keep the workspace fresh; also rolls the view over at midnight
    if (state.viewDate === todayISO() || state.timer) refreshToday().catch(() => {});
  }, 30000);
}

boot();
