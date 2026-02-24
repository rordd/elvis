package providers

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/providers/ruleengine"
)

// RuleEngineProvider wraps the ruleengine sub-package provider.
type RuleEngineProvider struct {
	delegate *ruleengine.Provider
}

// NewRuleEngineProvider creates a rule engine provider loading rules from rulesFile.
// skillsDir may be empty to disable skill resolution.
func NewRuleEngineProvider(rulesFile, logFile, skillsDir string) (*RuleEngineProvider, error) {
	p, err := ruleengine.NewProvider(rulesFile, logFile, skillsDir)
	if err != nil {
		return nil, err
	}
	return &RuleEngineProvider{delegate: p}, nil
}

func (p *RuleEngineProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	return p.delegate.Chat(ctx, messages, tools, model, options)
}

func (p *RuleEngineProvider) GetDefaultModel() string {
	return p.delegate.GetDefaultModel()
}
