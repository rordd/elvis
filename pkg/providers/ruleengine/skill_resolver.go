package ruleengine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SkillAction represents a single action parsed from a SKILL.md @actions table.
type SkillAction struct {
	Intent      string
	Command     string
	Description string
}

// SkillResolver loads and caches @actions tables from SKILL.md files.
type SkillResolver struct {
	mu        sync.RWMutex
	skillsDir string
	// cache maps skill name -> (intent -> SkillAction)
	cache map[string]map[string]*SkillAction
}

// NewSkillResolver creates a resolver that reads skills from the given directory.
func NewSkillResolver(skillsDir string) *SkillResolver {
	return &SkillResolver{
		skillsDir: skillsDir,
		cache:     make(map[string]map[string]*SkillAction),
	}
}

// Resolve finds the command for the given skill and intent, substituting variables.
// Returns the fully resolved command string and the skill directory path, or an error.
func (sr *SkillResolver) Resolve(skill, intent string, vars map[string]string) (command string, skillDir string, err error) {
	actions, err := sr.loadSkill(skill)
	if err != nil {
		return "", "", err
	}

	action, ok := actions[intent]
	if !ok {
		return "", "", fmt.Errorf("intent %q not found in skill %q", intent, skill)
	}

	cmd := action.Command
	for key, val := range vars {
		cmd = strings.ReplaceAll(cmd, "{{"+key+"}}", val)
	}

	return cmd, filepath.Join(sr.skillsDir, skill), nil
}

// loadSkill returns cached actions for a skill, parsing SKILL.md on first access.
func (sr *SkillResolver) loadSkill(skill string) (map[string]*SkillAction, error) {
	sr.mu.RLock()
	actions, ok := sr.cache[skill]
	sr.mu.RUnlock()
	if ok {
		return actions, nil
	}

	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Double-check after acquiring write lock.
	if actions, ok := sr.cache[skill]; ok {
		return actions, nil
	}

	skillMD := filepath.Join(sr.skillsDir, skill, "SKILL.md")
	actions, err := parseActionsTable(skillMD)
	if err != nil {
		return nil, fmt.Errorf("loading skill %q: %w", skill, err)
	}

	sr.cache[skill] = actions
	return actions, nil
}

// parseActionsTable reads a SKILL.md file and extracts the @actions table.
// It expects markdown table rows between <!-- @actions --> and <!-- @end --> markers.
func parseActionsTable(path string) (map[string]*SkillAction, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	actions := make(map[string]*SkillAction)
	scanner := bufio.NewScanner(f)
	inActions := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "<!-- @actions -->" {
			inActions = true
			continue
		}
		if line == "<!-- @end -->" {
			break
		}
		if !inActions {
			continue
		}

		// Skip header and separator rows.
		if strings.HasPrefix(line, "| intent") || strings.HasPrefix(line, "|---") {
			continue
		}

		// Parse table row: | intent | command | params | description |
		if !strings.HasPrefix(line, "|") {
			continue
		}

		cols := strings.Split(line, "|")
		// cols[0] is empty (before first |), cols[1..4] are the columns, cols[5] is empty (after last |)
		if len(cols) < 5 {
			continue
		}

		intent := strings.TrimSpace(cols[1])
		command := strings.TrimSpace(cols[2])
		description := strings.TrimSpace(cols[4])

		if intent == "" || command == "" {
			continue
		}

		actions[intent] = &SkillAction{
			Intent:      intent,
			Command:     command,
			Description: description,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(actions) == 0 {
		return nil, fmt.Errorf("no actions found in %s", path)
	}

	return actions, nil
}
