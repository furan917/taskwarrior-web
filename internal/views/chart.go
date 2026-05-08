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
			padBottom = 22 // room for axis labels
			padSide   = 8
			barGap    = 2
		)
		drawWidth := width - 2*padSide
		drawHeight := height - padTop - padBottom

		// Reverse to oldest-first for left-to-right rendering. The slice
		// arrives newest-first from the handler.
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
			max = 1 // avoid div by zero; bars all render at min height
		}

		barWidth := float64(drawWidth)/float64(len(ordered)) - barGap
		if barWidth < 1 {
			barWidth = 1
		}

		var b strings.Builder
		fmt.Fprintf(&b,
			`<svg viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet" class="h-32 w-full" role="img" aria-label="Completion history bar chart">`,
			width, height)
		// Baseline rule.
		fmt.Fprintf(&b,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" class="stroke-zinc-200 dark:stroke-zinc-700" stroke-width="1"/>`,
			padSide, padTop+drawHeight, width-padSide, padTop+drawHeight)

		for i, d := range ordered {
			x := float64(padSide) + float64(i)*(barWidth+barGap)
			h := 0.0
			if d.Count > 0 {
				h = float64(drawHeight) * float64(d.Count) / float64(max)
				if h < 2 {
					h = 2 // visible-floor for non-zero days
				}
			}
			y := float64(padTop+drawHeight) - h
			fmt.Fprintf(&b,
				`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="fill-emerald-500/80 dark:fill-emerald-400/70"><title>%s: %d completed</title></rect>`,
				x, y, barWidth, h, templ.EscapeString(d.Date), d.Count)
		}

		// Axis labels: render every Nth label so they don't overlap. Aim
		// for ~7 visible labels regardless of window size.
		step := 1
		if len(ordered) > 7 {
			step = (len(ordered) + 6) / 7
		}
		for i, d := range ordered {
			if i%step != 0 && i != len(ordered)-1 {
				continue
			}
			x := float64(padSide) + float64(i)*(barWidth+barGap) + barWidth/2
			fmt.Fprintf(&b,
				`<text x="%.2f" y="%d" text-anchor="middle" class="fill-zinc-500 text-[10px] dark:fill-zinc-400">%s</text>`,
				x, height-6, templ.EscapeString(d.Label))
		}
		b.WriteString(`</svg>`)
		_, err := io.WriteString(w, b.String())
		return err
	})
}
