# Task: Implement ProviderPool for proper multi-provider fallback

## Problem
Currently, pkg/agent/loop.go uses `agent.Provider.Chat()` for ALL fallback candidates, but this always calls the same provider (e.g., ruleengine). When fallback to a different provider (e.g., gemini) is needed, it still calls ruleengine.

The current workaround in loop.go creates a new provider on every fallback call, which is wasteful.

## Solution: ProviderPool

### 1. Create pkg/providers/pool.go
```go
// ProviderPool pre-creates and caches providers for all fallback candidates.
type ProviderPool struct {
    mu        sync.RWMutex
    providers map[string]LLMProvider  // key: "provider/model"
    cfg       *config.Config
}

func NewProviderPool(cfg *config.Config) *ProviderPool
func (p *ProviderPool) Get(provider, model string) (LLMProvider, error)  // lazy-init + cache
func (p *ProviderPool) Warmup(candidates []FallbackCandidate) error      // pre-create all
```

- Key is "provider/model" string
- Get() checks cache first, creates if missing, caches for reuse
- Warmup() pre-creates all candidates at startup (optional optimization)
- Thread-safe with sync.RWMutex

### 2. Modify pkg/agent/loop.go
- Add `providerPool *providers.ProviderPool` to AgentLoop struct
- Initialize in NewAgentLoop() with config
- Call `Warmup()` for each agent's candidates at startup
- In the fallback Execute callback, use `providerPool.Get(provider, model)` instead of creating new providers

Remove the current workaround that calls `al.cfg.GetModelConfig()` and `providers.CreateProviderFromConfig()` inside the fallback callback.

### 3. Modify pkg/agent/loop.go fallback section
Change the fallback run function from the current workaround to:
```go
func(ctx context.Context, candidateProvider, candidateModel string) (*LLMResponse, error) {
    p, err := al.providerPool.Get(candidateProvider, candidateModel)
    if err != nil {
        return nil, err
    }
    return p.Chat(ctx, messages, providerToolDefs, candidateModel, map[string]any{
        "max_tokens":  agent.MaxTokens,
        "temperature": agent.Temperature,
    })
}
```

### 4. Build & test
- go build -o picoclaw-elvis ./cmd/picoclaw
- Verify no compilation errors

### 5. Git commit
- git add -A && git commit -m "refactor: add ProviderPool for clean multi-provider fallback"

## Important
- Read existing loop.go carefully before modifying
- Don't break existing single-provider behavior
- The pool should work for ANY number of providers/models
- Lazy initialization is fine (don't need to pre-create if Get() caches)
