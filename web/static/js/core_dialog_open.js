// core_dialog_open.js - when HTMX swaps a <dialog> into #modal, auto-open
// it via showModal(). Replaces the inline <script>showModal()</script>
// pattern that our CSP blocks (script-src does not include unsafe-inline).
// When HTMX swaps a <dialog> into #modal, auto-open it. Replaces the inline
// <script>showModal()</script> we used to ship in modal templates - blocked
// by our CSP (script-src does not include 'unsafe-inline').
document.body.addEventListener('htmx:afterSettle', function (evt) {
  const target = evt.detail && evt.detail.target;
  if (!target || target.id !== 'modal') return;
  const dlg = target.querySelector('dialog:not([open])');
  if (dlg && typeof dlg.showModal === 'function') {
    dlg.showModal();
  }
});
