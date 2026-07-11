// The library: full-text search over templates + past work, with the
// template editor (and its attachments) in the slide-over panel.

import { $, esc, fmtSize, toast, GET, POST, PUT, DEL, ic } from "../lib/api.js";
import { state } from "../lib/state.js";
import { claimPanel, openPanel, closePanel, panelClaimIsCurrent } from "../lib/panel.js";

let editingTplId = null;
let editingPanelClaim = null;
let loaded = false;

function workTypeOptionsFor(sel) {
  return `<option value="">— work type —</option>` +
    state.workTypes.map((w) =>
      `<option value="${w.id}" ${w.id === sel ? "selected" : ""}>${esc(w.name)}</option>`).join("");
}

export async function loadLibrary() {
  loaded = true;
  const q = $("#lib-search").value.trim();
  if (q) return runLibSearch(q);
  try {
    const { templates } = await GET("/api/v1/templates");
    renderLibList(templates);
  } catch (err) {
    toast(err.message, true);
  }
}

export const libraryLoaded = () => loaded;

function renderLibList(templates) {
  const box = $("#lib-results");
  if (!templates.length) {
    box.innerHTML = `<p class="empty">No templates yet. Save a good entry as a template (⧉ on any entry), or start one fresh.</p>`;
    return;
  }
  box.innerHTML = `<div class="lib-group">Templates</div>` + templates.map((t) => `
    <button class="lib-hit" data-kind="template" data-id="${t.id}">
      <span class="lib-title">${esc(t.title)}</span>
      <span class="lib-meta">${t.work_type_name ? `<span class="badge">${esc(t.work_type_name)}</span>` : ""}${esc(t.tags || "")}</span>
    </button>`).join("");
}

export async function runLibSearch(q) {
  try {
    const { hits } = await GET("/api/v1/search?q=" + encodeURIComponent(q));
    if ($("#lib-search").value.trim() !== q) return; // stale response
    const box = $("#lib-results");
    if (!hits.length) {
      box.innerHTML = `<p class="empty">Nothing matches "${esc(q)}".</p>`;
      return;
    }
    const tpl = hits.filter((h) => h.kind === "template");
    const ent = hits.filter((h) => h.kind === "entry");
    // snippet comes from the server with <b> marks around matches; everything
    // else is escaped server-side data, but escape defensively anyway.
    const safeSnippet = (s) => esc(s).replaceAll("&lt;b&gt;", "<b>").replaceAll("&lt;/b&gt;", "</b>");
    const row = (h) => `
      <div class="lib-hit ${h.kind === "template" ? "clickable" : ""}" data-kind="${h.kind}" data-id="${h.id}">
        <span class="lib-title">${esc(h.title)}${h.date ? `<span class="lib-date">${h.date}</span>` : ""}</span>
        <span class="lib-snippet">${safeSnippet(h.snippet)}</span>
        <span class="lib-meta">
          ${h.client_name ? `<span class="who">${esc(h.client_name)}</span>` : ""}
          ${h.kind === "entry" ? `<button type="button" class="mini" data-act="promote">Save as template</button>` : ""}
        </span>
      </div>`;
    box.innerHTML =
      (tpl.length ? `<div class="lib-group">Templates</div>` + tpl.map(row).join("") : "") +
      (ent.length ? `<div class="lib-group">Past work</div>` + ent.map(row).join("") : "");
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

// ---------- the template editor (in the panel) ----------
export function openTplEditor(t, claim) {
  if (claim === undefined) claim = claimPanel();
  if (!panelClaimIsCurrent(claim)) return;
  editingTplId = t ? t.id : null;
  editingPanelClaim = claim;
  openPanel(t ? "Edit template" : "New template", (body) => {
    body.innerHTML = `
      <form id="tpl-form">
        <label>Title<input name="title" required /></label>
        <div class="tpl-row">
          <label>Work type<select name="work_type"></select></label>
          <label>Tags<input name="tags" placeholder="comma, separated" /></label>
        </div>
        <label>Body<textarea name="body" rows="12" placeholder="The reusable text — letter skeleton, step list, checklist…"></textarea></label>
        <div class="form-actions">
          <button type="button" id="tpl-delete" class="ghost-btn danger" ${t ? "" : "hidden"}>Delete</button>
          <button type="submit">Save</button>
        </div>
      </form>
      <div class="attach" id="tpl-attach" ${t ? "" : "hidden"}>
        <div class="attach-head">
          <span class="section-title">Attachments</span>
          <button type="button" id="attach-btn" class="ghost-btn sm">Attach file</button>
        </div>
        <div id="attach-list" class="attach-list"></div>
      </div>`;
    const f = $("#tpl-form", body);
    f.title.value = t?.title || "";
    f.tags.value = t?.tags || "";
    f.body.value = t?.body || "";
    f.work_type.innerHTML = workTypeOptionsFor(t?.work_type_id);
    f.addEventListener("submit", submitTpl);
    $("#tpl-delete", body).addEventListener("click", deleteTpl);
    $("#attach-btn", body).addEventListener("click", () => $("#attach-file").click());
    $("#attach-list", body).addEventListener("click", onAttachDelete);
    if (t) renderAttachList(t.attachments);
  }, { claim });
}

export async function openTplEditorById(id) {
  const claim = claimPanel();
  try {
    const t = await GET("/api/v1/templates/" + id);
    if (!panelClaimIsCurrent(claim)) return;
    openTplEditor(t, claim);
  } catch (err) {
    if (panelClaimIsCurrent(claim)) toast(err.message, true);
  }
}

async function submitTpl(e) {
  e.preventDefault();
  const f = e.target;
  const claim = editingPanelClaim;
  const templateID = editingTplId;
  const body = {
    title: f.title.value.trim(),
    tags: f.tags.value.trim(),
    body: f.body.value,
    work_type_id: f.work_type.value ? parseInt(f.work_type.value, 10) : null,
  };
  try {
    const t = templateID
      ? await PUT("/api/v1/templates/" + templateID, body)
      : await POST("/api/v1/templates", body);
    toast("Template saved");
    if (panelClaimIsCurrent(claim)) {
      editingTplId = t.id;
      $("#panel-title").textContent = "Edit template";
      $("#tpl-delete").hidden = false;
      $("#tpl-attach").hidden = false;
      renderAttachList(t.attachments);
    }
    if (loaded) await loadLibrary();
  } catch (err) {
    toast(err.message, true);
  }
}

async function deleteTpl() {
  if (!editingTplId || !confirm("Delete this template?")) return;
  const claim = editingPanelClaim;
  const templateID = editingTplId;
  try {
    await DEL("/api/v1/templates/" + templateID);
    if (panelClaimIsCurrent(claim)) {
      editingTplId = null;
      editingPanelClaim = null;
      closePanel();
    }
    if (loaded) await loadLibrary();
    toast("Template deleted");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- attachments (Word / PDF files) ----------
function renderAttachList(atts) {
  const box = $("#attach-list");
  if (!box) return;
  box.innerHTML = (atts || []).length
    ? atts.map((a) => `
      <div class="attach-row" data-id="${a.id}">
        ${ic.file}
        <a class="attach-name" href="/api/v1/attachments/${a.id}" title="Download">${esc(a.filename)}</a>
        <span class="attach-size">${fmtSize(a.size_bytes)}</span>
        <button type="button" data-act="att-del" title="Remove">${ic.x}</button>
      </div>`).join("")
    : `<p class="empty small">No files yet — attach the Word or PDF version of this letter.</p>`;
}

async function refreshAttachments(templateID = editingTplId, claim = editingPanelClaim) {
  if (!templateID) return;
  try {
    const t = await GET("/api/v1/templates/" + templateID);
    if (!panelClaimIsCurrent(claim)) return;
    renderAttachList(t.attachments);
  } catch (err) {
    if (panelClaimIsCurrent(claim)) toast(err.message, true);
  }
}

async function onAttachDelete(e) {
  const btn = e.target.closest('button[data-act="att-del"]');
  if (!btn) return;
  const claim = editingPanelClaim;
  const templateID = editingTplId;
  try {
    await DEL("/api/v1/attachments/" + btn.closest(".attach-row").dataset.id);
    await refreshAttachments(templateID, claim);
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- wiring ----------
async function onLibClick(e) {
  const promote = e.target.closest('button[data-act="promote"]');
  if (promote) {
    const id = promote.closest(".lib-hit").dataset.id;
    const claim = claimPanel();
    try {
      const t = await POST(`/api/v1/entries/${id}/template`);
      toast("Saved to library");
      if (!panelClaimIsCurrent(claim)) return;
      openTplEditor(t, claim);
    } catch (err) {
      toast(err.message, true);
    }
    return;
  }
  const hit = e.target.closest('.lib-hit[data-kind="template"]');
  if (hit) openTplEditorById(hit.dataset.id);
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
  $("#lib-results").addEventListener("click", onLibClick);
  $("#tpl-new").addEventListener("click", () => openTplEditor(null));
  $("#attach-file").addEventListener("change", async (e) => {
    const file = e.target.files[0];
    e.target.value = "";
    if (!file || !editingTplId) return;
    const claim = editingPanelClaim;
    const templateID = editingTplId;
    const fd = new FormData();
    fd.append("file", file);
    try {
      const res = await fetch(`/api/v1/templates/${templateID}/attachments`, {
        method: "POST",
        body: fd,
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || res.statusText);
      toast("File attached");
      await refreshAttachments(templateID, claim);
    } catch (err) {
      toast(err.message, true);
    }
  });
}
