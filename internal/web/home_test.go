package web

import (
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/runtimes"
)

func TestStepCommands(t *testing.T) {
	a := app.Assessment{Match: match.Result{
		TargetID: "claude-code",
		Candidates: []match.Candidate{
			{Model: catalog.Model{ID: "best"}, Installed: &match.Installed{Runtime: runtimes.Ollama}},
			{Model: catalog.Model{ID: "other"}},
		},
	}}
	for _, tc := range []struct {
		name string
		step app.NextStep
		cmd  string
		href string
	}{
		{"defaults are left out", app.NextStep{Kind: app.StepTry, Harness: "claude-code", Model: "best", Runtime: runtimes.Ollama}, "lo0 try", ""},
		{"another harness", app.NextStep{Kind: app.StepTry, Harness: "opencode", Model: "best"}, "lo0 try --harness opencode", ""},
		{"another model", app.NextStep{Kind: app.StepTry, Model: "other"}, "lo0 try --model other", ""},
		{"another runtime", app.NextStep{Kind: app.StepTry, Model: "best", Runtime: runtimes.LMStudio}, "lo0 try --runtime lmstudio", ""},
		{"keep uses --to", app.NextStep{Kind: app.StepKeep, Harness: "opencode", Model: "best"}, "lo0 migrate --to opencode", ""},
		{"downloads go to the interface", app.NextStep{Kind: app.StepDownload, Model: "other"}, "lo0 ui", ""},
		{"compare opens the model", app.NextStep{Kind: app.StepCompare, Model: "other"}, "", "/models/other"},
		{"browse opens every model", app.NextStep{Kind: app.StepBrowse}, "", "/models?show=all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := step(tc.step, a)
			if got.Command != tc.cmd || got.Href != tc.href {
				t.Errorf("command %q, href %q; want %q, %q", got.Command, got.Href, tc.cmd, tc.href)
			}
		})
	}
}

func TestPace(t *testing.T) {
	for _, tc := range []struct {
		name       string
		p          match.Pace
		text, tone string
	}{
		{"very slow", match.Pace{Band: match.BandVerySlow, Slower: 13}, "very slow", "bad"},
		{"slower", match.Pace{Band: match.BandSlower, Slower: 2.9}, "2.9× slower", "warn"},
		{"slower without a reference", match.Pace{Band: match.BandSlower}, "slow", "warn"},
		{"comparable", match.Pace{Band: match.BandComparable, Slower: 1.05}, "about as fast", "good"},
		{"comparable but measurably slower", match.Pace{Band: match.BandComparable, Slower: 1.6}, "1.6× slower", "good"},
		{"nothing to compare", match.Pace{Band: match.BandComparable}, "", "good"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, tone := pace(tc.p)
			if text != tc.text || tone != tc.tone {
				t.Errorf("pace = %q %q, want %q %q", text, tone, tc.text, tc.tone)
			}
		})
	}
}
