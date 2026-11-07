package codex

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/migrate"
)

// agentStep converts the user's own Claude Code subagents into Codex custom
// agents, one TOML file each in the Codex home's agents folder, with the
// subagent's prompt as developer_instructions
// (developers.openai.com/codex/subagents). Tool lists and models are left
// out: a Codex agent inherits both from the session.
func agentStep(req migrate.Request, home string) (migrate.Step, bool) {
	var items []setup.Extension
	for _, e := range req.Setup.ExtensionsOf(setup.ExtensionAgent) {
		if e.Scope == setup.ScopeUser {
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		return migrate.Step{}, false
	}
	target := filepath.Join(home, "agents")
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Convert " + count(len(items), "subagent", "subagents") + " to Codex custom agents",
		Detail: append([]string{target}, wrapNames(names)...),
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				src, err := fx.Files.ReadFile(e.Path)
				if err != nil {
					return fmt.Errorf("reading %s: %w", e.Path, err)
				}
				file := strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path)) + ".toml"
				write := migrate.WriteFile(filepath.Join(target, file), 0o644, func([]byte) ([]byte, error) { return agentTOML(e.Name, src), nil })
				if err := write(ctx, fx, func(int64, int64) {}); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}, true
}

// agentTOML is a Codex custom agent made from a Claude Code subagent file.
// Codex requires a description, so a subagent without one is described by
// its name.
func agentTOML(name string, src []byte) []byte {
	front, body := splitFrontmatter(strings.ReplaceAll(string(src), "\r\n", "\n"))
	description := cmp.Or(front["description"], name)
	var b strings.Builder
	b.WriteString("name = " + tomlString(name) + "\n")
	b.WriteString("description = " + tomlString(description) + "\n")
	b.WriteString("developer_instructions = " + multiline(strings.TrimSpace(body)) + "\n")
	return []byte(b.String())
}

// splitFrontmatter returns the single-line keys of a Markdown file's YAML
// frontmatter and the text after it.
func splitFrontmatter(src string) (map[string]string, string) {
	front := map[string]string{}
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return front, src
	}
	head, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return front, src
	}
	s := bufio.NewScanner(strings.NewReader(head))
	for s.Scan() {
		k, v, ok := strings.Cut(s.Text(), ":")
		if ok && !strings.HasPrefix(k, " ") {
			front[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return front, strings.TrimPrefix(body, "\n")
}

// multiline is s as a TOML multi-line literal string, which keeps it
// readable, or as a basic string when it holds what a literal cannot.
func multiline(s string) string {
	if strings.Contains(s, "'''") || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' || r == 0x7f }) {
		return tomlString(s)
	}
	return "'''\n" + s + "'''"
}
