package handlers

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// fixedTime keeps the tests deterministic regardless of when CI runs.
// Tuesday 12 May 2026 13:00 local; pick a time inside a workday so
// today-detection and same-day grouping both exercise the happy path.
func fixedTime() time.Time {
	return time.Date(2026, 5, 12, 13, 0, 0, 0, time.Local)
}

func session(start time.Time, duration time.Duration, uuid, desc string) tw.Session {
	stop := start.Add(duration)
	return tw.Session{TaskUUID: uuid, Description: desc, Start: start, Stop: stop}
}

func TestTimesheetWindow_Day(t *testing.T) {
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	from, to := timesheetWindow(views.TimesheetViewDay, anchor)
	if !from.Equal(anchor) {
		t.Errorf("day from = %v, want %v", from, anchor)
	}
	if !to.Equal(anchor.AddDate(0, 0, 1)) {
		t.Errorf("day to = %v, want %v", to, anchor.AddDate(0, 0, 1))
	}
}

func TestTimesheetWindow_WeekStartsMonday(t *testing.T) {
	// Friday 8 May 2026 - the window should start on Mon 4 May.
	anchor := time.Date(2026, 5, 8, 0, 0, 0, 0, time.Local)
	from, to := timesheetWindow(views.TimesheetViewWeek, anchor)
	wantFrom := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	if !from.Equal(wantFrom) {
		t.Errorf("week from = %v, want %v (Mon)", from, wantFrom)
	}
	if !to.Equal(wantFrom.AddDate(0, 0, 7)) {
		t.Errorf("week to = %v, want %v", to, wantFrom.AddDate(0, 0, 7))
	}
}

func TestBuildTimesheetPage_WeekProducesSevenDays(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, nil, now)
	if len(data.Days) != 7 {
		t.Fatalf("week view should produce 7 day slots, got %d", len(data.Days))
	}
	// Day 0 (Mon 11 May), day 1 (Tue 12), ... day 6 (Sun 17)
	if !data.Days[0].Date.Equal(time.Date(2026, 5, 11, 0, 0, 0, 0, time.Local)) {
		t.Errorf("day[0] should be Mon 11 May, got %v", data.Days[0].Date)
	}
	if !data.Days[1].IsToday {
		t.Errorf("day[1] (Tue 12) should be marked IsToday=true")
	}
	if data.Days[0].IsToday {
		t.Errorf("day[0] (Mon 11) should NOT be marked IsToday")
	}
}

func TestBuildTimesheetPage_DayProducesOneSlot(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 10, 0, 0, 0, 0, time.Local)
	data := BuildTimesheetPage(views.TimesheetViewDay, anchor, nil, now)
	if len(data.Days) != 1 {
		t.Fatalf("day view should produce 1 day slot, got %d", len(data.Days))
	}
	if data.Days[0].IsToday {
		t.Errorf("Sun 10 May is not today (Tue 12), should not be marked IsToday")
	}
}

func TestBuildTimesheetPage_SessionsBucketByLocalDay(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	sessions := []tw.Session{
		session(time.Date(2026, 5, 11, 10, 0, 0, 0, time.Local), 45*time.Minute, "u1", "Task A"),
		session(time.Date(2026, 5, 11, 14, 0, 0, 0, time.Local), 15*time.Minute, "u2", "Task B"),
		session(time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local), 30*time.Minute, "u3", "Task C"),
	}
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, sessions, now)
	if got := len(data.Days[0].Sessions); got != 2 {
		t.Errorf("Mon 11 should have 2 sessions, got %d", got)
	}
	if got := data.Days[0].Total; got != 60*time.Minute {
		t.Errorf("Mon 11 total = %v, want 1h", got)
	}
	if got := len(data.Days[1].Sessions); got != 1 {
		t.Errorf("Tue 12 should have 1 session, got %d", got)
	}
	if got := data.TotalDuration; got != 90*time.Minute {
		t.Errorf("week total = %v, want 1h30m", got)
	}
}

func TestBuildTimesheetPage_DropsSessionsOutsideWindow(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	sessions := []tw.Session{
		// Inside the week (Mon 11 May - Sun 17 May).
		session(time.Date(2026, 5, 13, 11, 0, 0, 0, time.Local), 30*time.Minute, "u1", "in"),
		// Outside - prior week, should be discarded.
		session(time.Date(2026, 5, 7, 11, 0, 0, 0, time.Local), 30*time.Minute, "u2", "out"),
	}
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, sessions, now)
	total := 0
	for _, d := range data.Days {
		total += len(d.Sessions)
	}
	if total != 1 {
		t.Errorf("only the in-window session should be retained, got %d", total)
	}
}

func TestBuildTimesheetPage_SessionsSortedWithinDay(t *testing.T) {
	now := fixedTime()
	anchor := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)
	// Three sessions on Wed 13 inserted out of chronological order.
	sessions := []tw.Session{
		session(time.Date(2026, 5, 13, 14, 0, 0, 0, time.Local), 30*time.Minute, "u1", "afternoon"),
		session(time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local), 30*time.Minute, "u2", "morning"),
		session(time.Date(2026, 5, 13, 11, 0, 0, 0, time.Local), 30*time.Minute, "u3", "midday"),
	}
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, sessions, now)
	wed := data.Days[2]
	if got := wed.Sessions[0].Description; got != "morning" {
		t.Errorf("sessions[0] = %q, want morning", got)
	}
	if got := wed.Sessions[2].Description; got != "afternoon" {
		t.Errorf("sessions[2] = %q, want afternoon", got)
	}
}

func TestBuildTimesheetPage_TodayURLUsesNowNotAnchor(t *testing.T) {
	now := fixedTime() // Tue 12 May
	anchor := time.Date(2026, 5, 10, 0, 0, 0, 0, time.Local)
	data := BuildTimesheetPage(views.TimesheetViewWeek, anchor, nil, now)
	want := "/timesheet?view=week&date=2026-05-12"
	if data.TodayURL != want {
		t.Errorf("TodayURL = %q, want %q", data.TodayURL, want)
	}
}
