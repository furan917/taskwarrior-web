package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
)

// allowedHosts contains bare hostnames (no port). Port is intentionally not
// checked: DNS-rebinding protection comes from verifying the hostname, not the
// port. Stripping the port also means Docker port mappings (where the external
// port differs from the container-internal bind port) work transparently.
var allowedHosts = func() map[string]bool {
	out := map[string]bool{}
	for _, h := range config.AllowedHosts() {
		if host, _, err := net.SplitHostPort(h); err == nil {
			out[host] = true
		} else {
			out[h] = true
		}
	}
	return out
}()

// allowedOrigins contains scheme+host without port, mirroring allowedHosts.
var allowedOrigins = func() map[string]bool {
	out := map[string]bool{}
	for _, o := range config.AllowedOrigins() {
		if u, err := url.Parse(o); err == nil {
			out[u.Scheme+"://"+u.Hostname()] = true
		} else {
			out[o] = true
		}
	}
	return out
}()

// hostAllowlist gates ALL requests on the hostname in the Host header.
// The port is stripped before the check — DNS-rebinding protection relies on
// the hostname, not the port, so stripping port is correct and also means
// Docker port mappings (external port ≠ container port) work transparently.
// 421 (Misdirected Request) is the spec-correct status for an unrecognised host.
func hostAllowlist(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if config.DisableHostCheck() {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !allowedHosts[host] {
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
// Port is stripped from the Origin header before checking, for the same reason
// as hostAllowlist: the hostname is what matters for CSRF protection.
func originAllowlist(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if config.DisableHostCheck() {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		rawOrigin := r.Header.Get("Origin")
		if rawOrigin == "" {
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
		// Strip port from Origin before checking — scheme://host is sufficient
		// for CSRF protection, and the port may differ under Docker port mapping.
		origin := rawOrigin
		if u, err := url.Parse(rawOrigin); err == nil {
			origin = u.Scheme + "://" + u.Hostname()
		}
		if !allowedOrigins[origin] {
			if logger != nil {
				logger.Warn("origin rejected",
					"middleware", "originAllowlist",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"origin", rawOrigin)
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
