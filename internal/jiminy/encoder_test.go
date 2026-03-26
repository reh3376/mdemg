package jiminy

import (
	"strings"
	"testing"
)

func TestProtocolEncoder_T1Format(t *testing.T) {
	enc := NewProtocolEncoder(TierCoded)

	items := []GuidanceItem{
		{
			Type:           GuidanceConstraint,
			Priority:       "high",
			Content:        "Never force push to main",
			Confidence:     0.95,
			SourceNodes:    []string{"abc123"},
			ConstraintCode: "no-force-push-main",
		},
	}

	result := enc.Encode(items, SourceCounts{Constraints: 1}, 0.95, 0.9)

	if !strings.Contains(result, "C:!|no-force-push-main") {
		t.Errorf("T1 encoding should contain coded format, got:\n%s", result)
	}
	// Default compact level (T1CompactGlossary) drops src: metadata
	if strings.Contains(result, "src:abc123") {
		t.Errorf("T1 encoding at default compact level should NOT contain src:, got:\n%s", result)
	}
}

func TestProtocolEncoder_T1Format_NoCompact(t *testing.T) {
	enc := NewProtocolEncoder(TierCoded)
	enc.SetT1Compact(T1CompactNone)

	items := []GuidanceItem{
		{
			Type:           GuidanceConstraint,
			Priority:       "high",
			Content:        "Never force push to main",
			Confidence:     0.95,
			SourceNodes:    []string{"abc123"},
			ConstraintCode: "no-force-push-main",
		},
	}

	result := enc.Encode(items, SourceCounts{Constraints: 1}, 0.95, 0.9)

	if !strings.Contains(result, "C:!|no-force-push-main") {
		t.Errorf("T1 encoding should contain coded format, got:\n%s", result)
	}
	if !strings.Contains(result, "src:abc123") {
		t.Errorf("T1 encoding at compact=0 should contain source node, got:\n%s", result)
	}
}

func TestProtocolEncoder_T2Format(t *testing.T) {
	enc := NewProtocolEncoder(TierCoded)

	items := []GuidanceItem{
		{
			Type:           GuidanceConstraint,
			Priority:       "high",
			Content:        "You should always run tests before committing any changes to the codebase",
			Confidence:     0.85,
			SourceNodes:    []string{"def456"},
			ConstraintCode: "test-before-commit",
		},
	}

	// Low trust → T2 for items with codes
	result := enc.Encode(items, SourceCounts{Constraints: 1}, 0.85, 0.3)

	if !strings.Contains(result, "JIMINY GUIDANCE") {
		t.Errorf("should have guidance header, got:\n%s", result)
	}
	// T2 should contain the code in brackets
	if !strings.Contains(result, "[test-before-commit]") {
		t.Errorf("T2 encoding should contain code in brackets, got:\n%s", result)
	}
}

func TestProtocolEncoder_T3Fallback(t *testing.T) {
	enc := NewProtocolEncoder(TierFullNL)

	items := []GuidanceItem{
		{
			Type:       GuidanceCorrection,
			Priority:   "medium",
			Content:    "Authentication middleware rewrite is driven by compliance requirements",
			Confidence: 0.7,
		},
	}

	// No code + normal trust → T3
	result := enc.Encode(items, SourceCounts{Corrections: 1}, 0.7, 0.5)

	if !strings.Contains(result, "correction") {
		t.Errorf("T3 encoding should contain type name, got:\n%s", result)
	}
	if !strings.Contains(result, "Authentication middleware") {
		t.Errorf("T3 encoding should contain full content, got:\n%s", result)
	}
}

func TestProtocolEncoder_TierSelection(t *testing.T) {
	enc := NewProtocolEncoder(TierCoded)

	tests := []struct {
		name       string
		hasCode    bool
		trust      float64
		wantTier   int
	}{
		{"high trust + code → T1", true, 0.9, TierCoded},
		{"moderate trust + code → T2", true, 0.5, TierTelegraphic},
		{"low trust + code → T2", true, 0.3, TierTelegraphic},
		{"high trust + no code → T2", false, 0.9, TierTelegraphic},
		{"normal trust + no code → T3", false, 0.5, TierFullNL},
		{"low trust + no code → T3", false, 0.2, TierFullNL},
	}

	for _, tt := range tests {
		item := GuidanceItem{
			Type:     GuidanceConstraint,
			Priority: "high",
			Content:  "test constraint",
		}
		if tt.hasCode {
			item.ConstraintCode = "test-code"
		}

		tier := enc.selectTier(item, tt.trust)
		if tier != tt.wantTier {
			t.Errorf("%s: tier = %d, want %d", tt.name, tier, tt.wantTier)
		}
	}
}

func TestProtocolEncoder_EmptyItems(t *testing.T) {
	enc := NewProtocolEncoder(TierCoded)

	result := enc.Encode(nil, SourceCounts{}, 0, 0.5)
	if result != "" {
		t.Errorf("empty items should produce empty string, got: %q", result)
	}
}

func TestFormatBootstrap(t *testing.T) {
	bootstrap := FormatBootstrap()

	if !strings.Contains(bootstrap, "J17:INIT|v1") {
		t.Error("bootstrap should contain version init")
	}
	if !strings.Contains(bootstrap, "CODES:") {
		t.Error("bootstrap should contain CODES legend")
	}
	if !strings.Contains(bootstrap, "SEV:") {
		t.Error("bootstrap should contain SEV legend")
	}
	if !strings.Contains(bootstrap, "ESC:") {
		t.Error("bootstrap should contain ESC legend")
	}
}

func TestTelegraphicCompress(t *testing.T) {
	tests := []struct {
		input    string
		contains []string
		excludes []string
	}{
		{
			"You should always run the tests before committing",
			[]string{"always", "run", "tests", "before", "committing"},
			[]string{"You", "the"},
		},
		{
			"This is a very important constraint that must be followed",
			[]string{"important", "constraint", "followed"},
			[]string{"This", "a", "very"},
		},
	}

	for _, tt := range tests {
		result := telegraphicCompress(tt.input)
		for _, want := range tt.contains {
			if !strings.Contains(result, want) {
				t.Errorf("telegraphicCompress(%q) should contain %q, got: %q", tt.input, want, result)
			}
		}
		for _, exclude := range tt.excludes {
			if strings.Contains(result, exclude+" ") || strings.HasPrefix(result, exclude) {
				t.Errorf("telegraphicCompress(%q) should not contain %q as standalone word, got: %q", tt.input, exclude, result)
			}
		}
	}
}

func TestGuidanceTypePrefix(t *testing.T) {
	tests := []struct {
		gtype    GuidanceType
		expected string
	}{
		{GuidanceConstraint, "C"},
		{GuidanceCorrection, "X"},
		{GuidanceFrontier, "F"},
		{GuidanceDecision, "D"},
		{GuidancePattern, "P"},
		{GuidanceLearning, "L"},
		{GuidanceConflict, "!"},
		{GuidanceConcept, "K"},
	}

	for _, tt := range tests {
		got := guidanceTypePrefix(tt.gtype)
		if got != tt.expected {
			t.Errorf("guidanceTypePrefix(%q) = %q, want %q", tt.gtype, got, tt.expected)
		}
	}
}
