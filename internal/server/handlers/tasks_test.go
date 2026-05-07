package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

func newTasks() *Tasks {
	return &Tasks{TW: tw.NewClient(), Logger: discardLogger()}
}

// formRequest builds an x-www-form-urlencoded POST/PUT request.
func formRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestTasks_Create_Success(t *testing.T) {
	installFakeTask(t, "[]") // not actually exporting; OK for write-side
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=buy+milk&project=shop&tags=urgent,errand&due=tomorrow")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Trigger"); got != "refresh" {
		t.Errorf("HX-Trigger: got %q want refresh", got)
	}
}

func TestTasks_Create_EmptyDescription(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks", "description=&project=x")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Create_InvalidProject(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks", "description=x&project=../etc")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Create_500WhenTaskBinaryFails(t *testing.T) {
	installFailingTask(t)
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks", "description=x")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestTasks_Modify_Success(t *testing.T) {
	installFakeTask(t, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "old",
		"status": "pending",
		"entry": "20260501T120000Z",
		"tags": ["old-tag"]
	}]`)
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42",
		"description=updated&tags=new-tag")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Trigger"); got != "refresh" {
		t.Errorf("HX-Trigger: got %q", got)
	}
}

func TestTasks_Modify_RejectsBadID(t *testing.T) {
	tk := newTasks()
	for _, id := range []string{"abc", "1;ls", "../"} {
		req := formRequest(http.MethodPut, "/tasks/x", "description=x")
		req.SetPathValue("id", id)
		rr := httptest.NewRecorder()
		tk.Modify(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("id %q: got %d want 400", id, rr.Code)
		}
	}
}

func TestTasks_Modify_404WhenTaskNotFound(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/99", "description=x")
	req.SetPathValue("id", "99")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d want 404", rr.Code)
	}
}

func TestTasks_Modify_400OnInvalidInput(t *testing.T) {
	installFakeTask(t, `[{"id":42,"uuid":"u","description":"x","status":"pending","entry":"20260501T120000Z"}]`)
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42", "description=&project=ok")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Done_Success(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := httptest.NewRequest(http.MethodPost, "/tasks/42/done", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Done(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204", rr.Code)
	}
	if rr.Header().Get("HX-Trigger") != "refresh" {
		t.Errorf("HX-Trigger missing")
	}
}

func TestTasks_Done_RejectsBadID(t *testing.T) {
	tk := newTasks()
	req := httptest.NewRequest(http.MethodPost, "/tasks/x/done", nil)
	req.SetPathValue("id", "abc")
	rr := httptest.NewRecorder()
	tk.Done(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Delete_Success(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := httptest.NewRequest(http.MethodDelete, "/tasks/42", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Delete(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204", rr.Code)
	}
	if rr.Header().Get("HX-Trigger") != "refresh" {
		t.Errorf("HX-Trigger missing")
	}
}

func TestTasks_Delete_RejectsBadID(t *testing.T) {
	tk := newTasks()
	req := httptest.NewRequest(http.MethodDelete, "/tasks/x", nil)
	req.SetPathValue("id", "../")
	rr := httptest.NewRecorder()
	tk.Delete(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_BulkDone_Success(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/bulk/done", "bulk-ids=1,2,3")
	rr := httptest.NewRecorder()
	tk.BulkDone(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
}

func TestTasks_BulkDelete_Success(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/bulk/delete", "bulk-ids=1,2,3")
	rr := httptest.NewRecorder()
	tk.BulkDelete(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204", rr.Code)
	}
}

func TestTasks_Bulk_NoIDs(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/bulk/done", "bulk-ids=")
	rr := httptest.NewRecorder()
	tk.BulkDone(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Bulk_OverCap(t *testing.T) {
	tk := newTasks()
	// 101 ids -> over the maxBulkIDs=100 cap.
	parts := make([]string, 101)
	for i := range parts {
		parts[i] = "1"
	}
	req := formRequest(http.MethodPost, "/tasks/bulk/done", "bulk-ids="+strings.Join(parts, ","))
	rr := httptest.NewRecorder()
	tk.BulkDone(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "max 100") {
		t.Errorf("body should mention cap: %s", rr.Body.String())
	}
}

func TestTasks_Bulk_AtCap_Allowed(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	parts := make([]string, 100)
	for i := range parts {
		parts[i] = "1"
	}
	req := formRequest(http.MethodPost, "/tasks/bulk/done", "bulk-ids="+strings.Join(parts, ","))
	rr := httptest.NewRecorder()
	tk.BulkDone(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204", rr.Code)
	}
}

func TestTasks_Bulk_RejectsBadID(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/bulk/done", "bulk-ids=1,2,abc")
	rr := httptest.NewRecorder()
	tk.BulkDone(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in  string
		out []string
	}{
		{"1,2,3", []string{"1", "2", "3"}},
		{"1, 2 , 3", []string{"1", "2", "3"}},
		{",,,", []string{}},
		{"", []string{}},
		{"1", []string{"1"}},
		{"a, b,c ,, ", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if len(got) != len(c.out) {
			t.Errorf("in=%q: got %v want %v", c.in, got, c.out)
			continue
		}
		for i := range got {
			if got[i] != c.out[i] {
				t.Errorf("in=%q[%d]: got %q want %q", c.in, i, got[i], c.out[i])
			}
		}
	}
}

func TestStringSet(t *testing.T) {
	m := stringSet([]string{"a", "b", "a"})
	if _, ok := m["a"]; !ok {
		t.Errorf("missing 'a' in %v", m)
	}
	if _, ok := m["b"]; !ok {
		t.Errorf("missing 'b' in %v", m)
	}
	if len(m) != 2 {
		t.Errorf("len=%d want 2: %v", len(m), m)
	}
}

func TestTasks_Annotate_RejectsEmpty(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/42/annotate", "text=")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Annotate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Annotate_RejectsWhitespaceOnly(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/42/annotate", "text=%20%09")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Annotate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Annotate_RejectsBadID(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/x/annotate", "text=note")
	req.SetPathValue("id", "abc")
	rr := httptest.NewRecorder()
	tk.Annotate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Denotate_RejectsEmpty(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/42/denotate", "text=")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Denotate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestTasks_Annotate_Success_RendersPanel(t *testing.T) {
	// Use a recording fake so we can assert that the handler actually shells
	// out to `task <id> annotate -- <text>`. The pre-baked export branch in
	// the previous version of this test let it pass even if Annotate did
	// nothing; this version verifies the side effect happened.
	logDir := installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: `[{
			"id": 42,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "buy milk",
			"status": "pending",
			"entry": "20260501T120000Z",
			"annotations": [{"entry":"20260501T130000Z","description":"first note"}]
		}]`,
		RecordArgv: true,
	})

	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/42/annotate", "text=please+do+the+thing")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Annotate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "first note") {
		t.Errorf("body missing annotation: %s", body)
	}
	// Annotate deliberately does NOT trigger a list refresh (would close the
	// modal). HX-Trigger should be empty.
	if got := rr.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger: got %q want empty", got)
	}

	// One of the recorded invocations must be the annotate call. Look for
	// argv that contains both "annotate" and the supplied text; export and
	// other discovery calls won't match.
	invocations := readAllInvocations(t, logDir)
	got := findInvocationContaining(invocations, "please do the thing")
	if got == nil {
		t.Fatalf("no invocation issued the annotate text; saw %v", invocations)
	}
	if findInvocationContaining([][]string{got}, "annotate") == nil {
		t.Errorf("expected annotate verb in invocation, got %v", got)
	}
	// IDPattern guard means the id appears as its own argv element.
	if findInvocationContaining([][]string{got}, "42") == nil {
		t.Errorf("expected id 42 in invocation, got %v", got)
	}
}

func TestTasks_Denotate_Success_RendersPanel(t *testing.T) {
	installFakeTask(t, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "buy milk",
		"status": "pending",
		"entry": "20260501T120000Z"
	}]`)
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks/42/denotate", "text=remove+me")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Denotate(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// readAddInput is exercised through Create above, but a direct test sanity-
// checks that whitespace is preserved (description) and tags are split.
func TestReadAddInput(t *testing.T) {
	req := formRequest(http.MethodPost, "/tasks",
		"description=hello+world&project=p&tags=a,b&due=tomorrow&wait=eom&scheduled=due-3d")
	got := readAddInput(req)
	if got.Description != "hello world" {
		t.Errorf("desc: %q", got.Description)
	}
	if got.Project != "p" {
		t.Errorf("project: %q", got.Project)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "a" || got.Tags[1] != "b" {
		t.Errorf("tags: %v", got.Tags)
	}
	if got.Due != "tomorrow" || got.Wait != "eom" || got.Scheduled != "due-3d" {
		t.Errorf("dates: due=%q wait=%q scheduled=%q", got.Due, got.Wait, got.Scheduled)
	}
}

// installRecordingTaskWithUDAs is installRecordingTask plus a one-UDA taskrc
// (so the Tasks handlers see a non-empty UDA list and emit the matching
// args). exportJSON is the literal stdout for any `task ... export` call - if
// empty, defaults to "[]". Returns the per-invocation argv log directory.
func installRecordingTaskWithUDAs(t *testing.T, udas []struct{ Name, Type, Label string }, exportJSON string) string {
	t.Helper()
	if exportJSON == "" {
		exportJSON = "[]"
	}
	conv := make([]fakeUDA, 0, len(udas))
	for _, u := range udas {
		conv = append(conv, fakeUDA{Name: u.Name, Type: u.Type, Label: u.Label})
	}
	return installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: exportJSON,
		UDAs:       conv,
		RecordArgv: true,
	})
}

// readAllInvocations returns every captured argv log in order, one slice per
// task invocation. Tests can scan across all invocations to find the one
// they care about (e.g. the actual `task add ...` call as opposed to the
// preceding `_udas` discovery calls).
func readAllInvocations(t *testing.T, logDir string) [][]string {
	t.Helper()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var out [][]string
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var lines []string
		for _, l := range strings.Split(string(data), "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		out = append(out, lines)
	}
	return out
}

// findInvocationContaining returns the first captured argv that has the given
// arg present anywhere; nil if none match. Used to skip past UDA discovery
// invocations and locate the real `add` or `modify` call.
func findInvocationContaining(invocations [][]string, want string) []string {
	for _, inv := range invocations {
		for _, a := range inv {
			if a == want {
				return inv
			}
		}
	}
	return nil
}

// TestTasks_Create_PassesUDAValues confirms that on POST /tasks the UDA form
// fields land in the argv as <name>:"<value>".
func TestTasks_Create_PassesUDAValues(t *testing.T) {
	logDir := installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "duration", Label: "Estimate"},
		{Name: "client", Type: "string", Label: "Client"},
	}, "")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&uda_estimate=PT4H&uda_client=Acme")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d body=%s", rr.Code, rr.Body.String())
	}
	addInvoc := findInvocationContaining(readAllInvocations(t, logDir), "add")
	if addInvoc == nil {
		t.Fatalf("no `add` invocation captured")
	}
	for _, want := range []string{
		`estimate:"PT4H"`,
		`client:"Acme"`,
	} {
		found := false
		for _, a := range addInvoc {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in argv: %v", want, addInvoc)
		}
	}
}

// TestTasks_Create_EmptyUDAStaysUnset confirms that an empty UDA field on
// CREATE produces NO argv entry (Taskwarrior leaves the attribute unset; we
// don't emit a clearing arg on add).
func TestTasks_Create_EmptyUDAStaysUnset(t *testing.T) {
	logDir := installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "string", Label: "Estimate"},
	}, "")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&uda_estimate=")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	addInvoc := findInvocationContaining(readAllInvocations(t, logDir), "add")
	for _, a := range addInvoc {
		if strings.HasPrefix(a, "estimate:") {
			t.Errorf("empty UDA leaked into create argv: %v", addInvoc)
		}
	}
}

// TestTasks_Modify_EmptyUDAClears confirms that an empty UDA on PUT (modify)
// emits the bare `<name>:` clear-arg, mirroring how due/wait/scheduled clear.
func TestTasks_Modify_EmptyUDAClears(t *testing.T) {
	logDir := installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "string", Label: "Estimate"},
	}, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "old",
		"status": "pending",
		"entry": "20260501T120000Z"
	}]`)
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42",
		"description=updated&uda_estimate=")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	modifyInvoc := findInvocationContaining(readAllInvocations(t, logDir), "modify")
	if modifyInvoc == nil {
		t.Fatalf("no modify invocation captured")
	}
	found := false
	for _, a := range modifyInvoc {
		if a == "estimate:" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bare clear-arg `estimate:` in argv: %v", modifyInvoc)
	}
}

// TestTasks_Modify_NonEmptyUDAOverrides confirms that providing a value on
// modify wraps it in `<name>:"<value>"` form (same shape as create).
func TestTasks_Modify_NonEmptyUDAOverrides(t *testing.T) {
	logDir := installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "duration", Label: "Estimate"},
	}, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "old",
		"status": "pending",
		"entry": "20260501T120000Z"
	}]`)
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42",
		"description=updated&uda_estimate=PT8H")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	modifyInvoc := findInvocationContaining(readAllInvocations(t, logDir), "modify")
	want := `estimate:"PT8H"`
	found := false
	for _, a := range modifyInvoc {
		if a == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in modify argv: %v", want, modifyInvoc)
	}
}

// TestTasks_Create_RejectsBadNumeric confirms that a non-numeric value on a
// numeric-typed UDA returns 400 rather than passing junk to Taskwarrior.
func TestTasks_Create_RejectsBadNumeric(t *testing.T) {
	installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "points", Type: "numeric", Label: "Points"},
	}, "")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&uda_points=not-a-number")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestTasks_Create_RejectsBadDate confirms a malformed date-typed UDA is
// rejected before reaching Taskwarrior.
func TestTasks_Create_RejectsBadDate(t *testing.T) {
	installRecordingTaskWithUDAs(t, []struct{ Name, Type, Label string }{
		{Name: "ddate", Type: "date", Label: "Deadline"},
	}, "")
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&uda_ddate=tomorrow%3B+rm+-rf+%2F")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestReadUDAArgs_NoUDAsNoop confirms readUDAArgs is a noop when no UDAs are
// declared - even if the form has stray uda_* fields, they're ignored
// (defence: a hostile form value cannot smuggle a fake UDA past the cache).
func TestReadUDAArgs_NoUDAsNoop(t *testing.T) {
	req := formRequest(http.MethodPost, "/tasks", "uda_evil=hi&uda_other=foo")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	args, err := readUDAArgs(req, nil, false)
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

func TestTasks_Create_DescriptionInjectionStaysLiteral(t *testing.T) {
	// Capture the args the fake task binary actually received. Use a script
	// that writes its argv to a file we can read back. Each invocation is
	// truncated so we observe the LAST call.
	argsLog := installRecordingTask(t)

	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=%2Burgent+due%3Atomorrow+rc.data.location%3D%2Ftmp%2Fevil")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	logged := readArgs(t, argsLog)
	// The whole DOM-ish payload must appear inside description:"..." form, NOT
	// as a free-floating arg.
	want := `description:"+urgent due:tomorrow rc.data.location=/tmp/evil"`
	found := false
	for _, a := range logged {
		if a == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected literal description arg %q in %v", want, logged)
	}
	// Defence-in-depth: no rc.* arg should be passed to the binary.
	for _, a := range logged {
		if strings.HasPrefix(a, "rc.data.location") {
			t.Errorf("rc.* leaked as a separate arg: %v", logged)
		}
	}
}

// TestTasks_Create_PassesDependsArg confirms that on POST /tasks the depends
// form field lands in the argv as a single `depends:UUID,UUID` arg.
func TestTasks_Create_PassesDependsArg(t *testing.T) {
	logDir := installRecordingTask(t)
	tk := newTasks()
	body := "description=demo&depends=" +
		"11111111-2222-3333-4444-555555555555,22222222-3333-4444-5555-666666666666"
	req := formRequest(http.MethodPost, "/tasks", body)
	rr := httptest.NewRecorder()
	tk.Create(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	addInvoc := findInvocationContaining(readAllInvocations(t, logDir), "add")
	if addInvoc == nil {
		t.Fatalf("no add invocation")
	}
	want := "depends:11111111-2222-3333-4444-555555555555,22222222-3333-4444-5555-666666666666"
	found := false
	for _, a := range addInvoc {
		if a == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing %q in add argv: %v", want, addInvoc)
	}
}

// TestTasks_Create_OneDependsArg confirms a single-uuid depends list still
// ends up wrapped in the same `depends:UUID` form (not split into one arg per
// uuid).
func TestTasks_Create_OneDependsArg(t *testing.T) {
	logDir := installRecordingTask(t)
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&depends=11111111-2222-3333-4444-555555555555")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	addInvoc := findInvocationContaining(readAllInvocations(t, logDir), "add")
	want := "depends:11111111-2222-3333-4444-555555555555"
	found := false
	for _, a := range addInvoc {
		if a == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing %q in add argv: %v", want, addInvoc)
	}
}

// TestTasks_Create_NoDependsArgWhenEmpty confirms create skips the depends arg
// entirely when no deps are submitted (matching the date-clear semantics: on
// add we leave attributes unset rather than emit a clear).
func TestTasks_Create_NoDependsArgWhenEmpty(t *testing.T) {
	logDir := installRecordingTask(t)
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks", "description=demo")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	addInvoc := findInvocationContaining(readAllInvocations(t, logDir), "add")
	for _, a := range addInvoc {
		if strings.HasPrefix(a, "depends:") {
			t.Errorf("unexpected depends arg in create with no deps: %v", addInvoc)
		}
	}
}

// TestTasks_Create_RejectsBadDepends confirms a malformed UUID in the depends
// list returns 400 before reaching Taskwarrior.
func TestTasks_Create_RejectsBadDepends(t *testing.T) {
	tk := newTasks()
	req := formRequest(http.MethodPost, "/tasks",
		"description=demo&depends=not-a-uuid")
	rr := httptest.NewRecorder()
	tk.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestTasks_Modify_DependsClearsWhenEmpty confirms an empty depends form field
// on modify emits the bare `depends:` clear-arg, mirroring how due/wait
// /scheduled clear in the same code path.
func TestTasks_Modify_DependsClearsWhenEmpty(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: `[{
			"id": 42,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "old",
			"status": "pending",
			"entry": "20260501T120000Z",
			"depends": ["22222222-3333-4444-5555-666666666666"]
		}]`,
		RecordArgv: true,
	})
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42", "description=updated&depends=")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	modifyInvoc := findInvocationContaining(readAllInvocations(t, logDir), "modify")
	if modifyInvoc == nil {
		t.Fatalf("no modify invocation")
	}
	found := false
	for _, a := range modifyInvoc {
		if a == "depends:" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bare 'depends:' clear arg in argv: %v", modifyInvoc)
	}
}

// TestTasks_Modify_DependsSetsList confirms a non-empty depends form field on
// modify wraps the list in the same `depends:UUID,UUID` form as create.
func TestTasks_Modify_DependsSetsList(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: `[{
			"id": 42,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "old",
			"status": "pending",
			"entry": "20260501T120000Z"
		}]`,
		RecordArgv: true,
	})
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42",
		"description=updated&depends=22222222-3333-4444-5555-666666666666,33333333-4444-5555-6666-777777777777")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	modifyInvoc := findInvocationContaining(readAllInvocations(t, logDir), "modify")
	want := "depends:22222222-3333-4444-5555-666666666666,33333333-4444-5555-6666-777777777777"
	found := false
	for _, a := range modifyInvoc {
		if a == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in modify argv: %v", want, modifyInvoc)
	}
}

// TestTasks_Modify_RejectsBadDepends confirms an invalid UUID on modify
// returns 400 (the same defence as create).
func TestTasks_Modify_RejectsBadDepends(t *testing.T) {
	installFakeTask(t, `[{"id":42,"uuid":"u","description":"x","status":"pending","entry":"20260501T120000Z"}]`)
	tk := newTasks()
	req := formRequest(http.MethodPut, "/tasks/42", "description=x&depends=not-a-uuid")
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	tk.Modify(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: %d want 400", rr.Code)
	}
}

// TestReadAddInput_ParsesDepends verifies the form-field parsing for depends
// (comma-separated, whitespace-tolerant, drops blanks).
func TestReadAddInput_ParsesDepends(t *testing.T) {
	req := formRequest(http.MethodPost, "/tasks",
		"description=x&depends=11111111-2222-3333-4444-555555555555,+22222222-3333-4444-5555-666666666666+,,")
	got := readAddInput(req)
	if len(got.Depends) != 2 {
		t.Fatalf("Depends: got %v", got.Depends)
	}
	if got.Depends[0] != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("Depends[0]: %q", got.Depends[0])
	}
	if got.Depends[1] != "22222222-3333-4444-5555-666666666666" {
		t.Errorf("Depends[1]: %q", got.Depends[1])
	}
}

func TestTasks_Undo_Success(t *testing.T) {
	installFakeTask(t, "[]")
	tk := newTasks()
	req := httptest.NewRequest(http.MethodPost, "/undo", nil)
	rr := httptest.NewRecorder()
	tk.Undo(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Trigger"); got != "refresh" {
		t.Errorf("HX-Trigger: got %q want refresh", got)
	}
}

func TestTasks_Undo_500WhenTaskBinaryFails(t *testing.T) {
	installFailingTask(t)
	tk := newTasks()
	req := httptest.NewRequest(http.MethodPost, "/undo", nil)
	rr := httptest.NewRecorder()
	tk.Undo(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

// TestTasks_Undo_PassesUndoArg confirms that the handler invokes the Client's
// Undo method which in turn shells `task undo` (the only positional argument
// after the safetyArgs prefix).
func TestTasks_Undo_PassesUndoArg(t *testing.T) {
	argsLog := installRecordingTask(t)
	tk := newTasks()
	req := httptest.NewRequest(http.MethodPost, "/undo", nil)
	rr := httptest.NewRecorder()
	tk.Undo(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	logged := readArgs(t, argsLog)
	// The argv should contain "undo" as the only non-safetyArgs positional.
	foundUndo := false
	for _, a := range logged {
		if a == "undo" {
			foundUndo = true
		}
		// Defence: no other write/read commands should sneak in.
		if a == "add" || a == "modify" || a == "done" || a == "delete" || a == "export" {
			t.Errorf("unexpected sibling command %q in argv: %v", a, logged)
		}
	}
	if !foundUndo {
		t.Errorf("expected `undo` in argv: %v", logged)
	}
}
