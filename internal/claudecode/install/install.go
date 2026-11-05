// Package install finds the Claude Code builds on this machine and, when
// allowed, the newest published release.
//
// Claude Code changes where and how it stores things between releases, so
// lo0 reports which versions it is reading and how they compare with the
// newest release lo0 was checked against.
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"

	"lo0.ai/lo0/internal/system"
	"lo0.ai/lo0/internal/version"
)

// DefaultRegistryURL is the npm registry Claude Code is published to.
const DefaultRegistryURL = "https://registry.npmjs.org"

// packageName is Claude Code's npm package.
const packageName = "@anthropic-ai/claude-code"

// Installs are the Claude Code versions found.
type Installs struct {
	// CLI is the version of the claude command on PATH.
	CLI string `json:"cli,omitzero"`
	// App is the newest Claude Code build the Claude desktop app has
	// downloaded. The app runs its own copy, independent of the CLI.
	App string `json:"app,omitzero"`
	// Latest and Stable are the newest releases on each channel, empty when
	// the lookup was skipped or failed.
	Latest string `json:"latest,omitzero"`
	Stable string `json:"stable,omitzero"`
}

// AppDir returns the directory where the Claude desktop app keeps its Claude
// Code builds, one subdirectory per version, or "" on platforms where lo0
// does not know it.
func AppDir(goos, home string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude-code")
	}
	return ""
}

// Detector finds Claude Code installs.
type Detector struct {
	Runner system.Runner
	// App is the desktop app's Claude Code directory, or nil.
	App fs.FS
	// Client and RegistryURL look up the newest release. An empty
	// RegistryURL skips the lookup, for offline use.
	Client      *http.Client
	RegistryURL string
}

// Detect finds the installed versions and the newest release. Each part that
// fails is reported in the error and left empty in the result, except a
// release lookup that could not connect: lo0 works offline, so being offline
// is not a problem worth a warning.
func (d Detector) Detect(ctx context.Context) (Installs, error) {
	var in Installs
	var errs []error
	if _, err := d.Runner.LookPath("claude"); err == nil {
		out, err := d.Runner.Run(ctx, "claude", "--version")
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("reading the claude command's version: %w", err))
		default:
			in.CLI = version.Parse(string(out))
		}
	}
	if d.App != nil {
		in.App = newestBuild(d.App)
	}
	if d.RegistryURL != "" && d.Client != nil {
		latest, stable, err := d.releases(ctx)
		if _, offline := errors.AsType[*url.Error](err); err != nil && !offline {
			errs = append(errs, fmt.Errorf("checking the newest Claude Code release: %w", err))
		}
		in.Latest, in.Stable = latest, stable
	}
	return in, errors.Join(errs...)
}

// newestBuild returns the highest version among the directory's entries.
func newestBuild(app fs.FS) string {
	entries, err := fs.ReadDir(app, ".")
	if err != nil {
		return ""
	}
	newest := ""
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if v := version.Parse(e.Name()); v != "" && version.Compare(v, newest) > 0 {
			newest = v
		}
	}
	return newest
}

func (d Detector) releases(ctx context.Context) (latest, stable string, err error) {
	url := d.RegistryURL + "/-/package/" + packageName + "/dist-tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var tags struct {
		Latest string `json:"latest"`
		Stable string `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", "", fmt.Errorf("decoding %s: %w", url, err)
	}
	return version.Parse(tags.Latest), version.Parse(tags.Stable), nil
}
