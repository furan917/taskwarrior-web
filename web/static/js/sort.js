// sort.js - sort sync. After an HTMX swap into #task-list, mirror the
// chosen sort param into the list's hx-get so the next 30s poll preserves
// the sort. Sibling of search.js - each handles one URL param so neither
// has to know about the other's state.
// Clicking a sort header issues hx-get to /partials/list?...&sort=<key>:<dir>
// and swaps the result into #task-list. After each such swap we mirror the
// chosen sort param into #task-list's hx-get so the next 30 s poll preserves
// the order. The signal we read is the request URL HTMX just made, available
// via evt.detail.pathInfo.requestPath on htmx:afterSwap.
//
// This is intentionally an additive sibling of the search-sync IIFE above,
// not a modification of it: each sync handles one URL param so neither needs
// to know about the other's state.
(function () {
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (!evt.detail || !evt.detail.target || evt.detail.target.id !== 'task-list') return;
    const reqPath = evt.detail.pathInfo && evt.detail.pathInfo.requestPath;
    if (!reqPath) return;
    let incomingURL;
    try {
      incomingURL = new URL(reqPath, window.location.origin);
    } catch (e) {
      return;
    }
    // Only act on swaps driven by /partials/list - other endpoints might pass
    // through the same swap event but don't carry our sort param.
    if (incomingURL.pathname !== '/partials/list') return;
    const sortVal = incomingURL.searchParams.get('sort');
    const list = document.getElementById('task-list');
    if (!list) return;
    const cur = list.getAttribute('hx-get');
    if (!cur) return;
    const url = new URL(cur, window.location.origin);
    if (sortVal) url.searchParams.set('sort', sortVal);
    else url.searchParams.delete('sort');
    const next = url.pathname + url.search;
    if (next !== cur) {
      list.setAttribute('hx-get', next);
      if (window.htmx) window.htmx.process(list);
    }
  });
})();
