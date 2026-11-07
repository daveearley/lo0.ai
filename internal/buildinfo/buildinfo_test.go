package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{"go install", debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}, "v0.3.0"},
		{
			"local clean build",
			debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "4f2a1b0c9d8e7f6a"}}},
			"dev-4f2a1b0c9d8e",
		},
		{
			"local dirty build",
			debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}}},
			"dev-abc123-dirty",
		},
		{"no vcs", debug.BuildInfo{}, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromBuildInfo(&tt.info); got != tt.want {
				t.Errorf("fromBuildInfo = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionPrefersLinkedValue(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })
	version = "v9.9.9"
	if got := Version(); got != "v9.9.9" {
		t.Errorf("Version() = %q", got)
	}
}
