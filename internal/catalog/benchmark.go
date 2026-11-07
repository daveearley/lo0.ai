package catalog

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Benchmark is a score as the lab published it.
type Benchmark struct {
	// Key names the benchmark across models, one of the curated
	// BenchmarkDefs. It is empty when only one lab reports the benchmark or
	// the card leaves its version unclear; such scores are shown but never
	// compared.
	Key  string `json:"key,omitzero"`
	Name string `json:"name"`
	// Setting is the qualifier the lab ran it with, such as a reasoning
	// effort or a harness.
	Setting string `json:"setting,omitzero"`
	Score   string `json:"score"`
}

// Label is the benchmark's name followed by its setting, if any.
func (b Benchmark) Label() string {
	if b.Setting == "" {
		return b.Name
	}
	return b.Name + " (" + b.Setting + ")"
}

// Value is the score as a number. It reports false for a score that is not
// one.
func (b Benchmark) Value() (float64, bool) {
	v, err := strconv.ParseFloat(b.Score, 64)
	return v, err == nil
}

// BenchmarkDef is a benchmark lo0 recognises under one key, whatever a model
// card calls it. Every benchmark lo0 keys is better when higher.
type BenchmarkDef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Dataset and Task identify the benchmark in Hugging Face eval results,
	// where the generator looks for scores the curated entry lacks.
	Dataset string `json:"dataset,omitzero"`
	Task    string `json:"task,omitzero"`
}

// Benchmark returns the model's first score for key.
func (m Model) Benchmark(key string) (Benchmark, bool) {
	i := slices.IndexFunc(m.Profile.Benchmarks, func(b Benchmark) bool { return b.Key == key })
	if i < 0 {
		return Benchmark{}, false
	}
	return m.Profile.Benchmarks[i], true
}

// SharedBenchmarks returns the keys of the benchmarks at least two of models
// report, the most widely reported first, so a comparison shows only scores
// it can set side by side.
func SharedBenchmarks(models []Model) []string {
	count := map[string]int{}
	var order []string
	for _, m := range models {
		seen := map[string]bool{}
		for _, b := range m.Profile.Benchmarks {
			if b.Key == "" || seen[b.Key] {
				continue
			}
			seen[b.Key] = true
			if count[b.Key] == 0 {
				order = append(order, b.Key)
			}
			count[b.Key]++
		}
	}
	shared := slices.DeleteFunc(order, func(k string) bool { return count[k] < 2 })
	slices.SortStableFunc(shared, func(a, b string) int { return cmp.Compare(count[b], count[a]) })
	return shared
}

// ResolveBenchmarks names each keyed benchmark from its definition and
// writes scores as plain numbers. A key with no definition is an error, so a
// typo in the sources cannot silently drop a score from comparisons.
func ResolveBenchmarks(defs []BenchmarkDef, in []Benchmark) ([]Benchmark, error) {
	out := make([]Benchmark, 0, len(in))
	for _, b := range in {
		b.Score = strings.TrimSpace(strings.TrimSuffix(b.Score, "%"))
		if b.Key != "" {
			i := slices.IndexFunc(defs, func(d BenchmarkDef) bool { return d.Key == b.Key })
			if i < 0 {
				return nil, fmt.Errorf("unknown benchmark key %q", b.Key)
			}
			b.Name = defs[i].Name
		}
		if b.Name == "" || b.Score == "" {
			return nil, fmt.Errorf("benchmark %+v needs a key or name and a score", b)
		}
		out = append(out, b)
	}
	return out, nil
}
