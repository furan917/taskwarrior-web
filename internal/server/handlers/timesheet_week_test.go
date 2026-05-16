package handlers

import (
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// weekNow is a fixed reference: Tuesday 12 May 2026 13:00 local.
// Current week = Mon 11 May – Sun 17 May 2026.
var weekNow = time.Date(2026, 5, 12, 13, 0, 0, 0, time.Local)

// taskWithSession builds a task fixture whose annotations encode one
// start/stop pair. Used to feed computeWeekSummaries without needing a
// real Taskwarrior binary.
func taskWithSession(uuid string, start, stop time.Time) tw.Task {
	return tw.Task{
		UUID:   uuid,
		Status: "pending",
		Annotations: []tw.Annotation{
			{Entry: tw.FormatTime(start.UTC()), Description: tw.JournalStartDescription},
			{Entry: tw.FormatTime(stop.UTC()), Description: tw.JournalStopDescription},
		},
	}
}

func TestComputeWeekSummaries_EmptyTasksReturnsNil(t *testing.T) {
	got := computeWeekSummaries(nil, weekSummaryWeeks, weekNow)
	if got != nil {
		t.Errorf("empty tasks: want nil, got %v", got)
	}
}

func TestComputeWeekSummaries_SkipsZeroTimeWeeks(t *testing.T) {
	// Only the previous week (Mon 4 May – Sun 10 May) has a session.
	start := time.Date(2026, 5, 5, 10, 0, 0, 0, time.Local)
	stop := start.Add(2 * time.Hour)
	tasks := []tw.Task{taskWithSession("u1", start, stop)}

	got := computeWeekSummaries(tasks, weekSummaryWeeks, weekNow)
	if len(got) != 1 {
		t.Fatalf("want 1 week (non-zero), got %d: %v", len(got), got)
	}
	if got[0].IsCurrentWeek {
		t.Errorf("session is in prev week, should not be marked IsCurrentWeek")
	}
}

func TestComputeWeekSummaries_MarksCurrentWeek(t *testing.T) {
	// Session on Tuesday 12 May = inside the current week.
	start := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	stop := start.Add(1 * time.Hour)
	tasks := []tw.Task{taskWithSession("u1", start, stop)}

	got := computeWeekSummaries(tasks, weekSummaryWeeks, weekNow)
	if len(got) == 0 {
		t.Fatal("want at least 1 week, got 0")
	}
	if !got[0].IsCurrentWeek {
		t.Errorf("week containing now should have IsCurrentWeek=true")
	}
}

func TestComputeWeekSummaries_LabelFormat(t *testing.T) {
	start := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	stop := start.Add(1 * time.Hour)
	tasks := []tw.Task{taskWithSession("u1", start, stop)}

	got := computeWeekSummaries(tasks, 1, weekNow)
	if len(got) == 0 {
		t.Fatal("want 1 week")
	}
	want := "11 May – 17 May 2026"
	if got[0].Label != want {
		t.Errorf("Label = %q, want %q", got[0].Label, want)
	}
}

func TestComputeWeekSummaries_URLFormat(t *testing.T) {
	start := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	stop := start.Add(1 * time.Hour)
	tasks := []tw.Task{taskWithSession("u1", start, stop)}

	got := computeWeekSummaries(tasks, 1, weekNow)
	if len(got) == 0 {
		t.Fatal("want 1 week")
	}
	want := "/timesheet?view=week&date=2026-05-11"
	if got[0].URL != want {
		t.Errorf("URL = %q, want %q", got[0].URL, want)
	}
}

func TestComputeWeekSummaries_SumsDurationCorrectly(t *testing.T) {
	// Two sessions in the current week totalling 3h30m.
	s1start := time.Date(2026, 5, 11, 9, 0, 0, 0, time.Local)
	s2start := time.Date(2026, 5, 12, 14, 0, 0, 0, time.Local)
	tasks := []tw.Task{
		taskWithSession("u1", s1start, s1start.Add(2*time.Hour)),
		taskWithSession("u2", s2start, s2start.Add(90*time.Minute)),
	}

	got := computeWeekSummaries(tasks, 1, weekNow)
	if len(got) == 0 {
		t.Fatal("want 1 week")
	}
	want := 3*time.Hour + 30*time.Minute
	if got[0].Total != want {
		t.Errorf("Total = %v, want %v", got[0].Total, want)
	}
}

func TestComputeWeekSummaries_NewestFirst(t *testing.T) {
	// Sessions in the current week and in the previous week.
	currStart := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	prevStart := time.Date(2026, 5, 5, 9, 0, 0, 0, time.Local)
	tasks := []tw.Task{
		taskWithSession("u1", currStart, currStart.Add(1*time.Hour)),
		taskWithSession("u2", prevStart, prevStart.Add(2*time.Hour)),
	}

	got := computeWeekSummaries(tasks, weekSummaryWeeks, weekNow)
	if len(got) < 2 {
		t.Fatalf("want 2 weeks, got %d", len(got))
	}
	if !got[0].IsCurrentWeek {
		t.Errorf("got[0] should be the current (newest) week")
	}
	if got[1].IsCurrentWeek {
		t.Errorf("got[1] should be the previous week")
	}
}

func TestComputeWeekSummaries_RespectsNWeeks(t *testing.T) {
	// n=1 with sessions in two different weeks; only the current week counts.
	currStart := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	prevStart := time.Date(2026, 5, 5, 9, 0, 0, 0, time.Local)
	tasks := []tw.Task{
		taskWithSession("u1", currStart, currStart.Add(1*time.Hour)),
		taskWithSession("u2", prevStart, prevStart.Add(1*time.Hour)),
	}

	got := computeWeekSummaries(tasks, 1, weekNow)
	if len(got) != 1 {
		t.Errorf("n=1: want 1 week, got %d", len(got))
	}
	if !got[0].IsCurrentWeek {
		t.Errorf("n=1: only result should be the current week")
	}
}
