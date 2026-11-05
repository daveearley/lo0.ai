package report

import (
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/match"
)

// SourceLabel says where a speed comes from, in a few words: "measured on
// this machine, 5 Nov" or "estimated from llama.cpp's results for this
// chip".
func SourceLabel(c match.Curve) string {
	switch c.Source {
	case match.SourceMeasured:
		if !c.MeasuredAt.IsZero() {
			return "measured on this machine, " + c.MeasuredAt.Format("2 Jan")
		}
		return "measured on this machine"
	case match.SourceMachine:
		return "estimated from models measured on this machine"
	case match.SourcePublished:
		return "estimated from llama.cpp's results for this chip"
	default:
		return "rough estimate from memory bandwidth"
	}
}

// SpeedPoints are the context sizes speeds are shown at: short, the
// reference and the window, without repeats.
func SpeedPoints(o match.Option, reference int) []int {
	points := []int{8 << 10}
	for _, n := range []int{reference, o.MaxContext} {
		if n > points[len(points)-1]*3/2 && n <= o.MaxContext {
			points = append(points, n)
		}
	}
	return points
}

// SpeedLines are a candidate's writing and reading speeds at a few context
// sizes, then where they come from.
func SpeedLines(c match.Candidate, m match.Result) []string {
	o := c.Best
	points := SpeedPoints(o, m.ReferenceContext)
	write := make([]string, 0, len(points))
	read := make([]string, 0, len(points))
	for _, n := range points {
		write = append(write, strconv.Itoa(int(m.SpeedAt(c, n)))+" at "+format.Tokens(n))
		read = append(read, format.Int(int(m.ReadSpeedAt(c, n)))+" at "+format.Tokens(n))
	}
	return []string{
		"writing " + strings.Join(write, " · ") + " tok/s",
		"reading " + strings.Join(read, " · ") + " tok/s",
		SourceLabel(o.Curve),
	}
}
