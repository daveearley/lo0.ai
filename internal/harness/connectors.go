package harness

import (
	"regexp"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/usage"
)

// connectorID matches the UUIDs claude.ai connectors appear under in tool
// names, such as mcp__917d60b0-8a5b-4ceb-8448-c4cd013721a8__search_threads.
var connectorID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// connectorSignatures recognise a product from fragments of its tool names.
// A product is named only when a fragment is distinctive enough that no
// other common connector would use it.
var connectorSignatures = []struct {
	name      string
	fragments []string
}{
	{"Stripe", []string{"stripe"}},
	{"Atlassian", []string{"jira", "confluence", "atlassian", "compass"}},
	{"Cloudflare", []string{"workers_", "d1_database", "kv_namespace", "r2_bucket", "hyperdrive"}},
	{"Notion", []string{"notion-"}},
	{"Gmail", []string{"search_threads", "create_draft", "label_thread", "mark_thread_spam"}},
	{"Google Calendar", []string{"list_calendars", "suggest_time", "respond_to_event"}},
	{"Google Drive", []string{"download_file_content", "list_recent_files", "get_file_permissions"}},
	{"Slack", []string{"slack"}},
	{"Linear", []string{"linear"}},
	{"GitHub", []string{"github", "pull_request"}},
	{"HubSpot", []string{"hubspot"}},
	{"Figma", []string{"figma"}},
	{"Asana", []string{"asana"}},
	{"Canva", []string{"canva"}},
	{"AllTrails", []string{"trails"}},
	{"PostHog", []string{"posthog", "render-ui"}},
}

// ServerName is the readable name of an MCP server seen in the history:
// claude.ai connectors appear under UUIDs and are named by their tools.
func ServerName(t usage.Tally) string {
	return connectorName(t.Name, t.Tools)
}

// connectorName gives a claude.ai connector a readable name. Servers that are
// not connectors keep their own name; connectors that cannot be recognised
// are described by the tools they offer rather than by their UUID.
func connectorName(server string, tools []string) string {
	if !connectorID.MatchString(server) {
		return server
	}
	lower := make([]string, len(tools))
	for i, t := range tools {
		lower[i] = strings.ToLower(t)
	}
	for _, sig := range connectorSignatures {
		for _, f := range sig.fragments {
			if slices.ContainsFunc(lower, func(t string) bool { return strings.Contains(t, f) }) {
				return sig.name + " connector"
			}
		}
	}
	if len(tools) == 0 {
		return "claude.ai connector " + server[:8]
	}
	shown := tools[:min(len(tools), 3)]
	return "claude.ai connector (" + strings.Join(shown, ", ") + ")"
}
