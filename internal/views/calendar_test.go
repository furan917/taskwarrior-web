package views

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

// TestTaskSpan_NoDueExcluded: a task with no Due is invisible on the
// calendar. Mirrors the documented "no due date: not on the calendar" rule.
func TestTaskSpan_NoDueExcluded(t *testing.T) {
	_, _, ok := TaskSpan(tw.Task{Description: "x"})
	if ok {
		t.Errorf("expected ok=false for task with no Due")
	}
}

// TestTaskSpan_DueOnlyIsSingleDay: Due-only is a one-day chip. Defends the
// trivial-case path that the recurrence and span logic short-circuits to.
func TestTaskSpan_DueOnlyIsSingleDay(t *testing.T) {
	due := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	start, end, ok := TaskSpan(tw.Task{Due: due.UTC().Format("20060102T150405Z")})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !start.Equal(end) {
		t.Errorf("expected single-day chip, got start=%v end=%v", start, end)
	}
}

// TestTaskSpan_RecurringIgnoresScheduled: a recurring task with a
// scheduled-much-earlier-than-due (TW's recurrence engine quirk producing
// month-long bars) collapses to a single-day chip on the due date. Defends
// the user-visible bug found in audit (calendar cell flooded with the same
// recurring instance for 31 days).
func TestTaskSpan_RecurringIgnoresScheduled(t *testing.T) {
	due := time.Date(2026, 6, 7, 23, 0, 0, 0, time.UTC)
	scheduled := due.AddDate(0, -1, 0) // 31 days before due
	start, end, ok := TaskSpan(tw.Task{
		Due:       due.UTC().Format("20060102T150405Z"),
		Scheduled: scheduled.UTC().Format("20060102T150405Z"),
		Recur:     "monthly", // marks it as a recurring row (parent or instance)
		Status:    "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !start.Equal(end) {
		t.Errorf("recurring task with old Scheduled should collapse to single-day; got start=%v end=%v (span=%v)",
			start, end, end.Sub(start))
	}
}

// TestTaskSpan_NonRecurringSpanCapped: a non-recurring task with a
// far-past Scheduled has its rendered span capped at maxSpanDays. Defends
// against any other source of stale Scheduled (manual edit, import) leaking
// a calendar-wide bar.
func TestTaskSpan_NonRecurringSpanCapped(t *testing.T) {
	due := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	scheduled := due.AddDate(0, -3, 0) // 3 months before due
	start, end, ok := TaskSpan(tw.Task{
		Due:       due.UTC().Format("20060102T150405Z"),
		Scheduled: scheduled.UTC().Format("20060102T150405Z"),
		Status:    "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	span := end.Sub(start)
	maxAllowed := time.Duration(maxSpanDays) * 24 * time.Hour
	if span > maxAllowed {
		t.Errorf("span %v exceeds cap %v", span, maxAllowed)
	}
	if span <= 0 {
		t.Errorf("span should be positive when scheduled is before due; got %v", span)
	}
}

// TestTaskSpan_NonRecurringShortSpanPreserved: a Scheduled within the cap
// window is preserved exactly. Guards against the cap function over-
// clamping the common "scheduled a week before due" case.
func TestTaskSpan_NonRecurringShortSpanPreserved(t *testing.T) {
	due := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	scheduled := due.AddDate(0, 0, -3) // 3 days before due
	start, end, ok := TaskSpan(tw.Task{
		Due:       due.UTC().Format("20060102T150405Z"),
		Scheduled: scheduled.UTC().Format("20060102T150405Z"),
		Status:    "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := end.Sub(start); got != 3*24*time.Hour {
		t.Errorf("expected 3-day span preserved; got %v", got)
	}
}

// TestTaskSpan_RecurringIgnoresWait: a recurring task with stale Wait
// should also collapse to single-day, mirroring the Scheduled-ignored
// case. TW's recurrence engine doesn't reliably offset Wait per-child
// either, so the same "calendar painted for 31 days" symptom can appear
// via Wait if a user pairs `recur:` with `wait:`.
func TestTaskSpan_RecurringIgnoresWait(t *testing.T) {
	due := time.Date(2026, 6, 7, 23, 0, 0, 0, time.UTC)
	wait := due.AddDate(0, -1, 0) // 31 days before due
	start, end, ok := TaskSpan(tw.Task{
		Due:    due.UTC().Format("20060102T150405Z"),
		Wait:   wait.UTC().Format("20060102T150405Z"),
		Recur:  "monthly",
		Status: "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !start.Equal(end) {
		t.Errorf("recurring task with old Wait should collapse to single-day; got start=%v end=%v", start, end)
	}
}

// TestTaskSpan_WaitFallback_WithinCapPreserved: non-recurring task with
// only Wait (no Scheduled) - the Wait fallback at the bottom of TaskSpan
// must still respect maxSpanDays. Defends the path through capSpanStart
// for the Wait-fallback branch (the prior tests only exercised Scheduled).
func TestTaskSpan_WaitFallback_WithinCapPreserved(t *testing.T) {
	due := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	wait := due.AddDate(0, 0, -5) // 5 days before due
	start, end, ok := TaskSpan(tw.Task{
		Due:    due.UTC().Format("20060102T150405Z"),
		Wait:   wait.UTC().Format("20060102T150405Z"),
		Status: "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := end.Sub(start); got != 5*24*time.Hour {
		t.Errorf("expected 5-day Wait fallback span preserved; got %v", got)
	}
}

// TestTaskSpan_WaitFallback_FarPastCapped: non-recurring task with Wait
// 6 months in the past clamps to maxSpanDays. Closes the symmetric cap
// coverage gap on the Wait branch.
func TestTaskSpan_WaitFallback_FarPastCapped(t *testing.T) {
	due := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	wait := due.AddDate(0, -6, 0) // 6 months before due
	start, end, ok := TaskSpan(tw.Task{
		Due:    due.UTC().Format("20060102T150405Z"),
		Wait:   wait.UTC().Format("20060102T150405Z"),
		Status: "pending",
	})
	if !ok {
		t.Fatal("expected ok=true")
	}
	span := end.Sub(start)
	maxAllowed := time.Duration(maxSpanDays) * 24 * time.Hour
	if span > maxAllowed {
		t.Errorf("Wait-fallback span %v exceeds cap %v", span, maxAllowed)
	}
}
