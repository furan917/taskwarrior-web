package tw

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// argvRecorder returns a fake `task` script that writes each invocation's argv
// (one arg per line, blank line as separator) to logFile, then exits 0.
func argvRecorder(t *testing.T, logFile string) {
	t.Helper()
	script := `#!/bin/sh
for a in "$@"; do
  printf "%s\n" "$a" >> "` + logFile + `"
done
printf "\n" >> "` + logFile + `"
exit 0
`
	dir := t.TempDir()
	path := filepath.Join(dir, "task")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// readAllInvocations returns every recorded invocation from a log file written
// by argvRecorder. Each element is the full argv of one invocation.
func readAllInvocations(t *testing.T, logFile string) [][]string {
	t.Helper()
	data, _ := os.ReadFile(logFile)
	var all [][]string
	for _, block := range strings.Split(string(data), "\n\n") {
		var args []string
		for _, line := range strings.Split(block, "\n") {
			if line != "" {
				args = append(args, line)
			}
		}
		if len(args) > 0 {
			all = append(all, args)
		}
	}
	return all
}

func TestDefineContext_CallsCorrectArgv(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	if err := c.DefineContext(context.Background(), "work", "+work"); err != nil {
		t.Fatalf("DefineContext: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	if len(invocations) == 0 {
		t.Fatal("no invocations recorded")
	}
	// Find the invocation containing "define"
	found := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "define") {
			found = true
			if !strings.Contains(joined, "work") {
				t.Errorf("define invocation missing context name: %v", args)
			}
			if !strings.Contains(joined, "+work") {
				t.Errorf("define invocation missing filter: %v", args)
			}
		}
	}
	if !found {
		t.Errorf("no 'define' invocation found in: %v", invocations)
	}
}

func TestDefineContext_InvalidatesCache(t *testing.T) {
	dir := t.TempDir()
	callCount := 0
	// Script that counts calls and returns a context list.
	body := `#!/bin/sh
echo X >> "` + filepath.Join(dir, "calls") + `"
case "$*" in
  *"context list"*) printf 'Name  Type   Filter   Active\n----- ------ -------- ---\nwork  read   +work     yes\n'; exit 0;;
esac
exit 0
`
	scriptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(scriptDir, "task"), []byte(body), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	ctx := context.Background()

	// Warm the cache.
	_ = c.ContextsCached(ctx)
	data1, _ := os.ReadFile(filepath.Join(dir, "calls"))
	callCount = strings.Count(string(data1), "X")

	// DefineContext should invalidate; next ContextsCached must re-fetch.
	_ = c.DefineContext(ctx, "work", "+work")
	_ = c.ContextsCached(ctx)
	data2, _ := os.ReadFile(filepath.Join(dir, "calls"))
	callCountAfter := strings.Count(string(data2), "X")

	if callCountAfter <= callCount {
		t.Errorf("cache was not invalidated: invocations before=%d, after=%d", callCount, callCountAfter)
	}
}

func TestDefineContext_RejectsBadName(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	for _, bad := range []string{"", "a b", "a;b", "+evil", "../etc", "rc.foo=bar"} {
		if err := c.DefineContext(ctx, bad, "+work"); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

func TestDefineContext_RejectsRcOverrideInFilter(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	for _, bad := range []string{
		"rc.data.location=/tmp/evil",
		"+work rc.confirmation=no",
	} {
		if err := c.DefineContext(ctx, "work", bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("filter %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

func TestSetContextWriteFilter_CallsConfigKey(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	if err := c.SetContextWriteFilter(context.Background(), "work", "+work"); err != nil {
		t.Fatalf("SetContextWriteFilter: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	found := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "config") && strings.Contains(joined, "context.work.write") {
			found = true
			if !strings.Contains(joined, "+work") {
				t.Errorf("config invocation missing write filter value: %v", args)
			}
		}
	}
	if !found {
		t.Errorf("no 'config context.work.write' invocation found in: %v", invocations)
	}
}

func TestSetContextWriteFilter_RejectsBadName(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	for _, bad := range []string{"", "a b", "+evil"} {
		if err := c.SetContextWriteFilter(ctx, bad, "+work"); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

func TestSetContextWriteFilter_RejectsRcOverride(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	if err := c.SetContextWriteFilter(ctx, "work", "rc.confirmation=no"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for rc.* write filter, got %v", err)
	}
}

func TestDeleteContext_CallsCorrectArgv(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	if err := c.DeleteContext(context.Background(), "work"); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	found := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "context") && strings.Contains(joined, "delete") && strings.Contains(joined, "work") {
			found = true
		}
	}
	if !found {
		t.Errorf("no 'context delete work' invocation found in: %v", invocations)
	}
}

func TestDeleteContext_RejectsBadName(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	for _, bad := range []string{"", "a b", "+evil", "a;b"} {
		if err := c.DeleteContext(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

func TestRenameContext_SameNameSkipsDelete(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	// Same old and new name: should define (update filter) but NOT delete.
	if err := c.RenameContext(context.Background(), "work", "work", "+team", ""); err != nil {
		t.Fatalf("RenameContext: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	defineFound := false
	deleteFound := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "define") {
			defineFound = true
		}
		if strings.Contains(joined, "delete") {
			deleteFound = true
		}
	}
	if !defineFound {
		t.Error("expected 'define' invocation for same-name rename (filter update)")
	}
	if deleteFound {
		t.Error("unexpected 'delete' invocation for same-name rename")
	}
}

func TestRenameContext_DifferentNameDefinesAndDeletes(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	if err := c.RenameContext(context.Background(), "work", "office", "+office", ""); err != nil {
		t.Fatalf("RenameContext: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	defineFound := false
	deleteFound := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "define") && strings.Contains(joined, "office") {
			defineFound = true
		}
		if strings.Contains(joined, "delete") && strings.Contains(joined, "work") {
			deleteFound = true
		}
	}
	if !defineFound {
		t.Errorf("expected 'context define office' invocation; got: %v", invocations)
	}
	if !deleteFound {
		t.Errorf("expected 'context delete work' invocation; got: %v", invocations)
	}
}

func TestRenameContext_WithWriteFilterCallsConfig(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	argvRecorder(t, logFile)

	c := NewClient()
	if err := c.RenameContext(context.Background(), "work", "work", "+team", "+team"); err != nil {
		t.Fatalf("RenameContext: %v", err)
	}

	invocations := readAllInvocations(t, logFile)
	configFound := false
	for _, args := range invocations {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "config") && strings.Contains(joined, "context.work.write") {
			configFound = true
		}
	}
	if !configFound {
		t.Errorf("expected 'config context.work.write' invocation; got: %v", invocations)
	}
}
