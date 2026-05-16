package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTaskOpts configures the unified fake-task script builder. The zero
// value is a valid "exit 0, no output" stub. Each field controls one branch
// of the generated `task` script:
//
//   - ExportJSON: returned for any invocation containing "export". Empty
//     means no JSON branch is emitted (caller doesn't care about export).
//   - Projects, Tags: emitted for `_projects` / `_tags` lookups.
//   - UDAs: list of declared UDAs; emitted for `_udas` and per-name
//     `_get rc.uda.<name>.type` / `.label` lookups.
//   - RecordArgv: when true, every invocation appends its argv to a
//     per-invocation file under <tempdir>/argv. The directory path is
//     returned to the caller.
//   - ExitCode: terminal exit code for any invocation that doesn't match a
//     branch above. Zero is the "succeed silently" default.
type fakeTaskOpts struct {
	ExportJSON string
	Projects   []string
	Tags       []string
	UDAs       []fakeUDA
	// ActiveContext is the value emitted for `task _context`. Empty string =
	// no active context, which is the implicit default when the field is
	// not set.
	ActiveContext string
	// Contexts is the table emitted for `task contexts`. Empty list means
	// "No contexts defined." is emitted instead, mirroring real Taskwarrior.
	Contexts   []fakeContext
	RecordArgv bool
	ExitCode   int
	// SyncOutput is the text printed by `task sync`. SyncExitCode controls
	// whether the sync command succeeds (0) or fails.
	SyncOutput   string
	SyncExitCode int
	// JournalTimeRC is returned for `task _get rc.journal.time`. Set to "yes"
	// to simulate journal.time being enabled; empty string (default) = disabled.
	JournalTimeRC string
}

type fakeContext struct {
	Name        string
	ReadFilter  string
	WriteFilter string
	Active      bool
}

type fakeUDA struct {
	Name, Type, Label string
}

// installFakeTaskWith builds a single shell script that handles every shape
// of fake-task call the handler tests need. Returns the argv-log directory
// when RecordArgv is set; otherwise the empty string.
//
// The script is written to a fresh t.TempDir() and that directory is
// prepended to PATH for the test's lifetime via t.Setenv.
func installFakeTaskWith(t *testing.T, opts fakeTaskOpts) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "task")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")

	logDir := ""
	if opts.RecordArgv {
		logDir = filepath.Join(dir, "argv")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("mkdir argv: %v", err)
		}
		b.WriteString(`seq=$(ls "` + logDir + `" 2>/dev/null | wc -l | tr -d ' ')
out="` + logDir + `/$(printf %03d $seq)"
: > "$out"
for a in "$@"; do
  printf "%s\n" "$a" >> "$out"
done
`)
	}

	// Discovery branches go through a `case "$*"` block. Any non-matching
	// invocation falls through to the export / fallback handling below.
	// `_context` and `contexts` always emit a (possibly empty) response so
	// the per-request lookup never falls through to the export path - this
	// keeps the test free to set ExportJSON without accidentally feeding the
	// JSON to the active-context call.
	b.WriteString(`case "$*" in` + "\n")
	b.WriteString(`  *"_get rc.context"*) printf '` + opts.ActiveContext + `'; exit 0;;` + "\n")
	b.WriteString(`  *"_get rc.journal.time"*) printf '` + opts.JournalTimeRC + `'; exit 0;;` + "\n")
	if len(opts.Contexts) > 0 {
		var table strings.Builder
		table.WriteString("Name  Type   Filter   Active\\n")
		table.WriteString("----- ------ -------- --------\\n")
		for _, c := range opts.Contexts {
			active := "no"
			if c.Active {
				active = "yes"
			}
			rf := c.ReadFilter
			if rf == "" {
				rf = "+" + c.Name
			}
			table.WriteString(c.Name + " read   " + rf + " " + active + "\\n")
			if c.WriteFilter != "" {
				table.WriteString(c.Name + " write  " + c.WriteFilter + " no\\n")
			}
		}
		b.WriteString(`  *"context list"*) printf '` + table.String() + `'; exit 0;;` + "\n")
	} else {
		b.WriteString(`  *"context list"*) printf 'No contexts defined.\n'; exit 0;;` + "\n")
	}
	if len(opts.Projects) > 0 {
		b.WriteString(`  *"_projects"*) printf '` + strings.Join(opts.Projects, "\n") + `\n'; exit 0;;` + "\n")
	}
	if len(opts.Tags) > 0 {
		b.WriteString(`  *"_tags"*) printf '` + strings.Join(opts.Tags, "\n") + `\n'; exit 0;;` + "\n")
	}
	if len(opts.UDAs) > 0 {
		udaNames := ""
		for _, u := range opts.UDAs {
			udaNames += u.Name + "\n"
		}
		b.WriteString(`  *"_udas"*) printf '` + udaNames + `'; exit 0;;` + "\n")
		for _, u := range opts.UDAs {
			b.WriteString(`  *"_get rc.uda.` + u.Name + `.type"*) printf '` + u.Type + `'; exit 0;;` + "\n")
			b.WriteString(`  *"_get rc.uda.` + u.Name + `.label"*) printf '` + u.Label + `'; exit 0;;` + "\n")
		}
	}
	if opts.SyncOutput != "" || opts.SyncExitCode != 0 {
		b.WriteString(`  *"sync"*) printf '` + opts.SyncOutput + `'; exit ` + itoa(opts.SyncExitCode) + `;;` + "\n")
	}
	b.WriteString("esac\n")

	if opts.ExportJSON != "" {
		b.WriteString(`for a in "$@"; do
  if [ "$a" = "export" ]; then
    cat <<'JSON_EOF'
` + opts.ExportJSON + `
JSON_EOF
    exit 0
  fi
done
`)
	}

	if opts.ExitCode != 0 {
		b.WriteString("exit ")
		b.WriteString(itoa(opts.ExitCode))
		b.WriteString("\n")
	} else {
		b.WriteString("exit 0\n")
	}

	if err := os.WriteFile(script, []byte(b.String()), 0o755); err != nil {
		t.Fatalf("write fake task: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logDir
}

// itoa is a tiny strconv-free int->string helper so we don't pull strconv
// into a test-only file just for one digit.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
