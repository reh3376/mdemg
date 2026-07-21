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
		name     string
		hasCode  bool
		trust    float64
		wantTier int
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

// ─── J17-TIER-GATE-001: comprehension-keyed T1 promotion ───

// TestSelectTier_TrustMode_ByteIdenticalDefault pins that the default mode
// ("trust", or an unset/invalid mode) leaves tier selection byte-identical
// to the legacy compliance-trust logic — even with a provider wired.
func TestSelectTier_TrustMode_ByteIdenticalDefault(t *testing.T) {
	coded := GuidanceItem{ConstraintCode: "test-code", Content: "c"}
	uncoded := GuidanceItem{Content: "c"}

	legacy := NewProtocolEncoder(TierCoded) // no gate configured at all
	gated := NewProtocolEncoder(TierCoded)
	gated.SetComprehensionGate("trust", 0.6, 20)
	gated.SetComprehensionProvider(func() (float64, int64) { return 0.99, 1000 })

	for _, trust := range []float64{0.0, 0.2, 0.35, 0.5, 0.75, 0.76, 1.0} {
		for _, item := range []GuidanceItem{coded, uncoded} {
			if a, b := legacy.selectTier(item, trust), gated.selectTier(item, trust); a != b {
				t.Errorf("trust-mode divergence at trust=%v hasCode=%v: legacy=%d gated=%d",
					trust, item.ConstraintCode != "", a, b)
			}
		}
	}
}

// TestSelectTier_ComprehensionMode_TruthTable pins the comprehension-keyed
// promotion: T1 for coded items once measured comprehension clears the floor
// — regardless of trust (the axis dense encoding actually risks is
// incomprehension, not non-compliance).
func TestSelectTier_ComprehensionMode_TruthTable(t *testing.T) {
	coded := GuidanceItem{ConstraintCode: "test-code", Content: "c"}
	uncoded := GuidanceItem{Content: "c"}

	mk := func(score float64, samples int64) *ProtocolEncoder {
		e := NewProtocolEncoder(TierCoded)
		e.SetComprehensionGate("comprehension", 0.6, 20)
		e.SetComprehensionProvider(func() (float64, int64) { return score, samples })
		return e
	}

	const lowTrust = 0.23 // the live trust value that blocked T1 for months

	cases := []struct {
		name string
		enc  *ProtocolEncoder
		item GuidanceItem
		want int
	}{
		{"coded + comprehension above floor → T1 (despite low trust)", mk(0.73, 3592), coded, TierCoded},
		{"coded + comprehension below floor → T2", mk(0.45, 3592), coded, TierTelegraphic},
		{"uncoded + comprehension above floor → T2", mk(0.73, 3592), uncoded, TierTelegraphic},
		{"uncoded + comprehension below floor → T3", mk(0.45, 3592), uncoded, TierFullNL},
		{"cold start (samples < min) falls back to trust logic → T2 for coded at low trust", mk(0.99, 5), coded, TierTelegraphic},
		{"cold start uncoded low trust → T3", mk(0.99, 5), uncoded, TierFullNL},
	}
	for _, tc := range cases {
		if got := tc.enc.selectTier(tc.item, lowTrust); got != tc.want {
			t.Errorf("%s: got tier %d, want %d", tc.name, got, tc.want)
		}
	}

	// Provider nil → trust logic even in comprehension mode.
	noProv := NewProtocolEncoder(TierCoded)
	noProv.SetComprehensionGate("comprehension", 0.6, 20)
	if got := noProv.selectTier(coded, lowTrust); got != TierTelegraphic {
		t.Errorf("nil provider must fall back to trust logic: got %d, want %d (T2)", got, TierTelegraphic)
	}
}

// TestSetComprehensionGate_Validation pins the setter's defensive fallbacks.
func TestSetComprehensionGate_Validation(t *testing.T) {
	e := NewProtocolEncoder(TierCoded)
	e.SetComprehensionGate("bogus", -1, 0)
	if e.tierGateMode != "trust" {
		t.Errorf("invalid mode must fall back to trust, got %q", e.tierGateMode)
	}
	if e.comprehensionHigh != 0.6 {
		t.Errorf("out-of-range high must fall back to 0.6, got %v", e.comprehensionHigh)
	}
	if e.comprehensionMinSamps != 1 {
		t.Errorf("minSamples floor is 1, got %d", e.comprehensionMinSamps)
	}
}
