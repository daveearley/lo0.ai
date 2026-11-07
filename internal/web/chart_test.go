package web

import (
	"strings"
	"testing"
)

func TestContextCurve(t *testing.T) {
	peaks := []int{10_000, 50_000, 100_000, 200_000, 300_000, 600_000, 900_000, 1_000_000}
	for _, tc := range []struct {
		name      string
		peaks     []int
		window    int
		ok        bool
		split     float64
		fit, over bool
	}{
		{"split at the window", peaks, 256 << 10, true, 0.5, true, true},
		{"everything fits", peaks, 1 << 20, true, 1, true, false},
		{"nothing fits", peaks, 4 << 10, true, 0, false, true},
		{"no sessions", nil, 256 << 10, false, 0, false, false},
		{"no window", peaks, 0, false, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, ok := contextCurve(tc.peaks, tc.window)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if c.Split != tc.split {
				t.Errorf("split = %v, want %v", c.Split, tc.split)
			}
			if (c.Fit != "") != tc.fit || (c.Over != "") != tc.over {
				t.Errorf("fit %q, over %q", c.Fit, c.Over)
			}
			if c.Line < 0 || c.Line > 1 {
				t.Errorf("line = %v, outside the chart", c.Line)
			}
			for _, p := range []string{c.Fit, c.Over} {
				if p != "" && (!strings.HasPrefix(p, "M") || !strings.HasSuffix(p, "Z")) {
					t.Errorf("area %q is not a closed path", p)
				}
			}
		})
	}
}

func TestContextCurveIsBounded(t *testing.T) {
	peaks := make([]int, 5_000)
	for i := range peaks {
		peaks[i] = 1_000 + i*200
	}
	c, ok := contextCurve(peaks, 256<<10)
	if !ok {
		t.Fatal("no curve")
	}
	if n := strings.Count(c.Fit+c.Over, "L"); n > 2*chartCols+8 {
		t.Errorf("paths have %d points for %d sessions; they should be capped near %d", n, len(peaks), chartCols)
	}
}

func BenchmarkContextCurve(b *testing.B) {
	peaks := make([]int, 5_000)
	for i := range peaks {
		peaks[i] = 1_000 + i*200
	}
	for b.Loop() {
		contextCurve(peaks, 256<<10)
	}
}
