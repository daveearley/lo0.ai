package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/cli"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/system/systemtest"
)

// fakeOllama answers the requests a try makes: pulls and creates succeed
// at once and the check gets a reply.
func fakeOllama(t *testing.T) (*httptest.Server, *[]string) {
	var (
		mu    sync.Mutex
		paths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/pull", "/api/create":
			_, _ = w.Write([]byte(`{"status":"success"}` + "\n"))
		case "/v1/messages":
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ready"}]}`))
		case "/api/generate":
			// Report reading as many tokens as the prompt has, at 47 a line.
			var body struct {
				Prompt string `json:"prompt"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			tokens := strings.Count(body.Prompt, "\n") * 47
			_, _ = fmt.Fprintf(w, `{"prompt_eval_count":%d,"prompt_eval_duration":%d,"eval_count":300,"eval_duration":5000000000}`, tokens, tokens*600_000)
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ready"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func trying(files *migratetest.Files, srv *httptest.Server) fakeService {
	res := result()
	res.Runtimes = []runtimes.Runtime{{
		ID: runtimes.Ollama, Name: "Ollama", Installed: true, Service: true, Running: true,
		Version: "0.34.4", Endpoint: srv.URL,
	}}
	runner := &systemtest.Runner{Paths: map[string]string{"claude": "/home/u/.local/bin/claude", "opencode": "/usr/bin/opencode"}}
	res.Runtimes[0].Models = []runtimes.Model{{Name: "qwen3.6:latest", SizeBytes: 23_938_333_577, Quantization: "Q4_K_M"}}
	return fakeService{res: res, migrator: app.Migrator{
		Benchmarks: "/home/u/.local/state/lo0/benchmarks.json",
		Backends:   []migrate.Backend{backend.Ollama{}, backend.LMStudio{}, backend.LlamaCpp{}},
		GOOS:       "linux", Home: "/home/u",
		Getenv:   func(string) string { return "" },
		LookPath: runner.LookPath,
		Effects: migrate.Effects{
			Files: files, Open: files.Open, Runner: runner, Client: srv.Client(),
			Now: migratetest.Clock, Journal: migrate.Journal{Path: journal},
		},
	}}
}

type launched struct {
	program   string
	args, env []string
}

func launcher(code int, got *[]launched) cli.Launch {
	return func(_ context.Context, program string, args, env []string) (int, error) {
		*got = append(*got, launched{program, args, env})
		return code, nil
	}
}

func TestTryDryRunShowsTheSessionAndStartsNothing(t *testing.T) {
	srv, paths := fakeOllama(t)
	files := migratetest.NewFiles(nil)
	h := newHarness(trying(files, srv))
	var got []launched
	h.app.Launch = launcher(0, &got)
	if code := h.app.Run(t.Context(), []string{"try", "--dry-run", "--harness", "claude-code"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	for _, want := range []string{"lo0 try", "Session", "Start claude", "ANTHROPIC_BASE_URL=" + srv.URL, "settings are not changed"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, h.out)
		}
	}
	if len(got) != 0 || len(*paths) != 0 || len(files.Names()) != 0 {
		t.Errorf("a dry run launched %v, called %v and wrote %v", got, *paths, files.Names())
	}
}

func TestTryNeedsATerminal(t *testing.T) {
	srv, _ := fakeOllama(t)
	h := newHarness(trying(migratetest.NewFiles(nil), srv))
	var got []launched
	h.app.Launch = launcher(0, &got)
	if code := h.app.Run(t.Context(), []string{"try", "--yes"}); code != cli.ExitFailure {
		t.Errorf("exit %d, want failure", code)
	}
	if !strings.Contains(h.err.String(), "needs a terminal") || len(got) != 0 {
		t.Errorf("stderr = %q, launched %v", h.err, got)
	}
}

func TestTryPreparesTheModelAndStartsTheHarness(t *testing.T) {
	for _, tt := range []struct {
		harness, program, envVar string
	}{
		{"claude-code", "claude", "ANTHROPIC_BASE_URL"},
		{"opencode", "opencode", "OPENCODE_CONFIG_CONTENT"},
	} {
		t.Run(tt.harness, func(t *testing.T) {
			srv, paths := fakeOllama(t)
			files := migratetest.NewFiles(nil)
			h := newHarness(trying(files, srv))
			h.app.Term = fakeTerminal{tty: true}
			h.app.Env.Environ = []string{"HOME=/home/u", "ANTHROPIC_API_KEY=sk-secret"}
			h.app.Now = migratetest.Clock
			var got []launched
			h.app.Launch = launcher(0, &got)
			args := []string{"try", "--harness", tt.harness, "--yes", "--", "--verbose"}
			if code := h.app.Run(t.Context(), args); code != cli.ExitOK {
				t.Fatalf("exit %d, stderr: %s", code, h.err)
			}
			if len(got) != 1 || got[0].program != tt.program || !slices.Equal(got[0].args, []string{"--verbose"}) {
				t.Fatalf("launched %+v", got)
			}
			env := got[0].env
			if !slices.Contains(env, "HOME=/home/u") || !slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, tt.envVar+"=") }) {
				t.Errorf("env = %q", env)
			}
			if slices.Contains(env, "ANTHROPIC_API_KEY=sk-secret") && tt.harness == "claude-code" {
				t.Error("the user's API key reached a local session")
			}
			for _, want := range []string{"/api/pull", "/api/create"} {
				if !slices.Contains(*paths, want) {
					t.Errorf("Ollama was not asked for %s: %v", want, *paths)
				}
			}
			for _, name := range files.Names() {
				if name != journal {
					t.Errorf("try wrote %s; only the journal of runtime changes should change", name)
				}
			}
			if !strings.Contains(h.out.String(), "Session") || !strings.Contains(h.out.String(), "No requests to") {
				t.Errorf("no session report after the harness exited:\n%s", h.out)
			}
			if !strings.Contains(h.err.String(), "lo0 migrate --to "+tt.harness) {
				t.Errorf("stderr does not say how to keep the setup: %q", h.err)
			}
		})
	}
}

func TestTryAsksBeforeDownloading(t *testing.T) {
	srv, paths := fakeOllama(t)
	h := newHarness(trying(migratetest.NewFiles(nil), srv))
	h.app.Term = fakeTerminal{tty: true}
	h.app.In = strings.NewReader("n\n")
	var got []launched
	h.app.Launch = launcher(0, &got)
	if code := h.app.Run(t.Context(), []string{"try"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if len(got) != 0 || len(*paths) != 0 {
		t.Errorf("declined, yet launched %v and called %v", got, *paths)
	}
	if !strings.Contains(h.err.String(), "Run these steps and start") || !strings.Contains(h.err.String(), "Nothing was changed") {
		t.Errorf("stderr = %q", h.err)
	}
}

func TestTryPassesTheSessionsExitCodeOn(t *testing.T) {
	srv, _ := fakeOllama(t)
	h := newHarness(trying(migratetest.NewFiles(nil), srv))
	h.app.Term = fakeTerminal{tty: true}
	var got []launched
	h.app.Launch = launcher(3, &got)
	if code := h.app.Run(t.Context(), []string{"try", "--yes"}); code != 3 {
		t.Errorf("exit %d, want the session's 3 (stderr %q)", code, h.err)
	}
	if strings.Contains(h.err.String(), "lo0 try:") {
		t.Errorf("a session's exit code was reported as an lo0 error: %q", h.err)
	}
}

func TestTryWaitsForManualSteps(t *testing.T) {
	files := migratetest.NewFiles(nil)
	h := newHarness(migrating(files))
	h.app.Term = fakeTerminal{tty: true}
	var got []launched
	h.app.Launch = launcher(0, &got)
	if code := h.app.Run(t.Context(), []string{"try", "--yes"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if len(got) != 0 {
		t.Errorf("launched %v without Ollama installed", got)
	}
	for _, want := range []string{"Still for you to do", "Install Ollama", "run lo0 try again"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("outcome lacks %q:\n%s", want, h.out)
		}
	}
}

func TestTryJSON(t *testing.T) {
	srv, _ := fakeOllama(t)
	h := newHarness(trying(migratetest.NewFiles(nil), srv))
	if code := h.app.Run(t.Context(), []string{"try", "--json", "--harness", "opencode"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	var got struct {
		Schema string `json:"schema"`
		Result struct {
			Plan migrate.Plan `json:"plan"`
		} `json:"result"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, h.out)
	}
	if got.Schema != cli.TrySchema || got.Result.Plan.Session == nil || got.Result.Plan.Session.Program != "opencode" {
		t.Errorf("JSON = %+v", got)
	}
}

func TestBenchmarkMeasuresTheBestDownloadedModel(t *testing.T) {
	srv, paths := fakeOllama(t)
	files := migratetest.NewFiles(nil)
	h := newHarness(trying(files, srv))
	h.app.Now = migratetest.Clock
	if code := h.app.Run(t.Context(), []string{"benchmark", "--yes"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if n := strings.Count(strings.Join(*paths, " "), "/api/generate"); n < 2 {
		t.Errorf("Ollama was asked to generate %d times, want one per context size: %v", n, *paths)
	}
	if _, ok := files.Get("/home/u/.local/state/lo0/benchmarks.json"); !ok {
		t.Errorf("no measurements kept: %v", files.Names())
	}
	for _, want := range []string{"lo0 benchmark", "Measure qwen3.6:latest at 8k of context", "measured on this machine"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, h.out)
		}
	}
}

func TestBenchmarkWithoutYesAsks(t *testing.T) {
	srv, paths := fakeOllama(t)
	h := newHarness(trying(migratetest.NewFiles(nil), srv))
	if code := h.app.Run(t.Context(), []string{"benchmark"}); code != cli.ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, h.err)
	}
	if len(*paths) != 0 || !strings.Contains(h.err.String(), "lo0 benchmark --model") {
		t.Errorf("measured without asking: %v, stderr %q", *paths, h.err)
	}
}
