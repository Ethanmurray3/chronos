// The command palette (⌘K / Ctrl+K): actions, client jump, library search.
// Action handlers are injected from app.js so this module stays free of
// section imports.

import { $, esc, GET, toast } from "./api.js";
import { state, clientLabel } from "./state.js";

let actions = []; // { label, hint, when?, run }
let handlers = {}; // { openClient(id), openTemplate(id), searchLibrary(q) }
let items = []; // currently rendered { label, hint, kind, run }
let sel = 0;
let searchTimer = null;
let searchHits = [];
let searchedFor = "";

export function initPalette(actionList, handlerMap) {
  actions = actionList;
  handlers = handlerMap;
  const dlg = $("#palette");
  const input = $("#palette-input");

  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      dlg.open ? close() : open();
    }
  });
  $("#palette-btn").addEventListener("click", open);

  input.addEventListener("input", () => {
    const q = input.value.trim();
    render(q);
    clearTimeout(searchTimer);
    if (q.length >= 2) {
      searchTimer = setTimeout(() => runSearch(q), 220);
    } else {
      searchHits = [];
      searchedFor = "";
    }
  });

  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      move(1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      move(-1);
    } else if (e.key === "Enter") {
      e.preventDefault();
      const it = items[sel];
      if (it) {
        close();
        it.run();
      }
    }
  });

  $("#palette-list").addEventListener("click", (e) => {
    const li = e.target.closest("li[data-i]");
    if (!li) return;
    const it = items[Number(li.dataset.i)];
    close();
    it.run();
  });

  // dialog closes itself on Escape; nothing to clean up
}

function open() {
  const dlg = $("#palette");
  const input = $("#palette-input");
  input.value = "";
  searchHits = [];
  searchedFor = "";
  dlg.showModal();
  render("");
  input.focus();
}

function close() {
  $("#palette").close();
}

async function runSearch(q) {
  try {
    // templates and past work live behind separate searches now; the
    // palette shows both, templates first.
    const enc = encodeURIComponent(q);
    const [tpl, ent] = await Promise.all([
      GET("/api/v1/templates?q=" + enc),
      GET("/api/v1/search?q=" + enc),
    ]);
    searchHits = [...(tpl.hits || []).slice(0, 4), ...(ent.hits || []).slice(0, 4)];
    searchedFor = q;
    if ($("#palette").open && $("#palette-input").value.trim() === q) render(q);
  } catch {
    /* palette search is best-effort */
  }
}

function match(text, q) {
  return text.toLowerCase().includes(q.toLowerCase());
}

function render(q) {
  items = [];

  for (const a of actions) {
    if (a.when && !a.when()) continue;
    if (q && !match(a.label, q)) continue;
    items.push({ ...a, kind: "action" });
  }

  if (q) {
    for (const c of state.clients.filter((c) => match(clientLabel(c), q)).slice(0, 6)) {
      items.push({
        label: clientLabel(c),
        hint: "client",
        kind: "client",
        run: () => handlers.openClient(c.id),
      });
    }
    if (searchedFor === q) {
      for (const h of searchHits) {
        items.push({
          label: h.title,
          hint: h.kind === "template" ? "template" : `past work${h.date ? " · " + h.date : ""}`,
          kind: h.kind,
          run: () =>
            h.kind === "template" ? handlers.openTemplate(h.id) : handlers.searchLibrary(q),
        });
      }
    }
  }

  sel = 0;
  const list = $("#palette-list");
  if (items.length === 0) {
    list.innerHTML = `<li class="palette-empty">${q ? `Nothing for “${esc(q)}”` : "Type to search"}</li>`;
    return;
  }
  list.innerHTML = items
    .map(
      (it, i) => `
    <li data-i="${i}" class="${i === sel ? "sel" : ""}" data-kind="${it.kind}">
      <span class="pl-label">${esc(it.label)}</span>
      ${it.hint ? `<span class="pl-hint">${esc(it.hint)}</span>` : ""}
    </li>`,
    )
    .join("");
}

function move(dir) {
  if (items.length === 0) return;
  sel = (sel + dir + items.length) % items.length;
  const list = $("#palette-list");
  for (const li of list.children) li.classList.toggle("sel", Number(li.dataset.i) === sel);
  list.children[sel]?.scrollIntoView({ block: "nearest" });
}
