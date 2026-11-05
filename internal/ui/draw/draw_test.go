package draw_test

import (
	"math"
	"slices"
	"testing"
	"unicode/utf8"

	"lo0.ai/lo0/internal/ui/draw"
)

func TestBar(t *testing.T) {
	tests := []struct {
		frac          float64
		width         int
		filled, track string
	}{
		{0, 4, "", "░░░░"},
		{1, 4, "████", ""},
		{0.5, 4, "██", "░░"},
		{0.61, 10, "██████▏", "░░░"},
		{1.7, 3, "███", ""},
		{-1, 3, "", "░░░"},
		{math.NaN(), 2, "", "░░"},
		{0.5, 0, "", ""},
	}
	for _, tt := range tests {
		filled, track := draw.Bar(tt.frac, tt.width)
		if filled != tt.filled || track != tt.track {
			t.Errorf("Bar(%v, %d) = %q + %q, want %q + %q", tt.frac, tt.width, filled, track, tt.filled, tt.track)
		}
		if tt.width > 0 {
			if cells := utf8.RuneCountInString(filled + track); cells != tt.width {
				t.Errorf("Bar(%v, %d) is %d cells", tt.frac, tt.width, cells)
			}
		}
	}
}

func TestStacked(t *testing.T) {
	got := draw.Stacked([]float64{17.2, 12.8, 1.5}, 48, 48)
	if !slices.Equal(got, []int{17, 13, 2, 16}) {
		t.Errorf("Stacked = %v", got)
	}
	sum := 0
	for _, n := range draw.Stacked([]float64{1, 1, 1}, 3, 10) {
		sum += n
	}
	if sum != 10 {
		t.Errorf("segments sum to %d, want the full width", sum)
	}
	if got := draw.Stacked([]float64{5}, 0, 6); !slices.Equal(got, []int{0, 6}) {
		t.Errorf("Stacked with no total = %v", got)
	}
	if got := draw.Stacked([]float64{10, 10}, 5, 4); !slices.Equal(got, []int{4, 0, 0}) {
		t.Errorf("overflowing Stacked = %v, want it clamped to the width", got)
	}
}

func TestSparkline(t *testing.T) {
	if got := draw.Sparkline([]int{0, 1, 4, 8, 2}); got != " ▁▄█▂" {
		t.Errorf("Sparkline = %q", got)
	}
	if got := draw.Sparkline([]int{0, 0}); got != "  " {
		t.Errorf("Sparkline of zeros = %q", got)
	}
}

func TestHistogram(t *testing.T) {
	got := draw.Histogram([]int{0, 10, 49, 50, 99, 100, 500}, 2, 100)
	if !slices.Equal(got, []int{3, 4}) {
		t.Errorf("Histogram = %v", got)
	}
	if got := draw.Histogram([]int{1}, 0, 10); len(got) != 0 {
		t.Errorf("Histogram with no buckets = %v", got)
	}
}

func TestPosition(t *testing.T) {
	tests := []struct{ value, limit, width, want int }{
		{0, 100, 10, 0}, {50, 100, 10, 5}, {100, 100, 10, 9}, {500, 100, 10, 9}, {5, 0, 10, 0},
	}
	for _, tt := range tests {
		if got := draw.Position(tt.value, tt.limit, tt.width); got != tt.want {
			t.Errorf("Position(%d, %d, %d) = %d, want %d", tt.value, tt.limit, tt.width, got, tt.want)
		}
	}
}

func BenchmarkBar(b *testing.B) {
	for b.Loop() {
		draw.Bar(0.61, 40)
	}
}

func TestColumns(t *testing.T) {
	got := draw.Columns([]int{0, 1, 4, 8}, 2)
	want := []string{"   █", " ▂██"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Columns = %q, want %q", got, want)
	}
	tiny := draw.Columns([]int{1, 1000}, 1)
	if []rune(tiny[0])[0] != '▁' {
		t.Errorf("Columns = %q, want a non-zero value to show at least an eighth", tiny)
	}
	if draw.Columns([]int{1}, 0) != nil {
		t.Error("Columns with no height should draw nothing")
	}
	half := draw.Columns([]int{4, 8}, 1)
	if half[0] != "▄█" {
		t.Errorf("Columns = %q, want eighth-block tops", half)
	}
}
