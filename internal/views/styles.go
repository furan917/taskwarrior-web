package views

// Style helpers and tunables shared across templates. Keeping them in one Go
// file (rather than scattering string literals through each .templ) lets us
// (a) deduplicate repeated class strings and (b) surface the few magic
// numbers as named constants the templates can interpolate.
//
// Anything in here is a literal class string Tailwind's JIT can see at build
// time - resist composing classes from variables, since that defeats the
// scanner.

// CardClass is the canonical "rendered surface" chrome: rounded corner, light
// border, white fill in light mode, dark fill in dark mode. Used by every
// task-list row, the calendar grid surfaces, and the day-view section.
//
// Dark border is zinc-800 (matching row.templ's task card) - we picked the
// darker tone as canonical because it sits better against the zinc-950 page
// background. Some legacy sites used dark:border-zinc-700; those have been
// normalised onto this helper.
func CardClass() string {
	return "rounded border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
}

// EmptyCardClass is the dashed-border empty-state variant of CardClass. Same
// fill, same dark-mode rules, but the border becomes dashed and we drop the
// shadow because the surface is conceptually inactive.
func EmptyCardClass() string {
	return "rounded border border-dashed border-zinc-300 bg-white dark:border-zinc-800 dark:bg-zinc-900"
}

// RowCardClass is CardClass plus an at-a-glance active-task highlight when
// active=true: emerald left stripe + faint emerald tint. The two states are
// each emitted as fully literal class strings (no concatenation of utility
// fragments) so Tailwind's JIT scanner can see every class - composing
// utilities by string concat would defeat the scanner.
func RowCardClass(active bool) string {
	if active {
		return "rounded border border-zinc-200 border-l-4 border-l-emerald-500 bg-emerald-50/40 shadow-sm dark:border-zinc-800 dark:border-l-emerald-500 dark:bg-emerald-950/20"
	}
	return CardClass()
}

// PollIntervalSeconds is how often the task-list polls for updates. The HTMX
// trigger is gated by shouldRefresh() in app.js so polling pauses while the
// user is editing.
const PollIntervalSeconds = 30

// SearchDebounceMs is the delay applied to the search input's hx-trigger so
// keystrokes don't issue one request per character. 200ms keeps the feel
// snappy without flooding the server.
const SearchDebounceMs = 200

// FilterDebounceMs is the debounce for the ad-hoc Taskwarrior filter input.
// Slightly longer than SearchDebounceMs because each keystroke triggers a
// `task export` subprocess rather than an in-memory substring scan.
const FilterDebounceMs = 350

// SearchInputWidth is the Tailwind width class for the search input.
// Surfacing it as a const keeps the magic number out of the template and
// lets layout adjustments live in one place.
const SearchInputWidth = "w-full sm:w-36"

// FilterInputWidth is the Tailwind width class for the ad-hoc filter input.
// Wider than search because filter expressions are longer (e.g.
// "priority:H project:work due.before:eom").
const FilterInputWidth = "w-full sm:w-56"

// ModalMaxWidth is the Tailwind max-width class shared by every modal
// dialog. Centralising means one place to widen all modals at once.
const ModalMaxWidth = "max-w-xl"
