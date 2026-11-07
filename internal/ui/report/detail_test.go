package report_test

import (
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

func plain(width int) report.Options {
	return report.Options{Width: width, Theme: theme.Plain(), Home: home()}
}

func contains(t *testing.T, what, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("%s missing %q:\n%s", what, w, out)
		}
	}
}

func TestModelDetail(t *testing.T) {
	_, m := sampleMatch()
	i := slices.IndexFunc(m.Candidates, func(c match.Candidate) bool { return c.Installed != nil })
	if i < 0 {
		t.Fatal("sample has no installed candidate")
	}
	c := m.Candidates[i]
	out := report.ModelDetail(c, m, plain(70))
	contains(t, "model detail", out,
		c.Model.Name, "Recommended", "Memory", "weights", "KV", "of 48 GiB available",
		"Fits", "Speed", "Installed", "qwen3-coder:30b in Ollama", "Options", "quality",
		"layers cache every token",
	)
}

func TestTooLargeDetail(t *testing.T) {
	_, m := sampleMatch()
	if len(m.TooLarge) == 0 {
		t.Fatal("sample has nothing too large")
	}
	contains(t, "too-large detail", report.TooLargeDetail(m.TooLarge[0], m, plain(70)), "Giant-480B", "Needs", "48 GiB for models")
}

func TestInstalledDetail(t *testing.T) {
	out := report.InstalledDetail(runtimes.Model{Name: "hf.co/someone/tiny:Q4_K_M", Parameters: "4B", Quantization: "Q4_K_M", SizeBytes: 2 << 30, ContextLength: 32768}, plain(70))
	contains(t, "installed detail", out, "tiny:Q4_K_M", "4B parameters", "Q4_K_M · 2 GiB", "32k tokens", "no tool calling", "Not in the lo0 catalogue")
}

func TestFeatureDetail(t *testing.T) {
	f := harness.Feature{Area: harness.AreaAppTools, Name: "Claude_Browser", Status: harness.StatusLost, How: "provided by the Claude app", Calls: 4741}
	contains(t, "feature detail", report.FeatureDetail(f, "OpenCode", plain(70)), "Claude_Browser", "App tools", "In OpenCode", "lost", "provided by the Claude app", "4,741 calls")
}

func TestStatusLabels(t *testing.T) {
	for status, want := range map[harness.Status]string{
		harness.StatusNative: "works as is", harness.StatusTranslate: "converted", harness.StatusPartial: "partly", harness.StatusLost: "lost",
	} {
		if got := report.StatusLabel(status, plain(80)); got != want {
			t.Errorf("StatusLabel(%s) = %q, want %q", status, got, want)
		}
	}
}

func TestRenderSection(t *testing.T) {
	res, m := sampleMatch()
	sections := map[report.Section]string{
		report.SectionVerdict:   "Verdict",
		report.SectionCarryover: "Carries over to Claude Code on a local model",
		report.SectionMachine:   "Machine",
		report.SectionSetup:     "Claude Code setup",
		report.SectionHistory:   "Session history",
		report.SectionWarnings:  "1 warning",
	}
	for s, heading := range sections {
		out := report.RenderSection(s, app.Assessment{Scan: res, Match: m}, plain(80))
		if !strings.HasPrefix(out, heading) {
			t.Errorf("section %d starts %q, want %q", s, firstLine(out), heading)
		}
		if strings.Contains(out, "lo0 match") || strings.Contains(out, "lo0 scan") {
			t.Errorf("section %d includes the report header", s)
		}
	}
	res.Warnings = nil
	if out := report.RenderSection(report.SectionWarnings, app.Assessment{Scan: res, Match: m}, plain(80)); out != "" {
		t.Errorf("empty warnings section = %q, want nothing", out)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func TestProseDoesNotBreakAtHyphens(t *testing.T) {
	res, m := sampleMatch()
	m.Verdict.Headline = "A local model can handle many of your sessions, with trade-offs and Qwen3-Coder-30B-A3B."
	for width := 40; width <= 90; width++ {
		out := report.RenderSection(report.SectionVerdict, app.Assessment{Scan: res, Match: m}, plain(width))
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, "-") && !strings.HasSuffix(trimmed, " -") {
				t.Fatalf("width %d: line ends in a hyphen: %q", width, line)
			}
		}
	}
}
