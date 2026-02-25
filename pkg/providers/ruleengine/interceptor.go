package ruleengine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Interceptor provides a simple pre-LLM check against the rule engine.
// If the input matches a rule, it executes the skill command and returns the response.
// If no match, returns empty string so the caller proceeds to LLM.
type Interceptor struct {
	ruleSet     *RuleSet
	resolver    *SkillResolver
	maxInputLen int
}

// NewInterceptor creates a rule engine interceptor from config values.
func NewInterceptor(rulesFile, skillsDir string, maxInputLen int) (*Interceptor, error) {
	rs := &RuleSet{}
	if err := rs.LoadFromFile(rulesFile); err != nil {
		return nil, fmt.Errorf("ruleengine interceptor: %w", err)
	}

	var resolver *SkillResolver
	if skillsDir != "" {
		resolver = NewSkillResolver(skillsDir)
	}

	return &Interceptor{
		ruleSet:     rs,
		resolver:    resolver,
		maxInputLen: maxInputLen,
	}, nil
}

// TryMatch checks input against rules. Returns (response, nil) on match,
// ("", nil) on no match so caller proceeds to LLM.
func (i *Interceptor) TryMatch(ctx context.Context, input string) (string, error) {
	if len([]rune(input)) > i.maxInputLen {
		return "", nil
	}

	result := i.ruleSet.Match(input)
	if result == nil {
		return "", nil
	}

	fmt.Printf("[ruleengine-intercept] matched: %s\n", result.Rule.ID)

	response := result.Rule.Response
	for k, v := range result.Variables {
		response = strings.ReplaceAll(response, "{{"+k+"}}", v)
	}

	// Execute skill command if resolver available
	if i.resolver != nil && result.Rule.Skill != "" {
		command, skillDir, err := i.resolver.Resolve(result.Rule.Skill, result.Rule.Intent, result.Variables)
		if err == nil && command != "" {
			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Dir = skillDir
			if output, execErr := cmd.CombinedOutput(); execErr != nil {
				fmt.Printf("[ruleengine-intercept] exec error: %v, output: %s\n", execErr, string(output))
			}
		}
	}

	return response, nil
}
