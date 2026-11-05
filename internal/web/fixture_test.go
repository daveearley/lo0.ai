package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/cost"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/claude"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/usage"
)

const testToken = "0123456789abcdef0123456789abcdef"

var (
	testHome = filepath.FromSlash("/Users/dev")
	testNow  = time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)
)

func fixtureCatalog() *catalog.Catalog {
	model := func(id string, tier catalog.Tier, params, active int64, layers int, ollama ...string) catalog.Model {
		return catalog.Model{
			ID: id, Name: id, Family: "test", Repo: "lab/" + id, License: "apache-2.0", Released: "2026-01-01",
			Params: params, ActiveParams: active, ContextLength: 262144, Ollama: ollama,
			Attention: catalog.Attention{FullLayers: layers, FullKV: 1024},
			Quants: []catalog.Quant{
				{Name: "Q8_0", Bytes: params * 85 / 80, BitsPerWeight: 8.5},
				{Name: "Q4_K_M", Bytes: params * 485 / 800, BitsPerWeight: 4.85},
			},
			Tier: tier, TierNote: "Provisional: test tier.",
			Profile: catalog.Profile{
				Summary:    "A " + id + " model built for testing lo0.",
				GoodAt:     []string{"Tool calling in long agent loops"},
				WatchOut:   []string{"Needs its own tool-call parser"},
				Benchmarks: []catalog.Benchmark{{Key: "swe-bench-verified", Name: "SWE-bench Verified", Score: "61.0"}},
				Card:       "https://huggingface.co/lab/" + id,
			},
		}
	}
	return &catalog.Catalog{Generated: "2026-11-04", Models: []catalog.Model{
		model("Coder-30B-A3B", catalog.TierA, 30_500_000_000, 3_400_000_000, 48, "coder:30b"),
		model("General-27B", catalog.TierB, 27_800_000_000, 27_800_000_000, 16),
		model("Giant-480B", catalog.TierA, 480_000_000_000, 35_000_000_000, 62),
	}}
}

func fixtureScan() scan.Result {
	peaks := make([]int, 0, 50)
	for i := range 50 {
		peaks = append(peaks, 15_000+i*8_000)
	}
	records := make([]usage.SessionRecord, len(peaks))
	end := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for i, p := range peaks {
		repo := filepath.Join(testHome, "code", "app")
		if i%3 == 0 {
			repo = filepath.Join(testHome, "code", "site")
		}
		records[i] = usage.SessionRecord{
			Start: end.AddDate(0, 0, (i-len(peaks)+1)*2), PeakContext: p, Repo: repo,
			Usage: []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 40e3, CacheWrite1h: 800e3, CacheRead: 18e6, Output: 120e3}}},
		}
	}
	return scan.Result{
		GeneratedAt: testNow,
		Machine: machine.Profile{
			OS:     machine.OS{GOOS: "darwin", Arch: "arm64", Version: "27.0.1"},
			CPU:    machine.CPU{Model: "Apple M5 Pro", PhysicalCores: 18, LogicalCores: 18},
			Memory: machine.Memory{TotalBytes: 64 << 30, Unified: true},
			GPUs:   []machine.GPU{{Name: "Apple M5 Pro", Vendor: machine.VendorApple, Cores: 20, API: "Metal 4", Unified: true}},
			Disk:   machine.Disk{FreeBytes: 200 << 30, TotalBytes: 1 << 40},
		},
		Runtimes: []runtimes.Runtime{{
			ID: runtimes.Ollama, Name: "Ollama", Installed: true, Running: true, Version: "0.34.4",
			Models: []runtimes.Model{{Name: "coder:30b", Quantization: "Q4_K_M", SizeBytes: 18_500_000_000, ContextLength: 262144}},
		}},
		Setup: setup.Inventory{Account: setup.Account{Plan: setup.PlanMax, Tier: "5x"}},
		Usage: usage.Profile{
			Speeds: []usage.Speed{
				{Harness: "Claude Code", Model: "claude-opus-5-5", Replies: 22_974, Recent: 19_500, TokensPerSecond: 96, ReplyTokens: 565, ReplySeconds: 6.1},
			},
			Sessions: 50, Turns: 4000,
			Context:    usage.Context{P50: peaks[25], P90: peaks[45], Max: peaks[49], Peaks: peaks},
			Tools:      []usage.Tally{{Name: "Bash", Count: 900, Sessions: 50}, {Name: "Edit", Count: 210, Sessions: 30}},
			MCPServers: []usage.Tally{{Name: "github", Count: 40, Sessions: 5}},
			Models:     []usage.Tally{{Name: "claude-opus-5-5", Count: 3900, Sessions: 50}},
			Languages:  []usage.Tally{{Name: "Go", Count: 300, Sessions: 40}, {Name: "Markdown", Count: 100, Sessions: 20}},
			Subagents:  usage.Subagents{Sessions: 10, Share: 0.2, Calls: 40},
			Compaction: usage.Compaction{Sessions: 3, Share: 0.06, Total: 4, Auto: 2},
			First:      time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
			Last:       time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
			Usage:      []usage.ModelUsage{{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 2e6, CacheWrite1h: 40e6, CacheRead: 900e6, Output: 6e6}}},
			Records:    records,
		},
	}
}

// fakeService assesses the fixture, reporting every stage, unless it is
// told to fail or to wait for release.
type fakeService struct {
	err     error
	release chan struct{}
	// started is closed once the first stage is running.
	started chan struct{}
}

func (f fakeService) Assess(ctx context.Context, observe scan.Observer) (app.Assessment, error) {
	for _, st := range scan.Stages {
		observe(scan.Event{Stage: st, Status: scan.StatusRunning})
		if f.started != nil && st == scan.Stages[0] {
			close(f.started)
		}
		if f.release != nil {
			select {
			case <-f.release:
			case <-ctx.Done():
				return app.Assessment{}, ctx.Err()
			}
		}
		observe(scan.Event{Stage: st, Status: scan.StatusDone, Found: scan.Found{Machine: fixtureScan().Machine}})
	}
	if f.err != nil {
		return app.Assessment{}, f.err
	}
	res := fixtureScan()
	m := match.Match(res, fixtureCatalog(), []match.Harness{claude.Harness{}, opencode.Harness{}})
	best, _ := m.Best()
	local := cost.Local{Model: best.Model.Name, Window: best.Best.MaxContext, TokensPerSecond: best.Best.TokensPerSecond}
	return app.Assessment{Scan: res, Match: m, Cost: cost.Estimate(res.Usage, res.Setup.Account, local)}, nil
}

func (fakeService) Inventory(app.Assessment) (app.Inventory, error) {
	return app.Inventory{
		Runtimes: []app.InstalledRuntime{{
			ID: runtimes.Ollama, Name: "Ollama", Installed: true, Running: true, Version: "0.34.4",
			Models: []app.InstalledModel{
				{Name: "coder:30b", Bytes: 18_500_000_000, CatalogID: "Coder-30B-A3B", CatalogName: "Coder-30B-A3B", Quant: "Q4_K_M"},
				{
					Name: "lo0/coder:256k", Parent: "coder:30b", CatalogName: "Coder-30B-A3B", Quant: "Q4_K_M", MadeByLo0: testNow,
					UsedBy: []string{filepath.Join(testHome, ".local", "bin", "claude-local")},
				},
			},
		}},
		DiskBytes: 18_500_000_000,
		FreeBytes: 200 << 30,
		Changes: []migrate.Change{
			{ID: 1, At: testNow, Runtime: runtimes.Ollama, RuntimeName: "Ollama", Model: "lo0/coder:256k"},
			{ID: 2, At: testNow, Path: filepath.Join(testHome, ".local", "bin", "claude-local")},
		},
	}, nil
}

func (fakeService) Harnesses(context.Context, app.Assessment) []app.HarnessView {
	return []app.HarnessView{{
		ID: "claude-code", Name: "Claude Code", Product: "Claude Code", Checked: true,
		Installation: harness.Installation{Installed: true, Version: "2.1.317", Uses: []harness.Use{
			{Command: "claude", Model: "Claude"},
			{Command: "claude-local", Model: "Coder-30B-A3B through Ollama", Local: true},
		}},
	}}
}

// newTestServer returns a server over svc whose first scan has finished.
func newTestServer(t *testing.T, svc Service) *Server {
	t.Helper()
	s, err := New(Options{Service: svc, Token: testToken, Home: testHome, Version: "test", Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
