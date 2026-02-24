package ruleengine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

type (
	ToolCall           = protocoltypes.ToolCall
	FunctionCall       = protocoltypes.FunctionCall
	LLMResponse        = protocoltypes.LLMResponse
	Message            = protocoltypes.Message
	ToolDefinition     = protocoltypes.ToolDefinition
)

// Provider implements the LLMProvider interface using local pattern-matching rules.
type Provider struct {
	ruleSet *RuleSet
	logger  *InteractionLogger
}

// NewProvider creates a rule engine provider, loading rules from the given file path.
// logFile may be empty to disable logging.
func NewProvider(rulesFile, logFile string) (*Provider, error) {
	rs := NewRuleSet()
	if err := rs.LoadFromFile(rulesFile); err != nil {
		return nil, fmt.Errorf("ruleengine: %w", err)
	}

	var logger *InteractionLogger
	if logFile != "" {
		logger = NewInteractionLogger(logFile)
	}

	return &Provider{
		ruleSet: rs,
		logger:  logger,
	}, nil
}

// Chat extracts the last user message and matches it against loaded rules.
// On match, it returns a templated response. On no match, it returns a FailoverError.
func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	// Extract last user message.
	var userInput string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userInput = messages[i].Content
			break
		}
	}

	if userInput == "" {
		return nil, &FailoverError{
			Reason: FailoverUnknown,
			Wrapped: fmt.Errorf("no user message found"),
		}
	}

	result := p.ruleSet.Match(userInput)
	if result == nil {
		return nil, &FailoverError{
			Reason: FailoverUnknown,
			Wrapped: fmt.Errorf("no rule matched input"),
		}
	}

	responseText := TemplateResponse(result.Rule.Response, result.Variables)

	// Build tool calls if the rule defines them.
	var toolCalls []ToolCall
	for i, rtc := range result.Rule.ToolCalls {
		// Template variable substitution in tool call arguments.
		args := make(map[string]any, len(rtc.Arguments))
		for k, v := range rtc.Arguments {
			if s, ok := v.(string); ok {
				args[k] = TemplateResponse(s, result.Variables)
			} else {
				args[k] = v
			}
		}

		argsJSON, _ := json.Marshal(args)
		toolCalls = append(toolCalls, ToolCall{
			ID:   fmt.Sprintf("rule_%s_%d", result.Rule.ID, i),
			Name: rtc.Name,
			Arguments: args,
			Function: &FunctionCall{
				Name:      rtc.Name,
				Arguments: string(argsJSON),
			},
		})
	}

	resp := &LLMResponse{
		Content:      responseText,
		ToolCalls:    toolCalls,
		FinishReason: "stop",
	}

	// Log the interaction.
	if p.logger != nil {
		p.logger.Log(userInput, responseText, toolCalls, result.Rule.Intent)
	}

	return resp, nil
}

// GetDefaultModel returns the default model identifier for the rule engine.
func (p *Provider) GetDefaultModel() string {
	return "ruleengine/local"
}

// FailoverReason classifies why a request failed.
type FailoverReason string

const (
	FailoverUnknown FailoverReason = "unknown"
)

// FailoverError signals that the rule engine could not handle the request.
type FailoverError struct {
	Reason  FailoverReason
	Wrapped error
}

func (e *FailoverError) Error() string {
	return fmt.Sprintf("ruleengine failover(%s): %v", e.Reason, e.Wrapped)
}

func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}
