package opencode

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	"lo0.ai/lo0/internal/migrate"
)

// envRef matches Claude Code's ${VAR} references. Forms with a default,
// ${VAR:-default}, have no OpenCode equivalent and are left as they are.
var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func envRefs(s string) string {
	return envRef.ReplaceAllString(s, "{env:$1}")
}

// copyStep converts the user's own commands or subagents into OpenCode's
// global commands or agents directory.
func copyStep(req migrate.Request, r release, dir string, kind setup.ExtensionKind) (migrate.Step, bool) {
	var items []setup.Extension
	for _, e := range req.Setup.ExtensionsOf(kind) {
		if e.Scope == setup.ScopeUser {
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		return migrate.Step{}, false
	}
	sub, noun, convert := "commands", "commands", command
	if kind == setup.ExtensionAgent {
		sub, noun = "agents", "subagents"
		convert = func(src []byte) []byte { return agent(src, r) }
	}
	target := filepath.Join(dir, sub)
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  fmt.Sprintf("Convert %d %s to %s", len(items), noun, target),
		Detail: []string{strings.Join(names, ", ")},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				src, err := fx.Files.ReadFile(e.Path)
				if err != nil {
					return fmt.Errorf("reading %s: %w", e.Path, err)
				}
				// Nested Claude Code commands are named dir:name; OpenCode
				// takes the same folders.
				rel := filepath.FromSlash(strings.ReplaceAll(e.Name, ":", "/")) + ".md"
				if !filepath.IsLocal(rel) {
					rel = filepath.Base(e.Path)
				}
				dst := filepath.Join(target, rel)
				write := migrate.WriteFile(dst, 0o644, func([]byte) ([]byte, error) { return convert(src), nil })
				if err := write(ctx, fx, func(int64, int64) {}); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}, true
}

// command drops the frontmatter keys OpenCode reads differently:
// allowed-tools has no equivalent, and model names a Claude model.
func command(src []byte) []byte {
	front, body, ok := frontmatter(src)
	if !ok {
		return src
	}
	front = dropKeys(front, "allowed-tools", "model", "argument-hint", "disable-model-invocation")
	return render(front, body)
}

// agent makes a Claude Code subagent an OpenCode subagent. Its tool list
// becomes permission rules: a subagent that was not given a tool is denied
// the OpenCode permission covering it. OpenCode 1 takes a map of
// permissions; OpenCode 2 takes a list of rules and calls bash "shell"
// (https://opencode.ai/v2/docs/agents/).
func agent(src []byte, r release) []byte {
	front, body, ok := frontmatter(src)
	if !ok {
		return render([]string{"mode: subagent"}, string(src))
	}
	tools, listed := value(front, "tools")
	front = dropKeys(front, "name", "tools", "model", "color")
	front = append(front, "mode: subagent")
	if listed {
		var deny []string
		for _, p := range permissionTools {
			if slices.ContainsFunc(p.tools, func(t string) bool { return slices.Contains(tools, t) }) {
				continue
			}
			if r == v1 {
				deny = append(deny, "  "+p.v1+": deny")
			} else {
				deny = append(deny, "  - action: "+p.v2, `    resource: "*"`, "    effect: deny")
			}
		}
		if len(deny) > 0 {
			front = append(append(front, r.permissionKey()+":"), deny...)
		}
	}
	return render(front, body)
}

// permissionTools pairs OpenCode permissions, by their name in each
// release, with the Claude Code tools that need them.
var permissionTools = []struct {
	v1, v2 string
	tools  []string
}{
	{"edit", "edit", []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}},
	{"bash", "shell", []string{"Bash"}},
	{"webfetch", "webfetch", []string{"WebFetch"}},
}

// frontmatter splits a Markdown file into its YAML frontmatter lines and its
// body.
func frontmatter(src []byte) ([]string, string, bool) {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return nil, s, false
	}
	front, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		if front, ok = strings.CutSuffix(rest, "\n---"); !ok {
			return nil, s, false
		}
	}
	return strings.Split(front, "\n"), body, true
}

func render(front []string, body string) []byte {
	return []byte("---\n" + strings.Join(front, "\n") + "\n---\n" + body)
}

// dropKeys removes top-level keys and their indented continuation lines.
func dropKeys(lines []string, keys ...string) []string {
	var out []string
	dropping := false
	for _, l := range lines {
		if l != "" && l[0] != ' ' && l[0] != '\t' && l[0] != '-' {
			key, _, _ := strings.Cut(l, ":")
			dropping = slices.Contains(keys, strings.TrimSpace(key))
		}
		if !dropping {
			out = append(out, l)
		}
	}
	return out
}

// value reads a top-level key as a list, accepting the comma-separated,
// flow and block forms Claude Code's frontmatter uses.
func value(lines []string, key string) ([]string, bool) {
	for i, l := range lines {
		k, v, ok := strings.Cut(l, ":")
		if !ok || strings.TrimSpace(k) != key || strings.HasPrefix(l, " ") {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), "[]")
		var out []string
		if v == "" {
			for _, item := range lines[i+1:] {
				item = strings.TrimSpace(item)
				rest, ok := strings.CutPrefix(item, "-")
				if !ok {
					break
				}
				out = append(out, unquote(rest))
			}
			return out, true
		}
		for part := range strings.SplitSeq(v, ",") {
			if t := unquote(part); t != "" {
				out = append(out, t)
			}
		}
		return out, true
	}
	return nil, false
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}

// skillStep copies skills that come from Claude Code plugins, which OpenCode
// does not search, into its global skills directory.
func skillStep(req migrate.Request, dir string) (migrate.Step, bool) {
	var items []setup.Extension
	for _, e := range req.Setup.ExtensionsOf(setup.ExtensionSkill) {
		if e.Scope == setup.ScopePlugin {
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		return migrate.Step{}, false
	}
	target := filepath.Join(dir, "skills")
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  fmt.Sprintf("Copy %d plugin skills to %s", len(items), target),
		Detail: []string{strings.Join(names, ", ")},
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				src := filepath.Dir(e.Path)
				// The name comes from the plugin's SKILL.md, so it must not
				// reach outside the skills folder.
				name := e.Name
				if !filepath.IsLocal(name) || strings.ContainsAny(name, `/\`) {
					name = filepath.Base(src)
				}
				if err := copyTree(ctx, fx, src, filepath.Join(target, name)); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}, true
}

// copyTree copies every file under src to dst.
func copyTree(ctx context.Context, fx migrate.Effects, src, dst string) error {
	fsys := fx.Open(src)
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Join(src, filepath.FromSlash(p)), err)
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Join(src, filepath.FromSlash(p)), err)
		}
		perm := fs.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			perm = 0o755
		}
		write := migrate.WriteFile(filepath.Join(dst, filepath.FromSlash(path.Clean(p))), perm, func([]byte) ([]byte, error) { return data, nil })
		return write(ctx, fx, func(int64, int64) {})
	})
}
