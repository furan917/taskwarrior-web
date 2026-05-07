package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furan917/taskwarrior-web/internal/config"
	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// Views holds dependencies for read-side handlers. Constructed once in
// server.New and shared across goroutines (TW.Client is goroutine-safe; the
// logger is from slog).
type Views struct {
	TW     *tw.Client
	Logger *slog.Logger
}

// reportSpec is the static metadata for a named report. The filter field for
// "agenda" and "forecast" is treated as a fallback; the live value comes from
// `task _get rc.report.<name>.filter` and is cached on tw.Client.
type reportSpec struct {
	filter string
	title  string
	active string
}

var reportSpecs = map[string]reportSpec{
	// Next is the broad pending umbrella (urgency-sorted, top 50). The OR
	// clause catches lapsed-wait tasks that Taskwarrior 3.x with taskchampion
	// hasn't auto-promoted in storage yet — pure query-time fix, no writes.
	"next": {"(status:pending or (status:waiting and wait.before:now)) limit:50", "Next", "next"},
	"ready":    {"+READY", "Ready", "ready"},
	"agenda":   {"(status:pending or status:waiting) (due.before:14d or wait.before:14d)", "Agenda · 14 days", "agenda"},
	"forecast": {"(status:pending or status:waiting) (due.before:30d or wait.before:30d)", "Forecast · 30 days", "forecast"},
}

// reportFilterTimeout aliases config.ReportFilterTimeout to keep the
// existing site comment readable; the canonical source of truth is the
// config package.
const reportFilterTimeout = config.ReportFilterTimeout

// specForReport returns the report spec with its filter resolved against
// .taskrc for "agenda" and "forecast"; for other reports the static filter is
// returned as-is. The first call per report name shells `task _get
// rc.report.<name>.filter` (cached on tw.Client); empty/error responses fall
// back to the hardcoded spec filter.
func (v *Views) specForReport(name string) (reportSpec, bool) {
	spec, ok := reportSpecs[name]
	if !ok {
		return reportSpec{}, false
	}
	switch name {
	case "agenda", "forecast":
		ctx, cancel := context.WithTimeout(context.Background(), reportFilterTimeout)
		defer cancel()
		if got := v.TW.ReportFilterCached(ctx, name); got != "" {
			spec.filter = got
		}
	}
	return spec, true
}

func (v *Views) Home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ready", http.StatusFound)
}

// Report returns a handler for one of the named reports.
func (v *Views) Report(name string) http.HandlerFunc {
	if _, ok := reportSpecs[name]; !ok {
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		spec, _ := v.specForReport(name)
		tasks, err := v.fetch(r, spec.filter)
		if err != nil {
			v.Logger.Error("report fetch failed", "report", name, "err", err)
			http.Error(w, "task export failed", http.StatusInternalServerError)
			return
		}
		page := v.buildPage(r, spec.title, spec.active, true)
		renderHTML(w, r, "Report",
			views.ListPage(page, name, "", tasks, views.ParseSort(r.URL.Query())),
			v.Logger, "report", name)
	}
}

// Labels lists every project and every tag currently attached to open
// (pending or waiting) tasks, with counts. Each entry links into the
// corresponding /project/{name} or /tag/{name} drilldown.
func (v *Views) Labels(w http.ResponseWriter, r *http.Request) {
	tasks, err := v.exportWithContext(r, "(status:pending or status:waiting)")
	if err != nil {
		v.Logger.Error("labels fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}
	projectCounts := map[string]int{}
	tagCounts := map[string]int{}
	for _, t := range tasks {
		if t.Project != "" {
			projectCounts[t.Project]++
		}
		for _, tag := range t.Tags {
			tagCounts[tag]++
		}
	}
	page := v.buildPage(r, "Browse", "browse", false)
	renderHTML(w, r, "Labels",
		views.LabelsPage(page, sortedCounted(projectCounts), sortedCounted(tagCounts)),
		v.Logger)
}

// sortedCounted returns name/count pairs sorted by count desc, then name asc.
func sortedCounted(m map[string]int) []views.Counted {
	out := make([]views.Counted, 0, len(m))
	for k, v := range m {
		out = append(out, views.Counted{Name: k, Count: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Tag handles /tag/{name} - filters tasks by `+name`.
func (v *Views) Tag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !tw.TagPattern.MatchString(name) {
		http.Error(w, "invalid tag name", http.StatusBadRequest)
		return
	}
	tasks, err := v.fetch(r, "+"+name)
	if err != nil {
		v.Logger.Error("tag fetch failed", "tag", name, "err", err)
		http.Error(w, "task export failed", http.StatusInternalServerError)
		return
	}
	page := v.buildPage(r, "+"+name, "", true)
	renderHTML(w, r, "Tag",
		views.ListPage(page, "tag-"+name, "", tasks, views.ParseSort(r.URL.Query())),
		v.Logger, "tag", name)
}

// Project handles /project/{name}.
func (v *Views) Project(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !tw.ProjectPattern.MatchString(name) {
		http.Error(w, "invalid project name", http.StatusBadRequest)
		return
	}
	tasks, err := v.fetch(r, "project:"+name)
	if err != nil {
		v.Logger.Error("project fetch failed", "project", name, "err", err)
		http.Error(w, "task export failed", http.StatusInternalServerError)
		return
	}
	page := v.buildPage(r, "project: "+name, "", true)
	renderHTML(w, r, "Project",
		views.ListPage(page, "", name, tasks, views.ParseSort(r.URL.Query())),
		v.Logger, "project", name)
}

// Calendar renders the month/week/day calendar view. Open tasks (pending or
// waiting) with a `due` date appear; if `scheduled` (or `wait`, falling back)
// is also set and earlier than `due`, the task spans every day in that range
// as a per-cell chip with rounded-corner cues at the edges.
//
// Query params (both optional):
//   - view=month|week|day  (default: month)
//   - date=YYYY-MM-DD       (default: today)
func (v *Views) Calendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := q.Get("view")
	if view == "" {
		view = views.CalendarMonth
	}
	switch view {
	case views.CalendarMonth, views.CalendarWeek, views.CalendarDay:
	default:
		http.Error(w, "invalid view", http.StatusBadRequest)
		return
	}

	now := time.Now()
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if d := q.Get("date"); d != "" {
		parsed, err := time.ParseInLocation("2006-01-02", d, now.Location())
		if err != nil {
			http.Error(w, "invalid date (want YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
		anchor = parsed
	}

	tasks, err := v.exportWithContext(r, "(status:pending or status:waiting)")
	if err != nil {
		v.Logger.Error("calendar fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	page := BuildCalendarPage(view, anchor, tasks)
	page.Page = v.buildPage(r, "Calendar", "calendar", false)

	renderHTML(w, r, "Calendar", views.Calendar(page), v.Logger)
}

// exportWithContext wraps tw.Client.Export, prepending the active context's
// read filter as a parenthesised AND clause when one is set. Taskwarrior
// 3.x's `task export` does NOT honour the active context implicitly (unlike
// report commands like `task list` / `task next`), so reports rendered in
// the web UI would otherwise leak tasks from outside the active context.
// Composing explicitly here is the only reliable fix.
//
// Empty filters are dropped so we don't pass empty argv elements; an empty
// active-context name means no context clause is added.
func (v *Views) exportWithContext(r *http.Request, filters ...string) ([]tw.Task, error) {
	args := make([]string, 0, len(filters)+1)
	if cf := v.activeContextFilter(r); cf != "" {
		args = append(args, "("+cf+")")
	}
	for _, f := range filters {
		if f != "" {
			args = append(args, f)
		}
	}
	return v.TW.Export(r.Context(), args...)
}

// fetch is exportWithContext + the URL's sort spec applied. Used by the
// list-style read handlers (Report / Tag / Project / partials) where the
// URL carries an optional ?sort=key:dir.
func (v *Views) fetch(r *http.Request, filter string) ([]tw.Task, error) {
	tasks, err := v.exportWithContext(r, filter)
	if err != nil {
		return nil, err
	}
	applySort(tasks, views.ParseSort(r.URL.Query()))
	return tasks, nil
}

// buildPage assembles the views.Page envelope shared by every read handler.
// Replaces the duplicated CSRFToken / ActiveContext / Contexts plumbing
// that used to live in five copies (Report / Labels / Tag / Project /
// Calendar / Done). hasTaskList drives the keyHint footer's row-action
// keybindings; pages without a task list (Labels, Calendar, Done before
// the v5 row chrome unification) pass false to suppress them.
func (v *Views) buildPage(r *http.Request, title, activeView string, hasTaskList bool) views.Page {
	active := activeContext(v.TW, r)
	return views.Page{
		Title:         title,
		ActiveView:    activeView,
		CSRFToken:     csrfToken(r),
		HasTaskList:   hasTaskList,
		ActiveContext: active,
		Contexts:      namedContextsForRender(v.TW, r, active),
	}
}

// activeContextFilter returns the read filter of the currently-active
// context, or empty string if no context is active or the filter contains
// rc.* override tokens (defence in depth - see tw.Context.SafeReadFilter).
// Reads the per-request active name (live, never cached) and looks up
// the cached read filter.
func (v *Views) activeContextFilter(r *http.Request) string {
	name := activeContext(v.TW, r)
	if name == "" {
		return ""
	}
	for _, c := range v.TW.ContextsCached(r.Context()) {
		if c.Name == name {
			return c.SafeReadFilter()
		}
	}
	return ""
}

// applySort orders tasks in place per the spec. Stable sort preserves prior
// ordering for equal keys, which matters when sorting by description / project
// with many empty values.
func applySort(tasks []tw.Task, s views.SortSpec) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		var less bool
		switch s.Key {
		case "urgency":
			less = a.Urgency < b.Urgency
		case "due":
			// Empty due strings sort last regardless of direction so tasks with
			// a date always surface above undated ones.
			if a.Due == "" && b.Due == "" {
				return false
			}
			if a.Due == "" {
				return false
			}
			if b.Due == "" {
				return true
			}
			less = a.Due < b.Due
		case "project":
			less = a.Project < b.Project
		case "description":
			less = strings.ToLower(a.Description) < strings.ToLower(b.Description)
		case "entry":
			less = a.Entry < b.Entry
		default:
			return false
		}
		if s.Asc {
			return less
		}
		return !less
	})
}

// csrfToken extracts the per-request token set by the CSRF middleware.
func csrfToken(r *http.Request) string { return CSRFToken(r.Context()) }

// Done shows recently completed tasks. Default window: last 14 days; the
// window can be widened via `?days=N` (clamped to 1..90). Sorted by Modified
// descending — for completed tasks Taskwarrior sets `modified` to the
// completion timestamp, which is a close-enough proxy for `end` until we
// surface that field on tw.Task directly.
func (v *Views) Done(w http.ResponseWriter, r *http.Request) {
	days := 14
	if s := r.URL.Query().Get("days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 90 {
			days = n
		}
	}
	// Pass the two clauses as separate argv elements so Taskwarrior treats
	// them as a clean implicit-AND. Combined into a single argv the parser
	// loses the AND when a `status:` reset is involved and silently returns
	// zero rows - which is why marked-done tasks weren't appearing here.
	tasks, err := v.exportWithContext(r, "status:completed", fmt.Sprintf("end.after:now-%dd", days))
	if err != nil {
		v.Logger.Error("done fetch failed", "err", err)
		http.Error(w, "task export failed", http.StatusInternalServerError)
		return
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Modified > tasks[j].Modified
	})
	page := v.buildPage(r, fmt.Sprintf("Done · last %d days", days), "done", false)
	renderHTML(w, r, "Done", views.DonePage(page, tasks, days), v.Logger)
}
