package jiminy

import (
	"strings"
	"testing"
)

func TestNormalize_SemanticExtraction(t *testing.T) {
	input := `Module: error-sanitizer. Related to: caching, authentication, error-handling SEMANTIC: Contains utility functions for sanitizing error messages before logging, preventing sensitive data exposure in error outputs.`
	got := normalizeGuidanceContent(input)
	want := "Contains utility functions for sanitizing error messages before logging, preventing sensitive data exposure in error outputs."
	if got != want {
		t.Errorf("SEMANTIC extraction:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestNormalize_SemanticWithSuggestionPrefix(t *testing.T) {
	input := `Risk warning: Module: error-reporting.service. Related to: authentication SEMANTIC: Orchestrates the entire error reporting process, linking error categorization and storage.`
	got := normalizeGuidanceContent(input)
	want := "Orchestrates the entire error reporting process, linking error categorization and storage."
	if got != want {
		t.Errorf("SEMANTIC with prefix:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestNormalize_StructuralModule(t *testing.T) {
	input := "Module: error-sanitizer. Related to: caching, authentication, error-handling. Key functions: containsSensitiveInfo, redactSensitiveInfo"
	got := normalizeGuidanceContent(input)
	// Should be natural language prose
	if got == input {
		t.Errorf("structural content should be normalized, got unchanged: %s", got)
	}
	// Should NOT contain "Module:" prefix
	if strings.Contains(got, "Module:") {
		t.Errorf("should strip Module: prefix, got: %s", got)
	}
	// Should contain the entity name
	if !strings.Contains(got, "error-sanitizer") {
		t.Errorf("should preserve entity name, got: %s", got)
	}
}

func TestNormalize_StructuralClass(t *testing.T) {
	input := "Class ErrorStorageService in error-storage.service. Related to: authentication, error-handling, validation"
	got := normalizeGuidanceContent(input)
	if strings.Contains(got, "Class ") {
		t.Errorf("should strip Class prefix, got: %s", got)
	}
	if !strings.Contains(got, "ErrorStorageService") {
		t.Errorf("should preserve class name, got: %s", got)
	}
}

func TestNormalize_SuggestionPrefixStripped(t *testing.T) {
	input := "Previous solution to similar problem: Function Error in auth. Related to: authentication, error-handling"
	got := normalizeGuidanceContent(input)
	if strings.Contains(got, "Previous solution") {
		t.Errorf("should strip suggestion prefix, got: %s", got)
	}
}

func TestNormalize_RiskWarningPrefixStripped(t *testing.T) {
	input := "Risk warning: Module: error-reporting.service. Related to: authentication"
	got := normalizeGuidanceContent(input)
	if strings.Contains(got, "Risk warning") {
		t.Errorf("should strip Risk warning prefix, got: %s", got)
	}
}

func TestNormalize_ConstraintPassthrough(t *testing.T) {
	input := "[must_not] You must never use UUID v4 in this codebase. Always use CUIDv2."
	got := normalizeGuidanceContent(input)
	if got != input {
		t.Errorf("constraints should pass through unchanged:\n  got:  %s\n  want: %s", got, input)
	}
}

func TestNormalize_DuplicateNameRemoval(t *testing.T) {
	// Source E produces "name: Module: name. Related to: ..."
	input := "error-sanitizer: Module: error-sanitizer. Related to: caching"
	got := normalizeGuidanceContent(input)
	// Should not have "error-sanitizer" appearing twice at the start
	if strings.Contains(got, "error-sanitizer: Module: error-sanitizer") {
		t.Errorf("should remove duplicate name prefix, got: %s", got)
	}
}

func TestNormalize_EmptyContent(t *testing.T) {
	if got := normalizeGuidanceContent(""); got != "" {
		t.Errorf("empty input should return empty, got: %s", got)
	}
}

func TestNormalize_PlainTextPassthrough(t *testing.T) {
	input := "ensure error handling uses structured logging"
	got := normalizeGuidanceContent(input)
	if got != input {
		t.Errorf("plain natural language should pass through, got: %s", got)
	}
}

func TestNormalize_RelatedToProseConversion(t *testing.T) {
	input := "Module: auth-service. Related to: authentication, authorization, and validation"
	got := normalizeGuidanceContent(input)
	if strings.Contains(got, "Related to:") {
		t.Errorf("should convert 'Related to:' to prose, got: %s", got)
	}
}

func TestNaturalJoin(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a", "a"},
		{"a, b", "a and b"},
		{"a, b, c", "a, b, and c"},
		{"authentication, authorization, error-handling", "authentication, authorization, and error-handling"},
	}
	for _, tt := range tests {
		got := naturalJoin(tt.input)
		if got != tt.want {
			t.Errorf("naturalJoin(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

