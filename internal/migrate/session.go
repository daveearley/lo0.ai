package migrate

import (
	"slices"
	"strings"

	"lo0.ai/lo0/internal/harness"
)

// Launcher starts one coding tool against a served model for a single
// session, passing everything through the session's environment so that
// none of the tool's own files change.
type Launcher interface {
	ID() harness.ID
	Name() string
	// Protocol is the API the harness speaks.
	Protocol() Protocol
	// Session returns how to start the harness against served and any steps
	// that must come first, such as installing it. It does no I/O beyond
	// req.LookPath.
	Session(req Request, served Served) (Session, []Step, error)
}

// EnvVar is one environment variable a session sets.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Session is a harness run once against a served model.
type Session struct {
	Program string   `json:"program"`
	Args    []string `json:"args,omitzero"`
	// Env is set for the session alone. Values are never secrets: local
	// runtimes take a placeholder token.
	Env []EnvVar `json:"env,omitzero"`
	// Unset are removed from the session's environment, such as an API key
	// the harness would otherwise send alongside the placeholder.
	Unset []string `json:"unset,omitzero"`
	// Detail says what the session is given, as the user reviews it.
	Detail []string `json:"detail,omitzero"`
}

// Command is the command line the session runs, as the user would type it
// in a POSIX shell.
func (s Session) Command() string {
	words := make([]string, 0, 1+len(s.Args))
	words = append(words, s.Program)
	for _, a := range s.Args {
		words = append(words, ShellQuote(a))
	}
	return strings.Join(words, " ")
}

// ShellQuote leaves a word that needs no quoting as it is and single-quotes
// any other, so it reads back as one word in a POSIX shell.
func ShellQuote(w string) string {
	special := func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("-_./:=@%+,", r)
	}
	if w != "" && !strings.ContainsFunc(w, special) {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

// Environ returns base, an environment in os.Environ form, with the
// session's variables unset or set. Later entries win in exec, but a
// duplicate would still be visible to the harness, so replaced and unset
// names are removed first.
func (s Session) Environ(base []string) []string {
	drop := slices.Clone(s.Unset)
	for _, v := range s.Env {
		drop = append(drop, v.Name)
	}
	out := make([]string, 0, len(base)+len(s.Env))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(drop, name) {
			out = append(out, kv)
		}
	}
	for _, v := range s.Env {
		out = append(out, v.Name+"="+v.Value)
	}
	return out
}

// BuildTry plans a single session of l with req's model: the backend's
// steps, the harness's own, a request that proves the two connect, and the
// session to start once they have run. Only the runtime changes; the
// harness's configuration is left as it is.
func BuildTry(req Request, b Backend, l Launcher) (Plan, error) {
	served, steps, err := prepare(req, b, l.Protocol())
	if err != nil {
		return Plan{}, err
	}
	session, own, err := l.Session(req, served)
	if err != nil {
		return Plan{}, err
	}
	keep := "lo0 migrate --to " + string(l.ID()) + " --model " + req.Model.ID + " --runtime " + string(served.Runtime)
	return Plan{
		Command: "try", Harness: l.ID(), HarnessName: l.Name(),
		Runtime: served.Runtime, RuntimeName: served.RuntimeName,
		Model: req.Model.Name, Quant: req.Quant.Name, Context: req.Context,
		Served:  served,
		Steps:   append(append(steps, own...), served.checks(l.Protocol())...),
		Session: &session,
		Then:    []string{"To keep " + req.Model.Name + " in " + l.Name() + ", run " + keep + "."},
	}, nil
}

// Changes reports whether applying the plan changes anything on the
// machine: a download, runtime change, write or install still has work to
// do. The final check and steps for the user change nothing.
func (p Plan) Changes() bool {
	for _, s := range p.Steps {
		if !s.Done && (s.Kind == KindDownload || s.Kind == KindRuntime || s.Kind == KindWrite || s.Kind == KindInstall) {
			return true
		}
	}
	return false
}
