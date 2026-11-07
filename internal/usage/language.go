package usage

import (
	"path"
	"strings"
)

var languagesByExt = map[string]string{
	".go": "Go", ".rs": "Rust", ".zig": "Zig",
	".ts": "TypeScript", ".tsx": "TypeScript", ".mts": "TypeScript", ".cts": "TypeScript",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".py": "Python", ".ipynb": "Python",
	".php": "PHP", ".rb": "Ruby", ".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin",
	".swift": "Swift", ".m": "Objective-C", ".dart": "Dart", ".scala": "Scala",
	".c": "C", ".h": "C", ".cc": "C++", ".cpp": "C++", ".cxx": "C++", ".hpp": "C++",
	".cs": "C#", ".fs": "F#", ".ex": "Elixir", ".exs": "Elixir", ".erl": "Erlang",
	".hs": "Haskell", ".ml": "OCaml", ".clj": "Clojure", ".lua": "Lua", ".r": "R",
	".vue": "Vue", ".svelte": "Svelte", ".astro": "Astro",
	".html": "HTML", ".css": "CSS", ".scss": "CSS", ".sass": "CSS", ".less": "CSS",
	".sql": "SQL", ".graphql": "GraphQL", ".proto": "Protobuf",
	".sh": "Shell", ".bash": "Shell", ".zsh": "Shell", ".fish": "Shell", ".ps1": "PowerShell",
	".tf": "Terraform", ".hcl": "Terraform", ".nix": "Nix",
	".md": "Markdown", ".mdx": "Markdown", ".typ": "Typst", ".tex": "LaTeX",
	".json": "JSON", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".xml": "XML",
}

var languagesByName = map[string]string{
	"dockerfile": "Docker", "makefile": "Make", "justfile": "Make",
	"gemfile": "Ruby", "rakefile": "Ruby",
}

// Language names the language of a file path, or returns "" when the file
// type is not one lo0 recognises. Both slash styles are accepted, because
// transcripts keep the separator of the machine that wrote them.
func Language(p string) string {
	if p == "" {
		return ""
	}
	base := strings.ToLower(path.Base(strings.ReplaceAll(p, `\`, "/")))
	if strings.HasSuffix(base, ".blade.php") {
		return "PHP"
	}
	if lang, ok := languagesByName[base]; ok {
		return lang
	}
	if lang, ok := languagesByName[strings.TrimSuffix(base, path.Ext(base))]; ok {
		return lang
	}
	return languagesByExt[path.Ext(base)]
}
