// search.js - search box sync. After an HTMX swap into #task-list, mirror
// the current ?q= value into the list's hx-get so the next 30s poll keeps
// the filter applied.
// ─── Search filter (v2) ─────────────────────────────────────────────────
// The search input emits hx-get to /partials/list?report=...&q=... on input.
// After each swap, mirror the q value into #task-list's hx-get so the 30s
// poll keeps the filter alive. Without this, polling drops the filter.
(function () {
  function syncListUrl() {
    const search = document.querySelector('input[type="search"][name="q"]');
    const list = document.getElementById('task-list');
    if (!search || !list) return;
    const cur = list.getAttribute('hx-get');
    if (!cur) return;
    const url = new URL(cur, window.location.origin);
    const v = search.value.trim();
    if (v) url.searchParams.set('q', v);
    else url.searchParams.delete('q');
    const next = url.pathname + url.search;
    if (next !== cur) {
      list.setAttribute('hx-get', next);
      if (window.htmx) window.htmx.process(list);
    }
  }
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (evt.detail && evt.detail.target && evt.detail.target.id === 'task-list') {
      syncListUrl();
    }
  });
})();
