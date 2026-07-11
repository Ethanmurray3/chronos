// The settings dialog: seasonal targets, rounding, thresholds, timezone,
// plus the work-type manager and CSV imports.

import { $, esc, toast, POST, PUT, DEL } from "../lib/api.js";
import { state, reloadRefs } from "../lib/state.js";
import { refreshToday } from "./today.js";

export function openSettings() {
  const s = state.settings;
  const f = $("#settings-form");
  f.busy.value = (s.busy_season_target_min / 60).toFixed(1);
  f.off.value = (s.off_season_target_min / 60).toFixed(1);
  f.rounding.value = s.rounding_min;
  f.duesoon.value = s.due_soon_days;
  f.staledays.value = s.stale_days;
  f.timezone.value = s.timezone || "";
  renderWtManager();
  $("#settings-dialog").showModal();
}

function wireSettingsForm() {
  $("#settings-btn").addEventListener("click", openSettings);
  $("#settings-form").addEventListener("submit", async (e) => {
    if (e.submitter && e.submitter.value !== "save") return; // cancel
    e.preventDefault();
    const f = e.target;
    try {
      state.settings = await PUT("/api/v1/settings", {
        busy_season_target_min: Math.round(parseFloat(f.busy.value) * 60) || 0,
        off_season_target_min: Math.round(parseFloat(f.off.value) * 60) || 0,
        rounding_min: parseInt(f.rounding.value, 10) || 0,
        due_soon_days: parseInt(f.duesoon.value, 10) || 0,
        stale_days: parseInt(f.staledays.value, 10) || 0,
        timezone: f.timezone.value.trim(),
      });
      $("#settings-dialog").close();
      await refreshToday();
      toast("Settings saved");
    } catch (err) {
      toast(err.message, true);
    }
  });
}

// ---------- work-type manager ----------
function renderWtManager() {
  $("#wt-list").innerHTML = state.workTypes.map((w) => `
    <div class="wt-row" data-id="${w.id}">
      <input class="wt-name" value="${esc(w.name)}" aria-label="Work type name" />
      <input class="wt-cat short" value="${esc(w.category)}" aria-label="Category" />
      <label class="chk"><input type="checkbox" class="wt-bill" ${w.billable_default ? "checked" : ""} /> Billable</label>
      <button type="button" data-act="wt-del" title="Delete">✕</button>
    </div>`).join("");
}

async function saveWtRow(row) {
  const id = row.dataset.id;
  try {
    await PUT("/api/v1/work-types/" + id, {
      name: row.querySelector(".wt-name").value.trim(),
      category: row.querySelector(".wt-cat").value.trim(),
      billable_default: row.querySelector(".wt-bill").checked,
    });
    await reloadRefs();
  } catch (err) {
    toast(err.message, true);
  }
}

function wireWtManager() {
  $("#wt-list").addEventListener("change", (e) => {
    const row = e.target.closest(".wt-row");
    if (row) saveWtRow(row);
  });
  $("#wt-list").addEventListener("click", async (e) => {
    const btn = e.target.closest('button[data-act="wt-del"]');
    if (!btn) return;
    const row = btn.closest(".wt-row");
    try {
      await DEL("/api/v1/work-types/" + row.dataset.id);
      await reloadRefs();
      renderWtManager();
    } catch (err) {
      toast(err.message, true); // e.g. 409: in use by entries
    }
  });
  $("#wt-add-btn").addEventListener("click", async () => {
    const name = $("#wt-new-name").value.trim();
    if (!name) return toast("Enter a name", true);
    try {
      await POST("/api/v1/work-types", {
        name,
        category: $("#wt-new-cat").value.trim(),
        billable_default: $("#wt-new-bill").checked,
      });
      $("#wt-new-name").value = "";
      $("#wt-new-cat").value = "";
      await reloadRefs();
      renderWtManager();
    } catch (err) {
      toast(err.message, true);
    }
  });
}

// ---------- CSV imports (clients + work types) ----------
function wireImport(btnSel, fileSel, url, after) {
  $(btnSel).addEventListener("click", () => $(fileSel).click());
  $(fileSel).addEventListener("change", async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    e.target.value = "";
    try {
      const text = await file.text();
      const res = await fetch(url, { method: "POST", headers: { "Content-Type": "text/csv" }, body: text });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || res.statusText);
      const probs = (data.problems || []).length;
      toast(`Imported: ${data.created} new, ${data.updated} updated${probs ? `, ${probs} problems` : ""}`, probs > 0);
      await reloadRefs();
      after?.();
    } catch (err) {
      toast(err.message, true);
    }
  });
}

export function initSettings() {
  wireSettingsForm();
  wireWtManager();
  // reloadRefs re-renders the dropdowns and client list via onRefsChanged;
  // only the dialog-local work-type manager needs an explicit refresh
  wireImport("#wt-import-btn", "#wt-file", "/api/v1/work-types/import", renderWtManager);
  wireImport("#client-import-btn", "#client-file", "/api/v1/clients/import", null);
}
