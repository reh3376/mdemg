package guardrail

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mdemg/internal/llmclient"
)

// TestEvaluateWithLLM_PassesMessagesAndOptsToClient verifies Sprint FT-LORA-B
// wiring: the guardrail service builds a system+user message pair and passes
// it to the injected llmclient.Completer, along with the configured MaxTokens.
// This is the positive-path counterpart to the TSDB integration test in
// Epic 6 (which verifies the interaction lands in llm_interactions).
func TestEvaluateWithLLM_PassesMessagesAndOptsToClient(t *testing.T) {
	var capturedMessages []llmclient.Message
	var capturedOpts llmclient.CompleteOpts
	stub := &llmclient.TestClient{
		CompleteFn: func(ctx context.Context, messages []llmclient.Message, opts llmclient.CompleteOpts) (string, error) {
			capturedMessages = messages
			capturedOpts = opts
			return `{"violations":[],"warnings":[]}`, nil
		},
	}

	svc := &GuardrailService{
		cfg: GuardrailConfig{
			Enabled:   true,
			Provider:  "openai",
			Model:     "gpt-5.4-mini",
			MaxTokens: 2048,
		},
		llm: stub,
	}

	diffCtx := DiffContext{
		FilePaths:  []string{"main.go"},
		AddedLines: []string{"password := \"hunter2\""},
	}
	constraints := []constraintMatch{
		{NodeID: "c-1", ConstraintType: "must_not", Content: "no plaintext passwords", Confidence: 0.9},
	}

	result, err := svc.evaluateWithLLM(context.Background(), diffCtx, constraints)
	if err != nil {
		t.Fatalf("evaluateWithLLM: %v", err)
	}
	if result == nil || len(result.Violations) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	if len(capturedMessages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != "system" {
		t.Errorf("message[0].Role = %q, want \"system\"", capturedMessages[0].Role)
	}
	if !strings.Contains(capturedMessages[0].Content, "guardrail evaluator") {
		t.Errorf("system prompt missing expected content; got prefix %q", capturedMessages[0].Content[:min(60, len(capturedMessages[0].Content))])
	}
	if capturedMessages[1].Role != "user" {
		t.Errorf("message[1].Role = %q, want \"user\"", capturedMessages[1].Role)
	}
	if !strings.Contains(capturedMessages[1].Content, "main.go") {
		t.Error("user prompt missing file path from diff context")
	}
	// MaxTokens is bumped to 2000 when cfg.MaxTokens < 2000 (reasoning models);
	// cfg set 2048, so 2048 should flow through unchanged.
	if capturedOpts.MaxTokens != 2048 {
		t.Errorf("opts.MaxTokens = %d, want 2048", capturedOpts.MaxTokens)
	}
}

// TestEvaluateWithLLM_UsesCompactPromptWhenConfigured verifies that the
// CompressPrompts config toggle selects the compact system prompt variant.
func TestEvaluateWithLLM_UsesCompactPromptWhenConfigured(t *testing.T) {
	var capturedSystem string
	stub := &llmclient.TestClient{
		CompleteFn: func(ctx context.Context, messages []llmclient.Message, opts llmclient.CompleteOpts) (string, error) {
			if len(messages) > 0 {
				capturedSystem = messages[0].Content
			}
			return `{"violations":[],"warnings":[]}`, nil
		},
	}
	svc := &GuardrailService{
		cfg: GuardrailConfig{Enabled: true, Provider: "openai", MaxTokens: 2000, CompressPrompts: true},
		llm: stub,
	}
	_, err := svc.evaluateWithLLM(context.Background(), DiffContext{}, []constraintMatch{
		{NodeID: "c-1", ConstraintType: "must", Content: "x", Confidence: 0.5},
	})
	if err != nil {
		t.Fatalf("evaluateWithLLM: %v", err)
	}
	if capturedSystem != guardrailSystemPromptCompact {
		t.Errorf("expected compact system prompt; got %q...", capturedSystem[:min(60, len(capturedSystem))])
	}
}

// TestEvaluateWithLLM_NilClient returns a clean error (not a panic) when
// no llmclient is wired. This guards against mis-configuration at server
// startup (cfg.Enabled=true but llm nil).
func TestEvaluateWithLLM_NilClient(t *testing.T) {
	svc := &GuardrailService{cfg: GuardrailConfig{Enabled: true, Provider: "openai"}}
	_, err := svc.evaluateWithLLM(context.Background(), DiffContext{}, []constraintMatch{
		{NodeID: "c-1", ConstraintType: "must", Content: "x", Confidence: 0.5},
	})
	if err == nil {
		t.Fatal("expected error when llm is nil")
	}
	if !strings.Contains(err.Error(), "llm client not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestEvaluateWithLLM_PropagatesClientError ensures a Complete() error
// surfaces unchanged (no silent swallowing). Upstream Validate() is
// expected to fail-open on this error per the documented contract.
func TestEvaluateWithLLM_PropagatesClientError(t *testing.T) {
	stub := &llmclient.TestClient{
		CompleteFn: func(ctx context.Context, messages []llmclient.Message, opts llmclient.CompleteOpts) (string, error) {
			return "", fmt.Errorf("simulated upstream failure")
		},
	}
	svc := &GuardrailService{
		cfg: GuardrailConfig{Enabled: true, Provider: "openai", MaxTokens: 2000},
		llm: stub,
	}
	_, err := svc.evaluateWithLLM(context.Background(), DiffContext{}, []constraintMatch{
		{NodeID: "c-1", ConstraintType: "must", Content: "x", Confidence: 0.5},
	})
	if err == nil || !strings.Contains(err.Error(), "simulated upstream failure") {
		t.Fatalf("expected upstream error to propagate; got %v", err)
	}
}

// TestTaskNameConstant documents that the package exposes a canonical
// task-name constant matching the ULTS spec guardrail_evaluate.ults.json.
// If this test fails, the ULTS spec and the TSDB attribution have drifted.
func TestTaskNameConstant(t *testing.T) {
	if TaskName != "guardrail.evaluate" {
		t.Errorf("TaskName = %q, want \"guardrail.evaluate\" (must match docs/tests/ults/specs/guardrail_evaluate.ults.json)", TaskName)
	}
}
