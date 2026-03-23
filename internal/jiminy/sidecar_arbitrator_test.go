package jiminy

import (
	"testing"
)

func makeItem(priority string, code string) GuidanceItem {
	return GuidanceItem{
		Type:           GuidanceConstraint,
		Priority:       priority,
		Content:        "test constraint",
		ConstraintCode: code,
	}
}

// --- Shadow Mode ---

func TestArbitrator_Shadow_AlwaysRule(t *testing.T) {
	a := NewSidecarArbitrator("shadow", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("high", "code-1"), 1, 2, 0.9)
	if result.ChosenTier != 1 || result.Source != "rule" {
		t.Errorf("shadow: got tier=%d source=%s, want tier=1 source=rule", result.ChosenTier, result.Source)
	}
	if result.Agreed {
		t.Error("shadow: expected agreed=false when ml=2 rule=1")
	}
}

func TestArbitrator_Shadow_MLUnavailable(t *testing.T) {
	a := NewSidecarArbitrator("shadow", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 2, 0, 0)
	if result.ChosenTier != 2 || result.Source != "rule" || !result.Agreed {
		t.Errorf("shadow ml=0: got tier=%d source=%s agreed=%v", result.ChosenTier, result.Source, result.Agreed)
	}
}

// --- Compare Mode ---

func TestArbitrator_Compare_AlwaysRule(t *testing.T) {
	a := NewSidecarArbitrator("compare", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("medium", "code-1"), 1, 1, 0.95)
	if result.Source != "rule" {
		t.Errorf("compare: expected source=rule, got %s", result.Source)
	}
	if !result.Agreed {
		t.Error("compare: expected agreed=true when ml==rule")
	}
}

func TestArbitrator_Compare_TracksDisagreement(t *testing.T) {
	a := NewSidecarArbitrator("compare", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 1, 3, 0.8)
	if result.Agreed {
		t.Error("compare: expected agreed=false when ml=3 rule=1")
	}
	if result.ChosenTier != 1 {
		t.Errorf("compare: expected chosen=1 (rule), got %d", result.ChosenTier)
	}
}

// --- Canary Mode ---

func TestArbitrator_Canary_HighPriorityAlwaysRule(t *testing.T) {
	a := NewSidecarArbitrator("canary", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("high", "code-1"), 3, 1, 0.95)
	if result.Source != "rule" || result.ChosenTier != 3 {
		t.Errorf("canary high-priority: got tier=%d source=%s, want tier=3 source=rule", result.ChosenTier, result.Source)
	}
}

func TestArbitrator_Canary_LowPriority_100Pct(t *testing.T) {
	a := NewSidecarArbitrator("canary", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.8)
	if result.Source != "ml" || result.ChosenTier != 1 {
		t.Errorf("canary 100%%: got tier=%d source=%s, want tier=1 source=ml", result.ChosenTier, result.Source)
	}
}

func TestArbitrator_Canary_0Pct(t *testing.T) {
	a := NewSidecarArbitrator("canary", 0, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.8)
	if result.Source != "rule" {
		t.Errorf("canary 0%%: got source=%s, want rule", result.Source)
	}
}

func TestArbitrator_Canary_BelowConfidenceFloor(t *testing.T) {
	a := NewSidecarArbitrator("canary", 100, 0.8, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.5)
	if result.Source != "fallback" {
		t.Errorf("canary below floor: got source=%s, want fallback", result.Source)
	}
	if result.ChosenTier != 3 {
		t.Errorf("canary below floor: got tier=%d, want 3 (rule)", result.ChosenTier)
	}
}

func TestArbitrator_Canary_MediumPriority(t *testing.T) {
	a := NewSidecarArbitrator("canary", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("medium", "code-1"), 2, 1, 0.75)
	if result.Source != "ml" || result.ChosenTier != 1 {
		t.Errorf("canary medium: got tier=%d source=%s, want tier=1 source=ml", result.ChosenTier, result.Source)
	}
}

// --- Active Mode ---

func TestArbitrator_Active_HighConfidence(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.85)
	if result.Source != "ml" || result.ChosenTier != 1 {
		t.Errorf("active high-conf: got tier=%d source=%s, want tier=1 source=ml", result.ChosenTier, result.Source)
	}
}

func TestArbitrator_Active_LowConfidence(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.3)
	if result.Source != "fallback" || result.ChosenTier != 3 {
		t.Errorf("active low-conf: got tier=%d source=%s, want tier=3 source=fallback", result.ChosenTier, result.Source)
	}
}

func TestArbitrator_Active_ExactFloor(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("low", "code-1"), 3, 1, 0.6)
	if result.Source != "ml" {
		t.Errorf("active exact floor: got source=%s, want ml", result.Source)
	}
}

func TestArbitrator_Active_HighPriorityNotProtected(t *testing.T) {
	// In active mode, high-priority items are NOT automatically protected (unlike canary)
	a := NewSidecarArbitrator("active", 100, 0.6, "", true)

	result := a.ArbitrateTier(makeItem("high", "code-1"), 3, 1, 0.9)
	if result.Source != "ml" {
		t.Errorf("active high-priority: got source=%s, want ml (not protected in active)", result.Source)
	}
}

// --- Protected Codes (NS-10) ---

func TestArbitrator_ProtectedCode_Shadow(t *testing.T) {
	a := NewSidecarArbitrator("shadow", 100, 0.6, "no-force-push,no-direct-main", true)

	result := a.ArbitrateTier(makeItem("low", "no-force-push"), 3, 1, 0.9)
	if result.Source != "rule" {
		t.Errorf("protected shadow: got source=%s, want rule", result.Source)
	}
}

func TestArbitrator_ProtectedCode_Active(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "no-force-push,no-direct-main", true)

	result := a.ArbitrateTier(makeItem("low", "no-force-push"), 3, 1, 0.95)
	if result.Source != "rule" || result.ChosenTier != 3 {
		t.Errorf("protected active: got tier=%d source=%s, want tier=3 source=rule", result.ChosenTier, result.Source)
	}
}

func TestArbitrator_ProtectedCode_Canary(t *testing.T) {
	a := NewSidecarArbitrator("canary", 100, 0.6, "no-force-push", true)

	result := a.ArbitrateTier(makeItem("low", "no-force-push"), 2, 1, 0.9)
	if result.Source != "rule" {
		t.Errorf("protected canary: got source=%s, want rule", result.Source)
	}
}

func TestArbitrator_UnprotectedCode_Active(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "no-force-push", true)

	result := a.ArbitrateTier(makeItem("low", "some-other-code"), 3, 1, 0.9)
	if result.Source != "ml" {
		t.Errorf("unprotected active: got source=%s, want ml", result.Source)
	}
}

func TestArbitrator_EmptyCode_NotProtected(t *testing.T) {
	a := NewSidecarArbitrator("active", 100, 0.6, "no-force-push", true)

	result := a.ArbitrateTier(makeItem("low", ""), 3, 1, 0.9)
	if result.Source != "ml" {
		t.Errorf("empty code: got source=%s, want ml", result.Source)
	}
}

// --- IsCausal ---

func TestArbitrator_IsCausal(t *testing.T) {
	tests := []struct {
		mode   string
		causal bool
	}{
		{"shadow", false},
		{"compare", false},
		{"canary", true},
		{"active", true},
	}
	for _, tt := range tests {
		a := NewSidecarArbitrator(tt.mode, 100, 0.6, "", true)
		if a.IsCausal() != tt.causal {
			t.Errorf("IsCausal(%q) = %v, want %v", tt.mode, a.IsCausal(), tt.causal)
		}
	}
}

// --- Agreement tracking ---

func TestArbitrator_AgreementTracking(t *testing.T) {
	a := NewSidecarArbitrator("compare", 100, 0.6, "", true)

	// ML agrees with rule
	r1 := a.ArbitrateTier(makeItem("low", "code-1"), 1, 1, 0.9)
	if !r1.Agreed {
		t.Error("expected agreed=true when ml==rule")
	}

	// ML disagrees with rule
	r2 := a.ArbitrateTier(makeItem("low", "code-1"), 1, 2, 0.9)
	if r2.Agreed {
		t.Error("expected agreed=false when ml!=rule")
	}
}
