// filter.js - ad-hoc Taskwarrior filter expression sync.
//
// After an HTMX swap into #task-list, mirror the current filter value into:
//   1. #task-list's hx-get so the 30s poll keeps the filter alive.
//   2. The browser URL (?filter=...) so refresh, back, and bookmarks work.
//
// Sibling of search.js and sort.js - each handles one URL param independently
// so none needs to know about the others' state.
(function () {
  function syncFilter() {
    const input = document.getElementById('filter-input');
    const list = document.getElementById('task-list');
    if (!input || !list) return;

    const val = input.value.trim();

    // --- 1. Keep polling URL in sync ---
    const cur = list.getAttribute('hx-get');
    if (cur) {
      const pollUrl = new URL(cur, window.location.origin);
      if (val) pollUrl.searchParams.set('filter', val);
      else pollUrl.searchParams.delete('filter');
      const next = pollUrl.pathname + pollUrl.search;
      if (next !== cur) {
        list.setAttribute('hx-get', next);
        if (window.htmx) window.htmx.process(list);
      }
    }

    // --- 2. Keep browser URL in sync (replaceState - no history clutter) ---
    const pageUrl = new URL(window.location.href);
    if (val) pageUrl.searchParams.set('filter', val);
    else pageUrl.searchParams.delete('filter');
    const newHref = pageUrl.pathname + pageUrl.search;
    if (newHref !== window.location.pathname + window.location.search) {
      history.replaceState(null, '', newHref);
    }
  }

  document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (evt.detail && evt.detail.target && evt.detail.target.id === 'task-list') {
      syncFilter();
    }
  });
})();
