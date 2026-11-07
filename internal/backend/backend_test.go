package backend_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/backend"
	"lo0.ai/lo0/internal/bench"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/system/systemtest"
)

func coder() catalog.Model {
	return catalog.Model{
		ID: "qwen3-coder-30b-a3b", Name: "Qwen3-Coder-30B-A3B", GGUF: "unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF",
		Ollama: []string{"qwen3-coder:30b"},
		Quants: []catalog.Quant{{Name: "Q8_0", Bytes: 32 << 30}, {Name: "Q4_K_M", Bytes: 18 << 30}},
	}
}

func request(rt runtimes.Runtime) migrate.Request {
	m := coder()
	return migrate.Request{Model: m, Quant: m.Quants[1], Context: 64 << 10, Runtime: rt}
}

func titles(steps []migrate.Step) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Title
		if s.Done {
			out[i] += " (done)"
		}
	}
	return out
}

func TestLocalName(t *testing.T) {
	if got := backend.LocalName(coder(), 64<<10); got != "lo0/qwen3-coder-30b-a3b:64k" {
		t.Errorf("LocalName = %q", got)
	}
}

func TestOllamaPlansPullAndCreate(t *testing.T) {
	rt := runtimes.Runtime{ID: runtimes.Ollama, Installed: true, Running: true, Version: "0.21.0", Endpoint: "http://127.0.0.1:11434"}
	served, steps, err := backend.Ollama{}.Prepare(request(rt), migrate.Anthropic)
	if err != nil {
		t.Fatal(err)
	}
	if served.BaseURL != rt.Endpoint || served.Model != "lo0/qwen3-coder-30b-a3b:64k" || served.Token == "" || served.Context != 64<<10 {
		t.Errorf("served = %+v", served)
	}
	want := []string{"Download Qwen3-Coder-30B-A3B at Q4_K_M", "Create lo0/qwen3-coder-30b-a3b:64k with a 64k context window"}
	if !slices.Equal(titles(steps), want) {
		t.Errorf("steps = %q, want %q", titles(steps), want)
	}
	if !slices.Contains(steps[0].Detail, "ollama pull hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M") || steps[0].Bytes != 18<<30 {
		t.Errorf("pull = %+v", steps[0])
	}
}

func TestOllamaReusesWhatIsInstalled(t *testing.T) {
	rt := runtimes.Runtime{ID: runtimes.Ollama, Installed: true, Running: true, Models: []runtimes.Model{
		{Name: "hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"}, {Name: "lo0/qwen3-coder-30b-a3b:64k"},
	}}
	req := request(rt)
	req.Installed = "hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"
	_, steps, err := backend.Ollama{}.Prepare(req, migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if !s.Done {
			t.Errorf("step %q is not done although Ollama has everything", s.Title)
		}
	}
}

func TestOllamaUsesItsLibraryForOwnFormats(t *testing.T) {
	m := catalog.Model{ID: "gpt-oss-20b", Name: "gpt-oss-20b", GGUF: "ggml-org/gpt-oss-20b-GGUF", Ollama: []string{"gpt-oss:20b"}, Quants: []catalog.Quant{{Name: "MXFP4"}}}
	_, steps, err := backend.Ollama{}.Prepare(migrate.Request{Model: m, Quant: m.Quants[0], Context: 128 << 10, Runtime: runtimes.Runtime{Installed: true, Running: true}}, migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(steps[0].Detail, "ollama pull gpt-oss:20b") {
		t.Errorf("pull detail = %q", steps[0].Detail)
	}
}

func TestOllamaAsksForWhatOnlyTheUserCanDo(t *testing.T) {
	tests := []struct {
		name string
		rt   runtimes.Runtime
		p    migrate.Protocol
		want string
	}{
		{"not installed", runtimes.Runtime{}, migrate.Anthropic, "Install Ollama"},
		{"stopped", runtimes.Runtime{Installed: true}, migrate.Anthropic, "Start Ollama"},
		{"too old for Claude Code", runtimes.Runtime{Installed: true, Running: true, Version: "0.13.5"}, migrate.Anthropic, "Update Ollama"},
		{"too old for Codex", runtimes.Runtime{Installed: true, Running: true, Version: "0.13.3"}, migrate.Responses, "Update Ollama"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, steps, err := backend.Ollama{}.Prepare(request(tt.rt), tt.p)
			if err != nil {
				t.Fatal(err)
			}
			if steps[0].Kind != migrate.KindManual || steps[0].Title != tt.want {
				t.Errorf("first step = %s %q, want manual %q", steps[0].Kind, steps[0].Title, tt.want)
			}
		})
	}
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Version: "0.13.5"}), migrate.OpenAI)
	if steps[0].Kind == migrate.KindManual {
		t.Error("an old Ollama is fine for OpenCode, which does not need the Anthropic API")
	}
	_, steps, _ = backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Version: "0.13.4"}), migrate.Responses)
	if steps[0].Kind == migrate.KindManual {
		t.Error("Ollama 0.13.4 serves the Responses API Codex needs")
	}
}

func TestOllamaNeedsADownload(t *testing.T) {
	m := catalog.Model{ID: "x", Name: "X", Quants: []catalog.Quant{{Name: "Q4_K_M"}, {Name: "Q8_0"}}}
	if _, _, err := (backend.Ollama{}).Prepare(migrate.Request{Model: m, Quant: m.Quants[0]}, migrate.OpenAI); err == nil {
		t.Error("a model with no download source was planned")
	}
}

func TestOllamaPullAndCreateStream(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		data, _ := json.Marshal(body)
		bodies = append(bodies, r.URL.Path+" "+string(data))
		switch r.URL.Path {
		case "/api/show":
			http.NotFound(w, r)
		case "/api/pull":
			fmt.Fprintln(w, `{"status":"pulling manifest"}`)
			// Two layers, the second named only once the first is under way.
			fmt.Fprintln(w, `{"status":"pulling abc","digest":"abc","total":15000000000,"completed":6000000000}`)
			fmt.Fprintln(w, `{"status":"pulling def","digest":"def","total":5000000000,"completed":1000000000}`)
			fmt.Fprintln(w, `{"status":"success"}`)
		case "/api/create":
			fmt.Fprintln(w, `{"status":"writing manifest"}`)
			fmt.Fprintln(w, `{"status":"success"}`)
		}
	}))
	t.Cleanup(srv.Close)
	rt := runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}
	_, steps, err := backend.Ollama{}.Prepare(request(rt), migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	fx := migrate.Effects{Client: srv.Client()}
	var progress [][2]int64
	for _, s := range steps {
		if err := s.Run(t.Context(), fx, func(done, total int64) { progress = append(progress, [2]int64{done, total}) }); err != nil {
			t.Fatalf("%s: %v", s.Title, err)
		}
	}
	want := []string{
		`/api/show {"model":"hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"}`,
		`/api/pull {"model":"hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"}`,
		`/api/create {"from":"hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M","model":"lo0/qwen3-coder-30b-a3b:64k","parameters":{"num_ctx":65536}}`,
	}
	if !slices.Equal(bodies, want) {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(bodies, "\n"), strings.Join(want, "\n"))
	}
	// Until every layer is known the planned size, 19,327,352,832 bytes, is
	// the total; after that it is the sum of the layers.
	if !slices.Equal(progress, [][2]int64{{6_000_000_000, 19_327_352_832}, {7_000_000_000, 20_000_000_000}}) {
		t.Errorf("progress = %v", progress)
	}
}

func TestOllamaStreamErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, `{"status":"pulling manifest"}`)
		fmt.Fprintln(w, `{"error":"pull model manifest: file does not exist"}`)
	}))
	t.Cleanup(srv.Close)
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}), migrate.OpenAI)
	err := steps[0].Run(t.Context(), migrate.Effects{Client: srv.Client()}, func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "file does not exist") {
		t.Errorf("err = %v, want Ollama's error", err)
	}
}

func TestLMStudioLoadsADownloadedModel(t *testing.T) {
	rt := runtimes.Runtime{ID: runtimes.LMStudio, Installed: true, Running: true, Models: []runtimes.Model{
		{Name: "qwen3-coder-30b-a3b-instruct", Quantization: "Q4_K_M"},
	}}
	req := request(rt)
	req.LookPath = (&systemtest.Runner{Paths: map[string]string{"lms": "/usr/local/bin/lms"}}).LookPath
	served, steps, err := backend.LMStudio{}.Prepare(req, migrate.Anthropic)
	if err != nil {
		t.Fatal(err)
	}
	if served.BaseURL != runtimes.DefaultLMStudioURL || served.Model != "lo0-qwen3-coder-30b-a3b-64k" {
		t.Errorf("served = %+v", served)
	}
	want := []string{"Download Qwen3-Coder-30B-A3B at Q4_K_M in LM Studio (done)", "Load it as lo0-qwen3-coder-30b-a3b-64k with a 64k context window"}
	if !slices.Equal(titles(steps), want) {
		t.Fatalf("steps = %q, want %q", titles(steps), want)
	}
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{
		"lms load qwen3-coder-30b-a3b-instruct --context-length 65536 --identifier lo0-qwen3-coder-30b-a3b-64k": {},
	}}
	if err := steps[1].Run(t.Context(), migrate.Effects{Runner: runner}, func(int64, int64) {}); err != nil {
		t.Errorf("load: %v (calls %q)", err, runner.Calls())
	}
}

func TestLMStudioLeavesTheDownloadToTheUser(t *testing.T) {
	_, steps, err := backend.LMStudio{}.Prepare(request(runtimes.Runtime{ID: runtimes.LMStudio}), migrate.Anthropic)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if s.Kind != migrate.KindManual || s.Done {
			t.Errorf("step %q is %s, want manual and pending without LM Studio or the model", s.Title, s.Kind)
		}
	}
	if steps[0].Title != "Install LM Studio" {
		t.Errorf("first step = %q", steps[0].Title)
	}
}

func TestLlamaCppGivesTheServerCommandAndChecksIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"other","aliases":["lo0/qwen3-coder-30b-a3b:64k"]}]}`))
	}))
	t.Cleanup(srv.Close)
	_, steps, err := backend.LlamaCpp{}.Prepare(request(runtimes.Runtime{ID: runtimes.LlamaCpp, Installed: true}), migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != migrate.KindManual || steps[0].Run == nil {
		t.Fatalf("steps = %+v, want one manual step that can check itself", steps)
	}
	cmd := []string{
		`Run: llama-server \`,
		`  -hf unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M \`,
		`  --jinja -c 65536 --port 8080 \`,
		`  --alias lo0/qwen3-coder-30b-a3b:64k`,
	}
	if got := steps[0].Detail[:len(cmd)]; !slices.Equal(got, cmd) {
		t.Errorf("command = %q\nwant      %q", got, cmd)
	}
	win := request(runtimes.Runtime{ID: runtimes.LlamaCpp, Installed: true})
	win.GOOS = "windows"
	if _, w, _ := (backend.LlamaCpp{}).Prepare(win, migrate.OpenAI); w[0].Detail[0] != "Run: llama-server `" {
		t.Errorf("Windows command starts %q, want PowerShell's line continuation", w[0].Detail[0])
	}

	_, live, err := backend.LlamaCpp{BaseURL: srv.URL}.Prepare(request(runtimes.Runtime{ID: runtimes.LlamaCpp, Installed: true}), migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if err := live[0].Run(t.Context(), migrate.Effects{Client: srv.Client()}, func(int64, int64) {}); err != nil {
		t.Errorf("check with the server running: %v", err)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"someone-elses-model"}]}`))
	}))
	t.Cleanup(other.Close)
	_, wrong, _ := backend.LlamaCpp{BaseURL: other.URL}.Prepare(request(runtimes.Runtime{ID: runtimes.LlamaCpp, Installed: true}), migrate.OpenAI)
	if err := wrong[0].Run(t.Context(), migrate.Effects{Client: other.Client()}, func(int64, int64) {}); err == nil {
		t.Error("check passed although the server serves a different model")
	}
}

func TestLlamaCppKeepsExpertsInSystemMemory(t *testing.T) {
	req := request(runtimes.Runtime{ID: runtimes.LlamaCpp, Installed: true})
	req.ExpertLayers = 37
	_, steps, err := backend.LlamaCpp{}.Prepare(req, migrate.OpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(steps[0].Detail, "  -ngl 99 --n-cpu-moe 37") {
		t.Errorf("command = %q, want every layer on the GPU and the first 37 layers' experts in system memory", steps[0].Detail)
	}
	if !slices.Contains(steps[0].Detail, "--n-cpu-moe 37 keeps the first 37 layers' experts in system memory.") {
		t.Errorf("detail = %q, want it to say what --n-cpu-moe does", steps[0].Detail)
	}
	if !migrate.OffloadsExperts(backend.LlamaCpp{}) {
		t.Error("llama.cpp does not report that it can keep experts in system memory")
	}
	for _, b := range []migrate.Backend{backend.Ollama{}, backend.LMStudio{}} {
		if migrate.OffloadsExperts(b) {
			t.Errorf("%s claims to keep experts in system memory", b.ID())
		}
	}
}

func TestOllamaRecordsAndRevertsWhatItAdds(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			http.NotFound(w, r)
		case "/api/pull", "/api/create":
			fmt.Fprintln(w, `{"status":"success"}`)
		case "/api/delete":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != http.MethodDelete {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			deleted = append(deleted, body.Model)
			if body.Model == "gone" {
				http.Error(w, "not found", http.StatusNotFound)
			}
		}
	}))
	t.Cleanup(srv.Close)
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Client: srv.Client(), Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}), migrate.OpenAI)
	for _, s := range steps {
		if err := s.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	changes, _ := fx.Journal.Load(files)
	if len(changes) != 2 || changes[0].Model != "hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M" || changes[0].Bytes != 18<<30 ||
		changes[1].Model != "lo0/qwen3-coder-30b-a3b:64k" || changes[1].BaseURL != srv.URL {
		t.Fatalf("journal = %+v", changes)
	}
	for _, c := range append(changes, migrate.Change{Model: "gone", BaseURL: srv.URL}) {
		if err := (backend.Ollama{}).Revert(t.Context(), fx, c); err != nil {
			t.Errorf("revert %s: %v", c.Model, err)
		}
	}
	if len(deleted) != 3 {
		t.Errorf("deleted = %v", deleted)
	}
}

// A model the scan missed, or the user pulled after planning, is theirs: lo0
// neither pulls it nor records it, so undo never deletes it.
func TestOllamaLeavesAModelTheUserAlreadyHas(t *testing.T) {
	var pulled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			fmt.Fprintln(w, `{}`)
		case "/api/pull":
			pulled = true
			fmt.Fprintln(w, `{"status":"success"}`)
		}
	}))
	t.Cleanup(srv.Close)
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Client: srv.Client(), Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}), migrate.OpenAI)
	if err := steps[0].Run(t.Context(), fx, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if changes, _ := fx.Journal.Load(files); pulled || len(changes) != 0 {
		t.Errorf("pulled = %v, journal = %+v; want neither", pulled, changes)
	}
}

// When Ollama cannot say whether it has the model, the model may be the
// user's: lo0 pulls it, which Ollama skips for a model it holds, but never
// records it, so undo cannot delete it.
func TestOllamaDoesNotRecordAPullAfterAFailedCheck(t *testing.T) {
	var pulled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			http.Error(w, "busy", http.StatusServiceUnavailable)
		case "/api/pull":
			pulled = true
			fmt.Fprintln(w, `{"status":"success"}`)
		}
	}))
	t.Cleanup(srv.Close)
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Client: srv.Client(), Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}), migrate.OpenAI)
	if err := steps[0].Run(t.Context(), fx, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if changes, _ := fx.Journal.Load(files); !pulled || len(changes) != 0 {
		t.Errorf("pulled = %v, journal = %+v; want a pull and no record", pulled, changes)
	}
}

// A pull that fails part way is already recorded, so undo clears what it
// left behind.
func TestOllamaRecordsAPullBeforeItRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			http.NotFound(w, r)
		case "/api/pull":
			fmt.Fprintln(w, `{"error":"no space left on device"}`)
		}
	}))
	t.Cleanup(srv.Close)
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Client: srv.Client(), Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	_, steps, _ := backend.Ollama{}.Prepare(request(runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL}), migrate.OpenAI)
	if err := steps[0].Run(t.Context(), fx, func(int64, int64) {}); err == nil {
		t.Fatal("a failed pull reported success")
	}
	if changes, _ := fx.Journal.Load(files); len(changes) != 1 {
		t.Errorf("journal = %+v, want the pull recorded", changes)
	}
}

func TestLMStudioUnloadsWhatItLoaded(t *testing.T) {
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{"lms unload lo0-m": {}}}
	if err := (backend.LMStudio{}).Revert(t.Context(), migrate.Effects{Runner: runner}, migrate.Change{Model: "lo0-m"}); err != nil {
		t.Errorf("revert: %v (calls %q)", err, runner.Calls())
	}
	// LM Studio unloads everything when it quits, so a model that is no
	// longer loaded must not block the rest of the undo.
	gone := &systemtest.Runner{}
	if err := (backend.LMStudio{}).Revert(t.Context(), migrate.Effects{Runner: gone}, migrate.Change{Model: "lo0-m"}); err != nil {
		t.Errorf("revert of an unloaded model: %v", err)
	}
}

func TestOllamaRemovesAModelAndForgetsMakingIt(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/api/delete" && r.Method == http.MethodDelete {
			deleted = append(deleted, body.Model)
		}
	}))
	t.Cleanup(srv.Close)
	const journal = "/state/migrations.json"
	files := migratetest.NewFiles(map[string]string{journal: `{"changes":[{"id":1,"runtime":"ollama","model":"lo0/m:64k"},{"id":2,"path":"/x"}]}`})
	fx := migrate.Effects{Files: files, Client: srv.Client(), Journal: migrate.Journal{Path: journal}}
	step := backend.Ollama{}.Remove(runtimes.Runtime{Endpoint: srv.URL}, "lo0/m:64k")
	if step.Kind != migrate.KindRuntime || step.Detail[0] != "ollama rm lo0/m:64k" {
		t.Errorf("step = %+v", step)
	}
	if err := step.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	left, _ := fx.Journal.Load(files)
	if len(deleted) != 1 || deleted[0] != "lo0/m:64k" || len(left) != 1 || left[0].Path != "/x" {
		t.Errorf("deleted %v, journal %+v; want the model gone and its record forgotten", deleted, left)
	}
}

func withBackground(req migrate.Request) migrate.Request {
	m := catalog.Model{ID: "qwen3.5-2b", Name: "Qwen3.5-2B", GGUF: "unsloth/Qwen3.5-2B-GGUF", Quants: []catalog.Quant{{Name: "Q8_0", Bytes: 2 << 30}}}
	req.Background = &migrate.Background{Model: m, Quant: m.Quants[0], Context: 64 << 10}
	return req
}

func TestOllamaPreparesTheBackgroundModelToo(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			http.NotFound(w, r)
		case "/api/pull", "/api/create":
			fmt.Fprintln(w, `{"status":"success"}`)
		case "/api/delete":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			deleted = append(deleted, body.Model)
		}
	}))
	t.Cleanup(srv.Close)
	rt := runtimes.Runtime{Installed: true, Running: true, Endpoint: srv.URL, Models: []runtimes.Model{{Name: "hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"}}}
	req := withBackground(request(rt))
	req.Installed = "hf.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M"
	served, steps, err := backend.Ollama{}.Prepare(req, migrate.Anthropic)
	if err != nil {
		t.Fatal(err)
	}
	want := migrate.ServedModel{Model: "lo0/qwen3.5-2b:64k", DisplayName: "Qwen3.5-2B", Quant: "Q8_0", Context: 64 << 10}
	if served.Background == nil || *served.Background != want || served.BackgroundModel() != want.Model || served.BackgroundNote != "" {
		t.Errorf("served background = %+v, note %q", served.Background, served.BackgroundNote)
	}
	titled := []string{
		"Qwen3-Coder-30B-A3B at Q4_K_M is already in Ollama (done)",
		"Create lo0/qwen3-coder-30b-a3b:64k with a 64k context window",
		"Download Qwen3.5-2B at Q8_0",
		"Create lo0/qwen3.5-2b:64k with a 64k context window",
	}
	if !slices.Equal(titles(steps), titled) {
		t.Fatalf("steps = %q, want %q", titles(steps), titled)
	}
	if steps[2].Bytes != 2<<30 || !slices.Contains(steps[2].Detail, "ollama pull hf.co/unsloth/Qwen3.5-2B-GGUF:Q8_0") {
		t.Errorf("background pull = %+v", steps[2])
	}

	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Client: srv.Client(), Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	for _, s := range steps[1:] {
		if err := s.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	undo, err := migrate.PlanUndo(fx.Journal, files, []migrate.Backend{backend.Ollama{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range undo.Steps {
		if err := s.Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	gone := []string{"lo0/qwen3.5-2b:64k", "hf.co/unsloth/Qwen3.5-2B-GGUF:Q8_0", "lo0/qwen3-coder-30b-a3b:64k"}
	if !slices.Equal(deleted, gone) || undo.Steps[1].Bytes != 2<<30 {
		t.Errorf("undo deleted %q, want %q, the download freeing its size", deleted, gone)
	}
}

func TestLMStudioLoadsTheBackgroundModelToo(t *testing.T) {
	rt := runtimes.Runtime{ID: runtimes.LMStudio, Installed: true, Running: true, Models: []runtimes.Model{
		{Name: "qwen3-coder-30b-a3b-instruct", Quantization: "Q4_K_M"}, {Name: "qwen3.5-2b", Quantization: "Q8_0"},
	}}
	req := withBackground(request(rt))
	req.LookPath = (&systemtest.Runner{Paths: map[string]string{"lms": "/usr/local/bin/lms"}}).LookPath
	served, steps, err := backend.LMStudio{}.Prepare(req, migrate.Anthropic)
	if err != nil {
		t.Fatal(err)
	}
	if served.Background == nil || served.Background.Model != "lo0-qwen3.5-2b-64k" {
		t.Errorf("served background = %+v", served.Background)
	}
	want := []string{
		"Download Qwen3-Coder-30B-A3B at Q4_K_M in LM Studio (done)", "Load it as lo0-qwen3-coder-30b-a3b-64k with a 64k context window",
		"Download Qwen3.5-2B at Q8_0 in LM Studio (done)", "Load it as lo0-qwen3.5-2b-64k with a 64k context window",
	}
	if !slices.Equal(titles(steps), want) {
		t.Fatalf("steps = %q, want %q", titles(steps), want)
	}
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{
		"lms load qwen3.5-2b --context-length 65536 --identifier lo0-qwen3.5-2b-64k": {},
	}}
	files := migratetest.NewFiles(nil)
	fx := migrate.Effects{Files: files, Runner: runner, Now: migratetest.Clock, Journal: migrate.Journal{Path: "/state/migrations.json"}}
	if err := steps[3].Run(t.Context(), fx, func(int64, int64) {}); err != nil {
		t.Fatalf("load: %v (calls %q)", err, runner.Calls())
	}
	if changes, _ := fx.Journal.Load(files); len(changes) != 1 || changes[0].Model != "lo0-qwen3.5-2b-64k" {
		t.Errorf("journal = %+v, want the load recorded for undo", changes)
	}
}

func TestBackgroundModelsARuntimeCannotServeAreNoted(t *testing.T) {
	noGGUF := withBackground(request(runtimes.Runtime{Installed: true, Running: true}))
	noGGUF.Background.Model.GGUF = ""
	tests := []struct {
		name string
		b    migrate.Backend
		req  migrate.Request
		note string
	}{
		{"llama-server runs one model", backend.LlamaCpp{}, withBackground(request(runtimes.Runtime{Installed: true})), "llama-server runs one model per process"},
		{"Ollama without a download", backend.Ollama{}, noGGUF, "no download of Qwen3.5-2B"},
		{"LM Studio without a download", backend.LMStudio{}, noGGUF, "no download of Qwen3.5-2B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			served, steps, err := tt.b.Prepare(tt.req, migrate.Anthropic)
			if err != nil {
				t.Fatal(err)
			}
			if served.Background != nil || served.BackgroundModel() != served.Model || !strings.Contains(served.BackgroundNote, tt.note) {
				t.Errorf("served = %+v, want no background and a note containing %q", served, tt.note)
			}
			for _, s := range steps {
				if strings.Contains(s.Title, "Qwen3.5-2B") || strings.Contains(strings.Join(s.Detail, " "), "Qwen3.5-2B") {
					t.Errorf("step %q prepares the background model anyway", s.Title)
				}
			}
		})
	}
}

func TestOllamaBenchmarkRecordsItsTimings(t *testing.T) {
	var asked []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked = append(asked, body)
		// Two tokens streamed, then the timings: 8,000 tokens read in 5s
		// and 300 written in 4s.
		fmt.Fprintln(w, `{"response":"","thinking":"The","done":false}`)
		fmt.Fprintln(w, `{"response":" handlers","done":false}`)
		fmt.Fprintln(w, `{"done":true,"prompt_eval_count":8000,"prompt_eval_duration":5000000000,"eval_count":300,"eval_duration":4000000000}`)
	}))
	t.Cleanup(srv.Close)
	into := &bench.Result{}
	steps := backend.Ollama{}.Benchmark(runtimes.Runtime{Endpoint: srv.URL}, "qwen3.6:latest", []int{8000, 8000}, into)
	if len(steps) != 2 || steps[0].Kind != migrate.KindMeasure {
		t.Fatalf("steps = %+v", steps)
	}
	fx := migrate.Effects{Client: srv.Client()}
	var progress [][2]int64
	for _, s := range steps {
		if err := s.Run(t.Context(), fx, func(done, total int64) { progress = append(progress, [2]int64{done, total}) }); err != nil {
			t.Fatal(err)
		}
	}
	if want := [][2]int64{{0, 300}, {1, 300}, {2, 300}, {300, 300}}; !slices.Equal(progress[:4], want) {
		t.Errorf("progress = %v, want the reply's length first, each streamed token, thinking or not, then the step done: %v", progress, want)
	}
	if first := asked[0]; first["stream"] != true {
		t.Errorf("the benchmark does not stream: %+v", first)
	}
	if len(into.Points) != 2 || into.Points[0].Context != 8000 || into.Points[0].ReadTokensPerSecond != 1600 || into.Points[0].WriteTokensPerSecond != 75 {
		t.Errorf("points = %+v, want Ollama's own counts and durations", into.Points)
	}
	first, second := asked[0], asked[1]
	opts := first["options"].(map[string]any)
	if first["model"] != "qwen3.6:latest" || opts["num_ctx"].(float64) < 10000 || first["prompt"] == second["prompt"] {
		t.Errorf("requests = %+v; want one window for both and a fresh prompt each time", asked)
	}
}

func TestOllamaBenchmarkRejectsATruncatedPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Ollama read half the prompt, as it does when one overflows.
		fmt.Fprint(w, `{"prompt_eval_count":4000,"prompt_eval_duration":3000000000,"eval_count":300,"eval_duration":4000000000}`)
	}))
	t.Cleanup(srv.Close)
	into := &bench.Result{}
	step := backend.Ollama{}.Benchmark(runtimes.Runtime{Endpoint: srv.URL}, "m", []int{8000}, into)[0]
	err := step.Run(t.Context(), migrate.Effects{Client: srv.Client()}, func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "read 4,000 of the 8,000-token prompt") || len(into.Points) != 0 {
		t.Errorf("err = %v, points %+v; want the short read refused and nothing recorded", err, into.Points)
	}
}
