package views

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/furan917/taskwarrior-web-portal/internal/tw"
)

// ParseSort extracts the SortSpec from a request's URL values. Unknown keys or
// missing params fall back to the default spec (urgency desc). The optional
// `:asc`/`:desc` suffix overrides the key's natural default direction.
//
// Lives in views (alongside SortSpec / SortKeys / defaultDirAscMap) so handlers
// don't have to keep a parallel allowlist; this is the single source of truth.
func ParseSort(q url.Values) SortSpec {
	raw := strings.TrimSpace(q.Get("sort"))
	if raw == "" {
		return DefaultSort
	}
	parts := strings.SplitN(raw, ":", 2)
	key := parts[0]
	if _, ok := defaultDirAscMap[key]; !ok {
		return DefaultSort
	}
	asc := defaultDirAscMap[key]
	if len(parts) == 2 {
		switch parts[1] {
		case "asc":
			asc = true
		case "desc":
			asc = false
		}
	}
	return SortSpec{Key: key, Asc: asc}
}

// SortSpec is the parsed shape of the URL `?sort=<key>[:<dir>]` parameter. Key
// is one of: urgency, due, project, description, entry. Asc=true means
// ascending. Defaults: urgency desc.
type SortSpec struct {
	Key string
	Asc bool
}

// SortKeys lists every sort key the list views accept, in the display order
// used by the column header.
var SortKeys = []string{"urgency", "due", "project", "description", "entry"}

// SortLabels maps sort keys to their human-readable column label.
var SortLabels = map[string]string{
	"urgency":     "Urgency",
	"due":         "Due",
	"project":     "Project",
	"description": "Description",
	"entry":       "Created",
}

// DefaultSort is the default ordering used when no ?sort= is supplied: urgency
// desc.
var DefaultSort = SortSpec{Key: "urgency", Asc: false}

// FormatSort encodes a SortSpec as the URL value form `<key>:asc` / `<key>:desc`.
func FormatSort(s SortSpec) string {
	dir := "desc"
	if s.Asc {
		dir = "asc"
	}
	return s.Key + ":" + dir
}

func taskKey(t tw.Task) string {
	if t.UUID != "" {
		return t.UUID
	}
	return fmt.Sprintf("%d", t.ID)
}

func rowDoneURL(t tw.Task) string { return "/tasks/" + taskKey(t) + "/done" }
func rowEditURL(t tw.Task) string { return "/forms/edit/" + taskKey(t) }

// rowEditURLByUUID returns the edit-form URL for a raw UUID string. Used by
// the row-detail panel's "Blocked by" deep links, which only have the
// dependent task's UUID, not a full tw.Task value.
func rowEditURLByUUID(uuid string) string { return "/forms/edit/" + uuid }

// taskURL returns the canonical /tasks/<key> URL. Used by edit/save, delete,
// annotate, denotate; centralised so we have one site to grep when the route
// shape changes.
func taskURL(t tw.Task) string { return "/tasks/" + taskKey(t) }

// taskAnnotateURL and taskDenotateURL build the annotation sub-routes off
// taskURL.
func taskAnnotateURL(t tw.Task) string { return taskURL(t) + "/annotate" }
func taskDenotateURL(t tw.Task) string { return taskURL(t) + "/denotate" }

// taskStartURL / taskStopURL toggle the active state. taskDuplicateURL
// clones the task via `task <id> duplicate`. All three are POST endpoints
// under taskURL.
func taskStartURL(t tw.Task) string     { return taskURL(t) + "/start" }
func taskStopURL(t tw.Task) string      { return taskURL(t) + "/stop" }
func taskDuplicateURL(t tw.Task) string { return taskURL(t) + "/duplicate" }

// contextSetURL is the POST endpoint the context dropdown / clear-x targets.
// One named constant beats sprinkling the literal "/context" through five
// templ files.
const contextSetURL = "/context"

// denotateVals JSON-encodes the annotation description for HTMX hx-vals on
// the x button. Uses encoding/json so control characters (\n, \t, \r, 0x00-
// 0x1F) and unicode line terminators (U+2028 / U+2029) get encoded as \uXXXX
// escapes that HTMX can parse - the previous hand-rolled `\` and `"` only
// escaper produced invalid JSON for any annotation containing a literal
// newline (possible when the note was added via the CLI), silently breaking
// the per-annotation × button.
func denotateVals(description string) string {
	b, err := json.Marshal(map[string]string{"text": description})
	if err != nil {
		// json.Marshal of a string-keyed map[string]string can't fail in
		// practice; fall back to an empty body so the request still
		// fires and the server's denotate handler returns a 400 the
		// user can understand rather than the button doing nothing.
		return `{"text": ""}`
	}
	return string(b)
}

func partialURL(report, project string) string {
	if project != "" {
		return "/partials/list?project=" + project
	}
	return "/partials/list?report=" + report
}

// partialURLWithSort builds the polling URL including the current sort and
// filter params. Default sort is omitted to keep URLs clean; empty filter is
// also omitted.
func partialURLWithSort(report, project string, s SortSpec, filter string) string {
	base := partialURL(report, project)
	var extras []string
	if !(s.Key == DefaultSort.Key && s.Asc == DefaultSort.Asc) {
		extras = append(extras, "sort="+FormatSort(s))
	}
	if filter != "" {
		extras = append(extras, "filter="+url.QueryEscape(filter))
	}
	if len(extras) == 0 {
		return base
	}
	sep := "&"
	if !strings.Contains(base, "?") {
		sep = "?"
	}
	return base + sep + strings.Join(extras, "&")
}

// sortURL builds the hx-get for a sortable column header. Clicking the
// currently-sorted key flips its direction; clicking any other key uses that
// key's default direction. report is the named report ("next", "ready", ...)
// or empty for project drilldowns; project is set on /project/<name> drilldowns.
func sortURL(report, project string, cur SortSpec, key string) string {
	next := SortSpec{Key: key, Asc: defaultDirAscMap[key]}
	if cur.Key == key {
		next.Asc = !cur.Asc
	}
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	} else if report != "" {
		q.Set("report", report)
	}
	q.Set("sort", FormatSort(next))
	return "/partials/list?" + q.Encode()
}

// defaultDirAscMap maps each sort key to its default ascending-or-descending
// direction. urgency desc (highest first), entry desc (newest first); the rest
// ascending.
var defaultDirAscMap = map[string]bool{
	"urgency":     false,
	"due":         true,
	"project":     true,
	"description": true,
	"entry":       false,
}

// sortLinkClass returns Tailwind classes for one column header, highlighting
// the currently-active sort key.
func sortLinkClass(cur SortSpec, key string) string {
	base := "inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-xs hover:text-zinc-900 dark:hover:text-zinc-100"
	if cur.Key == key {
		return base + " font-semibold text-zinc-900 dark:text-zinc-100"
	}
	return base + " text-zinc-500 dark:text-zinc-400"
}

// sortArrow returns the up/down glyph for a column whose key matches cur, or
// the empty string otherwise. Rendered next to the active column's label.
func sortArrow(cur SortSpec, key string) string {
	if cur.Key != key {
		return ""
	}
	if cur.Asc {
		return "↑" // up arrow
	}
	return "↓" // down arrow
}

// calendarSwitchURL preserves the current anchor date when toggling view mode.
func calendarSwitchURL(p CalendarPage, mode string) string {
	return fmt.Sprintf("/calendar?view=%s&date=%s", mode, p.Anchor.Format("2006-01-02"))
}

// calendarDayURL is the link from a month/week cell to that day's view.
func calendarDayURL(d time.Time) string {
	return fmt.Sprintf("/calendar?view=day&date=%s", d.Format("2006-01-02"))
}

// timesheetSwitchURL preserves the current anchor date when toggling view mode.
func timesheetSwitchURL(data TimesheetData, mode string) string {
	return fmt.Sprintf("/timesheet?view=%s&date=%s", mode, data.Anchor.Format("2006-01-02"))
}
