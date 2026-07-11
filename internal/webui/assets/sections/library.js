// The library: full-text search over everything you've ever coded — past
// time-entry descriptions. Templates moved to their own section.

import { $, esc, toast, GET } from "../lib/api.js";

let loaded = false;

export async function loadLibrary() {
  loaded = true;
  const q = $("#lib-search").value.trim();
  if (q) return runLibSearch(q);
  $("#lib-results").innerHTML =
    `<p class="empty">Type to search every entry you've ever coded — client work, CRA calls, that thing from two Marches ago.</p>`;
}

export const libraryLoaded = () => loaded;

export async function runLibSearch(q) {
  try {
    const { hits } = await GET("/api/v1/search?q=" + encodeURIComponent(q));
    if ($("#lib-search").value.trim() !== q) return; // stale response
    const box = $("#lib-results");
    if (!hits.length) {
      box.innerHTML = `<p class="empty">Nothing matches "${esc(q)}".</p>`;
      return;
    }
    // snippet comes from the server with <b> marks around matches; everything
    // else is escaped server-side data, but escape defensively anyway.
    const safeSnippet = (s) => esc(s).replaceAll("&lt;b&gt;", "<b>").replaceAll("&lt;/b&gt;", "</b>");
    box.innerHTML = `<div class="lib-group">Past work</div>` + hits.map((h) => `
      <div class="lib-hit" data-kind="entry" data-id="${h.id}">
        <span class="lib-title">${esc(h.title)}${h.date ? `<span class="lib-date">${h.date}</span>` : ""}</span>
        <span class="lib-snippet">${safeSnippet(h.snippet)}</span>
        <span class="lib-meta">
          ${h.client_name ? `<span class="who">${esc(h.client_name)}</span>` : ""}
        </span>
      </div>`).join("");
  } catch (err) {
    toast(err.message, true);
  }
}

// jump to the library section with a query pre-filled (used by the palette)
export function searchLibraryFor(q) {
  $("#library").scrollIntoView({ behavior: "smooth", block: "start" });
  const input = $("#lib-search");
  input.value = q;
  loaded = true;
  runLibSearch(q);
}

export function initLibrary() {
  let t;
  $("#lib-search").addEventListener("input", (e) => {
    clearTimeout(t);
    t = setTimeout(() => {
      const q = e.target.value.trim();
      q ? runLibSearch(q) : loadLibrary();
    }, 250);
  });
}
