package opencode_test

import (
	"testing"

	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/system/systemtest"
)

func TestStatusReadsTheConfiguredModel(t *testing.T) {
	for _, tt := range []struct {
		name, config string
		want         harness.Use
	}{
		{"no configuration", "", harness.Use{Command: "opencode", Model: "its default model"}},
		{
			"a local provider",
			`{"model":"ollama/lo0/q:64k","provider":{"ollama":{"name":"Ollama (local)","options":{"baseURL":"http://127.0.0.1:11434/v1"}}}}`,
			harness.Use{Command: "opencode", Model: "lo0/q:64k through Ollama", Local: true},
		},
		{
			"an OpenCode 2 provider",
			`{"model":"llamacpp/q","providers":{"llamacpp":{"name":"llama.cpp (local)","package":"@opencode/ai/providers/openai-compatible","settings":{"baseURL":"http://127.0.0.1:8080/v1"}}}}`,
			harness.Use{Command: "opencode", Model: "q through llama.cpp", Local: true},
		},
		{
			"OpenCode 2 and 1 entries for one provider",
			`{"model":"ollama/q","provider":{"ollama":{"name":"Old","options":{"baseURL":"https://far.example/v1"}}},"providers":{"ollama":{"name":"Ollama (local)","settings":{"baseURL":"http://localhost:11434/v1"}}}}`,
			harness.Use{Command: "opencode", Model: "q through Ollama", Local: true},
		},
		{
			"a hosted provider",
			`{"model":"anthropic/claude-opus-5-5"}`,
			harness.Use{Command: "opencode", Model: "anthropic/claude-opus-5-5"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.config != "" {
				files[p(".config/opencode/opencode.json")] = tt.config
			}
			got := opencode.Harness{}.Status(t.Context(), request(), migratetest.NewFiles(files), &systemtest.Runner{})
			if !got.Installed || len(got.Uses) != 1 || got.Uses[0] != tt.want {
				t.Errorf("status = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStatusReadsTheVersion(t *testing.T) {
	runner := &systemtest.Runner{Commands: map[string]systemtest.Result{"opencode --version": {Output: "1.14.2\n"}}}
	if got := (opencode.Harness{}).Status(t.Context(), request(), migratetest.NewFiles(nil), runner); got.Version != "1.14.2" {
		t.Errorf("version = %q, want 1.14.2", got.Version)
	}
}

func TestStatusWhenMissing(t *testing.T) {
	req := request()
	req.LookPath = (&systemtest.Runner{}).LookPath
	if got := (opencode.Harness{}).Status(t.Context(), req, migratetest.NewFiles(nil), &systemtest.Runner{}); got.Installed {
		t.Errorf("status = %+v, want not installed", got)
	}
}

func TestInstall(t *testing.T) {
	if s := (opencode.Harness{}).Install(migrate.Request{GOOS: "linux"}); s.Kind != migrate.KindInstall || s.Detail[0] != "curl -fsSL https://opencode.ai/v2/install | bash" {
		t.Errorf("step = %+v", s)
	}
	if s := (opencode.Harness{}).Install(migrate.Request{GOOS: "windows"}); s.Kind != migrate.KindManual || s.Detail[0] != "Run: npm install -g @opencode/cli" {
		t.Errorf("windows step = %+v, want one for the user", s)
	}
}
