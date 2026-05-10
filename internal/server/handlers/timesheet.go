package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

func (v *Views) Timesheet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	view := r.URL.Query().Get("view")
	if view != views.TimesheetViewDay {
		view = views.TimesheetViewWeek
	}

	anchor := views.DateOnly(now)
	if d := r.URL.Query().Get("date"); d != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", d, now.Location()); err == nil {
			anchor = views.DateOnly(parsed)
		}
	}

	page := v.buildPage(r, "Timesheet", "timesheet", false)

	if !v.TW.JournalTimeEnabled(ctx) {
		renderHTML(w, r, "Timesheet", views.TimesheetPage(page, false, views.TimesheetData{}), v.Logger)
		return
	}

	from, to := timesheetWindow(view, anchor)

	tasks, err := v.TW.Export(ctx,
		"(status:pending or status:waiting or status:completed)",
		"modified.after:"+from.UTC().Format("20060102T150405Z"),
	)
	if err != nil {
		v.Logger.Error("timesheet fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	sessions := tw.SessionsInRange(tasks, from, to, now)
	data := BuildTimesheetPage(view, anchor, sessions, now)
	renderHTML(w, r, "Timesheet", views.TimesheetPage(page, true, data), v.Logger)
}

// BuildTimesheetPage assembles TimesheetData from request inputs. Mirrors
// BuildCalendarPage: takes raw inputs, derives all derived state internally so
// the handler stays a thin orchestrator.
func BuildTimesheetPage(view string, anchor time.Time, sessions []tw.Session, now time.Time) views.TimesheetData {
	anchor = views.DateOnly(anchor)
	today := views.DateOnly(now)

	data := views.TimesheetData{
		View:     view,
		Anchor:   anchor,
		TodayURL: fmt.Sprintf("/timesheet?view=%s&date=%s", view, today.Format("2006-01-02")),
		Now:      now,
	}

	switch view {
	case views.TimesheetViewDay:
		data.Title = anchor.Format("Mon 2 January 2006")
		data.PrevURL = fmt.Sprintf("/timesheet?view=day&date=%s", views.AddDays(anchor, -1).Format("2006-01-02"))
		data.NextURL = fmt.Sprintf("/timesheet?view=day&date=%s", views.AddDays(anchor, 1).Format("2006-01-02"))
	default: // TimesheetWeek
		weekStart := views.StartOfMonday(anchor)
		weekEnd := views.AddDays(weekStart, 6)
		data.Title = fmt.Sprintf("%s – %s", weekStart.Format("2 Jan"), weekEnd.Format("2 Jan 2006"))
		data.PrevURL = fmt.Sprintf("/timesheet?view=week&date=%s", views.AddDays(weekStart, -7).Format("2006-01-02"))
		data.NextURL = fmt.Sprintf("/timesheet?view=week&date=%s", views.AddDays(weekStart, 7).Format("2006-01-02"))
	}

	data.Days = groupByDay(view, anchor, sessions, now)
	for _, d := range data.Days {
		data.TotalDuration += d.Total
	}
	return data
}

// timesheetWindow returns the [from, to) fetch window for a given view and anchor.
func timesheetWindow(view string, anchor time.Time) (from, to time.Time) {
	switch view {
	case views.TimesheetViewDay:
		return anchor, views.AddDays(anchor, 1)
	default: // TimesheetWeek
		from = views.StartOfMonday(anchor)
		return from, views.AddDays(from, 7)
	}
}

// groupByDay maps sessions into per-day slots. Week view always returns 7
// slots (Mon–Sun) so the summary row is always complete; day view returns 1.
// Sessions within each slot are sorted chronologically.
func groupByDay(view string, anchor time.Time, sessions []tw.Session, now time.Time) []views.TimesheetDay {
	today := views.DateOnly(now)
	var days []views.TimesheetDay
	switch view {
	case views.TimesheetViewDay:
		days = []views.TimesheetDay{{Date: anchor, IsToday: anchor.Equal(today)}}
	default: // TimesheetViewWeek
		weekStart := views.StartOfMonday(anchor)
		days = make([]views.TimesheetDay, 7)
		for i := range days {
			d := views.AddDays(weekStart, i)
			days[i] = views.TimesheetDay{Date: d, IsToday: d.Equal(today)}
		}
	}

	// Index date → slot for O(1) placement.
	idx := make(map[time.Time]int, len(days))
	for i, d := range days {
		idx[d.Date] = i
	}

	for _, s := range sessions {
		key := views.DateOnly(s.Start.Local())
		i, ok := idx[key]
		if !ok {
			continue
		}
		days[i].Sessions = append(days[i].Sessions, s)
		days[i].Total += s.Duration(now)
	}

	for i := range days {
		sort.Slice(days[i].Sessions, func(a, b int) bool {
			return days[i].Sessions[a].Start.Before(days[i].Sessions[b].Start)
		})
	}
	return days
}

// EnableTimeTracking handles POST /timesheet/enable-tracking. Writes
// journal.time=yes to ~/.taskrc and navigates back to the timesheet.
func (v *Views) EnableTimeTracking(w http.ResponseWriter, r *http.Request) {
	if err := v.TW.EnableJournalTime(r.Context()); err != nil {
		v.Logger.Error("enable journal.time failed", "err", err)
		http.Error(w, "could not enable time tracking", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/timesheet")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/timesheet", http.StatusSeeOther)
}
