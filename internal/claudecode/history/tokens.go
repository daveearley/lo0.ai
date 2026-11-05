package history

import (
	"strings"

	"lo0.ai/lo0/internal/claudecode/transcript"
	"lo0.ai/lo0/internal/usage"
)

// tokensOf converts one response's usage. Cache writes without a lifetime
// breakdown are counted as 5-minute writes, the API default.
func tokensOf(u transcript.Usage) usage.Tokens {
	t := usage.Tokens{Input: int64(u.InputTokens), CacheRead: int64(u.CacheReadInputTokens), Output: int64(u.OutputTokens)}
	if c := u.CacheCreation; c != nil && c.Ephemeral5m+c.Ephemeral1h == u.CacheCreationInputTokens {
		t.CacheWrite5m, t.CacheWrite1h = int64(c.Ephemeral5m), int64(c.Ephemeral1h)
	} else {
		t.CacheWrite5m = int64(u.CacheCreationInputTokens)
	}
	return t
}

// variantOf is the pricing variant a response is billed under.
func variantOf(model string, u transcript.Usage) usage.Variant {
	return usage.Variant{
		Model:      model,
		Fast:       u.Speed == "fast",
		USOnly:     u.InferenceGeo == "us",
		LongPrompt: u.ContextTokens() > usage.LongPromptTokens,
	}
}

// mcpTool splits a Claude Code MCP tool name of the form
// mcp__<server>__<tool>.
func mcpTool(name string) (usage.MCPTool, bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return usage.MCPTool{}, false
	}
	server, tool, ok := strings.Cut(rest, "__")
	if !ok || server == "" {
		return usage.MCPTool{}, false
	}
	return usage.MCPTool{Server: server, Tool: tool}, true
}
