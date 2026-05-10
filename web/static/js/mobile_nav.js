// mobile_nav.js — hamburger slide-out nav drawer.
(function () {
  var drawer = document.getElementById('mobile-nav-drawer');
  if (!drawer) return;

  var backdrop = document.getElementById('mobile-nav-backdrop');
  var openBtn  = document.getElementById('mobile-nav-open');
  var closeBtn = document.getElementById('mobile-nav-close');

  function open() {
    drawer.hidden = false;
    drawer.setAttribute('aria-hidden', 'false');
    openBtn && openBtn.setAttribute('aria-expanded', 'true');
    document.body.style.overflow = 'hidden';
  }

  function close() {
    drawer.hidden = true;
    drawer.setAttribute('aria-hidden', 'true');
    openBtn && openBtn.setAttribute('aria-expanded', 'false');
    document.body.style.overflow = '';
  }

  openBtn   && openBtn.addEventListener('click', open);
  closeBtn  && closeBtn.addEventListener('click', close);
  backdrop  && backdrop.addEventListener('click', close);

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && !drawer.hidden) close();
  });
})();
