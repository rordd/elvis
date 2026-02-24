# Task: Implement Rule Engine LLM Provider

Read existing code first: pkg/providers/types.go, pkg/providers/factory.go, pkg/providers/factory_provider.go, pkg/config/config.go, and one simple provider like pkg/providers/openai_compat/provider.go for reference.

## Create these files:

### 1. pkg/providers/ruleengine/provider.go
- Implements LLMProvider interface from types.go
- Loads rules from JSON file path
- Chat(): extract last user message, regex match against rules
- Match → extract vars, template response, return LLMResponse
- No match → return FailoverError{Reason: FailoverUnknown} for fallback chain
- GetDefaultModel() returns "ruleengine/local"

### 2. pkg/providers/ruleengine/rules.go  
- Rule struct: ID, Patterns []string, Intent, Extract map, Response template, ToolCalls, Confidence, Source
- RuleSet with sync.RWMutex thread-safe
- Match(input) returns rule + extracted vars

### 3. pkg/providers/ruleengine/logger.go
- InteractionLogger appends to interaction_log.jsonl
- Logs timestamp, user input, response, tool_calls, intent

### 4. Config: add RuleEngine to ProvidersConfig in pkg/config/config.go
- RulesFile, LogFile, AutoLearn bool

### 5. Factory: add "ruleengine" case in pkg/providers/factory.go resolveProviderSelection

### 6. Sample rules: workspace/skills/ruleengine/rules.json
Korean TV/IoT mock rules (채널, 볼륨, 전원, 에어컨, 세탁기)

### 7. Tests: pkg/providers/ruleengine/rules_test.go

### 8. Build: go generate ./cmd/picoclaw/ && go build -o picoclaw-elvis ./cmd/picoclaw

### 9. Commit: git add -A && git commit -m "feat: add rule engine LLM provider"
