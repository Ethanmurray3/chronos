// Chronos frontend — vanilla ES modules, no build step. Talks to /api/v1.
// This module only boots and wires; the work lives in lib/ and sections/.

import { $, toast, todayISO } from "./lib/api.js";
import { state, reloadRefs } from "./lib/state.js";
import { initPanel } from "./lib/panel.js";
import { initPalette } from "./lib/palette.js";
import { initToday, refreshToday, tickTimer, scrollToTimer, stopTimer } from "./sections/today.js";
import { initTodos, focusTodoForm } from "./sections/todos.js";
import { initClients, openClientPanel } from "./sections/clients.js";
import { initLibrary, loadLibrary, libraryLoaded, searchLibraryFor } from "./sections/library.js";
import { initTemplates, loadTemplates, templatesLoaded, openTplEditor, openTplEditorById } from "./sections/templates.js";
import { initReports, autoRunReports, exportURL } from "./sections/reports.js";
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

// ---------- topbar: scroll-spy + lazy section init ----------
let spyObs, firstViewObs; // module refs so the observers can never be collected

function wireTopbar() {
  const links = [...document.querySelectorAll("#topnav a")];
  const byId = new Map(links.map((a) => [a.getAttribute("href").slice(1), a]));
  const sections = [...document.querySelectorAll("main .section")];

  // idempotent first-view handling: entrance animation + lazy data loads
  const markFirstView = (sec) => {
    if (sec.classList.contains("in-view")) return;
    sec.classList.add("in-view");
    if (sec.id === "templates" && !templatesLoaded()) loadTemplates();
    if (sec.id === "library" && !libraryLoaded()) loadLibrary();
    if (sec.id === "reports") autoRunReports();
  };

  // Observers only report on rendered frames, and ones created while the
  // document is still loading can miss entirely — attach after load and
  // seed the initial state from geometry so nothing depends on a frame.
  const attach = () => {
    // which section is "current" — a band around the upper third of the viewport
    spyObs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          links.forEach((a) => a.classList.remove("active"));
          byId.get(e.target.id)?.classList.add("active");
        }
      },
      { rootMargin: "-20% 0px -65% 0px" },
    );
    firstViewObs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          markFirstView(e.target);
          firstViewObs.unobserve(e.target);
        }
      },
      { threshold: 0.15 },
    );
    for (const sec of sections) {
      spyObs.observe(sec);
      firstViewObs.observe(sec);
      const r = sec.getBoundingClientRect();
      if (r.top < window.innerHeight && r.bottom > 0) markFirstView(sec);
    }
  };
  if (document.readyState === "complete") attach();
  else window.addEventListener("load", attach, { once: true });

  $("#bar-timer").addEventListener("click", () => scrollToTimer(false));
}

// ---------- command palette actions ----------
const scrollTo = (id) => () =>
  $("#" + id).scrollIntoView({ behavior: "smooth", block: "start" });

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
  { label: "Go to Clients", run: scrollTo("clients") },
  { label: "Go to Templates", run: scrollTo("templates") },
  { label: "Go to Library", run: scrollTo("library") },
  { label: "Go to Reports", run: scrollTo("reports") },
  { label: "Settings", run: openSettings },
  { label: "Export CSV", hint: "current range", run: () => window.open(exportURL(), "_blank") },
];

// ---------- boot ----------
window.__CHRONOS_UI = "r2"; // build marker: confirms which app.js the page runs

async function boot() {
  wireTheme();
  initPanel();
  initPalette(paletteActions, {
    openClient: openClientPanel,
    openTemplate: openTplEditorById,
    searchLibrary: searchLibraryFor,
  });
  initToday();
  initTodos();
  initClients();
  initTemplates();
  initLibrary();
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
