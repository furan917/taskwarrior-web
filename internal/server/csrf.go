package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/furan917/taskwarrior-web-portal/internal/server/handlers"
)

const (
	csrfCookieName = "tw_csrf"
	csrfHeaderName = "X-CSRF-Token"
	csrfFormField  = "_csrf"
	csrfTokenBytes = 32
)

// generateCSRFToken returns 32 cryptographically random bytes hex-encoded.
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// withCSRF is the middleware:
//   - Safe methods (GET, HEAD, OPTIONS): ensure a token cookie exists, expose
//     the token to handlers via context for rendering into <meta>.
//   - Other methods: validate that the X-CSRF-Token header (or _csrf form
//     value) matches the cookie via constant-time compare; 403 on mismatch.
func withCSRF(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			token := readOrSetToken(w, r)
			next.ServeHTTP(w, r.WithContext(handlers.WithCSRFToken(r.Context(), token)))
		default:
			if !validateCSRF(r, logger) {
				if logger != nil {
					logger.Warn("csrf rejected",
						"method", r.Method,
						"path", r.URL.Path,
						"remote", r.RemoteAddr)
				}
				http.Error(w, "csrf failed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func readOrSetToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil && len(c.Value) == csrfTokenBytes*2 {
		return c.Value
	}
	token, err := generateCSRFToken()
	if err != nil {
		// Should never happen; rand.Read failure is catastrophic.
		http.Error(w, "csrf init failed", http.StatusInternalServerError)
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // localhost http; flip when TLS is added
	})
	return token
}

func validateCSRF(r *http.Request, logger *slog.Logger) bool {
	c, err := r.Cookie(csrfCookieName)
	if err != nil || len(c.Value) != csrfTokenBytes*2 {
		return false
	}
	supplied := r.Header.Get(csrfHeaderName)
	if supplied == "" {
		// HTMX always sends the header (configured in app.js); native form
		// posts use the hidden field. ParseForm errors (oversize body, bad
		// Content-Type) are logged so we'd see breakage rather than swallow it
		// silently; the empty supplied value still fails the compare below.
		if err := r.ParseForm(); err != nil && logger != nil {
			logger.Warn("csrf parseForm failed",
				"method", r.Method,
				"path", r.URL.Path,
				"err", err)
		}
		supplied = r.PostFormValue(csrfFormField)
	}
	if len(supplied) != len(c.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(c.Value)) == 1
}
