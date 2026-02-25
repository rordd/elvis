package ruleengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Integration tests for the RuleEngine Provider.
// These verify end-to-end behavior: rule loading, matching, templating,
// tool result loop prevention, and input length gating.
// ---------------------------------------------------------------------------

// writeTestRulesFile creates a temporary rules.json with test rules.
func writeTestRulesFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.json")

	rules := []map[string]any{
		{
			"id":       "tv_channel_set",
			"patterns": []string{`채널\s*(?P<channel>\d+)\s*(번|로).*(?:틀어|변경|바꿔)`},
			"intent":   "tv.channel.set",
			"skill":    "tv-control",
			"response": "채널을 {{channel}}번으로 변경합니다.",
		},
		{
			"id":       "greeting",
			"patterns": []string{`^(안녕|하이|헬로)`},
			"intent":   "general.greeting",
			"response": "안녕하세요! 무엇을 도와드릴까요?",
		},
		{
			"id":       "volume_set",
			"patterns": []string{`볼륨\s*(?P<level>\d+)`},
			"intent":   "tv.volume.set",
			"response": "볼륨을 {{level}}(으)로 설정합니다.",
		},
	}

	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return rulesPath
}

// writeTestSkillDir creates a tv-control skill with SKILL.md in a temp dir.
func writeTestSkillDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "tv-control")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := `# TV Control

## Actions
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| tv.channel.set | ./set_channel.sh {{channel}} | channel:number | 채널 변경 |
| tv.volume.up | ./volume.sh up | | 볼륨 올리기 |
<!-- @end -->
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestIntegration_ToolResultLoopPrevention verifies that when the last
// message has role="tool", the provider returns the preceding assistant's
// natural language response instead of re-matching (infinite loop prevention).
func TestIntegration_ToolResultLoopPrevention(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	p, err := NewProvider(rulesPath, "", "", 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "채널 5번으로 틀어"},
		{Role: "assistant", Content: "채널을 5번으로 변경합니다."},
		{Role: "tool", Content: `{"status":"ok"}`, ToolCallID: "skill_tv_channel_set"},
	}

	resp, err := p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "채널을 5번으로 변경합니다." {
		t.Errorf("content = %q, want previous assistant response", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want stop", resp.FinishReason)
	}
	// Must NOT have tool calls (that would re-trigger the loop).
	if len(resp.ToolCalls) > 0 {
		t.Error("tool result response should NOT contain new tool calls")
	}
}

// TestIntegration_ToolResultFallbackDefault verifies that when the last
// message is role="tool" but no preceding assistant message is found,
// a default response is returned.
func TestIntegration_ToolResultFallbackDefault(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	p, err := NewProvider(rulesPath, "", "", 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	messages := []Message{
		{Role: "tool", Content: `{"status":"ok"}`, ToolCallID: "skill_test"},
	}

	resp, err := p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "완료되었습니다." {
		t.Errorf("content = %q, want default response", resp.Content)
	}
}

// TestIntegration_MaxInputLength_ExceedsLimit verifies that inputs longer
// than max_input_length are skipped with a FailoverError.
func TestIntegration_MaxInputLength_ExceedsLimit(t *testing.T) {
	rulesPath := writeTestRulesFile(t)

	// Use a strict limit of 10 runes.
	p, err := NewProvider(rulesPath, "", "", 10)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	// This input is 15 Korean characters — exceeds 10 rune limit.
	longInput := "이것은 매우 긴 입력입니다 채널 5번으로 변경해"
	messages := []Message{
		{Role: "user", Content: longInput},
	}

	_, err = p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err == nil {
		t.Fatal("expected FailoverError for long input, got nil")
	}

	var failErr *FailoverError
	if ok := isRuleEngineFailoverError(err, &failErr); !ok {
		t.Fatalf("expected *FailoverError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "input too long") {
		t.Errorf("error message = %q, should contain 'input too long'", err.Error())
	}
}

// TestIntegration_MaxInputLength_WithinLimit verifies that inputs within
// the max_input_length limit are matched normally.
func TestIntegration_MaxInputLength_WithinLimit(t *testing.T) {
	rulesPath := writeTestRulesFile(t)

	// Generous limit.
	p, err := NewProvider(rulesPath, "", "", 200)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "안녕"},
	}

	resp, err := p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "안녕하세요! 무엇을 도와드릴까요?" {
		t.Errorf("content = %q, want greeting response", resp.Content)
	}
}

// TestIntegration_VariableCaptureAndTemplate verifies end-to-end variable
// extraction from named capture groups and template substitution.
func TestIntegration_VariableCaptureAndTemplate(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	p, err := NewProvider(rulesPath, "", "", 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		wantResp string
	}{
		{
			name:     "채널 5번",
			input:    "채널 5번으로 틀어",
			wantResp: "채널을 5번으로 변경합니다.",
		},
		{
			name:     "채널 123번",
			input:    "채널 123번으로 변경해",
			wantResp: "채널을 123번으로 변경합니다.",
		},
		{
			name:     "볼륨 50",
			input:    "볼륨 50",
			wantResp: "볼륨을 50(으)로 설정합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []Message{
				{Role: "user", Content: tt.input},
			}
			resp, err := p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Content != tt.wantResp {
				t.Errorf("content = %q, want %q", resp.Content, tt.wantResp)
			}
		})
	}
}

// TestIntegration_NoMatch_ReturnsFailoverError verifies that an unmatched
// input returns a FailoverError with reason "unknown".
func TestIntegration_NoMatch_ReturnsFailoverError(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	p, err := NewProvider(rulesPath, "", "", 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "이 문장은 아무 규칙에도 매칭 안 됨"},
	}

	_, err = p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err == nil {
		t.Fatal("expected FailoverError for unmatched input")
	}

	var failErr *FailoverError
	if !isRuleEngineFailoverError(err, &failErr) {
		t.Fatalf("expected *FailoverError, got %T: %v", err, err)
	}
	if failErr.Reason != FailoverUnknown {
		t.Errorf("reason = %q, want %q", failErr.Reason, FailoverUnknown)
	}
}

// TestIntegration_SkillResolution_ToolCall verifies that a matched rule
// with a skill field produces a tool call with the resolved command.
func TestIntegration_SkillResolution_ToolCall(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	skillsDir := writeTestSkillDir(t)

	p, err := NewProvider(rulesPath, "", skillsDir, 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "채널 7번으로 바꿔"},
	}

	resp, err := p.Chat(context.Background(), messages, nil, "ruleengine/local", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have the templated response text.
	if resp.Content != "채널을 7번으로 변경합니다." {
		t.Errorf("content = %q, want 채널을 7번으로 변경합니다.", resp.Content)
	}

	// Should have tool_calls finish reason.
	if resp.FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", resp.FinishReason)
	}

	// Should contain exactly one tool call.
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.Function == nil {
		t.Fatal("tool call function is nil")
	}
	if tc.Function.Name != "exec" {
		t.Errorf("function name = %q, want exec", tc.Function.Name)
	}

	// Parse arguments and verify command.
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("unmarshal tool call args: %v", err)
	}
	cmd, ok := args["command"].(string)
	if !ok {
		t.Fatal("command not found in tool call args")
	}
	if cmd != "./set_channel.sh 7" {
		t.Errorf("command = %q, want ./set_channel.sh 7", cmd)
	}

	wdir, ok := args["working_dir"].(string)
	if !ok {
		t.Fatal("working_dir not found in tool call args")
	}
	if !strings.HasSuffix(wdir, "tv-control") {
		t.Errorf("working_dir = %q, should end with tv-control", wdir)
	}
}

// TestIntegration_EmptyMessages_ReturnsFailoverError verifies that an
// empty message list returns a FailoverError.
func TestIntegration_EmptyMessages_ReturnsFailoverError(t *testing.T) {
	rulesPath := writeTestRulesFile(t)
	p, err := NewProvider(rulesPath, "", "", 0)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = p.Chat(context.Background(), nil, nil, "ruleengine/local", nil)
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
	var failErr *FailoverError
	if !isRuleEngineFailoverError(err, &failErr) {
		t.Fatalf("expected *FailoverError, got %T", err)
	}
}

// isRuleEngineFailoverError is a test helper that checks if err is a
// ruleengine FailoverError.
func isRuleEngineFailoverError(err error, target **FailoverError) bool {
	var fe *FailoverError
	if ok := errors.As(err, &fe); ok {
		if target != nil {
			*target = fe
		}
		return true
	}
	return false
}
