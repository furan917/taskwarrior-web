// core_form_errors.js - polyfills hx-target-error for htmx 2.x.
// htmx 2.0.4 does not support hx-target-error natively (it lives in the
// response-targets extension). We implement it via htmx:beforeSwap: when the
// server returns a 4xx/5xx and the triggering element carries hx-target-error,
// redirect the swap to that selector and force innerHTML. This makes error
// responses from any form with hx-target-error land in the right container
// regardless of the form's own hx-swap value.
(function () {
  document.body.addEventListener('htmx:beforeSwap', function (evt) {
    var d = evt.detail;
    if (!d.isError) return;
    var elt = d.elt;
    if (!elt) return;
    var sel = elt.getAttribute('hx-target-error') || elt.getAttribute('data-hx-target-error');
    if (!sel) return;
    var target = document.querySelector(sel);
    if (!target) return;
    d.shouldSwap = true;
    d.target = target;
    d.swapOverride = 'innerHTML';
  });

  // Clear the error container before each new submission so stale errors
  // from a prior attempt don't persist if the user fixes and resubmits.
  document.body.addEventListener('htmx:beforeRequest', function (evt) {
    var elt = evt.detail && evt.detail.elt;
    if (!elt) return;
    var sel = elt.getAttribute('hx-target-error') || elt.getAttribute('data-hx-target-error');
    if (!sel) return;
    var target = document.querySelector(sel);
    if (target) target.innerHTML = '';
  });
})();
