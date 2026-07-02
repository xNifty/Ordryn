import { ensureToastContainer } from "./utils.js";

export function showToast(message, opts) {
  opts = opts || {};
  const container = ensureToastContainer();
  const t = document.createElement("div");
  t.className = "app-toast" + (opts.error ? " app-toast--error" : "");
  t.setAttribute("role", "status");
  t.setAttribute("aria-live", "polite");

  const text = document.createElement("span");
  text.className = "app-toast-text";
  text.textContent = message;
  t.appendChild(text);

  if (opts.actionLabel && typeof opts.onAction === "function") {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "btn btn-sm btn-link app-toast-action";
    btn.textContent = opts.actionLabel;
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      opts.onAction();
      clearTimeout(to);
      remove();
    });
    t.appendChild(btn);
  }

  container.appendChild(t);

  requestAnimationFrame(() => {
    t.classList.add("show");
  });

  const timeout = typeof opts.duration === "number" ? opts.duration : 3500;
  const remove = () => {
    t.classList.remove("show");
    setTimeout(() => {
      try {
        t.remove();
      } catch (e) {}
    }, 220);
  };

  const to = setTimeout(remove, timeout);

  if (!opts.actionLabel) {
    t.addEventListener("click", function () {
      clearTimeout(to);
      remove();
    });
  }
}

export function attachNotificationListeners() {
  // Reserved for future server-driven notification triggers.
}
