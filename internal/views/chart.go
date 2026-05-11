package views

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"
)

// completionChartSVG renders a simple horizontal-axis bar chart of
// completion counts over time, scaled to the window's max value. Pure
// server-rendered SVG; no JS, no external assets. Width is fluid via
// `preserveAspectRatio="xMidYMid meet"` and `viewBox`, so the chart fills
// whatever container it's dropped into.
//
// Colour comes from the same emerald palette as the "Done" stat card -
// the bar IS a completion. Empty days render as a thin baseline stroke
// rather than a zero-height invisible bar so the day axis is still
// readable.
//
// We hand-roll SVG (rather than reaching for a Charts library) because
// this is the only chart in the app, the data is tiny (~14-30 bars), and
// avoiding a JS dep keeps the page server-rendered.
func completionChartSVG(history []DayCount) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if len(history) == 0 {
			return nil
		}
		const (
			width     = 800
			height    = 140
			padTop    = 8
			padBottom = 22
			padLeft   = 28 // room for y-axis labels
			padRight  = 8
			barGap    = 2
		)
		drawWidth := width - padLeft - padRight
		drawHeight := height - padTop - padBottom

		// Reverse to oldest-first for left-to-right rendering.
		ordered := make([]DayCount, len(history))
		for i, d := range history {
			ordered[len(history)-1-i] = d
		}

		max := 0
		for _, d := range ordered {
			if d.Count > max {
				max = d.Count
			}
		}
		if max == 0 {
			max = 1
		}

		barWidth := float64(drawWidth)/float64(len(ordered)) - barGap
		if barWidth < 1 {
			barWidth = 1
		}

		var b strings.Builder
		fmt.Fprintf(&b,
			`<svg viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet" class="h-32 w-full" role="img" aria-label="Completion history bar chart">`,
			width, height)
		// Baseline and y-axis tick lines.
		fmt.Fprintf(&b,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" class="stroke-zinc-200 dark:stroke-zinc-700" stroke-width="1"/>`,
			padLeft, padTop+drawHeight, width-padRight, padTop+drawHeight)
		fmt.Fprintf(&b,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" class="stroke-zinc-200 dark:stroke-zinc-700" stroke-width="1"/>`,
			padLeft, padTop, padLeft, padTop+drawHeight)
		// Y-axis labels.
		fmt.Fprintf(&b,
			`<text x="%d" y="%d" text-anchor="end" class="fill-zinc-400 text-[10px] dark:fill-zinc-500">%d</text>`,
			padLeft-4, padTop+10, max)
		fmt.Fprintf(&b,
			`<text x="%d" y="%d" text-anchor="end" class="fill-zinc-400 text-[10px] dark:fill-zinc-500">0</text>`,
			padLeft-4, padTop+drawHeight)

		for i, d := range ordered {
			x := float64(padLeft) + float64(i)*(barWidth+barGap)
			h := 0.0
			if d.Count > 0 {
				h = float64(drawHeight) * float64(d.Count) / float64(max)
				if h < 2 {
					h = 2
				}
			}
			y := float64(padTop+drawHeight) - h
			fmt.Fprintf(&b,
				`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="fill-emerald-500/80 dark:fill-emerald-400/70"><title>%s: %d completed</title></rect>`,
				x, y, barWidth, h, templ.EscapeString(d.Date), d.Count)
		}

		step := 1
		if len(ordered) > 7 {
			step = (len(ordered) + 6) / 7
		}
		for i, d := range ordered {
			if i%step != 0 && i != len(ordered)-1 {
				continue
			}
			x := float64(padLeft) + float64(i)*(barWidth+barGap) + barWidth/2
			fmt.Fprintf(&b,
				`<text x="%.2f" y="%d" text-anchor="middle" class="fill-zinc-500 text-[10px] dark:fill-zinc-400">%s</text>`,
				x, height-6, templ.EscapeString(d.Label))
		}
		b.WriteString(`</svg>`)
		_, err := io.WriteString(w, b.String())
		return err
	})
}

// burndownChartSVG renders the three-band stacked burndown chart matching
// `task burndown`: Pending (blue, bottom), Started (amber, middle), Done
// (emerald, top — cumulative within the data window). Bars are oldest-first.
func burndownChartSVG(bars []BurndownBar) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if len(bars) == 0 {
			return nil
		}
		const (
			width     = 800
			height    = 140
			padTop    = 8
			padBottom = 22
			padLeft   = 28
			padRight  = 8
			barGap    = 2
		)
		drawWidth := width - padLeft - padRight
		drawHeight := height - padTop - padBottom

		maxTotal := 0
		for _, b := range bars {
			if t := b.Pending + b.Started + b.Done; t > maxTotal {
				maxTotal = t
			}
		}
		if maxTotal == 0 {
			maxTotal = 1
		}

		barWidth := float64(drawWidth)/float64(len(bars)) - barGap
		if barWidth < 1 {
			barWidth = 1
		}
		scale := float64(drawHeight) / float64(maxTotal)
		baseline := float64(padTop + drawHeight)

		var sb strings.Builder
		fmt.Fprintf(&sb,
			`<svg viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet" class="h-32 w-full" role="img" aria-label="Burndown chart">`,
			width, height)
		fmt.Fprintf(&sb,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" class="stroke-zinc-200 dark:stroke-zinc-700" stroke-width="1"/>`,
			padLeft, padTop+drawHeight, width-padRight, padTop+drawHeight)
		fmt.Fprintf(&sb,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" class="stroke-zinc-200 dark:stroke-zinc-700" stroke-width="1"/>`,
			padLeft, padTop, padLeft, padTop+drawHeight)
		fmt.Fprintf(&sb,
			`<text x="%d" y="%d" text-anchor="end" class="fill-zinc-400 text-[10px] dark:fill-zinc-500">%d</text>`,
			padLeft-4, padTop+10, maxTotal)
		fmt.Fprintf(&sb,
			`<text x="%d" y="%d" text-anchor="end" class="fill-zinc-400 text-[10px] dark:fill-zinc-500">0</text>`,
			padLeft-4, padTop+drawHeight)

		for i, b := range bars {
			x := float64(padLeft) + float64(i)*(barWidth+barGap)

			pendingH := float64(b.Pending) * scale
			startedH := float64(b.Started) * scale
			doneH := float64(b.Done) * scale

			// Ensure non-zero bands have a visible minimum height.
			if b.Pending > 0 && pendingH < 2 {
				pendingH = 2
			}
			if b.Started > 0 && startedH < 2 {
				startedH = 2
			}
			if b.Done > 0 && doneH < 2 {
				doneH = 2
			}

			// Stacked from baseline upward: pending (bottom), started, done (top).
			if pendingH > 0 {
				y := baseline - pendingH
				fmt.Fprintf(&sb,
					`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="fill-blue-500/70 dark:fill-blue-400/60"><title>%s: %d pending</title></rect>`,
					x, y, barWidth, pendingH, templ.EscapeString(b.Date), b.Pending)
			}
			if startedH > 0 {
				y := baseline - pendingH - startedH
				fmt.Fprintf(&sb,
					`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="fill-amber-400/80 dark:fill-amber-300/70"><title>%s: %d started</title></rect>`,
					x, y, barWidth, startedH, templ.EscapeString(b.Date), b.Started)
			}
			if doneH > 0 {
				y := baseline - pendingH - startedH - doneH
				fmt.Fprintf(&sb,
					`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="fill-emerald-500/70 dark:fill-emerald-400/60"><title>%s: %d done (cumulative)</title></rect>`,
					x, y, barWidth, doneH, templ.EscapeString(b.Date), b.Done)
			}
		}

		step := 1
		if len(bars) > 7 {
			step = (len(bars) + 6) / 7
		}
		for i, b := range bars {
			if i%step != 0 && i != len(bars)-1 {
				continue
			}
			x := float64(padLeft) + float64(i)*(barWidth+barGap) + barWidth/2
			fmt.Fprintf(&sb,
				`<text x="%.2f" y="%d" text-anchor="middle" class="fill-zinc-500 text-[10px] dark:fill-zinc-400">%s</text>`,
				x, height-6, templ.EscapeString(b.Label))
		}
		sb.WriteString(`</svg>`)
		_, err := io.WriteString(w, sb.String())
		return err
	})
}
