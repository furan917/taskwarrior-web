package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// DefaultAddr is re-exported from config so the existing main.go default
// path keeps working. New callers should reference config.Addr() directly.
var DefaultAddr = config.Addr()

type Config struct {
	Addr   string
	TW     *tw.Client
	Logger *slog.Logger
	Static fs.FS // embedded static assets (htmx.min.js, app.css)
}

type Server struct {
	cfg      Config
	httpSrv  *http.Server
	listener net.Listener
}

// New constructs a Server bound explicitly to tcp4 (avoids accidental dual-stack
// listening on ::1). The listener is opened eagerly so EADDRINUSE surfaces
// during startup, not deferred until the first request.
func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}
	if cfg.TW == nil {
		cfg.TW = tw.NewClient()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	mux := http.NewServeMux()
	registerRoutes(mux, cfg)

	// Outermost-first: logger -> reject-options -> host -> origin -> security headers -> mux.
	// CSRF is wrapped inside registerRoutes, scoped to /app routes only.
	logger := cfg.Logger
	handler := stack(mux,
		func(h http.Handler) http.Handler { return requestLogger(logger, h) },
		func(h http.Handler) http.Handler { return rejectOptions(logger, h) },
		func(h http.Handler) http.Handler { return hostAllowlist(logger, h) },
		func(h http.Handler) http.Handler { return originAllowlist(logger, h) },
		securityHeaders,
	)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       config.HTTPReadTimeout,
		WriteTimeout:      config.HTTPWriteTimeout,
		IdleTimeout:       config.HTTPIdleTimeout,
	}

	ln, err := net.Listen("tcp4", cfg.Addr)
	if err != nil {
		return nil, err
	}

	return &Server{cfg: cfg, httpSrv: httpSrv, listener: ln}, nil
}

func (s *Server) Serve() error {
	s.cfg.Logger.Info("server listening", "addr", s.cfg.Addr)
	if err := s.httpSrv.Serve(s.listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.cfg.Logger.Info("server shutting down")
	return s.httpSrv.Shutdown(ctx)
}
