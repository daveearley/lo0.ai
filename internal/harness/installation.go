package harness

// Installation is a harness as it is on this machine: whether it is
// installed and what each of its commands is set up to use now.
type Installation struct {
	Installed bool `json:"installed"`
	// Version is the installed version, when the harness can say.
	Version string `json:"version,omitzero"`
	Uses    []Use  `json:"uses,omitzero"`
}

// Use is one command a harness is started with and the model it reaches.
type Use struct {
	Command string `json:"command"`
	// Model describes what the command uses, such as "Claude" or
	// "Qwen3.6-35B-A3B through Ollama".
	Model string `json:"model"`
	// Local is true when the model runs on this machine.
	Local bool `json:"local,omitzero"`
}
