// Package runtimes detects local model runtimes and the models they already
// have on disk.
package runtimes

import "slices"

// ID identifies a runtime.
type ID string

// Supported runtimes.
const (
	Ollama   ID = "ollama"
	LMStudio ID = "lmstudio"
	LlamaCpp ID = "llamacpp"
)

// Name is the runtime's product name.
func (id ID) Name() string {
	switch id {
	case Ollama:
		return "Ollama"
	case LMStudio:
		return "LM Studio"
	case LlamaCpp:
		return "llama.cpp"
	default:
		return string(id)
	}
}

// Runtime is what lo0 found out about one local runtime.
type Runtime struct {
	ID        ID     `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	// Service is true for runtimes that run as a background server lo0 can
	// query, as opposed to command-line tools started per use.
	Service bool   `json:"service"`
	Running bool   `json:"running"`
	Version string `json:"version,omitzero"`
	// Endpoint is the API address the runtime answered on, if it is running.
	Endpoint string  `json:"endpoint,omitzero"`
	Models   []Model `json:"models,omitzero"`
}

// Model is one model available to a runtime.
type Model struct {
	Name         string `json:"name"`
	Family       string `json:"family,omitzero"`
	Parameters   string `json:"parameters,omitzero"`
	Quantization string `json:"quantization,omitzero"`
	Format       Format `json:"format,omitzero"`
	SizeBytes    uint64 `json:"sizeBytes,omitzero"`
	// Parent is the model this one was made from, sharing its weights, as
	// a model with its own settings made from another is. Empty for a
	// download.
	Parent string `json:"parent,omitzero"`
	// ContextLength is the longest context the model supports, in tokens.
	ContextLength int          `json:"contextLength,omitzero"`
	Capabilities  []Capability `json:"capabilities,omitzero"`
}

// Format is a model weight format.
type Format string

// Weight formats.
const (
	FormatGGUF Format = "gguf"
	FormatMLX  Format = "mlx"
)

// Capability is something a model can do beyond plain completion.
type Capability string

// Capabilities that matter for agentic coding.
const (
	CapabilityTools    Capability = "tools"
	CapabilityThinking Capability = "thinking"
	CapabilityVision   Capability = "vision"
)

// HasCapability reports whether m declares c.
func (m Model) HasCapability(c Capability) bool {
	return slices.Contains(m.Capabilities, c)
}
