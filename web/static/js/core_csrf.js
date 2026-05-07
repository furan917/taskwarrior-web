// core_csrf.js - attach the CSRF token from <meta name="csrf-token"> to every
// HTMX request as X-CSRF-Token. Server middleware enforces on writes.
// Attach the CSRF token (set in <meta name="csrf-token">) as an X-CSRF-Token
// header on every HTMX request. Server middleware validates it on writes.
document.body.addEventListener('htmx:configRequest', function (evt) {
  const meta = document.querySelector('meta[name="csrf-token"]');
  if (meta && meta.content) {
    evt.detail.headers['X-CSRF-Token'] = meta.content;
  }
});
