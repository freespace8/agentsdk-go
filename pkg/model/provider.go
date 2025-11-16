package model

import "context"

// Provider constructs concrete Model implementations for a specific backend
// such as Anthropic or OpenAI.
type Provider interface {
	Name() string
	NewModel(ctx context.Context, cfg ModelConfig) (Model, error)
}

// ModelConfig captures the minimal settings required to build a Model
// instance. Extra can be used for provider-specific tweaks without bloating
// the common surface.
type ModelConfig struct {
	Name     string
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	Headers  map[string]string
	Extra    map[string]any
}
