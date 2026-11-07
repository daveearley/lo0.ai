package usage_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/usage"
)

// harnessPackages are the packages that know one coding agent's files. The
// usage profile must stay independent of all of them, so that adding a
// harness never means changing what sits above usage.
var harnessPackages = []string{"lo0.ai/lo0/internal/claudecode", "lo0.ai/lo0/internal/codex"}

func TestUsageKnowsNoHarness(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, h := range harnessPackages {
				if path == h || strings.HasPrefix(path, h+"/") {
					t.Errorf("%s imports %s; harness formats belong in the harness package", name, path)
				}
			}
		}
	}
}

func TestGitReposWithoutManagedWorktrees(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app", ".claude", "worktrees", "gone")
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := usage.NewGitRepos().Repo(dir); got != dir {
		t.Errorf("Repo = %q, want the directory itself when no managed worktree dirs are given", got)
	}
}
