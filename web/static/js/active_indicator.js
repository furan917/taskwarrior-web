// active_indicator.js — nav active-task chip behaviour.
//
// 1. Click outside the open dropdown → close it.
// 2. Click a task item inside the dropdown → close dropdown so the edit
//    modal opens cleanly without a stale dropdown underneath.
(function () {
  document.addEventListener('click', function (e) {
    const indicator = document.getElementById('active-indicator');
    if (!indicator) return;
    const det = indicator.querySelector('details[data-active-dropdown]');
    if (!det || !det.open) return;
    if (!det.contains(e.target)) {
      det.removeAttribute('open');
    }
  });

  document.addEventListener('click', function (e) {
    const item = e.target.closest && e.target.closest('[data-active-task-item]');
    if (!item) return;
    const det = item.closest('details[data-active-dropdown]');
    if (det) det.removeAttribute('open');
  });
})();
