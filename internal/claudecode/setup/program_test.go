package setup

import "testing"

func TestProgramNameKeepsOnlyTheProgram(t *testing.T) {
	tests := map[string]string{
		"npx":                                "npx",
		"/usr/local/bin/uvx":                 "uvx",
		`C:\Program Files\nodejs\node.exe`:   "node.exe",
		`C:\tools\srv.exe --key=SECRET_K`:    "srv.exe",
		"API_KEY=SECRET_TOKEN_123 npx":       "npx",
		"npx -y @scope/pkg --token=SECRET_T": "npx",
		"docker run -e TOKEN=SECRET_T image": "docker",
		"SECRET_TOKEN=abc":                   "",
		"":                                   "",
		"./bin/serve:SECRET_PART":            "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := programName(in); got != want {
				t.Errorf("programName(%q) = %q, want %q", in, got, want)
			}
		})
	}
}
