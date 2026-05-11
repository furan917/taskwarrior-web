package handlers

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// ── computeBurndown ───────────────────────────────────────────────────────────

// TestComputeBurndown_Shape: correct bar count, oldest-first ordering, and
// non-empty labels/dates even for an all-zero dataset.
func TestComputeBurndown_Shape(t *testing.T) {
	result := computeBurndown(nil, nil, true, 5, time.Now())
	if len(result) != 5 {
		t.Fatalf("len: got %d want 5", len(result))
	}
	// result[0] = 4 days ago, result[4] = today.
	today := time.Now().Format("2006-01-02")
	fourAgo := time.Now().AddDate(0, 0, -4).Format("2006-01-02")
	if result[4].Date != today {
		t.Errorf("result[4].Date: got %q want %q (today)", result[4].Date, today)
	}
	if result[0].Date != fourAgo {
		t.Errorf("result[0].Date: got %q want %q (-4d)", result[0].Date, fourAgo)
	}
	for i, b := range result {
		if b.Label == "" {
			t.Errorf("bar %d: Label is empty", i)
		}
	}
}

// TestComputeBurndown_OpenPendingAllBars: an open task created before the
// window appears as Pending in every bar.
func TestComputeBurndown_OpenPendingAllBars(t *testing.T) {
	open := []tw.Task{
		{Status: "pending", Entry: daysAgo(10)},
	}
	result := computeBurndown(open, nil, true, 3, time.Now())
	for i, b := range result {
		if b.Pending != 1 || b.Started != 0 || b.Done != 0 {
			t.Errorf("bar %d: want Pending=1 Started=0 Done=0, got %+v", i, b)
		}
	}
}

// TestComputeBurndown_OpenPendingCreatedMidWindow: a task created 1 day ago
// must not appear in bars whose period ended before that date.
func TestComputeBurndown_OpenPendingCreatedMidWindow(t *testing.T) {
	open := []tw.Task{
		{Status: "pending", Entry: daysAgo(1)},
	}
	// 3 daily bars: result[0]=-2d, result[1]=-1d, result[2]=today
	result := computeBurndown(open, nil, true, 3, time.Now())
	if result[0].Pending != 0 {
		t.Errorf("bar 0 (-2d): task not yet created, want Pending=0, got %d", result[0].Pending)
	}
	if result[1].Pending != 1 {
		t.Errorf("bar 1 (-1d): want Pending=1, got %d", result[1].Pending)
	}
	if result[2].Pending != 1 {
		t.Errorf("bar 2 (today): want Pending=1, got %d", result[2].Pending)
	}
}

// TestComputeBurndown_OpenStarted: a started task is Pending before its start
// date and Started from that point onward.
func TestComputeBurndown_OpenStarted(t *testing.T) {
	open := []tw.Task{
		// Created 3 days ago, started 1 day ago.
		{Status: "pending", Entry: daysAgo(3), Start: daysAgo(1)},
	}
	result := computeBurndown(open, nil, true, 3, time.Now()) // bars: -2d, -1d, today
	if result[0].Pending != 1 || result[0].Started != 0 {
		t.Errorf("bar 0 (-2d, before start): want Pending=1 Started=0, got %+v", result[0])
	}
	if result[1].Started != 1 || result[1].Pending != 0 {
		t.Errorf("bar 1 (-1d, at start): want Started=1 Pending=0, got %+v", result[1])
	}
	if result[2].Started != 1 || result[2].Pending != 0 {
		t.Errorf("bar 2 (today): want Started=1 Pending=0, got %+v", result[2])
	}
}

// TestComputeBurndown_CompletedTask: a completed task is Pending before its
// completion date and Done (cumulative) from that point onward. End is the
// canonical "when did this complete" field (CompletedAt prefers it over
// Modified); seeded explicitly so a future code change that goes back to
// reading Modified fails here instead of silently rebucketing tasks.
func TestComputeBurndown_CompletedTask(t *testing.T) {
	completed := []tw.Task{
		// Created 4 days ago, completed 1 day ago.
		{Status: "completed", Entry: daysAgo(4), End: daysAgo(1), Modified: daysAgo(1)},
	}
	result := computeBurndown(nil, completed, true, 3, time.Now()) // bars: -2d, -1d, today
	if result[0].Pending != 1 || result[0].Done != 0 {
		t.Errorf("bar 0 (-2d, before completion): want Pending=1 Done=0, got %+v", result[0])
	}
	if result[1].Done != 1 || result[1].Pending != 0 {
		t.Errorf("bar 1 (-1d, at completion): want Done=1 Pending=0, got %+v", result[1])
	}
	if result[2].Done != 1 || result[2].Pending != 0 {
		t.Errorf("bar 2 (today, cumulative): want Done=1 Pending=0, got %+v", result[2])
	}
}

// TestComputeBurndown_EndBeatsModified: a task completed then later modified
// must be placed at its End date, not its Modified date.
func TestComputeBurndown_EndBeatsModified(t *testing.T) {
	// End = 2 days ago; Modified = 1 day ago (e.g. annotated after completion).
	completed := []tw.Task{
		{Status: "completed", Entry: daysAgo(5), End: daysAgo(2), Modified: daysAgo(1)},
	}
	result := computeBurndown(nil, completed, true, 3, time.Now()) // bars: -2d, -1d, today
	// Using End (2d ago): task is Done in all three bars.
	// If Modified (1d ago) were used: bar 0 would be Pending, not Done.
	for i, b := range result {
		if b.Done != 1 || b.Pending != 0 {
			t.Errorf("bar %d: End must determine completion (not Modified); got %+v", i, b)
		}
	}
}

// TestComputeBurndown_CompletedAndStarted: a completed task that was also
// started should appear as Started (not Pending) for the period between its
// start date and completion date. End seeded explicitly (same rationale as
// TestComputeBurndown_CompletedTask above) so the Started→Done transition
// is anchored on End rather than the legacy Modified path.
func TestComputeBurndown_CompletedAndStarted(t *testing.T) {
	// Created 5d ago, started 3d ago, completed 1d ago.
	completed := []tw.Task{
		{Status: "completed", Entry: daysAgo(5), Start: daysAgo(3), End: daysAgo(1), Modified: daysAgo(1)},
	}
	// Use 4 bars: -3d, -2d, -1d, today
	result := computeBurndown(nil, completed, true, 4, time.Now())
	// bar 0 (-3d): entry<=bar, start<=bar, end>bar → Started
	if result[0].Started != 1 || result[0].Pending != 0 {
		t.Errorf("bar 0 (-3d, at start): want Started=1 Pending=0, got %+v", result[0])
	}
	// bar 1 (-2d): started, not yet done → Started
	if result[1].Started != 1 || result[1].Pending != 0 {
		t.Errorf("bar 1 (-2d): want Started=1 Pending=0, got %+v", result[1])
	}
	// bar 2 (-1d): done (cumulative)
	if result[2].Done != 1 || result[2].Pending != 0 || result[2].Started != 0 {
		t.Errorf("bar 2 (-1d, at completion): want Done=1 others 0, got %+v", result[2])
	}
}

// TestComputeBurndown_Weekly: weekly=false produces the correct bar count and
// spacing. Smoke test — the algorithm is period-agnostic; full logic is
// covered by the daily tests above.
func TestComputeBurndown_Weekly(t *testing.T) {
	result := computeBurndown(nil, nil, false, 13, time.Now())
	if len(result) != 13 {
		t.Fatalf("weekly: len got %d want 13", len(result))
	}
	today := time.Now().Format("2006-01-02")
	if result[12].Date != today {
		t.Errorf("result[12].Date: got %q want %q (today)", result[12].Date, today)
	}
	twelveWeeksAgo := time.Now().AddDate(0, 0, -12*7).Format("2006-01-02")
	if result[0].Date != twelveWeeksAgo {
		t.Errorf("result[0].Date: got %q want %q (-12w)", result[0].Date, twelveWeeksAgo)
	}
}

// ── computeMonthlyHistory ─────────────────────────────────────────────────────

// TestComputeMonthlyHistory_Shape: result is newest-first; current month is
// always present even with no data; months beyond 12 are excluded.
func TestComputeMonthlyHistory_Shape(t *testing.T) {
	result := computeMonthlyHistory(nil, nil)
	if len(result) == 0 {
		t.Fatal("expected at least 1 entry (current month), got 0")
	}
	thisMonth := time.Now().Format("2006-01")
	if result[0].YearMonth != thisMonth {
		t.Errorf("result[0].YearMonth: got %q want %q", result[0].YearMonth, thisMonth)
	}
	// No entry should be older than 12 months.
	cutoff := time.Now().AddDate(-1, 0, 0).Format("2006-01")
	for _, m := range result {
		if m.YearMonth < cutoff {
			t.Errorf("entry %q is older than 12 months (cutoff %q)", m.YearMonth, cutoff)
		}
	}
}

// TestComputeMonthlyHistory_Counts: tasks added and completed this month land
// in the current-month bucket with the correct counts and Net.
func TestComputeMonthlyHistory_Counts(t *testing.T) {
	open := []tw.Task{
		{Status: "pending", Entry: daysAgo(1)},
		{Status: "pending", Entry: daysAgo(3)},
	}
	completed := []tw.Task{
		{Status: "completed", Entry: daysAgo(5), Modified: daysAgo(0)},
	}
	result := computeMonthlyHistory(open, completed)
	if len(result) == 0 {
		t.Fatal("got empty result")
	}
	m := result[0] // current month
	// Added: 2 open + 1 completed (all created within the last month).
	if m.Added != 3 {
		t.Errorf("Added: got %d want 3", m.Added)
	}
	if m.Completed != 1 {
		t.Errorf("Completed: got %d want 1", m.Completed)
	}
	if m.Net() != 2 {
		t.Errorf("Net: got %d want 2 (Added-Completed)", m.Net())
	}
}

// TestComputeMonthlyHistory_OldTasksExcluded: tasks created or completed more
// than 12 months ago must not appear in any bucket.
func TestComputeMonthlyHistory_OldTasksExcluded(t *testing.T) {
	ancient := time.Now().UTC().AddDate(-2, 0, 0).Format("20060102T150405Z")
	completed := []tw.Task{
		{Status: "completed", Entry: ancient, Modified: ancient},
	}
	result := computeMonthlyHistory(nil, completed)
	for _, m := range result {
		if m.Added != 0 || m.Completed != 0 {
			t.Errorf("month %q: expected zeros for ancient task, got Added=%d Completed=%d",
				m.YearMonth, m.Added, m.Completed)
		}
	}
}

// TestComputeMonthlyHistory_EndBeatsModified: a task completed then annotated
// must land in the month of End, not Modified.
func TestComputeMonthlyHistory_EndBeatsModified(t *testing.T) {
	// End = last month; Modified = this month (e.g. annotation added later).
	lastMonth := time.Now().UTC().AddDate(0, -1, 0).Format("20060102T150405Z")
	thisMonthTS := time.Now().UTC().Format("20060102T150405Z")
	completed := []tw.Task{
		{Status: "completed", Entry: lastMonth, End: lastMonth, Modified: thisMonthTS},
	}
	result := computeMonthlyHistory(nil, completed)

	thisMonthKey := time.Now().Format("2006-01")
	lastMonthKey := time.Now().AddDate(0, -1, 0).Format("2006-01")

	for _, m := range result {
		switch m.YearMonth {
		case thisMonthKey:
			if m.Completed != 0 {
				t.Errorf("this month: Completed should be 0 (End is last month), got %d", m.Completed)
			}
		case lastMonthKey:
			if m.Completed != 1 {
				t.Errorf("last month: Completed should be 1, got %d", m.Completed)
			}
		}
	}
}

// TestComputeMonthlyHistory_NetSign: negative net (more done than added) is
// good — it means the backlog is shrinking.
func TestComputeMonthlyHistory_NetSign(t *testing.T) {
	m := views.MonthCount{Added: 2, Completed: 5}
	if m.Net() != -3 {
		t.Errorf("Net: got %d want -3", m.Net())
	}
}
