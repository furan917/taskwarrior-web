package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// freePort allocates a free TCP4 port on the loopback by opening then closing
// a listener and returning the now-free address. Brief race window between
// close and re-bind is acceptable for these tests.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	addr := freePort(t)
	srv, err := New(Config{
		Addr:   addr,
		TW:     tw.NewClient(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Static: nil,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestNew_RejectsBadAddr(t *testing.T) {
	_, err := New(Config{
		Addr:   "not-a-real-host:99999", // bad port
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err == nil {
		t.Errorf("expected error for invalid addr")
	}
}

func TestNew_BindsTCP4(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// listener.Addr() returns a *net.TCPAddr; for tcp4 binds the IP is a 4-byte
	// IPv4 address (To4() != nil). For tcp6/dual-stack listens the IP would be
	// the unspecified IPv6 ::, where To4() returns nil.
	tcpAddr, ok := srv.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type: got %T want *net.TCPAddr", srv.listener.Addr())
	}
	if tcpAddr.IP.To4() == nil {
		t.Errorf("expected IPv4 bind, got IP=%v", tcpAddr.IP)
	}
}

func TestNew_DefaultsAreFilledIn(t *testing.T) {
	addr := freePort(t)
	srv, err := New(Config{Addr: addr}) // no TW, no Logger, no Static
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()
	if srv.cfg.TW == nil {
		t.Errorf("TW not defaulted")
	}
	if srv.cfg.Logger == nil {
		t.Errorf("Logger not defaulted")
	}
}

// waitForServer polls a TCP connect until it succeeds (or deadline).
func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server never came up at %s", addr)
}

// requestWithHost issues a request to addr, but overrides the Host header to
// host (so the hostAllowlist sees a value it accepts even though we bound to
// an ephemeral port).
func requestWithHost(t *testing.T, method, addr, path, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+addr+path, nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

// E2E: spin up the full middleware stack and hit /healthz.
func TestServer_E2E_Healthz(t *testing.T) {
	srv := newTestServer(t)
	go func() { _ = srv.Serve() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	waitForServer(t, srv.cfg.Addr)

	resp := requestWithHost(t, http.MethodGet, srv.cfg.Addr, "/healthz", "127.0.0.1")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("body: got %q", string(body))
	}

	// Security headers set on /healthz too.
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing X-Content-Type-Options")
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("missing CSP")
	}
}

// E2E: bad Host gets 421.
func TestServer_E2E_BadHostRejected(t *testing.T) {
	srv := newTestServer(t)
	go func() { _ = srv.Serve() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	waitForServer(t, srv.cfg.Addr)

	resp := requestWithHost(t, http.MethodGet, srv.cfg.Addr, "/healthz", "evil.com")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("status: got %d want 421", resp.StatusCode)
	}
}

// E2E: OPTIONS gets 403 with allowed Host.
func TestServer_E2E_OptionsRejected(t *testing.T) {
	srv := newTestServer(t)
	go func() { _ = srv.Serve() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	waitForServer(t, srv.cfg.Addr)

	resp := requestWithHost(t, http.MethodOptions, srv.cfg.Addr, "/healthz", "127.0.0.1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status: got %d want 403", resp.StatusCode)
	}
}

func TestServer_Shutdown_Idempotent(t *testing.T) {
	srv := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("first shutdown: %v", err)
	}
	// Second shutdown should also be safe (returns ErrServerClosed silently).
	if err := srv.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
		t.Errorf("second shutdown: %v", err)
	}
}
