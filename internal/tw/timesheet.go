package tw

import (
	"sort"
	"strings"
	"time"
)

// Canonical Taskwarrior 3.x journal-time annotation descriptions. The READ
// path in ParseSessions also accepts TW 2.x "Started <timestamp>" / "Stopped
// <timestamp>" forms for back-compat, but the WRITE path (ReplaceIntervals)
// only emits the canonical 3.x form. Hardcoded as constants (NOT user input)
// because the writer's annotation-description field MUST be a fixed Go value
// so a future bug can't smuggle attacker-controlled text into a write.
const (
	JournalStartDescription = "Started task"
	JournalStopDescription  = "Stopped task"
)

// IsJournalAnnotation reports whether an annotation description belongs
// to the time-tracking journal (any era). ReplaceIntervals uses this to
// STRIP existing journal annotations before re-emitting from the FE's
// intervals; the Notes panel in the edit modal uses it to HIDE journal
// annotations from the user-facing notes list (otherwise dozens of
// "Started task"/"Stopped task" rows from time tracking pollute what
// should only carry the user's own scribbles).
//
// Exported because both the tw and views layers need the same predicate
// and forking the regex would invite drift.
func IsJournalAnnotation(desc string) bool {
	desc = strings.TrimSpace(desc)
	switch {
	case desc == JournalStartDescription || strings.HasPrefix(desc, JournalStartDescription+" "):
		return true
	case desc == JournalStopDescription || strings.HasPrefix(desc, JournalStopDescription+" "):
		return true
	case strings.HasPrefix(desc, "Started "):
		// TW 2.x: "Started <timestamp>". Match conservatively: only when
		// the suffix parses as a timestamp, so a user annotation like
		// "Started the conversation" passes through unchanged.
		_, ok := parseJournalTimestamp(desc, "Started ")
		return ok
	case strings.HasPrefix(desc, "Stopped "):
		_, ok := parseJournalTimestamp(desc, "Stopped ")
		return ok
	}
	return false
}

// Session is one continuous work block on a task: a start time and either a
// stop time (non-zero) or an ongoing session (Stop.IsZero() == true when the
// task is still active).
type Session struct {
	TaskUUID    string
	Description string
	Project     string
	Start       time.Time
	Stop        time.Time // zero == still running
}

// Duration returns the length of the session. Ongoing sessions use now as the
// implicit stop.
func (s Session) Duration(now time.Time) time.Duration {
	end := s.Stop
	if end.IsZero() {
		end = now
	}
	if end.Before(s.Start) {
		return 0
	}
	return end.Sub(s.Start)
}

// ParseSessions extracts time-tracking sessions from a task's journal
// annotations (written by taskwarrior when rc.journal.time=yes).
//
// Taskwarrior 3.x stores the event time in the annotation's `entry` field and
// uses plain descriptions "Started task" / "Stopped task". Taskwarrior 2.x
// embedded the timestamp in the description ("Started 20240101T120000Z").
// We support both: entry-based first, falling back to description-embedded.
//
// Each "Started" annotation opens a session; the next "Stopped" closes it.
// An unclosed session on a currently active task is left with a zero Stop so
// callers can treat it as ongoing.
func ParseSessions(t Task, now time.Time) []Session {
	var starts, stops []time.Time

	for _, ann := range t.Annotations {
		desc := strings.TrimSpace(ann.Description)
		switch {
		case desc == "Started task" || strings.HasPrefix(desc, "Started task "):
			if ts, err := ParseTime(ann.Entry); err == nil && !ts.IsZero() {
				starts = append(starts, ts)
			}
		case desc == "Stopped task" || strings.HasPrefix(desc, "Stopped task "):
			if ts, err := ParseTime(ann.Entry); err == nil && !ts.IsZero() {
				stops = append(stops, ts)
			}
		default:
			// Taskwarrior 2.x: timestamp embedded in description.
			if ts, ok := parseJournalTimestamp(desc, "Started "); ok {
				starts = append(starts, ts)
			} else if ts, ok := parseJournalTimestamp(desc, "Stopped "); ok {
				stops = append(stops, ts)
			}
		}
	}

	if len(starts) == 0 {
		return nil
	}

	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	sort.Slice(stops, func(i, j int) bool { return stops[i].Before(stops[j]) })

	var sessions []Session
	si := 0
	for _, start := range starts {
		// Advance past stops STRICTLY before this start - orphaned
		// stops. A stop AT the same instant as a start is treated as
		// a valid (zero-duration) pairing rather than an orphan: the
		// editor's minute-granular datetime-local input can produce
		// genuine same-instant Started/Stopped pairs for sub-minute
		// real intervals, and round-tripping must be lossless.
		for si < len(stops) && stops[si].Before(start) {
			si++
		}

		var stop time.Time
		if si < len(stops) {
			stop = stops[si]
			si++
		} else if t.IsActive() {
			// No paired stop and task is running - ongoing session; zero stop.
			stop = time.Time{}
		} else {
			// Orphaned start with no stop and task not active - skip.
			continue
		}

		sessions = append(sessions, Session{
			TaskUUID:    t.UUID,
			Description: t.Description,
			Project:     t.Project,
			Start:       start,
			Stop:        stop,
		})
	}
	return sessions
}

// SessionsInRange returns all sessions from tasks that overlap with the
// half-open interval [from, to). A session overlaps when it starts before
// `to` and ends (or is still running with now) after `from`.
func SessionsInRange(tasks []Task, from, to, now time.Time) []Session {
	var out []Session
	for _, t := range tasks {
		for _, s := range ParseSessions(t, now) {
			end := s.Stop
			if end.IsZero() {
				end = now
			}
			if s.Start.Before(to) && end.After(from) {
				out = append(out, s)
			}
		}
	}
	return out
}

func parseJournalTimestamp(desc, prefix string) (time.Time, bool) {
	if !strings.HasPrefix(desc, prefix) {
		return time.Time{}, false
	}
	ts, err := ParseTime(strings.TrimSpace(strings.TrimPrefix(desc, prefix)))
	if err != nil || ts.IsZero() {
		return time.Time{}, false
	}
	return ts, true
}
