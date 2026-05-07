// core_theme.js - theme toggle. theme.js (loaded synchronously in <head>)
// sets the initial class before paint; this handler flips it on click and
// persists via localStorage.
// Theme toggle. theme.js (loaded synchronously in <head>) sets the initial
// class before paint; this handler flips it on user click and persists the
// choice in localStorage so it sticks across reloads. The button lives in
// the header in layout.templ and carries .theme-toggle - delegated so it
// survives any HTMX swap that might reset its bindings.
document.addEventListener('click', function (evt) {
  const btn = evt.target.closest && evt.target.closest('.theme-toggle');
  if (!btn) return;
  const root = document.documentElement;
  const nowDark = !root.classList.contains('dark');
  root.classList.toggle('dark', nowDark);
  try { localStorage.setItem('theme', nowDark ? 'dark' : 'light'); } catch (e) {}
});
