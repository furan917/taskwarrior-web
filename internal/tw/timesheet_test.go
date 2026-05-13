package tw

import (
	"testing"
	"time"
)

// ts converts a wall-clock time literal into the YYYYMMDDTHHMMSSZ string TW
// stores. Tests stay readable instead of stringifying inline.
func ts(t time.Time) string { return FormatTime(t.UTC()) }

// utc constructs a UTC time. Local zone-independent for stable assertions
// across CI runners in arbitrary timezones.
func utc(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, time.UTC)
}

// TestParseSessions_PairedStartedStopped: the canonical happy path. Two
// pairs of journal annotations produce two Session entries with the right
// timestamps. ParseSessions is the read-side aggregator that the timesheet
// view and the (forthcoming) interval editor both depend on.
func TestParseSessions_PairedStartedStopped(t *testing.T) {
	start1, stop1 := utc(2026, 5, 8, 16, 0), utc(2026, 5, 8, 17, 0)
	start2, stop2 := utc(2026, 5, 11, 9, 0), utc(2026, 5, 11, 9, 30)
	task := Task{
		UUID:        "11111111-2222-3333-4444-555555555555",
		Description: "x",
		Status:      "pending",
		Annotations: []Annotation{
			{Entry: ts(start1), Description: "Started task"},
			{Entry: ts(stop1), Description: "Stopped task"},
			{Entry: ts(start2), Description: "Started task"},
			{Entry: ts(stop2), Description: "Stopped task"},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 2 {
		t.Fatalf("len: got %d want 2; got=%+v", len(got), got)
	}
	if !got[0].Start.Equal(start1) || !got[0].Stop.Equal(stop1) {
		t.Errorf("session[0]: got %v..%v want %v..%v", got[0].Start, got[0].Stop, start1, stop1)
	}
	if !got[1].Start.Equal(start2) || !got[1].Stop.Equal(stop2) {
		t.Errorf("session[1]: got %v..%v want %v..%v", got[1].Start, got[1].Stop, start2, stop2)
	}
}

// TestParseSessions_OpenSessionOnActiveTask: a Started without a matching
// Stopped on an ACTIVE task (task.Start != "") yields a Session with
// zero-valued Stop, signalling "still running". The timesheet renders this
// with an amber "running" pill; the editor disables its End input.
func TestParseSessions_OpenSessionOnActiveTask(t *testing.T) {
	startedAt := utc(2026, 5, 12, 8, 0)
	task := Task{
		UUID:        "u",
		Description: "x",
		Status:      "pending",
		Start:       ts(startedAt), // task IS active
		Annotations: []Annotation{
			{Entry: ts(startedAt), Description: "Started task"},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 1 {
		t.Fatalf("len: got %d want 1; got=%+v", len(got), got)
	}
	if !got[0].Start.Equal(startedAt) {
		t.Errorf("start: got %v want %v", got[0].Start, startedAt)
	}
	if !got[0].Stop.IsZero() {
		t.Errorf("Stop should be zero for open session; got %v", got[0].Stop)
	}
}

// TestParseSessions_OrphanStartOnInactiveTaskDropped: a Started with no
// matching Stop AND task is NOT active is data corruption; ParseSessions
// drops it rather than fabricate a zero-length session. Defensive.
func TestParseSessions_OrphanStartOnInactiveTaskDropped(t *testing.T) {
	task := Task{
		UUID:        "u",
		Description: "x",
		Status:      "pending", // inactive: Start field empty
		Annotations: []Annotation{
			{Entry: ts(utc(2026, 5, 8, 9, 0)), Description: "Started task"},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 0 {
		t.Errorf("expected 0 sessions for orphan-start on inactive task; got %+v", got)
	}
}

// TestParseSessions_OrphanStopsAdvanced: stray Stopped annotations that
// don't pair with any Started (e.g. left over from a manual edit) must NOT
// produce phantom sessions and must NOT consume a real Started that
// follows them in time. Defends the `si` advance loop in ParseSessions.
func TestParseSessions_OrphanStopsAdvanced(t *testing.T) {
	staleStop := utc(2026, 5, 8, 5, 0)
	realStart := utc(2026, 5, 8, 9, 0)
	realStop := utc(2026, 5, 8, 10, 0)
	task := Task{
		UUID:        "u",
		Description: "x",
		Status:      "pending",
		Annotations: []Annotation{
			{Entry: ts(staleStop), Description: "Stopped task"}, // orphan
			{Entry: ts(realStart), Description: "Started task"},
			{Entry: ts(realStop), Description: "Stopped task"},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 1 {
		t.Fatalf("len: got %d want 1", len(got))
	}
	if !got[0].Start.Equal(realStart) || !got[0].Stop.Equal(realStop) {
		t.Errorf("got %v..%v want %v..%v", got[0].Start, got[0].Stop, realStart, realStop)
	}
}

// TestParseSessions_CustomAnnotationsIgnored: user-authored annotations
// that don't match the journal-time pattern must NOT influence session
// parsing. Both ParseSessions (read) and ReplaceIntervals (write) rely on
// this invariant - the writer preserves non-journal annotations verbatim
// and the reader skips them.
func TestParseSessions_CustomAnnotationsIgnored(t *testing.T) {
	start := utc(2026, 5, 8, 9, 0)
	stop := utc(2026, 5, 8, 10, 0)
	task := Task{
		UUID:        "u",
		Description: "x",
		Status:      "pending",
		Annotations: []Annotation{
			{Entry: ts(utc(2026, 5, 8, 8, 0)), Description: "called supplier"},
			{Entry: ts(start), Description: "Started task"},
			{Entry: ts(utc(2026, 5, 8, 9, 30)), Description: "stuck on the auth flow"},
			{Entry: ts(stop), Description: "Stopped task"},
			{Entry: ts(utc(2026, 5, 8, 10, 30)), Description: "Started the conversation - keep me"},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 1 {
		t.Fatalf("len: got %d want 1 (custom annotations must not parse as sessions); got=%+v", len(got), got)
	}
	if !got[0].Start.Equal(start) || !got[0].Stop.Equal(stop) {
		t.Errorf("session bounds: got %v..%v want %v..%v", got[0].Start, got[0].Stop, start, stop)
	}
}

// TestParseSessions_TW2xLegacyFormat: ParseSessions accepts the older TW
// 2.x form where the timestamp was embedded in the annotation description
// ("Started 20260508T090000Z"). Defends back-compat: imports of historical
// data from TW 2.x must remain readable.
func TestParseSessions_TW2xLegacyFormat(t *testing.T) {
	start := utc(2026, 5, 8, 9, 0)
	stop := utc(2026, 5, 8, 10, 0)
	task := Task{
		UUID:        "u",
		Description: "x",
		Status:      "pending",
		Annotations: []Annotation{
			// Note: TW 2.x put `now` in `entry` and the event-time in the description.
			{Entry: ts(utc(2026, 5, 8, 9, 0 /*sec=*/)), Description: "Started " + FormatTime(start)},
			{Entry: ts(utc(2026, 5, 8, 10, 0)), Description: "Stopped " + FormatTime(stop)},
		},
	}
	got := ParseSessions(task, time.Now())
	if len(got) != 1 {
		t.Fatalf("len: got %d want 1", len(got))
	}
	if !got[0].Start.Equal(start) || !got[0].Stop.Equal(stop) {
		t.Errorf("TW2.x session bounds: got %v..%v want %v..%v", got[0].Start, got[0].Stop, start, stop)
	}
}

// TestIsJournalAnnotation: the writer relies on this predicate to decide
// which annotations to strip and which to preserve. Cover the four shapes
// (TW3.x exact, TW3.x with text, TW2.x with valid ts, TW2.x with non-ts
// text) plus a user-authored false-positive guard.
func TestIsJournalAnnotation(t *testing.T) {
	cases := []struct {
		desc string
		want bool
	}{
		{"Started task", true},              // TW3.x exact
		{"Stopped task", true},              // TW3.x exact
		{"Started task with comment", true}, // TW3.x + suffix
		{"Stopped task with comment", true}, // TW3.x + suffix
		{"Started 20260508T090000Z", true},  // TW2.x: valid ts
		{"Stopped 20260508T100000Z", true},  // TW2.x: valid ts
		{"Started the conversation", false}, // user annotation, NOT a ts
		{"Stopped using slack", false},      // user annotation, NOT a ts
		{"called supplier", false},          // unrelated
		{"", false},                         // empty
		{"  Started task  ", true},          // surrounding whitespace tolerated
	}
	for _, c := range cases {
		if got := IsJournalAnnotation(c.desc); got != c.want {
			t.Errorf("IsJournalAnnotation(%q) = %v, want %v", c.desc, got, c.want)
		}
	}
}
