package report_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/usage"
)

// offloadedMatch matches the catalogue on an 8 GB card with 32 GB of RAM.
func offloadedMatch(t *testing.T) (match.Candidate, match.Result) {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	res := scan.Result{
		Machine: machine.Profile{
			Memory: machine.Memory{TotalBytes: 32 << 30},
			GPUs:   []machine.GPU{{Name: "NVIDIA GeForce RTX 4060", Vendor: machine.VendorNVIDIA, VRAMBytes: 8 << 30}},
		},
		Usage: usage.Profile{Sessions: 3, Context: usage.Context{P50: 40_000, Peaks: []int{20_000, 40_000, 90_000}}},
	}
	m := match.Match(res, cat, nil)
	for _, c := range m.Candidates {
		if c.Best.Offloads() {
			return c, m
		}
	}
	t.Fatal("no candidate keeps experts in system memory on an 8 GB card")
	return match.Candidate{}, match.Result{}
}

func TestModelDetailShowsExpertsInSystemMemory(t *testing.T) {
	c, m := offloadedMatch(t)
	out := report.ModelDetail(c, m, plain(80))
	contains(t, "model detail", out,
		"RAM", "the experts of "+report.OffloadShare(c.Best, c)+" layers",
		"--n-cpu-moe ", "GB/s link to the GPU", "experts in RAM",
	)
	for line := range strings.Lines(out) {
		if n := utf8.RuneCountInString(strings.TrimRight(line, "\n")); n > 80 {
			t.Errorf("line is %d columns wide: %q", n, line)
		}
	}
}

func TestOffloadLinesAreEmptyOnTheGPU(t *testing.T) {
	_, m := sampleMatch()
	best, _ := m.Best()
	if lines := report.OffloadLines(best, m); lines != nil {
		t.Errorf("OffloadLines = %q for a model held on the GPU", lines)
	}
	if out := report.ModelDetail(best, m, plain(80)); strings.Contains(out, "experts in RAM") {
		t.Error("options table has an experts column although no option keeps experts in system memory")
	}
}

func TestVerdictSaysWhereTheExpertsGo(t *testing.T) {
	c, m := offloadedMatch(t)
	if best, _ := m.Best(); best.Model.ID != c.Model.ID {
		t.Skipf("best is %s, which keeps the whole model on the GPU", best.Model.ID)
	}
	reasons := strings.Join(m.Verdict.Reasons, "\n")
	contains(t, "verdict", reasons, "in system memory, which lo0 sets up with llama.cpp", "of its 40 layers")
}
