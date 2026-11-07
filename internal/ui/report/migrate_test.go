package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/ui/report"
	"lo0.ai/lo0/internal/ui/theme"
)

func tryPlan(t *testing.T) migrate.Plan {
	t.Helper()
	served := migrate.Served{
		Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: "http://127.0.0.1:11434",
		Model: "lo0/qwen3-coder-30b-a3b:64k", DisplayName: "Qwen3-Coder-30B-A3B", Context: 65536, Token: "ollama",
		Background: &migrate.ServedModel{Model: "lo0/qwen3.5-2b:64k", DisplayName: "Qwen3.5-2B", Quant: "Q8_0", Context: 65536},
	}
	look := func(string) (string, error) { return "/Users/u/.local/bin/claude", nil }
	session, _, err := claude.Harness{}.Session(migrate.Request{GOOS: "darwin", Home: "/Users/u", LookPath: look}, served)
	if err != nil {
		t.Fatal(err)
	}
	return migrate.Plan{
		Harness: harness.ClaudeCode, HarnessName: "Claude Code on a local model",
		Runtime: runtimes.Ollama, RuntimeName: "Ollama",
		Model: "Qwen3-Coder-30B-A3B", Quant: "Q4_K_M", Context: 65536, Served: served,
		Steps: []migrate.Step{
			{Kind: migrate.KindDownload, Title: "Download Qwen3-Coder-30B-A3B at Q4_K_M", Done: true},
			{Kind: migrate.KindRuntime, Title: "Create lo0/qwen3-coder-30b-a3b:64k with a 64k context window", Detail: []string{"num_ctx 65,536"}},
			{Kind: migrate.KindDownload, Title: "Download Qwen3.5-2B at Q8_0", Detail: []string{"ollama pull hf.co/unsloth/Qwen3.5-2B-GGUF:Q8_0"}, Bytes: 2_416_199_188},
			{Kind: migrate.KindRuntime, Title: "Create lo0/qwen3.5-2b:64k with a 64k context window", Detail: []string{"num_ctx 65,536"}},
			migrate.Check(served, migrate.Anthropic),
		},
		Session: &session,
		Then:    []string{"To keep Qwen3-Coder-30B-A3B in Claude Code on a local model, run lo0 migrate --to claude-code --model qwen3-coder-30b-a3b --runtime ollama."},
	}
}

func TestRenderTryPlanGolden(t *testing.T) {
	got := filepath.ToSlash(report.RenderPlan(tryPlan(t), report.Options{Width: 80, Theme: theme.Plain(), Home: "/Users/u"}))
	golden := filepath.Join("testdata", "try_80.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden file (run go test -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("try plan differs from %s; run go test ./internal/ui/report -update and review the diff\n\ngot:\n%s", golden, got)
	}
}

func TestRenderPlanSaysWhyThereIsNoBackgroundModel(t *testing.T) {
	p := migrate.Plan{
		HarnessName: "Claude Code on a local model", Model: "Qwen3-Coder-30B-A3B", RuntimeName: "llama.cpp",
		Served: migrate.Served{Model: "lo0/qwen3-coder-30b-a3b:64k", BackgroundNote: "llama-server runs one model per process, so Qwen3-Coder-30B-A3B does background work too."},
	}
	got := report.RenderPlan(p, report.Options{Width: 80, Theme: theme.Plain()})
	want := "  Background    llama-server runs one model per process, so Qwen3-Coder-30B-A3B\n" +
		"                does background work too.\n"
	if !strings.Contains(got, want) {
		t.Errorf("plan lacks the note:\n%s", got)
	}
}

func TestBenchmarkOutcomeShowsWhatWasMeasured(t *testing.T) {
	p := migrate.Plan{
		Command: "benchmark", Model: "Qwen3.6-35B-A3B",
		Steps:    []migrate.Step{{Kind: migrate.KindMeasure, Title: "Measure at 8k of context"}},
		Measured: &bench.Result{Points: []bench.Point{{Context: 8176, ReadTokensPerSecond: 1603.2, WriteTokensPerSecond: 78.8}}},
	}
	out := report.RenderOutcome(p, migrate.Outcome{States: []migrate.State{migrate.StateDone}}, report.Options{Width: 80, Theme: theme.Plain()})
	for _, want := range []string{"Measured", "8k", "1,603 tok/s", "78 tok/s", "is measured from now"} {
		if !strings.Contains(out, want) {
			t.Errorf("outcome missing %q:\n%s", want, out)
		}
	}
}

func TestByHandStatesSharedAdviceOnce(t *testing.T) {
	p := tryPlan(t)
	copyHook := "copy to \"hooks\" in ~/.claude/settings.json"
	p.ByHand = []harness.Feature{
		{Area: harness.AreaHooks, Name: "PreToolUse", From: "Codex", How: copyHook},
		{Area: harness.AreaHooks, Name: "Stop", From: "Codex", How: copyHook},
		{Area: harness.AreaHooks, Name: "SessionStart", How: "rewrite it"},
	}
	got := report.RenderPlan(p, report.Options{Width: 120, Theme: theme.Plain(), Home: "/Users/u"})
	if n := strings.Count(got, copyHook); n != 1 {
		t.Errorf("shared advice appears %d times, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "PreToolUse, Stop from Codex") {
		t.Errorf("hooks with the same advice are not listed together:\n%s", got)
	}
	if !strings.Contains(got, "SessionStart · rewrite it") {
		t.Errorf("hook with other advice is merged:\n%s", got)
	}
}
