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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
	"github.com/furan917/taskwarrior-web-portal/internal/server"
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

// checkDataDir refuses to start if the Taskwarrior data directory is
// group/other-readable. Tasks contain sensitive material; a permissive mode
// would leak content to other local users.
func checkDataDir(logger *slog.Logger) error {
	dataDir := resolveDataDir()
	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("task data dir does not exist; the binary still runs but task commands will fail", "path", dataDir)
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

// resolveDataDir returns the effective Taskwarrior data directory. It asks
// `task _get rc.data.location` first so that TASKRC overrides (e.g. in Docker
// where data lives at /config/task rather than ~/.task) are respected.
// Falls back to ~/.task if task is not on PATH or the call fails.
func resolveDataDir() string {
	out, err := exec.Command("task", "_get", "rc.data.location").Output()
	if err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			if strings.HasPrefix(dir, "~/") {
				home, herr := os.UserHomeDir()
				if herr == nil {
					return filepath.Join(home, dir[2:])
				}
			}
			return dir
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".task"
	}
	return filepath.Join(home, ".task")
}

// logDirFor is the testable core of log-dir resolution: takes an explicit
// goos + home + xdg state-home string so a single test run can exercise
// every branch (otherwise tests can only see whichever GOOS they happened
// to run on). macOS uses ~/Library/Logs/<app> per Apple's filesystem
// conventions; everything else (Linux, BSDs) uses the XDG state-home path
// (~/.local/state/<app> by default), where systemd --user services
// conventionally write persistent state.
//
// Kept in lock-step with scripts/install.sh: the install script's mkdir
// + chmod 700 must hit the same directory the binary then opens, or the
// install summary lies about where logs actually land.
func logDirFor(goos, home, xdgStateHome string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Logs", "taskwarrior-web-portal")
	}
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "taskwarrior-web-portal")
	}
	return filepath.Join(home, ".local", "state", "taskwarrior-web-portal")
}

// logDirForOS is the production wrapper that snapshots the live runtime.GOOS
// and XDG_STATE_HOME env var. Kept as a thin shim so the binary's call
// sites stay terse; logDirFor is the unit-tested function.
func logDirForOS(home string) string {
	return logDirFor(runtime.GOOS, home, os.Getenv("XDG_STATE_HOME"))
}

// newLogWriter returns an io.Writer that fans slog output to both stdout
// (so `make run` shows logs in the foreground terminal AND the supervisor's
// captured stream catches anything in production) and a size-rotated
// app.log file under the OS-appropriate state directory. Rotation policy:
// 10 MB per file, 3 backups, 30 day max age, gzip-compressed.
//
// If the home dir or log dir can't be resolved/created we fall back to
// stdout-only - the app should still run; we just lose the rotated file.
func newLogWriter() (io.Writer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Stdout, fmt.Errorf("home dir: %w", err)
	}
	dir := logDirForOS(home)
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

	if err := config.Validate(); err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	if err := checkDataDir(logger); err != nil {
		logger.Error("data dir check failed", "err", err)
		os.Exit(1)
	}

	srv, err := server.New(server.Config{Logger: logger, Static: staticFS()})
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			logger.Error("port already in use",
				"addr", config.Addr(),
				"hint", "another taskwarrior-web-portal instance? `lsof -nP -iTCP:"+addrPort(config.Addr())+"`")
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
