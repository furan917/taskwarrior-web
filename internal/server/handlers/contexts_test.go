package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// --- CRUD handler tests ---

func TestContexts_CreateContext_HappyPath(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{RecordArgv: true})
	c := newContexts()

	form := url.Values{"name": {"myctx"}, "read_filter": {"+myctx"}, "write_filter": {""}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh: got %q want true", got)
	}
	// Verify a "define" invocation with the name and filter reached the binary.
	entries, _ := os.ReadDir(logDir)
	found := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(logDir, e.Name()))
		joined := strings.Join(strings.Fields(string(data)), " ")
		if strings.Contains(joined, "define") && strings.Contains(joined, "myctx") {
			found = true
		}
	}
	if !found {
		t.Error("no 'context define myctx' invocation recorded")
	}
}

func TestContexts_CreateContext_RejectsBadName(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	for _, bad := range []string{"bad name", "+evil", "a;b", "rc.foo=bar"} {
		form := url.Values{"name": {bad}, "read_filter": {"+work"}}
		req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		c.CreateContext(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("name %q: got %d want 400", bad, rr.Code)
		}
	}
}

func TestContexts_CreateContext_RejectsRcOverrideInFilter(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {"rc.data.location=/tmp/evil"}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("rc.* filter: got %d want 400", rr.Code)
	}
}

func TestContexts_CreateContext_RequiresReadFilter(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {""}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty read filter: got %d want 400", rr.Code)
	}
}

func TestContexts_UpdateContext_HappyPath(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{RecordArgv: true})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {"+work"}, "write_filter": {""}}
	req := httptest.NewRequest(http.MethodPut, "/contexts/work", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Simulate path value from ServeMux.
	req.SetPathValue("name", "work")
	rr := httptest.NewRecorder()
	c.UpdateContext(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("HX-Refresh") != "true" {
		t.Error("HX-Refresh not set")
	}
	entries, _ := os.ReadDir(logDir)
	found := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(logDir, e.Name()))
		joined := strings.Join(strings.Fields(string(data)), " ")
		if strings.Contains(joined, "define") && strings.Contains(joined, "work") {
			found = true
		}
	}
	if !found {
		t.Error("no 'define work' invocation recorded")
	}
}

func TestContexts_UpdateContext_RejectsBadOldName(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {"+work"}}
	req := httptest.NewRequest(http.MethodPut, "/contexts/bad+name", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "bad+name")
	rr := httptest.NewRecorder()
	c.UpdateContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad path name: got %d want 400", rr.Code)
	}
}

func TestContexts_DeleteContext_HappyPath(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{RecordArgv: true})
	c := newContexts()

	req := httptest.NewRequest(http.MethodDelete, "/contexts/work", nil)
	req.SetPathValue("name", "work")
	rr := httptest.NewRecorder()
	c.DeleteContext(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("HX-Refresh") != "true" {
		t.Error("HX-Refresh not set")
	}
	entries, _ := os.ReadDir(logDir)
	found := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(logDir, e.Name()))
		joined := strings.Join(strings.Fields(string(data)), " ")
		if strings.Contains(joined, "context") && strings.Contains(joined, "delete") && strings.Contains(joined, "work") {
			found = true
		}
	}
	if !found {
		t.Error("no 'context delete work' invocation recorded")
	}
}

func TestContexts_DeleteContext_RejectsBadName(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	req := httptest.NewRequest(http.MethodDelete, "/contexts/bad+name", nil)
	req.SetPathValue("name", "bad+name")
	rr := httptest.NewRecorder()
	c.DeleteContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad name: got %d want 400", rr.Code)
	}
}

func TestContexts_DeleteContext_500WhenTaskFails(t *testing.T) {
	installFailingTask(t)
	c := newContexts()

	req := httptest.NewRequest(http.MethodDelete, "/contexts/work", nil)
	req.SetPathValue("name", "work")
	rr := httptest.NewRecorder()
	c.DeleteContext(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestContexts_ManageContexts_RendersPage(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		Contexts: []fakeContext{
			{Name: "work", ReadFilter: "+work", Active: true},
			{Name: "home", ReadFilter: "+home"},
		},
		ActiveContext: "work",
	})
	c := newContexts()

	req := httptest.NewRequest(http.MethodGet, "/contexts", nil)
	rr := httptest.NewRecorder()
	c.ManageContexts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "work") {
		t.Error("body missing context name 'work'")
	}
	if !strings.Contains(body, "home") {
		t.Error("body missing context name 'home'")
	}
}

func TestContexts_CreateContextForm_RendersModal(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	req := httptest.NewRequest(http.MethodGet, "/forms/context/new", nil)
	rr := httptest.NewRecorder()
	c.CreateContextForm(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d; body=%s", rr.Code, rr.Body.String())
	}
	// Form should target POST /contexts (create path).
	if !strings.Contains(rr.Body.String(), "/contexts") {
		t.Error("create form missing POST /contexts action")
	}
}

func TestContexts_EditContextForm_PreFills(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		Contexts: []fakeContext{{Name: "work", ReadFilter: "+work"}},
	})
	c := newContexts()

	req := httptest.NewRequest(http.MethodGet, "/forms/context/work", nil)
	req.SetPathValue("name", "work")
	rr := httptest.NewRecorder()
	c.EditContextForm(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "work") {
		t.Error("edit form not pre-filled with context name")
	}
}

func TestContexts_EditContextForm_RejectsBadName(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	req := httptest.NewRequest(http.MethodGet, "/forms/context/bad+name", nil)
	req.SetPathValue("name", "bad+name")
	rr := httptest.NewRecorder()
	c.EditContextForm(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad name: got %d want 400", rr.Code)
	}
}

func newContexts() *Contexts {
	return &Contexts{TW: tw.NewClient(), Logger: discardLogger()}
}

func TestContexts_Set_ActivatesNamedContext(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{RecordArgv: true})

	c := newContexts()
	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.Set(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh: got %q want true", got)
	}

	// Argv check: the most recent fake invocation must have included `context work`.
	args := readArgs(t, logDir)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "context") || !strings.Contains(joined, "work") {
		t.Errorf("argv missing context/work: %v", args)
	}
}

func TestContexts_Set_ClearWithEmptyName(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{RecordArgv: true})

	c := newContexts()
	form := url.Values{"name": {""}}
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.Set(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh: got %q want true", got)
	}

	args := readArgs(t, logDir)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "context") || !strings.Contains(joined, "none") {
		t.Errorf("argv missing context/none for clear: %v", args)
	}
}

// TestViews_Report_ComposesActiveContextFilter is the regression test for
// the bug where reports leaked tasks across contexts. Taskwarrior 3.x's
// `task export` does not honour the active context implicitly (unlike
// `task list` etc), so our handler must compose the context's read filter
// into every Export argv. We assert that the recorded export invocation
// includes both the context filter (in parens) AND the report's own filter.
func TestViews_Report_ComposesActiveContextFilter(t *testing.T) {
	logDir := installFakeTaskWith(t, fakeTaskOpts{
		ActiveContext: "work",
		Contexts: []fakeContext{
			{Name: "work", ReadFilter: "+team or project:team"},
		},
		ExportJSON: "[]",
		RecordArgv: true,
	})
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	h := v.Report("ready")
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	// Walk every recorded invocation; at least one must be the export with
	// the composed filter. Other invocations (`_get rc.context`, `context
	// list`, etc.) are filtered out by checking for "export".
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	found := false
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		line := strings.Join(strings.Fields(string(data)), " ")
		if !strings.Contains(line, "export") {
			continue
		}
		if !strings.Contains(line, "(+team or project:team)") {
			t.Errorf("export argv missing active context clause: %q", line)
		}
		if !strings.Contains(line, "+READY") {
			t.Errorf("export argv missing report filter: %q", line)
		}
		found = true
	}
	if !found {
		t.Fatal("no export invocation recorded")
	}
}

func TestContexts_Set_RejectsBadName(t *testing.T) {
	c := newContexts()
	for _, bad := range []string{"work; ls", "../etc", "rc.foo=bar", "a b", "+evil", "work.x"} {
		form := url.Values{"name": {bad}}
		req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		c.Set(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("name %q: got %d want 400", bad, rr.Code)
		}
	}
}

func TestContexts_Set_500WhenTaskFails(t *testing.T) {
	installFailingTask(t)
	c := newContexts()
	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.Set(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

// TestContexts_Set_NoNamePresent: empty body with no Content-Type goes
// through to the "clear" branch (FormValue is "").
func TestContexts_Set_NoNamePresent(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()
	// Empty body, no Content-Type: ParseForm returns nil and FormValue is "".
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(""))
	rr := httptest.NewRecorder()
	c.Set(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
}

// TestContexts_Set_MalformedForm: a malformed application/x-www-form-urlencoded
// body causes ParseForm to fail; the handler must return 400 and never
// invoke the binary.
func TestContexts_Set_MalformedForm(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()
	// "%" is an invalid percent-encoding; url.ParseQuery rejects it.
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader("name=%"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.Set(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("malformed form: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestViews_Report_ThreadsActiveContext checks that the rendered HTML carries
// the active-context pill markup when the fake binary reports an active
// context. The test fakes both `_context` and the contexts table so the
// dropdown also lists the entry.
func TestViews_Report_ThreadsActiveContext(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON:    `[]`,
		ActiveContext: "work",
		Contexts: []fakeContext{
			{Name: "work", ReadFilter: "+work", Active: true},
			{Name: "home", ReadFilter: "+home"},
		},
	})
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Pill ("Context: work" aria-label is the most stable selector hook).
	if !strings.Contains(body, `aria-label="Context: work"`) {
		t.Errorf("active pill markup missing: body=%s", body)
	}
	// Dropdown lists both entries. Selector tracks the generic popover
	// attribute (was data-context-item before the popover refactor).
	if !strings.Contains(body, `data-popover-item>home `) {
		t.Errorf("home not listed in dropdown; body=%s", body)
	}
	// Title carries the context hint.
	if !strings.Contains(body, "[work]") {
		t.Error("title missing context hint")
	}
}

// TestViews_Report_NoActiveContextRendersInactivePill verifies the fallback:
// no active context => "all" pill, no title hint, no nav rule.
func TestViews_Report_NoActiveContextRendersInactivePill(t *testing.T) {
	installFakeTask(t, "[]")
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `aria-label="Context: none"`) {
		t.Errorf("inactive pill markup missing: body=%s", body)
	}
	// Title should not include any [...] hint.
	if strings.Contains(body, "[") && strings.Contains(body, "] · taskwarrior-web-portal") {
		t.Error("title unexpectedly carried context hint")
	}
}

// TestViews_Report_EmptyStateWithActiveContext checks the bespoke empty-state
// copy fires when the list is empty AND a context is active.
func TestViews_Report_EmptyStateWithActiveContext(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON:    "[]",
		ActiveContext: "work",
		Contexts:      []fakeContext{{Name: "work", Active: true}},
	})
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "No tasks match in context") {
		t.Errorf("expected context-aware empty-state copy: body=%s", body)
	}
	if !strings.Contains(body, ">Clear context<") {
		t.Error("expected Clear context button in empty-state")
	}
}

// TestForms_Add_RendersContextPicker: when contexts are defined, the Add
// modal carries a context-picker dropdown in its header preselected to the
// active one, with a helper line under it explaining what the selection
// will attach to the new task.
func TestForms_Add_RendersContextPicker(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{
		ActiveContext: "work",
		Contexts: []fakeContext{
			{Name: "work", ReadFilter: "+work"},
			{Name: "home", ReadFilter: "+home or project:home"},
		},
	})
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	rr := httptest.NewRecorder()
	f.Add(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`data-context-control`,
		`data-context-select`,
		`data-context-helper`,
		`data-prefill-tags="work"`,
		`data-prefill-tags="home"`,
		// Active context = "work" with prefill +work, helper text mirrors
		// views.ContextHelperText output.
		`Adds +work tag.`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body=%s", want, body)
		}
	}
}

// TestForms_Add_NoContextPickerWhenNoContexts: with zero contexts defined
// the picker is hidden entirely - no clutter for users who haven't set up
// contexts yet.
func TestForms_Add_NoContextPickerWhenNoContexts(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	rr := httptest.NewRecorder()
	f.Add(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "data-context-control") {
		t.Errorf("context picker leaked when no contexts defined: body=%s", rr.Body.String())
	}
}

// TestNamedContextsForRender confirms the helper marks the per-request active
// name even when the cached snapshot disagrees (e.g. user changed the active
// context after first cache fill). This guards against the stale-active-flag
// bug class.
func TestNamedContextsForRender(t *testing.T) {
	dir := t.TempDir()
	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "task")
	body := `#!/bin/sh
case "$*" in
  *"context list"*)
    cat <<'EOF'
Name  Type   Filter   Active
----- ------ -------- --------
work  read   +work     yes
home  read   +home     no
EOF
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = dir // tempdir bookkeeping only

	c := tw.NewClient()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// First fill: per-request active is "home" - dropdown should mark home.
	got := namedContextsForRender(c, req, "home")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
	for _, n := range got {
		switch n.Name {
		case "home":
			if !n.Active {
				t.Errorf("home should be active for per-request 'home', got %+v", n)
			}
		case "work":
			if n.Active {
				t.Errorf("work should NOT be active for per-request 'home', got %+v", n)
			}
		default:
			t.Errorf("unexpected name: %+v", n)
		}
	}

	// Second call (same Client; cache still warm) but per-request now "work":
	// the helper must use the new value, not the cached snapshot's flag.
	got = namedContextsForRender(c, req, "work")
	for _, n := range got {
		if n.Name == "work" && !n.Active {
			t.Errorf("work should be active for per-request 'work', got %+v", n)
		}
		if n.Name == "home" && n.Active {
			t.Errorf("home should NOT be active for per-request 'work', got %+v", n)
		}
	}

	// Empty active: nothing in the dropdown is marked.
	got = namedContextsForRender(c, req, "")
	for _, n := range got {
		if n.Active {
			t.Errorf("nothing should be active for empty per-request, got %+v", n)
		}
	}
}

func TestContexts_CreateContext_RejectsLongReadFilter(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {strings.Repeat("a", 1025)}, "write_filter": {""}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("oversized read filter: got %d want 400", rr.Code)
	}
}

func TestContexts_CreateContext_RejectsLongWriteFilter(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {"+work"}, "write_filter": {strings.Repeat("b", 1025)}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("oversized write filter: got %d want 400", rr.Code)
	}
}

func TestContexts_CreateContext_AcceptsMaxLengthFilter(t *testing.T) {
	installFakeTaskWith(t, fakeTaskOpts{})
	c := newContexts()

	form := url.Values{"name": {"work"}, "read_filter": {strings.Repeat("a", 1024)}, "write_filter": {""}}
	req := httptest.NewRequest(http.MethodPost, "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	c.CreateContext(rr, req)

	// 1024 chars is exactly at the limit — should not be rejected for length
	if rr.Code == http.StatusBadRequest && strings.Contains(rr.Body.String(), "too long") {
		t.Errorf("filter at exactly 1024 chars should not be rejected for length")
	}
}
