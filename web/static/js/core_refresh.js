// core_refresh.js - shouldRefresh() (used by hx-trigger="every 30s [shouldRefresh()]")
// and visibilitychange immediate-refresh trigger.
// taskwarrior-web client-side helpers.
// Loaded after htmx.min.js. Jobs:
//   1. shouldRefresh()  - HTMX trigger gate (used by hx-trigger="every 30s [shouldRefresh()]")
//   2. visibilitychange - immediate refresh when the tab returns to foreground
//   3. Keybindings      - n/j/k/Enter/x/? for keyboard-only navigation
//   4. Theme toggle     - .theme-toggle click flips light/dark and persists

function shouldRefresh() {
  if (document.visibilityState !== 'visible') return false;
  if (document.querySelector('dialog[open]')) return false;
  const a = document.activeElement;
  if (a && (a.tagName === 'INPUT' || a.tagName === 'TEXTAREA' || a.isContentEditable)) return false;
  return true;
}

document.addEventListener('visibilitychange', function () {
  if (document.visibilityState === 'visible' && window.htmx) {
    window.htmx.trigger('body', 'refresh');
  }
});
