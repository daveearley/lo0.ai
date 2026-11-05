package catalog

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// EvalResult is one score from a Hugging Face repository's eval results.
type EvalResult struct {
	Dataset string
	Task    string
	Value   float64
	// Source is the URL the result cites.
	Source string
}

// FillFromEvals finds the keyed benchmarks a curated entry lacks in the
// repository's eval results. A result counts only when it cites the model
// card and is the only value for its benchmark there: anyone can propose eval
// results, and a benchmark run at several settings has no single score to
// take. It also reports where a curated score without a setting disagrees with
// the one the card's result gives, for a person to check.
func FillFromEvals(defs []BenchmarkDef, have []Benchmark, evals []EvalResult, card string) (added []Benchmark, conflicts []string) {
	for _, d := range defs {
		if d.Dataset == "" {
			continue
		}
		var values []float64
		for _, e := range evals {
			if e.Dataset == d.Dataset && e.Task == d.Task && sameURL(e.Source, card) && !slices.Contains(values, e.Value) {
				values = append(values, e.Value)
			}
		}
		if len(values) != 1 {
			continue
		}
		score := strconv.FormatFloat(values[0], 'f', -1, 64)
		i := slices.IndexFunc(have, func(b Benchmark) bool { return b.Key == d.Key })
		if i < 0 {
			added = append(added, Benchmark{Key: d.Key, Name: d.Name, Score: score})
			continue
		}
		if v, ok := have[i].Value(); ok && have[i].Setting == "" && math.Abs(v-values[0]) > 0.05 {
			conflicts = append(conflicts, fmt.Sprintf("%s is %s in the sources and %s in the card's eval results", d.Name, have[i].Score, score))
		}
	}
	return added, conflicts
}

func sameURL(a, b string) bool {
	return a != "" && strings.EqualFold(strings.TrimSuffix(a, "/"), strings.TrimSuffix(b, "/"))
}
