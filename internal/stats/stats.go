// Package stats has the small numeric helpers the profiles need.
package stats

import (
	"cmp"
	"math"
	"slices"
)

// Percentile returns the nearest-rank p-th percentile of values, where p is
// in [0, 100]. It returns the zero value for an empty slice. values does not
// need to be sorted and is not modified.
func Percentile[T cmp.Ordered](values []T, p float64) T {
	var zero T
	if len(values) == 0 {
		return zero
	}
	sorted := slices.Sorted(slices.Values(values))
	p = min(max(p, 0), 100)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

// Share returns part/whole as a fraction in [0, 1], or 0 when whole is 0.
func Share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
