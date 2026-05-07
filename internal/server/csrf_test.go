package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web/internal/server/handlers"
)

// noopHandler returns 200 OK with a fixed body. Used as the inner handler
// behind withCSRF so we can detect "request reached the handler".
func noopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestCSRF_GET_SetsCookieAndPropagatesToken(t *testing.T) {
	var got string
	h := withCSRF(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = handlers.CSRFToken(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("no cookie set on GET")
	}
	var c *http.Cookie
	for _, k := range cookies {
		if k.Name == csrfCookieName {
			c = k
		}
	}
	if c == nil {
		t.Fatalf("csrf cookie %q not set; got %v", csrfCookieName, cookies)
	}
	if !c.HttpOnly {
		t.Errorf("cookie HttpOnly false")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite: got %v want Strict", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("cookie Path: got %q want /", c.Path)
	}
	if len(c.Value) != csrfTokenBytes*2 {
		t.Errorf("cookie value len: got %d want %d", len(c.Value), csrfTokenBytes*2)
	}
	if got == "" {
		t.Errorf("CSRFToken in context is empty")
	}
	if got != c.Value {
		t.Errorf("context token != cookie value: %q vs %q", got, c.Value)
	}
}

func TestCSRF_GET_ReusesExistingValidCookie(t *testing.T) {
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" // 64 hex chars

	var seen string
	h := withCSRF(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = handlers.CSRFToken(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if seen != tok {
		t.Errorf("token not reused: got %q want %q", seen, tok)
	}
	// No new Set-Cookie should appear because we already had a valid one.
	for _, c := range rr.Result().Cookies() {
		if c.Name == csrfCookieName {
			t.Errorf("unexpected Set-Cookie: %v", c)
		}
	}
}

func TestCSRF_GET_RegeneratesIfCookieWrongLength(t *testing.T) {
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "short"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == csrfCookieName && len(c.Value) == csrfTokenBytes*2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fresh cookie when stored value too short")
	}
}

func TestCSRF_POST_RejectsMissingCookie(t *testing.T) {
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rr.Code)
	}
}

func TestCSRF_POST_RejectsCookieWithoutHeader(t *testing.T) {
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rr.Code)
	}
}

func TestCSRF_POST_RejectsHeaderMismatch(t *testing.T) {
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	const bad = "0000000000000000000000000000000000000000000000000000000000000000"
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	req.Header.Set(csrfHeaderName, bad)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rr.Code)
	}
}

func TestCSRF_POST_AcceptsHeaderMatch(t *testing.T) {
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	hits := 0
	h := withCSRF(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	req.Header.Set(csrfHeaderName, tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if hits != 1 {
		t.Errorf("handler hits: got %d want 1", hits)
	}
}

func TestCSRF_POST_AcceptsFormFieldMatch(t *testing.T) {
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	h := withCSRF(nil, noopHandler())
	body := strings.NewReader(csrfFormField + "=" + tok)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCSRF_POST_RejectsSuppliedDifferentLength(t *testing.T) {
	// Length-mismatch path returns false before constant-time compare is reached.
	const tok = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tok})
	req.Header.Set(csrfHeaderName, "short")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rr.Code)
	}
}

func TestCSRF_DELETE_AlsoValidates(t *testing.T) {
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodDelete, "/tasks/1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rr.Code)
	}
}

func TestCSRF_HEAD_BehavesLikeGET(t *testing.T) {
	h := withCSRF(nil, noopHandler())
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", rr.Code)
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == csrfCookieName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cookie set on HEAD")
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	a, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(a) != csrfTokenBytes*2 {
		t.Errorf("len: got %d want %d", len(a), csrfTokenBytes*2)
	}
	b, _ := generateCSRFToken()
	if a == b {
		t.Errorf("expected unique tokens, got duplicate %q", a)
	}
}
