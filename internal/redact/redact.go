// Package redact removes the user's own paths from text lo0 is about to
// share: each project becomes "project 1", "project 2" and so on, and the
// home directory becomes "~", so a report can be passed on without naming
// the user, their clients or their repositories.
package redact

import (
	"cmp"
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Paths replaces every project path in s, in its absolute, "~"-relative,
// JSON-escaped and Claude Code folder-name forms, then the home directory
// itself. Each is matched with either slash and either case of drive letter,
// because Windows accepts all of them and the files lo0 reads do not agree
// on one. projects are numbered in the order given, so callers decide which
// is "project 1".
//
// A path matches only where its name ends, so project /code/app leaves
// /code/app2 alone. A folder name runs on to the next character that cannot
// be in one, so a subfolder's folder is hidden whole, and any folder under
// home that names no known project becomes "another project". A filesystem
// root is never a project: replacing it would rewrite every slash.
func Paths(s, home string, projects []string) string {
	var swaps []swap
	add := func(path, to string) {
		for _, from := range forms(path) {
			swaps = append(swaps, swap{from: from, to: to})
			if quoted := jsonEscaped(from); quoted != from {
				swaps = append(swaps, swap{from: quoted, to: to})
			}
		}
		if dir := folderName(path); dir != "" {
			swaps = append(swaps, swap{from: dir, to: to, folder: true})
		}
	}
	seen := map[string]bool{}
	n := 0
	for _, raw := range projects {
		p := filepath.Clean(raw)
		if p == "." || seen[p] || p == filepath.Clean(home) || isRoot(p) {
			continue
		}
		seen[p] = true
		n++
		name := "project " + strconv.Itoa(n)
		add(raw, name)
		add(p, name)
		if home != "" {
			if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
				add(filepath.Join("~", rel), name)
			}
		}
	}
	if home != "" && !isRoot(filepath.Clean(home)) {
		h := filepath.Clean(home)
		for _, from := range forms(h) {
			swaps = append(swaps, swap{from: from, to: "~"})
			if quoted := jsonEscaped(from); quoted != from {
				swaps = append(swaps, swap{from: quoted, to: "~"})
			}
		}
		if dir := folderName(h); dir != "" {
			swaps = append(swaps, swap{from: dir, to: "~", folder: true, other: "another project"})
		}
	}
	// Longer paths first, so a project inside another is replaced whole
	// rather than as its parent's name with a suffix.
	slices.SortStableFunc(swaps, func(a, b swap) int { return cmp.Compare(len(b.from), len(a.from)) })
	return replace(s, swaps)
}

// swap is one spelling to replace. A folder swap is a Claude Code folder
// name, which runs on through the rest of the name; when there is a rest
// and other is set, other replaces the whole name instead of to.
type swap struct {
	from, to, other string
	folder          bool
}

// replace applies swaps, longest first, at boundaries only.
func replace(s string, swaps []swap) string {
	if len(swaps) == 0 {
		return s
	}
	var first [256]bool
	for _, sw := range swaps {
		first[sw.from[0]] = true
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); {
		if !first[s[i]] {
			i++
			continue
		}
		matched := false
		for _, sw := range swaps {
			if !strings.HasPrefix(s[i:], sw.from) {
				continue
			}
			end, to := i+len(sw.from), sw.to
			if sw.folder {
				rest := end
				for rest < len(s) && folderChar(s[rest]) {
					rest++
				}
				if rest > end && sw.other != "" {
					to = sw.other
				}
				end = rest
			} else if end < len(s) && nameChar(s[end]) {
				continue
			}
			b.WriteString(s[last:i])
			b.WriteString(to)
			i, last, matched = end, end, true
			break
		}
		if !matched {
			i++
		}
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// nameChar reports whether c can continue a file name, so a path followed
// by it is a different, longer name.
func nameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-' || c >= 0x80
}

// folderChar reports whether c can be in a Claude Code folder name, which
// holds only ASCII letters, digits and "-".
func folderChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}

// isRoot reports whether p is a filesystem or volume root, such as "/" or
// "C:\", in either slash.
func isRoot(p string) bool {
	q := strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/")
	return q == "" || len(q) == 2 && q[1] == ':'
}

// forms are the spellings of path to replace: as given and cleaned, each
// with the separators it has and with forward slashes, and each of those
// with the drive letter in the other case. Backslashes are swapped by hand
// rather than with filepath.ToSlash, which leaves them alone outside
// Windows, so a report made elsewhere from a Windows machine's files is
// still covered.
func forms(path string) []string {
	if path == "" {
		return nil
	}
	var out []string
	for _, p := range []string{path, filepath.Clean(path)} {
		for _, f := range []string{p, strings.ReplaceAll(p, `\`, "/")} {
			out = append(out, f)
			if len(f) >= 2 && f[1] == ':' && unicode.IsLetter(rune(f[0])) {
				out = append(out, string(unicode.SimpleFold(rune(f[0])))+f[1:])
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// folderName is the name Claude Code gives the folder under
// ~/.claude/projects that holds path's transcripts, which lo0's warnings
// name: every character but a letter or digit becomes "-". It is empty for a
// relative path, whose folder name would match too much.
func folderName(path string) string {
	drive := len(path) >= 2 && path[1] == ':'
	if !drive && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, `\`) {
		return ""
	}
	name := strings.Map(func(r rune) rune {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '-'
	}, path)
	if strings.Trim(name, "-") == "" {
		return ""
	}
	return name
}

// jsonEscaped is s as it appears inside a JSON string, which differs from s
// for Windows paths and anything with quotes.
func jsonEscaped(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return s
	}
	return string(b[1 : len(b)-1])
}
