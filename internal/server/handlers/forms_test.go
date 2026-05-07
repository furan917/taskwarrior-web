package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

// installFakeTask is a thin wrapper around installFakeTaskWith for the most
// common test shape: the fake binary just needs to emit the given JSON for
// any `export` call and silently succeed otherwise.
func installFakeTask(t *testing.T, outputJSON string) {
	t.Helper()
	installFakeTaskWith(t, fakeTaskOpts{ExportJSON: outputJSON})
}

// installFakeTaskWithSuggest is the project/tag-discovery shape: emits the
// supplied lists for `_projects` / `_tags` and the supplied JSON for export.
func installFakeTaskWithSuggest(t *testing.T, outputJSON string, projects, tags []string) {
	t.Helper()
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: outputJSON,
		Projects:   projects,
		Tags:       tags,
	})
}

// installFakeTaskWithUDAs is the UDA-discovery shape: emits the supplied UDA
// names for `_udas`, per-name type/label for `_get rc.uda.<name>.{type,label}`,
// and the supplied JSON for export.
func installFakeTaskWithUDAs(t *testing.T, outputJSON string, udas []struct{ Name, Type, Label string }) {
	t.Helper()
	conv := make([]fakeUDA, 0, len(udas))
	for _, u := range udas {
		conv = append(conv, fakeUDA{Name: u.Name, Type: u.Type, Label: u.Label})
	}
	installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: outputJSON,
		UDAs:       conv,
	})
}

// installFailingTask makes every `task` invocation exit non-zero.
func installFailingTask(t *testing.T) {
	t.Helper()
	installFakeTaskWith(t, fakeTaskOpts{ExitCode: 2})
}

// installRecordingTask is the argv-recording shape: every invocation appends
// its argv to a per-invocation file, and `export` returns "[]" so post-write
// re-fetches still parse. Returns the argv-log directory.
func installRecordingTask(t *testing.T) string {
	t.Helper()
	return installFakeTaskWith(t, fakeTaskOpts{
		ExportJSON: "[]",
		RecordArgv: true,
	})
}

// readArgs returns the argv of the most recent invocation captured by
// installRecordingTask.
func readArgs(t *testing.T, logDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		return nil
	}
	// Files are named 0,1,2,...; pick the highest. They're zero-padded by
	// nothing (just integer names), so lexical max works for runs <10.
	last := entries[len(entries)-1].Name()
	data, err := os.ReadFile(filepath.Join(logDir, last))
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	lines := []string{}
	for _, l := range strings.Split(string(data), "\n") {
		if l == "" {
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newForms() *Forms {
	return &Forms{TW: tw.NewClient(), Logger: discardLogger()}
}

func TestForms_Add_RendersModal(t *testing.T) {
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	// Stamp a CSRF token in context so the rendered form embeds it.
	req = req.WithContext(WithCSRFToken(context.Background(), "tok-123"))
	rr := httptest.NewRecorder()
	f.Add(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Result().Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type: got %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "tok-123") {
		t.Errorf("body missing CSRF token: %s", body)
	}
}

func TestForms_Edit_RejectsBadID(t *testing.T) {
	f := newForms()
	// Note: the http path-value is set explicitly via SetPathValue, since in
	// production it comes from the {id} segment of the routing pattern. We use
	// a fixed URL and only vary what the router would have parsed as {id}.
	for _, id := range []string{"abc", "../etc", "1;ls", "1 2", ""} {
		req := httptest.NewRequest(http.MethodGet, "/forms/edit/x", nil)
		req.SetPathValue("id", id)
		rr := httptest.NewRecorder()
		f.Edit(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("id %q: got %d want 400; body=%s", id, rr.Code, rr.Body.String())
		}
	}
}

func TestForms_Edit_NotFoundForUnknownID(t *testing.T) {
	installFakeTask(t, "[]") // empty array
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/42", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	f.Edit(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404", rr.Code)
	}
}

func TestForms_Edit_RendersForExistingID(t *testing.T) {
	installFakeTask(t, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "buy milk",
		"status": "pending",
		"entry": "20260501T120000Z",
		"project": "shop",
		"tags": ["urgent", "errand"]
	}]`)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/42", nil)
	req.SetPathValue("id", "42")
	req = req.WithContext(WithCSRFToken(context.Background(), "edit-tok"))
	rr := httptest.NewRecorder()
	f.Edit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"buy milk", "shop", "urgent", "errand", "edit-tok"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

// TestForms_Add_RendersUDAFields confirms that when the taskrc has UDAs
// declared, /forms/add includes one input per UDA with the expected
// uda_<name> form-name and the resolved label.
func TestForms_Add_RendersUDAFields(t *testing.T) {
	installFakeTaskWithUDAs(t, "[]", []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "duration", Label: "Estimate"},
		{Name: "client", Type: "string", Label: ""},
	})
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	req = req.WithContext(WithCSRFToken(context.Background(), "tok-uda"))
	rr := httptest.NewRecorder()
	f.Add(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`name="uda_estimate"`,
		`name="uda_client"`,
		"Estimate",
		// Label fallback: when label is empty the bare name shows.
		"client",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

// TestForms_Add_NoUDAsRendersCleanForm confirms zero UDAs == no extra inputs:
// the modal looks identical to the legacy form.
func TestForms_Add_NoUDAsRendersCleanForm(t *testing.T) {
	installFakeTask(t, "[]") // no _udas response -> zero UDAs
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	req = req.WithContext(WithCSRFToken(context.Background(), "tok"))
	rr := httptest.NewRecorder()
	f.Add(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, `name="uda_`) {
		t.Errorf("body has UDA inputs but no UDAs are defined: %s", body)
	}
}

// TestForms_Edit_PrefillsUDAValues confirms the edit modal pre-fills
// previously-set UDA values. Uses a fixture where the task has both a known
// UDA value (estimate=PT4H) and a UDA with no value (client) - the empty UDA
// renders an empty input, the set UDA renders with its value pre-filled.
func TestForms_Edit_PrefillsUDAValues(t *testing.T) {
	installFakeTaskWithUDAs(t, `[{
		"id": 42,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "design review",
		"status": "pending",
		"entry": "20260501T120000Z",
		"estimate": "PT4H"
	}]`, []struct{ Name, Type, Label string }{
		{Name: "estimate", Type: "duration", Label: "Estimate"},
		{Name: "client", Type: "string", Label: "Client"},
	})
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/42", nil)
	req.SetPathValue("id", "42")
	req = req.WithContext(WithCSRFToken(context.Background(), "tok"))
	rr := httptest.NewRecorder()
	f.Edit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// estimate has a value -> input value attr is pre-filled.
	if !strings.Contains(body, `value="PT4H"`) {
		t.Errorf("estimate value not pre-filled: %s", body)
	}
	// client is empty -> input is rendered but with no/empty value.
	if !strings.Contains(body, `name="uda_client"`) {
		t.Errorf("client UDA missing: %s", body)
	}
}

func TestForms_Edit_500WhenExportFails(t *testing.T) {
	installFailingTask(t)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/42", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	f.Edit(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", rr.Code)
	}
}

// TestForms_Add_RendersSuggestDatalists confirms that the add modal embeds
// the themed autocomplete dropdowns for project and tags (a [data-ac]
// container per field with one <li role="option"> per cached project/tag,
// each carrying data-ac-value and data-ac-text). The native <datalist>
// element was replaced in v5 because we couldn't theme it light/dark.
func TestForms_Add_RendersSuggestDatalists(t *testing.T) {
	installFakeTaskWithSuggest(t, "[]",
		[]string{"team.alpha", "shop", "admin"},
		[]string{"urgent", "offboarding", "review"},
	)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	req = req.WithContext(WithCSRFToken(context.Background(), "tok-suggest"))
	rr := httptest.NewRecorder()
	f.Add(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()

	// Project and tags inputs are wrapped in [data-ac] containers with the
	// expected mode and an [data-ac-input] inside.
	for _, want := range []string{
		`name="project"`,
		`name="tags"`,
		`data-ac data-ac-mode="single"`,
		`data-ac data-ac-mode="tokens"`,
		`data-ac-input`,
		`data-ac-list`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body=%s", want, body)
		}
	}

	// Each project / tag appears as a themed <li role="option"> with its
	// value as data-ac-value (also data-ac-text since they're identical for
	// these fields).
	for _, want := range []string{
		`data-ac-value="admin"`,
		`data-ac-value="shop"`,
		`data-ac-value="team.alpha"`,
		`data-ac-value="offboarding"`,
		`data-ac-value="review"`,
		`data-ac-value="urgent"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing option %q", want)
		}
	}
}

// TestForms_Add_RendersDepPicker confirms the add modal embeds the dependency
// picker scaffolding: a wrapper [data-dep-picker] div, a hidden input named
// "depends", and a [data-ac-list] populated with one <li role="option"> per
// open task (each carrying data-ac-value=<uuid> and data-ac-text=<desc>).
// The exporting `task` shell is the same call we use for the labels/calendar
// pages.
func TestForms_Add_RendersDepPicker(t *testing.T) {
	installFakeTask(t, `[
		{
			"id": 1,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "first task",
			"status": "pending",
			"entry": "20260501T120000Z"
		},
		{
			"id": 2,
			"uuid": "22222222-3333-4444-5555-666666666666",
			"description": "second task",
			"status": "pending",
			"entry": "20260501T120000Z"
		},
		{
			"id": 3,
			"uuid": "33333333-4444-5555-6666-777777777777",
			"description": "third task",
			"status": "waiting",
			"entry": "20260501T120000Z"
		}
	]`)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/add", nil)
	req = req.WithContext(WithCSRFToken(context.Background(), "tok"))
	rr := httptest.NewRecorder()
	f.Add(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`data-dep-picker`,
		`data-ac-mode="deps"`,
		`name="depends"`,
		`class="dep-input`,
		`first task`,
		`second task`,
		`third task`,
		`data-ac-value="11111111-2222-3333-4444-555555555555"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// Three tasks -> three <li role="option"> entries in the dep picker's
	// list. We count occurrences of `data-ac-value=` as a proxy (the only
	// other place that attribute appears is on the project/tags options,
	// which use disjoint values from the UUID-shaped ones here).
	if got := strings.Count(body, `data-ac-value="11111111`) + strings.Count(body, `data-ac-value="22222222`) + strings.Count(body, `data-ac-value="33333333`); got < 3 {
		t.Errorf("expected at least 3 dep options, got %d", got)
	}
}

// TestForms_Edit_RendersDepPickerPills confirms the edit modal pre-populates
// the dependency picker with one pill per existing dep on the task being
// edited, that the pill's data-uuid is the dep uuid, and that the hidden
// "depends" input carries the comma-joined uuid list.
func TestForms_Edit_RendersDepPickerPills(t *testing.T) {
	installFakeTask(t, `[
		{
			"id": 7,
			"uuid": "77777777-8888-9999-aaaa-bbbbbbbbbbbb",
			"description": "downstream task",
			"status": "pending",
			"entry": "20260501T120000Z",
			"depends": [
				"11111111-2222-3333-4444-555555555555",
				"22222222-3333-4444-5555-666666666666"
			]
		},
		{
			"id": 1,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "upstream one",
			"status": "pending",
			"entry": "20260501T120000Z"
		},
		{
			"id": 2,
			"uuid": "22222222-3333-4444-5555-666666666666",
			"description": "upstream two",
			"status": "pending",
			"entry": "20260501T120000Z"
		}
	]`)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/77777777-8888-9999-aaaa-bbbbbbbbbbbb", nil)
	req.SetPathValue("id", "77777777-8888-9999-aaaa-bbbbbbbbbbbb")
	req = req.WithContext(WithCSRFToken(context.Background(), "tok-deps"))
	rr := httptest.NewRecorder()
	f.Edit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Two pills, one per existing dep, each carrying the matching data-uuid.
	for _, want := range []string{
		`class="dep-pill`,
		`data-uuid="11111111-2222-3333-4444-555555555555"`,
		`data-uuid="22222222-3333-4444-5555-666666666666"`,
		// Hidden field carries the comma-joined uuid list.
		`name="depends" class="dep-hidden" value="11111111-2222-3333-4444-555555555555,22222222-3333-4444-5555-666666666666"`,
		// Pills resolve descriptions from the open-tasks list.
		`upstream one`,
		`upstream two`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// TestForms_Edit_DepPickerExcludesSelf confirms the dep picker's datalist for
// the edit modal does NOT include the task being edited - a task cannot
// depend on itself, so it must not appear as a candidate.
func TestForms_Edit_DepPickerExcludesSelf(t *testing.T) {
	installFakeTask(t, `[
		{
			"id": 7,
			"uuid": "77777777-8888-9999-aaaa-bbbbbbbbbbbb",
			"description": "self task",
			"status": "pending",
			"entry": "20260501T120000Z"
		},
		{
			"id": 1,
			"uuid": "11111111-2222-3333-4444-555555555555",
			"description": "other task",
			"status": "pending",
			"entry": "20260501T120000Z"
		}
	]`)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/77777777-8888-9999-aaaa-bbbbbbbbbbbb", nil)
	req.SetPathValue("id", "77777777-8888-9999-aaaa-bbbbbbbbbbbb")
	req = req.WithContext(WithCSRFToken(context.Background(), "tok"))
	rr := httptest.NewRecorder()
	f.Edit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// The other task must be present in the dep picker option list.
	if !strings.Contains(body, `data-ac-value="11111111-2222-3333-4444-555555555555"`) {
		t.Errorf("body missing other task's option")
	}
	// The self uuid must NOT appear as a dep option. Since the self-uuid
	// otherwise has no reason to appear in this fixture (no pills, no
	// dependents), any match implies the exclusion regressed.
	if strings.Contains(body, `data-ac-value="77777777-8888-9999-aaaa-bbbbbbbbbbbb"`) {
		t.Errorf("self uuid leaked into dep picker: %s", body)
	}
}

// TestForms_Edit_RendersSuggestDatalists confirms the edit modal carries the
// same datalists as the add modal, populated identically.
func TestForms_Edit_RendersSuggestDatalists(t *testing.T) {
	installFakeTaskWithSuggest(t, `[{
		"id": 7,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "ship docs",
		"status": "pending",
		"entry": "20260501T120000Z",
		"project": "shop",
		"tags": ["urgent"]
	}]`,
		[]string{"shop", "team.alpha"},
		[]string{"urgent", "review"},
	)
	f := newForms()
	req := httptest.NewRequest(http.MethodGet, "/forms/edit/7", nil)
	req.SetPathValue("id", "7")
	req = req.WithContext(WithCSRFToken(context.Background(), "tok-edit"))
	rr := httptest.NewRecorder()
	f.Edit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`data-ac data-ac-mode="single"`,
		`data-ac data-ac-mode="tokens"`,
		`data-ac-value="shop"`,
		`data-ac-value="team.alpha"`,
		`data-ac-value="urgent"`,
		`data-ac-value="review"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}
