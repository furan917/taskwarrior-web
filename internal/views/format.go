package views

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/furan917/taskwarrior-web/internal/tw"
)

// udaDisplayLabel turns a raw UDA name into a friendly heading for the row
// expand panel. Without access to the UDA's `label` directive we fall back
// to capitalising the first letter and replacing underscores with spaces -
// good enough for the common naming patterns ("priority" -> "Priority",
// "eng_lead" -> "Eng lead"). When we plumb UDA defs through to row.templ
// we'll prefer the user-defined label over this fallback.
func udaDisplayLabel(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "_", " ")
}

// reportDisplayLabel title-cases a Taskwarrior report name for nav display
// ("recurring" -> "Recurring", "burndown.daily" -> "Burndown.daily"). TW
// emits report names lowercase by convention, but the nav reads as a
// proper noun list ("Recurring", "Blocked", ...) so the bare lowercase
// looks broken alongside the curated tabs (Ready / Next / Agenda).
//
// ASCII-only by design: tw.ReportNamePattern is `^[a-zA-Z0-9_-]+$`, so
// name[:1] always slices a single rune. A multi-byte first char would
// produce invalid UTF-8 here; the regex guard upstream prevents it.
func reportDisplayLabel(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// sortedUDANames returns the UDA map's keys (those with a non-empty value)
// in stable alphabetical order so re-renders of the same task keep field
// order consistent. Map iteration order in Go is randomised per range and
// would otherwise reshuffle the panel on every refresh.
func sortedUDANames(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// humanDate parses a Taskwarrior YYYYMMDDTHHMMSSZ timestamp and renders it as
// YYYY-MM-DD in the user's local zone - what the form should pre-fill so the
// user sees something readable. Empty input returns empty. Unparseable input
// is echoed back verbatim: a date <input> will reject it but the user sees
// the original value rather than a silent blank, which surfaces malformed
// data instead of hiding it.
func humanDate(raw string) string {
	if raw == "" {
		return ""
	}
	t, err := tw.ParseTime(raw)
	if err != nil || t.IsZero() {
		return raw
	}
	return t.Local().Format("2006-01-02")
}

// humanDateTime is humanDate's sibling for places that want to show time too
// (annotation entries, completion timestamps). Format: "Mon 2 Jan, 15:04".
func humanDateTime(raw string) string {
	if raw == "" {
		return ""
	}
	t, err := tw.ParseTime(raw)
	if err != nil || t.IsZero() {
		return raw
	}
	return t.Local().Format("Mon 2 Jan, 15:04")
}

// dueBadgeClass colours the small badge using the 4-tier palette so the visual
// language of "how urgent is this" stays consistent with the urgency bar and
// calendar chips. Tiers map to time-until-due, not raw urgency, because the
// badge IS the time signal:
//   - critical (red): overdue
//   - high (orange): due within 24h
//   - med (yellow): due within 7d
//   - low (blue): further out / no parse
//
// Lives in format.go (alongside humanDate / dueLabel) because the work here is
// time arithmetic. The colour-tier helper it delegates to (tierBadge) stays in
// palette.go.
//
// Sibling of urgencyBarColour (palette.go): both feed the same 4-tier palette
// but from different inputs (time-until-due here vs raw urgency score there).
// They deliberately share hue identity so a row's badge and bar tell the same
// story at a glance. Keep their tier semantics aligned.
func dueBadgeClass(raw string) string {
	const base = "inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
	t, err := tw.ParseTime(raw)
	if err != nil || t.IsZero() {
		return base + " " + tierBadge("low")
	}
	d := time.Until(t)
	var tier string
	switch {
	case d < 0:
		tier = "critical"
	case d < 24*time.Hour:
		tier = "high"
	case d < 7*24*time.Hour:
		tier = "med"
	default:
		tier = "low"
	}
	return base + " " + tierBadge(tier)
}

// dueLabel renders a Taskwarrior YYYYMMDDTHHMMSSZ string as a human label.
// Negative values mean overdue; <24h becomes "today"; otherwise "in Nd".
func dueLabel(raw string) string {
	t, err := tw.ParseTime(raw)
	if err != nil || t.IsZero() {
		return raw
	}
	d := time.Until(t)
	switch {
	case d < 0:
		return fmt.Sprintf("%dd overdue", int(math.Ceil(-d.Hours()/24)))
	case d < 24*time.Hour:
		return "today"
	default:
		return fmt.Sprintf("in %dd", int(math.Ceil(d.Hours()/24)))
	}
}

// udaInputType maps a Taskwarrior UDA type to an HTML input type. Unknown or
// empty types fall back to "text" so users still get a working field.
func udaInputType(t string) string {
	switch t {
	case "date":
		return "date"
	case "numeric":
		return "number"
	default:
		return "text"
	}
}

// udaPlaceholder is a hint string shown when the field is empty. Duration
// gets a literal example because Taskwarrior accepts ISO 8601 durations
// ("PT4H") as well as shorthand forms ("2d", "1w") and a placeholder is the
// only place the user sees that affordance without consulting the docs.
func udaPlaceholder(t string) string {
	switch t {
	case "duration":
		return "PT4H / 2d / 1w"
	case "numeric":
		return "0"
	case "date":
		return "YYYY-MM-DD"
	default:
		return ""
	}
}

// udaLabel returns the user-facing label, falling back to the bare name when
// the user didn't define a label in their taskrc.
func udaLabel(u tw.UDA) string {
	if u.Label != "" {
		return u.Label
	}
	return u.Name
}

// udaFormValues builds the name -> form-value map for the edit modal, mapping
// each declared UDA's stored value into a string the matching <input> can
// pre-fill. Date-typed UDAs are humanised to YYYY-MM-DD so the native date
// picker accepts them; everything else is passed through unchanged.
func udaFormValues(t tw.Task, udas []tw.UDA) map[string]string {
	if len(udas) == 0 || len(t.UDAs) == 0 {
		return nil
	}
	out := make(map[string]string, len(udas))
	for _, u := range udas {
		raw, ok := t.UDAs[u.Name]
		if !ok || raw == "" {
			continue
		}
		if u.Type == "date" {
			out[u.Name] = humanDate(raw)
		} else {
			out[u.Name] = raw
		}
	}
	return out
}

// annotationCountLabel pluralises the annotation count for the row-detail
// expand panel. Kept as a helper so the templ doesn't bake the singular/plural
// rule into the template literal.
func annotationCountLabel(n int) string {
	if n == 1 {
		return "1 annotation"
	}
	return fmt.Sprintf("%d annotations", n)
}

// blockedBadgeLabel returns the row-badge text for a task with N dependencies.
// We use a lock glyph plus a count so a single dependency reads naturally and
// the plural never adds an "s" mid-word. Kept here so the templ stays free of
// pluralisation logic.
func blockedBadgeLabel(n int) string {
	if n == 1 {
		return "\U0001F512 1 blocked"
	}
	return fmt.Sprintf("\U0001F512 %d blocked", n)
}

// shortUUID truncates a Taskwarrior UUID to the first 8 hex characters plus a
// trailing ellipsis so the row-expand "Blocked by" links show a recognisable
// prefix without taking a full line. Non-UUID input (numeric ids, anything
// shorter than 8 chars) is echoed back verbatim so we never pad a junk value
// to look like a UUID.
func shortUUID(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8] + "…"
}

// depDescriptionFor returns the description of the open task whose UUID equals
// uuid, or an empty string when no such task exists in the slice. The
// dependency picker calls this once per pre-selected dep when rendering the
// edit modal so each pill shows the readable label even though the form
// submits raw UUIDs.
func depDescriptionFor(uuid string, allOpenTasks []tw.Task) string {
	for _, t := range allOpenTasks {
		if t.UUID == uuid {
			return t.Description
		}
	}
	return ""
}

// depPillLabel formats a dependency pill's visible text. We prefer the task
// description when we have it (resolved from the open-tasks list); otherwise
// the short UUID is the only sensible fallback - it could mean the dep is on
// a completed task no longer in the open set, or the cache is stale. Either
// way the pill must render something readable.
func depPillLabel(uuid, description string) string {
	if description != "" {
		return description
	}
	return shortUUID(uuid)
}

func emptyMessage(report string) string {
	switch report {
	case "next":
		return "No pending tasks. Press n to add one."
	case "ready":
		return "No tasks ready right now. Items still waiting are hidden - see Agenda or Forecast."
	case "agenda":
		return "Nothing surfacing in the next 14 days."
	case "forecast":
		return "Nothing surfacing in the next 30 days."
	default:
		return "No tasks here."
	}
}
