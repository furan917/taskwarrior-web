package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
)

// allowedHosts is built from config.AllowedHosts() at init. Extended by TWP_ALLOWED_HOSTS.
var allowedHosts = func() map[string]bool {
	out := map[string]bool{
		// Some clients omit the port for default-port URLs; safe to allow
		// the bare host since the listener only binds 127.0.0.1 anyway.
		"localhost": true,
		"127.0.0.1": true,
	}
	for _, h := range config.AllowedHosts() {
		out[h] = true
	}
	return out
}()

// allowedOrigins mirrors allowedHosts with http:// prefix. Extended by TWP_ALLOWED_HOSTS.
var allowedOrigins = func() map[string]bool {
	out := map[string]bool{}
	for _, o := range config.AllowedOrigins() {
		out[o] = true
	}
	return out
}()

// hostAllowlist gates ALL requests on Host header. 421 (Misdirected Request)
// is the spec-correct status for "this host doesn't serve here".
func hostAllowlist(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] {
			if logger != nil {
				logger.Warn("host rejected",
					"middleware", "hostAllowlist",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"host", r.Host)
			}
			http.Error(w, "host not allowed", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowlist applies only to state-changing methods. GETs are not
// origin-checked (browsers send Origin only on writes/CORS).
func originAllowlist(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Origin should be present on cross-origin POSTs from browsers.
			// Same-origin XHR/HTMX from our own page also sets it. Reject if
			// missing (legitimate browser writes always include it in 2026).
			if logger != nil {
				logger.Warn("origin rejected",
					"middleware", "originAllowlist",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"origin", "")
			}
			http.Error(w, "missing origin", http.StatusForbidden)
			return
		}
		if !allowedOrigins[origin] {
			if logger != nil {
				logger.Warn("origin rejected",
					"middleware", "originAllowlist",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"origin", origin)
			}
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets defence-in-depth response headers on every response.
//
// CSP trade-offs:
//   - script-src needs 'unsafe-eval': HTMX evaluates trigger filter expressions
//     (e.g. `every 30s [shouldRefresh()]`) at runtime via Function constructor.
//   - style-src needs 'unsafe-inline': HTMX injects inline styles during
//     swaps/transitions, and the urgency-bar component uses style="width:.."
//     for dynamic widths. A nonce-based scheme would require touching every
//     swap and is excessive for a local-only single-user app.
//
// The local-only deployment (127.0.0.1:5050, Host/Origin allowlists, no
// third-party scripts loadable due to script-src 'self') makes these
// relaxations acceptable.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-eval'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// rejectOptions short-circuits OPTIONS preflights with 403. Same-origin
// requests don't preflight, so any OPTIONS arriving is cross-origin.
func rejectOptions(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			if logger != nil {
				logger.Warn("preflight rejected",
					"middleware", "rejectOptions",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"origin", r.Header.Get("Origin"))
			}
			http.Error(w, "preflight rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger emits one structured log line per request: method, path
// without query (the query may contain project names - fine - but never form
// bodies). Generates a short request ID for correlation.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		// Path only; query stripped to avoid logging project names twice and
		// to keep policy uniform with write-side logs.
		path := r.URL.Path
		logger.Info("http",
			"id", id,
			"method", r.Method,
			"path", path,
			"status", ww.status,
			"dur_ms", time.Since(start).Milliseconds())
	})
}

func newRequestID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// stack composes middleware right-to-left for readability:
//
//	stack(h, a, b, c)  =>  a(b(c(h)))
func stack(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
