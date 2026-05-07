// form_validation.js - inline form-error highlighting. After an HTMX swap
// into #task-form-errors, red-border the input matching data-field-error and
// auto-focus it. Highlight clears on the first input event so it isn't sticky.
// ─── Form validation surfacing (v5) ─────────────────────────────────────
// When Create/Modify return 400, the server renders an error fragment into
// #task-form-errors carrying data-field-error="<name>". We add a red
// border to the matching input so the user has both an inline message AND
// a visual cue at the field itself, then clear that highlight as soon as
// the user edits the offending field.
//
// htmx:afterSwap fires after the error fragment lands; we run on that
// instead of htmx:afterRequest so the DOM has the data-field-error attr
// already in place when we read it.
(function () {
  const RED = ['border-red-500', 'ring-1', 'ring-red-500', 'focus:border-red-500'];

  function clearAllHighlights(scope) {
    scope.querySelectorAll('[data-field-invalid="1"]').forEach(function (el) {
      el.removeAttribute('data-field-invalid');
      RED.forEach(function (c) { el.classList.remove(c); });
    });
  }

  document.body.addEventListener('htmx:afterSwap', function (evt) {
    const t = evt.detail && evt.detail.target;
    if (!t || t.id !== 'task-form-errors') return;
    const dialog = t.closest('dialog');
    if (!dialog) return;
    clearAllHighlights(dialog);
    const fragment = t.querySelector('[data-field-error]');
    if (!fragment) return;
    const field = fragment.getAttribute('data-field-error');
    if (!field) return;
    const input = dialog.querySelector('[name="' + field + '"]');
    if (!input) return;
    input.setAttribute('data-field-invalid', '1');
    RED.forEach(function (c) { input.classList.add(c); });
    // Surface focus to the offending field so the user lands on it.
    if (typeof input.focus === 'function') input.focus();
  });

  // Clear the highlight + error fragment as soon as the user starts editing
  // the offending field. Keeps the error from feeling sticky once the user
  // is actively fixing it.
  document.addEventListener('input', function (e) {
    const inp = e.target;
    if (!(inp instanceof Element)) return;
    if (inp.getAttribute('data-field-invalid') !== '1') return;
    inp.removeAttribute('data-field-invalid');
    RED.forEach(function (c) { inp.classList.remove(c); });
    const dialog = inp.closest('dialog');
    if (!dialog) return;
    const errs = dialog.querySelector('#task-form-errors');
    if (errs) errs.innerHTML = '';
  });
})();
