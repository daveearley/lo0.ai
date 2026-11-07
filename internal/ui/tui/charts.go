package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/ui/draw"
	"lo0.ai/lo0/internal/ui/theme"
	"lo0.ai/lo0/internal/usage"
)

// sessionChart draws the user's session peaks as a histogram, green up to
// window and amber beyond it, with a marker line under the window. The scale
// runs to the longest session, so no bucket collects an overflow.
func sessionChart(t theme.Theme, ctx usage.Context, window, cells int) (chart, marker string) {
	limit := sessionLimit(ctx, window)
	spark := []rune(draw.Sparkline(draw.Histogram(ctx.Peaks, cells, limit)))
	pos := draw.Position(window, limit, cells)
	chart = t.Good.Render(string(spark[:pos])) + t.Warn.Render(string(spark[pos:]))
	label := "┊ " + format.Tokens(window)
	if pos+lipgloss.Width(label) > cells {
		label = format.Tokens(window) + " ┊"
		return chart, strings.Repeat(" ", max(pos-lipgloss.Width(label)+1, 0)) + label
	}
	return chart, strings.Repeat(" ", pos) + label
}

// memoryChart draws the recommended option's memory as weights, KV cache and
// runtime against the model memory budget.
func memoryChart(t theme.Theme, o match.Option, budget uint64, cells int) (chart, legend string) {
	seg := draw.Stacked([]float64{float64(o.WeightBytesOnGPU()), float64(o.KVBytes), float64(match.RuntimeOverhead)}, float64(budget), cells)
	chart = t.Accent.Render(strings.Repeat("█", seg[0])) +
		t.Value.Render(strings.Repeat("█", seg[1])) +
		t.Label.Render(strings.Repeat("█", seg[2])) +
		t.Muted.Render(strings.Repeat(draw.Track, seg[3]))
	legend = t.Accent.Render("■") + t.Muted.Render(" weights "+gb(o.WeightBytesOnGPU())+"   ") +
		t.Value.Render("■") + t.Muted.Render(" KV cache "+gb(o.KVBytes)+"   ") +
		t.Label.Render("■") + t.Muted.Render(" runtime "+gb(match.RuntimeOverhead))
	return chart, legend
}

func gb(b int64) string { return format.Bytes(uint64(max(b, 0))) }

// sessionLimit is the right edge of a session histogram: the longest session,
// or a little past the window if every session fits.
func sessionLimit(ctx usage.Context, window int) int {
	longest := 0
	if n := len(ctx.Peaks); n > 0 {
		longest = ctx.Peaks[n-1]
	}
	return max(longest, window+window/8, 1)
}

// atoms lays out items on lines no wider than width, never breaking inside
// an item. Continuation lines start with indent.
func atoms(items []string, sep string, width int, indent string) string {
	var lines []string
	line := ""
	for _, it := range items {
		candidate := it
		if line != "" {
			candidate = line + sep + it
		}
		if line != "" && lipgloss.Width(candidate) > width {
			lines = append(lines, line)
			line = indent + it
			continue
		}
		line = candidate
	}
	return strings.Join(append(lines, line), "\n")
}

// bar draws frac of cells in style, with a muted track.
func bar(t theme.Theme, frac float64, cells int, style func(...string) string) string {
	filled, track := draw.Bar(frac, cells)
	return style(filled) + t.Muted.Render(track)
}
