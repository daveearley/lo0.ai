// Package buildinfo reports the version of the running binary.
package buildinfo

import "runtime/debug"

// version is set at link time:
//
//	go build -ldflags "-X lo0.ai/lo0/internal/buildinfo.version=v0.1.0"
var version string

// Version returns the linked version, then the module version recorded by
// `go install`, then the VCS revision, and finally "dev".
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return fromBuildInfo(info)
}

func fromBuildInfo(info *debug.BuildInfo) string {
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	v := "dev-" + revision[:min(len(revision), 12)]
	if modified {
		v += "-dirty"
	}
	return v
}
