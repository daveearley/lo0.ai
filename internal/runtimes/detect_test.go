package runtimes_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"lo0.ai/lo0/internal/runtimes"
	"lo0.ai/lo0/internal/system/systemtest"
)

func serve(t *testing.T, routes map[string]string) string {
	t.Helper()
	mux := http.NewServeMux()
	for route, body := range routes {
		mux.HandleFunc(route, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(data)
}

func unreachable(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func TestOllamaBaseURL(t *testing.T) {
	tests := map[string]string{
		"":                                  "http://127.0.0.1:11434",
		"0.0.0.0":                           "http://127.0.0.1:11434",
		"0.0.0.0:8080":                      "http://127.0.0.1:8080",
		"gpu-box.local":                     "http://gpu-box.local:11434",
		"https://ollama.lan:443":            "https://ollama.lan:443",
		"http://[::1]:11500/":               "http://[::1]:11500",
		"http://bob:SECRET_PW@gpu-box:8080": "http://gpu-box:8080",
		"bob:SECRET_PW@gpu-box":             "http://gpu-box:11434",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := runtimes.OllamaBaseURL(in); got != want {
				t.Errorf("OllamaBaseURL(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestOllamaDetectorRunning(t *testing.T) {
	base := serve(t, map[string]string{
		"GET /api/version": `{"version":"0.34.4"}`,
		"GET /api/tags":    fixture(t, "ollama_tags.json"),
	})
	d := runtimes.OllamaDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: base}
	rt, err := d.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !rt.Installed || !rt.Running || rt.Version != "0.34.4" || rt.Endpoint != base {
		t.Fatalf("Detect = %+v, want installed and running 0.34.4 at %s", rt, base)
	}
	coder := findModel(t, rt.Models, "qwen3-coder:30b")
	if coder.Parameters != "30.5B" || coder.Quantization != "Q4_K_M" || coder.ContextLength != 262144 {
		t.Errorf("qwen3-coder = %+v", coder)
	}
	if !coder.HasCapability(runtimes.CapabilityTools) || coder.HasCapability(runtimes.CapabilityVision) {
		t.Errorf("qwen3-coder capabilities = %v, want tools only", coder.Capabilities)
	}
	if v := findModel(t, rt.Models, "lo0/qwen3-coder-30b-a3b:256k"); v.Parent != "qwen3-coder:30b" || coder.Parent != "" {
		t.Errorf("variant parent %q, download parent %q; want the variant made from the download", v.Parent, coder.Parent)
	}
}

func TestOllamaDetectorEnrichesOlderServers(t *testing.T) {
	base := serve(t, map[string]string{
		"GET /api/version": `{"version":"0.6.0"}`,
		"GET /api/tags":    `{"models":[{"name":"devstral:24b","size":14000000000,"details":{"format":"gguf","parameter_size":"23.6B","quantization_level":"Q4_K_M"}}]}`,
		"POST /api/show":   `{"capabilities":["completion","tools"],"model_info":{"llama.context_length":131072}}`,
	})
	d := runtimes.OllamaDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: base}
	rt, err := d.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	m := findModel(t, rt.Models, "devstral:24b")
	if m.ContextLength != 131072 || !m.HasCapability(runtimes.CapabilityTools) {
		t.Errorf("devstral = %+v, want context and tools from /api/show", m)
	}
}

// A model /api/show cannot describe is still installed: it is listed, the
// models after it are too, and the failure is reported.
func TestOllamaDetectorKeepsModelsItCannotDescribe(t *testing.T) {
	base := serve(t, map[string]string{
		"GET /api/version": `{"version":"0.6.0"}`,
		"GET /api/tags":    `{"models":[{"name":"a:1b","details":{"format":"gguf"}},{"name":"b:1b","details":{"format":"gguf"}}]}`,
	})
	d := runtimes.OllamaDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: base}
	rt, err := d.Detect(t.Context())
	if err == nil || len(rt.Models) != 2 {
		t.Errorf("Detect = %d models, %v; want both models and an error", len(rt.Models), err)
	}
}

func TestOllamaDetectorInstalledButStopped(t *testing.T) {
	runner := &systemtest.Runner{
		Paths: map[string]string{"ollama": "/usr/local/bin/ollama"},
		Commands: map[string]systemtest.Result{
			"ollama --version": {Output: "Warning: could not connect to a running Ollama instance\nWarning: client version is 0.34.4\n"},
		},
	}
	d := runtimes.OllamaDetector{Runner: runner, Client: http.DefaultClient, BaseURL: unreachable(t)}
	rt, err := d.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !rt.Installed || rt.Running || rt.Version != "0.34.4" || len(rt.Models) != 0 {
		t.Errorf("Detect = %+v, want installed, stopped, version 0.34.4", rt)
	}
}

func TestOllamaDetectorNotInstalled(t *testing.T) {
	d := runtimes.OllamaDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: unreachable(t)}
	rt, err := d.Detect(t.Context())
	if err != nil || rt.Installed {
		t.Fatalf("Detect = %+v, %v; want not installed, no error", rt, err)
	}
}

func TestLMStudioDetectorPrefersAPI(t *testing.T) {
	base := serve(t, map[string]string{
		"GET /api/v0/models": `{"data":[
			{"id":"qwen3-coder-30b-a3b-instruct","type":"llm","arch":"qwen3moe","compatibility_type":"mlx","quantization":"4bit","max_context_length":262144,"capabilities":["tool_use"]},
			{"id":"text-embedding-nomic","type":"embeddings"}
		]}`,
	})
	d := runtimes.LMStudioDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: base, Models: fstest.MapFS{}}
	rt, err := d.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !rt.Running || len(rt.Models) != 1 {
		t.Fatalf("Detect = %+v, want running with one chat model", rt)
	}
	m := rt.Models[0]
	if m.Parameters != "30B" || m.Format != runtimes.FormatMLX || !m.HasCapability(runtimes.CapabilityTools) {
		t.Errorf("model = %+v", m)
	}
}

func TestLMStudioDetectorFallsBackToDisk(t *testing.T) {
	models := fstest.MapFS{
		"lmstudio-community/Devstral-24B-GGUF/Devstral-Small-24B-Q4_K_M.gguf": {Data: make([]byte, 40)},
		"mlx-community/Qwen3-Coder-30B-A3B-4bit/config.json":                  {Data: []byte("{}")},
		"mlx-community/Qwen3-Coder-30B-A3B-4bit/model-00001.safetensors":      {Data: make([]byte, 60)},
		"mlx-community/Qwen3-Coder-30B-A3B-4bit/model-00002.safetensors":      {Data: make([]byte, 40)},
		"mlx-community/incomplete/model.safetensors":                          {Data: make([]byte, 5)},
	}
	d := runtimes.LMStudioDetector{Runner: &systemtest.Runner{}, Client: http.DefaultClient, BaseURL: unreachable(t), Models: models}
	rt, err := d.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !rt.Installed || rt.Running {
		t.Fatalf("Detect = %+v, want installed and not running", rt)
	}
	names := make([]string, 0, len(rt.Models))
	for _, m := range rt.Models {
		names = append(names, m.Name)
	}
	want := []string{"Devstral-Small-24B-Q4_K_M", "mlx-community/Qwen3-Coder-30B-A3B-4bit"}
	if !slices.Equal(names, want) {
		t.Fatalf("models = %v, want %v", names, want)
	}
	mlx := rt.Models[1]
	if mlx.SizeBytes != 100 || mlx.Quantization != "4-bit" || mlx.Parameters != "30B" {
		t.Errorf("mlx model = %+v", mlx)
	}
}

func TestLlamaCppDetector(t *testing.T) {
	runner := &systemtest.Runner{
		Paths: map[string]string{"llama-server": "/opt/homebrew/bin/llama-server"},
		Commands: map[string]systemtest.Result{
			"llama-server --version": {Output: "version: 6123 (4f2a1b0)\nbuilt with Apple clang for arm64-apple-darwin\n"},
		},
	}
	legacy := fstest.MapFS{"unsloth_Qwen3-Coder-30B-A3B-Instruct-GGUF_Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf": {Data: make([]byte, 8)}}
	hub := fstest.MapFS{
		"models--unsloth--GLM-4.7-Flash-GGUF/snapshots/a1/GLM-4.7-Flash-Q5_K_M.gguf": {Data: make([]byte, 4)},
		"models--unsloth--GLM-4.7-Flash-GGUF/snapshots/b2/GLM-4.7-Flash-Q5_K_M.gguf": {Data: make([]byte, 4)},
	}
	rt, err := runtimes.LlamaCppDetector{Runner: runner, Caches: []fs.FS{hub, legacy}}.Detect(t.Context())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !rt.Installed || rt.Version != "b6123" || len(rt.Models) != 2 {
		t.Fatalf("Detect = %+v, want both caches' models", rt)
	}
	// Two revisions of one 4-byte file are one model of 4 bytes.
	if glm := findModel(t, rt.Models, "GLM-4.7-Flash-Q5_K_M"); glm.Quantization != "Q5_K_M" || glm.SizeBytes != 4 {
		t.Errorf("hub model = %+v", glm)
	}
}

func TestIDName(t *testing.T) {
	for id, want := range map[runtimes.ID]string{runtimes.Ollama: "Ollama", runtimes.LMStudio: "LM Studio", runtimes.LlamaCpp: "llama.cpp", "vllm": "vllm"} {
		if got := id.Name(); got != want {
			t.Errorf("%s.Name() = %q, want %q", id, got, want)
		}
	}
}

func TestLlamaCppCacheDirs(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		name, goos string
		vars       map[string]string
		want       []string
	}{
		{"override", "linux", map[string]string{"LLAMA_CACHE": "/models"}, []string{"/models"}},
		{"macOS", "darwin", nil, []string{filepath.Join("/home/u", ".cache", "huggingface", "hub"), filepath.Join("/home/u", "Library", "Caches", "llama.cpp")}},
		{"hub cache variable", "darwin", map[string]string{"HF_HUB_CACHE": "/hub", "HF_HOME": "/hf"}, []string{"/hub", filepath.Join("/home/u", "Library", "Caches", "llama.cpp")}},
		{"HF_HOME", "linux", map[string]string{"HF_HOME": "/hf"}, []string{filepath.Join("/hf", "hub"), filepath.Join("/home/u", ".cache", "llama.cpp")}},
		{"linux xdg", "linux", map[string]string{"XDG_CACHE_HOME": "/xdg"}, []string{filepath.Join("/xdg", "huggingface", "hub"), filepath.Join("/xdg", "llama.cpp")}},
		{"windows", "windows", map[string]string{"LOCALAPPDATA": `C:\Users\u\AppData\Local`}, []string{filepath.Join("/home/u", ".cache", "huggingface", "hub"), filepath.Join(`C:\Users\u\AppData\Local`, "llama.cpp")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runtimes.LlamaCppCacheDirs(tt.goos, "/home/u", env(tt.vars)); !slices.Equal(got, tt.want) {
				t.Errorf("LlamaCppCacheDirs = %q, want %q", got, tt.want)
			}
		})
	}
}

func findModel(t *testing.T, models []runtimes.Model, name string) runtimes.Model {
	t.Helper()
	i := slices.IndexFunc(models, func(m runtimes.Model) bool { return m.Name == name })
	if i < 0 {
		t.Fatalf("model %q not found in %+v", name, models)
	}
	return models[i]
}
