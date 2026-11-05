package setup

import (
	"bufio"
	"bytes"
	"encoding/json"
	"maps"
	"path"
	"slices"
	"strings"
)

type mcpServerConfig struct {
	Type    string         `json:"type"`
	Command string         `json:"command"`
	URL     string         `json:"url"`
	Env     map[string]any `json:"env"`
	Headers map[string]any `json:"headers"`
}

// mcpServers converts a name-to-config map into MCPServers, keeping only the
// shape of each server.
func mcpServers(origin Origin, configs map[string]mcpServerConfig) []MCPServer {
	servers := make([]MCPServer, 0, len(configs))
	for _, name := range slices.Sorted(maps.Keys(configs)) {
		c := configs[name]
		s := MCPServer{
			Origin:     origin,
			Name:       name,
			Transport:  transport(c),
			EnvKeys:    slices.Sorted(maps.Keys(c.Env)),
			HeaderKeys: slices.Sorted(maps.Keys(c.Headers)),
		}
		s.Command = programName(c.Command)
		servers = append(servers, s)
	}
	return servers
}

func transport(c mcpServerConfig) Transport {
	switch strings.ToLower(c.Type) {
	case "sse":
		return TransportSSE
	case "http", "streamable-http":
		return TransportHTTP
	case "stdio":
		return TransportStdio
	}
	if c.Command == "" && c.URL != "" {
		return TransportHTTP
	}
	return TransportStdio
}

type hookMatchers map[string][]struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type string `json:"type"`
	} `json:"hooks"`
}

func hooks(origin Origin, events hookMatchers) []Hook {
	var out []Hook
	for _, event := range slices.Sorted(maps.Keys(events)) {
		for _, m := range events[event] {
			for _, h := range m.Hooks {
				out = append(out, Hook{Origin: origin, Event: event, Matcher: m.Matcher, Type: h.Type})
			}
		}
	}
	return out
}

type settingsDoc struct {
	Permissions *struct {
		Allow       []string `json:"allow"`
		Deny        []string `json:"deny"`
		Ask         []string `json:"ask"`
		DefaultMode string   `json:"defaultMode"`
	} `json:"permissions"`
	Hooks          hookMatchers    `json:"hooks"`
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

type parsedSettings struct {
	file        SettingsFile
	permissions *Permissions
	hooks       []Hook
	plugins     map[string]bool
}

func parseSettings(origin Origin, data []byte) (parsedSettings, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return parsedSettings{}, err
	}
	var doc settingsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return parsedSettings{}, err
	}
	p := parsedSettings{
		file:    SettingsFile{Origin: origin, Keys: slices.Sorted(maps.Keys(keys))},
		hooks:   hooks(origin, doc.Hooks),
		plugins: doc.EnabledPlugins,
	}
	if doc.Permissions != nil {
		p.permissions = &Permissions{
			Origin:      origin,
			Allow:       len(doc.Permissions.Allow),
			Deny:        len(doc.Permissions.Deny),
			Ask:         len(doc.Permissions.Ask),
			DefaultMode: doc.Permissions.DefaultMode,
		}
	}
	return p, nil
}

// frontmatterName returns the name field of a Markdown file's YAML
// frontmatter, or "" if there is none.
func frontmatterName(data []byte) string {
	s := bufio.NewScanner(bytes.NewReader(data))
	if !s.Scan() || strings.TrimSpace(s.Text()) != "---" {
		return ""
	}
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// programName is the base name of the program a stdio server runs, such as
// "npx". The program belongs in command and its arguments in args, but a
// hand-written config can put arguments, even KEY=value pairs, in command
// too, and those can be secrets. Only the program's name is kept, and a
// name with anything but letters, digits and ._+- is dropped.
func programName(command string) string {
	fields := strings.Fields(strings.ReplaceAll(command, `\`, "/"))
	for len(fields) > 0 && strings.Contains(fields[0], "=") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	// A Windows path such as C:/Program Files/nodejs/node.exe is one
	// program split at its spaces.
	program := fields[0]
	for _, f := range fields[1:] {
		if !strings.Contains(program, "/") || !strings.Contains(f, "/") || strings.HasPrefix(f, "-") {
			break
		}
		program += " " + f
	}
	name := path.Base(program)
	if name == "" || strings.ContainsFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("._+-", r)
	}) {
		return ""
	}
	return name
}
