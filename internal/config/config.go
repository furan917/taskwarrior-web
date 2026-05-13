// Package config centralises every tunable timeout, address, and derived
// allowlist for the taskwarrior-web-portal binary. Previously these lived as ad-hoc
// `const`s and `var`s scattered across server / handlers / tw, with the
// listen address duplicated in three places. One central package keeps the
// numbers findable and prevents the port from drifting between server.go,
// middleware allowlists, and the README.
//
// The `tw` package keeps its own internal timeout/cache constants so the
// dependency graph stays one-way (handlers -> server -> config; tw is a
// peer that nobody imports from config). Cross-link comments live next to
// each duplicate so a future renormalisation pass has a paper trail.
package config

import (
	"strings"
	"time"
)

// Addr is the explicit IPv4 loopback bind. Port 5050 is the default because
// 5000 is taken by macOS AirPlay Receiver and 8080/8081 are commonly used by
// other dev tools (Docker Desktop, generic local servers). tcp4 (vs tcp) is
// enforced at server start so dual-stack systems don't accidentally also
// listen on [::1].
const Addr = "127.0.0.1:5050"

// HTTP server-level timeouts. Read/Write/Idle bound the per-connection
// liveness; Shutdown caps how long the binary will wait for in-flight
// requests to drain after SIGTERM/SIGINT.
const (
	HTTPReadTimeout     = 10 * time.Second
	HTTPWriteTimeout    = 30 * time.Second
	HTTPIdleTimeout     = 60 * time.Second
	HTTPShutdownTimeout = 5 * time.Second
)

// Handler-level timeouts. These are sub-request budgets - a per-request
// context.WithTimeout wraps the handler before it shells out.
const (
	// BulkTimeout caps the total time a /tasks/bulk-* request can spend
	// shelling out to Taskwarrior across N task ids. Larger than the
	// per-task timeout because a 50-id bulk delete legitimately runs
	// through 50 sequential Run() calls.
	BulkTimeout = 30 * time.Second

	// ReportFilterTimeout caps how long the first-call `task _get
	// rc.report.<name>.filter` lookup is allowed to block. After the
	// first call the result is cached and returned instantly.
	ReportFilterTimeout = 5 * time.Second

	// ActiveContextTimeout caps the per-request `task _get rc.context`
	// shell. Cheap (~10-30ms) but bounded so a wedged binary can't stall
	// page rendering.
	ActiveContextTimeout = 2 * time.Second
)

// AllowedHosts returns the closed set of Host header values the server
// accepts. Derived from Addr so a port change here doesn't have to be
// mirrored in three places. Set form returned for direct map[string]bool
// construction by the middleware.
func AllowedHosts() []string {
	port := portOf(Addr)
	return []string{
		"localhost:" + port,
		"127.0.0.1:" + port,
	}
}

// AllowedOrigins returns the closed set of Origin headers accepted on
// state-changing requests, mirroring AllowedHosts but with the http://
// scheme prefix. CSRF middleware compares Origin verbatim against this
// set on POST/PUT/DELETE.
func AllowedOrigins() []string {
	port := portOf(Addr)
	return []string{
		"http://localhost:" + port,
		"http://127.0.0.1:" + port,
	}
}

// portOf extracts the port suffix from a host:port string.
func portOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}
