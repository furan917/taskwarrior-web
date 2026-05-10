// chip_scroll.js — scrolls truncated calendar chip text on hover.
// When the inner text is wider than the chip button, a linear translateX
// animation slides it left to reveal the full title, then snaps back on mouseout.
(function () {
  document.addEventListener('mouseover', function (e) {
    var btn = e.target.closest && e.target.closest('[data-chip-scroll]');
    if (!btn) return;
    var text = btn.querySelector('[data-chip-text]');
    if (!text) return;
    var style = getComputedStyle(btn);
    var available = btn.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    var overflow = text.scrollWidth - available;
    if (overflow < 4) return;
    var ms = Math.max(800, Math.min(overflow * 20, 3500));
    text.style.transition = 'transform ' + ms + 'ms linear 500ms';
    text.style.transform = 'translateX(-' + overflow + 'px)';
  });

  document.addEventListener('mouseout', function (e) {
    var btn = e.target.closest && e.target.closest('[data-chip-scroll]');
    if (!btn) return;
    var text = btn.querySelector('[data-chip-text]');
    if (!text) return;
    text.style.transition = 'transform 150ms ease';
    text.style.transform = '';
  });
})();
