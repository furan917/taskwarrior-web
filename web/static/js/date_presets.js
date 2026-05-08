// date_presets.js - quick-pick chip buttons under each date input in the
// add/edit task modal. Click a chip and the sibling input[name=<target>]
// gets filled with a Taskwarrior date token (today, +3d, due-7d, ...).
//
// We dispatch a bubbling `input` event after setting .value so the
// form_validation.js IIFE clears any red border that the server may have
// stamped on the field. No other side effects - the form is submitted
// normally on save.
//
// Delegated at the document level so it survives modal swaps from htmx
// without needing to re-bind on each open.
document.addEventListener('click', function (evt) {
  const btn = evt.target.closest && evt.target.closest('[data-date-preset]');
  if (!btn) return;
  const target = btn.getAttribute('data-target');
  if (!target) return;
  // Scope the lookup to the enclosing <label> so two date fields with the
  // same parent grid don't fight each other. Fall back to the enclosing
  // dialog if the partial is ever rendered outside a label.
  const scope = btn.closest('label') || btn.closest('dialog') || document;
  const input = scope.querySelector('input[name="' + target + '"]');
  if (!input) return;
  input.value = btn.textContent.trim();
  input.dispatchEvent(new Event('input', { bubbles: true }));
});
