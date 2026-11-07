package codex

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
	codexhome "lo0.ai/lo0/internal/codex"
	codexsetup "lo0.ai/lo0/internal/codex/setup"
	"lo0.ai/lo0/internal/migrate"
)

// Program is the Codex command.
const Program = "codex"

// Protocol implements migrate.Harness: Codex speaks only the Responses API
// to every provider, since it dropped wire_api = "chat".
func (Harness) Protocol() migrate.Protocol { return migrate.Responses }

// ProviderID is the model provider lo0 adds for a runtime. Codex reserves
// the ids of its built-in providers, ollama and lmstudio among them, so
// lo0 uses its own.
func ProviderID(s migrate.Served) string { return "lo0-" + string(s.Runtime) }

// Plan implements migrate.Harness. config.toml is backed up and changed so
// codex starts with the local model; the user's own skills and subagents
// are converted into the folders Codex reads. The user's repositories are
// left alone, so project-level items are listed for the user to convert.
func (Harness) Plan(req migrate.Request, served migrate.Served) (migrate.Setup, error) {
	home := codexhome.Home(req.Home, getenv(req))
	steps := prepare(req)
	steps = append(steps, configStep(req, served, home))
	if s, ok := skillStep(req); ok {
		steps = append(steps, s)
	}
	if s, ok := agentStep(req, home); ok {
		steps = append(steps, s)
	}
	return migrate.Setup{
		Steps: steps,
		Then: []string{
			"Run codex in any project to use " + served.DisplayName + " through " + served.RuntimeName + ".",
			"Codex runs your hooks only after you trust them with /hooks. To undo, run lo0 migrate --undo.",
		},
	}, nil
}

// configStep sets the provider, the default model and its window, the
// user's MCP servers and the CLAUDE.md fallback in config.toml.
func configStep(req migrate.Request, served migrate.Served, home string) migrate.Step {
	file := filepath.Join(home, codexsetup.ConfigFile)
	provider := ProviderID(served)
	var servers []string
	for _, s := range req.Setup.MCPServers {
		if s.Scope == setup.ScopeUser {
			servers = append(servers, s.Name)
		}
	}
	fallback := needsFallback(req.Features)
	detail := []string{
		file,
		"model = " + tomlString(served.Model),
		"model_provider = " + tomlString(provider),
		"model_context_window = " + strconv.Itoa(served.Context),
		"[model_providers." + provider + "] at " + baseURL(served),
	}
	if len(servers) > 0 {
		detail = append(detail, "[mcp_servers]: "+strings.Join(servers, ", "))
	}
	if fallback {
		detail = append(detail, "project_doc_fallback_filenames adds "+fallbackFile)
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Add " + served.DisplayName + " to Codex's configuration",
		Detail: detail,
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			mcp, err := userServers(fx.Files, req.Layout.GlobalConfig, servers)
			if err != nil {
				return err
			}
			e := edit{provider: provider, served: served, servers: mcp, fallback: fallback}
			// It may hold MCP env and header values, so only the user reads it.
			return migrate.WriteFile(file, 0o600, func(old []byte) ([]byte, error) { return mergeConfig(old, e) })(ctx, fx, progress)
		},
	}
}

// skillsDir is the user's skills folder Codex reads, under the home
// directory rather than the Codex home, which Codex keeps reading only for
// skills put there before.
func skillsDir(req migrate.Request) string {
	return filepath.Join(req.Home, ".agents", "skills")
}

// skillStep copies the user's own and plugin skills to Codex's skills
// folder. Codex reads the same SKILL.md format.
func skillStep(req migrate.Request) (migrate.Step, bool) {
	var items []setup.Extension
	for _, e := range req.Setup.ExtensionsOf(setup.ExtensionSkill) {
		if e.Scope == setup.ScopeUser || e.Scope == setup.ScopePlugin {
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		return migrate.Step{}, false
	}
	target := skillsDir(req)
	names := make([]string, 0, len(items))
	for _, e := range items {
		names = append(names, e.Name)
	}
	return migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Copy " + count(len(items), "skill", "skills") + " to the folder Codex reads skills from",
		Detail: append([]string{target}, wrapNames(names)...),
		Run: func(ctx context.Context, fx migrate.Effects, progress func(done, total int64)) error {
			for i, e := range items {
				src := filepath.Dir(e.Path)
				// The name comes from SKILL.md, so it must not reach outside
				// the skills folder.
				name := e.Name
				if !filepath.IsLocal(name) || strings.ContainsAny(name, `/\`) {
					name = filepath.Base(src)
				}
				if err := migrate.CopyTree(ctx, fx, src, filepath.Join(target, name)); err != nil {
					return err
				}
				progress(int64(i+1), int64(len(items)))
			}
			return nil
		},
	}, true
}

// detailWidth is how wide a list in a step's detail may run, so the plan
// fits in 80 columns.
const detailWidth = 72

// wrapNames lists names, comma-separated, over as many lines as it takes.
func wrapNames(names []string) []string {
	var lines []string
	line := ""
	for i, n := range names {
		if i < len(names)-1 {
			n += ","
		}
		if line != "" && len(line)+1+len(n) > detailWidth {
			lines, line = append(lines, line), ""
		}
		if line != "" {
			line += " "
		}
		line += n
	}
	return append(lines, line)
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func getenv(req migrate.Request) func(string) string {
	if req.Getenv == nil {
		return func(string) string { return "" }
	}
	return req.Getenv
}
