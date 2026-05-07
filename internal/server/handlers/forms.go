package handlers

import (
	"log/slog"
	"net/http"

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
