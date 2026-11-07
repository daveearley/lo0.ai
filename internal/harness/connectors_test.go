package harness

import "testing"

func TestConnectorName(t *testing.T) {
	const id = "917d60b0-8a5b-4ceb-8448-c4cd013721a8"
	tests := []struct {
		server string
		tools  []string
		want   string
	}{
		{"github", []string{"create_pull_request"}, "github"},
		{id, []string{"search_threads", "get_thread", "create_draft"}, "Gmail connector"},
		{id, []string{"stripe_api_read"}, "Stripe connector"},
		{id, []string{"getAccessibleAtlassianResources"}, "Atlassian connector"},
		{id, []string{"workers_list"}, "Cloudflare connector"},
		{id, []string{"update", "guide", "batch", "read"}, "claude.ai connector (update, guide, batch)"},
		{id, nil, "claude.ai connector 917d60b0"},
	}
	for _, tt := range tests {
		if got := connectorName(tt.server, tt.tools); got != tt.want {
			t.Errorf("connectorName(%s, %v) = %q, want %q", tt.server, tt.tools, got, tt.want)
		}
	}
}

func TestAppPlumbingIsNotAFeature(t *testing.T) {
	if !AppPlumbing("ccd_turn") || AppPlumbing("Claude_Browser") || AppPlumbing("github") {
		t.Error("appPlumbing misclassifies servers")
	}
}
