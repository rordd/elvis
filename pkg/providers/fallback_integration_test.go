package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Integration tests: ruleengine ↔ FallbackChain interactions.
// These verify the end-to-end fallback path when a ruleengine provider is
// included as a candidate together with an LLM provider.
// ---------------------------------------------------------------------------

// simulateRuleEngineMatch returns a run func that succeeds on "ruleengine"
// with canned content, and succeeds on any other provider with LLM content.
func simulateRuleEngineMatch(ruleContent, llmContent string) func(ctx context.Context, provider, model string) (*LLMResponse, error) {
	return func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		if provider == "ruleengine" {
			return &LLMResponse{Content: ruleContent, FinishReason: "stop"}, nil
		}
		return &LLMResponse{Content: llmContent, FinishReason: "stop"}, nil
	}
}

// simulateRuleEngineNoMatch returns a run func that returns a ruleengine
// failover error on "ruleengine" and succeeds on any other provider.
func simulateRuleEngineNoMatch(llmContent string) func(ctx context.Context, provider, model string) (*LLMResponse, error) {
	return func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		if provider == "ruleengine" {
			return nil, fmt.Errorf("ruleengine failover(unknown): no rule matched input")
		}
		return &LLMResponse{Content: llmContent, FinishReason: "stop"}, nil
	}
}

// TestIntegration_RuleEngineMatch_ReturnsWithoutFallback verifies that when
// the ruleengine matches, its response is returned and no subsequent
// provider is called.
func TestIntegration_RuleEngineMatch_ReturnsWithoutFallback(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	called := map[string]bool{}
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		called[provider] = true
		if provider == "ruleengine" {
			return &LLMResponse{Content: "채널을 5번으로 변경합니다.", FinishReason: "stop"}, nil
		}
		return &LLMResponse{Content: "LLM response", FinishReason: "stop"}, nil
	}

	result, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "ruleengine" {
		t.Errorf("provider = %q, want ruleengine", result.Provider)
	}
	if result.Response.Content != "채널을 5번으로 변경합니다." {
		t.Errorf("content = %q, want ruleengine response", result.Response.Content)
	}
	if called["openai"] {
		t.Error("openai should NOT have been called when ruleengine matched")
	}
}

// TestIntegration_RuleEngineNoMatch_FallsBackToNextCandidate verifies that
// a ruleengine "no match" error (FailoverError) causes fallback to the
// next candidate.
func TestIntegration_RuleEngineNoMatch_FallsBackToNextCandidate(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	run := simulateRuleEngineNoMatch("LLM fallback response")

	result, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "openai" {
		t.Errorf("provider = %q, want openai (fallback)", result.Provider)
	}
	if result.Response.Content != "LLM fallback response" {
		t.Errorf("content = %q, want LLM fallback response", result.Response.Content)
	}
	// One failed attempt from ruleengine should be recorded.
	if len(result.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (ruleengine fail recorded)", len(result.Attempts))
	}
}

// TestIntegration_RuleEngineNoMatch_NoCooldownPenalty verifies that
// ruleengine "no match" does NOT trigger MarkFailure on the cooldown
// tracker. The ruleengine should remain immediately available.
func TestIntegration_RuleEngineNoMatch_NoCooldownPenalty(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	run := simulateRuleEngineNoMatch("LLM response")

	_, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ruleengine should NOT be in cooldown.
	if !ct.IsAvailable("ruleengine") {
		t.Error("ruleengine should remain available (no cooldown penalty)")
	}
	if ct.ErrorCount("ruleengine") != 0 {
		t.Errorf("ruleengine error count = %d, want 0", ct.ErrorCount("ruleengine"))
	}
}

// TestIntegration_ConsecutiveRuleEngineFailures_StillTriedNext verifies
// that even after multiple consecutive ruleengine no-match failures,
// the ruleengine is still tried on subsequent requests (no cooldown
// accumulation).
func TestIntegration_ConsecutiveRuleEngineFailures_StillTriedNext(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	noMatchRun := simulateRuleEngineNoMatch("LLM response")

	// Simulate 5 consecutive no-match failures.
	for i := 0; i < 5; i++ {
		_, err := fc.Execute(context.Background(), candidates, noMatchRun)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
	}

	// ruleengine must still be available.
	if !ct.IsAvailable("ruleengine") {
		t.Error("ruleengine should remain available after consecutive no-match failures")
	}

	// Now if ruleengine matches, it should succeed.
	ruleEngineAttempted := false
	matchRun := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		if provider == "ruleengine" {
			ruleEngineAttempted = true
			return &LLMResponse{Content: "rule matched!", FinishReason: "stop"}, nil
		}
		return &LLMResponse{Content: "LLM", FinishReason: "stop"}, nil
	}

	result, err := fc.Execute(context.Background(), candidates, matchRun)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ruleEngineAttempted {
		t.Error("ruleengine should have been attempted on next request")
	}
	if result.Provider != "ruleengine" {
		t.Errorf("provider = %q, want ruleengine", result.Provider)
	}
}

// TestIntegration_RuleEngineNoMatch_LLMAlsoFails_ExhaustedError verifies
// that when both ruleengine and LLM fail, a FallbackExhaustedError is returned.
func TestIntegration_RuleEngineNoMatch_LLMAlsoFails_ExhaustedError(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		if provider == "ruleengine" {
			return nil, fmt.Errorf("ruleengine failover(unknown): no rule matched input")
		}
		return nil, errors.New("rate limit exceeded")
	}

	_, err := fc.Execute(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error when all candidates fail")
	}

	var exhausted *FallbackExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("expected FallbackExhaustedError, got %T: %v", err, err)
	}
	if len(exhausted.Attempts) != 2 {
		t.Errorf("attempts = %d, want 2", len(exhausted.Attempts))
	}
}

// TestIntegration_LLMCooldownButRuleEngineAvailable verifies that when the
// LLM provider is in cooldown, ruleengine is still tried (and can succeed).
func TestIntegration_LLMCooldownButRuleEngineAvailable(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	// Put openai in cooldown.
	ct.MarkFailure("openai", FailoverRateLimit)

	candidates := []FallbackCandidate{
		makeCandidate("ruleengine", "local"),
		makeCandidate("openai", "gpt-4"),
	}

	run := simulateRuleEngineMatch("rule response", "LLM response")

	result, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "ruleengine" {
		t.Errorf("provider = %q, want ruleengine", result.Provider)
	}
	if result.Response.Content != "rule response" {
		t.Errorf("content = %q, want rule response", result.Response.Content)
	}
}
