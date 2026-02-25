package ruleengine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestSkill(t *testing.T, dir, skillName string) {
	t.Helper()
	skillDir := filepath.Join(dir, skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := `# Test Skill

## Actions
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| tv.channel.set | ./set_channel.sh {{channel}} | channel:number | 채널 변경 |
| tv.volume.up | ./volume.sh up | | 볼륨 올리기 |
| tv.volume.down | ./volume.sh down | | 볼륨 내리기 |
<!-- @end -->
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillResolver_Resolve(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "tv-control")

	sr := NewSkillResolver(dir)

	tests := []struct {
		name    string
		skill   string
		intent  string
		vars    map[string]string
		wantCmd string
		wantErr bool
	}{
		{
			name:    "channel set with variable",
			skill:   "tv-control",
			intent:  "tv.channel.set",
			vars:    map[string]string{"channel": "5"},
			wantCmd: "./set_channel.sh 5",
		},
		{
			name:    "volume up no variables",
			skill:   "tv-control",
			intent:  "tv.volume.up",
			vars:    nil,
			wantCmd: "./volume.sh up",
		},
		{
			name:    "unknown intent",
			skill:   "tv-control",
			intent:  "tv.nonexistent",
			vars:    nil,
			wantErr: true,
		},
		{
			name:    "unknown skill",
			skill:   "nonexistent-skill",
			intent:  "foo.bar",
			vars:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := sr.Resolve(tt.skill, tt.intent, tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestSkillResolver_SkillDir(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "tv-control")

	sr := NewSkillResolver(dir)
	_, skillDir, err := sr.Resolve("tv-control", "tv.volume.up", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "tv-control")
	if skillDir != want {
		t.Errorf("skillDir = %q, want %q", skillDir, want)
	}
}

func TestSkillResolver_Caching(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "tv-control")

	sr := NewSkillResolver(dir)

	// First call loads from disk.
	cmd1, _, err := sr.Resolve("tv-control", "tv.volume.up", nil)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	// Second call should use cache (same result).
	cmd2, _, err := sr.Resolve("tv-control", "tv.volume.up", nil)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if cmd1 != cmd2 {
		t.Errorf("cached result differs: %q vs %q", cmd1, cmd2)
	}
}

func TestParseActionsTable_NoActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("# Empty skill\nNo actions here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseActionsTable(path)
	if err == nil {
		t.Fatal("expected error for skill with no actions")
	}
}

// TestParseActionsTable_FullParsing verifies that the @actions table parser
// correctly extracts intent, command, and description from SKILL.md.
func TestParseActionsTable_FullParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `# IoT Controller

## Actions
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| tv.channel.set | ./set_channel.sh {{channel}} | channel:number | 채널 변경 |
| tv.volume.up | ./volume.sh up | | 볼륨 올리기 |
| tv.power.off | ./power.sh off | | 전원 끄기 |
<!-- @end -->
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, err := parseActionsTable(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("actions count = %d, want 3", len(actions))
	}

	tests := []struct {
		intent      string
		wantCmd     string
		wantDesc    string
	}{
		{"tv.channel.set", "./set_channel.sh {{channel}}", "채널 변경"},
		{"tv.volume.up", "./volume.sh up", "볼륨 올리기"},
		{"tv.power.off", "./power.sh off", "전원 끄기"},
	}

	for _, tt := range tests {
		a, ok := actions[tt.intent]
		if !ok {
			t.Errorf("intent %q not found", tt.intent)
			continue
		}
		if a.Command != tt.wantCmd {
			t.Errorf("intent %q command = %q, want %q", tt.intent, a.Command, tt.wantCmd)
		}
		if a.Description != tt.wantDesc {
			t.Errorf("intent %q description = %q, want %q", tt.intent, a.Description, tt.wantDesc)
		}
	}
}

// TestSkillResolver_EndToEnd_ActionsToExecCommand verifies the full flow:
// SKILL.md @actions table → intent resolution → variable substitution → exec command.
func TestSkillResolver_EndToEnd_ActionsToExecCommand(t *testing.T) {
	dir := t.TempDir()

	// Create a multi-action skill with variable placeholders.
	skillDir := filepath.Join(dir, "home-control")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := `# Home Control

<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| light.set | ./light.sh {{room}} {{brightness}} | room:string,brightness:number | 조명 설정 |
| aircon.temp | ./aircon.sh temp {{temp}} | temp:number | 에어컨 온도 |
<!-- @end -->
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	sr := NewSkillResolver(dir)

	tests := []struct {
		name    string
		intent  string
		vars    map[string]string
		wantCmd string
		wantDir string
	}{
		{
			name:    "multiple variables substituted",
			intent:  "light.set",
			vars:    map[string]string{"room": "bedroom", "brightness": "80"},
			wantCmd: "./light.sh bedroom 80",
			wantDir: filepath.Join(dir, "home-control"),
		},
		{
			name:    "single variable substituted",
			intent:  "aircon.temp",
			vars:    map[string]string{"temp": "24"},
			wantCmd: "./aircon.sh temp 24",
			wantDir: filepath.Join(dir, "home-control"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, skillDir, err := sr.Resolve("home-control", tt.intent, tt.vars)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd, tt.wantCmd)
			}
			if skillDir != tt.wantDir {
				t.Errorf("skillDir = %q, want %q", skillDir, tt.wantDir)
			}
		})
	}
}

// TestParseActionsTable_MalformedRows verifies that malformed table rows
// (missing columns, empty intent/command) are silently skipped.
func TestParseActionsTable_MalformedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `# Test
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| valid.intent | ./run.sh | | 유효한 행 |
| | ./no_intent.sh | | intent 없음 |
| no.command | | | command 없음 |
| short
<!-- @end -->
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, err := parseActionsTable(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only "valid.intent" should be parsed; others are malformed.
	if len(actions) != 1 {
		t.Errorf("actions count = %d, want 1 (only valid row)", len(actions))
	}
	if _, ok := actions["valid.intent"]; !ok {
		t.Error("expected valid.intent to be parsed")
	}
}
