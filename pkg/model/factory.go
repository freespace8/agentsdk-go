package model

import (
	"context"
	"fmt"
	"sync"
)

// Factory holds the registered Provider implementations and creates models
// on demand.
type Factory struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewFactory constructs a factory seeded with the provided providers.
func NewFactory(providers ...Provider) *Factory {
	f := &Factory{
		providers: make(map[string]Provider, len(providers)),
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		f.providers[p.Name()] = p
	}
	return f
}

// Register attaches or replaces a Provider implementation.
func (f *Factory) Register(p Provider) {
	if p == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.providers == nil {
		f.providers = map[string]Provider{}
	}
	f.providers[p.Name()] = p
}

// NewModel builds a model instance through the provider declared in cfg.
func (f *Factory) NewModel(ctx context.Context, cfg ModelConfig) (Model, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("model provider not specified")
	}

	f.mu.RLock()
	provider := f.providers[cfg.Provider]
	f.mu.RUnlock()
	if provider == nil {
		return nil, fmt.Errorf("model provider %q is not registered", cfg.Provider)
	}

	return provider.NewModel(ctx, cfg)
}
