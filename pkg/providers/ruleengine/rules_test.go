package ruleengine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestRules(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	data := []byte(`[
  {
    "id": "tv_channel",
    "patterns": ["채널\\s*(?P<channel>\\d+)\\s*(번|로).*(?:틀어|변경|바꿔)"],
    "intent": "tv.channel.change",
    "response": "채널을 {{channel}}번으로 변경합니다.",
    "tool_calls": [{"name": "tv_control", "arguments": {"action": "set_channel", "channel": "{{channel}}"}}],
    "confidence": 0.95,
    "source": "test"
  },
  {
    "id": "tv_volume_up",
    "patterns": ["볼륨\\s*(?:올려|높여|키워)"],
    "intent": "tv.volume.up",
    "response": "볼륨을 올립니다.",
    "confidence": 0.9,
    "source": "test"
  },
  {
    "id": "aircon_temp",
    "patterns": ["에어컨\\s*(?P<temp>\\d+)\\s*도"],
    "intent": "aircon.temp.set",
    "response": "에어컨 온도를 {{temp}}도로 설정합니다.",
    "confidence": 0.9,
    "source": "test"
  }
]`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRuleSetMatch(t *testing.T) {
	path := writeTestRules(t)
	rs := NewRuleSet()
	if err := rs.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantID    string
		wantVars  map[string]string
	}{
		{
			name:      "channel change",
			input:     "채널 5번으로 틀어줘",
			wantMatch: true,
			wantID:    "tv_channel",
			wantVars:  map[string]string{"channel": "5"},
		},
		{
			name:      "channel change two digits",
			input:     "채널 11번으로 변경해줘",
			wantMatch: true,
			wantID:    "tv_channel",
			wantVars:  map[string]string{"channel": "11"},
		},
		{
			name:      "volume up",
			input:     "볼륨 올려",
			wantMatch: true,
			wantID:    "tv_volume_up",
		},
		{
			name:      "aircon temperature",
			input:     "에어컨 24도로 맞춰",
			wantMatch: true,
			wantID:    "aircon_temp",
			wantVars:  map[string]string{"temp": "24"},
		},
		{
			name:      "no match",
			input:     "오늘 날씨 어때?",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rs.Match(tt.input)
			if tt.wantMatch {
				if result == nil {
					t.Fatal("expected match, got nil")
				}
				if result.Rule.ID != tt.wantID {
					t.Errorf("rule ID = %q, want %q", result.Rule.ID, tt.wantID)
				}
				for k, v := range tt.wantVars {
					if got := result.Variables[k]; got != v {
						t.Errorf("var %q = %q, want %q", k, got, v)
					}
				}
			} else {
				if result != nil {
					t.Fatalf("expected no match, got rule %q", result.Rule.ID)
				}
			}
		})
	}
}

func TestTemplateResponse(t *testing.T) {
	tmpl := "채널을 {{channel}}번으로 변경합니다."
	vars := map[string]string{"channel": "7"}
	got := TemplateResponse(tmpl, vars)
	want := "채널을 7번으로 변경합니다."
	if got != want {
		t.Errorf("TemplateResponse = %q, want %q", got, want)
	}
}

func TestLoadFromFile_InvalidPath(t *testing.T) {
	rs := NewRuleSet()
	err := rs.LoadFromFile("/nonexistent/path/rules.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := NewRuleSet()
	err := rs.LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromFile_InvalidPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_pattern.json")
	data := []byte(`[{"id": "bad", "patterns": ["[invalid"]}]`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	rs := NewRuleSet()
	err := rs.LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}
