package usage

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GitRepos maps working directories to the repository they belong to. A
// linked git worktree maps to its main repository, so sessions run in
// throwaway worktrees count towards the project they came from.
type GitRepos struct {
	stat     func(string) (fs.FileInfo, error)
	readFile func(string) ([]byte, error)
	// managed are the directories, relative to a project, where coding
	// agents create their own worktrees.
	managed []string

	mu    sync.Mutex
	cache map[string]string
}

// NewGitRepos returns a GitRepos that reads the real filesystem. managed
// names the directories, relative to a project such as ".claude/worktrees",
// where harnesses create worktrees, so that a session in one that has since
// been deleted still counts towards its project.
func NewGitRepos(managed ...string) *GitRepos {
	return &GitRepos{stat: os.Stat, readFile: os.ReadFile, managed: managed, cache: map[string]string{}}
}

// Repo returns the repository root for dir. A directory outside any
// repository, or one that no longer exists, maps to itself, except that a
// deleted managed worktree still maps to its parent project.
func (g *GitRepos) Repo(dir string) string {
	if dir == "" {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if repo, ok := g.cache[dir]; ok {
		return repo
	}
	repo := g.resolve(filepath.Clean(dir))
	g.cache[dir] = repo
	return repo
}

func (g *GitRepos) resolve(dir string) string {
	for d := dir; ; {
		dotgit := filepath.Join(d, ".git")
		if info, err := g.stat(dotgit); err == nil {
			if info.IsDir() {
				return d
			}
			if main := g.worktreeMain(dotgit); main != "" {
				return main
			}
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	for _, m := range g.managed {
		if before, _, ok := strings.Cut(filepath.ToSlash(dir), "/"+strings.Trim(filepath.ToSlash(m), "/")+"/"); ok {
			return filepath.FromSlash(before)
		}
	}
	return dir
}

// worktreeMain reads a worktree's .git file, which points at
// <main>/.git/worktrees/<name>, and returns <main>.
func (g *GitRepos) worktreeMain(dotgit string) string {
	data, err := g.readFile(dotgit)
	if err != nil {
		return ""
	}
	target, ok := bytes.CutPrefix(bytes.TrimSpace(data), []byte("gitdir:"))
	if !ok {
		return ""
	}
	gitdir := strings.TrimSpace(string(target))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(dotgit), gitdir)
	}
	before, _, ok := strings.Cut(filepath.ToSlash(gitdir), "/.git/worktrees/")
	if !ok {
		return ""
	}
	return filepath.FromSlash(before)
}
