package migrate_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/harness"
	"lo0.ai/lo0/internal/migrate"
	"lo0.ai/lo0/internal/migrate/migratetest"
	"lo0.ai/lo0/internal/runtimes"
)

func effects(files *migratetest.Files) migrate.Effects {
	return migrate.Effects{Files: files, Open: files.Open, Client: http.DefaultClient, Now: migratetest.Clock}
}

func step(kind migrate.Kind, title string, run migrate.Action) migrate.Step {
	return migrate.Step{Kind: kind, Title: title, Run: run}
}

func ok(context.Context, migrate.Effects, func(int64, int64)) error { return nil }

func TestApplyRunsStepsInOrder(t *testing.T) {
	var ran []string
	record := func(name string) migrate.Action {
		return func(_ context.Context, _ migrate.Effects, progress func(int64, int64)) error {
			ran = append(ran, name)
			progress(1, 2)
			return nil
		}
	}
	p := migrate.Plan{Steps: []migrate.Step{
		{Kind: migrate.KindDownload, Title: "already", Done: true, Run: record("already")},
		step(migrate.KindDownload, "pull", record("pull")),
		step(migrate.KindWrite, "write", record("write")),
	}}
	var events []migrate.Event
	out, err := migrate.Apply(t.Context(), p, effects(migratetest.NewFiles(nil)), func(e migrate.Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"pull", "write"}) {
		t.Errorf("ran %v, want the steps that were not done, in order", ran)
	}
	if want := []migrate.State{migrate.StateAlready, migrate.StateDone, migrate.StateDone}; !slices.Equal(out.States, want) || !out.Complete() {
		t.Errorf("states = %v, want %v", out.States, want)
	}
	if !slices.ContainsFunc(events, func(e migrate.Event) bool { return e.State == migrate.StateProgress && e.Done == 1 && e.Total == 2 }) {
		t.Errorf("progress was not reported: %+v", events)
	}
}

func TestApplyWritesFilesButWaitsOnManualSteps(t *testing.T) {
	p := migrate.Plan{Steps: []migrate.Step{
		{Kind: migrate.KindManual, Title: "Start Ollama"},
		step(migrate.KindDownload, "pull", func(context.Context, migrate.Effects, func(int64, int64)) error {
			t.Error("a download ran while the runtime was not ready")
			return nil
		}),
		step(migrate.KindWrite, "write", ok),
		step(migrate.KindCheck, "check", ok),
	}}
	out, err := migrate.Apply(t.Context(), p, effects(migratetest.NewFiles(nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []migrate.State{migrate.StateWaiting, migrate.StateSkipped, migrate.StateDone, migrate.StateSkipped}
	if !slices.Equal(out.States, want) || out.Complete() || !slices.Equal(out.Waiting(), []int{0}) {
		t.Errorf("states = %v, want %v", out.States, want)
	}
}

func TestApplyChecksWhetherManualStepsAreDone(t *testing.T) {
	failing := func(context.Context, migrate.Effects, func(int64, int64)) error { return errors.New("not yet") }
	p := migrate.Plan{Steps: []migrate.Step{
		{Kind: migrate.KindManual, Title: "done by the user", Run: ok},
		{Kind: migrate.KindManual, Title: "not done", Run: failing},
	}}
	out, err := migrate.Apply(t.Context(), p, effects(migratetest.NewFiles(nil)), nil)
	if err != nil {
		t.Fatalf("a manual step that is not done is not a failure: %v", err)
	}
	if !slices.Equal(out.States, []migrate.State{migrate.StateDone, migrate.StateWaiting}) {
		t.Errorf("states = %v", out.States)
	}
}

func TestApplyStopsAtTheFirstFailure(t *testing.T) {
	p := migrate.Plan{Steps: []migrate.Step{
		step(migrate.KindRuntime, "create", func(context.Context, migrate.Effects, func(int64, int64)) error { return errors.New("no space") }),
		step(migrate.KindWrite, "write", func(context.Context, migrate.Effects, func(int64, int64)) error {
			t.Error("a step after a failure ran")
			return nil
		}),
	}}
	out, err := migrate.Apply(t.Context(), p, effects(migratetest.NewFiles(nil)), nil)
	if err == nil || !strings.Contains(err.Error(), "create: no space") {
		t.Errorf("err = %v, want the failing step named", err)
	}
	if out.States[0] != migrate.StateFailed || out.States[1] != "" {
		t.Errorf("states = %v", out.States)
	}
}

// Ollama not running must not keep an undo from restoring the user's files.
func TestUndoCarriesOnPastAFailedRuntimeStep(t *testing.T) {
	down := func(context.Context, migrate.Effects, func(int64, int64)) error {
		return errors.New("connection refused")
	}
	p := migrate.Plan{Undo: true, Steps: []migrate.Step{
		step(migrate.KindRuntime, "remove the model", down),
		step(migrate.KindWrite, "restore opencode.json", ok),
	}}
	out, err := migrate.Apply(t.Context(), p, effects(migratetest.NewFiles(nil)), nil)
	if err == nil || !strings.Contains(err.Error(), "remove the model: connection refused") {
		t.Errorf("err = %v, want the failed step named", err)
	}
	if !slices.Equal(out.States, []migrate.State{migrate.StateFailed, migrate.StateDone}) {
		t.Errorf("states = %v, want the restore to run", out.States)
	}
}

func TestApplyHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p := migrate.Plan{Steps: []migrate.Step{step(migrate.KindWrite, "write", ok)}}
	if _, err := migrate.Apply(ctx, p, effects(migratetest.NewFiles(nil)), nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestWriteFileBacksUpWhatItReplaces(t *testing.T) {
	files := migratetest.NewFiles(map[string]string{"/home/u/a.json": "old"})
	write := func(content string) error {
		return migrate.WriteFile("/home/u/a.json", 0o600, func([]byte) ([]byte, error) {
			return []byte(content), nil
		})(t.Context(), effects(files), func(int64, int64) {})
	}
	if err := write("new"); err != nil {
		t.Fatal(err)
	}
	if got, _ := files.Get("/home/u/a.json"); got != "new" || files.Perm("/home/u/a.json") != 0o644 {
		t.Errorf("file = %q, %v; want new, keeping its mode", got, files.Perm("/home/u/a.json"))
	}
	backup := "/home/u/a.json.lo0-backup-20261104-183000"
	if got, ok := files.Get(backup); !ok || got != "old" || files.Perm(backup) != 0o600 {
		t.Errorf("backup = %q, %v, %v; files %v", got, ok, files.Perm(backup), files.Names())
	}
	before := len(files.Names())
	if err := write("new"); err != nil {
		t.Fatal(err)
	}
	if len(files.Names()) != before {
		t.Errorf("writing the same contents again made a backup: %v", files.Names())
	}
}

func TestWriteFileCreatesWithoutBackup(t *testing.T) {
	files := migratetest.NewFiles(nil)
	var got []byte
	err := migrate.WriteFile("/home/u/new", 0o644, func(old []byte) ([]byte, error) {
		got = old
		return []byte("x"), nil
	})(t.Context(), effects(files), func(int64, int64) {})
	if err != nil || got != nil || len(files.Names()) != 1 {
		t.Errorf("err %v, old %q, files %v", err, got, files.Names())
	}
}

func TestCheckSpeaksEachProtocol(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, r.URL.Path+" "+body.Model+" "+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/messages":
			if r.Header.Get("anthropic-version") == "" {
				http.Error(w, "missing anthropic-version", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ready"}]}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ready"}}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	served := migrate.Served{RuntimeName: "Ollama", BaseURL: srv.URL, Model: "lo0/m:64k", Token: "ollama"}
	fx := migrate.Effects{Client: srv.Client()}
	for _, p := range []migrate.Protocol{migrate.Anthropic, migrate.OpenAI} {
		if err := migrate.Check(served, p).Run(t.Context(), fx, func(int64, int64) {}); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	want := []string{"/v1/messages lo0/m:64k Bearer ollama", "/v1/chat/completions lo0/m:64k Bearer ollama"}
	if !slices.Equal(seen, want) {
		t.Errorf("requests = %q, want %q", seen, want)
	}
}

func TestCheckReportsTheRuntimesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"model \"lo0/m:64k\" not found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	err := migrate.Check(migrate.Served{RuntimeName: "Ollama", BaseURL: srv.URL, Model: "lo0/m:64k"}, migrate.Anthropic).
		Run(t.Context(), migrate.Effects{Client: srv.Client()}, func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v", err)
	}
}

type fakeBackend struct{ base string }

func (fakeBackend) ID() runtimes.ID { return runtimes.Ollama }
func (b fakeBackend) Prepare(req migrate.Request, p migrate.Protocol) (migrate.Served, []migrate.Step, error) {
	s := migrate.Served{Runtime: runtimes.Ollama, RuntimeName: "Ollama", BaseURL: b.base, Model: "lo0/" + string(p), Context: 65536}
	if bg := req.Background; bg != nil {
		s.Background = &migrate.ServedModel{Model: "lo0/" + bg.Model.ID, Context: bg.Context}
	}
	return s, []migrate.Step{{Kind: migrate.KindDownload, Title: "pull", Done: true}}, nil
}

func (fakeBackend) Revert(context.Context, migrate.Effects, migrate.Change) error { return nil }

type fakeHarness struct{}

func (fakeHarness) ID() harness.ID             { return harness.OpenCode }
func (fakeHarness) Name() string               { return "Fake" }
func (fakeHarness) Protocol() migrate.Protocol { return migrate.OpenAI }
func (fakeHarness) Plan(_ migrate.Request, s migrate.Served) (migrate.Setup, error) {
	return migrate.Setup{Steps: []migrate.Step{{Kind: migrate.KindWrite, Title: "write " + s.Model, Run: ok}}, Then: []string{"go"}}, nil
}

func TestBuildJoinsBackendAndHarness(t *testing.T) {
	req := migrate.Request{Features: []harness.Feature{
		{Name: "native", Status: harness.StatusNative},
		{Name: "migrated", Status: harness.StatusTranslate, Migrated: true},
		{Name: "by hand", Status: harness.StatusTranslate},
		{Name: "partly", Status: harness.StatusPartial},
		{Name: "lost", Status: harness.StatusLost},
	}}
	p, err := migrate.Build(req, fakeBackend{}, fakeHarness{})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		titles[i] = s.Title
	}
	if want := []string{"pull", "write lo0/openai", "Check that Ollama answers as lo0/openai"}; !slices.Equal(titles, want) {
		t.Errorf("steps = %q, want %q", titles, want)
	}
	var hand []string
	for _, f := range p.ByHand {
		hand = append(hand, f.Name)
	}
	if !slices.Equal(hand, []string{"by hand", "partly"}) {
		t.Errorf("by hand = %v", hand)
	}
	if p.HarnessName != "Fake" || p.Runtime != runtimes.Ollama || !p.Pending() || p.Then[0] != "go" {
		t.Errorf("plan = %+v", p)
	}
}

func TestBuildChecksTheBackgroundModelToo(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked = append(asked, body.Model)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ready"}}]}`))
	}))
	t.Cleanup(srv.Close)
	req := migrate.Request{Background: &migrate.Background{Model: catalog.Model{ID: "small"}, Context: 32768}}
	p, err := migrate.Build(req, fakeBackend{base: srv.URL}, fakeHarness{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Served.BackgroundModel() != "lo0/small" {
		t.Errorf("background model = %q", p.Served.BackgroundModel())
	}
	checks := p.Steps[len(p.Steps)-2:]
	if checks[0].Title != "Check that Ollama answers as lo0/openai" || checks[1].Title != "Check that Ollama answers as lo0/small" {
		t.Fatalf("checks = %q, %q", checks[0].Title, checks[1].Title)
	}
	for _, c := range checks {
		if err := c.Run(t.Context(), migrate.Effects{Client: srv.Client()}, func(int64, int64) {}); err != nil {
			t.Errorf("%s: %v", c.Title, err)
		}
	}
	if !slices.Equal(asked, []string{"lo0/openai", "lo0/small"}) {
		t.Errorf("asked %q, want each model once", asked)
	}
}

func TestServedOutputTokens(t *testing.T) {
	for ctx, want := range map[int]int{8192: 4096, 65536: 16384, 262144: 32000} {
		if got := (migrate.Served{Context: ctx}).OutputTokens(); got != want {
			t.Errorf("OutputTokens at %d = %d, want %d", ctx, got, want)
		}
	}
}

func TestOSFilesWritesAtomicallyWithParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "launcher")
	var files migrate.OSFiles
	if err := files.WriteFile(path, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := files.WriteFile(path, []byte("two"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := files.ReadFile(path)
	if err != nil || string(got) != "two" {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
	// Windows has no execute bit: it runs files by their extension.
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0) {
		t.Errorf("mode = %v, %v; want executable", info.Mode(), err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temporary files were left behind: %v", entries)
	}
}

// A config the user keeps private, or links in from a dotfiles repository,
// stays that way after lo0 rewrites it.
func TestOSFilesKeepsModeAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix modes, and symlinks need privileges")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "opencode.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := (migrate.OSFiles{}).WriteFile(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	if got, _ := os.ReadFile(real); string(got) != "new" {
		t.Errorf("target = %q, want the write to go through the link", got)
	}
	if info, err := os.Stat(real); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v; want 0600 kept", info.Mode(), err)
	}
}

// A file that will hold secrets is never left readable by others, and is
// never written through a link into a git repository, where the secrets
// could be committed.
func TestOSFilesKeepsPrivateWritesPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix modes, and symlinks need privileges")
	}
	dir := t.TempDir()
	open := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(open, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (migrate.OSFiles{}).WriteFile(open, []byte("SECRET_TOKEN"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(open); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v; want 0600", info.Mode(), err)
	}

	repo := filepath.Join(dir, "dotfiles")
	real := filepath.Join(repo, "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := (migrate.OSFiles{}).WriteFile(link, []byte("SECRET_TOKEN"), 0o600); !errors.Is(err, migrate.ErrPrivateInRepo) {
		t.Errorf("write through the link = %v, want ErrPrivateInRepo", err)
	}
	if got, _ := os.ReadFile(real); string(got) != "old" {
		t.Errorf("repository file = %q, want it untouched", got)
	}
	// A write with nothing private goes through, as before.
	if err := (migrate.OSFiles{}).WriteFile(link, []byte("new"), 0o644); err != nil {
		t.Errorf("public write through the link: %v", err)
	}
}
