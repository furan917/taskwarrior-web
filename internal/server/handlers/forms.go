package handlers

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

type Forms struct {
	TW     *tw.Client
	Logger *slog.Logger
}

func (f *Forms) Add(w http.ResponseWriter, r *http.Request) {
	udas := f.TW.UDAsCached(r.Context())
	projects := f.TW.ProjectsCached(r.Context())
	tags := f.TW.TagsCached(r.Context())
	active := activeContext(f.TW, r)
	openTasks := f.openTasksForDeps(r, "")
	// Build the dropdown options for the modal's Context picker. Each entry
	// carries the prefill values derived from the context's read filter via
	// ContextPrefill so the JS handler can swap Tags / Project inputs on
	// change without a server round-trip. The active context's prefill
	// also seeds the initial form state below.
	contexts := f.TW.ContextsCached(r.Context())
	options := make([]views.ContextOption, 0, len(contexts))
	var prefillProject, prefillTags string
	for _, c := range contexts {
		proj, tag := views.ContextPrefill(c.ReadFilter)
		options = append(options, views.ContextOption{
			Name:           c.Name,
			PrefillTags:    tag,
			PrefillProject: proj,
		})
		if c.Name == active {
			prefillProject = proj
			prefillTags = tag
		}
	}
	renderHTML(w, r, "ModalAdd", views.ModalAdd(CSRFToken(r.Context()), udas, projects, tags, active, openTasks, prefillProject, prefillTags, options), f.Logger)
}

func (f *Forms) Edit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	tasks, err := f.TW.Export(r.Context(), id)
	if err != nil {
		f.Logger.Error("edit form fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}
	if len(tasks) == 0 {
		http.NotFound(w, r)
		return
	}
	udas := f.TW.UDAsCached(r.Context())
	projects := f.TW.ProjectsCached(r.Context())
	tags := f.TW.TagsCached(r.Context())
	openTasks := f.openTasksForDeps(r, tasks[0].UUID)
	renderHTML(w, r, "ModalEdit", views.ModalEdit(CSRFToken(r.Context()), tasks[0], udas, projects, tags, openTasks), f.Logger)
}

// Sessions handles GET /forms/sessions/{id}. Renders the retroactive
// time-tracking editor as a stacked <dialog> in the #sessions-modal slot.
// Query params:
//   - day=YYYY-MM-DD: scope the initial visible list to that day. The
//     "Show all" toggle in the rendered dialog re-fetches without this
//     param to expand to full history.
//   - from=timesheet: flips the header to show the back-chevron "Edit
//     task" affordance so a user who arrived from /timesheet can pivot
//     to the full edit modal without retracing.
//
// Emits HX-Trigger: showSessions on success so the client-side JS opens
// the dialog reliably regardless of whether HTMX's swap completed before
// or after the script polled for the element.
func (f *Forms) Sessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !tw.IDPattern.MatchString(id) {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	tasks, err := f.TW.Export(r.Context(), id)
	if err != nil {
		f.Logger.Error("sessions form fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}
	if len(tasks) == 0 {
		http.NotFound(w, r)
		return
	}
	t := tasks[0]

	// Sort newest-first so the editor opens at the user's most-recent
	// activity (the entry they're most likely correcting). The aggregator
	// emits chronological order by default; we re-sort here once so the
	// page-builder and group helper can rely on the contract.
	sessions := tw.ParseSessions(t, time.Now())
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Start.After(sessions[j].Start)
	})

	q := r.URL.Query()

	// Day filter: looks like YYYY-MM-DD. Anything else (or empty) means
	// "show all"; we deliberately do not 400 on a malformed day so a
	// stale bookmark just renders the full list instead of erroring.
	day := q.Get("day")
	if day != "" {
		if _, err := time.ParseInLocation("2006-01-02", day, time.Local); err != nil {
			day = ""
		}
	}

	// Pagination offset is in DAYS, not sessions. Clamped to a sane
	// upper bound to defend against the URL being hand-crafted with a
	// 1e9 offset that would still make us walk the full session list.
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10000 {
			offset = n
		}
	}

	fromTimesheet := q.Get("from") == "timesheet"

	page := views.BuildSessionsPage(sessions, day, fromTimesheet, offset)

	// ?fragment=1 is the "Earlier days" path: return only the new date-
	// groups + (optionally) the next load-more button, NOT the dialog
	// chrome. The button uses hx-target="this" hx-swap="outerHTML" so
	// each click chain-appends a page without resetting scroll or
	// existing datetime-local input values.
	if q.Get("fragment") == "1" {
		w.Header().Set("Cache-Control", "no-store")
		renderHTML(w, r, "SessionsFragment", views.SessionsListFragment(t.UUID, page), f.Logger)
		return
	}

	w.Header().Set("HX-Trigger", "showSessions")
	renderHTML(w, r, "Sessions", views.SessionsModal(CSRFToken(r.Context()), t, page), f.Logger)
}

// openTasksForDeps fetches every pending or waiting task to feed the
// dependency picker's datalist. The task currently being edited is excluded
// (a task cannot depend on itself); excludeUUID is empty on the Add modal.
//
// Errors are folded into an empty slice so the modal still renders without a
// dep picker if Taskwarrior is briefly unreachable - dependencies are
// optional, so a degraded render is preferable to a 500 that blocks plain
// edits.
func (f *Forms) openTasksForDeps(r *http.Request, excludeUUID string) []tw.Task {
	tasks, err := f.TW.Export(r.Context(), "(status:pending or status:waiting)")
	if err != nil {
		f.Logger.Warn("dep picker open-tasks fetch failed", "err", err)
		return nil
	}
	if excludeUUID == "" {
		return tasks
	}
	out := tasks[:0]
	for _, t := range tasks {
		if t.UUID == excludeUUID {
			continue
		}
		out = append(out, t)
	}
	return out
}
