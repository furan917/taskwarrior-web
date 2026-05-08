// context_pill.js - generic dropdown popover used by the context pill AND
// the nav's "More" menu. Any element with [data-popover-wrap] becomes a
// togglable popover: the trigger inside it carries [data-popover-trigger],
// the menu carries [data-popover-menu], and individual items carry
// [data-popover-item]. Clicking the trigger toggles [data-open="1"] on the
// wrapper; clicking outside / pressing Esc closes whichever popover is open.
//
// The context pill's clear-context "×" carries [data-context-clear] - that
// short-circuits the trigger toggle so HTMX handles the POST without the
// menu flapping open. That attribute is intentionally NOT generalised to
// data-popover-clear because it's pill-specific behaviour.
//
// Each menu item's click is left to its own action (htmx for context items,
// plain anchor navigation for nav links). We close the popover before the
// click bubbles so the visual feedback is "menu closes, then page reacts"
// rather than the menu hanging open during an inflight request.
(function () {
  // The open-state visibility rule lives in tailwind.input.css under
  // @layer components (see [data-popover-wrap][data-open="1"] selector).
  // Keeping it in the static stylesheet means it applies on first paint
  // before this defer'd script executes - no FOUC for server-rendered
  // popovers.

  function close(wrap) { if (wrap) wrap.removeAttribute('data-open'); }
  function open(wrap)  { if (wrap) wrap.setAttribute('data-open', '1'); }
  function closeAll() {
    document.querySelectorAll('[data-popover-wrap][data-open="1"]').forEach(close);
  }

  document.addEventListener('click', function (e) {
    const t = e.target;
    if (!(t instanceof Element)) return;

    // Context-pill clear "×": let HTMX handle the request, do NOT toggle.
    if (t.closest('[data-context-clear]')) return;

    // Click on a menu item: close before the action so the visual reads
    // "menu dismissed, page updates" rather than menu lingering during
    // an inflight htmx swap or page navigation.
    const item = t.closest('[data-popover-item]');
    if (item) {
      close(item.closest('[data-popover-wrap]'));
      return;
    }

    // Click on a trigger: close every other open popover, then toggle this
    // one. The closeAll-then-open dance means at most one popover is ever
    // open at a time, which matches user intuition for nav-bar dropdowns.
    const trigger = t.closest('[data-popover-trigger]');
    if (trigger) {
      e.preventDefault();
      const wrap = trigger.closest('[data-popover-wrap]');
      if (!wrap) return;
      const wasOpen = wrap.getAttribute('data-open') === '1';
      closeAll();
      if (!wasOpen) open(wrap);
      return;
    }

    // Click outside any popover wrap: close everything.
    if (!t.closest('[data-popover-wrap]')) closeAll();
  });

  // Esc closes whichever popover is open. The kebab menu inside an open
  // ModalEdit needs Esc to close the menu, NOT the dialog underneath -
  // otherwise opening the kebab and pressing Esc would dismiss the entire
  // modal and drop in-progress edits. We handle Esc here and stop
  // propagation so the dialog's native Esc handler never fires; the
  // dialog can still be Esc-closed when no popover is open (no listener
  // is attached at all in that case).
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    const openWrap = document.querySelector('[data-popover-wrap][data-open="1"]');
    if (!openWrap) return;
    e.stopPropagation();
    e.preventDefault();
    close(openWrap);
  }, true);
})();
