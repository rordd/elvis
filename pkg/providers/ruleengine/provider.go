package ruleengine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

type (
	ToolCall       = protocoltypes.ToolCall
	FunctionCall   = protocoltypes.FunctionCall
	LLMResponse    = protocoltypes.LLMResponse
	Message        = protocoltypes.Message
	ToolDefinition = protocoltypes.ToolDefinition
)

// Provider implements the LLMProvider interface using local pattern-matching rules.
// DefaultMaxInputLen is the default max rune length for rule matching.
// Inputs longer than this are skipped (likely summarization/system text).
const DefaultMaxInputLen = 100

type Provider struct {
	ruleSet     *RuleSet
	logger      *InteractionLogger
	resolver    *SkillResolver
	maxInputLen int
}

// NewProvider creates a rule engine provider, loading rules from the given file path.
// logFile may be empty to disable logging.
// skillsDir may be empty to disable skill resolution (response-only mode).
func NewProvider(rulesFile, logFile, skillsDir string, maxInputLen int) (*Provider, error) {
	rs := NewRuleSet()
	if err := rs.LoadFromFile(rulesFile); err != nil {
		return nil, fmt.Errorf("ruleengine: %w", err)
	}

	var logger *InteractionLogger
	if logFile != "" {
		logger = NewInteractionLogger(logFile)
	}

	var resolver *SkillResolver
	if skillsDir != "" {
		resolver = NewSkillResolver(skillsDir)
	}

	if maxInputLen <= 0 {
		maxInputLen = DefaultMaxInputLen
	}

	return &Provider{
		ruleSet:     rs,
		logger:      logger,
		resolver:    resolver,
		maxInputLen: maxInputLen,
	}, nil
}

// Chat extracts the last user message and matches it against loaded rules.
// On match, it returns a templated response with skill-resolved tool calls.
// On no match, it returns a FailoverError.
func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	// If the last message is a tool result, the previous tool call already executed.
	// Find the original assistant response (natural language) from the rule match
	// and return it instead of raw tool output.
	if len(messages) > 0 && messages[len(messages)-1].Role == "tool" {
		// Look for the preceding assistant message with the natural language response.
		responseText := "완료되었습니다."
		for i := len(messages) - 2; i >= 0; i-- {
			if messages[i].Role == "assistant" && messages[i].Content != "" {
				responseText = messages[i].Content
				break
			}
		}
		return &LLMResponse{
			Content:      responseText,
			FinishReason: "stop",
		}, nil
	}

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
			Reason:  FailoverUnknown,
			Wrapped: fmt.Errorf("no user message found"),
		}
	}

	// Skip long inputs — voice/IoT commands are short. Long inputs are likely
	// summarization, heartbeat, or system-generated text that may contain
	// previously matched keywords. Configurable via max_input_length.
	if p.maxInputLen > 0 && len([]rune(userInput)) > p.maxInputLen {
		return nil, &FailoverError{
			Reason:  FailoverUnknown,
			Wrapped: fmt.Errorf("input too long (%d runes, max %d)", len([]rune(userInput)), p.maxInputLen),
		}
	}

	fmt.Printf("[ruleengine] user input: %q (len=%d)\n", userInput, len(userInput))
	result := p.ruleSet.Match(userInput)
	fmt.Printf("[ruleengine] match result: %v\n", result != nil)
	if result == nil {
		return nil, &FailoverError{
			Reason:  FailoverUnknown,
			Wrapped: fmt.Errorf("no rule matched input"),
		}
	}

	responseText := TemplateResponse(result.Rule.Response, result.Variables)

	resp := &LLMResponse{
		Content:      responseText,
		FinishReason: "stop",
	}

	// Resolve skill command and build tool call.
	if p.resolver != nil && result.Rule.Skill != "" {
		command, skillDir, err := p.resolver.Resolve(result.Rule.Skill, result.Rule.Intent, result.Variables)
		if err != nil {
			fmt.Printf("[ruleengine] skill resolve error: %v\n", err)
		} else {
			argsJSON, _ := json.Marshal(map[string]any{
				"command":     command,
				"working_dir": skillDir,
			})
			resp.ToolCalls = []ToolCall{
				{
					ID:   fmt.Sprintf("skill_%s", result.Rule.ID),
					Type: "function",
					Function: &FunctionCall{
						Name:      "exec",
						Arguments: string(argsJSON),
					},
				},
			}
			resp.FinishReason = "tool_calls"
		}
	}

	// Log the interaction.
	if p.logger != nil {
		p.logger.Log(userInput, responseText, resp.ToolCalls, result.Rule.Intent)
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
