package tw

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClient_GuardArgs(t *testing.T) {
	c := NewClient()
	err := c.Run(context.Background(), "rc.data.location=/tmp/evil")
	if !errors.Is(err, ErrUnsafeArg) {
		t.Fatalf("expected ErrUnsafeArg, got %v", err)
	}
	_, err = c.Export(context.Background(), "rc.something=x")
	if !errors.Is(err, ErrUnsafeArg) {
		t.Fatalf("expected ErrUnsafeArg from Export, got %v", err)
	}
}

// Smoke test against the host's real `task` binary. Skipped if `task` is not
// on PATH or if there's no data - the goal is to confirm Export round-trips
// real Taskwarrior 3 output through our struct without losing fields.
func TestClient_ExportSmoke(t *testing.T) {
	c := NewClient()
	tasks, err := c.Export(context.Background(), "limit:3")
	if err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}
	if len(tasks) == 0 {
		t.Skip("no tasks in test env")
	}
	for i, tk := range tasks {
		if tk.UUID == "" {
			t.Errorf("task[%d] missing UUID: %+v", i, tk)
		}
		if tk.Description == "" {
			t.Errorf("task[%d] missing description: %+v", i, tk)
		}
	}
}

// TestClient_ResolveReportFilter_RejectsBadName ensures the validator catches
// shell metacharacters and rc.* injection attempts in the report name.
func TestClient_ResolveReportFilter_RejectsBadName(t *testing.T) {
	c := NewClient()
	for _, bad := range []string{"", "agenda;ls", "agenda rc.foo=bar", "../etc", "a b"} {
		_, err := c.ResolveReportFilter(context.Background(), bad)
		if !errors.Is(err, ErrUnsafeArg) {
			t.Errorf("name %q: expected ErrUnsafeArg, got %v", bad, err)
		}
	}
}

// Smoke test against the host's real `task` binary. Skipped if `task` is not
// on PATH. Confirms that `task _get rc.report.agenda.filter` works through
// runRaw without being rejected by guardArgs.
func TestClient_ResolveReportFilterSmoke(t *testing.T) {
	c := NewClient()
	got, err := c.ResolveReportFilter(context.Background(), "agenda")
	if err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}
	// got may legitimately be "" if .taskrc has no report.agenda.filter.
	// We just want to confirm no error and that the call routes through the
	// trusted-caller path. A non-empty result is logged for visibility.
	t.Logf("resolved agenda filter: %q", got)
}

func TestClient_AnnotateRejectsEmpty(t *testing.T) {
	c := NewClient()
	for _, text := range []string{"", "   ", "\t\n"} {
		err := c.Annotate(context.Background(), "1", text)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("annotate text %q: expected ErrInvalid, got %v", text, err)
		}
	}
}

func TestClient_DenotateRejectsEmpty(t *testing.T) {
	c := NewClient()
	for _, text := range []string{"", "   ", "\t\n"} {
		err := c.Denotate(context.Background(), "1", text)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("denotate text %q: expected ErrInvalid, got %v", text, err)
		}
	}
}

// TestClient_StartStopDuplicate_RejectsBadID covers the input-validation
// guards on the three new control methods. They all share the same shape
// (validate id, then Run), so one table-test exercises every guard.
func TestClient_StartStopDuplicate_RejectsBadID(t *testing.T) {
	c := NewClient()
	for _, id := range []string{"", "abc", "1; ls", "../etc", "1 2"} {
		if err := c.Start(context.Background(), id); !errors.Is(err, ErrInvalid) {
			t.Errorf("Start(%q): expected ErrInvalid, got %v", id, err)
		}
		if err := c.Stop(context.Background(), id); !errors.Is(err, ErrInvalid) {
			t.Errorf("Stop(%q): expected ErrInvalid, got %v", id, err)
		}
		if err := c.Duplicate(context.Background(), id); !errors.Is(err, ErrInvalid) {
			t.Errorf("Duplicate(%q): expected ErrInvalid, got %v", id, err)
		}
	}
}

// TestClient_ListReports_FakeBinary covers the discovery round-trip.
// Mirrors TestClient_ListProjects_FakeBinary - drops names that fail
// the allowlist regex (shell metas, leading dashes), dedupes, sorts.
func TestClient_ListReports_FakeBinary(t *testing.T) {
	body := `#!/bin/sh
case "$*" in
  *"_reports"*)
    printf 'next\nready\nlatest\nready\nbad name\n+evil\n../etc\noldest\n'
    ;;
  *)
    printf ''
    ;;
esac
exit 0
`
	installScript(t, body)
	c := NewClient()
	got, err := c.ListReports(context.Background())
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	want := []string{"latest", "next", "oldest", "ready"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestClient_AnnotateRejectsBadID(t *testing.T) {
	c := NewClient()
	for _, id := range []string{"", "abc", "1; ls", "../etc", "1 2"} {
		err := c.Annotate(context.Background(), id, "note")
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("annotate id %q: expected ErrInvalid, got %v", id, err)
		}
		err = c.Denotate(context.Background(), id, "note")
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("denotate id %q: expected ErrInvalid, got %v", id, err)
		}
	}
}

// Smoke test against the host's real `task` binary. Creates a throwaway task,
// annotates it, exports to confirm the annotation is present, denotates,
// confirms removal, then deletes and purges the task. Skipped if `task` is
// unavailable.
func TestClient_AnnotateDenotateSmoke(t *testing.T) {
	c := NewClient()
	ctx := context.Background()

	// Create a tagged throwaway task so we can find its UUID.
	const marker = "twb-annot-smoke-DELETE-ME"
	if err := c.Run(ctx, "add", "description:\""+marker+"\""); err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}

	tasks, err := c.Export(ctx, "description.is:"+marker)
	if err != nil || len(tasks) == 0 {
		t.Skipf("could not locate seeded task (skip): err=%v len=%d", err, len(tasks))
	}
	uuid := tasks[0].UUID

	// Cleanup, no matter what happens.
	defer func() {
		_ = c.Run(ctx, uuid, "delete")
		_ = c.Run(ctx, uuid, "purge")
	}()

	// Annotate text containing rc.* would be rejected by guardArgs as defence
	// in depth - that's by design. Use a plainer note that still includes
	// DOM-modifier-looking tokens (+tag) to confirm `--` keeps them literal.
	const safeNote = "smoke note with +tag in it"
	if err := c.Annotate(ctx, uuid, safeNote); err != nil {
		t.Fatalf("annotate failed: %v", err)
	}

	got, err := c.Export(ctx, uuid)
	if err != nil || len(got) == 0 {
		t.Fatalf("re-export after annotate: err=%v len=%d", err, len(got))
	}
	found := false
	for _, a := range got[0].Annotations {
		if a.Description == safeNote {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("annotation not present after Annotate: %+v", got[0].Annotations)
	}

	if err := c.Denotate(ctx, uuid, safeNote); err != nil {
		t.Fatalf("denotate failed: %v", err)
	}
	got, err = c.Export(ctx, uuid)
	if err != nil || len(got) == 0 {
		t.Fatalf("re-export after denotate: err=%v len=%d", err, len(got))
	}
	for _, a := range got[0].Annotations {
		if a.Description == safeNote {
			t.Errorf("annotation still present after Denotate: %+v", got[0].Annotations)
		}
	}
}

// TestClient_ListUDAsSmoke against the host's real `task` binary. Skipped if
// the binary is unavailable. The user's actual UDA configuration is whatever
// it is; we only assert the call shape (no error, every returned name is
// valid). A non-empty result is logged for visibility.
func TestClient_ListUDAsSmoke(t *testing.T) {
	c := NewClient()
	udas, err := c.ListUDAs(context.Background())
	if err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}
	for _, u := range udas {
		if !UDANamePattern.MatchString(u.Name) {
			t.Errorf("ListUDAs returned an invalid name %q", u.Name)
		}
	}
	t.Logf("ListUDAs returned %d entries", len(udas))
}

// TestClient_ListUDAs_FakeBinary stubs `task` with a script that:
//   - on `_udas` emits two valid names plus one with a leading digit (must be
//     rejected) and one obviously hostile entry;
//   - on `_get rc.uda.estimate.type` emits "duration", `.label` emits
//     "Estimate";
//   - on `_get rc.uda.client.type` emits "string", `.label` emits "" (no
//     label, fall back to name in the UI).
func TestClient_ListUDAs_FakeBinary(t *testing.T) {
	body := `#!/bin/sh
case "$*" in
  *"_udas"*)
    printf 'estimate\nclient\n1bad\n+evil\n'
    ;;
  *"_get rc.uda.estimate.type"*)
    printf 'duration'
    ;;
  *"_get rc.uda.estimate.label"*)
    printf 'Estimate'
    ;;
  *"_get rc.uda.client.type"*)
    printf 'string'
    ;;
  *"_get rc.uda.client.label"*)
    printf ''
    ;;
  *)
    printf ''
    ;;
esac
exit 0
`
	installScript(t, body)
	c := NewClient()
	udas, err := c.ListUDAs(context.Background())
	if err != nil {
		t.Fatalf("ListUDAs: %v", err)
	}
	if len(udas) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(udas), udas)
	}
	if udas[0].Name != "estimate" || udas[0].Type != "duration" || udas[0].Label != "Estimate" {
		t.Errorf("estimate: %+v", udas[0])
	}
	if udas[1].Name != "client" || udas[1].Type != "string" || udas[1].Label != "" {
		t.Errorf("client: %+v", udas[1])
	}
}

// TestClient_UDAsCached_OnceOnly confirms ListUDAs is invoked at most once
// per Client lifetime. The fake script appends to a file each invocation, so
// multiple UDAsCached calls should leave a one-line trace if the cache is
// working.
func TestClient_UDAsCached_OnceOnly(t *testing.T) {
	dir := t.TempDir()
	counterFile := dir + "/calls"
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
echo X >> ` + counterFile + `
case "$*" in
  *"_udas"*)
    printf 'foo\n'
    ;;
  *"_get rc.uda.foo.type"*)
    printf 'string'
    ;;
  *"_get rc.uda.foo.label"*)
    printf 'Foo'
    ;;
  *)
    printf ''
    ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	for i := 0; i < 5; i++ {
		got := c.UDAsCached(context.Background())
		if len(got) != 1 || got[0].Name != "foo" {
			t.Fatalf("UDAsCached call %d: %+v", i, got)
		}
	}

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	// First UDAsCached -> 1 _udas call + 1 .type + 1 .label + 1 .values = 4
	// task invocations. Subsequent UDAsCached calls -> 0 (cached).
	gotCalls := strings.Count(string(data), "X")
	if gotCalls != 4 {
		t.Errorf("expected 4 task invocations across all UDAsCached calls, got %d", gotCalls)
	}
}

// installScript drops a fake `task` script onto the test PATH and returns
// the directory it lives in. Used to test Client behaviour against a
// controlled subprocess without invoking the real binary.
func installScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "task")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// TestClient_RunRaw_OutputCap simulates a `task` binary that writes far more
// than maxOutputBytes. The Client should NOT block forever and the recorded
// output must be at most maxOutputBytes.
//
// The test caps the script's output at 1 MB to keep the test cheap; the
// LimitReader still applies. We assert: output is bounded and no error
// surfaces purely because the binary kept writing.
func TestClient_RunRaw_OutputBounded(t *testing.T) {
	// 1 MB of zeroes is fine - Export will fail to JSON-decode them, which is
	// expected. We're testing the raw bound; checking that the Export path
	// returns *some* error is enough to confirm the read was bounded and the
	// process did not hang.
	installScript(t, "#!/bin/sh\nhead -c 1048576 /dev/zero\n")
	c := NewClient()
	_, err := c.Export(context.Background(), "x")
	if err == nil {
		t.Errorf("expected error decoding non-JSON output")
	}
}

// TestClient_Run_StderrSuppressed: stderr from the subprocess must not surface
// in the returned error string (could leak task descriptions). We have stderr
// echo "leaked sensitive content" but only a generic exit-status message must
// come back.
func TestClient_Run_StderrSuppressed(t *testing.T) {
	installScript(t, "#!/bin/sh\necho 'task description with secret content' >&2\nexit 1\n")
	c := NewClient()
	err := c.Run(context.Background(), "modify")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret content") {
		t.Errorf("stderr leaked into error: %v", err)
	}
}

// TestClient_RunRaw_Timeout: the Client wraps every call in a 10 s timeout.
// We override timeout via the unexported field by calling Export with a
// pre-cancelled parent context - the runRaw call should return promptly with
// an error rather than blocking on the subprocess.
func TestClient_RunRaw_TimeoutCancelled(t *testing.T) {
	installScript(t, "#!/bin/sh\nsleep 30\n")
	c := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	_, err := c.Export(ctx, "x")
	if err == nil {
		t.Errorf("expected error from cancelled context")
	}
}

// TestClient_RunRaw_BinaryMissing: if the host has no `task` binary on PATH,
// Run/Export should return an error rather than panic.
func TestClient_RunRaw_BinaryMissing(t *testing.T) {
	// Empty PATH so exec.LookPath fails.
	t.Setenv("PATH", "")
	c := NewClient()
	err := c.Run(context.Background(), "info")
	if err == nil {
		t.Errorf("expected error when binary unavailable")
	}
}

// TestClient_AddInjectionEndToEnd uses the real `task` binary to verify that
// a description containing DOM-modifier-like tokens round-trips back from
// `task export` as the literal string (no fake +tag, project, due, or rc.*
// applied). Skipped if `task` is unavailable.
func TestClient_AddInjectionEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("task binary unavailable")
	}
	c := NewClient()
	ctx := context.Background()

	const marker = "twb-injection-DELETE-ME"
	const evil = marker + " +faketag due:tomorrow project:evil rc.data.location=/tmp/x"
	in := AddInput{Description: evil}
	args, err := in.AddArgs()
	if err != nil {
		t.Fatalf("AddArgs: %v", err)
	}
	if err := c.Run(ctx, append([]string{"add"}, args...)...); err != nil {
		t.Skipf("task add failed (skip): %v", err)
	}

	tasks, err := c.Export(ctx, "description.contains:"+marker)
	if err != nil || len(tasks) == 0 {
		t.Skipf("could not locate seeded task (skip): err=%v len=%d", err, len(tasks))
	}
	tk := tasks[0]
	uuid := tk.UUID
	t.Cleanup(func() {
		_ = c.Run(ctx, uuid, "delete")
		_ = c.Run(ctx, uuid, "purge")
	})

	if tk.Description != evil {
		t.Errorf("description mutated by Taskwarrior: got %q want literal %q", tk.Description, evil)
	}
	for _, tag := range tk.Tags {
		if tag == "faketag" {
			t.Errorf("DOM-modifier +faketag was applied as a real tag: %v", tk.Tags)
		}
	}
	if tk.Project == "evil" {
		t.Errorf("DOM-modifier project:evil was applied as a real project")
	}
	if tk.Due != "" {
		t.Errorf("DOM-modifier due:tomorrow leaked into Due field: %q", tk.Due)
	}
}

// TestClient_Undo_FakeBinary records the argv passed to a stub `task` binary
// and confirms the Undo method invokes it with `undo` as the sole positional
// arg after the safetyArgs prefix. This is the deterministic, hermetic
// counterpart to a live-binary smoke test.
func TestClient_Undo_FakeBinary(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "task")
	body := `#!/bin/sh
: > "` + logFile + `"
for a in "$@"; do
  printf "%s\n" "$a" >> "` + logFile + `"
done
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	if err := c.Undo(context.Background()); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var logged []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			logged = append(logged, l)
		}
	}

	// safetyArgs (rc.confirmation=no, rc.recurrence.confirmation=no,
	// rc.json.array=on) plus the single positional `undo`.
	if len(logged) != len(safetyArgs)+1 {
		t.Fatalf("argv: got %d entries want %d: %v", len(logged), len(safetyArgs)+1, logged)
	}
	for i, want := range safetyArgs {
		if logged[i] != want {
			t.Errorf("argv[%d]: got %q want %q", i, logged[i], want)
		}
	}
	if logged[len(logged)-1] != "undo" {
		t.Errorf("expected trailing 'undo', got %q (full argv: %v)", logged[len(logged)-1], logged)
	}
}

// TestClient_Undo_PropagatesError confirms that a non-zero exit from the
// `task undo` subprocess surfaces as an error from Undo (so the handler can
// return 500). Uses a fake binary that exits 1.
func TestClient_Undo_PropagatesError(t *testing.T) {
	installScript(t, "#!/bin/sh\nexit 1\n")
	c := NewClient()
	if err := c.Undo(context.Background()); err == nil {
		t.Errorf("expected error from failing task undo")
	}
}

// TestClient_ListProjectsSmoke against the host's real `task` binary. Skipped
// if the binary is unavailable. The user's actual project set is whatever it
// is; we only assert the call shape (no error, every returned name passes the
// validator).
func TestClient_ListProjectsSmoke(t *testing.T) {
	c := NewClient()
	got, err := c.ListProjects(context.Background())
	if err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}
	for _, p := range got {
		if !projectListPattern.MatchString(p) {
			t.Errorf("ListProjects returned an invalid name %q", p)
		}
	}
	t.Logf("ListProjects returned %d entries", len(got))
}

// TestClient_ListTagsSmoke against the host's real `task` binary. Skipped if
// the binary is unavailable.
func TestClient_ListTagsSmoke(t *testing.T) {
	c := NewClient()
	got, err := c.ListTags(context.Background())
	if err != nil {
		t.Skipf("task binary unavailable or errored (skip): %v", err)
	}
	for _, tag := range got {
		if !tagListPattern.MatchString(tag) {
			t.Errorf("ListTags returned an invalid name %q", tag)
		}
	}
	t.Logf("ListTags returned %d entries", len(got))
}

// TestClient_ListProjects_FakeBinary stubs `task` with a script that emits a
// mix of valid project names (some duplicated, some empty lines, some hostile
// entries with shell metacharacters). The Client should dedupe + sort + drop
// invalid entries silently.
func TestClient_ListProjects_FakeBinary(t *testing.T) {
	body := `#!/bin/sh
case "$*" in
  *"_projects"*)
    printf 'team.alpha\nshop\n\nteam.alpha\n+evil\nbad space\n../etc\nadmin_tools\n'
    ;;
  *)
    printf ''
    ;;
esac
exit 0
`
	installScript(t, body)
	c := NewClient()
	got, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	want := []string{"admin_tools", "shop", "team.alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

// TestClient_ListTags_FakeBinary mirrors the projects test but for tags. Tag
// names allow dashes (which projects do not) so the fake script includes a
// dashed entry to confirm it survives the filter. Virtual tags (uppercase,
// computed from task state) are also injected and must be stripped because
// suggesting them would offer the user something they cannot actually set.
func TestClient_ListTags_FakeBinary(t *testing.T) {
	body := `#!/bin/sh
case "$*" in
  *"_tags"*)
    printf 'urgent\noffboarding\nurgent\n\n+evil\nin-progress\nbad space\nteam_a\nACTIVE\nBLOCKED\nOVERDUE\nREADY\nTAGGED\n'
    ;;
  *)
    printf ''
    ;;
esac
exit 0
`
	installScript(t, body)
	c := NewClient()
	got, err := c.ListTags(context.Background())
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	want := []string{"in-progress", "offboarding", "team_a", "urgent"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

// TestClient_ProjectsCached_OnceOnly: the cache must invoke `task _projects`
// at most once even when called concurrently from many goroutines. We use a
// counter file written by the fake binary plus parallel calls to surface any
// race in the sync.Once gating.
func TestClient_ProjectsCached_OnceOnly(t *testing.T) {
	dir := t.TempDir()
	counterFile := dir + "/calls"
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
echo X >> ` + counterFile + `
case "$*" in
  *"_projects"*) printf 'alpha\nbeta\n' ;;
  *) printf '' ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	// Fan out 10 parallel callers to expose any sync.Once miswiring under -race.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := c.ProjectsCached(context.Background())
			if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
				t.Errorf("ProjectsCached: %+v", got)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	gotCalls := strings.Count(string(data), "X")
	if gotCalls != 1 {
		t.Errorf("expected 1 task invocation across all ProjectsCached calls, got %d", gotCalls)
	}
}

// TestClient_TagsCached_OnceOnly is the tags-side mirror of the above.
func TestClient_TagsCached_OnceOnly(t *testing.T) {
	dir := t.TempDir()
	counterFile := dir + "/calls"
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
echo X >> ` + counterFile + `
case "$*" in
  *"_tags"*) printf 'one\ntwo\n' ;;
  *) printf '' ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := c.TagsCached(context.Background())
			if len(got) != 2 || got[0] != "one" || got[1] != "two" {
				t.Errorf("TagsCached: %+v", got)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	gotCalls := strings.Count(string(data), "X")
	if gotCalls != 1 {
		t.Errorf("expected 1 task invocation across all TagsCached calls, got %d", gotCalls)
	}
}

// TestClient_ParseContextsTable covers the table parser for the realistic
// shapes Taskwarrior 3 emits: header + separator + per-type rows + footer,
// with mixed read/write entries, an inactive context, and a hostile entry
// that must be dropped silently.
func TestClient_ParseContextsTable(t *testing.T) {
	raw := `
Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     yes
work  write  +work     no
home  read   +home
+evil read   +evil     yes

3 contexts (1 of which are active)
`
	got := parseContextsTable(raw)
	if len(got) != 2 {
		t.Fatalf("got %d contexts, want 2: %+v", len(got), got)
	}
	// Sorted alphabetically: home, work.
	if got[0].Name != "home" || got[0].ReadFilter != "+home" || got[0].Active {
		t.Errorf("home: %+v", got[0])
	}
	if got[1].Name != "work" || got[1].ReadFilter != "+work" || got[1].WriteFilter != "+work" || !got[1].Active {
		t.Errorf("work: %+v", got[1])
	}
}

// TestClient_ParseContextsTable_NoneDefined: empty / "No contexts defined."
// returns an empty slice without error.
func TestClient_ParseContextsTable_NoneDefined(t *testing.T) {
	for _, raw := range []string{
		"",
		"No contexts defined.\n",
		"\n\n",
	} {
		got := parseContextsTable(raw)
		if len(got) != 0 {
			t.Errorf("raw %q: got %+v, want empty", raw, got)
		}
	}
}

// TestClient_ParseContextsTable_FooterVariants covers the trailing "N
// contexts ..." line variants Taskwarrior emits at the bottom of the table.
// We need to skip them rather than parse them as context rows.
func TestClient_ParseContextsTable_FooterVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{
			name: "footer with active count",
			raw: `Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     yes

3 contexts (1 of which are active)
`,
			want: 1,
		},
		{
			name: "footer no active",
			raw: `Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     no

3 contexts (0 of which are active)
`,
			want: 1,
		},
		{
			name: "footer ending with active.",
			raw: `Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     no

1 context (0 of which are active.)
`,
			want: 1,
		},
		{
			name: "footer ending with active",
			raw: `Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     no

1 context (0 of which are active)
`,
			want: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseContextsTable(c.raw)
			if len(got) != c.want {
				t.Errorf("got %d contexts, want %d: %+v", len(got), c.want, got)
			}
			for _, ctx := range got {
				if !strings.HasPrefix(ctx.Name, "work") && !strings.HasPrefix(ctx.Name, "home") {
					t.Errorf("footer leaked into a Context entry: %+v", ctx)
				}
			}
		})
	}
}

// TestClient_ParseContextsTable_DropsRcOverride drops a context whose
// filter contains rc.* tokens (Context.SafeReadFilter handles the runtime
// case but parseContextsTable could carry the value forward; this guards
// against future code reading ReadFilter without going through the safe
// helper).
func TestClient_ParseContextsTable_DropsRcOverride(t *testing.T) {
	// We don't currently drop the entry at parse time (that's a separate
	// design call - see Context.SafeReadFilter); this test pins the
	// current contract: the entry IS present, but SafeReadFilter returns
	// empty. If we later move the drop to parse time, update this test.
	raw := `Name  Type   Filter                                Active
----- ------ ------------------------------------- --------
evil  read   +x or rc.data.location=/tmp/evil      no
`
	got := parseContextsTable(raw)
	if len(got) != 1 {
		t.Fatalf("got %d contexts, want 1: %+v", len(got), got)
	}
	if got[0].SafeReadFilter() != "" {
		t.Errorf("SafeReadFilter should drop rc.* override, got %q", got[0].SafeReadFilter())
	}
}

// TestClient_ListContexts_FakeBinary covers the round-trip from the fake
// binary's stdout through ListContexts and into a parsed []Context.
func TestClient_ListContexts_FakeBinary(t *testing.T) {
	body := `#!/bin/sh
case "$*" in
  *"context list"*)
    cat <<'EOF'
Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     yes
work  write  +work     no
home  read   +home

2 contexts (1 of which are active)
EOF
    exit 0
    ;;
esac
exit 0
`
	installScript(t, body)
	c := NewClient()
	got, err := c.ListContexts(context.Background())
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "home" || got[0].Active {
		t.Errorf("home: %+v", got[0])
	}
	if got[1].Name != "work" || !got[1].Active {
		t.Errorf("work: %+v", got[1])
	}
}

// TestClient_ContextsCached_OnceOnly: the cache must invoke `task context
// list` at most once, mirroring the projects/tags behaviour.
func TestClient_ContextsCached_OnceOnly(t *testing.T) {
	dir := t.TempDir()
	counterFile := dir + "/calls"
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
echo X >> ` + counterFile + `
case "$*" in
  *"context list"*) printf 'Name  Type   Filter   Active\n----- ------ -------- ---\nwork  read   +work     yes\n' ;;
  *) printf '' ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := c.ContextsCached(context.Background())
			if len(got) != 1 || got[0].Name != "work" {
				t.Errorf("ContextsCached: %+v", got)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	gotCalls := strings.Count(string(data), "X")
	if gotCalls != 1 {
		t.Errorf("expected 1 invocation across all ContextsCached calls, got %d", gotCalls)
	}
}

// TestClient_ActiveContext_FakeBinary: `task _get rc.context` returns the
// bare name of the active context (or empty when none is set).
func TestClient_ActiveContext_FakeBinary(t *testing.T) {
	installScript(t, `#!/bin/sh
case "$*" in
  *"_get rc.context"*) printf 'work\n' ;;
  *) printf '' ;;
esac
exit 0
`)
	c := NewClient()
	if got := c.ActiveContext(context.Background()); got != "work" {
		t.Errorf("ActiveContext: got %q want %q", got, "work")
	}
}

// TestClient_ActiveContext_Empty: empty stdout means no active context.
func TestClient_ActiveContext_Empty(t *testing.T) {
	installScript(t, "#!/bin/sh\nprintf ''\nexit 0\n")
	c := NewClient()
	if got := c.ActiveContext(context.Background()); got != "" {
		t.Errorf("ActiveContext: got %q want empty", got)
	}
}

// TestClient_ActiveContext_RejectsBadName: defence-in-depth - if `_context`
// somehow returned a name with shell metacharacters, the validator drops it.
func TestClient_ActiveContext_RejectsBadName(t *testing.T) {
	installScript(t, "#!/bin/sh\nprintf 'evil; ls\\n'\nexit 0\n")
	c := NewClient()
	if got := c.ActiveContext(context.Background()); got != "" {
		t.Errorf("ActiveContext: got %q want empty (rejected)", got)
	}
}

// TestClient_ActiveContext_BinaryMissing: error path returns "" not panic.
func TestClient_ActiveContext_BinaryMissing(t *testing.T) {
	t.Setenv("PATH", "")
	c := NewClient()
	if got := c.ActiveContext(context.Background()); got != "" {
		t.Errorf("ActiveContext on missing binary: got %q want empty", got)
	}
}

// TestClient_SetContext_FakeBinary records argv to confirm the right
// subcommand is issued for both the activate and clear shapes.
func TestClient_SetContext_FakeBinary(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "task")
	body := `#!/bin/sh
for a in "$@"; do
  printf "%s\n" "$a" >> "` + logFile + `"
done
printf '\n' >> "` + logFile + `"
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()

	if err := c.SetContext(context.Background(), "work"); err != nil {
		t.Fatalf("SetContext work: %v", err)
	}
	if err := c.SetContext(context.Background(), ""); err != nil {
		t.Fatalf("SetContext clear: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	// The two invocations are separated by a blank line by the recorder above.
	chunks := strings.Split(strings.TrimRight(string(data), "\n"), "\n\n")
	if len(chunks) < 2 {
		t.Fatalf("expected 2 invocations, got %d: %q", len(chunks), data)
	}
	if !strings.Contains(chunks[0], "context") || !strings.Contains(chunks[0], "work") {
		t.Errorf("first invocation missing context/work: %q", chunks[0])
	}
	if !strings.Contains(chunks[1], "context") || !strings.Contains(chunks[1], "none") {
		t.Errorf("second invocation missing context/none: %q", chunks[1])
	}
}

// TestClient_SetContext_RejectsBadName: defence-in-depth - even if a handler
// somehow forwarded a hostile name, the Client refuses to invoke `task
// context <evil>`.
func TestClient_SetContext_RejectsBadName(t *testing.T) {
	c := NewClient()
	for _, bad := range []string{"work; ls", "../etc", "rc.foo=bar", "a b", "+evil"} {
		if err := c.SetContext(context.Background(), bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

// TestClient_Dependents_RejectsBadUUID confirms the IDPattern guard runs
// before the binary is ever invoked - otherwise a hostile uuid value could
// propagate into a `depends.has:<...>` filter fragment and end up parsed as a
// rc.* override or shell metachar.
func TestClient_Dependents_RejectsBadUUID(t *testing.T) {
	c := NewClient()
	for _, bad := range []string{"", "abc", "rc.foo=bar", "1; ls", "u u"} {
		_, err := c.Dependents(context.Background(), bad)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("uuid %q: expected ErrInvalid, got %v", bad, err)
		}
	}
}

// TestClient_Dependents_FakeBinary records the argv passed to a stub `task`
// binary and confirms the Dependents method emits both the depends.has filter
// and a status:pending filter, then decodes the JSON the fake returns.
func TestClient_Dependents_FakeBinary(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "argv")
	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "task")
	body := `#!/bin/sh
for a in "$@"; do
  printf "%s\n" "$a" >> "` + logFile + `"
done
cat <<'JSON_EOF'
[{
  "id": 7,
  "uuid": "22222222-3333-4444-5555-666666666666",
  "description": "downstream",
  "status": "pending",
  "entry": "20260501T080000Z",
  "depends": ["11111111-2222-3333-4444-555555555555"]
}]
JSON_EOF
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	got, err := c.Dependents(context.Background(), "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}
	if len(got) != 1 || got[0].Description != "downstream" {
		t.Errorf("got %+v", got)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	args := string(data)
	if !strings.Contains(args, "depends.has:11111111-2222-3333-4444-555555555555") {
		t.Errorf("missing depends.has filter: %s", args)
	}
	if !strings.Contains(args, "status:pending") {
		t.Errorf("missing status:pending filter: %s", args)
	}
	if !strings.Contains(args, "export") {
		t.Errorf("missing export verb: %s", args)
	}
}

// TestClient_ProjectsCached_FailureRetries: when `task _projects` fails on
// the first call (and there's no prior good value to keep), subsequent calls
// retry the fetch instead of caching empty for the process lifetime. This
// is the no-poison contract on ttlCache: a transient binary failure must
// not blank the dropdown until restart.
func TestClient_ProjectsCached_FailureRetries(t *testing.T) {
	dir := t.TempDir()
	counterFile := dir + "/calls"
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
echo X >> ` + counterFile + `
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	for i := 0; i < 3; i++ {
		got := c.ProjectsCached(context.Background())
		if len(got) != 0 {
			t.Errorf("call %d: got %v, want empty on failure", i, got)
		}
	}
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	gotCalls := strings.Count(string(data), "X")
	if gotCalls != 3 {
		t.Errorf("expected 3 invocations across 3 failures (no poison), got %d", gotCalls)
	}
}

// TestClient_ProjectsCached_KeepsPriorOnTransientFailure: once a successful
// fetch lands, a subsequent failure must NOT blank the cached list. The
// ttlCache keeps the prior value on error so a flaky binary doesn't strip
// the dropdown mid-session.
func TestClient_ProjectsCached_KeepsPriorOnTransientFailure(t *testing.T) {
	dir := t.TempDir()
	stateFile := dir + "/state"
	if err := os.WriteFile(stateFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	scriptDir := t.TempDir()
	script := scriptDir + "/task"
	body := `#!/bin/sh
state=$(cat ` + stateFile + `)
if [ "$state" = "ok" ]; then
  printf 'team.alpha\nteam.beta\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := NewClient()
	got := c.ProjectsCached(context.Background())
	if len(got) != 2 {
		t.Fatalf("first call: got %v, want 2 entries", got)
	}
	// Flip the script to fail; force cache miss by expiring the entry.
	if err := os.WriteFile(stateFile, []byte("fail"), 0o644); err != nil {
		t.Fatalf("flip state: %v", err)
	}
	c.projects.expiry = time.Now().Add(-time.Second) // force re-fetch

	got2 := c.ProjectsCached(context.Background())
	if len(got2) != 2 {
		t.Errorf("second call after transient failure: got %v, want prior 2 entries kept", got2)
	}
}

// TestIsNoOpExit covers the "TW reported 0 tasks affected" detection used by
// idempotent action handlers (delete/done/modify) to convert a non-zero
// exit on an already-in-target-state task into a quiet success rather than
// a spurious 500. Patterns drawn from real TW 3.x output.
func TestIsNoOpExit(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		wantNo bool
	}{
		{"deleted-zero", &TaskExitError{ExitCode: 1, Stdout: "Task ac20209b 'X' is not deletable.\nDeleted 0 tasks.\n"}, true},
		{"completed-zero", &TaskExitError{ExitCode: 1, Stdout: "Completed 0 tasks.\n"}, true},
		{"modified-zero", &TaskExitError{ExitCode: 1, Stdout: "Modified 0 tasks.\n"}, true},
		{"deleted-one", &TaskExitError{ExitCode: 0, Stdout: "Deleting task 5 'X'.\nDeleted 1 task.\n"}, false},
		{"unrelated-error", &TaskExitError{ExitCode: 1, Stdout: "", Stderr: "Could not interpret the date 'x'."}, false},
		{"non-task-error", errors.New("network down"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsNoOpExit(c.err); got != c.wantNo {
				t.Errorf("IsNoOpExit(%v) = %v, want %v", c.err, got, c.wantNo)
			}
		})
	}
}
