// Package codex knows where OpenAI's Codex keeps its configuration and
// history.
package codex

import "path/filepath"

// VerifiedVersion is the newest Codex release whose session log format lo0
// was checked against. Raise it only after checking a newer release's logs.
const VerifiedVersion = "0.155.0"

// Harness is how the report names Codex.
const Harness = "Codex"

// HomeVar moves the Codex home directory, as it does in Codex.
const HomeVar = "CODEX_HOME"

// Home returns the Codex home directory: CODEX_HOME if set, else ~/.codex.
func Home(home string, getenv func(string) string) string {
	if dir := getenv(HomeVar); dir != "" {
		return dir
	}
	return filepath.Join(home, ".codex")
}

// Folders under the Codex home that hold session logs: SessionsSubdir in
// YYYY/MM/DD folders, ArchivedSubdir flat, where archiving a thread moves it.
const (
	SessionsSubdir = "sessions"
	ArchivedSubdir = "archived_sessions"
)
