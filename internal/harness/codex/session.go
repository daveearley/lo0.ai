package codex

import (
	"strconv"

	"lo0.ai/lo0/internal/migrate"
)

// Session implements migrate.Launcher. The provider, model and window are
// passed as -c overrides, which Codex applies over every config.toml for
// this run alone, so no file changes. Codex reads each value as TOML.
func (Harness) Session(req migrate.Request, served migrate.Served) (migrate.Session, []migrate.Step, error) {
	steps := prepare(req)
	provider := ProviderID(served)
	key := "model_providers." + provider
	overrides := []string{
		key + ".name=" + tomlString(providerName(served)),
		key + ".base_url=" + tomlString(baseURL(served)),
		"model_provider=" + tomlString(provider),
		"model=" + tomlString(served.Model),
		"model_context_window=" + strconv.Itoa(served.Context),
	}
	args := make([]string, 0, 2*len(overrides))
	for _, o := range overrides {
		args = append(args, "-c", o)
	}
	return migrate.Session{
		Program: Program,
		Args:    args,
		Detail:  []string{"Passed with -c for this session only; config.toml is not changed."},
	}, steps, nil
}
