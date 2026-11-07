package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
)

// codexServerNames names the Codex MCP servers lo0 adds to opencode.json:
// the user's own, apart from any the Claude Code setup has a server of the
// same name for, which is carried over instead.
func codexServerNames(req migrate.Request) []string {
	var out []string
	for _, name := range harness.CodexUserServers(req.Codex) {
		if !claudeHasServer(req.Setup, name) {
			out = append(out, name)
		}
	}
	return out
}

// codexInstructionPaths are the user's own AGENTS.md files in the Codex
// home, which OpenCode reads once they are in "instructions".
func codexInstructionPaths(req migrate.Request) []string {
	var out []string
	for _, f := range harness.CodexOwnInstructions(req.Codex, req.Setup) {
		if f.Scope == codexsetup.ScopeUser {
			out = append(out, f.Path)
		}
	}
	return out
}

// addCodexServers reads the named servers from Codex's config.toml and adds
// them to mcp in OpenCode's form. Values move from one file on this
// machine to the other and are never shown.
func addCodexServers(files migrate.Files, req migrate.Request, names []string, mcp map[string]any, r release) (map[string]any, error) {
	if len(names) == 0 {
		return mcp, nil
	}
	servers, err := harness.CodexServers(files.ReadFile, req.Codex)
	if err != nil {
		return nil, err
	}
	if mcp == nil {
		mcp = map[string]any{}
	}
	for _, name := range names {
		if c, ok := servers[name]; ok {
			if _, taken := mcp[name]; !taken {
				mcp[name] = codexServer(c, r)
			}
		}
	}
	return mcp, nil
}

// codexServer converts a Codex MCP server. OpenCode starts local servers
// with its own environment, so env_vars needs no entry, and expands
// {env:VAR} anywhere in its configuration, so a header Codex read from the
// environment is still read from it.
func codexServer(c codexsetup.MCPConfig, r release) map[string]any {
	out := map[string]any{}
	if r == v1 {
		out["enabled"] = true
	}
	if c.HTTP() {
		out["type"], out["url"] = "remote", c.URL
		if h := c.Headers(func(name string) string { return "{env:" + name + "}" }); len(h) > 0 {
			out["headers"] = h
		}
		return out
	}
	out["type"], out["command"] = "local", append([]string{c.Command}, c.Args...)
	if len(c.Env) > 0 {
		out["environment"] = c.Env
	}
	if c.Cwd != "" {
		out["cwd"] = c.Cwd
	}
	return out
}

// codexSteps copy the user's own Codex prompts, agents and skills into
// OpenCode's global folders. Skills in ~/.agents/skills need nothing, since
// OpenCode reads that folder too.
func codexSteps(req migrate.Request, dir string) []migrate.Step {
	claude := req.Setup
	var steps []migrate.Step
	pick := func(k codexsetup.ExtensionKind, keep func(codexsetup.Extension) bool) []codexsetup.Extension {
		var out []codexsetup.Extension
		for _, e := range harness.CodexUserExtensions(req.Codex, k) {
			if safeName(e.Name) && !claudeHasExtension(claude, k, e.Name) && keep(e) {
				out = append(out, e)
			}
		}
		return out
	}
	all := func(codexsetup.Extension) bool { return true }
	if items := pick(codexsetup.ExtensionPrompt, all); len(items) > 0 {
		steps = append(steps, codexCopy(items, "Codex prompt", "Codex prompts", filepath.Join(dir, "commands"), func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, target string) error {
			return convertFile(ctx, fx, e.Path, filepath.Join(target, e.Name+".md"), func(src []byte) ([]byte, error) { return command(src), nil })
		}))
	}
	if items := pick(codexsetup.ExtensionAgent, all); len(items) > 0 {
		steps = append(steps, codexCopy(items, "Codex agent", "Codex agents", filepath.Join(dir, "agents"), func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, target string) error {
			return convertFile(ctx, fx, e.Path, filepath.Join(target, e.Name+".md"), codexAgent)
		}))
	}
	if items := pick(codexsetup.ExtensionSkill, func(e codexsetup.Extension) bool { return !inAgentsSkills(e.Path) }); len(items) > 0 {
		steps = append(steps, codexCopy(items, "Codex skill", "Codex skills", filepath.Join(dir, "skills"), func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, target string) error {
			return migrate.CopyTree(ctx, fx, filepath.Dir(e.Path), filepath.Join(target, e.Name))
		}))
	}
	return steps
}

func codexCopy(items []codexsetup.Extension, one, many, target string, write func(context.Context, migrate.Effects, codexsetup.Extension, string) error) migrate.Step {
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	noun := many
	if len(items) == 1 {
		noun = one
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  fmt.Sprintf("Copy %d %s to OpenCode", len(items), noun),
		Detail: []string{target, strings.Join(names, ", ")},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				if err := write(ctx, fx, e, target); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}
}

// safeName reports whether a name read from the user's files can name a
// file or folder without reaching outside the folder it goes in.
func safeName(name string) bool {
	return name != "" && filepath.IsLocal(name) && !strings.ContainsAny(name, `/\:`)
}

func convertFile(ctx context.Context, fx migrate.Effects, src, dst string, convert func([]byte) ([]byte, error)) error {
	data, err := fx.Files.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	out, err := convert(data)
	if err != nil {
		return fmt.Errorf("converting %s: %w", src, err)
	}
	return migrate.WriteFile(dst, 0o644, func([]byte) ([]byte, error) { return out, nil })(ctx, fx, func(int64, int64) {})
}

// codexAgent makes a Codex custom agent an OpenCode subagent: its
// description, and its developer instructions as the prompt. The model it
// names is OpenAI's, so it is left out and the agent uses the local model.
func codexAgent(src []byte) ([]byte, error) {
	a, err := codexsetup.DecodeAgent(src)
	if err != nil {
		return nil, err
	}
	description, _ := json.Marshal(a.Description)
	body := a.DeveloperInstructions
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return render([]string{"description: " + string(description), "mode: subagent"}, body), nil
}
