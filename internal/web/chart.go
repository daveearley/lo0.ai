package web

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/format"
)

// Chart geometry, in the SVG's own units. The SVG stretches to its box, so
// only the proportions matter.
const (
	chartW    = 1000
	chartH    = 200
	chartCols = 250
)

// curve is every session's peak context, shortest first, on a log scale,
// split at a model's window: the sessions left of the split fit, the rest
// would compact.
type curve struct {
	// Fit and Over are SVG paths for the two areas, and FitLine and
	// OverLine their top edges.
	Fit, Over, FitLine, OverLine string
	// Line is the window's height from the top, from 0 to 1, and Split the
	// share of sessions that fit.
	Line, Split float64
	Window      string
	Ticks       []tick
}

// tick labels a context size on the curve's scale.
type tick struct {
	Y     float64
	Label string
}

// contextCurve draws peaks, which are sorted ascending, against window.
func contextCurve(peaks []int, window int) (curve, bool) {
	if len(peaks) == 0 || window <= 0 {
		return curve{}, false
	}
	lo := math.Log2(float64(max(peaks[0], 4<<10)))
	hi := math.Log2(float64(max(peaks[len(peaks)-1], window, 1<<20)))
	if hi <= lo {
		return curve{}, false
	}
	y := func(tokens int) float64 {
		v := (math.Log2(float64(max(tokens, 1))) - lo) / (hi - lo)
		return 1 - min(max(v, 0), 1)
	}
	fit, _ := slices.BinarySearch(peaks, window+1)
	c := curve{Line: y(window), Split: float64(fit) / float64(len(peaks)), Window: format.Tokens(window)}
	for _, t := range []int{32 << 10, 128 << 10, 1 << 20} {
		if yt := y(t); yt > 0.04 && yt < 0.96 && math.Abs(yt-c.Line) > 0.08 {
			c.Ticks = append(c.Ticks, tick{Y: yt, Label: format.Tokens(t)})
		}
	}

	cols := min(chartCols, len(peaks))
	split := int(math.Round(c.Split * float64(cols)))
	heights := make([]float64, cols)
	for i := range cols {
		heights[i] = y(peaks[i*len(peaks)/cols])
	}
	c.Fit, c.FitLine = area(heights, 0, split, cols)
	c.Over, c.OverLine = area(heights, split, cols, cols)
	return c, true
}

// area is the closed path under heights[from:to], with cols columns
// across the chart, and the line along its top.
func area(heights []float64, from, to, cols int) (string, string) {
	if from >= to {
		return "", ""
	}
	w := float64(chartW) / float64(cols)
	num := func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
	points := make([]string, 0, to-from+1)
	for i := from; i < to; i++ {
		points = append(points, num(float64(i)*w)+" "+num(heights[i]*chartH))
	}
	points = append(points, num(float64(to)*w)+" "+num(heights[to-1]*chartH))
	line := "M" + strings.Join(points, "L")
	fill := "M" + num(float64(from)*w) + " " + num(chartH) + "L" + strings.Join(points, "L") +
		"L" + num(float64(to)*w) + " " + num(chartH) + "Z"
	return fill, line
}
