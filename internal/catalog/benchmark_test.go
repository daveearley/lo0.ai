package catalog_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
)

var defs = []catalog.BenchmarkDef{
	{Key: "swe-bench-verified", Name: "SWE-bench Verified", Dataset: "SWE-bench/SWE-bench_Verified", Task: "swe_bench_%_resolved"},
	{Key: "terminal-bench-2.0", Name: "Terminal-Bench 2.0", Dataset: "harborframework/terminal-bench-2.0", Task: "terminalbench_2"},
	{Key: "bfcl-v4", Name: "BFCL v4"},
}

func TestResolveBenchmarks(t *testing.T) {
	got, err := catalog.ResolveBenchmarks(defs, []catalog.Benchmark{
		{Key: "swe-bench-verified", Setting: "high", Score: "60.7%"},
		{Name: "MCPMark", Score: "37.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []catalog.Benchmark{
		{Key: "swe-bench-verified", Name: "SWE-bench Verified", Setting: "high", Score: "60.7"},
		{Name: "MCPMark", Score: "37.0"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got[0].Label() != "SWE-bench Verified (high)" || got[1].Label() != "MCPMark" {
		t.Errorf("labels = %q, %q", got[0].Label(), got[1].Label())
	}
}

func TestResolveBenchmarksRejects(t *testing.T) {
	tests := map[string]catalog.Benchmark{
		"unknown key":    {Key: "swe-bench-verfied", Score: "60"},
		"no key or name": {Score: "60"},
		"no score":       {Key: "bfcl-v4"},
	}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := catalog.ResolveBenchmarks(defs, []catalog.Benchmark{b}); err == nil {
				t.Error("want an error")
			}
		})
	}
}

func TestFillFromEvals(t *testing.T) {
	const card = "https://huggingface.co/x/m"
	evals := []catalog.EvalResult{
		{Dataset: "harborframework/terminal-bench-2.0", Task: "terminalbench_2", Value: 36.2, Source: card + "/"},
		{Dataset: "harborframework/terminal-bench-2.0", Task: "terminalbench_2", Value: 36.2, Source: card},
		{Dataset: "SWE-bench/SWE-bench_Verified", Task: "swe_bench_%_resolved", Value: 70.6, Source: "https://arxiv.org/abs/1"},
	}
	t.Run("adds a score only the card's results give", func(t *testing.T) {
		added, conflicts := catalog.FillFromEvals(defs, nil, evals, card)
		want := []catalog.Benchmark{{Key: "terminal-bench-2.0", Name: "Terminal-Bench 2.0", Score: "36.2"}}
		if !slices.Equal(added, want) || len(conflicts) != 0 {
			t.Errorf("added %+v, conflicts %v", added, conflicts)
		}
	})
	t.Run("skips a benchmark with several values", func(t *testing.T) {
		several := append(slices.Clone(evals), catalog.EvalResult{Dataset: "harborframework/terminal-bench-2.0", Task: "terminalbench_2", Value: 41, Source: card})
		if added, _ := catalog.FillFromEvals(defs, nil, several, card); len(added) != 0 {
			t.Errorf("added %+v, want nothing", added)
		}
	})
	t.Run("reports a disagreement and keeps the curated score", func(t *testing.T) {
		have := []catalog.Benchmark{{Key: "terminal-bench-2.0", Name: "Terminal-Bench 2.0", Score: "35"}}
		added, conflicts := catalog.FillFromEvals(defs, have, evals, card)
		if len(added) != 0 || len(conflicts) != 1 || !strings.Contains(conflicts[0], "35") || !strings.Contains(conflicts[0], "36.2") {
			t.Errorf("added %+v, conflicts %v", added, conflicts)
		}
	})
	t.Run("a curated score with a setting is not compared", func(t *testing.T) {
		have := []catalog.Benchmark{{Key: "terminal-bench-2.0", Name: "Terminal-Bench 2.0", Setting: "Max", Score: "35"}}
		if _, conflicts := catalog.FillFromEvals(defs, have, evals, card); len(conflicts) != 0 {
			t.Errorf("conflicts %v", conflicts)
		}
	})
}

func TestSharedBenchmarks(t *testing.T) {
	model := func(keys ...string) catalog.Model {
		var m catalog.Model
		for _, k := range keys {
			m.Profile.Benchmarks = append(m.Profile.Benchmarks, catalog.Benchmark{Key: k, Name: k, Score: "1"})
		}
		return m
	}
	models := []catalog.Model{
		model("bfcl-v4", "swe-bench-verified", "", "swe-bench-verified"),
		model("swe-bench-verified", "terminal-bench-2.0"),
		model("bfcl-v4", "swe-bench-verified"),
	}
	if got, want := catalog.SharedBenchmarks(models), []string{"swe-bench-verified", "bfcl-v4"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if b, ok := models[1].Benchmark("terminal-bench-2.0"); !ok || b.Key != "terminal-bench-2.0" {
		t.Errorf("Benchmark = %+v, %v", b, ok)
	}
	if _, ok := models[1].Benchmark("bfcl-v4"); ok {
		t.Error("found a benchmark the model does not report")
	}
}
