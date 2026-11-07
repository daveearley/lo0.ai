package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
)

// mcpFile is the file of lo0's that claude-local passes to Claude Code
// with --mcp-config.
func mcpFile(req migrate.Request) string {
	return filepath.Join(req.State, Command, "mcp.json")
}

// codexServers names the Codex MCP servers claude-local loads: the user's
// own, apart from any the Claude Code setup has a server of the same name
// for, since the one from --mcp-config would replace it.
func codexServers(req migrate.Request) []string {
	var out []string
	for _, name := range harness.CodexUserServers(req.Codex) {
		if !claudeHasServer(req.Setup, name) {
			out = append(out, name)
		}
	}
	return out
}

func claudeHasServer(inv setup.Inventory, name string) bool {
	return slices.ContainsFunc(inv.MCPServers, func(s setup.MCPServer) bool { return s.Name == name })
}

// claudeHasExtension reports whether the user's Claude Code setup has a
// command, subagent or skill of the same name as a Codex prompt, agent or
// skill, which copying it would overwrite.
func claudeHasExtension(inv setup.Inventory, k codexsetup.ExtensionKind, name string) bool {
	var kind setup.ExtensionKind
	switch k {
	case codexsetup.ExtensionPrompt:
		kind = setup.ExtensionCommand
	case codexsetup.ExtensionAgent:
		kind = setup.ExtensionAgent
	case codexsetup.ExtensionSkill:
		kind = setup.ExtensionSkill
	}
	return slices.ContainsFunc(inv.ExtensionsOf(kind), func(e setup.Extension) bool { return e.Scope == setup.ScopeUser && e.Name == name })
}

// safeName reports whether a name read from the user's files can name a
// file or folder without reaching outside the folder it goes in.
func safeName(name string) bool {
	return name != "" && filepath.IsLocal(name) && !strings.ContainsAny(name, `/\:`)
}

// codexSteps are the steps that carry the user's own Codex setup over to
// Claude Code, and the arguments claude-local passes for it. Claude Code
// keeps user MCP servers in ~/.claude.json, which it rewrites as it runs,
// so lo0 could neither change that file safely nor undo the change: the
// servers go in a file of lo0's that claude-local loads instead.
func codexSteps(req migrate.Request) ([]migrate.Step, []string) {
	var steps []migrate.Step
	var args []string
	if names := codexServers(req); len(names) > 0 {
		path := mcpFile(req)
		steps = append(steps, mcpStep(req, path, names))
		args = append(args, "--mcp-config="+path)
	}
	for _, k := range []extensionCopy{skillCopy, promptCopy, agentCopy} {
		if s, ok := k.step(req); ok {
			steps = append(steps, s)
		}
	}
	return steps, args
}

func mcpStep(req migrate.Request, path string, names []string) migrate.Step {
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Write your Codex MCP servers for " + Command,
		Detail: []string{path, "mcpServers: " + strings.Join(names, ", ")},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			servers, err := harness.CodexServers(fx.Files.ReadFile, req.Codex)
			if err != nil {
				return err
			}
			out := make(map[string]any, len(names))
			for _, name := range names {
				if c, ok := servers[name]; ok {
					out[name] = claudeServer(c)
				}
			}
			data, err := json.MarshalIndent(map[string]any{"mcpServers": out}, "", "  ")
			if err != nil {
				return err
			}
			// It holds MCP env and header values, so only the user reads it.
			return migrate.WriteFile(path, 0o600, func([]byte) ([]byte, error) { return append(data, '\n'), nil })(ctx, fx, progress)
		},
	}
}

// claudeServer converts a Codex MCP server to Claude Code's form. Claude
// Code expands ${VAR} in these values, so a header Codex read from the
// environment is still read from it. Stdio servers get Claude Code's whole
// environment, so env_vars needs no entry.
func claudeServer(c codexsetup.MCPConfig) map[string]any {
	if c.HTTP() {
		out := map[string]any{"type": "http", "url": c.URL}
		if h := c.Headers(func(name string) string { return "${" + name + "}" }); len(h) > 0 {
			out["headers"] = h
		}
		return out
	}
	out := map[string]any{"type": "stdio", "command": c.Command}
	if len(c.Args) > 0 {
		out["args"] = c.Args
	}
	if len(c.Env) > 0 {
		out["env"] = c.Env
	}
	return out
}

// extensionCopy copies the user's own Codex extensions of one kind into
// the Claude Code folder for them.
type extensionCopy struct {
	kind codexsetup.ExtensionKind
	// dir is the folder under ~/.claude; one and many name the items.
	dir, one, many string
	// write copies one extension, given its folder of Claude Code's.
	write func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, dir string) error
}

var (
	skillCopy = extensionCopy{
		kind: codexsetup.ExtensionSkill, dir: "skills", one: "Codex skill", many: "Codex skills",
		write: func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, dir string) error {
			return migrate.CopyTree(ctx, fx, filepath.Dir(e.Path), filepath.Join(dir, e.Name))
		},
	}
	promptCopy = extensionCopy{
		kind: codexsetup.ExtensionPrompt, dir: "commands", one: "Codex prompt", many: "Codex prompts",
		write: func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, dir string) error {
			return convertFile(ctx, fx, e.Path, filepath.Join(dir, e.Name+".md"), func(src []byte) ([]byte, error) { return codexPrompt(src), nil })
		},
	}
	agentCopy = extensionCopy{
		kind: codexsetup.ExtensionAgent, dir: "agents", one: "Codex agent", many: "Codex agents",
		write: func(ctx context.Context, fx migrate.Effects, e codexsetup.Extension, dir string) error {
			return convertFile(ctx, fx, e.Path, filepath.Join(dir, e.Name+".md"), func(src []byte) ([]byte, error) { return codexAgent(e.Name, src) })
		},
	}
)

func (k extensionCopy) step(req migrate.Request) (migrate.Step, bool) {
	var items []codexsetup.Extension
	for _, e := range harness.CodexUserExtensions(req.Codex, k.kind) {
		if safeName(e.Name) && !claudeHasExtension(req.Setup, k.kind, e.Name) {
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		return migrate.Step{}, false
	}
	dir := filepath.Join(req.Layout.ConfigDir, k.dir)
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	noun := k.many
	if len(items) == 1 {
		noun = k.one
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  fmt.Sprintf("Copy %d %s to Claude Code", len(items), noun),
		Detail: []string{dir, strings.Join(names, ", ")},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				if err := k.write(ctx, fx, e, dir); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}, true
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

// codexPrompt makes a Codex custom prompt a Claude Code command. Both read
// description and argument-hint and fill $ARGUMENTS the same way, but
// Codex numbers arguments from $1 and Claude Code from $0, and Codex
// writes a literal dollar as $$ where Claude Code needs \$ only before
// what it would fill in.
func codexPrompt(src []byte) []byte {
	var b strings.Builder
	s := string(src)
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		next := s[i+1]
		switch {
		case next == '$':
			if rest := s[i+2:]; rest != "" && (rest[0] >= '0' && rest[0] <= '9' || strings.HasPrefix(rest, "ARGUMENTS")) {
				b.WriteByte('\\')
			}
			b.WriteByte('$')
			i++
		case next >= '1' && next <= '9' && (i+2 == len(s) || !isWord(s[i+2])):
			b.WriteString("$" + strconv.Itoa(int(next-'1')))
			i++
		default:
			b.WriteByte('$')
		}
	}
	return []byte(b.String())
}

func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// codexAgent makes a Codex custom agent a Claude Code subagent: its
// description, and its developer instructions as the subagent's prompt.
// The model it names is OpenAI's, so it is left out and the subagent uses
// the local model.
func codexAgent(name string, src []byte) ([]byte, error) {
	a, err := codexsetup.DecodeAgent(src)
	if err != nil {
		return nil, err
	}
	quote := func(s string) string {
		q, _ := json.Marshal(s)
		return string(q)
	}
	front := "---\nname: " + quote(name) + "\ndescription: " + quote(a.Description) + "\n---\n"
	body := a.DeveloperInstructions
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return []byte(front + body), nil
}
