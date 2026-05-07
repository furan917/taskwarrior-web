// core_refresh_modal.js - on HX-Trigger=refresh, close any open <dialog>
// and reload the page when no #task-list element is present (Calendar /
// Done / Browse). Pages with #task-list listen for refresh themselves
// via hx-trigger.
// When the server sends `HX-Trigger: refresh` after a successful write,
// close any open <dialog> and let the page update.
//
// Pages with #task-list (Ready, Next, Agenda, Forecast, project, tag) have
// `hx-trigger="refresh from:body"` so the list partial re-fetches itself
// without a full reload. Pages without #task-list (Calendar, Done, Browse)
// have no listener, so we fall back to window.location.reload() so the
// view picks up the change without the user having to refresh manually.
document.body.addEventListener('htmx:afterRequest', function (evt) {
  if (!(evt.detail.successful && evt.detail.xhr.getResponseHeader('HX-Trigger') === 'refresh')) {
    return;
  }
  document.querySelectorAll('dialog[open]').forEach(function (d) { d.close(); });
  if (!document.getElementById('task-list')) {
    window.location.reload();
  }
});
