package main

import (
	"path/filepath"
	"testing"
)

// TestLogDirFor exercises every OS/XDG branch in a single run. The
// production wrapper logDirForOS calls runtime.GOOS and os.Getenv, which a
// test on macOS could only ever exercise the "darwin" branch of - so the
// real logic lives in logDirFor (goos, home, xdg) where the inputs are
// injectable.
//
// Lock-step contract: these paths MUST match what scripts/install.sh
// creates (install_darwin: ~/Library/Logs/taskwarrior-web-portal; install_linux:
// ${XDG_STATE_HOME:-$HOME/.local/state}/taskwarrior-web-portal). If you change one
// side, change the other and update this test.
func TestLogDirFor(t *testing.T) {
	const home = "/home/franklin"
	cases := []struct {
		name string
		goos string
		xdg  string
		want string
	}{
		{
			name: "macOS: Apple-conventional Library/Logs",
			goos: "darwin",
			xdg:  "", // ignored on darwin
			want: filepath.Join(home, "Library", "Logs", "taskwarrior-web-portal"),
		},
		{
			name: "macOS ignores XDG_STATE_HOME (filesystem convention is Library/Logs)",
			goos: "darwin",
			xdg:  "/some/exotic/state",
			want: filepath.Join(home, "Library", "Logs", "taskwarrior-web-portal"),
		},
		{
			name: "Linux default: ~/.local/state",
			goos: "linux",
			xdg:  "",
			want: filepath.Join(home, ".local", "state", "taskwarrior-web-portal"),
		},
		{
			name: "Linux with XDG_STATE_HOME set: honour the env var",
			goos: "linux",
			xdg:  "/var/lib/franklin/state",
			want: filepath.Join("/var/lib/franklin/state", "taskwarrior-web-portal"),
		},
		{
			name: "FreeBSD/other unix follows the Linux/XDG path (not Library/Logs)",
			goos: "freebsd",
			xdg:  "",
			want: filepath.Join(home, ".local", "state", "taskwarrior-web-portal"),
		},
		{
			name: "Windows falls through to the XDG path (install.sh doesn't support Windows but the binary stays harmless)",
			goos: "windows",
			xdg:  "",
			want: filepath.Join(home, ".local", "state", "taskwarrior-web-portal"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := logDirFor(c.goos, home, c.xdg); got != c.want {
				t.Errorf("logDirFor(%q, %q, %q) = %q, want %q", c.goos, home, c.xdg, got, c.want)
			}
		})
	}
}
