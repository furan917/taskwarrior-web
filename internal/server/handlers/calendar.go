package handlers

import (
	"fmt"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
	"github.com/furan917/taskwarrior-web-portal/internal/views"
)

// BuildCalendarPage assembles the rendered grid for the requested view.
// Lives in handlers because it's controller logic: take request inputs,
// shape them into a views envelope. The templ types stay in views.
func BuildCalendarPage(view string, anchor time.Time, tasks []tw.Task) views.CalendarPage {
	anchor = views.DateOnly(anchor)
	now := views.DateOnly(time.Now().In(anchor.Location()))

	p := views.CalendarPage{
		View:   view,
		Anchor: anchor,
	}

	// Compute the grid's first/last visible date and the period label.
	var gridStart, gridEnd time.Time
	switch view {
	case views.CalendarWeek:
		gridStart = views.StartOfMonday(anchor)
		gridEnd = views.AddDays(gridStart, 6)
		p.Title = "Week of " + gridStart.Format("2 January 2006")
		p.PrevURL = fmt.Sprintf("/calendar?view=week&date=%s", views.AddDays(gridStart, -7).Format("2006-01-02"))
		p.NextURL = fmt.Sprintf("/calendar?view=week&date=%s", views.AddDays(gridStart, 7).Format("2006-01-02"))
	case views.CalendarDay:
		gridStart = anchor
		gridEnd = anchor
		p.Title = anchor.Format("Mon 2 January 2006")
		p.PrevURL = fmt.Sprintf("/calendar?view=day&date=%s", views.AddDays(anchor, -1).Format("2006-01-02"))
		p.NextURL = fmt.Sprintf("/calendar?view=day&date=%s", views.AddDays(anchor, 1).Format("2006-01-02"))
	default: // CalendarMonth
		monthStart := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
		monthEnd := views.AddDays(views.AddMonths(monthStart, 1), -1)
		gridStart = views.StartOfMonday(monthStart)
		// Pad until the grid ends on a Sunday so the month view always
		// renders complete weeks.
		gridEnd = monthEnd
		for gridEnd.Weekday() != time.Sunday {
			gridEnd = views.AddDays(gridEnd, 1)
		}
		p.Title = anchor.Format("January 2006")
		p.PrevURL = fmt.Sprintf("/calendar?view=month&date=%s", views.AddMonths(monthStart, -1).Format("2006-01-02"))
		p.NextURL = fmt.Sprintf("/calendar?view=month&date=%s", views.AddMonths(monthStart, 1).Format("2006-01-02"))
	}
	p.TodayURL = fmt.Sprintf("/calendar?view=%s&date=%s", view, now.Format("2006-01-02"))

	// Map each visible date to its day index for fast chip placement.
	totalDays := int(gridEnd.Sub(gridStart).Hours()/24) + 1
	if totalDays < 1 {
		totalDays = 1
	}
	cells := make([]views.CalendarCell, totalDays)
	for i := 0; i < totalDays; i++ {
		d := views.AddDays(gridStart, i)
		inPeriod := true
		if view == views.CalendarMonth {
			inPeriod = d.Month() == anchor.Month() && d.Year() == anchor.Year()
		}
		cells[i] = views.CalendarCell{
			Date:     d,
			InPeriod: inPeriod,
			IsToday:  d.Equal(now),
		}
	}

	// Walk tasks and place chips.
	var dayTasks []tw.Task
	for _, t := range tasks {
		start, end, ok := views.TaskSpan(t)
		if !ok {
			continue
		}
		// Skip tasks that don't intersect the visible window.
		if end.Before(gridStart) || start.After(gridEnd) {
			continue
		}
		// Clip to the visible window.
		clipStart := start
		if clipStart.Before(gridStart) {
			clipStart = gridStart
		}
		clipEnd := end
		if clipEnd.After(gridEnd) {
			clipEnd = gridEnd
		}

		startIdx := int(clipStart.Sub(gridStart).Hours() / 24)
		endIdx := int(clipEnd.Sub(gridStart).Hours() / 24)
		single := start.Equal(end)

		for i := startIdx; i <= endIdx; i++ {
			if i < 0 || i >= len(cells) {
				continue
			}
			pos := "middle"
			cellDate := cells[i].Date
			switch {
			case single:
				pos = "single"
			case cellDate.Equal(start):
				pos = "start"
			case cellDate.Equal(end):
				pos = "end"
			default:
				// Also use rounded ends when clipped at the visible window
				// boundary - the bar visually terminates at the grid edge.
				if cellDate.Equal(gridStart) && start.Before(gridStart) {
					pos = "start"
				}
				if cellDate.Equal(gridEnd) && end.After(gridEnd) {
					pos = "end"
				}
			}
			cells[i].Chips = append(cells[i].Chips, views.CalendarChip{Task: t, Position: pos})
		}

		// Day-view list: collect every task active on the anchor.
		if view == views.CalendarDay {
			if !start.After(anchor) && !end.Before(anchor) {
				dayTasks = append(dayTasks, t)
			}
		}
	}

	// Stable sort within each cell: urgency desc so red bars float to the top.
	for i := range cells {
		views.SortChipsByUrgency(cells[i].Chips)
	}

	if view == views.CalendarDay {
		views.SortTasksByUrgency(dayTasks)
		p.DayTasks = dayTasks
	}
	p.Cells = cells
	return p
}
