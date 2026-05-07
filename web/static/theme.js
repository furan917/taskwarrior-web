// theme.js - synchronous early-load script that applies the dark class to
// <html> before first paint to avoid a flash of unstyled (light) content.
// Loaded in <head> with no defer; runs before the document body is parsed.
//
// Resolution order:
//   1. localStorage 'theme' === 'dark' or 'light' (explicit user choice)
//   2. window.matchMedia('(prefers-color-scheme: dark)') (OS preference)
//
// The toggle handler in app.js writes localStorage and toggles the class.
(function () {
  try {
    var stored = localStorage.getItem('theme');
    var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    var dark = stored === 'dark' || (stored == null && prefersDark);
    if (dark) document.documentElement.classList.add('dark');
  } catch (e) {}
})();
