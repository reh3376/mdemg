package config

import (
	"regexp"
	"strings"
	"testing"
)

// JIMINY-CORPUS-001: ConvObs→constraint promotion gate config.

func TestConstraintPromotionGateDefaults(t *testing.T) {
	setMinimalEnv(t)
	for _, e := range []string{
		"CONSTRAINT_PROMOTION_GATE_ENABLED",
		"CONSTRAINT_PROMOTION_DENY_OBS_TYPES",
		"CONSTRAINT_PROMOTION_REJECT_PATTERNS",
	} {
		t.Setenv(e, "")
	}
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.ConstraintPromotionGateEnabled {
		t.Error("ConstraintPromotionGateEnabled default = false, want true (the gate is the forward-fix)")
	}
	wantDeny := []string{"progress", "error", "task", "context"}
	if len(cfg.ConstraintPromotionDenyObsTypes) != len(wantDeny) {
		t.Fatalf("ConstraintPromotionDenyObsTypes = %v, want %v", cfg.ConstraintPromotionDenyObsTypes, wantDeny)
	}
	for i, w := range wantDeny {
		if cfg.ConstraintPromotionDenyObsTypes[i] != w {
			t.Errorf("ConstraintPromotionDenyObsTypes[%d] = %q, want %q", i, cfg.ConstraintPromotionDenyObsTypes[i], w)
		}
	}
	if len(cfg.ConstraintPromotionRejectPatterns) != len(DefaultConstraintPromotionRejectPatterns()) {
		t.Errorf("ConstraintPromotionRejectPatterns = %d patterns, want the %d built-in defaults",
			len(cfg.ConstraintPromotionRejectPatterns), len(DefaultConstraintPromotionRejectPatterns()))
	}
}

// Every built-in default pattern must compile — FromEnv fails closed on an
// invalid operator pattern, so the defaults must never trip that check.
func TestDefaultConstraintPromotionRejectPatterns_Compile(t *testing.T) {
	pats := DefaultConstraintPromotionRejectPatterns()
	if len(pats) == 0 {
		t.Fatal("default reject pattern set is empty")
	}
	for _, p := range pats {
		if _, err := regexp.Compile(p); err != nil {
			t.Errorf("default pattern %q does not compile: %v", p, err)
		}
	}
}

func TestConstraintPromotionGateOverrides(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CONSTRAINT_PROMOTION_GATE_ENABLED", "false")
	t.Setenv("CONSTRAINT_PROMOTION_DENY_OBS_TYPES", " Progress , NOTE ")
	t.Setenv("CONSTRAINT_PROMOTION_REJECT_PATTERNS", `["^custom junk"]`)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.ConstraintPromotionGateEnabled {
		t.Error("ConstraintPromotionGateEnabled = true, want false (operator disable path)")
	}
	if len(cfg.ConstraintPromotionDenyObsTypes) != 2 ||
		cfg.ConstraintPromotionDenyObsTypes[0] != "progress" ||
		cfg.ConstraintPromotionDenyObsTypes[1] != "note" {
		t.Errorf("ConstraintPromotionDenyObsTypes = %v, want [progress note] (trimmed, lowercased)", cfg.ConstraintPromotionDenyObsTypes)
	}
	if len(cfg.ConstraintPromotionRejectPatterns) != 1 || cfg.ConstraintPromotionRejectPatterns[0] != "^custom junk" {
		t.Errorf("ConstraintPromotionRejectPatterns = %v, want the single override", cfg.ConstraintPromotionRejectPatterns)
	}
}

// "[]" disables the pattern layer without touching the provenance layer.
func TestConstraintPromotionRejectPatterns_EmptyArrayDisablesPatternLayer(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CONSTRAINT_PROMOTION_REJECT_PATTERNS", "[]")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if len(cfg.ConstraintPromotionRejectPatterns) != 0 {
		t.Errorf("ConstraintPromotionRejectPatterns = %v, want empty", cfg.ConstraintPromotionRejectPatterns)
	}
}

func TestConstraintPromotionRejectPatterns_InvalidJSONFails(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CONSTRAINT_PROMOTION_REJECT_PATTERNS", "not-json")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "CONSTRAINT_PROMOTION_REJECT_PATTERNS") {
		t.Errorf("FromEnv err = %v, want CONSTRAINT_PROMOTION_REJECT_PATTERNS JSON error", err)
	}
}

func TestConstraintPromotionRejectPatterns_InvalidRegexFails(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CONSTRAINT_PROMOTION_REJECT_PATTERNS", `["(unclosed"]`)
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "invalid regexp") {
		t.Errorf("FromEnv err = %v, want invalid regexp error", err)
	}
}
