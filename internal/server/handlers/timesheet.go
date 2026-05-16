package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

const weekSummaryWeeks = 8

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

	// Fetch from the wider of: the view window or the week-summary lookback.
	summaryFrom := views.AddDays(views.StartOfMonday(views.DateOnly(now)), -7*(weekSummaryWeeks-1))
	fetchFrom := from
	if summaryFrom.Before(fetchFrom) {
		fetchFrom = summaryFrom
	}

	tasks, err := v.exportWithContext(r,
		"(status:pending or status:waiting or status:completed)",
		"modified.after:"+fetchFrom.UTC().Format("20060102T150405Z"),
	)
	if err != nil {
		v.Logger.Error("timesheet fetch failed", "err", err)
		http.Error(w, "fetch failed", http.StatusInternalServerError)
		return
	}

	sessions := tw.SessionsInRange(tasks, from, to, now)
	data := BuildTimesheetPage(view, anchor, sessions, now)
	data.WeekSummary = computeWeekSummaries(tasks, weekSummaryWeeks, now)
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
	data.ProjectTotals = buildProjectTimeTree(sessions, now)
	return data
}

// buildProjectTimeTree aggregates sessions by project and builds a collapsed
// dot-notation tree. Returns nil when all sessions share a single project (no
// breakdown to show). The tree mirrors Browse's ProjectTreeNode structure but
// carries time.Duration instead of task counts.
func buildProjectTimeTree(sessions []tw.Session, now time.Time) []views.ProjectTimeNode {
	type accum struct {
		total time.Duration
	}
	flat := map[string]*accum{}
	for _, s := range sessions {
		key := s.Project
		if flat[key] == nil {
			flat[key] = &accum{}
		}
		flat[key].total += s.Duration(now)
	}
	if len(flat) <= 1 {
		return nil
	}

	nodeMap := map[string]*views.ProjectTimeNode{}
	for proj, a := range flat {
		name := strings.Trim(proj, ".")
		if name == "" {
			nodeMap[""] = &views.ProjectTimeNode{
				Segment:      "(no project)",
				FullName:     "",
				SelfTotal:    a.total,
				SubtreeTotal: a.total,
			}
			continue
		}
		segments := strings.Split(name, ".")
		for i, seg := range segments {
			full := strings.Join(segments[:i+1], ".")
			if nodeMap[full] == nil {
				nodeMap[full] = &views.ProjectTimeNode{Segment: seg, FullName: full}
			}
			if i == len(segments)-1 {
				nodeMap[full].SelfTotal = a.total
			}
		}
	}

	ptrChildren := map[string][]*views.ProjectTimeNode{}
	var rootPtrs []*views.ProjectTimeNode
	for full, node := range nodeMap {
		if full == "" {
			rootPtrs = append(rootPtrs, node)
			continue
		}
		dot := strings.LastIndex(full, ".")
		if dot == -1 {
			rootPtrs = append(rootPtrs, node)
		} else {
			parent := full[:dot]
			ptrChildren[parent] = append(ptrChildren[parent], node)
		}
	}

	var buildNode func(full string) views.ProjectTimeNode
	buildNode = func(full string) views.ProjectTimeNode {
		node := *nodeMap[full]
		node.Children = nil
		for _, childPtr := range ptrChildren[full] {
			node.Children = append(node.Children, buildNode(childPtr.FullName))
		}
		node.SubtreeTotal = node.SelfTotal
		for _, c := range node.Children {
			node.SubtreeTotal += c.SubtreeTotal
		}
		sort.Slice(node.Children, func(i, j int) bool {
			return node.Children[i].SubtreeTotal > node.Children[j].SubtreeTotal
		})
		return node
	}

	result := make([]views.ProjectTimeNode, 0, len(rootPtrs))
	for _, r := range rootPtrs {
		if r.FullName == "" {
			result = append(result, *r)
		} else {
			result = append(result, buildNode(r.FullName))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SubtreeTotal > result[j].SubtreeTotal
	})
	return result
}

// computeWeekSummaries builds a newest-first list of weeks (up to n) that have
// tracked time. Each week covers Mon–Sun local time; the current week may be
// partial. Weeks with zero tracked time are omitted.
func computeWeekSummaries(tasks []tw.Task, n int, now time.Time) []views.WeekTotal {
	thisMonday := views.StartOfMonday(views.DateOnly(now))
	var result []views.WeekTotal
	for i := 0; i < n; i++ {
		weekStart := views.AddDays(thisMonday, -7*i)
		weekEnd := views.AddDays(weekStart, 7)
		sessions := tw.SessionsInRange(tasks, weekStart, weekEnd, now)
		var total time.Duration
		for _, s := range sessions {
			total += s.Duration(now)
		}
		if total == 0 {
			continue
		}
		weekLast := views.AddDays(weekEnd, -1)
		label := fmt.Sprintf("%s – %s", weekStart.Format("2 Jan"), weekLast.Format("2 Jan 2006"))
		result = append(result, views.WeekTotal{
			Label:         label,
			URL:           fmt.Sprintf("/timesheet?view=week&date=%s", weekStart.Format("2006-01-02")),
			Total:         total,
			IsCurrentWeek: i == 0,
		})
	}
	return result
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
