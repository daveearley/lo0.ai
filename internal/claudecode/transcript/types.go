package transcript

// knownTypes are the record types Claude Code wrote up to the version lo0
// was last checked against. A type outside this set is a sign the format has
// moved on.
var knownTypes = map[string]bool{
	"assistant": true, "user": true, "system": true, "attachment": true, "summary": true,
	"last-prompt": true, "custom-title": true, "ai-title": true, "agent-name": true,
	"queue-operation": true, "mode": true, "pr-link": true, "frame-link": true,
	"file-history-snapshot": true, "file-history-delta": true, "bridge-session": true,
	"api-request": true, "api-request-blob": true, "api-request-shape": true,
	"cost-state": true, "tools-used": true, "atis-latch": true,
	"artifact-autoreact-ledger": true, "artifact-comment-monitor": true,
	"agent-color": true, "agent-setting": true, "attribution-snapshot": true,
	"content-replacement": true, "continued-in": true, "dev-mods": true,
	"ended-by-model": true, "fork-context-ref": true, "history-suppression": true,
	"isolation-latch": true, "memory-mode": true, "observer-ref": true,
	"permission-mode": true, "relocated": true, "repl-mode": true,
	"session-owner": true, "tag": true, "worktree-state": true,
}

// KnownType reports whether t is a record type lo0 has seen Claude Code
// write. The empty type is known: some tools write records without one.
func KnownType(t string) bool {
	return t == "" || knownTypes[t]
}
