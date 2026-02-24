package ruleengine

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Rule represents a single pattern-matching rule.
type Rule struct {
	ID       string            `json:"id"`
	Patterns []string          `json:"patterns"`
	Intent   string            `json:"intent"`
	Extract  map[string]string `json:"extract,omitempty"`
	Response string            `json:"response"`
	Skill    string            `json:"skill,omitempty"`
	Source   string            `json:"source,omitempty"`

	compiled []*regexp.Regexp
}

// MatchResult holds the result of a successful rule match.
type MatchResult struct {
	Rule      *Rule
	Variables map[string]string
}

// RuleSet is a thread-safe collection of rules.
type RuleSet struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewRuleSet creates an empty RuleSet.
func NewRuleSet() *RuleSet {
	return &RuleSet{}
}

// LoadFromFile loads rules from a JSON file and compiles their patterns.
func (rs *RuleSet) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading rules file: %w", err)
	}

	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("parsing rules file: %w", err)
	}

	for i := range rules {
		compiled := make([]*regexp.Regexp, 0, len(rules[i].Patterns))
		for _, p := range rules[i].Patterns {
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				return fmt.Errorf("compiling pattern %q in rule %q: %w", p, rules[i].ID, err)
			}
			compiled = append(compiled, re)
		}
		rules[i].compiled = compiled
	}

	rs.mu.Lock()
	rs.rules = rules
	rs.mu.Unlock()

	return nil
}

// Match finds the first rule whose pattern matches the input and extracts variables.
func (rs *RuleSet) Match(input string) *MatchResult {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	for i := range rs.rules {
		rule := &rs.rules[i]
		for _, re := range rule.compiled {
			matches := re.FindStringSubmatch(input)
			if matches == nil {
				continue
			}

			vars := make(map[string]string)
			names := re.SubexpNames()
			for j, name := range names {
				if j > 0 && name != "" && j < len(matches) {
					vars[name] = matches[j]
				}
			}

			// Also populate from the Extract map (static extraction patterns).
			for key, pattern := range rule.Extract {
				extRe, err := regexp.Compile("(?i)" + pattern)
				if err != nil {
					continue
				}
				extMatch := extRe.FindStringSubmatch(input)
				if len(extMatch) > 1 {
					vars[key] = extMatch[1]
				}
			}

			return &MatchResult{Rule: rule, Variables: vars}
		}
	}

	return nil
}

// TemplateResponse replaces {{key}} placeholders in the response template with extracted variables.
func TemplateResponse(template string, vars map[string]string) string {
	result := template
	for key, val := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", val)
	}
	return result
}
