// Package draw renders small data graphics with block characters: bars,
// stacked bars, sparklines and histograms. Functions return plain strings so
// callers style them; nothing here knows about colours or themes.
package draw

import (
	"math"
	"strings"
)

// eighths are the partial blocks from one eighth to seven eighths wide.
var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// levels are the sparkline heights from lowest to highest.
var levels = []rune("▁▂▃▄▅▆▇█")

// Track is the character for the unfilled part of a bar.
const Track = "░"

// Bar splits a bar of width cells filled to frac (0 to 1) into its filled
// part, drawn with eighth-block precision, and its track. Styling them
// separately keeps the bar legible without colour.
func Bar(frac float64, width int) (filled, track string) {
	if width <= 0 {
		return "", ""
	}
	frac = clamp(frac)
	units := int(math.Round(frac * float64(width*8)))
	full, part := units/8, units%8
	filled = strings.Repeat("█", full) + eighths[part]
	used := full
	if part > 0 {
		used++
	}
	return filled, strings.Repeat(Track, width-used)
}

// Stacked divides width cells between values in proportion to their share
// of total, so the segments line up exactly with a bar of the same width.
// The last element of the result is the unused remainder.
func Stacked(values []float64, total float64, width int) []int {
	out := make([]int, len(values)+1)
	if width <= 0 || total <= 0 {
		out[len(values)] = max(width, 0)
		return out
	}
	used, sum := 0, 0.0
	for i, v := range values {
		sum += math.Max(v, 0)
		end := int(math.Round(clamp(sum/total) * float64(width)))
		out[i] = max(end-used, 0)
		used += out[i]
	}
	out[len(values)] = width - used
	return out
}

// Sparkline draws values as a row of heights scaled to the largest value.
// Zero draws as a space so empty buckets read as gaps.
func Sparkline(values []int) string {
	top := 0
	for _, v := range values {
		top = max(top, v)
	}
	var b strings.Builder
	for _, v := range values {
		if v <= 0 || top == 0 {
			b.WriteByte(' ')
			continue
		}
		i := int(math.Ceil(float64(v)/float64(top)*float64(len(levels)))) - 1
		b.WriteRune(levels[min(max(i, 0), len(levels)-1)])
	}
	return b.String()
}

// Histogram counts values into buckets equal-width buckets covering
// [0, limit]. Values above limit go in the last bucket.
func Histogram(values []int, buckets, limit int) []int {
	out := make([]int, max(buckets, 0))
	if buckets <= 0 || limit <= 0 {
		return out
	}
	for _, v := range values {
		i := int(float64(max(v, 0)) / float64(limit) * float64(buckets))
		out[min(i, buckets-1)]++
	}
	return out
}

// Position maps value on a scale of [0, limit] to a cell in [0, width).
func Position(value, limit, width int) int {
	if width <= 0 || limit <= 0 {
		return 0
	}
	return min(int(float64(max(value, 0))/float64(limit)*float64(width)), width-1)
}

func clamp(f float64) float64 {
	if math.IsNaN(f) {
		return 0
	}
	return math.Min(math.Max(f, 0), 1)
}

// Columns draws values as vertical bars height rows tall, scaled to the
// largest value, with eighth-block precision at the top of each bar. It
// returns the rows from top to bottom, each one cell per value. A value
// above zero always shows at least its lowest eighth.
func Columns(values []int, height int) []string {
	if height <= 0 {
		return nil
	}
	top := 0
	for _, v := range values {
		top = max(top, v)
	}
	rows := make([][]rune, height)
	for i := range rows {
		rows[i] = []rune(strings.Repeat(" ", len(values)))
	}
	for col, v := range values {
		if v <= 0 || top == 0 {
			continue
		}
		units := max(int(math.Round(float64(v)/float64(top)*float64(height*8))), 1)
		for row := range height {
			fromBottom := height - 1 - row
			fill := units - fromBottom*8
			switch {
			case fill >= 8:
				rows[row][col] = '█'
			case fill > 0:
				rows[row][col] = levels[fill-1]
			}
		}
	}
	out := make([]string, height)
	for i, r := range rows {
		out[i] = string(r)
	}
	return out
}
