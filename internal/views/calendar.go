package views

import (
	"sort"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// DateOnly truncates t to YYYY-MM-DD 00:00 in its existing location. Calendar
// arithmetic uses the local zone so "today" matches the user's wall clock.
//
// Exported because handlers/calendar.go uses it to build calendar pages.
func DateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// StartOfMonday returns the Monday on or before t (Mon-Sun week, UK style).
// Truncates to date-only.
func StartOfMonday(t time.Time) time.Time {
	t = DateOnly(t)
	// time.Weekday: Sunday=0, Monday=1, ..., Saturday=6.
	// We want Monday to map to 0 days back, Sunday to 6 days back.
	wd := int(t.Weekday())
	offset := wd - 1
	if offset < 0 {
		offset = 6
	}
	return t.AddDate(0, 0, -offset)
}

// AddDays is a thin alias around AddDate(0, 0, n) for readability.
func AddDays(t time.Time, n int) time.Time { return t.AddDate(0, 0, n) }

// AddMonths shifts by whole months, preserving the input's day-of-month
// where possible (Go's AddDate normalises overflow, e.g. Jan 31 + 1 month
// becomes Mar 3 - that's fine for calendar nav since we only care about the
// resulting month).
func AddMonths(t time.Time, n int) time.Time { return t.AddDate(0, n, 0) }

// calendarCellClass styles a month-grid cell. Out-of-month cells fade; today
// gets a subtle highlight.
//
// Dark mode: out-of-period cells fade DARKER than the surrounding cells (not
// lighter, which is the light-mode behaviour) so they recede against the
// page. Today's blue-50 highlight becomes a deep-tinted blue-950/40.
func calendarCellClass(c CalendarCell) string {
	base := "min-h-24 border-b border-r border-zinc-100 p-1.5 text-xs dark:border-zinc-800"
	if !c.InPeriod {
		return base + " bg-zinc-50 text-zinc-400 dark:bg-zinc-950/60 dark:text-zinc-500"
	}
	if c.IsToday {
		return base + " bg-blue-50 dark:bg-blue-950/40"
	}
	return base + " bg-white dark:bg-zinc-900"
}

// calendarWeekCellClass is taller than the month cell since week mode has more
// room per day.
func calendarWeekCellClass(c CalendarCell) string {
	base := "min-h-64 border-r border-zinc-100 p-2 text-xs dark:border-zinc-800"
	if c.IsToday {
		return base + " bg-blue-50 dark:bg-blue-950/40"
	}
	return base + " bg-white dark:bg-zinc-900"
}

// calendarDayNumberClass shapes the day-number badge. Today gets a coloured pill.
func calendarDayNumberClass(c CalendarCell) string {
	base := "inline-flex h-6 min-w-6 items-center justify-center rounded-full px-1.5 text-xs font-medium hover:bg-zinc-200 dark:hover:bg-zinc-700"
	if c.IsToday {
		return base + " bg-blue-600 text-white hover:bg-blue-700 dark:hover:bg-blue-500"
	}
	if !c.InPeriod {
		return base + " text-zinc-400 dark:text-zinc-500"
	}
	return base + " text-zinc-700 dark:text-zinc-200"
}

// calendarChipClass colours by urgency and shapes the corners by Position so a
// multi-day span reads as one continuous bar.
//
// Due day vs scheduled day is signalled by FILL WEIGHT, not by hue:
//   - Due day (Position "end" or "single"): solid fill, white text - the
//     deadline marker.
//   - Scheduled days (Position "start" or "middle"): light tint, dark text,
//     inset ring - "scheduled, not yet due".
//
// Brightness contrast is reliable across red/green colourblindness; we
// deliberately don't pair red+green or rely on hue alone to distinguish.
func calendarChipClass(c CalendarChip) string {
	isDueDay := c.Position == "end" || c.Position == "single"
	tier := urgencyTier(c.Task.Urgency)

	var colour string
	if isDueDay {
		colour = tierSolid(tier)
	} else {
		colour = tierLight(tier)
	}

	rounded := "rounded"
	switch c.Position {
	case "start":
		rounded = "rounded-l rounded-r-none"
	case "middle":
		rounded = "rounded-none"
	case "end":
		rounded = "rounded-r rounded-l-none"
	}
	return "block w-full overflow-hidden px-1.5 py-0.5 text-left text-[11px] font-medium hover:opacity-90 " + colour + " " + rounded
}

// calendarDayTaskClass is the wrapper for the day-view list items.
func calendarDayTaskClass(_ tw.Task) string {
	return "block w-full rounded border border-zinc-200 bg-white p-3 text-left text-sm shadow-sm hover:border-zinc-300 hover:bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-700 dark:hover:bg-zinc-800"
}

// TaskSpan returns the inclusive [start, end] date range a task occupies on
// the calendar (local zone, midnight-truncated). The bool is false when the
// task has no due date and so should not be rendered at all.
//
// Rules:
//   - No due date: not on the calendar.
//   - Due only: single-day event (start == end == due).
//   - Scheduled set and parses earlier than due: scheduled..due, capped at
//     maxSpanDays so a stale scheduled doesn't paint the whole month.
//   - Recurring instances ignore Scheduled entirely - TW spawns children
//     with the parent's literal Scheduled regardless of the instance's
//     own Due, producing absurd month-long bars (see calendar audit).
//   - Otherwise wait set and parses earlier than due: wait..due (also
//     capped).
//   - Otherwise: collapse to due.
//
// Exported because handlers/calendar.go consumes it.
func TaskSpan(t tw.Task) (time.Time, time.Time, bool) {
	if t.Due == "" {
		return time.Time{}, time.Time{}, false
	}
	due, err := tw.ParseTime(t.Due)
	if err != nil || due.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	end := DateOnly(due.Local())

	start := end
	// Recurring tasks (parent or instance) get a single-day chip. TW's
	// recurrence engine doesn't reliably offset Scheduled per child, so any
	// scheduled-precedes-due reading on a recurring row is noise.
	if !t.IsRecurring() && t.Scheduled != "" {
		if s, err := tw.ParseTime(t.Scheduled); err == nil && !s.IsZero() {
			ds := DateOnly(s.Local())
			if !ds.After(end) {
				start = capSpanStart(ds, end)
			}
		}
	}
	if !t.IsRecurring() && start.Equal(end) && t.Wait != "" {
		if wt, err := tw.ParseTime(t.Wait); err == nil && !wt.IsZero() {
			dw := DateOnly(wt.Local())
			if dw.Before(end) {
				start = capSpanStart(dw, end)
			}
		}
	}
	return start, end, true
}

// maxSpanDays caps the rendered length of a multi-day chip so a far-past
// Scheduled or Wait doesn't paint the entire visible window. 14 days is the
// agenda horizon - anything longer is far enough out that "actively spanning
// to today" stops being a useful read.
const maxSpanDays = 14

// capSpanStart returns start clamped to no more than maxSpanDays before end,
// preserving the end boundary. Used so a Scheduled six months ago renders as
// a 14-day lead-in to its Due rather than as a six-month bar.
func capSpanStart(start, end time.Time) time.Time {
	cap := end.AddDate(0, 0, -maxSpanDays)
	if start.Before(cap) {
		return cap
	}
	return start
}

func calendarMobileStripCellClass(c CalendarCell) string {
	base := "flex flex-col items-center py-2"
	if !c.InPeriod {
		return base + " opacity-30"
	}
	return base
}

func calendarWeekMobileDayHeaderClass(c CalendarCell) string {
	base := "flex items-center border-b border-zinc-200 px-3 py-2 dark:border-zinc-800"
	if c.IsToday {
		return base + " bg-blue-50 dark:bg-blue-950/40"
	}
	return base + " bg-zinc-50 dark:bg-zinc-950"
}

// SortChipsByUrgency sorts chips in-place by urgency descending so the
// most-urgent chip floats to the top of a calendar cell.
func SortChipsByUrgency(chips []CalendarChip) {
	sort.SliceStable(chips, func(i, j int) bool {
		return chips[i].Task.Urgency > chips[j].Task.Urgency
	})
}

// SortTasksByUrgency sorts tasks in-place by urgency descending.
func SortTasksByUrgency(tasks []tw.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Urgency > tasks[j].Urgency
	})
}
