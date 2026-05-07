// context_pill.js - context pill dropdown in the nav header. Click pill
// toggles the open state; click on a menu item triggers the htmx POST /context.
// Clear-x and click-outside / Esc close the menu.
// Click on the pill toggles [data-open="1"] on the wrapper; the sibling
// dropdown menu's hidden state is controlled by a CSS rule injected here so
// we don't have to touch tailwind.input.css. Click-outside or Esc closes.
//
// The clear "×" inside the active pill must NOT toggle the dropdown - it's
// already an htmx POST target, so we let it through and short-circuit the
// pill click handler when the event originated inside [data-context-clear].
//
// Each menu item's click is left to htmx (hx-post + HX-Refresh on the
// server). We only take care of opening/closing the menu.
(function () {
  // Inject the CSS that drives the open/closed state. tailwind.input.css is
  // off-limits in this pass; a one-line stylesheet here is the smallest
  // surface that gets the job done.
  const style = document.createElement('style');
  style.textContent =
    '[data-context-wrap][data-open="1"] [data-context-menu]{display:block !important;}';
  document.head.appendChild(style);

  function wrap()   { return document.querySelector('[data-context-wrap]'); }
  function isOpen() { const w = wrap(); return !!(w && w.getAttribute('data-open') === '1'); }

  function open()  { const w = wrap(); if (w) w.setAttribute('data-open', '1'); }
  function close() { const w = wrap(); if (w) w.removeAttribute('data-open'); }

  // Pill click toggles. The clear-context "×" lives inside the pill button
  // and carries data-context-clear; we let HTMX handle it without flipping
  // the menu open. Same for clicks landing inside the menu itself - HTMX
  // takes the click; we simply don't intercept.
  document.addEventListener('click', function (e) {
    const t = e.target;
    if (!(t instanceof Element)) return;

    // Click on the clear "×": let it through.
    if (t.closest('[data-context-clear]')) return;

    // Click on a menu item: let HTMX handle the request; close after.
    if (t.closest('[data-context-item]')) {
      // The HX-Refresh server response will reload the page so we don't
      // need to update anything client-side; closing is a courtesy in
      // case the request races visibly slow.
      close();
      return;
    }

    // Click on the pill itself: toggle.
    if (t.closest('[data-context-pill]')) {
      e.preventDefault();
      if (isOpen()) close();
      else open();
      return;
    }

    // Click anywhere else: close if open.
    if (isOpen() && !t.closest('[data-context-wrap]')) close();
  });

  // Esc closes the menu (only when no dialog is open - the dialog has its
  // own Esc handling).
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    if (document.querySelector('dialog[open]')) return;
    if (!isOpen()) return;
    e.stopPropagation();
    close();
  }, true);
})();
