// burndown_tabs.js - tab switching for the burndown chart on /stats.
// Uses event delegation so it survives any HTMX swap. Selected tab
// (daily/weekly) is persisted in sessionStorage so a reload preserves
// the user's choice rather than always snapping back to "daily".
(function () {
  const STORAGE_KEY = 'twweb.burndown.tab';

  const ACTIVE_CLASS = 'rounded px-2 py-0.5 text-xs font-medium bg-zinc-800 text-white dark:bg-zinc-100 dark:text-zinc-900';
  const INACTIVE_CLASS = 'rounded px-2 py-0.5 text-xs font-medium text-zinc-500 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800';

  function setPeriod(period, persist) {
    const daily = document.getElementById('bd-daily');
    const weekly = document.getElementById('bd-weekly');
    if (!daily || !weekly) return;

    daily.hidden = period !== 'daily';
    weekly.hidden = period !== 'weekly';

    document.querySelectorAll('[data-burndown-tab]').forEach(function (t) {
      t.className = t.dataset.burndownTab === period ? ACTIVE_CLASS : INACTIVE_CLASS;
    });

    if (persist) {
      try { sessionStorage.setItem(STORAGE_KEY, period); } catch (_) { /* private mode */ }
    }
  }

  // Click handler: switch + persist.
  document.addEventListener('click', function (evt) {
    const btn = evt.target.closest && evt.target.closest('[data-burndown-tab]');
    if (!btn) return;
    setPeriod(btn.dataset.burndownTab, true);
  });

  // On initial load (and after htmx swaps that re-render the burndown
  // section), apply the persisted choice if the section is present.
  function restore() {
    if (!document.getElementById('bd-daily')) return;
    let saved;
    try { saved = sessionStorage.getItem(STORAGE_KEY); } catch (_) { saved = null; }
    if (saved !== 'weekly' && saved !== 'daily') return; // no preference yet → server default wins
    setPeriod(saved, false);
  }
  document.addEventListener('DOMContentLoaded', restore);
  document.body.addEventListener('htmx:afterSwap', restore);
})();
