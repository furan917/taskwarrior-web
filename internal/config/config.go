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
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = "5050"
)

// Addr composes the bind address from TWP_BIND_HOST and TWP_BIND_PORT.
// Defaults to 127.0.0.1:5050 for desktop installs (local-only by design).
// Container deployments set TWP_BIND_HOST=0.0.0.0; Unraid users control
// the external port via Docker's port-mapping and TWP_BIND_PORT.
func Addr() string {
	return bindHost() + ":" + bindPort()
}

func bindHost() string {
	if v := os.Getenv("TWP_BIND_HOST"); v != "" {
		return v
	}
	return defaultHost
}

func bindPort() string {
	if v := os.Getenv("TWP_BIND_PORT"); v != "" {
		return v
	}
	return defaultPort
}

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
// accepts. Always includes localhost and 127.0.0.1 on the configured port.
// Set TWP_ALLOWED_HOSTS to a comma-separated list of bare hostnames or IPs
// (e.g. "192.168.1.10,myhostname") for deployments accessed via a LAN IP
// or custom hostname. The port is appended automatically from TWP_BIND_PORT.
func AllowedHosts() []string {
	port := bindPort()
	hosts := []string{
		"localhost:" + port,
		"127.0.0.1:" + port,
	}
	if v := os.Getenv("TWP_ALLOWED_HOSTS"); v != "" {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hosts = append(hosts, h+":"+port)
			}
		}
	}
	return hosts
}

// AllowedOrigins returns the closed set of Origin headers accepted on
// state-changing requests, mirroring AllowedHosts but with the http://
// scheme prefix. CSRF middleware compares Origin verbatim against this
// set on POST/PUT/DELETE.
func AllowedOrigins() []string {
	port := bindPort()
	origins := []string{
		"http://localhost:" + port,
		"http://127.0.0.1:" + port,
	}
	if v := os.Getenv("TWP_ALLOWED_HOSTS"); v != "" {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				origins = append(origins, "http://"+h+":"+port)
			}
		}
	}
	return origins
}

// Validate checks all env var overrides for correctness and returns a combined
// error if any are invalid. Call this early in main before starting the server.
func Validate() error {
	var errs []string

	if v := os.Getenv("TWP_BIND_PORT"); v != "" {
		if err := validPort(v, "TWP_BIND_PORT"); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if v := os.Getenv("TWP_ALLOWED_HOSTS"); v != "" {
		for _, h := range strings.Split(v, ",") {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			// Bare hostnames only — port comes from TWP_BIND_PORT.
			// A single colon means the user wrote host:port (old format).
			if strings.Count(h, ":") == 1 {
				errs = append(errs, "TWP_ALLOWED_HOSTS: "+h+": use a bare hostname or IP; port is set via TWP_BIND_PORT")
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func validPort(s, label string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s: %q is not a valid port (1-65535)", label, s)
	}
	return nil
}

// portOf extracts the port suffix from a host:port string.
func portOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}
