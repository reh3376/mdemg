package jiminy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mdemg/internal/llmclient"
)

func TestConstraintCodeGenerator_FallbackCode(t *testing.T) {
	gen := NewConstraintCodeGenerator(nil) // no LLM

	code, err := gen.GenerateCode(context.Background(), "must_not", "Never force push to main branch")
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	if code == "" {
		t.Fatal("code is empty")
	}

	// Fallback should start with "auto-"
	if len(code) < 5 || code[:5] != "auto-" {
		t.Errorf("fallback code %q should start with 'auto-'", code)
	}

	// Same description should produce same code (deterministic)
	code2, _ := gen.GenerateCode(context.Background(), "must_not", "Never force push to main branch")
	if code != code2 {
		t.Errorf("deterministic fallback failed: %q != %q", code, code2)
	}
}

func TestConstraintCodeGenerator_CollisionAvoidance(t *testing.T) {
	gen := NewConstraintCodeGenerator(nil)

	gen.RegisterExistingCode("no-force-push")
	gen.RegisterExistingCode("test-first")

	// Fallback codes should be unique even if the existing set is populated
	code, _ := gen.GenerateCode(context.Background(), "must", "Test before commit")
	if code == "no-force-push" || code == "test-first" {
		t.Errorf("code %q collided with existing codes", code)
	}
}

// TestConstraintCodeGenerator_CollisionFallbackNoDeadlock pins the
// DORMANT-CENSUS-001 live-smoke fix: when the LLM returns a code that is
// already registered, the collision branch (which holds g.mu) must use
// fallbackCodeLocked — the old code called fallbackCode, re-locking the
// non-reentrant mutex and wedging the generator (and every constraint-typed
// Observe) for the life of the process.
func TestConstraintCodeGenerator_CollisionFallbackNoDeadlock(t *testing.T) {
	// Fake OpenAI-compat endpoint that always returns the same code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"colliding-code"}}]}`))
	}))
	defer srv.Close()

	client := llmclient.New(llmclient.Config{
		Provider:  "openai",
		Model:     "test",
		APIKey:    "test",
		BaseURL:   srv.URL,
		TimeoutMs: 5000,
	})
	gen := NewConstraintCodeGenerator(client)
	gen.RegisterExistingCode("colliding-code")

	done := make(chan string, 1)
	go func() {
		code, err := gen.GenerateCode(context.Background(), "must", "some constraint description")
		if err != nil {
			t.Errorf("GenerateCode failed: %v", err)
		}
		done <- code
	}()

	select {
	case code := <-done:
		if !strings.HasPrefix(code, "auto-") {
			t.Errorf("collision path should return a deterministic auto- code, got %q", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GenerateCode deadlocked on the collision-fallback path")
	}

	// The generator must remain usable afterwards (the wedge was permanent).
	code2, err := gen.GenerateCode(context.Background(), "must", "another description")
	if err != nil {
		t.Fatalf("subsequent GenerateCode failed: %v", err)
	}
	if code2 == "" {
		t.Fatal("subsequent GenerateCode returned empty code")
	}
}

func TestSanitizeCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"no-force-push", "no-force-push"},
		{"  No Force Push  ", "no-force-push"},
		{"\"test_first\"", "test-first"},
		{"a--b---c", "a-b-c"},
		{"has spaces here", "has-spaces-here"},
		{"UPPER-case", "upper-case"},
		{"special!@#chars", "specialchars"},
		{"", ""},
		{"`code`", "code"},
	}

	for _, tt := range tests {
		got := sanitizeCode(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeCode(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
