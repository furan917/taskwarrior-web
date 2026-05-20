package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

func (v *Views) Kanban(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	pending, err := v.exportWithContext(r, "status:pending")
	if err != nil {
		v.Logger.Error("kanban pending fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	sevenDaysAgo := views.AddDays(views.DateOnly(now), -7)
	done, err := v.exportWithContext(r, "status:completed",
		"end.after:"+sevenDaysAgo.UTC().Format("20060102T150405Z"))
	if err != nil {
		v.Logger.Error("kanban done fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	var inbox, backlog, inProgress, onHold []tw.Task
	for _, t := range pending {
		if t.IsRecurringParent() {
			continue
		}
		switch views.KanbanColumnFor(t) {
		case "inbox":
			inbox = append(inbox, t)
		case "inprogress":
			inProgress = append(inProgress, t)
		case "onhold":
			onHold = append(onHold, t)
		default:
			backlog = append(backlog, t)
		}
	}

	// Sort pending columns by urgency descending. Taskwarrior's urgency
	// score already incorporates due dates (high weight), blocked status
	// (-5 for tasks with unmet dependencies), blocking bonus (+8), active
	// status, and scheduled dates — so the cap keeps the most actionable
	// tasks visible without any additional logic.
	byUrgency := func(tasks []tw.Task) {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].Urgency > tasks[j].Urgency
		})
	}
	byUrgency(inbox)
	byUrgency(backlog)
	byUrgency(inProgress)
	byUrgency(onHold)

	// Done: most recently completed first (End is ISO-8601 so string sort works).
	sort.Slice(done, func(i, j int) bool { return done[i].End > done[j].End })

	data := views.KanbanData{
		Inbox:      views.CapKanbanCol(inbox),
		Backlog:    views.CapKanbanCol(backlog),
		InProgress: views.CapKanbanCol(inProgress),
		OnHold:     views.CapKanbanCol(onHold),
		Done:       views.CapKanbanCol(done),
	}

	page := v.buildPage(r, "Kanban", "kanban", false)
	renderHTML(w, r, "Kanban", views.KanbanPage(page, data), v.Logger)
}
