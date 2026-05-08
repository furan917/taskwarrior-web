package handlers

import (
	"net/http"
	"strings"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// Partial serves /partials/list?report=<name> or /partials/list?project=<name>.
// Optional &q=<text> performs a case-insensitive substring filter against
// description, project, and tags. Optional &sort=<key>[:<dir>] reorders the
// list - see views.ParseSort for the accepted shape. Returns ONLY the <ul>
// fragment (plus the sort header) for HTMX swap into the polling container.
func (v *Views) Partial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	report := q.Get("report")
	project := q.Get("project")
	search := strings.TrimSpace(q.Get("q"))
	sortSpec := views.ParseSort(q)

	var filter string
	switch {
	case project != "":
		if !tw.ProjectPattern.MatchString(project) {
			http.Error(w, "invalid project", http.StatusBadRequest)
			return
		}
		filter = "project:" + project
	case report != "":
		// Reports come in two flavours: the curated four (next/ready/agenda/
		// forecast) handled by specForReport, and the dynamic set (built-in
		// shortlist + user-defined taskrc reports) routed through
		// dynamicReportSpec. The polling endpoint must accept both or the
		// /r/{name} pages 400 every 30s.
		if !tw.ReportNamePattern.MatchString(report) {
			http.Error(w, "invalid report name", http.StatusBadRequest)
			return
		}
		spec, ok := v.specForReport(report)
		if !ok {
			spec, ok = v.dynamicReportSpec(r.Context(), report)
		}
		if !ok {
			http.Error(w, "unknown report", http.StatusBadRequest)
			return
		}
		filter = spec.filter
	default:
		http.Error(w, "report or project required", http.StatusBadRequest)
		return
	}

	tasks, err := v.fetch(r, filter)
	if err != nil {
		v.Logger.Error("partial fetch failed", "filter", filter, "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	if search != "" {
		tasks = filterTasks(tasks, search)
	}

	w.Header().Set("Cache-Control", "no-store")
	// Partial swaps re-render the empty-state when a search returns nothing,
	// so they need the active-context name to show the "context X is hiding
	// results" copy. Read fresh per request - the user could have flipped
	// the context out-of-band since the last full-page render.
	active := activeContext(v.TW, r)
	renderHTML(w, r, "Partial", views.TaskListFragment(report, project, search, tasks, sortSpec, active), v.Logger)
}

// filterTasks returns the subset of tasks whose description, project, or any
// tag contains q (case-insensitive substring match). q is assumed already
// trimmed and non-empty.
func filterTasks(tasks []tw.Task, q string) []tw.Task {
	needle := strings.ToLower(q)
	out := make([]tw.Task, 0, len(tasks))
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Description), needle) {
			out = append(out, t)
			continue
		}
		if t.Project != "" && strings.Contains(strings.ToLower(t.Project), needle) {
			out = append(out, t)
			continue
		}
		matched := false
		for _, tag := range t.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				matched = true
				break
			}
		}
		if matched {
			out = append(out, t)
		}
	}
	return out
}
