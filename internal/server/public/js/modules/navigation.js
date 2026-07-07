import { initTheme, attachThemeToggle } from "./theme.js";

/** Re-init page modules after hx-boost navigation. */
export function initNavigation() {
  if (typeof htmx === "undefined") return;
  document.body.addEventListener("htmx:afterSwap", (e) => {
    if (e.detail.boosted) {
      initTheme();
      attachThemeToggle();
      if (document.body.classList.contains("profile-page")) {
        import("./profile.js").then((m) => m.initProfilePage());
      }
      if (document.body.classList.contains("calendar-page")) {
        import("./calendar.js").then((m) => m.initCalendarPage());
      }
    }
  });
}
