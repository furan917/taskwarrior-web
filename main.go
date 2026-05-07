package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/furan917/taskwarrior-web/internal/config"
	"github.com/furan917/taskwarrior-web/internal/server"
)

// addrPort returns the trailing port from a "host:port" string. Used only
// to format the EADDRINUSE hint message; defensive helper because the
// addr is now a config-derived const but the hint must echo the actual
// port the user is fighting over.
func addrPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

//go:embed web/static
var embeddedAssets embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(embeddedAssets, "web/static")
	if err != nil {
		panic(err)
	}
	return sub
}

// checkDataDir refuses to start if ~/.task is group/other-readable. Tasks
// contain sensitive material (offboarding, hiring, board); a permissive mode
// would leak content to other local users.
func checkDataDir(logger *slog.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dataDir := filepath.Join(home, ".task")
	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("~/.task does not exist; the binary still runs but every Export will fail", "path", dataDir)
			return nil
		}
		return err
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf("%s mode %o is group/other readable; run `chmod 700 %s`", dataDir, mode, dataDir)
	}
	logger.Info("data dir ok", "path", dataDir, "mode", fmt.Sprintf("%o", mode))
	return nil
}

// newLogWriter returns an io.Writer that fans slog output to both stdout
// (so `make run` shows logs in the foreground terminal AND launchd's
// StandardOutPath catches anything in production) and a size-rotated
// app.log file under ~/Library/Logs/taskwarrior-web. Rotation policy:
// 10 MB per file, 3 backups, 30 day max age, gzip-compressed.
//
// If the home dir or log dir can't be resolved/created we fall back to
// stdout-only - the app should still run; we just lose the rotated file.
func newLogWriter() (io.Writer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Stdout, fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, "Library", "Logs", "taskwarrior-web")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return os.Stdout, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(dir, "app.log"),
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     30, // days
		Compress:   true,
	}
	return io.MultiWriter(os.Stdout, logFile), nil
}

func main() {
	logWriter, logErr := newLogWriter()
	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if logErr != nil {
		// Non-fatal: the multiwriter fell back to stdout-only.
		logger.Warn("rotated log unavailable, falling back to stdout only", "err", logErr)
	}

	if err := checkDataDir(logger); err != nil {
		logger.Error("data dir check failed", "err", err)
		os.Exit(1)
	}

	srv, err := server.New(server.Config{Logger: logger, Static: staticFS()})
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			logger.Error("port already in use",
				"addr", config.Addr,
				"hint", "another taskwarrior-web instance? `lsof -nP -iTCP:"+addrPort(config.Addr)+"`")
			os.Exit(1)
		}
		logger.Error("server init failed", "err", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.Info("signal received", "signal", sig)
	case err := <-errCh:
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.HTTPShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
}
