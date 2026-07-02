import { apiPath } from "./utils.js";
import { initSortable } from "./sortable.js";
import { syncSortButtonState, syncFilterToolbarState } from "./events.js";
import { showToast } from "./notifications.js";

export function initUndoDelete() {
  document.body.addEventListener("task-deleted", (e) => {
    const detail = (e && e.detail) || {};
    const count = detail.count || 1;
    const label = count === 1 ? "1 task deleted" : `${count} tasks deleted`;

    showToast(label, {
      duration: 5000,
      actionLabel: "Undo",
      onAction: () => {
        const form = new URLSearchParams();
        form.append("page", getCurrentPage());
        appendUndoFilterFields(form);

        fetch(apiPath("/api/undo-delete"), {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
            "HX-Request": "true",
          },
          body: form.toString(),
        })
          .then((res) => {
            if (!res.ok) {
              return res.text().then((t) => {
                throw new Error(t || "Undo failed");
              });
            }
            return res.text();
          })
          .then((html) => {
            const container = document.getElementById("task-container");
            if (container) {
              container.innerHTML = html;
              document.body.dispatchEvent(new CustomEvent("bulk-list-updated"));
            }
            try {
              initSortable();
              syncSortButtonState();
              syncFilterToolbarState();
            } catch (err) {}
            showToast("Delete undone.");
          })
          .catch((err) => {
            showToast(err.message || "Undo failed.", { error: true });
          });
      },
    });
  });

  document.body.addEventListener("htmx:afterRequest", (evt) => {
    const xhr = evt.detail && evt.detail.xhr;
    if (!xhr) return;
    const trigger = xhr.getResponseHeader("HX-Trigger");
    if (!trigger || trigger.indexOf("task-deleted") === -1) return;
    try {
      const parsed = JSON.parse(trigger);
      if (parsed["task-deleted"]) {
        document.body.dispatchEvent(
          new CustomEvent("task-deleted", { detail: parsed["task-deleted"] }),
        );
      }
    } catch (e) {
      document.body.dispatchEvent(
        new CustomEvent("task-deleted", { detail: { count: 1 } }),
      );
    }
  });
}

function getCurrentPage() {
  const pageInput = document.getElementById("current-page");
  if (pageInput && pageInput.value) {
    const page = parseInt(pageInput.value, 10);
    if (!Number.isNaN(page) && page > 0) return String(page);
  }
  return "1";
}

function appendUndoFilterFields(form) {
  const append = (name, hiddenId, toolbarId) => {
    let val = "";
    if (toolbarId) {
      const toolbar = document.getElementById(toolbarId);
      if (toolbar) val = toolbar.value;
    }
    if (!val && hiddenId) {
      const hidden = document.getElementById(hiddenId);
      if (hidden) val = hidden.value;
    }
    if (val) form.append(name, val);
  };
  append("project", "project-filter-value", "project-filter");
  append("status", "status-filter", "status-filter-select");
  append("due", "due-filter", null);
  append("sort", "sort-filter", null);
  append("priority", "priority-filter", "priority-filter-toolbar");
  append("tag", "tag-filter", "tag-filter-toolbar");
  const search = document.getElementById("search");
  if (search && search.value) form.append("search", search.value);
}
