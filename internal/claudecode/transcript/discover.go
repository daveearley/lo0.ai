package transcript

import (
	"cmp"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"lo0.ai/lo0/internal/jsonl"
)

// Session locates the transcript files of one session.
type Session struct {
	ID string `json:"id"`
	// Dir is the encoded project directory name Claude Code filed the session
	// under.
	Dir string `json:"dir"`
	// Main is the path of the session's own transcript.
	Main string `json:"main"`
	// Subagents are the transcripts of subagents the session started.
	Subagents []string `json:"subagents,omitzero"`
}

// Discover finds sessions in fsys, which must be rooted at the projects
// directory. The layout is:
//
//	<dir>/<session>.jsonl
//	<dir>/<session>/subagents/**/*.jsonl
//
// Subagent transcripts whose parent transcript is missing are ignored.
// A folder that cannot be read is skipped and reported in the returned error
// with the rest; only an unreadable root stops the walk.
func Discover(fsys fs.FS) ([]Session, error) {
	byKey := map[string]*Session{}
	var subagents []struct{ key, path string }
	var errs []error
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return skip(p, d, err, &errs)
		}
		if d.IsDir() || path.Ext(p) != ".jsonl" || !jsonl.Regular(fsys, p, d) {
			return nil
		}
		parts := strings.Split(p, "/")
		switch {
		case len(parts) == 2:
			id := strings.TrimSuffix(parts[1], ".jsonl")
			byKey[parts[0]+"/"+id] = &Session{ID: id, Dir: parts[0], Main: p}
		case len(parts) >= 4 && parts[2] == "subagents":
			subagents = append(subagents, struct{ key, path string }{parts[0] + "/" + parts[1], p})
		}
		return nil
	})
	for _, o := range subagents {
		if s := byKey[o.key]; s != nil {
			s.Subagents = append(s.Subagents, o.path)
		}
	}
	sessions := make([]Session, 0, len(byKey))
	for _, s := range byKey {
		slices.Sort(s.Subagents)
		sessions = append(sessions, *s)
	}
	slices.SortFunc(sessions, func(a, b Session) int {
		return cmp.Or(cmp.Compare(a.Dir, b.Dir), cmp.Compare(a.ID, b.ID))
	})
	return sessions, errors.Join(err, errors.Join(errs...))
}

// skip is how a walk carries on past an error: a file or folder that has
// gone since it was listed is no loss, one that cannot be read is reported,
// and only the root failing stops the walk.
func skip(p string, d fs.DirEntry, err error, errs *[]error) error {
	switch {
	case p == ".":
		return err
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	*errs = append(*errs, err)
	if d != nil && d.IsDir() {
		return fs.SkipDir
	}
	return nil
}
