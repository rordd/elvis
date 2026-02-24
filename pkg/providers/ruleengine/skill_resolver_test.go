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
