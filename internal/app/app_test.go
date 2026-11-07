package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

type fakeScanner struct {
	res      scan.Result
	err      error
	observed bool
}

func (f *fakeScanner) Run(_ context.Context, observe scan.Observer) (scan.Result, error) {
	if observe != nil {
		f.observed = true
		observe(scan.Event{Stage: scan.StageMachine, Status: scan.StatusDone})
	}
	return f.res, f.err
}

func harnesses() []app.Harness {
	return []app.Harness{claude.Harness{}, opencode.Harness{}}
}

func result() scan.Result {
	return scan.Result{
		Machine: machine.Profile{
			Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Unified: true}},
		},
		Usage: usage.Profile{Sessions: 2, Context: usage.Context{P50: 20_000, Peaks: []int{20_000, 40_000}}},
	}
}

func TestAssessMatchesTheScan(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	scanner := &fakeScanner{res: result()}
	svc, err := app.New(scanner, cat, harnesses(), app.Migrator{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Assess(t.Context(), func(scan.Event) {})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if !scanner.observed {
		t.Error("observer was not passed to the scanner")
	}
	if a.Scan.Usage.Sessions != 2 || a.Match.Budget.Bytes != 48<<30 || len(a.Match.Candidates) == 0 {
		t.Errorf("assessment = %+v", a.Match)
	}
	if a.Match.Verdict.Level == "" {
		t.Error("assessment has no verdict")
	}
}

func TestAssessReturnsScanErrors(t *testing.T) {
	cat, _ := catalog.Default()
	svc, _ := app.New(&fakeScanner{err: context.Canceled}, cat, harnesses(), app.Migrator{})
	a, err := svc.Assess(t.Context(), nil)
	if !errors.Is(err, context.Canceled) || a.Match.Verdict.Level != "" {
		t.Errorf("Assess = %+v, %v; want the scan error and no match", a.Match, err)
	}
}

func TestModelsIsACopy(t *testing.T) {
	cat := &catalog.Catalog{Models: []catalog.Model{{ID: "a"}}}
	svc, _ := app.New(&fakeScanner{}, cat, harnesses(), app.Migrator{})
	models := svc.Models()
	models[0].ID = "changed"
	if cat.Models[0].ID != "a" {
		t.Error("Models exposed the catalog's backing array")
	}
}

func TestNewRequiresDependencies(t *testing.T) {
	if _, err := app.New(nil, &catalog.Catalog{}, harnesses(), app.Migrator{}); err == nil {
		t.Error("New accepted a nil scanner")
	}
	if _, err := app.New(&fakeScanner{}, nil, harnesses(), app.Migrator{}); err == nil {
		t.Error("New accepted a nil catalog")
	}
}

func TestProjectPaths(t *testing.T) {
	res := scan.Result{
		Usage: usage.Profile{
			Repos:   []usage.Repo{{Path: "/r/busy"}, {Path: "/r/quiet"}},
			Records: []usage.SessionRecord{{Repo: "/r/busy"}, {Repo: "/r/old"}, {}},
		},
		Setup: setup.Inventory{
			Projects:     []string{"/r/quiet", "/r/configured"},
			Instructions: []setup.InstructionFile{{Origin: setup.Origin{Project: "/r/rules"}}, {Origin: setup.Origin{}}},
		},
	}
	got := app.ProjectPaths(res)
	want := []string{"/r/busy", "/r/quiet", "/r/old", "/r/configured", "/r/rules"}
	if !slices.Equal(got, want) {
		t.Errorf("ProjectPaths = %q, want %q", got, want)
	}
}

func TestAssessPricesEachHarness(t *testing.T) {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	opus := []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 1e6, Output: 1e5}}}
	gpt := []usage.ModelUsage{{Model: "gpt-5.2-codex", Tokens: usage.Tokens{Input: 1e6, Output: 1e6}}}
	claudeRecords := []usage.SessionRecord{{Start: day, PeakContext: 20_000, Harness: "Claude Code", Usage: opus}}
	codexRecords := []usage.SessionRecord{
		{Start: day.AddDate(0, -6, 0), PeakContext: 40_000, Harness: "Codex", Usage: gpt},
		{Start: day.AddDate(0, -5, 0), PeakContext: 40_000, Harness: "Codex"},
	}
	res := result()
	res.Usage.Records = slices.Concat(claudeRecords, codexRecords)
	res.Usage.Harnesses = []usage.HarnessProfile{
		{Name: "Claude Code", Profile: usage.Profile{Sessions: 1, First: day, Last: day, Usage: opus, Records: claudeRecords}},
		{Name: "Codex", Profile: usage.Profile{Sessions: 2, First: day.AddDate(0, -6, 0), Plan: "plus", Usage: gpt, Records: codexRecords}},
	}
	cat, _ := catalog.Default()
	svc, _ := app.New(&fakeScanner{res: res}, cat, harnesses(), app.Migrator{})
	a, err := svc.Assess(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := a.Cost.Of("Claude Code")
	if len(c.ByMonth) != 1 || c.Local.SessionShare != 1 {
		t.Errorf("Claude Code by month %+v, local session share %v; want Claude Code's sessions alone", c.ByMonth, c.Local.SessionShare)
	}
	x := a.Cost.Of("Codex")
	if len(x.ByMonth) != 2 || x.TotalUSD != 1.75+14 || x.Plan.Name != "ChatGPT Plus" {
		t.Errorf("Codex by month %+v, $%v on %q; want its own sessions on its ChatGPT plan", x.ByMonth, x.TotalUSD, x.Plan.Name)
	}
	if a.Cost.TotalUSD != c.TotalUSD+x.TotalUSD {
		t.Errorf("total $%v, want both harnesses", a.Cost.TotalUSD)
	}
}

// A profile that was not pooled has no Codex sessions to price.
func TestPriceUnpooledProfileIsClaudeCodes(t *testing.T) {
	res := result()
	res.Usage = usage.Profile{Usage: []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Output: 1e6}}}}
	s := app.Price(res, match.Result{})
	if len(s.Harnesses) != 1 || s.Harnesses[0].Harness != "Claude Code" {
		t.Errorf("harnesses = %+v, want Claude Code alone", s.Harnesses)
	}
}
