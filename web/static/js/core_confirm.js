// core_confirm.js - replace HTMX native window.confirm() with the styled
// #confirm-dialog from layout.templ. issueRequest(true) skips htmx 2.x's
// own re-confirm path which would otherwise stack a native popup behind ours.
// Replace HTMX's native window.confirm() with a styled <dialog> defined in
// layout.templ (#confirm-dialog). The flow: button has hx-confirm="..." ->
// HTMX fires htmx:confirm -> we preventDefault, show the modal, and call
// evt.detail.issueRequest(true) on confirm. The `true` is the
// skipConfirmation flag - without it, htmx 2.x re-runs its native
// window.confirm() check inside issueAjaxRequest and the user sees TWO
// confirmation dialogs stacked (browser-native on top, ours behind).
document.body.addEventListener('htmx:confirm', function (evt) {
  if (!evt.detail.question) return;
  evt.preventDefault();
  const dialog = document.getElementById('confirm-dialog');
  const message = document.getElementById('confirm-message');
  if (!dialog || !message) {
    // Fallback: if the dialog isn't on this page, behave like the default.
    if (window.confirm(evt.detail.question)) evt.detail.issueRequest(true);
    return;
  }
  message.textContent = evt.detail.question;
  dialog.returnValue = '';
  dialog.showModal();
  dialog.addEventListener('close', function handler() {
    dialog.removeEventListener('close', handler);
    if (dialog.returnValue === 'confirm') {
      evt.detail.issueRequest(true);
    }
  });
});
