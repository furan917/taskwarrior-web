// burndown_tabs.js — tab switching for the burndown chart on /stats.
// Uses event delegation so it survives any HTMX swap.
document.addEventListener('click', function (evt) {
  const btn = evt.target.closest && evt.target.closest('[data-burndown-tab]');
  if (!btn) return;

  const period = btn.dataset.burndownTab;
  const daily = document.getElementById('bd-daily');
  const weekly = document.getElementById('bd-weekly');
  if (!daily || !weekly) return;

  daily.hidden = period !== 'daily';
  weekly.hidden = period !== 'weekly';

  const active = 'rounded px-2 py-0.5 text-xs font-medium bg-zinc-800 text-white dark:bg-zinc-100 dark:text-zinc-900';
  const inactive = 'rounded px-2 py-0.5 text-xs font-medium text-zinc-500 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800';
  document.querySelectorAll('[data-burndown-tab]').forEach(function (t) {
    t.className = t.dataset.burndownTab === period ? active : inactive;
  });
});
