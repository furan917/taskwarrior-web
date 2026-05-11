// mobile_nav.js - opens / closes the mobile nav <dialog>.
//
// Using <dialog>.showModal() means the browser handles:
//   - top-layer rendering (no z-index gymnastics)
//   - focus trap (Tab/Shift-Tab cycle inside the dialog)
//   - rest-of-page inert (no JS workaround needed)
//   - Escape to close
//   - scroll lock on <html>
//   - focus restoration when the dialog closes
//
// We own only the open trigger and the click-outside-to-close pattern
// (which <dialog> doesn't ship natively). aria-expanded on the
// hamburger trigger is synced so AT correctly announces drawer state.
(function () {
  var drawer = document.getElementById('mobile-nav-drawer');
  if (!drawer) return;

  var openBtn  = document.getElementById('mobile-nav-open');
  var closeBtn = document.getElementById('mobile-nav-close');

  function open() {
    if (typeof drawer.showModal !== 'function' || drawer.open) return;
    drawer.showModal();
    openBtn && openBtn.setAttribute('aria-expanded', 'true');
  }

  function close() {
    if (drawer.open) drawer.close();
  }

  // Native close event fires for every close path (button, Esc, ::backdrop,
  // programmatic). One listener keeps aria-expanded honest regardless of
  // how the drawer was dismissed.
  drawer.addEventListener('close', function () {
    openBtn && openBtn.setAttribute('aria-expanded', 'false');
  });

  // Click on the dialog ELEMENT itself (i.e. on the ::backdrop pseudo-
  // element, which bubbles up with target=dialog) closes it. Clicks on
  // child elements have target=child and pass through unchanged - this
  // is the standard "click outside the panel to dismiss" pattern.
  drawer.addEventListener('click', function (e) {
    if (e.target === drawer) close();
  });

  openBtn  && openBtn.addEventListener('click', open);
  closeBtn && closeBtn.addEventListener('click', close);
})();
