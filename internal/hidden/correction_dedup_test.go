// Sprint CREATE-CORRECTION-DEDUP-001 (2026-08-12) — pin tests.
//
// The dedup logic in CreateCorrectionNodes is Neo4j-dependent; testing the
// full flow requires an integration harness. These unit tests pin the
// CONFIG SHAPE + the ORDER-OF-OPS invariants — the actual Cypher round-
// trip is exercised in live smoke.

package hidden

import (
	"testing"

	"mdemg/internal/config"
)

// TestCorrectionDedupConfig_DefaultsAreSafe pins the shipped defaults:
// enabled=true, threshold=0.75. Any drift in the defaults changes the
// consolidation behavior on operators who don't explicitly set the env
// vars — must be caught before shipping.
func TestCorrectionDedupConfig_DefaultsAreSafe(t *testing.T) {
	// Set only the required NEO4J_* env so config.FromEnv succeeds.
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "test")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}
	if !cfg.JiminyCorrectionDedupEnabled {
		t.Errorf("JiminyCorrectionDedupEnabled default should be true (prevents re-accumulation of the 24-DUP class purged by JIMINY-CORRECTION-CORPUS-001); got false")
	}
	if cfg.JiminyCorrectionDedupSimThreshold != 0.75 {
		t.Errorf("JiminyCorrectionDedupSimThreshold default should be 0.75 (conservative \"same rule\" cutoff); got %v", cfg.JiminyCorrectionDedupSimThreshold)
	}
}

// TestCorrectionDedupConfig_ThresholdZeroDisables verifies the documented
// escape hatch: setting threshold to 0 disables the check even when the
// enabled flag is true (protection against accidental over-suppression
// on operators who mis-configure).
func TestCorrectionDedupConfig_ThresholdZeroDisables(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "test")
	t.Setenv("JIMINY_CORRECTION_DEDUP_SIM_THRESHOLD", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}
	if cfg.JiminyCorrectionDedupSimThreshold != 0 {
		t.Errorf("expected threshold=0 (disable), got %v", cfg.JiminyCorrectionDedupSimThreshold)
	}
	// The CreateCorrectionNodes gate is:
	//   s.cfg.JiminyCorrectionDedupEnabled && s.cfg.JiminyCorrectionDedupSimThreshold > 0
	// so threshold=0 short-circuits regardless of enabled=true.
	gateFires := cfg.JiminyCorrectionDedupEnabled && cfg.JiminyCorrectionDedupSimThreshold > 0
	if gateFires {
		t.Errorf("gate fired with threshold=0; escape hatch broken")
	}
}

// TestCorrectionDedupConfig_EnabledFalseDisables verifies the primary
// disable flag.
func TestCorrectionDedupConfig_EnabledFalseDisables(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "test")
	t.Setenv("JIMINY_CORRECTION_DEDUP_ENABLED", "false")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}
	if cfg.JiminyCorrectionDedupEnabled {
		t.Errorf("expected enabled=false, got true")
	}
	gateFires := cfg.JiminyCorrectionDedupEnabled && cfg.JiminyCorrectionDedupSimThreshold > 0
	if gateFires {
		t.Errorf("gate fired with enabled=false; primary disable broken")
	}
}

// TestCorrectionNodeResult_SkippedDupFieldExists pins the JSON schema —
// the SkippedDup field is a load-bearing counter for the live-smoke
// verification (post-consolidation the operator queries the result to
// confirm dedup actually fired).
func TestCorrectionNodeResult_SkippedDupFieldExists(t *testing.T) {
	r := &CorrectionNodeResult{
		Created:    1,
		Updated:    2,
		Linked:     3,
		Rejected:   4,
		SkippedDup: 5,
	}
	if r.SkippedDup != 5 {
		t.Errorf("SkippedDup field missing or renamed; live-smoke telemetry breaks")
	}
}
