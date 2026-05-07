package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

func TestFilterTasks_DescriptionMatch(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Description: "Buy milk"},
		{ID: 2, Description: "Schedule dentist"},
		{ID: 3, Description: "Call about MILK delivery"},
	}
	got := filterTasks(tasks, "milk")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("ids: %v %v", got[0].ID, got[1].ID)
	}
}

func TestFilterTasks_ProjectMatch(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Description: "x", Project: "team.alpha"},
		{ID: 2, Description: "y", Project: "personal"},
	}
	got := filterTasks(tasks, "ALPHA")
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestFilterTasks_TagMatch(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Description: "x", Tags: []string{"urgent"}},
		{ID: 2, Description: "y", Tags: []string{"chore"}},
		{ID: 3, Description: "z", Tags: []string{"URGENT-rev"}},
	}
	got := filterTasks(tasks, "urgent")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2; %+v", len(got), got)
	}
}

func TestFilterTasks_DeduplicatesAcrossFields(t *testing.T) {
	// Task matches in BOTH description AND project; should appear ONCE
	// (the handler "continue"s after the first hit per field walk).
	tasks := []tw.Task{
		{ID: 1, Description: "milk", Project: "milk"},
	}
	got := filterTasks(tasks, "milk")
	if len(got) != 1 {
		t.Errorf("got %d want 1", len(got))
	}
}

func TestFilterTasks_NoMatchReturnsEmpty(t *testing.T) {
	tasks := []tw.Task{{ID: 1, Description: "buy"}}
	got := filterTasks(tasks, "nope")
	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestFilterTasks_CaseInsensitive(t *testing.T) {
	tasks := []tw.Task{{ID: 1, Description: "Daily Standup"}}
	for _, q := range []string{"daily", "DAILY", "STANDUP", "Stand"} {
		if got := filterTasks(tasks, q); len(got) != 1 {
			t.Errorf("q=%q: got %d", q, len(got))
		}
	}
}

func TestPartial_RejectsBadProjectName(t *testing.T) {
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	for _, p := range []string{"../etc", "+team", "team alpha", "team;ls"} {
		req := httptest.NewRequest(http.MethodGet, "/partials/list?project="+url.QueryEscape(p), nil)
		rr := httptest.NewRecorder()
		v.Partial(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("project %q: got %d want 400", p, rr.Code)
		}
	}
}

func TestPartial_RejectsUnknownReport(t *testing.T) {
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list?report=does-not-exist", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestPartial_RejectsMissingArgs(t *testing.T) {
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestPartial_RendersListFragment(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "buy milk",
		"status": "pending",
		"entry": "20260501T120000Z"
	}]`)
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list?report=next", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "buy milk") {
		t.Errorf("body missing description: %s", body)
	}
	// Cache-Control: no-store on partials.
	if cc := rr.Result().Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control: got %q", cc)
	}
}

func TestPartial_AppliesSearchFilter(t *testing.T) {
	installFakeTask(t, `[
		{"id":1,"uuid":"a","description":"buy milk","status":"pending","entry":"20260501T120000Z"},
		{"id":2,"uuid":"b","description":"call mum","status":"pending","entry":"20260501T120000Z"}
	]`)
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list?report=next&q=milk", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "buy milk") {
		t.Errorf("expected milk task: %s", body)
	}
	if strings.Contains(body, "call mum") {
		t.Errorf("expected mum filtered out: %s", body)
	}
}

// TestPartial_AppliesSortParam confirms ?sort=description:asc reorders the
// rendered list. Two tasks with distinct descriptions whose alphabetical order
// inverts the default urgency-desc order.
func TestPartial_AppliesSortParam(t *testing.T) {
	installFakeTask(t, `[
		{"id":1,"uuid":"a","description":"zebra","status":"pending","entry":"20260501T120000Z","urgency":9},
		{"id":2,"uuid":"b","description":"apple","status":"pending","entry":"20260501T120000Z","urgency":1}
	]`)
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list?report=next&sort=description:asc", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	appleIdx := strings.Index(body, "apple")
	zebraIdx := strings.Index(body, "zebra")
	if appleIdx == -1 || zebraIdx == -1 {
		t.Fatalf("expected both tasks: appleIdx=%d zebraIdx=%d", appleIdx, zebraIdx)
	}
	if appleIdx > zebraIdx {
		t.Errorf("description:asc: apple should precede zebra; got appleIdx=%d zebraIdx=%d", appleIdx, zebraIdx)
	}
}

// TestPartial_RendersSortHeader confirms the sort header is rendered into
// every partial response so the column links show up after a polling swap.
func TestPartial_RendersSortHeader(t *testing.T) {
	installFakeTask(t, `[]`)
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/partials/list?report=next", nil)
	rr := httptest.NewRecorder()
	v.Partial(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, label := range []string{"Urgency", "Due", "Project", "Description", "Created"} {
		if !strings.Contains(body, label) {
			t.Errorf("sort header missing %q label: %s", label, body)
		}
	}
}
