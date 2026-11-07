package setup

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/claudecode"
)

// Opener returns a read-only filesystem rooted at an absolute directory.
// Production code passes os.DirFS; tests pass in-memory trees.
type Opener func(dir string) fs.FS

// Scanner builds an Inventory from a Claude Code layout.
type Scanner struct {
	layout claudecode.Layout
	open   Opener
}

// NewScanner returns a Scanner that reads the files in layout through open.
func NewScanner(layout claudecode.Layout, open Opener) *Scanner {
	return &Scanner{layout: layout, open: open}
}

// Scan reads every scope. Missing files are normal and ignored. Files that
// exist but cannot be parsed are reported in the returned error, and the rest
// of the inventory is still returned.
func (s *Scanner) Scan(ctx context.Context) (Inventory, error) {
	c := &collector{}
	global, found := s.readGlobal(c)
	if found {
		c.inv.Account = accountOf(global)
	}

	user := s.dir(s.layout.ConfigDir, Origin{Scope: ScopeUser})
	c.instruction(user, "CLAUDE.md", InstructionClaudeMD)
	enabled := c.settings(user, "settings.json")
	c.rules(user, "rules")
	c.extensions(user, "")
	s.scanPlugins(ctx, c, user, enabled)

	managed := s.dir(filepath.Dir(s.layout.ManagedSettings), Origin{Scope: ScopeManaged})
	c.settings(managed, filepath.Base(s.layout.ManagedSettings))
	c.mcpFile(managed, "managed-mcp.json")

	c.inv.MCPServers = append(c.inv.MCPServers, mcpServers(Origin{Scope: ScopeUser, Path: s.layout.GlobalConfig}, global.MCPServers)...)

	for _, project := range slices.Sorted(maps.Keys(global.Projects)) {
		if err := ctx.Err(); err != nil {
			return c.inv, err
		}
		if claudecode.IsWorktree(project) {
			continue
		}
		fsys := s.open(project)
		if _, err := fs.Stat(fsys, "."); err != nil {
			continue
		}
		c.inv.Projects = append(c.inv.Projects, project)
		s.scanProject(c, project, fsys, global.Projects[project].MCPServers)
	}
	return c.inv, errors.Join(c.errs...)
}

type globalConfig struct {
	OAuthAccount *struct {
		OrganizationType string `json:"organizationType"`
		RateLimitTier    string `json:"organizationRateLimitTier"`
		ExtraUsage       bool   `json:"hasExtraUsageEnabled"`
	} `json:"oauthAccount"`
	MaxTier    string                     `json:"claudeMaxTier"`
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]mcpServerConfig `json:"mcpServers"`
	} `json:"projects"`
}

func (s *Scanner) readGlobal(c *collector) (globalConfig, bool) {
	var g globalConfig
	d := s.dir(filepath.Dir(s.layout.GlobalConfig), Origin{Scope: ScopeUser})
	data, ok := c.read(d, filepath.Base(s.layout.GlobalConfig))
	if !ok {
		return g, false
	}
	if err := json.Unmarshal(data, &g); err != nil {
		c.fail(d.abs(filepath.Base(s.layout.GlobalConfig)), err)
		return g, false
	}
	return g, true
}

// maxTierPattern finds the usage multiple in a rate limit tier such as
// "default_claude_max_20x".
var maxTierPattern = regexp.MustCompile(`max_(\d+x)\b`)

// maxTier prefers the account profile's rate limit tier, which Claude Code
// refreshes from the server, over the top-level claudeMaxTier, which can be
// left behind after a plan change.
func maxTier(rateLimitTier, cached string) string {
	if m := maxTierPattern.FindStringSubmatch(rateLimitTier); m != nil {
		return m[1]
	}
	return cached
}

// accountOf reads the plan from the global config. No signed-in account
// means Claude Code runs on an API key.
func accountOf(g globalConfig) Account {
	if g.OAuthAccount == nil {
		return Account{Plan: PlanAPI}
	}
	a := Account{ExtraUsage: g.OAuthAccount.ExtraUsage}
	switch strings.TrimPrefix(g.OAuthAccount.OrganizationType, "claude_") {
	case "max":
		a.Plan, a.Tier = PlanMax, maxTier(g.OAuthAccount.RateLimitTier, g.MaxTier)
	case "pro":
		a.Plan = PlanPro
	case "team":
		a.Plan = PlanTeam
	case "enterprise":
		a.Plan = PlanEnterprise
	}
	return a
}

func (s *Scanner) scanProject(c *collector, project string, fsys fs.FS, local map[string]mcpServerConfig) {
	p := dir{fsys: fsys, path: project, origin: Origin{Scope: ScopeProject, Project: project}}
	l := p.withScope(ScopeLocal)

	c.instruction(p, "CLAUDE.md", InstructionClaudeMD)
	c.instruction(p, ".claude/CLAUDE.md", InstructionClaudeMD)
	c.instruction(p, "AGENTS.md", InstructionAgentsMD)
	c.instruction(l, "CLAUDE.local.md", InstructionLocalMD)
	c.rules(p, ".claude/rules")
	c.settings(p, ".claude/settings.json")
	c.settings(l, ".claude/settings.local.json")
	c.mcpFile(p, ".mcp.json")
	c.extensions(p, ".claude/")
	localOrigin := l.origin
	localOrigin.Path = s.layout.GlobalConfig
	c.inv.MCPServers = append(c.inv.MCPServers, mcpServers(localOrigin, local)...)
}

type installedPlugins struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
	} `json:"plugins"`
}

type pluginManifest struct {
	MCPServers json.RawMessage `json:"mcpServers"`
	Hooks      json.RawMessage `json:"hooks"`
}

func (s *Scanner) scanPlugins(ctx context.Context, c *collector, user dir, enabled map[string]bool) {
	data, ok := c.read(user, "plugins/installed_plugins.json")
	if !ok {
		return
	}
	var installed installedPlugins
	if err := json.Unmarshal(data, &installed); err != nil {
		c.fail(user.abs("plugins/installed_plugins.json"), err)
		return
	}
	for _, id := range slices.Sorted(maps.Keys(installed.Plugins)) {
		if ctx.Err() != nil {
			return
		}
		installs := installed.Plugins[id]
		if len(installs) == 0 {
			continue
		}
		name, marketplace, _ := strings.Cut(id, "@")
		install := installs[0]
		c.inv.Plugins = append(c.inv.Plugins, Plugin{
			Name:        name,
			Marketplace: marketplace,
			Version:     install.Version,
			Enabled:     enabled[id],
		})
		if !enabled[id] || install.InstallPath == "" {
			continue
		}
		root := s.dir(install.InstallPath, Origin{Scope: ScopePlugin, Plugin: name})
		s.scanPlugin(c, root)
	}
}

func (s *Scanner) scanPlugin(c *collector, root dir) {
	var manifest pluginManifest
	if data, ok := c.read(root, ".claude-plugin/plugin.json"); ok {
		if err := json.Unmarshal(data, &manifest); err != nil {
			c.fail(root.abs(".claude-plugin/plugin.json"), err)
		}
	}
	c.extensions(root, "")

	mcpPath, inline := embeddedOrPath(manifest.MCPServers, ".mcp.json")
	if inline != nil {
		var servers map[string]mcpServerConfig
		if err := json.Unmarshal(inline, &servers); err != nil {
			c.fail(root.abs(".claude-plugin/plugin.json"), err)
		}
		c.inv.MCPServers = append(c.inv.MCPServers, mcpServers(root.at(".claude-plugin/plugin.json"), servers)...)
	} else {
		c.mcpFile(root, mcpPath)
	}

	hooksPath, inline := embeddedOrPath(manifest.Hooks, "hooks/hooks.json")
	if inline != nil {
		var events hookMatchers
		if err := json.Unmarshal(inline, &events); err != nil {
			c.fail(root.abs(".claude-plugin/plugin.json"), err)
		}
		c.inv.Hooks = append(c.inv.Hooks, hooks(root.at(".claude-plugin/plugin.json"), events)...)
		return
	}
	if data, ok := c.read(root, hooksPath); ok {
		var file struct {
			Hooks hookMatchers `json:"hooks"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			c.fail(root.abs(hooksPath), err)
		}
		c.inv.Hooks = append(c.inv.Hooks, hooks(root.at(hooksPath), file.Hooks)...)
	}
}

// embeddedOrPath interprets a plugin manifest field that is either a relative
// path to a JSON file or the JSON object itself.
func embeddedOrPath(raw json.RawMessage, fallback string) (string, json.RawMessage) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return fallback, nil
	}
	var p string
	if err := json.Unmarshal(raw, &p); err == nil {
		return path.Clean(strings.TrimPrefix(p, "./")), nil
	}
	return "", raw
}

func (s *Scanner) dir(abs string, origin Origin) dir {
	return dir{fsys: s.open(abs), path: abs, origin: origin}
}

type dir struct {
	fsys   fs.FS
	path   string
	origin Origin
}

func (d dir) abs(name string) string {
	return filepath.Join(d.path, filepath.FromSlash(name))
}

func (d dir) at(name string) Origin {
	o := d.origin
	o.Path = d.abs(name)
	return o
}

func (d dir) withScope(s Scope) dir {
	d.origin.Scope = s
	return d
}

type collector struct {
	inv  Inventory
	errs []error
}

func (c *collector) fail(path string, err error) {
	c.errs = append(c.errs, fmt.Errorf("reading %s: %w", path, err))
}

func (c *collector) read(d dir, name string) ([]byte, bool) {
	data, err := fs.ReadFile(d.fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		c.fail(d.abs(name), err)
		return nil, false
	}
	return data, true
}

func (c *collector) instruction(d dir, name string, kind InstructionKind) {
	// The home directory can also be a project, whose .claude/CLAUDE.md is
	// the user's own file. It is one file, counted once, at user scope.
	at := d.at(name)
	if slices.ContainsFunc(c.inv.Instructions, func(f InstructionFile) bool { return f.Path == at.Path }) {
		return
	}
	data, ok := c.read(d, name)
	if !ok {
		return
	}
	c.inv.Instructions = append(c.inv.Instructions, InstructionFile{
		Origin: at,
		Kind:   kind,
		Bytes:  int64(len(data)),
		Lines:  lineCount(data),
	})
}

func (c *collector) rules(d dir, root string) {
	c.walk(d, root, func(p string) {
		if strings.EqualFold(path.Ext(p), ".md") {
			c.instruction(d, p, InstructionRule)
		}
	})
}

// settings records one settings file and returns the plugins it enables.
func (c *collector) settings(d dir, name string) map[string]bool {
	data, ok := c.read(d, name)
	if !ok {
		return nil
	}
	parsed, err := parseSettings(d.at(name), data)
	if err != nil {
		c.fail(d.abs(name), err)
		return nil
	}
	c.inv.Settings = append(c.inv.Settings, parsed.file)
	c.inv.Hooks = append(c.inv.Hooks, parsed.hooks...)
	if parsed.permissions != nil {
		c.inv.Permissions = append(c.inv.Permissions, *parsed.permissions)
	}
	return parsed.plugins
}

func (c *collector) mcpFile(d dir, name string) {
	data, ok := c.read(d, name)
	if !ok {
		return
	}
	var file struct {
		MCPServers map[string]mcpServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		c.fail(d.abs(name), err)
		return
	}
	c.inv.MCPServers = append(c.inv.MCPServers, mcpServers(d.at(name), file.MCPServers)...)
}

func (c *collector) extensions(d dir, prefix string) {
	commands := prefix + "commands"
	c.walk(d, commands, func(p string) {
		if !strings.EqualFold(path.Ext(p), ".md") {
			return
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(p, commands+"/"), path.Ext(p))
		c.extension(d, p, ExtensionCommand, strings.ReplaceAll(rel, "/", ":"))
	})
	c.walk(d, prefix+"agents", func(p string) {
		if !strings.EqualFold(path.Ext(p), ".md") {
			return
		}
		data, _ := c.read(d, p)
		name := cmp.Or(frontmatterName(data), strings.TrimSuffix(path.Base(p), path.Ext(p)))
		c.extension(d, p, ExtensionAgent, name)
	})
	skills := prefix + "skills"
	entries, err := fs.ReadDir(d.fsys, skills)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.fail(d.abs(skills), err)
	}
	for _, e := range entries {
		p := path.Join(skills, e.Name(), "SKILL.md")
		if data, ok := c.read(d, p); ok {
			c.extension(d, p, ExtensionSkill, cmp.Or(frontmatterName(data), e.Name()))
		}
	}
}

func (c *collector) extension(d dir, p string, kind ExtensionKind, name string) {
	c.inv.Extensions = append(c.inv.Extensions, Extension{Origin: d.at(p), Kind: kind, Name: name})
}

// walk calls visit for every file under root, in lexical order. Symlinked
// files are included because users often link shared commands and rules in.
func (c *collector) walk(d dir, root string, visit func(p string)) {
	err := fs.WalkDir(d.fsys, root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			visit(p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.fail(d.abs(root), err)
	}
}

func lineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}
