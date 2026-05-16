package views

import (
	"strings"
	"testing"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// journalTask builds a tw.Task with journal.time-style annotations so
// ParseSessions can produce session data without a real Taskwarrior binary.
// start and stop are formatted as YYYYMMDDTHHMMSSZ (UTC) matching TW export.
func journalTask(uuid string, starts, stops []time.Time) tw.Task {
	var ann []tw.Annotation
	for _, t := range starts {
		ann = append(ann, tw.Annotation{
			Entry:       t.UTC().Format("20060102T150405Z"),
			Description: tw.JournalStartDescription,
		})
	}
	for _, t := range stops {
		ann = append(ann, tw.Annotation{
			Entry:       t.UTC().Format("20060102T150405Z"),
			Description: tw.JournalStopDescription,
		})
	}
	return tw.Task{UUID: uuid, Description: "test task", Annotations: ann}
}

func TestSessionsTriggerLabel_NoSessions(t *testing.T) {
	task := tw.Task{UUID: "abc", Description: "no annotations"}
	got := sessionsTriggerLabel(task)
	if got != "Add time entry" {
		t.Errorf("got %q, want \"Add time entry\"", got)
	}
}

func TestSessionsTriggerLabel_WithSessions(t *testing.T) {
	base := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	stop := base.Add(90 * time.Minute)
	task := journalTask("u1", []time.Time{base}, []time.Time{stop})
	got := sessionsTriggerLabel(task)
	// Should contain "Tracked time" and the duration "1h 30m"
	if !strings.HasPrefix(got, "Tracked time") {
		t.Errorf("got %q, want prefix \"Tracked time\"", got)
	}
	if !strings.Contains(got, "1h 30m") {
		t.Errorf("got %q, want to contain \"1h 30m\"", got)
	}
}

func TestSessionsTriggerLabel_MultipleSessions(t *testing.T) {
	base := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	starts := []time.Time{base, base.Add(2 * time.Hour)}
	stops := []time.Time{base.Add(time.Hour), base.Add(3 * time.Hour)}
	task := journalTask("u2", starts, stops)
	got := sessionsTriggerLabel(task)
	if !strings.HasPrefix(got, "Tracked time") {
		t.Errorf("got %q, want prefix \"Tracked time\"", got)
	}
	// Total = 2h; should show "2h 0m" or similar
	if !strings.Contains(got, "2h") {
		t.Errorf("got %q, want to contain 2h total", got)
	}
}
