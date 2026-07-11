// The Templates page: the standalone deliverable library. Finished letters,
// worksheets, and step lists saved with notes about what they contain,
// grouped by category — decoupled from time coding. The editor (and its
// attachments) lives in the slide-over panel.

import { $, esc, fmtSize, toast, GET, POST, PUT, DEL, ic } from "../lib/api.js";
import { state, clientOptions } from "../lib/state.js";
import { claimPanel, openPanel, closePanel, panelClaimIsCurrent } from "../lib/panel.js";

let editingTplId = null;
let editingPanelClaim = null;
let loaded = false;
let categories = []; // distinct categories from the last full list, for the editor datalist

export async function loadTemplates() {
  loaded = true;
  const q = $("#tpl-search").value.trim();
  if (q) return runTplSearch(q);
  try {
    const { templates } = await GET("/api/v1/templates");
    categories = [...new Set(templates.map((t) => t.category).filter(Boolean))];
    renderTplList(templates);
  } catch (err) {
    toast(err.message, true);
  }
}

export const templatesLoaded = () => loaded;

function tplCard(t) {
  const files = t.attachment_count
    ? `<span class="tpl-files">${ic.file}${t.attachment_count}</span>`
    : "";
  const tags = (t.tags || "")
    .split(",").map((s) => s.trim()).filter(Boolean)
    .map((s) => `<span class="chip">${esc(s)}</span>`).join("");
  return `
    <button class="tpl-card" data-id="${t.id}">
      <span class="tpl-title">${esc(t.title)}${files}</span>
      ${t.notes ? `<span class="tpl-notes">${esc(t.notes)}</span>` : ""}
      <span class="tpl-meta">
        ${t.client_name ? `<span class="who">${esc(t.client_name)}</span>` : ""}
        ${tags}
      </span>
    </button>`;
}

function renderTplList(templates) {
  const box = $("#tpl-results");
  if (!templates.length) {
    box.innerHTML = `<p class="empty">Nothing saved yet. When you finish a letter or worksheet worth reusing, add it here with notes about what it contains.</p>`;
    return;
  }
  // The API returns templates ordered by category (uncategorized last);
  // group headers fall out of a single pass.
  let html = "", current = null;
  for (const t of templates) {
    const cat = t.category || "Uncategorized";
    if (cat !== current) {
      current = cat;
      html += `<div class="lib-group">${esc(cat)}</div>`;
    }
    html += tplCard(t);
  }
  box.innerHTML = html;
}

async function runTplSearch(q) {
  try {
    const { hits } = await GET("/api/v1/templates?q=" + encodeURIComponent(q));
    if ($("#tpl-search").value.trim() !== q) return; // stale response
    const box = $("#tpl-results");
    if (!hits.length) {
      box.innerHTML = `<p class="empty">Nothing matches "${esc(q)}".</p>`;
      return;
    }
    // snippet comes from the server with <b> marks around matches; everything
    // else is escaped server-side data, but escape defensively anyway.
    const safeSnippet = (s) => esc(s).replaceAll("&lt;b&gt;", "<b>").replaceAll("&lt;/b&gt;", "</b>");
    box.innerHTML = hits.map((h) => `
      <button class="tpl-card" data-id="${h.id}">
        <span class="tpl-title">${esc(h.title)}</span>
        <span class="tpl-notes">${safeSnippet(h.snippet)}</span>
        <span class="tpl-meta">
          ${h.category ? `<span class="chip">${esc(h.category)}</span>` : ""}
          ${h.client_name ? `<span class="who">${esc(h.client_name)}</span>` : ""}
        </span>
      </button>`).join("");
  } catch (err) {
    toast(err.message, true);
  }
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
        <label>Title<input name="title" required placeholder="Reorg step letter — holdco freeze" /></label>
        <div class="tpl-row">
          <label>Category
            <input name="category" list="tpl-cats" placeholder="Reorg letters" />
            <datalist id="tpl-cats">${categories.map((c) => `<option value="${esc(c)}">`).join("")}</datalist>
          </label>
          <label>Originally for<select name="client"></select></label>
        </div>
        <label>Tags<input name="tags" placeholder="comma, separated" /></label>
        <label>Notes<textarea name="notes" rows="8" placeholder="What this contains, which steps it covers, when to reuse it…"></textarea></label>
        <div class="form-actions">
          <button type="button" id="tpl-delete" class="ghost-btn danger" ${t ? "" : "hidden"}>Delete</button>
          <button type="submit">Save</button>
        </div>
      </form>
      <div class="attach" id="tpl-attach">
        <div class="attach-head">
          <span class="section-title">Files</span>
          <button type="button" id="attach-btn" class="ghost-btn sm" ${t ? "" : "disabled title='Save the template first'"}>Attach file</button>
        </div>
        <div id="attach-list" class="attach-list"></div>
      </div>`;
    const f = $("#tpl-form", body);
    f.title.value = t?.title || "";
    f.category.value = t?.category || "";
    f.tags.value = t?.tags || "";
    f.notes.value = t?.notes || "";
    f.client.innerHTML = clientOptions(t?.client_id);
    f.addEventListener("submit", submitTpl);
    $("#tpl-delete", body).addEventListener("click", deleteTpl);
    $("#attach-btn", body).addEventListener("click", () => $("#attach-file").click());
    $("#attach-list", body).addEventListener("click", onAttachDelete);
    renderAttachList(t ? t.attachments : null, !t);
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
    category: f.category.value.trim(),
    tags: f.tags.value.trim(),
    notes: f.notes.value,
    client_id: f.client.value ? parseInt(f.client.value, 10) : null,
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
      const btn = $("#attach-btn");
      btn.disabled = false;
      btn.removeAttribute("title");
      renderAttachList(t.attachments);
    }
    if (loaded) await loadTemplates();
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
    if (loaded) await loadTemplates();
    toast("Template deleted");
  } catch (err) {
    toast(err.message, true);
  }
}

// ---------- attachments (Word / PDF files) ----------
function renderAttachList(atts, unsaved = false) {
  const box = $("#attach-list");
  if (!box) return;
  if (unsaved) {
    box.innerHTML = `<p class="empty small">Save the template, then attach the Word or PDF deliverable — the file is the point.</p>`;
    return;
  }
  box.innerHTML = (atts || []).length
    ? atts.map((a) => `
      <div class="attach-row" data-id="${a.id}">
        ${ic.file}
        <a class="attach-name" href="/api/v1/attachments/${a.id}" title="Download">${esc(a.filename)}</a>
        <span class="attach-size">${fmtSize(a.size_bytes)}</span>
        <button type="button" data-act="att-del" title="Remove">${ic.x}</button>
      </div>`).join("")
    : `<p class="empty small">No files yet — attach the Word or PDF version of this deliverable.</p>`;
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
export function initTemplates() {
  let t;
  $("#tpl-search").addEventListener("input", (e) => {
    clearTimeout(t);
    t = setTimeout(() => {
      const q = e.target.value.trim();
      q ? runTplSearch(q) : loadTemplates();
    }, 250);
  });
  $("#tpl-results").addEventListener("click", (e) => {
    const card = e.target.closest(".tpl-card");
    if (card) openTplEditorById(card.dataset.id);
  });
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
