package handlers

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

// renderHTML writes the standard text/html content type and renders c. Render
// errors after the headers are committed are unrecoverable, so we log at Warn
// and return; this matches the boilerplate every handler used to inline.
//
// The view name is required so logs are attributable; logger may be nil during
// tests, in which case render errors are swallowed silently.
func renderHTML(w http.ResponseWriter, r *http.Request, name string, c templ.Component, logger *slog.Logger, extra ...any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil && logger != nil {
		args := append([]any{"view", name}, extra...)
		args = append(args, "err", err)
		logger.Warn("render failed", args...)
	}
}
