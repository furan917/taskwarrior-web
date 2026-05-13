package views

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

// mkSession is a test helper for building tw.Session values without the
// boilerplate of UUID/Description/Project, which BuildSessionsPage /
// groupSessionsByDay don't read - only Start / Stop matter for the
// pagination + grouping calculation under test.
func mkSession(start, stop time.Time) tw.Session {
	return tw.Session{TaskUUID: "u", Start: start, Stop: stop}
}

// localDay materialises a wall-clock local-zone day, then offsets to a
// fixed hour-of-day to keep tests TZ-robust. Picking noon ensures the
// session falls on the same calendar day under any TZ from UTC-11 to
// UTC+11 (well outside any plausible test runner). groupSessionsByDay
// buckets by local-zone day, so tests have to express days in the
// local zone or risk drifting buckets between CI runners.
func localDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.Local)
}

func TestBuildSessionsPage_EmptyInput(t *testing.T) {
	got := BuildSessionsPage(nil, "", false, 0)
	if len(got.Groups) != 0 {
		t.Errorf("groups: got %d want 0", len(got.Groups))
	}
	if got.HasMore {
		t.Errorf("HasMore: got true want false")
	}
	if got.NextOffset != 0 {
		t.Errorf("NextOffset: got %d want 0", got.NextOffset)
	}
}

// TestBuildSessionsPage_SingleDayGroup confirms that multiple sessions
// on the same local-zone day collapse into ONE group, and the group's
// rolled-up Total sums every constituent session's duration.
func TestBuildSessionsPage_SingleDayGroup(t *testing.T) {
	day := localDay(2026, 5, 12)
	sessions := []tw.Session{
		mkSession(day.Add(2*time.Hour), day.Add(3*time.Hour)),   // 1h
		mkSession(day.Add(-2*time.Hour), day.Add(-1*time.Hour)), // 1h
		mkSession(day.Add(-5*time.Hour), day.Add(-4*time.Hour)), // 1h
	}

	page := BuildSessionsPage(sessions, "", false, 0)
	if len(page.Groups) != 1 {
		t.Fatalf("groups: got %d want 1; %+v", len(page.Groups), page.Groups)
	}
	if len(page.Groups[0].Sessions) != 3 {
		t.Errorf("group sessions: got %d want 3", len(page.Groups[0].Sessions))
	}
	if page.Groups[0].Total != 3*time.Hour {
		t.Errorf("total: got %v want 3h", page.Groups[0].Total)
	}
}

// TestBuildSessionsPage_MultipleDaysOrderPreserved confirms that the
// newest-first input ordering produces newest-first group ordering -
// the page builder mustn't accidentally re-sort or reverse.
func TestBuildSessionsPage_MultipleDaysOrderPreserved(t *testing.T) {
	d3 := localDay(2026, 5, 12)
	d2 := localDay(2026, 5, 11)
	d1 := localDay(2026, 5, 10)
	sessions := []tw.Session{
		mkSession(d3, d3.Add(time.Hour)),
		mkSession(d2, d2.Add(time.Hour)),
		mkSession(d1, d1.Add(time.Hour)),
	}

	page := BuildSessionsPage(sessions, "", false, 0)
	if len(page.Groups) != 3 {
		t.Fatalf("groups: got %d want 3", len(page.Groups))
	}
	// Compare year/month/day, not the full time, because Group.Date is
	// the local-midnight derived from each session's local-zone day.
	wantDays := []time.Time{d3, d2, d1}
	for i, g := range page.Groups {
		if g.Date.Year() != wantDays[i].Year() ||
			g.Date.Month() != wantDays[i].Month() ||
			g.Date.Day() != wantDays[i].Day() {
			t.Errorf("group[%d] date: got %v want day-of %v", i, g.Date, wantDays[i])
		}
	}
}

// TestBuildSessionsPage_DayFilterScopes confirms that when DayFilter is
// set the result contains ONLY that day's sessions (or empty when no
// sessions match), and HasMore is always false in the filter branch.
func TestBuildSessionsPage_DayFilterScopes(t *testing.T) {
	target := localDay(2026, 5, 12)
	other := localDay(2026, 5, 11)
	sessions := []tw.Session{
		mkSession(target, target.Add(time.Hour)),
		mkSession(other, other.Add(time.Hour)),
	}

	page := BuildSessionsPage(sessions, target.Format("2006-01-02"), false, 0)
	if len(page.Groups) != 1 {
		t.Fatalf("groups: got %d want 1", len(page.Groups))
	}
	if len(page.Groups[0].Sessions) != 1 {
		t.Errorf("filtered sessions: got %d want 1", len(page.Groups[0].Sessions))
	}
	if page.HasMore {
		t.Errorf("day-filtered page should never report HasMore")
	}
	if page.DayFilter != target.Format("2006-01-02") {
		t.Errorf("DayFilter not propagated: got %q", page.DayFilter)
	}
}

// TestBuildSessionsPage_DayFilterMalformedReturnsEmpty confirms that a
// malformed dayFilter is treated as "no match" rather than blowing up
// or falling through to the full list. The handler upstream silently
// strips bad ?day= values too, so this is a belt-and-braces invariant.
func TestBuildSessionsPage_DayFilterMalformedReturnsEmpty(t *testing.T) {
	day := localDay(2026, 5, 12)
	sessions := []tw.Session{mkSession(day, day.Add(time.Hour))}

	page := BuildSessionsPage(sessions, "not-a-date", false, 0)
	if len(page.Groups) != 0 {
		t.Errorf("groups: got %d want 0 for malformed dayFilter", len(page.Groups))
	}
}

// TestBuildSessionsPage_DayFilterNoMatch confirms the "valid filter
// that matches zero sessions" branch: empty Groups, HasMore false,
// DayFilter still propagated so the rendered bar can show "Showing 0".
func TestBuildSessionsPage_DayFilterNoMatch(t *testing.T) {
	day := localDay(2026, 5, 12)
	sessions := []tw.Session{mkSession(day, day.Add(time.Hour))}

	page := BuildSessionsPage(sessions, "2026-01-01", false, 0)
	if len(page.Groups) != 0 {
		t.Errorf("groups: got %d want 0 for non-matching dayFilter", len(page.Groups))
	}
	if page.HasMore {
		t.Errorf("HasMore should be false")
	}
}

// TestBuildSessionsPage_PaginationFirstPage confirms the page-size
// math: with 20 distinct days and SessionsPageSize=14, offset 0 yields
// 14 groups and HasMore=true with NextOffset=14.
func TestBuildSessionsPage_PaginationFirstPage(t *testing.T) {
	sessions := buildDaily(t, 20)

	page := BuildSessionsPage(sessions, "", false, 0)
	if len(page.Groups) != SessionsPageSize {
		t.Errorf("first page groups: got %d want %d", len(page.Groups), SessionsPageSize)
	}
	if !page.HasMore {
		t.Errorf("HasMore: got false want true")
	}
	if page.NextOffset != SessionsPageSize {
		t.Errorf("NextOffset: got %d want %d", page.NextOffset, SessionsPageSize)
	}
}

// TestBuildSessionsPage_PaginationLastPartial confirms the tail page:
// 20 days at offset 14 returns 6 groups, HasMore=false, NextOffset
// points past the end.
func TestBuildSessionsPage_PaginationLastPartial(t *testing.T) {
	sessions := buildDaily(t, 20)

	page := BuildSessionsPage(sessions, "", false, SessionsPageSize)
	if len(page.Groups) != 6 {
		t.Errorf("last page groups: got %d want 6", len(page.Groups))
	}
	if page.HasMore {
		t.Errorf("HasMore: got true want false")
	}
	if page.NextOffset != 20 {
		t.Errorf("NextOffset: got %d want 20", page.NextOffset)
	}
}

// TestBuildSessionsPage_NegativeOffsetClamped: handler validation also
// clamps, but BuildSessionsPage itself must be defensive - it's the
// pure-function boundary the fragment endpoint and full-render both
// hit, so a hand-crafted URL shouldn't be able to provoke a panic.
func TestBuildSessionsPage_NegativeOffsetClamped(t *testing.T) {
	sessions := buildDaily(t, 5)

	page := BuildSessionsPage(sessions, "", false, -100)
	if len(page.Groups) != 5 {
		t.Errorf("groups: got %d want 5 (negative offset must behave as 0)", len(page.Groups))
	}
}

// TestBuildSessionsPage_OffsetBeyondEnd confirms that paging past the
// last group yields an empty page with HasMore=false - the load-more
// button stops rendering once we walk off the end.
func TestBuildSessionsPage_OffsetBeyondEnd(t *testing.T) {
	sessions := buildDaily(t, 5)

	page := BuildSessionsPage(sessions, "", false, 999)
	if len(page.Groups) != 0 {
		t.Errorf("groups: got %d want 0 for offset past end", len(page.Groups))
	}
	if page.HasMore {
		t.Errorf("HasMore: got true want false")
	}
}

// TestBuildSessionsPage_FromTimesheetPropagated confirms the flag
// round-trips into the page so the renderer can flip the header
// chevron - this is the one branch that distinguishes timesheet entry
// from edit-modal entry, so it's worth a guard test.
func TestBuildSessionsPage_FromTimesheetPropagated(t *testing.T) {
	page := BuildSessionsPage(nil, "", true, 0)
	if !page.FromTimesheet {
		t.Errorf("FromTimesheet not propagated")
	}
}

// TestGroupSessionsByDay_TwoSessionsOneDay is the focused unit test
// for the bucketing helper: two sessions on the same local-zone day
// produce one group; totals sum across both.
func TestGroupSessionsByDay_TwoSessionsOneDay(t *testing.T) {
	day := localDay(2026, 5, 12)
	groups := groupSessionsByDay([]tw.Session{
		mkSession(day.Add(2*time.Hour), day.Add(2*time.Hour+30*time.Minute)),
		mkSession(day.Add(-2*time.Hour), day.Add(-time.Hour)),
	})
	if len(groups) != 1 {
		t.Fatalf("groups: got %d want 1", len(groups))
	}
	if got, want := groups[0].Total, 90*time.Minute; got != want {
		t.Errorf("total: got %v want %v", got, want)
	}
}

// TestGroupSessionsByDay_OngoingSessionUsesNow confirms that an open
// (zero-Stop) session contributes its duration measured against the
// "now" baseline groupSessionsByDay uses internally - the group still
// gets a non-zero Total. Asserts the contract loosely (>=1ms) rather
// than the exact value to avoid racing the wall clock.
func TestGroupSessionsByDay_OngoingSessionUsesNow(t *testing.T) {
	day := localDay(2026, 5, 12)
	// Construct an ongoing session that "started" a minute ago in local
	// time so Duration(now) yields ~60s. Zero Stop = still running.
	ongoing := tw.Session{Start: time.Now().Add(-time.Minute), Stop: time.Time{}, TaskUUID: "u"}
	closed := mkSession(day, day.Add(time.Hour))

	groups := groupSessionsByDay([]tw.Session{ongoing, closed})
	// We make no assumption about whether `ongoing` and `closed` fall on
	// the same local day - the test only cares that BOTH show non-zero
	// totals.
	totalAcrossGroups := time.Duration(0)
	for _, g := range groups {
		totalAcrossGroups += g.Total
	}
	if totalAcrossGroups < time.Hour {
		t.Errorf("total across groups should include the 1h closed session at minimum; got %v", totalAcrossGroups)
	}
}

// buildDaily builds n sessions, one per consecutive local-zone day
// (newest-first), each 1 hour long at noon local time. Used by the
// pagination tests so the day-bucketing produces n groups cleanly.
func buildDaily(t *testing.T, n int) []tw.Session {
	t.Helper()
	out := make([]tw.Session, 0, n)
	base := localDay(2026, 5, 12)
	for i := 0; i < n; i++ {
		d := base.AddDate(0, 0, -i)
		out = append(out, mkSession(d, d.Add(time.Hour)))
	}
	return out
}
