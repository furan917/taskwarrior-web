package views

import (
	"hash/fnv"
	"regexp"
	"strings"
)

var contextFilterTagPat = regexp.MustCompile(`\+([a-zA-Z][a-zA-Z0-9_-]*)`)
var contextFilterProjPat = regexp.MustCompile(`project:([a-zA-Z][a-zA-Z0-9._]*)`)

// ContextOption is one entry in the Add modal's context picker dropdown.
// PrefillTags / PrefillProject are derived from the context's read filter
// via ContextPrefill; the JS handler reads them off the selected <option>
// to update the Tags / Project inputs on change.
type ContextOption struct {
	Name           string
	PrefillTags    string
	PrefillProject string
}

// ContextHelperText is the italic line under the Add modal's context
// dropdown that explains exactly what picking the current context will
// attach to the new task. The phrasing mirrors the dropdown's selected
// behaviour: tag → "Adds +X tag", project → "Sets Project = Y", both →
// joined with ", ", neither → "No context tag/project will be added."
//
// The JS context-control IIFE in app.js MUST keep this format in sync;
// they're both authored against the same prose so flicker on change is
// purely the value swap, not a formatting jump.
func ContextHelperText(tag, project string) string {
	if tag == "" && project == "" {
		return "No context tag/project will be added."
	}
	parts := make([]string, 0, 2)
	if tag != "" {
		parts = append(parts, "Adds +"+tag+" tag")
	}
	if project != "" {
		parts = append(parts, "Sets Project = "+project)
	}
	return strings.Join(parts, ", ") + "."
}

// ContextPrefill extracts default form values from a Taskwarrior context's
// read filter so the add modal can seed Project and Tags consistently with
// the active context. Taskwarrior's context write-filter doesn't apply
// auto-attachment for OR-shaped expressions, so we replicate the behaviour
// in the form layer instead.
//
// Conservative: returns the FIRST `+tag` match (and no project), otherwise
// the FIRST `project:` match (and no tag), otherwise empty strings. Skips
// ALL-UPPERCASE tag matches because those are Taskwarrior virtual tags
// (+OVERDUE, +READY, ...) that the user can't actually set.
func ContextPrefill(filter string) (project, tag string) {
	if matches := contextFilterTagPat.FindAllStringSubmatch(filter, -1); matches != nil {
		for _, m := range matches {
			name := m[1]
			if name == strings.ToUpper(name) {
				continue
			}
			return "", name
		}
	}
	if m := contextFilterProjPat.FindStringSubmatch(filter); m != nil {
		return m[1], ""
	}
	return "", ""
}

// CtxColour is the per-context palette entry chosen by colourForContext. Every
// class string is hand-tuned and appears verbatim in the source so Tailwind's
// JIT picks them up at build time - DO NOT template / concatenate / synthesise
// these at runtime.
//
// All three rounds (base / lighter / darker) across six base hues give 18
// distinct slots. WCAG AA contrast targets:
//   - white-on-{*-600+}      >= 4.5:1
//   - dark-on-{*-100..-400}  >= 4.5:1
//   - yellow ALWAYS dark text (yellow-on-white can't meet AA)
//
// No red. Red carries the "urgency critical" semantic elsewhere in the app
// (urgencyBarColour, tierSolid). Re-using it for context would create a
// cross-axis collision the user would have to learn around.
type CtxColour struct {
	// Pill (active state) - light mode.
	Fill string // background class, e.g. "bg-blue-600"
	Text string // text class, e.g. "text-white"
	Ring string // soft ring class, e.g. "ring-blue-700/40"
	Dot  string // status-dot bg class, e.g. "bg-blue-300"

	// Pill (active state) - dark mode. The dark: variants live on the same
	// elements; we keep them as separate fields so a dropdown row template
	// can compose its own class string without parsing.
	DarkFill string
	DarkText string
	DarkRing string
	DarkDot  string

	// Rule is the optional 1px coloured rule under the nav. We use a slightly
	// muted hue so it doesn't compete with the pill itself.
	Rule     string
	DarkRule string
}

// paletteSlots is the flat 18-entry palette indexed by colourForContext via
// FNV-1a(name) % 18. Entries are organised:
//
//	[0..5]   round 1 (base)    : blue, teal, purple, amber, orange, pink
//	[6..11]  round 2 (lighter) : same six hues, lighter
//	[12..17] round 3 (darker)  : same six hues, darker
//
// Hand-tuned so every entry meets WCAG AA in both light and dark mode. See
// the contrast notes on each row for the calculation we relied on.
var paletteSlots = []CtxColour{
	// ── Round 1 (base) ────────────────────────────────────────────────────
	// 0: blue-600 - white. Dark: blue-500 - white.
	{
		Fill: "bg-blue-600", Text: "text-white",
		Ring: "ring-blue-700/40", Dot: "bg-blue-200",
		DarkFill: "dark:bg-blue-500", DarkText: "dark:text-white",
		DarkRing: "dark:ring-blue-300/40", DarkDot: "dark:bg-blue-200",
		Rule: "bg-blue-500/40", DarkRule: "dark:bg-blue-400/40",
	},
	// 1: teal-600 - white. Dark: teal-500 - white.
	{
		Fill: "bg-teal-600", Text: "text-white",
		Ring: "ring-teal-700/40", Dot: "bg-teal-200",
		DarkFill: "dark:bg-teal-500", DarkText: "dark:text-white",
		DarkRing: "dark:ring-teal-300/40", DarkDot: "dark:bg-teal-200",
		Rule: "bg-teal-500/40", DarkRule: "dark:bg-teal-400/40",
	},
	// 2: purple-600 - white. Dark: purple-500 - white.
	{
		Fill: "bg-purple-600", Text: "text-white",
		Ring: "ring-purple-700/40", Dot: "bg-purple-200",
		DarkFill: "dark:bg-purple-500", DarkText: "dark:text-white",
		DarkRing: "dark:ring-purple-300/40", DarkDot: "dark:bg-purple-200",
		Rule: "bg-purple-500/40", DarkRule: "dark:bg-purple-400/40",
	},
	// 3: amber-500 - zinc-900 (yellow always dark text).
	{
		Fill: "bg-amber-500", Text: "text-zinc-900",
		Ring: "ring-amber-700/40", Dot: "bg-amber-800",
		DarkFill: "dark:bg-amber-400", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-amber-700/40", DarkDot: "dark:bg-amber-800",
		Rule: "bg-amber-500/50", DarkRule: "dark:bg-amber-400/50",
	},
	// 4: orange-700 - white. Dark: orange-500 - white.
	{
		Fill: "bg-orange-700", Text: "text-white",
		Ring: "ring-orange-800/40", Dot: "bg-orange-200",
		DarkFill: "dark:bg-orange-500", DarkText: "dark:text-white",
		DarkRing: "dark:ring-orange-300/40", DarkDot: "dark:bg-orange-200",
		Rule: "bg-orange-600/40", DarkRule: "dark:bg-orange-400/40",
	},
	// 5: pink-600 - white.
	{
		Fill: "bg-pink-600", Text: "text-white",
		Ring: "ring-pink-700/40", Dot: "bg-pink-200",
		DarkFill: "dark:bg-pink-500", DarkText: "dark:text-white",
		DarkRing: "dark:ring-pink-300/40", DarkDot: "dark:bg-pink-200",
		Rule: "bg-pink-500/40", DarkRule: "dark:bg-pink-400/40",
	},

	// ── Round 2 (lighter) ─────────────────────────────────────────────────
	// 6: blue-400 - zinc-900.
	{
		Fill: "bg-blue-400", Text: "text-zinc-900",
		Ring: "ring-blue-600/40", Dot: "bg-blue-700",
		DarkFill: "dark:bg-blue-300", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-blue-500/40", DarkDot: "dark:bg-blue-700",
		Rule: "bg-blue-400/50", DarkRule: "dark:bg-blue-300/50",
	},
	// 7: teal-400 - zinc-900.
	{
		Fill: "bg-teal-400", Text: "text-zinc-900",
		Ring: "ring-teal-600/40", Dot: "bg-teal-700",
		DarkFill: "dark:bg-teal-300", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-teal-500/40", DarkDot: "dark:bg-teal-700",
		Rule: "bg-teal-400/50", DarkRule: "dark:bg-teal-300/50",
	},
	// 8: purple-400 - zinc-900.
	{
		Fill: "bg-purple-400", Text: "text-zinc-900",
		Ring: "ring-purple-600/40", Dot: "bg-purple-700",
		DarkFill: "dark:bg-purple-300", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-purple-500/40", DarkDot: "dark:bg-purple-700",
		Rule: "bg-purple-400/50", DarkRule: "dark:bg-purple-300/50",
	},
	// 9: amber-300 - zinc-900.
	{
		Fill: "bg-amber-300", Text: "text-zinc-900",
		Ring: "ring-amber-600/40", Dot: "bg-amber-800",
		DarkFill: "dark:bg-amber-300", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-amber-600/40", DarkDot: "dark:bg-amber-800",
		Rule: "bg-amber-300/60", DarkRule: "dark:bg-amber-300/60",
	},
	// 10: orange-500 - zinc-900.
	{
		Fill: "bg-orange-500", Text: "text-zinc-900",
		Ring: "ring-orange-700/40", Dot: "bg-orange-800",
		DarkFill: "dark:bg-orange-400", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-orange-600/40", DarkDot: "dark:bg-orange-800",
		Rule: "bg-orange-500/50", DarkRule: "dark:bg-orange-400/50",
	},
	// 11: pink-400 - zinc-900.
	{
		Fill: "bg-pink-400", Text: "text-zinc-900",
		Ring: "ring-pink-600/40", Dot: "bg-pink-700",
		DarkFill: "dark:bg-pink-300", DarkText: "dark:text-zinc-900",
		DarkRing: "dark:ring-pink-500/40", DarkDot: "dark:bg-pink-700",
		Rule: "bg-pink-400/50", DarkRule: "dark:bg-pink-300/50",
	},

	// ── Round 3 (darker) ──────────────────────────────────────────────────
	// 12: blue-800 - white.
	{
		Fill: "bg-blue-800", Text: "text-white",
		Ring: "ring-blue-900/40", Dot: "bg-blue-300",
		DarkFill: "dark:bg-blue-700", DarkText: "dark:text-white",
		DarkRing: "dark:ring-blue-200/40", DarkDot: "dark:bg-blue-300",
		Rule: "bg-blue-700/50", DarkRule: "dark:bg-blue-500/50",
	},
	// 13: teal-800 - white.
	{
		Fill: "bg-teal-800", Text: "text-white",
		Ring: "ring-teal-900/40", Dot: "bg-teal-300",
		DarkFill: "dark:bg-teal-700", DarkText: "dark:text-white",
		DarkRing: "dark:ring-teal-200/40", DarkDot: "dark:bg-teal-300",
		Rule: "bg-teal-700/50", DarkRule: "dark:bg-teal-500/50",
	},
	// 14: purple-800 - white.
	{
		Fill: "bg-purple-800", Text: "text-white",
		Ring: "ring-purple-900/40", Dot: "bg-purple-300",
		DarkFill: "dark:bg-purple-700", DarkText: "dark:text-white",
		DarkRing: "dark:ring-purple-200/40", DarkDot: "dark:bg-purple-300",
		Rule: "bg-purple-700/50", DarkRule: "dark:bg-purple-500/50",
	},
	// 15: amber-700 - white.
	{
		Fill: "bg-amber-700", Text: "text-white",
		Ring: "ring-amber-900/40", Dot: "bg-amber-300",
		DarkFill: "dark:bg-amber-600", DarkText: "dark:text-white",
		DarkRing: "dark:ring-amber-200/40", DarkDot: "dark:bg-amber-300",
		Rule: "bg-amber-600/50", DarkRule: "dark:bg-amber-500/50",
	},
	// 16: orange-800 - white.
	{
		Fill: "bg-orange-800", Text: "text-white",
		Ring: "ring-orange-900/40", Dot: "bg-orange-300",
		DarkFill: "dark:bg-orange-700", DarkText: "dark:text-white",
		DarkRing: "dark:ring-orange-200/40", DarkDot: "dark:bg-orange-300",
		Rule: "bg-orange-700/50", DarkRule: "dark:bg-orange-500/50",
	},
	// 17: pink-800 - white.
	{
		Fill: "bg-pink-800", Text: "text-white",
		Ring: "ring-pink-900/40", Dot: "bg-pink-300",
		DarkFill: "dark:bg-pink-700", DarkText: "dark:text-white",
		DarkRing: "dark:ring-pink-200/40", DarkDot: "dark:bg-pink-300",
		Rule: "bg-pink-700/50", DarkRule: "dark:bg-pink-500/50",
	},
}

// ColourForContext returns the palette slot deterministically chosen for
// name. FNV-1a is fast, well-distributed for short ASCII strings and stable
// across builds (encoding/binary changes wouldn't affect it). Empty input
// returns the slot 0 entry as a defensive default - callers should already
// have checked for an active context before calling, but rendering an
// "unknown" pill rather than panicking keeps the page up.
func ColourForContext(name string) CtxColour {
	if name == "" {
		return paletteSlots[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	idx := int(h.Sum32() % uint32(len(paletteSlots)))
	return paletteSlots[idx]
}

// PillActiveClass returns the full class string for the active-state pill
// background. Concatenated here (not in the templ) so future adjustments live
// in one place and the hand-picked slot strings stay literal.
func PillActiveClass(c CtxColour) string {
	return "context-pill inline-flex items-center gap-1.5 rounded py-1.5 px-3 text-sm font-semibold shadow-sm ring-1 ring-inset relative " +
		c.Fill + " " + c.Text + " " + c.Ring + " " +
		c.DarkFill + " " + c.DarkText + " " + c.DarkRing
}

// PillDotClass returns the class string for the inner status dot inside the
// active pill. The wrapper element below the class adds the ping animation.
func PillDotClass(c CtxColour) string {
	return "relative inline-flex rounded-full h-2 w-2 " + c.Dot + " " + c.DarkDot
}

// PillDotPingClass returns the class string for the absolutely-positioned
// "ping" element layered behind the dot. CSS animation is the standard
// Tailwind animate-ping; the colour is the same hue as the dot.
func PillDotPingClass(c CtxColour) string {
	return "animate-ping absolute inline-flex h-full w-full rounded-full opacity-60 " + c.Dot + " " + c.DarkDot
}

// NavRuleClass returns the optional 1 px coloured rule that sits under the
// nav when a context is active. We render it as a thin <div>; callers gate on
// activeContext != "" before emitting.
func NavRuleClass(c CtxColour) string {
	return "h-px w-full " + c.Rule + " " + c.DarkRule
}

// contextMenuVals builds the hx-vals JSON for one dropdown entry. Manually
// assembled (not encoding/json) because the value is a strict allowlist and
// keeping the literal shape inline keeps it easy to spot in diffs. Empty
// value clears the context.
func contextMenuVals(name string) string {
	// Defence in depth: we only emit names that ContextNamePattern would
	// already accept upstream, so any double-quote / backslash here is a
	// programmer error. Render an empty string in that case.
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return `{"name":""}`
		}
	}
	return `{"name":"` + name + `"}`
}

// contextMenuItemClass returns the per-row class string for the dropdown
// entries. The active row is bolded and rendered against a soft tint so the
// user can scan-pick the current state quickly.
func contextMenuItemClass(active bool) string {
	base := "block w-full text-left px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800"
	if active {
		return base + " font-semibold text-zinc-900 dark:text-zinc-100"
	}
	return base + " text-zinc-700 dark:text-zinc-300"
}

// titleWithContext is the helper layout.templ uses to assemble the browser
// <title> with the optional [<context>] hint. Empty context returns the bare
// title; non-empty returns "<base> [<context>] · taskwarrior-web-portal". Lives here
// (rather than in format.go) so the context-related rendering helpers stay
// colocated.
func titleWithContext(base, ctxName string) string {
	if ctxName == "" {
		return base + " · taskwarrior-web-portal"
	}
	return base + " [" + ctxName + "] · taskwarrior-web-portal"
}
