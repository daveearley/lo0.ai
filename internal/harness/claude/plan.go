package claude

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/migrate"
)

// Command is the name of the launcher lo0 writes.
const Command = "claude-local"

// Protocol implements migrate.Harness: Claude Code speaks the Anthropic API.
func (Harness) Protocol() migrate.Protocol { return migrate.Anthropic }

// UsesBackground implements migrate.BackgroundUser: Claude Code sends
// session titles, summaries and haiku subagents such as Explore to its
// haiku model, which a small model answers faster.
func (Harness) UsesBackground() bool { return true }

// Plan implements migrate.Harness. The only change is one new file, the
// claude-local launcher, which sets Claude Code's documented environment
// variables for this process alone and starts the user's claude command.
func (Harness) Plan(req migrate.Request, served migrate.Served) (migrate.Setup, error) {
	dir := binDir(req)
	path := filepath.Join(dir, Command)
	if req.GOOS == "windows" {
		path += ".cmd"
	}
	vars := environment(served)
	script := launcher(req.GOOS, served, vars)

	detail := append([]string{path}, envDetail(vars)...)
	write := migrate.Step{
		Kind:   migrate.KindWrite,
		Title:  "Write the " + Command + " command",
		Detail: detail,
		Run:    migrate.WriteFile(path, 0o755, func([]byte) ([]byte, error) { return []byte(script), nil }),
	}

	run := Command
	if !onPath(req, dir) {
		run = path
	}
	then := []string{"Run " + run + " in any project to use Claude Code with " + served.DisplayName + " through " + served.RuntimeName + "."}
	if bg := served.Background; bg != nil {
		then = append(then, "Titles, summaries and haiku subagents such as Explore use "+bg.DisplayName+".")
	}
	then = append(then, "The claude command still uses Claude. To undo, run lo0 migrate --undo.")
	if run == path {
		then = append(then, dir+" is not on your PATH; add it to run "+Command+" by name.")
	}
	return migrate.Setup{Steps: []migrate.Step{write}, Then: then}, nil
}

// environment is what claude-local sets, all documented Claude Code
// settings. Every model alias resolves to a local model so nothing is sent
// to Anthropic: haiku, which Claude Code uses for background work, to the
// background model when there is one, and opus, sonnet and fable to the main
// model. The real window replaces the 200k Claude Code assumes for model
// names it does not know. The attribution block changes on every request,
// which would stop the runtime from reusing its cache of the prompt.
func environment(s migrate.Served) []migrate.EnvVar {
	vars := []migrate.EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: s.BaseURL},
		{Name: "ANTHROPIC_AUTH_TOKEN", Value: s.Token},
		{Name: "ANTHROPIC_MODEL", Value: s.Model},
		{Name: "ANTHROPIC_DEFAULT_OPUS_MODEL", Value: s.Model},
		{Name: "ANTHROPIC_DEFAULT_SONNET_MODEL", Value: s.Model},
		{Name: "ANTHROPIC_DEFAULT_FABLE_MODEL", Value: s.Model},
		{Name: "ANTHROPIC_DEFAULT_HAIKU_MODEL", Value: s.BackgroundModel()},
	}
	// The subagent setting applies to subagents whose definition names no
	// model; a definition that names an alias resolves through the aliases
	// above, and one that names a full model ID still wins over it. It is
	// left out with a background model; every alias a subagent can fall
	// back to is local either way.
	if s.Background == nil {
		vars = append(vars, migrate.EnvVar{Name: "CLAUDE_CODE_SUBAGENT_MODEL", Value: s.Model})
	}
	return append(vars, []migrate.EnvVar{
		{Name: "CLAUDE_CODE_MAX_CONTEXT_TOKENS", Value: strconv.Itoa(s.Context)},
		{Name: "CLAUDE_CODE_MAX_OUTPUT_TOKENS", Value: strconv.Itoa(s.OutputTokens())},
		{Name: "CLAUDE_CODE_ATTRIBUTION_HEADER", Value: "0"},
		{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
	}...)
}

// cleared are unset before Claude Code starts. An API key in the user's
// environment would be sent alongside the placeholder token.
var cleared = []string{"ANTHROPIC_API_KEY"}

func envDetail(vars []migrate.EnvVar) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Name+"="+v.Value)
	}
	return out
}

// launcher is the script for goos.
func launcher(goos string, s migrate.Served, vars []migrate.EnvVar) string {
	var b strings.Builder
	with := s.DisplayName
	if s.Background != nil {
		with += " and " + s.Background.DisplayName + " for background work"
	}
	about := "Written by lo0. Runs Claude Code with " + with + " through " + s.RuntimeName + ". Delete this file to undo."
	if goos == "windows" {
		b.WriteString("@echo off\r\nrem " + about + "\r\nsetlocal\r\n")
		for _, name := range cleared {
			b.WriteString("set " + name + "=\r\n")
		}
		// The quotes keep & | < > ^ in a value from being read as commands,
		// and a batch file expands % even inside them, so it is doubled.
		for _, v := range vars {
			b.WriteString(`set "` + v.Name + "=" + strings.ReplaceAll(v.Value, "%", "%%") + "\"\r\n")
		}
		b.WriteString("claude %*\r\n")
		return b.String()
	}
	b.WriteString("#!/bin/sh\n# " + about + "\n")
	for _, name := range cleared {
		b.WriteString("unset " + name + "\n")
	}
	for _, v := range vars {
		// Always quoted, with any quote in the value closed and escaped, so
		// a value such as one from OLLAMA_HOST can never end the string.
		b.WriteString("export " + v.Name + "='" + strings.ReplaceAll(v.Value, "'", `'\''`) + "'\n")
	}
	b.WriteString("exec claude \"$@\"\n")
	return b.String()
}

// binDir is where the launcher goes: the per-user program folder each
// platform's conventions use.
func binDir(req migrate.Request) string {
	if req.GOOS == "windows" {
		local := cmp.Or(getenv(req, "LOCALAPPDATA"), filepath.Join(req.Home, "AppData", "Local"))
		return filepath.Join(local, "lo0", "bin")
	}
	return filepath.Join(req.Home, ".local", "bin")
}

// onPath reports whether dir is listed in PATH. Windows compares folder
// names without regard to case and allows an entry to be quoted.
func onPath(req migrate.Request, dir string) bool {
	sep, same := ":", func(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }
	if req.GOOS == "windows" {
		// Either slash separates folders there, so both are reduced to one
		// form by hand: filepath only knows the host's own separator.
		folder := func(p string) string { return strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`) }
		sep, same = ";", func(a, b string) bool { return strings.EqualFold(folder(a), folder(b)) }
	}
	return slices.ContainsFunc(strings.Split(getenv(req, "PATH"), sep), func(p string) bool {
		p = strings.Trim(p, `"`)
		return p != "" && same(p, dir)
	})
}

func getenv(req migrate.Request, key string) string {
	if req.Getenv == nil {
		return ""
	}
	return req.Getenv(key)
}
