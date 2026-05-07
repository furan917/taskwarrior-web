// row_expand.js - row info expand/collapse. Click on .row-toggle flips
// data-expanded on the parent <li> and toggles the .row-details visibility.
// State persists across HTMX swaps via a Set keyed by task UUID id.
// ─── Row info expand/collapse (v3) ──────────────────────────────────────
// A click on .row-toggle flips data-expanded on the parent <li> and toggles
// the .row-details visibility. The chevron SVG rotates 90° via a class flip
// (CSS transition handles the animation). Persists across HTMX swaps because
// the toggle state is stored in a Set keyed by the task's UUID id
// ("task-<uuid>"), the same id app.js already uses for row focus.
(function () {
  const expanded = new Set();

  function applyToRow(li) {
    if (!li) return;
    const isOpen = expanded.has(li.id);
    li.setAttribute('data-expanded', isOpen ? '1' : '0');
    const det = li.querySelector('.row-details');
    if (det) det.classList.toggle('hidden', !isOpen);
    const chev = li.querySelector('.row-chevron');
    if (chev) chev.classList.toggle('rotate-90', isOpen);
    const btn = li.querySelector('.row-toggle');
    if (btn) btn.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
  }

  document.addEventListener('click', function (e) {
    const btn = e.target.closest && e.target.closest('.row-toggle');
    if (!btn) return;
    const li = btn.closest('li[id^="task-"]');
    if (!li) return;
    if (expanded.has(li.id)) expanded.delete(li.id);
    else expanded.add(li.id);
    applyToRow(li);
  });

  // After HTMX swap, restore expand state on each visible row. Also drop ids
  // whose row is no longer rendered so the set doesn't grow unbounded.
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (!evt.detail || !evt.detail.target) return;
    if (evt.detail.target.id !== 'task-list') return;
    const present = new Set();
    document.querySelectorAll('#task-list li[id^="task-"]').forEach(function (li) {
      present.add(li.id);
      applyToRow(li);
    });
    Array.from(expanded).forEach(function (id) {
      if (!present.has(id)) expanded.delete(id);
    });
  });
})();

