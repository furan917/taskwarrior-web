package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// TestViews_FilterCache_ResolvesOnce confirms that the per-name `task _get
// rc.report.<name>.filter` lookup runs exactly once per Client lifetime,
// regardless of how many specForReport calls hit the same report name.
//
// We install a fake `task` binary that records every `_get` invocation by
// appending to a counter file, then call specForReport repeatedly. The
// expected count is 1 per distinct (cached) report name across the whole run.
func TestViews_FilterCache_ResolvesOnce(t *testing.T) {
	dir := t.TempDir()
	getLog := filepath.Join(dir, "get-calls")
	scriptDir := t.TempDir()
	script := filepath.Join(scriptDir, "task")
	body := `#!/bin/sh
case "$*" in
  *"_get rc.report.agenda.filter"*)
    echo agenda >> ` + getLog + `
    printf '+RESOLVED_AGENDA'
    exit 0
    ;;
  *"_get rc.report.forecast.filter"*)
    echo forecast >> ` + getLog + `
    printf '+RESOLVED_FORECAST'
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake task: %v", err)
	}
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}

	for i := 0; i < 5; i++ {
		spec, ok := v.specForReport("agenda")
		if !ok || spec.filter != "+RESOLVED_AGENDA" {
			t.Fatalf("agenda call %d: ok=%v filter=%q", i, ok, spec.filter)
		}
		spec, ok = v.specForReport("forecast")
		if !ok || spec.filter != "+RESOLVED_FORECAST" {
			t.Fatalf("forecast call %d: ok=%v filter=%q", i, ok, spec.filter)
		}
	}

	// Static reports never trigger the cache and stay on their hardcoded
	// filter.
	if spec, _ := v.specForReport("next"); spec.filter != "(status:pending or (status:waiting and wait.before:now)) limit:50" {
		t.Errorf("next filter mutated: %q", spec.filter)
	}

	data, err := os.ReadFile(getLog)
	if err != nil {
		t.Fatalf("read getLog: %v", err)
	}
	gotAgenda := strings.Count(string(data), "agenda")
	gotForecast := strings.Count(string(data), "forecast")
	if gotAgenda != 1 {
		t.Errorf("agenda _get calls: got %d want 1", gotAgenda)
	}
	if gotForecast != 1 {
		t.Errorf("forecast _get calls: got %d want 1", gotForecast)
	}
}

// TestViews_specForReport_UnknownReport ensures unknown report names return
// ok=false and don't touch the cache.
func TestViews_specForReport_UnknownReport(t *testing.T) {
	v := &Views{TW: tw.NewClient(), Logger: discardLogger()}
	if _, ok := v.specForReport("does-not-exist"); ok {
		t.Fatal("expected unknown report to return ok=false")
	}
}

func newViewsForTest() *Views {
	return &Views{TW: tw.NewClient(), Logger: discardLogger()}
}

func TestViews_Home_RedirectsToReady(t *testing.T) {
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	v.Home(rr, req)
	if rr.Code != http.StatusFound {
		t.Errorf("status: got %d want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/ready" {
		t.Errorf("location: got %q want /ready", loc)
	}
}

func TestViews_Report_RendersAllNamedReports(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1, "uuid": "u-1", "description": "buy milk",
		"status": "pending", "entry": "20260501T120000Z"
	}]`)
	for _, name := range []string{"next", "ready", "agenda", "forecast"} {
		v := newViewsForTest()
		h := v.Report(name)
		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: got %d want 200; body=%s", name, rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, "buy milk") {
			t.Errorf("%s: body missing description: %s", name, body)
		}
	}
}

// TestViews_Report_RendersDependencyBadgeAndPanel covers the row-badge and
// expand-panel rendering for a task with `depends`. We render the Next report
// (which exercises Row + rowDetailGrid for every task) and assert both
// surfaces are present in the HTML.
func TestViews_Report_RendersDependencyBadgeAndPanel(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "blocked task",
		"status": "pending",
		"entry": "20260501T120000Z",
		"depends": [
			"22222222-3333-4444-5555-666666666666",
			"33333333-4444-5555-6666-777777777777"
		]
	}]`)
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Row-level badge: includes the count and the "blocked" word.
	if !strings.Contains(body, "2 blocked") {
		t.Errorf("body missing '2 blocked' badge: %s", body)
	}
	// Expand-panel "Blocked by" header and links to /forms/edit/<uuid>.
	if !strings.Contains(body, "Blocked by") {
		t.Errorf("body missing 'Blocked by' header")
	}
	if !strings.Contains(body, "/forms/edit/22222222-3333-4444-5555-666666666666") {
		t.Errorf("body missing link to first dep")
	}
	if !strings.Contains(body, "/forms/edit/33333333-4444-5555-6666-777777777777") {
		t.Errorf("body missing link to second dep")
	}
	// Truncated UUID: first 8 hex chars + ellipsis.
	if !strings.Contains(body, "22222222") {
		t.Errorf("body missing truncated dep UUID")
	}
}

// TestViews_Report_NoDependencyBadgeWhenEmpty: a task with no depends list
// must NOT carry the badge or the expand-panel section. Defends against an
// accidental `len(t.Depends) >= 0` regression.
func TestViews_Report_NoDependencyBadgeWhenEmpty(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"description": "free task",
		"status": "pending",
		"entry": "20260501T120000Z"
	}]`)
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "blocked") {
		t.Errorf("body unexpectedly mentions 'blocked' for a task with no deps: %s", body)
	}
	if strings.Contains(body, "Blocked by") {
		t.Errorf("body unexpectedly has 'Blocked by' section")
	}
}

func TestViews_Report_UnknownReturns404(t *testing.T) {
	v := newViewsForTest()
	h := v.Report("nope")
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d want 404", rr.Code)
	}
}

func TestViews_Report_500WhenExportFails(t *testing.T) {
	installFailingTask(t)
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestViews_Project_RejectsBadName(t *testing.T) {
	v := newViewsForTest()
	for _, name := range []string{"../etc", "+team", "team alpha", "team;ls", ""} {
		req := httptest.NewRequest(http.MethodGet, "/project/x", nil)
		req.SetPathValue("name", name)
		rr := httptest.NewRecorder()
		v.Project(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("name %q: got %d want 400", name, rr.Code)
		}
	}
}

func TestViews_Project_Renders(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1, "uuid": "u-1", "description": "demo",
		"status": "pending", "entry": "20260501T120000Z", "project": "shop"
	}]`)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/project/shop", nil)
	req.SetPathValue("name", "shop")
	rr := httptest.NewRecorder()
	v.Project(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "demo") {
		t.Errorf("body missing demo: %s", rr.Body.String())
	}
}

func TestViews_Tag_RejectsBadName(t *testing.T) {
	v := newViewsForTest()
	for _, name := range []string{"../etc", "+team", "tag alpha", "team;ls", "team.alpha" /* . is invalid for tags */, ""} {
		req := httptest.NewRequest(http.MethodGet, "/tag/x", nil)
		req.SetPathValue("name", name)
		rr := httptest.NewRecorder()
		v.Tag(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("name %q: got %d want 400", name, rr.Code)
		}
	}
}

func TestViews_Tag_Renders(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1, "uuid": "u-1", "description": "tagged",
		"status": "pending", "entry": "20260501T120000Z", "tags": ["urgent"]
	}]`)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/tag/urgent", nil)
	req.SetPathValue("name", "urgent")
	rr := httptest.NewRecorder()
	v.Tag(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "tagged") {
		t.Errorf("body missing tagged: %s", rr.Body.String())
	}
}

func TestViews_Done_DefaultDays(t *testing.T) {
	installFakeTask(t, `[{
		"id": 1, "uuid": "u-1", "description": "done thing",
		"status": "completed", "entry": "20260501T120000Z",
		"modified": "20260501T130000Z"
	}]`)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/done", nil)
	rr := httptest.NewRecorder()
	v.Done(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "done thing") {
		t.Errorf("body missing: %s", rr.Body.String())
	}
}

func TestViews_Done_DaysParamClamped(t *testing.T) {
	installFakeTask(t, "[]")
	v := newViewsForTest()
	for _, d := range []string{"5", "0", "-1", "100", "abc"} {
		req := httptest.NewRequest(http.MethodGet, "/done?days="+url.QueryEscape(d), nil)
		rr := httptest.NewRecorder()
		v.Done(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("days %q: got %d want 200", d, rr.Code)
		}
	}
}

func TestViews_Done_500WhenExportFails(t *testing.T) {
	installFailingTask(t)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/done", nil)
	rr := httptest.NewRecorder()
	v.Done(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestViews_Labels_Renders(t *testing.T) {
	installFakeTask(t, `[
		{"id":1,"uuid":"a","description":"x","status":"pending","entry":"20260501T120000Z","project":"shop","tags":["urgent"]},
		{"id":2,"uuid":"b","description":"y","status":"pending","entry":"20260501T120000Z","project":"shop","tags":["chore","urgent"]}
	]`)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/browse", nil)
	rr := httptest.NewRecorder()
	v.Labels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "shop") {
		t.Errorf("body missing project: %s", body)
	}
	if !strings.Contains(body, "urgent") {
		t.Errorf("body missing tag: %s", body)
	}
}

func TestViews_Labels_500WhenExportFails(t *testing.T) {
	installFailingTask(t)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/browse", nil)
	rr := httptest.NewRecorder()
	v.Labels(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestViews_Calendar_DefaultIsMonth(t *testing.T) {
	installFakeTask(t, "[]")
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rr := httptest.NewRecorder()
	v.Calendar(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d want 200", rr.Code)
	}
}

func TestViews_Calendar_RejectsBadView(t *testing.T) {
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/calendar?view=year", nil)
	rr := httptest.NewRecorder()
	v.Calendar(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestViews_Calendar_RejectsBadDate(t *testing.T) {
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/calendar?date=not-a-date", nil)
	rr := httptest.NewRecorder()
	v.Calendar(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400", rr.Code)
	}
}

func TestViews_Calendar_AcceptsValidQuery(t *testing.T) {
	installFakeTask(t, "[]")
	v := newViewsForTest()
	for _, view := range []string{"month", "week", "day"} {
		req := httptest.NewRequest(http.MethodGet, "/calendar?view="+view+"&date=2026-05-04", nil)
		rr := httptest.NewRecorder()
		v.Calendar(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("view %q: got %d", view, rr.Code)
		}
	}
}

func TestViews_Calendar_500WhenExportFails(t *testing.T) {
	installFailingTask(t)
	v := newViewsForTest()
	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rr := httptest.NewRecorder()
	v.Calendar(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", rr.Code)
	}
}

func TestSortedCounted(t *testing.T) {
	in := map[string]int{"a": 2, "b": 5, "c": 5, "d": 1}
	got := sortedCounted(in)
	// Sorted by count desc, then name asc.
	if got[0].Name != "b" || got[0].Count != 5 {
		t.Errorf("[0]: %+v", got[0])
	}
	if got[1].Name != "c" || got[1].Count != 5 {
		t.Errorf("[1]: %+v", got[1])
	}
	if got[2].Name != "a" || got[2].Count != 2 {
		t.Errorf("[2]: %+v", got[2])
	}
	if got[3].Name != "d" || got[3].Count != 1 {
		t.Errorf("[3]: %+v", got[3])
	}
}

func TestCSRFContext(t *testing.T) {
	// Round-trip: Set then Get returns the same token.
	ctx := WithCSRFToken(t.Context(), "abc")
	if got := CSRFToken(ctx); got != "abc" {
		t.Errorf("got %q want abc", got)
	}
	// Empty context returns empty string, never panics.
	if got := CSRFToken(t.Context()); got != "" {
		t.Errorf("empty ctx: got %q", got)
	}
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	Healthz(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control: %q", cc)
	}
	if !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("content-type: %q", rr.Header().Get("Content-Type"))
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "ok" {
		t.Errorf("body: %q", got)
	}
}

func TestParseSort(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want views.SortSpec
	}{
		{"empty defaults to urgency desc", "", views.SortSpec{Key: "urgency", Asc: false}},
		{"unknown key falls back", "ranking", views.SortSpec{Key: "urgency", Asc: false}},
		{"junk after colon falls back to default dir", "due:nope", views.SortSpec{Key: "due", Asc: true}},
		{"urgency asc explicit", "urgency:asc", views.SortSpec{Key: "urgency", Asc: true}},
		{"urgency desc explicit", "urgency:desc", views.SortSpec{Key: "urgency", Asc: false}},
		{"due default asc", "due", views.SortSpec{Key: "due", Asc: true}},
		{"due desc explicit", "due:desc", views.SortSpec{Key: "due", Asc: false}},
		{"project default asc", "project", views.SortSpec{Key: "project", Asc: true}},
		{"description default asc", "description", views.SortSpec{Key: "description", Asc: true}},
		{"entry default desc", "entry", views.SortSpec{Key: "entry", Asc: false}},
		{"entry asc explicit", "entry:asc", views.SortSpec{Key: "entry", Asc: true}},
		{"whitespace only falls back", "   ", views.SortSpec{Key: "urgency", Asc: false}},
		{"unknown key with dir still falls back", "evil:asc", views.SortSpec{Key: "urgency", Asc: false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			if c.in != "" {
				q.Set("sort", c.in)
			}
			got := views.ParseSort(q)
			if got != c.want {
				t.Errorf("views.ParseSort(%q): got %+v want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestApplySort_Urgency(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Urgency: 2.0},
		{ID: 2, Urgency: 9.0},
		{ID: 3, Urgency: 5.0},
	}
	applySort(tasks, views.SortSpec{Key: "urgency", Asc: false})
	if tasks[0].ID != 2 || tasks[1].ID != 3 || tasks[2].ID != 1 {
		t.Errorf("desc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
	applySort(tasks, views.SortSpec{Key: "urgency", Asc: true})
	if tasks[0].ID != 1 || tasks[1].ID != 3 || tasks[2].ID != 2 {
		t.Errorf("asc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_Due_EmptyLast(t *testing.T) {
	// Tasks without a due date should always sort last regardless of direction
	// so users see scheduled work first when sorting by Due.
	tasks := []tw.Task{
		{ID: 1, Due: "20260601T120000Z"},
		{ID: 2, Due: ""},
		{ID: 3, Due: "20260301T120000Z"},
	}
	applySort(tasks, views.SortSpec{Key: "due", Asc: true})
	if tasks[0].ID != 3 || tasks[1].ID != 1 || tasks[2].ID != 2 {
		t.Errorf("asc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
	applySort(tasks, views.SortSpec{Key: "due", Asc: false})
	if tasks[0].ID != 1 || tasks[1].ID != 3 || tasks[2].ID != 2 {
		t.Errorf("desc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_Project(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Project: "zeta"},
		{ID: 2, Project: "alpha"},
		{ID: 3, Project: "beta"},
	}
	applySort(tasks, views.SortSpec{Key: "project", Asc: true})
	if tasks[0].ID != 2 || tasks[1].ID != 3 || tasks[2].ID != 1 {
		t.Errorf("asc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_Description_CaseInsensitive(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Description: "banana"},
		{ID: 2, Description: "Apple"},
		{ID: 3, Description: "cherry"},
	}
	applySort(tasks, views.SortSpec{Key: "description", Asc: true})
	if tasks[0].ID != 2 || tasks[1].ID != 1 || tasks[2].ID != 3 {
		t.Errorf("asc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_Entry(t *testing.T) {
	tasks := []tw.Task{
		{ID: 1, Entry: "20260101T120000Z"},
		{ID: 2, Entry: "20260601T120000Z"},
		{ID: 3, Entry: "20260301T120000Z"},
	}
	applySort(tasks, views.SortSpec{Key: "entry", Asc: false})
	if tasks[0].ID != 2 || tasks[1].ID != 3 || tasks[2].ID != 1 {
		t.Errorf("entry desc: got order %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_UnknownKey_Noop(t *testing.T) {
	// Unknown sort key shouldn't panic and shouldn't reorder.
	tasks := []tw.Task{{ID: 1}, {ID: 2}, {ID: 3}}
	applySort(tasks, views.SortSpec{Key: "evil", Asc: true})
	if tasks[0].ID != 1 || tasks[1].ID != 2 || tasks[2].ID != 3 {
		t.Errorf("unknown key reordered: got %v %v %v", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestApplySort_EmptySliceNoPanic(t *testing.T) {
	tasks := []tw.Task{}
	applySort(tasks, views.SortSpec{Key: "urgency"})
	if len(tasks) != 0 {
		t.Errorf("empty slice mutated: %v", tasks)
	}
}

// TestViews_Report_AppliesSortFromQuery confirms that ?sort= on a list view
// flows through to the rendered task ordering. We use the urgency-asc spec to
// invert the default and expect the lowest-urgency task to render first.
func TestViews_Report_AppliesSortFromQuery(t *testing.T) {
	installFakeTask(t, `[
		{"id":1,"uuid":"a","description":"low","status":"pending","entry":"20260501T120000Z","urgency":2},
		{"id":2,"uuid":"b","description":"high","status":"pending","entry":"20260501T120000Z","urgency":9}
	]`)
	v := newViewsForTest()
	h := v.Report("next")
	req := httptest.NewRequest(http.MethodGet, "/next?sort=urgency:asc", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	lowIdx := strings.Index(body, ">low<")
	highIdx := strings.Index(body, ">high<")
	if lowIdx == -1 || highIdx == -1 {
		t.Fatalf("expected both tasks in body: low=%d high=%d", lowIdx, highIdx)
	}
	if lowIdx > highIdx {
		t.Errorf("urgency:asc: expected low to render before high; lowIdx=%d highIdx=%d", lowIdx, highIdx)
	}
}
