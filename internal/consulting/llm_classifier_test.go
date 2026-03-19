package consulting

import (
	"testing"

	"mdemg/internal/models"
)

func TestConstraintClassifier_ParseResponse_Must(t *testing.T) {
	cc := &ConstraintClassifier{}

	result, err := cc.parseResponse(`{"type": "must", "summary": "Always use config files"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "must" {
		t.Errorf("expected type 'must', got %q", result.Type)
	}
	if result.Summary != "Always use config files" {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
}

func TestConstraintClassifier_ParseResponse_None(t *testing.T) {
	cc := &ConstraintClassifier{}

	result, err := cc.parseResponse(`{"type": "none", "summary": ""}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "none" {
		t.Errorf("expected type 'none', got %q", result.Type)
	}
}

func TestConstraintClassifier_ParseResponse_AllTypes(t *testing.T) {
	cc := &ConstraintClassifier{}

	for _, tp := range []string{"must", "must_not", "should", "should_not", "none"} {
		result, err := cc.parseResponse(`{"type": "` + tp + `", "summary": "test"}`)
		if err != nil {
			t.Errorf("unexpected error for type %q: %v", tp, err)
		}
		if result.Type != tp {
			t.Errorf("expected type %q, got %q", tp, result.Type)
		}
	}
}

func TestConstraintClassifier_ParseResponse_InvalidType(t *testing.T) {
	cc := &ConstraintClassifier{}

	_, err := cc.parseResponse(`{"type": "maybe", "summary": "test"}`)
	if err == nil {
		t.Error("expected error for invalid constraint type")
	}
}

func TestConstraintClassifier_ParseResponse_CodeFence(t *testing.T) {
	cc := &ConstraintClassifier{}

	result, err := cc.parseResponse("```json\n{\"type\": \"should\", \"summary\": \"test\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "should" {
		t.Errorf("expected type 'should', got %q", result.Type)
	}
}

func TestConstraintClassifier_ParseResponse_InvalidJSON(t *testing.T) {
	cc := &ConstraintClassifier{}

	_, err := cc.parseResponse("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestConstraintClassifier_Cache(t *testing.T) {
	cc := NewConstraintClassifier(ConstraintClassifierConfig{Enabled: true}, nil)

	// Put and get
	cc.cachePut("node-1", ConstraintClassification{Type: "must", Summary: "test"})
	result := cc.cacheGet("node-1")
	if result == nil {
		t.Fatal("expected cached result")
	}
	if result.Type != "must" {
		t.Errorf("expected type 'must', got %q", result.Type)
	}

	// Miss
	result = cc.cacheGet("nonexistent")
	if result != nil {
		t.Error("expected nil for cache miss")
	}
}

func TestConstraintClassifier_Disabled(t *testing.T) {
	cc := NewConstraintClassifier(ConstraintClassifierConfig{Enabled: false}, nil)

	result, err := cc.Classify(nil, "node-1", "test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestKeywordClassifyConstraint_Must(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name     string
		summary  string
		score    float64
		expected string
	}{
		{"must", "You must always validate input", 0.7, "must"},
		{"must_not", "You must not hardcode secrets", 0.7, "must_not"},
		{"should", "You should use descriptive names", 0.7, "should"},
		{"should_not", "You should not use global state", 0.7, "should_not"},
		{"required", "Input validation is required", 0.7, "must"},
		{"no_match", "Just a regular description", 0.7, ""},
		{"low_score", "You must validate", 0.3, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := models.RetrieveResult{Name: "test", Summary: tt.summary, Score: tt.score}
			got := svc.keywordClassifyConstraint(r)
			if got != tt.expected {
				t.Errorf("keywordClassifyConstraint(%q, score=%f) = %q, want %q", tt.summary, tt.score, got, tt.expected)
			}
		})
	}
}
