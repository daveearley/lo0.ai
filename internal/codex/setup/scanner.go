package setup

import (
	"bufio"
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
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Opener returns a read-only filesystem rooted at an absolute directory.
// Production code passes os.DirFS; tests pass in-memory trees.
type Opener func(dir string) fs.FS

// Scanner builds an Inventory from a Codex home directory, the user's own
// skills folder and the projects config.toml names.
type Scanner struct {
	codexHome string
	home      string
	open      Opener
}

// NewScanner returns a Scanner that reads the Codex home directory
// codexHome, and skills under the user's home directory, through open.
func NewScanner(codexHome, home string, open Opener) *Scanner {
	return &Scanner{codexHome: codexHome, home: home, open: open}
}

// ConfigFile is config.toml's name in the Codex home directory and in a
// project's .codex folder.
const ConfigFile = "config.toml"

// ProfileSuffix ends the name of a profile file in the Codex home, such as
// fast.config.toml for codex --profile fast.
const ProfileSuffix = ".config.toml"

// Scan reads the Codex home, the user's skills and every project
// config.toml lists. Missing files are normal and ignored. Files that exist
// but cannot be parsed are reported in the returned error, and the rest of
// the inventory is still returned.
func (s *Scanner) Scan(ctx context.Context) (Inventory, error) {
	c := &collector{}
	user := dir{fsys: s.open(s.codexHome), path: s.codexHome, origin: Origin{Scope: ScopeUser}}
	cfg, found := c.config(user, ConfigFile)
	if found {
		f := cfg.file
		c.inv.Found, c.inv.Path = true, user.abs(ConfigFile)
		c.inv.Model, c.inv.Provider, c.inv.Keys = f.Model, f.ModelProvider, cfg.keys
		c.inv.Providers = providers(f.ModelProviders)
		c.inv.Plugins = plugins(f)
		c.inv.MCPServers = mcpServers(user.at(ConfigFile), f.MCPServers)
		c.inv.Hooks = inlineHooks(user.at(ConfigFile), f.Hooks)
	}
	c.profiles(user)
	c.instructions(user)
	c.hooks(user, HooksFile)
	c.skills(dir{fsys: s.open(s.home), path: s.home, origin: Origin{Scope: ScopeUser}}, ".agents/skills")
	c.skills(user, "skills")
	c.prompts(user, "prompts")
	c.agents(user, "agents")
	c.rules(user, "rules")

	for _, project := range slices.Sorted(maps.Keys(cfg.file.Projects)) {
		if err := ctx.Err(); err != nil {
			return c.inv, err
		}
		fsys := s.open(project)
		if _, err := fs.Stat(fsys, "."); err != nil {
			continue
		}
		c.inv.Projects = append(c.inv.Projects, project)
		p := dir{fsys: fsys, path: project, origin: Origin{Scope: ScopeProject, Project: project}}
		c.instructions(p)
		// A project at the home directory has the Codex home as its .codex
		// folder, which was read as the user's own.
		if filepath.Clean(p.abs(".codex")) == filepath.Clean(s.codexHome) {
			c.skills(p, ".agents/skills")
			continue
		}
		if pc, ok := c.config(p, ".codex/"+ConfigFile); ok {
			c.inv.MCPServers = append(c.inv.MCPServers, mcpServers(p.at(".codex/"+ConfigFile), pc.file.MCPServers)...)
			c.inv.Hooks = append(c.inv.Hooks, inlineHooks(p.at(".codex/"+ConfigFile), pc.file.Hooks)...)
		}
		c.hooks(p, ".codex/"+HooksFile)
		c.skills(p, ".agents/skills")
		c.skills(p, ".codex/skills")
		c.agents(p, ".codex/agents")
		c.rules(p, ".codex/rules")
	}
	return c.inv, errors.Join(c.errs...)
}

// Instruction file names Codex reads in each folder, the override first.
const (
	AgentsFile   = "AGENTS.md"
	OverrideFile = "AGENTS.override.md"
)

// HooksFile is the hooks file's name in the Codex home and a project's
// .codex folder.
const HooksFile = "hooks.json"

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

func (c *collector) config(d dir, name string) (parsedConfig, bool) {
	data, ok := c.read(d, name)
	if !ok {
		return parsedConfig{}, false
	}
	cfg, err := parseConfig(data)
	if err != nil {
		c.fail(d.abs(name), err)
		return parsedConfig{}, false
	}
	return cfg, true
}

// instructions records the AGENTS.md files in one folder. Codex reads the
// override instead of AGENTS.md when both are there, so only that one is
// recorded.
func (c *collector) instructions(d dir) {
	for _, name := range []string{OverrideFile, AgentsFile} {
		data, ok := c.read(d, name)
		if !ok {
			continue
		}
		c.inv.Instructions = append(c.inv.Instructions, InstructionFile{
			Origin: d.at(name), Override: name == OverrideFile, Bytes: int64(len(data)), Lines: lineCount(data),
		})
		return
	}
}

type hookMatchers map[string][]struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type string `json:"type"`
	} `json:"hooks"`
}

func (c *collector) hooks(d dir, name string) {
	data, ok := c.read(d, name)
	if !ok {
		return
	}
	var file struct {
		Hooks hookMatchers `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		c.fail(d.abs(name), err)
		return
	}
	origin := d.at(name)
	for _, event := range slices.Sorted(maps.Keys(file.Hooks)) {
		for _, m := range file.Hooks[event] {
			for _, h := range m.Hooks {
				c.inv.Hooks = append(c.inv.Hooks, Hook{Origin: origin, Event: event, Matcher: m.Matcher, Type: h.Type})
			}
		}
	}
}

// systemSkills is the folder Codex installs its own skills into, which are
// part of Codex rather than the user's setup.
const systemSkills = ".system"

func (c *collector) skills(d dir, root string) {
	for _, e := range c.entries(d, root) {
		if e.Name() == systemSkills {
			continue
		}
		p := path.Join(root, e.Name(), "SKILL.md")
		if data, ok := c.read(d, p); ok {
			c.extension(d, p, ExtensionSkill, cmp.Or(frontmatterName(data), e.Name()))
		}
	}
}

func (c *collector) extension(d dir, p string, kind ExtensionKind, name string) {
	at := d.at(p)
	// The home directory can also be a project, whose .agents/skills are
	// the user's own. They are counted once, at user scope.
	if slices.ContainsFunc(c.inv.Extensions, func(e Extension) bool { return e.Path == at.Path }) {
		return
	}
	c.inv.Extensions = append(c.inv.Extensions, Extension{Origin: at, Kind: kind, Name: name})
}

// prompts records the custom prompts in root. Codex reads only the
// Markdown files directly in it.
func (c *collector) prompts(d dir, root string) {
	for _, e := range c.entries(d, root) {
		if !e.IsDir() && strings.EqualFold(path.Ext(e.Name()), ".md") {
			c.extension(d, path.Join(root, e.Name()), ExtensionPrompt, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
		}
	}
}

// agents records the custom agents in root, one TOML file each, named by
// their name key.
func (c *collector) agents(d dir, root string) {
	for _, e := range c.entries(d, root) {
		if e.IsDir() || path.Ext(e.Name()) != ".toml" {
			continue
		}
		p := path.Join(root, e.Name())
		data, ok := c.read(d, p)
		if !ok {
			continue
		}
		var agent struct {
			Name string `toml:"name"`
		}
		if _, err := toml.Decode(string(data), &agent); err != nil {
			c.fail(d.abs(p), err)
			continue
		}
		c.extension(d, p, ExtensionAgent, cmp.Or(agent.Name, strings.TrimSuffix(e.Name(), ".toml")))
	}
}

// profiles records the profile files in the Codex home.
func (c *collector) profiles(d dir) {
	for _, e := range c.entries(d, ".") {
		name, ok := strings.CutSuffix(e.Name(), ProfileSuffix)
		if e.IsDir() || !ok || name == "" {
			continue
		}
		cfg, ok := c.config(d, e.Name())
		if !ok {
			continue
		}
		c.inv.Profiles = append(c.inv.Profiles, Profile{Name: name, Path: d.abs(e.Name()), Model: cfg.file.Model, Provider: cfg.file.ModelProvider})
	}
}

func (c *collector) entries(d dir, root string) []fs.DirEntry {
	entries, err := fs.ReadDir(d.fsys, root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.fail(d.abs(root), err)
	}
	return entries
}

func (c *collector) rules(d dir, root string) {
	for _, e := range c.entries(d, root) {
		if e.IsDir() || path.Ext(e.Name()) != ".rules" {
			continue
		}
		p := path.Join(root, e.Name())
		if data, ok := c.read(d, p); ok {
			c.inv.Rules = append(c.inv.Rules, RuleFile{Origin: d.at(p), Rules: bytes.Count(data, []byte("prefix_rule("))})
		}
	}
}

// frontmatterName returns the name field of a Markdown file's YAML
// frontmatter, or "" if there is none.
func frontmatterName(data []byte) string {
	s := bufio.NewScanner(bytes.NewReader(data))
	if !s.Scan() || strings.TrimSpace(s.Text()) != "---" {
		return ""
	}
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
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
