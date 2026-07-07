import { openSidebar } from "./sidebar.js";
import { bindDueDatePresets } from "./form-handlers.js";

function prepareAddTaskForm(dueDate) {
  const tf = document.getElementById("newTaskForm");
  if (!tf) return;

  const titleEl = tf.querySelector("#title");
  if (titleEl) titleEl.value = "";
  const descEl = tf.querySelector("#description");
  if (descEl) descEl.value = "";
  const projEl = tf.querySelector("#project_id");
  if (projEl) projEl.value = "";
  const dueEl = tf.querySelector("#due_date");
  if (dueEl) dueEl.value = dueDate || "";

  const idInput = tf.querySelector('input[name="id"]');
  if (idInput) idInput.remove();

  const submit = tf.querySelector('button[type="submit"]');
  if (submit) submit.textContent = "Add Task";

  tf.setAttribute("hx-post", tf.getAttribute("data-add-action") || tf.getAttribute("hx-post") || "");

  const sbTitle = document.querySelector("#sidebar .sidebar-header h5");
  if (sbTitle) sbTitle.textContent = "Add Task";

  const charCount = document.getElementById("char-count");
  if (charCount) charCount.textContent = "0";

  const newTagsEl = tf.querySelector("#new_tags");
  if (newTagsEl) newTagsEl.value = "";
  tf.querySelectorAll('input[name="tag_ids"]').forEach((cb) => {
    cb.checked = false;
  });

  bindDueDatePresets(tf);
}

export function initCalendarPage() {
  const page = document.querySelector(".calendar-page");
  if (!page) return;

  const addBtn = document.getElementById("calendar-add-task");
  if (addBtn) {
    addBtn.addEventListener("click", () => {
      prepareAddTaskForm("");
      openSidebar();
      const titleEl = document.getElementById("title");
      if (titleEl) titleEl.focus();
    });
  }

  page.addEventListener("click", (e) => {
    const dayBtn = e.target.closest(".calendar-add-btn");
    if (!dayBtn) return;
    e.preventDefault();
    prepareAddTaskForm(dayBtn.getAttribute("data-due-date") || "");
    openSidebar();
    const titleEl = document.getElementById("title");
    if (titleEl) titleEl.focus();
  });
}
