package opencode

import (
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/harness"
)

// release is the OpenCode major release a mapping or plan is for. The two
// share the opencode command and configuration directory but read
// different files and keys: OpenCode 2 reads AGENTS.md only, runs a new
// plugin API, and renames provider, permission and the agent permission
// keys (https://opencode.ai/v2/docs/migrate-v1/).
type release int

const (
	v1 release = 1
	v2 release = 2
)

// releaseOf is the release that is installed. An OpenCode that is not
// installed, or that did not say its version, is taken to be OpenCode 2:
// its installer and Homebrew have installed 2 since 2.0.0 shipped on
// 11 Sep 2026.
func releaseOf(in harness.Installation) release {
	major, _, _ := strings.Cut(in.Version, ".")
	if n, err := strconv.Atoi(major); in.Installed && err == nil && n < 2 {
		return v1
	}
	return v2
}

// Config keys that OpenCode 2 renamed. OpenCode 2 still reads the
// OpenCode 1 names, but a value under the new name wins and a clash is
// reported as a warning, so each release is written in its own names.
func (r release) providerKey() string {
	if r == v1 {
		return "provider"
	}
	return "providers"
}

func (r release) mcpKey() string {
	if r == v1 {
		return "mcp"
	}
	return "mcp.servers"
}

func (r release) permissionKey() string {
	if r == v1 {
		return "permission"
	}
	return "permissions"
}
