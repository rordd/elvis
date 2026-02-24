// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/config"
)

// ProviderPool pre-creates and caches providers for all fallback candidates.
// Thread-safe with sync.RWMutex. Providers are lazily initialized on first Get()
// and reused for subsequent calls with the same provider/model key.
type ProviderPool struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider // key: "provider/model"
	cfg       *config.Config
}

// NewProviderPool creates a new ProviderPool backed by the given config.
func NewProviderPool(cfg *config.Config) *ProviderPool {
	return &ProviderPool{
		providers: make(map[string]LLMProvider),
		cfg:       cfg,
	}
}

// Get returns a cached provider for the given provider/model pair,
// creating and caching it on first access.
func (p *ProviderPool) Get(provider, model string) (LLMProvider, error) {
	key := provider + "/" + model

	// Fast path: read lock
	p.mu.RLock()
	if prov, ok := p.providers[key]; ok {
		p.mu.RUnlock()
		return prov, nil
	}
	p.mu.RUnlock()

	// Slow path: write lock, double-check, create
	p.mu.Lock()
	defer p.mu.Unlock()

	if prov, ok := p.providers[key]; ok {
		return prov, nil
	}

	modelCfg, err := p.cfg.GetModelConfig(key)
	if err != nil {
		return nil, fmt.Errorf("provider pool: config for %s: %w", key, err)
	}

	prov, _, err := CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, fmt.Errorf("provider pool: create %s: %w", key, err)
	}

	fmt.Printf("[pool] created provider for key=%s\n", key)
	p.providers[key] = prov
	return prov, nil
}

// Warmup pre-creates providers for all given fallback candidates.
// Errors are logged but not fatal — Get() will retry on demand.
func (p *ProviderPool) Warmup(candidates []FallbackCandidate) error {
	var firstErr error
	for _, c := range candidates {
		if _, err := p.Get(c.Provider, c.Model); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
