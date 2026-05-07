// core_dismiss.js - delegated close handler for buttons carrying
// [data-dismiss-modal] (used in modal headers / cancel buttons).
// Delegated close handler. Buttons inside a modal carry data-dismiss-modal
// instead of inline onclick (CSP blocks inline event handlers under our
// script-src). One delegated listener finds the enclosing dialog and closes it.
document.addEventListener('click', function (evt) {
  const btn = evt.target.closest && evt.target.closest('[data-dismiss-modal]');
  if (!btn) return;
  const dlg = btn.closest('dialog');
  if (dlg && typeof dlg.close === 'function') dlg.close();
});
