package handlers

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// daysAgo returns a Taskwarrior-format timestamp (UTC) N days before now.
// Used to seed completion-timestamp fields on fixtures so in-window cutoff
// comparisons in computeStats / computeBurndown / computeMonthlyHistory
// land predictably. Production code reads `End` first (via CompletedAt())
// and falls back to `Modified`; these tests seed Modified for brevity
// because empty End => fallback path, and the burndown suite has dedicated
// EndBeatsModified tests for the prefer-End branch.
func daysAgo(n int) string {
	return time.Now().UTC().AddDate(0, 0, -n).Format("20060102T150405Z")
}

// TestComputeStats_OpenTaskBuckets confirms the per-status counters on
// the open-task pool: pending/waiting/active/recurring/blocked/overdue
// each tally independently from the same fixture.
func TestComputeStats_OpenTaskBuckets(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("20060102T150405Z")
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("20060102T150405Z")
	open := []tw.Task{
		// Plain pending.
		{Status: "pending"},
		// Waiting (not pending).
		{Status: "waiting"},
		// Pending + overdue (Due in the past).
		{Status: "pending", Due: yesterday},
		// Pending + active (has start).
		{Status: "pending", Start: yesterday},
		// Pending + recurring.
		{Status: "pending", Recur: "weekly"},
		// Pending + blocked (has deps).
		{Status: "pending", Depends: []string{"11111111-2222-3333-4444-555555555555"}},
		// Future-due pending: counted only as pending, NOT overdue.
		{Status: "pending", Due: tomorrow},
		// Pending + overdue + active: must increment ALL THREE buckets.
		// Guards against a regression that mutually-excludes overdue and
		// active (e.g. an else-if where independent ifs are needed).
		{Status: "pending", Due: yesterday, Start: yesterday},
	}
	got := computeStats(open, nil, 14)

	if got.Pending != 7 {
		t.Errorf("Pending: got %d want 7", got.Pending)
	}
	if got.Waiting != 1 {
		t.Errorf("Waiting: got %d want 1", got.Waiting)
	}
	if got.Overdue != 2 {
		t.Errorf("Overdue: got %d want 2", got.Overdue)
	}
	if got.Active != 2 {
		t.Errorf("Active: got %d want 2", got.Active)
	}
	// Recurring count is no longer computed by computeStats - it comes
	// from a dedicated status:recurring query in Stats(). The fixtures
	// here include a pending-with-recur task to guard the OTHER counters
	// don't false-positive on it; computeStats must NOT increment
	// Recurring from the open pool.
	if got.Recurring != 0 {
		t.Errorf("Recurring should be 0 (set by Stats() handler from a separate query, not derived from open pool); got %d", got.Recurring)
	}
	if got.Blocked != 1 {
		t.Errorf("Blocked: got %d want 1", got.Blocked)
	}
	if got.WindowDays != 14 {
		t.Errorf("WindowDays: got %d want 14", got.WindowDays)
	}
}

// TestComputeStats_CompletedWindow confirms that completed tasks land in
// the right bucket (7d, in-window) and that completions outside the window
// are dropped, not silently miscounted.
func TestComputeStats_CompletedWindow(t *testing.T) {
	completed := []tw.Task{
		{Status: "completed", Modified: daysAgo(0)},  // today
		{Status: "completed", Modified: daysAgo(2)},  // within 7
		{Status: "completed", Modified: daysAgo(6)},  // within 7
		{Status: "completed", Modified: daysAgo(8)},  // in window, NOT 7d
		{Status: "completed", Modified: daysAgo(13)}, // in window edge
		{Status: "completed", Modified: daysAgo(20)}, // outside 14d window - drop
		{Status: "completed", Modified: ""},          // unparseable - drop
	}
	got := computeStats(nil, completed, 14)

	if got.Completed7d != 3 {
		t.Errorf("Completed7d: got %d want 3", got.Completed7d)
	}
	if got.CompletedInWindow != 5 {
		t.Errorf("CompletedInWindow: got %d want 5", got.CompletedInWindow)
	}
}

// TestComputeStats_HistoryShape: History is newest-first, has exactly
// `days` entries, and each entry's date string round-trips the day index.
func TestComputeStats_HistoryShape(t *testing.T) {
	got := computeStats(nil, nil, 7)

	if len(got.History) != 7 {
		t.Fatalf("History len: got %d want 7", len(got.History))
	}
	// Index 0 is today.
	today := time.Now().Format("2006-01-02")
	if got.History[0].Date != today {
		t.Errorf("History[0].Date: got %q want %q (today)", got.History[0].Date, today)
	}
	// Index 6 is 6 days ago.
	sixAgo := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	if got.History[6].Date != sixAgo {
		t.Errorf("History[6].Date: got %q want %q (-6 days)", got.History[6].Date, sixAgo)
	}
	// All days have a Label set.
	for i, d := range got.History {
		if d.Label == "" {
			t.Errorf("History[%d].Label is empty", i)
		}
	}
}

// TestComputeStats_HistoryCounts: a completion lands in its day's bucket
// and a quiet day stays at zero (not skipped). Defends the "always emit
// every day in the window" contract that the chart relies on.
func TestComputeStats_HistoryCounts(t *testing.T) {
	completed := []tw.Task{
		{Status: "completed", Modified: daysAgo(0)}, // today
		{Status: "completed", Modified: daysAgo(0)}, // today (2 total)
		{Status: "completed", Modified: daysAgo(2)}, // 2 days ago
	}
	got := computeStats(nil, completed, 5)

	if got.History[0].Count != 2 {
		t.Errorf("today bucket: got %d want 2", got.History[0].Count)
	}
	if got.History[1].Count != 0 {
		t.Errorf("yesterday bucket: got %d want 0", got.History[1].Count)
	}
	if got.History[2].Count != 1 {
		t.Errorf("2-days-ago bucket: got %d want 1", got.History[2].Count)
	}
	for i := 3; i < len(got.History); i++ {
		if got.History[i].Count != 0 {
			t.Errorf("History[%d].Count: got %d want 0", i, got.History[i].Count)
		}
	}
}

// TestComputeStats_EmptyInputs: zero-length slices give zero counts and a
// fully-zero History of the requested length. No crashes, no nils where a
// slice is expected.
func TestComputeStats_EmptyInputs(t *testing.T) {
	got := computeStats(nil, nil, 14)
	if got.Pending+got.Waiting+got.Overdue+got.Active+got.Recurring+got.Blocked+got.Completed7d+got.CompletedInWindow != 0 {
		t.Errorf("expected zero counts, got %+v", got)
	}
	if len(got.History) != 14 {
		t.Errorf("History len: got %d want 14", len(got.History))
	}
}
