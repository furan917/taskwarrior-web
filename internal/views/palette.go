package views

// urgencyTier collapses Taskwarrior's open-ended urgency score into the four
// ordinal levels used by tierSolid/tierLight/tierBadge: low/med/high/critical.
// Thresholds chosen to spread the band across typical Taskwarrior data without
// either flooding "critical" or starving the high tiers.
func urgencyTier(score float64) string {
	switch {
	case score >= 9:
		return "critical"
	case score >= 6:
		return "high"
	case score >= 3:
		return "med"
	default:
		return "low"
	}
}

// tierSolid: filled chip with high-contrast text. For due-day chips and the
// urgency bar. Yellow uses dark text because yellow-on-white can't meet AA.
//
// Dark mode: blue/orange/red read well as-is on a dark page. Yellow-400 on
// a dark page can wash out, so we bump to yellow-500 with the same dark
// text - keeps the "med = yellow" identity but with more saturation.
//
// One row in the four-tier palette. The siblings (tierLight, tierBadge,
// urgencyBarColour) all consume the same urgencyTier() input so a score
// hitting "high" reads as orange in every surface. Keep the four functions'
// hue identity in sync if you tweak any one of them.
func tierSolid(tier string) string {
	switch tier {
	case "critical":
		return "bg-red-600 text-white"
	case "high":
		return "bg-orange-700 text-white"
	case "med":
		return "bg-yellow-400 text-zinc-900 dark:bg-yellow-500"
	default:
		return "bg-blue-600 text-white"
	}
}

// tierLight: light fill, dark text, inset ring. For scheduled-day chips
// (multi-day spans before the due day).
//
// Dark mode inverts the chip - very dark coloured fill + light coloured text
// + a still-visible ring - so the "scheduled day" remains distinguishable
// from the "due day" (solid) at a glance, the same colourblind-safe contrast
// distinction we rely on in light mode.
func tierLight(tier string) string {
	switch tier {
	case "critical":
		return "bg-red-100 text-red-900 ring-1 ring-inset ring-red-300 dark:bg-red-950/60 dark:text-red-200 dark:ring-red-800"
	case "high":
		return "bg-orange-100 text-orange-900 ring-1 ring-inset ring-orange-400 dark:bg-orange-950/60 dark:text-orange-200 dark:ring-orange-800"
	case "med":
		return "bg-yellow-100 text-yellow-900 ring-1 ring-inset ring-yellow-400 dark:bg-yellow-950/60 dark:text-yellow-200 dark:ring-yellow-800"
	default:
		return "bg-blue-100 text-blue-900 ring-1 ring-inset ring-blue-300 dark:bg-blue-950/60 dark:text-blue-200 dark:ring-blue-800"
	}
}

// tierBadge: light fill + dark text, no ring. For the small inline due-date
// badge inside row.templ - kept minimal so it doesn't compete with the row.
func tierBadge(tier string) string {
	switch tier {
	case "critical":
		return "bg-red-100 text-red-900 dark:bg-red-950/60 dark:text-red-200"
	case "high":
		return "bg-orange-100 text-orange-900 dark:bg-orange-950/60 dark:text-orange-200"
	case "med":
		return "bg-yellow-100 text-yellow-900 dark:bg-yellow-950/60 dark:text-yellow-200"
	default:
		return "bg-blue-100 text-blue-900 dark:bg-blue-950/60 dark:text-blue-200"
	}
}

// urgencyPercent maps Taskwarrior's open-ended urgency score to 0-100. Anything
// above ~10 is exceptional and clamps at 100.
func urgencyPercent(score float64) float64 {
	if score <= 0 {
		return 0
	}
	pct := score * 10
	if pct > 100 {
		pct = 100
	}
	return pct
}

// urgencyBarColour: the thin horizontal urgency-meter bar at the right of each
// row. Uses the 4-tier palette but strips text colour (the bar has no text);
// solid background only. Yellow gets the same dark-mode bump as tierSolid so
// the bar reads cleanly against bg-zinc-700 (the bar's track in dark mode).
//
// Sibling of dueBadgeClass (format.go): both are score/time-derived class
// helpers that bottom out on the same urgencyTier()-driven palette, so a
// "critical" row badge and a "critical" urgency bar always share a hue.
// urgencyDotColour returns the background-only Tailwind class for the
// small dot indicators used in the mobile calendar strip.
func urgencyDotColour(score float64) string {
	switch urgencyTier(score) {
	case "critical":
		return "bg-red-500"
	case "high":
		return "bg-orange-400"
	case "med":
		return "bg-yellow-400"
	default:
		return "bg-blue-500"
	}
}

func urgencyBarColour(score float64) string {
	switch urgencyTier(score) {
	case "critical":
		return "bg-red-600"
	case "high":
		return "bg-orange-700"
	case "med":
		return "bg-yellow-400 dark:bg-yellow-500"
	default:
		return "bg-blue-600"
	}
}
