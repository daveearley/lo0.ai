package stats_test

import (
	"slices"
	"testing"

	"lo0.ai/lo0/internal/stats"
)

func TestPercentile(t *testing.T) {
	values := []int{50, 10, 40, 20, 30, 60, 70, 80, 90, 100}
	original := slices.Clone(values)
	tests := []struct {
		p    float64
		want int
	}{
		{0, 10}, {10, 10}, {50, 50}, {90, 90}, {95, 100}, {100, 100}, {150, 100}, {-5, 10},
	}
	for _, tt := range tests {
		if got := stats.Percentile(values, tt.p); got != tt.want {
			t.Errorf("Percentile(p=%v) = %d, want %d", tt.p, got, tt.want)
		}
	}
	if !slices.Equal(values, original) {
		t.Error("Percentile modified its input")
	}
	if got := stats.Percentile([]int(nil), 50); got != 0 {
		t.Errorf("Percentile(empty) = %d, want 0", got)
	}
}

func TestShare(t *testing.T) {
	if got := stats.Share(1, 4); got != 0.25 {
		t.Errorf("Share(1, 4) = %v", got)
	}
	if got := stats.Share(3, 0); got != 0 {
		t.Errorf("Share(3, 0) = %v", got)
	}
}
