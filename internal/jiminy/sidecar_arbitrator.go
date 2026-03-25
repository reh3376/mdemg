package jiminy

import (
	"log/slog"
	"math/rand/v2"
	"strings"
)

// Sidecar arbitration modes control how ML predictions influence tier selection.
const (
	SidecarModeShadow  = "shadow"  // ML predictions logged only, never used
	SidecarModeCompare = "compare" // Same as shadow but records agreement metrics
	SidecarModeCanary  = "canary"  // Probabilistic routing: % of eligible requests use ML
	SidecarModeActive  = "active"  // ML predictions used when confidence >= floor
)

// SidecarArbitrator decides whether to use ML or rule-based tier selection.
// It implements the arbitration layer between the rule-based encoder and the
// ML tier predictor, respecting mode, confidence, protected codes, and canary %.
type SidecarArbitrator struct {
	mode            string
	canaryPct       int
	confidenceFloor float64
	protectedCodes  map[string]bool // NS-10: codes that NEVER use ML tier
	logProtected    bool
}

// ArbitrationResult captures the full arbitration decision for metrics and training data.
type ArbitrationResult struct {
	ChosenTier int    // The tier to use
	Source     string // "rule", "ml", or "fallback"
	Agreed     bool   // Whether ML and rule agree
}

// NewSidecarArbitrator creates a new arbitrator with the given configuration.
func NewSidecarArbitrator(mode string, canaryPct int, confidenceFloor float64, protectedCodesCSV string, logProtected bool) *SidecarArbitrator {
	protected := make(map[string]bool)
	if protectedCodesCSV != "" {
		for _, code := range strings.Split(protectedCodesCSV, ",") {
			code = strings.TrimSpace(code)
			if code != "" {
				protected[code] = true
			}
		}
	}

	return &SidecarArbitrator{
		mode:            mode,
		canaryPct:       canaryPct,
		confidenceFloor: confidenceFloor,
		protectedCodes:  protected,
		logProtected:    logProtected,
	}
}

// ArbitrateTier decides whether to use ML or rule-based tier for a given guidance item.
//
// Parameters:
//   - item: the guidance item being encoded
//   - ruleTier: tier selected by the rule-based encoder (selectTier)
//   - mlTier: tier predicted by the ML sidecar (0 if unavailable)
//   - mlConf: confidence of the ML prediction (0 if unavailable)
//
// Returns an ArbitrationResult with the chosen tier, its source, and agreement status.
func (a *SidecarArbitrator) ArbitrateTier(item GuidanceItem, ruleTier int, mlTier int, mlConf float64) ArbitrationResult {
	agreed := mlTier == ruleTier || mlTier == 0

	// If ML prediction is unavailable (tier=0), always use rule-based
	if mlTier == 0 {
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: true}
	}

	// NS-10: Protected codes NEVER use ML tier regardless of mode
	if a.isProtected(item.ConstraintCode) {
		if a.logProtected && mlTier != ruleTier {
			slog.Warn("j17-arbitrator: protected code ML tier change blocked",
				"code", item.ConstraintCode, "rule_tier", ruleTier, "ml_tier", mlTier)
		}
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}
	}

	switch a.mode {
	case SidecarModeShadow:
		// Shadow: always rule-based, ML logged elsewhere
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}

	case SidecarModeCompare:
		// Compare: same as shadow but records agreement for metrics
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}

	case SidecarModeCanary:
		return a.arbitrateCanary(item, ruleTier, mlTier, mlConf, agreed)

	case SidecarModeActive:
		return a.arbitrateActive(ruleTier, mlTier, mlConf, agreed)

	default:
		// Unknown mode: safe fallback
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}
	}
}

// arbitrateCanary implements canary-mode logic.
// High-priority items always use rule-based. Others are probabilistically routed.
func (a *SidecarArbitrator) arbitrateCanary(item GuidanceItem, ruleTier int, mlTier int, mlConf float64, agreed bool) ArbitrationResult {
	// High-priority (must-level) items always use rule-based in canary mode
	if item.Priority == "high" {
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}
	}

	// Confidence gate: ML must meet floor
	if mlConf < a.confidenceFloor {
		return ArbitrationResult{ChosenTier: ruleTier, Source: "fallback", Agreed: agreed}
	}

	// Probabilistic routing by canary percentage
	if a.canaryPct <= 0 {
		return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}
	}
	if a.canaryPct >= 100 || rand.IntN(100) < a.canaryPct {
		return ArbitrationResult{ChosenTier: mlTier, Source: "ml", Agreed: agreed}
	}

	return ArbitrationResult{ChosenTier: ruleTier, Source: "rule", Agreed: agreed}
}

// arbitrateActive implements active-mode logic.
// ML is used when confidence meets the floor; otherwise falls back to rule-based.
func (a *SidecarArbitrator) arbitrateActive(ruleTier int, mlTier int, mlConf float64, agreed bool) ArbitrationResult {
	if mlConf >= a.confidenceFloor {
		return ArbitrationResult{ChosenTier: mlTier, Source: "ml", Agreed: agreed}
	}
	return ArbitrationResult{ChosenTier: ruleTier, Source: "fallback", Agreed: agreed}
}

// isProtected checks whether a constraint code is in the protected set.
func (a *SidecarArbitrator) isProtected(code string) bool {
	if code == "" || len(a.protectedCodes) == 0 {
		return false
	}
	return a.protectedCodes[code]
}

// Mode returns the current arbitration mode.
func (a *SidecarArbitrator) Mode() string {
	return a.mode
}

// IsCausal returns true if the mode can actually change tier selection (canary or active).
func (a *SidecarArbitrator) IsCausal() bool {
	return a.mode == SidecarModeCanary || a.mode == SidecarModeActive
}
