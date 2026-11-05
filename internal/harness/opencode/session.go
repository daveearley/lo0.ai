package opencode

import (
	"encoding/json"

	"lo0.ai/lo0/internal/migrate"
)

// ConfigContentVar is the environment variable OpenCode reads inline
// configuration from. Both releases merge it over the global and project
// configuration, key by key, so the user's own settings, MCP servers and
// rules still apply and only the provider and model are lo0's. OpenCode 1
// documents it (https://opencode.ai/docs/config/); OpenCode 2's own docs do
// not, but it still loads it after the global and project files
// (packages/core/src/config.ts in anomalyco/opencode, release 2.2.5).
const ConfigContentVar = "OPENCODE_CONFIG_CONTENT"

// Session implements migrate.Launcher. The provider and default model are
// passed inline, in the installed release's form, so opencode.json is not
// touched.
func (Harness) Session(req migrate.Request, served migrate.Served) (migrate.Session, []migrate.Step, error) {
	var steps []migrate.Step
	if req.LookPath == nil || !found(req.LookPath, Program) {
		steps = append(steps, install())
	}
	r := releaseOf(req.Target)
	provider := string(served.Runtime)
	model := provider + "/" + served.Model
	inline, err := json.Marshal(map[string]any{
		"$schema":       "https://opencode.ai/config.json",
		r.providerKey(): map[string]any{provider: providerEntry(served, r)},
		"model":         model,
	})
	if err != nil {
		return migrate.Session{}, nil, err
	}
	return migrate.Session{
		Program: Program,
		Env:     []migrate.EnvVar{{Name: ConfigContentVar, Value: string(inline)}},
		Detail: []string{
			`"model": "` + model + `"`,
			`"` + r.providerKey() + "." + provider + `" at ` + served.BaseURL + "/v1",
			"Passed in " + ConfigContentVar + "; opencode.json is not changed.",
		},
	}, steps, nil
}
