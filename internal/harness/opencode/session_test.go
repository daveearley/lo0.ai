package opencode_test

import (
	"encoding/json"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/harness/opencode"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/system/systemtest"
)

func TestSessionPassesConfigInline(t *testing.T) {
	type provider struct {
		NPM      string            `json:"npm"`
		Package  string            `json:"package"`
		Options  map[string]string `json:"options"`
		Settings map[string]string `json:"settings"`
		Models   map[string]struct {
			Limit struct {
				Context int `json:"context"`
			} `json:"limit"`
		} `json:"models"`
	}
	for _, tt := range []struct {
		name    string
		req     migrate.Request
		key     string
		baseURL func(provider) string
	}{
		{"OpenCode 1", opencode1(request()), "provider", func(p provider) string { return p.Options["baseURL"] }},
		{"OpenCode 2", request(), "providers", func(p provider) string { return p.Settings["baseURL"] }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, steps, err := opencode.Harness{}.Session(tt.req, served())
			if err != nil {
				t.Fatal(err)
			}
			if len(steps) != 0 {
				t.Errorf("steps = %+v, want none when OpenCode is installed", steps)
			}
			if s.Program != "opencode" || len(s.Env) != 1 || s.Env[0].Name != opencode.ConfigContentVar {
				t.Fatalf("session = %+v", s)
			}
			var cfg map[string]json.RawMessage
			if err := json.Unmarshal([]byte(s.Env[0].Value), &cfg); err != nil {
				t.Fatalf("inline config is not JSON: %v", err)
			}
			var model string
			var providers map[string]provider
			_ = json.Unmarshal(cfg["model"], &model)
			_ = json.Unmarshal(cfg[tt.key], &providers)
			ollama := providers["ollama"]
			if model != "ollama/lo0/qwen3-coder-30b-a3b:64k" || tt.baseURL(ollama) != "http://127.0.0.1:11434/v1" ||
				ollama.Models["lo0/qwen3-coder-30b-a3b:64k"].Limit.Context != 65536 {
				t.Errorf("inline config = %s", s.Env[0].Value)
			}
			if len(cfg) != 3 {
				t.Errorf("inline config = %s, want only $schema, %s and model", s.Env[0].Value, tt.key)
			}
			if _, ok := cfg["mcp"]; ok {
				t.Error("the inline config carries MCP servers, whose env values would show in the process list")
			}
			if !strings.Contains(strings.Join(s.Detail, "\n"), `"`+tt.key+`.ollama"`) {
				t.Errorf("detail = %q", s.Detail)
			}
		})
	}
}

func TestSessionAsksForOpenCodeWhenMissing(t *testing.T) {
	req := request()
	req.LookPath = (&systemtest.Runner{}).LookPath
	_, steps, err := opencode.Harness{}.Session(req, served())
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != migrate.KindManual || steps[0].Title != "Install OpenCode" {
		t.Errorf("steps = %+v", steps)
	}
}
