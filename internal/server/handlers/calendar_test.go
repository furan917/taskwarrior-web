package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
	"github.com/furan917/taskwarrior-web/internal/views"
)

// twTime formats t as a Taskwarrior YYYYMMDDTHHMMSSZ string in UTC. It's a
// one-line copy of the same helper in views/components_test.go; we don't
// share the body across packages because exporting a one-liner just to
// satisfy DRY costs more than the duplication. If either copy changes, keep
// them in sync.
func twTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

func TestBuildCalendarPage_Month(t *testing.T) {
	anchor := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	tasks := []tw.Task{
		{ID: 1, UUID: "u-1", Description: "demo", Due: twTime(due), Urgency: 7},
		{ID: 2, UUID: "u-2", Description: "no due"}, // skipped (no due)
	}
	p := BuildCalendarPage(views.CalendarMonth, anchor, tasks)

	if p.View != views.CalendarMonth {
		t.Errorf("view: got %q", p.View)
	}
	if !p.Anchor.Equal(views.DateOnly(anchor)) {
		t.Errorf("anchor: got %v want %v", p.Anchor, anchor)
	}
	if len(p.Cells) == 0 {
		t.Fatalf("no cells")
	}
	// Find at least one chip placed for u-1.
	found := false
	for _, c := range p.Cells {
		for _, ch := range c.Chips {
			if ch.Task.UUID == "u-1" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("u-1 chip not placed")
	}

	// PrevURL / NextURL contain expected month-step dates.
	if !strings.Contains(p.PrevURL, "2026-04-01") {
		t.Errorf("prev: %q", p.PrevURL)
	}
	if !strings.Contains(p.NextURL, "2026-06-01") {
		t.Errorf("next: %q", p.NextURL)
	}
}

func TestBuildCalendarPage_Week(t *testing.T) {
	anchor := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC) // Wednesday
	p := BuildCalendarPage(views.CalendarWeek, anchor, nil)
	if len(p.Cells) != 7 {
		t.Errorf("week: got %d cells, want 7", len(p.Cells))
	}
	// First cell is Monday.
	if p.Cells[0].Date.Weekday() != time.Monday {
		t.Errorf("first cell: %v want Monday", p.Cells[0].Date.Weekday())
	}
}

func TestBuildCalendarPage_Day(t *testing.T) {
	// Use local zone so TaskSpan (which converts to local) and the anchor
	// comparison agree on the day boundary.
	anchor := time.Date(2026, 5, 6, 0, 0, 0, 0, time.Local)
	due := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	tasks := []tw.Task{{UUID: "u", Due: twTime(due)}}
	p := BuildCalendarPage(views.CalendarDay, anchor, tasks)
	if len(p.Cells) != 1 {
		t.Errorf("day: got %d cells, want 1", len(p.Cells))
	}
	if len(p.DayTasks) != 1 {
		t.Errorf("DayTasks: got %d want 1", len(p.DayTasks))
	}
}
