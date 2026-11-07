// Package version compares and parses dotted release numbers, such as the
// versions coding agents and runtimes report.
package version

import (
	"cmp"
	"strconv"
	"strings"
)

// Compare orders dotted version strings such as "2.1.317" by their
// numeric parts, so "2.1.10" sorts after "2.1.9". Anything after a space or
// hyphen, such as " (Claude Code)" or "-beta", is ignored. The empty string
// sorts before every version.
func Compare(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
}

// Parse extracts the version number from output such as
// "2.1.221 (Claude Code)", or returns "" if there is none.
func Parse(s string) string {
	for field := range strings.FieldsSeq(s) {
		v := strings.TrimPrefix(field, "v")
		if len(versionParts(v)) >= 2 {
			core, _, _ := strings.Cut(v, "-")
			return core
		}
	}
	return ""
}

func versionParts(v string) []int {
	v, _, _ = strings.Cut(strings.TrimSpace(v), " ")
	v, _, _ = strings.Cut(v, "-")
	if v == "" {
		return nil
	}
	var parts []int
	for p := range strings.SplitSeq(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		parts = append(parts, n)
	}
	return parts
}
