package jiminy

import (
	"context"
	"testing"
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
