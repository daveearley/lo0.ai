package codex

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// document is a TOML file as lines, with where each statement starts and
// ends, so lo0 can change a few keys and tables and leave the rest of the
// file, comments and order included, exactly as the user wrote it.
type document struct {
	lines []string
	stmts []statement
	// newline is the file's line ending.
	newline string
}

// statement is a table header or a key/value pair, which can span lines.
type statement struct {
	start, end int
	header     bool
	// table is the table an assignment is in; nil for the root table.
	table []string
	// key is an assignment's key or a header's table, with quotes removed.
	key []string
}

func parseDocument(data string) document {
	d := document{newline: "\n"}
	if strings.Contains(data, "\r\n") {
		d.newline = "\r\n"
	}
	data = strings.TrimSuffix(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	if data != "" {
		d.lines = strings.Split(data, "\n")
	}
	var table []string
	for i := 0; i < len(d.lines); i++ {
		line := strings.TrimSpace(d.lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inner := strings.TrimPrefix(line, "[")
			inner = strings.TrimPrefix(inner, "[")
			key, _ := parseKey(inner)
			table = key
			d.stmts = append(d.stmts, statement{start: i, end: i, header: true, key: key})
			continue
		}
		key, rest := parseKey(line)
		end := i
		state := valueState{}
		state.scan(strings.TrimPrefix(strings.TrimSpace(rest), "="))
		for state.open() && end+1 < len(d.lines) {
			end++
			state.scan(d.lines[end])
		}
		d.stmts = append(d.stmts, statement{start: i, end: end, table: table, key: key})
		i = end
	}
	return d
}

// parseKey reads a dotted key such as a."b.c".d from the start of s and
// returns its parts and what follows it.
func parseKey(s string) ([]string, string) {
	var parts []string
	for {
		s = strings.TrimLeft(s, " \t")
		switch {
		case strings.HasPrefix(s, `"`):
			v, n := basicString(s)
			parts, s = append(parts, v), s[n:]
		case strings.HasPrefix(s, "'"):
			end := strings.Index(s[1:], "'")
			if end < 0 {
				return append(parts, s[1:]), ""
			}
			parts, s = append(parts, s[1:end+1]), s[end+2:]
		default:
			n := 0
			for n < len(s) && isBare(s[n]) {
				n++
			}
			parts, s = append(parts, s[:n]), s[n:]
		}
		s = strings.TrimLeft(s, " \t")
		if !strings.HasPrefix(s, ".") {
			return parts, s
		}
		s = s[1:]
	}
}

func isBare(c byte) bool {
	return c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// basicString decodes the quoted string at the start of s and returns it
// and how many bytes it took. Escapes other than \" and \\ are kept as
// written, which is enough to compare keys.
func basicString(s string) (string, int) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			}
		case '"':
			return b.String(), i + 1
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), len(s)
}

// valueState follows a value across lines: the depth of open arrays and
// inline tables and whether a multi-line string is open.
type valueState struct {
	depth int
	// multi is the delimiter of an open multi-line string.
	multi string
}

func (v valueState) open() bool { return v.depth > 0 || v.multi != "" }

func (v *valueState) scan(line string) {
	for i := 0; i < len(line); i++ {
		if v.multi != "" {
			if v.multi == `"""` && line[i] == '\\' {
				i++
				continue
			}
			if strings.HasPrefix(line[i:], v.multi) {
				i += len(v.multi) - 1
				v.multi = ""
			}
			continue
		}
		switch c := line[i]; {
		case c == '#':
			return
		case strings.HasPrefix(line[i:], `"""`), strings.HasPrefix(line[i:], "'''"):
			v.multi = line[i : i+3]
			i += 2
		case c == '"':
			_, n := basicString(line[i:])
			i += n - 1
		case c == '\'':
			if end := strings.IndexByte(line[i+1:], '\''); end >= 0 {
				i += end + 1
			} else {
				i = len(line)
			}
		case c == '[' || c == '{':
			v.depth++
		case c == ']' || c == '}':
			v.depth--
		}
	}
}

func (d document) String() string {
	if len(d.lines) == 0 {
		return ""
	}
	return strings.Join(d.lines, d.newline) + d.newline
}

func (d *document) splice(start, end int, with []string) {
	d.lines = slices.Concat(d.lines[:start], with, d.lines[end:])
	*d = parseDocument(d.String())
}

// setRoot sets key in the root table to the TOML value text, in place if
// it is there and after the root table's last key if it is not.
func (d *document) setRoot(key, value string) {
	line := quoteKey(key) + " = " + value
	at := 0
	for _, s := range d.stmts {
		if s.header {
			break
		}
		if slices.Equal(s.key, []string{key}) {
			d.splice(s.start, s.end+1, []string{line})
			return
		}
		at = s.end + 1
	}
	d.splice(at, at, []string{line})
}

// setTable replaces the keys of the table at path with body, or appends the
// table to the end of the file if it is not there. Tables nested under it
// are left alone.
func (d *document) setTable(path []string, body []string) {
	block := append([]string{"[" + dottedKey(path) + "]"}, body...)
	for i, s := range d.stmts {
		if !s.header || !slices.Equal(s.key, path) {
			continue
		}
		end := s.end
		for _, next := range d.stmts[i+1:] {
			if next.header {
				break
			}
			end = next.end
		}
		d.splice(s.start, end+1, block)
		return
	}
	if len(d.lines) > 0 {
		block = append([]string{""}, block...)
	}
	d.splice(len(d.lines), len(d.lines), block)
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func quoteKey(k string) string {
	if bareKey.MatchString(k) {
		return k
	}
	return tomlString(k)
}

func dottedKey(path []string) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = quoteKey(p)
	}
	return strings.Join(parts, ".")
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f || r == utf8.RuneError {
				b.WriteString(`\u` + leftPad(strconv.FormatInt(int64(r), 16)))
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func leftPad(hex string) string {
	return strings.Repeat("0", max(4-len(hex), 0)) + hex
}

func tomlArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = tomlString(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// tomlInline is m as a one-line inline table, keys in order.
func tomlInline(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = quoteKey(k) + " = " + tomlString(m[k])
	}
	return "{ " + strings.Join(pairs, ", ") + " }"
}
