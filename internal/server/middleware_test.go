package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestHostAllowlist_AllowsKnownHosts(t *testing.T) {
	h := hostAllowlist(nil, okHandler())
	// Any port on an allowed hostname must pass — port is not part of the check.
	for _, host := range []string{"localhost", "localhost:5050", "localhost:5051", "localhost:9999", "127.0.0.1", "127.0.0.1:5050", "127.0.0.1:9999"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("host %q: got %d want 200", host, rr.Code)
		}
	}
}

func TestHostAllowlist_Rejects421(t *testing.T) {
	h := hostAllowlist(nil, okHandler())
	// Only hostname matters — any unrecognised hostname must be rejected
	// regardless of port.
	for _, host := range []string{"evil.com", "evil.com:5050", "0.0.0.0", "0.0.0.0:5050"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMisdirectedRequest {
			t.Errorf("host %q: got %d want 421", host, rr.Code)
		}
	}
}

func TestOriginAllowlist_AllowsGetWithoutOrigin(t *testing.T) {
	h := originAllowlist(nil, okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d want 200", rr.Code)
	}
}

func TestOriginAllowlist_RejectsPostMissingOrigin(t *testing.T) {
	h := originAllowlist(nil, okHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d want 403", rr.Code)
	}
}

func TestOriginAllowlist_RejectsBadOriginOnWrites(t *testing.T) {
	h := originAllowlist(nil, okHandler())
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		// Wrong hostname is rejected regardless of port. Wrong scheme (https vs http)
		// is also rejected. Port alone does not determine rejection.
		for _, origin := range []string{"http://evil.com", "https://localhost:5050", "https://localhost", "null"} {
			req := httptest.NewRequest(method, "/", nil)
			req.Header.Set("Origin", origin)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s %s: got %d want 403", method, origin, rr.Code)
			}
		}
	}
}

func TestOriginAllowlist_AcceptsAllowedOrigin(t *testing.T) {
	h := originAllowlist(nil, okHandler())
	// Any port on an allowed hostname+scheme must pass.
	for _, origin := range []string{"http://localhost:5050", "http://localhost:5051", "http://localhost:9999", "http://127.0.0.1:5050", "http://127.0.0.1:9999"} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Origin", origin)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("origin %q: got %d want 200", origin, rr.Code)
		}
	}
}

func TestRejectOptions(t *testing.T) {
	h := rejectOptions(nil, okHandler())
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("OPTIONS: got %d want 403", rr.Code)
	}
}

func TestRejectOptions_PassesOtherMethods(t *testing.T) {
	h := rejectOptions(nil, okHandler())
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: got %d want 200", m, rr.Code)
		}
	}
}

func TestSecurityHeaders_PresentOnAllResponses(t *testing.T) {
	h := securityHeaders(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	wantHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "", // just present
	}
	for k, v := range wantHeaders {
		got := rr.Result().Header.Get(k)
		if got == "" {
			t.Errorf("header %q missing", k)
		}
		if v != "" && got != v {
			t.Errorf("header %q: got %q want %q", k, got, v)
		}
	}

	csp := rr.Result().Header.Get("Content-Security-Policy")
	for _, must := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'"} {
		if !contains(csp, must) {
			t.Errorf("CSP missing %q: %s", must, csp)
		}
	}
}

func TestRequestLogger_LogsRequestID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	called := false
	h := requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Errorf("inner handler not called")
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("status: got %d want 418", rr.Code)
	}
}

func TestStatusRecorder_CapturesFirstStatusOnly(t *testing.T) {
	inner := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	sr.WriteHeader(http.StatusBadRequest)
	sr.WriteHeader(http.StatusInternalServerError) // ignored - only first wins
	if sr.status != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", sr.status)
	}
}

func TestStack_AppliesMiddlewareLeftToRight(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := stack(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "inner")
	}), mw("a"), mw("b"), mw("c"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	want := []string{"a", "b", "c", "inner"}
	if len(order) != len(want) {
		t.Fatalf("len: got %v want %v", order, want)
	}
	for i := range order {
		if order[i] != want[i] {
			t.Errorf("order[%d]: got %q want %q", i, order[i], want[i])
		}
	}
}

func TestNewRequestID(t *testing.T) {
	a := newRequestID()
	b := newRequestID()
	if a == "" || b == "" {
		t.Fatal("empty")
	}
	if a == b {
		t.Errorf("dup: %q", a)
	}
	if len(a) != 8 { // 4 random bytes hex-encoded
		t.Errorf("len: got %d want 8", len(a))
	}
}

func TestHostAllowlist_DisabledCheck_AllowsAnyHost(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "1")
	h := hostAllowlist(nil, okHandler())
	for _, host := range []string{"evil.com", "0.0.0.0", "192.168.1.100", "anything"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("host %q: got %d want 200 when check disabled", host, rr.Code)
		}
	}
}

func TestOriginAllowlist_DisabledCheck_AllowsPostWithoutOrigin(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "1")
	h := originAllowlist(nil, okHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d want 200 for POST without Origin when check disabled", rr.Code)
	}
}

func TestOriginAllowlist_DisabledCheck_AllowsAnyOrigin(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "1")
	h := originAllowlist(nil, okHandler())
	for _, origin := range []string{"http://evil.com", "null", "https://attacker.example"} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Origin", origin)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("origin %q: got %d want 200 when check disabled", origin, rr.Code)
		}
	}
}

// contains is a tiny helper to dodge an import.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
