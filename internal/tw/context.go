package tw

import (
	"regexp"
	"strings"
)

// Context represents one entry from `task contexts`. Taskwarrior 3 supports
// per-context read and write filters; we surface both columns even though the
// UI currently only activates a context (it doesn't define new ones).
type Context struct {
	Name        string
	ReadFilter  string
	WriteFilter string
	Active      bool
}

// ContextNamePattern is the strict allowlist used when activating a context.
// Taskwarrior accepts any non-whitespace name but we restrict to letters,
// digits, dash, underscore so a hostile taskrc cannot smuggle parser tokens
// or rc.* prefixes through `task context <name>`. Matches the shape of
// projectListPattern / tagListPattern.
var ContextNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// FilterContainsRcOverride reports whether a Taskwarrior filter expression
// embeds an `rc.*` configuration override token. A hostile or stale ~/.taskrc
// could store something like `rc.data.location=/tmp/evil` inside a context's
// read or write filter; Taskwarrior honours such overrides at exec time even
// when our argv composer wraps the whole filter in parens, because parens
// don't gate config-token parsing. The exported guardArgs check on caller
// args slices doesn't help because the entire filter is a single argv
// element starting with `(`.
//
// Used by Context.SafeReadFilter / SafeWriteFilter at point of use, and by
// context-defining code if we ever add it. Defence in depth - the user
// owns their taskrc, but we don't have to be a pass-through for tokens
// the rest of the codebase already rejects.
func FilterContainsRcOverride(filter string) bool {
	for tok := range strings.FieldsSeq(filter) {
		clean := strings.TrimLeft(tok, "(!~")
		if strings.HasPrefix(clean, "rc.") {
			return true
		}
	}
	return false
}

// exportIncompatibleToken matches the `-+word` pattern: a tag negation using
// the "has tag" operator. `task list` / `task next` accept this form, but
// `task export` does not — it returns exit 2 with "The expression could not
// be evaluated." The correct form for export is `-word` (no `+`).
var exportIncompatibleToken = regexp.MustCompile(`(?:^|\s)-\+\w`)

// FilterContainsExportIncompatible reports whether a filter uses syntax that
// `task export` cannot parse. Specifically, `-+tag` negation tokens that work
// in report commands but are rejected by the export expression evaluator.
func FilterContainsExportIncompatible(filter string) bool {
	return exportIncompatibleToken.MatchString(filter)
}

// SafeReadFilter returns the context's read filter only when it carries no
// rc.* override tokens and no export-incompatible syntax; otherwise empty.
// Callers that compose this into argv (handlers/views.exportWithContext) treat
// empty as "no context clause" — the user keeps a working app even if a
// context filter is malformed or uses report-only syntax.
func (c Context) SafeReadFilter() string {
	if FilterContainsRcOverride(c.ReadFilter) {
		return ""
	}
	if FilterContainsExportIncompatible(c.ReadFilter) {
		return ""
	}
	return c.ReadFilter
}

// SafeWriteFilter mirrors SafeReadFilter for the write side. We don't
// currently use the write filter for argv composition (the form-prefill
// helper reads the filter shape but doesn't pass it back to Taskwarrior),
// so this is here for symmetry and future safety.
func (c Context) SafeWriteFilter() string {
	if FilterContainsRcOverride(c.WriteFilter) {
		return ""
	}
	return c.WriteFilter
}

// SanitizeUserFilter trims whitespace and returns the filter expression if it
// is safe to pass to `task export` as a user-supplied filter. Returns empty
// string when the input contains rc.* override tokens so a hostile or
// accidental rc.* string cannot alter Taskwarrior's runtime config through
// the filter box. Export-incompatible -+tag syntax is left for Taskwarrior
// to reject (rather than silently stripped) so the user sees an error they
// can act on.
func SanitizeUserFilter(raw string) string {
	f := strings.TrimSpace(raw)
	if f == "" {
		return ""
	}
	if FilterContainsRcOverride(f) {
		return ""
	}
	return f
}
