package setup_test

import (
	"maps"
	"testing"

	"lo0.ai/lo0/internal/codex/setup"
)

func TestMCPConfigs(t *testing.T) {
	ref := func(name string) string { return "<" + name + ">" }
	servers, err := setup.MCPConfigs([]byte(`
model = "gpt-5.5"

[mcp_servers.fs]
command = "npx"
args = ["-y", "fs-mcp"]
env = { TOKEN = "literal", HOME_DIR = "/home/u" }
env_vars = ["PATH_EXTRA", { name = "ORG", source = "local" }, "TOKEN"]

[mcp_servers.docs]
url = "https://docs.example/mcp"
bearer_token_env_var = "DOCS_TOKEN"
http_headers = { "X-Team" = "core" }
env_http_headers = { "X-Key" = "DOCS_KEY" }
`))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		http    bool
		env     map[string]string
		headers map[string]string
	}{
		{
			name: "fs",
			env:  map[string]string{"TOKEN": "literal", "HOME_DIR": "/home/u", "PATH_EXTRA": "<PATH_EXTRA>", "ORG": "<ORG>"},
		},
		{
			name: "docs", http: true,
			headers: map[string]string{"X-Team": "core", "X-Key": "<DOCS_KEY>", "Authorization": "Bearer <DOCS_TOKEN>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := servers[tt.name]
			if !ok {
				t.Fatalf("servers = %v", servers)
			}
			if c.HTTP() != tt.http {
				t.Errorf("HTTP() = %v", c.HTTP())
			}
			if tt.http {
				if got := c.Headers(ref); !maps.Equal(got, tt.headers) {
					t.Errorf("Headers = %v, want %v", got, tt.headers)
				}
				return
			}
			if got := c.Environment(ref); !maps.Equal(got, tt.env) {
				t.Errorf("Environment = %v, want %v", got, tt.env)
			}
		})
	}
}

func TestDecodeAgent(t *testing.T) {
	a, err := setup.DecodeAgent([]byte(`name = "reviewer"
description = "Reviews diffs"
developer_instructions = """
Look for bugs.
"""
model = "gpt-5.5"
`))
	if err != nil || a.Name != "reviewer" || a.Description != "Reviews diffs" || a.DeveloperInstructions != "Look for bugs.\n" {
		t.Errorf("DecodeAgent = %+v, %v", a, err)
	}
}
