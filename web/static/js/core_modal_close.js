// core_modal_close.js - close the enclosing <dialog> after a successful HTMX
// request from any element carrying [data-close-on-success]. Replaces inline
// hx-on::after-request handlers so the CSP unsafe-eval relaxation stays a
// defence-in-depth signal rather than an active sink.
// Close the enclosing <dialog> after a successful HTMX request from any
// element carrying [data-close-on-success]. Replaces inline
// hx-on::after-request="..." in modal_edit.templ - those work via htmx
// 2.x's eval-backed hx-on handler (CSP allows 'unsafe-eval'), but the
// project convention is to keep all JS in app.js so the CSP relaxation
// stays a defence-in-depth signal rather than an active sink.
document.body.addEventListener('htmx:afterRequest', function (evt) {
  if (!evt.detail.successful) return;
  const trigger = evt.detail.elt;
  if (!trigger || !trigger.hasAttribute('data-close-on-success')) return;
  const dlg = trigger.closest('dialog');
  if (dlg && typeof dlg.close === 'function') dlg.close();
});
