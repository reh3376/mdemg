package hidden

import (
	"testing"

	"mdemg/internal/config"
)

// JIMINY-CORRECTION-PRODUCER-001 Epic 2: the L0 correction → L1 promotion
// gate must reject pathological content shapes (min length, doc/skill/
// completion-status dumps) while passing every genuine L0 correction shape
// observed live on mdemg-dev.

// defaultCorrectionGateConfig mirrors the FromEnv defaults without reading
// the environment.
func defaultCorrectionGateConfig() config.Config {
	return config.Config{
		CorrectionPromotionEnabled:        true,
		CorrectionPromotionMinContentLen:  20,
		CorrectionPromotionRejectPatterns: config.DefaultConstraintPromotionRejectPatterns(),
	}
}

func TestCorrectionPromotionGate_RejectsJunkShapes(t *testing.T) {
	g := NewCorrectionPromotionGate(defaultCorrectionGateConfig())

	cases := []struct {
		name    string
		content string
	}{
		// The same junk classes the constraint gate rejects — a correction
		// obs whose content is actually a build-status or PR-status dump
		// should not be promoted either.
		{"build_test_dump", "Build/test succeeded: gh pr comment 499 --body ..."},
		{"bash_error", "Bash error in command: go build ./... && golangci-lint run ..."},
		{"pr_status_merged", "PR #433 approved & merged. All required checks passed before merge."},
		{"sprint_status", "Sprint TSDB-CONSUME-001 complete (PR #441). V0025 retention+compression ..."},
		{"phase_status", "Phase 80 COMPLETED: CMS ANN Meta-Cognition & Self-Improvement Enforcement."},
		{"markdown_doc_dump", "# CMS Endpoints (Conversation Memory System)\n\n## MANDATORY: Use CMS on every session"},
		{"phase_spec_dump", "PHASE 105 SPEC: Global Meta-Learning — Cross-space promotion of L4/5 concepts."},
		{"skill_dump", "skill: memory-preservation-backup-integrity — before destructive maintenance ..."},
		{"sprint_plan_dump", "sprint plan v1.0 12-section format required for all sprint work."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, rejected := g.Reject(tc.content)
			if !rejected {
				t.Errorf("Reject(%.60q...) passed, want rejected", tc.content)
			}
			if reason == "" {
				t.Errorf("Reject returned empty reason for rejection")
			}
		})
	}
}

func TestCorrectionPromotionGate_RejectsTooShort(t *testing.T) {
	g := NewCorrectionPromotionGate(defaultCorrectionGateConfig())
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"one_word", "correct"},
		{"short_fragment", "use X not Y"}, // 11 chars — under the 20 default
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, rejected := g.Reject(tc.content)
			if !rejected {
				t.Errorf("Reject(%q) passed, want rejected for min_content_len", tc.content)
			}
			if reason != "content_too_short" {
				t.Errorf("reason = %q, want content_too_short", reason)
			}
		})
	}
}

func TestCorrectionPromotionGate_PassesGenuineCorrections(t *testing.T) {
	g := NewCorrectionPromotionGate(defaultCorrectionGateConfig())

	// Sampled live from mdemg-dev — every one of these is a real durable
	// correction that must be promoted, not blocked.
	cases := []struct {
		name    string
		content string
	}{
		{"correction_reinforcement", "CORRECTION REINFORCEMENT: never trust an unordered sample for novelty — the earlier assumption was wrong and must not recur."},
		{"correction_use_ordered", "CORRECTION: the previous approach was wrong — never use the unordered LIMIT sample for novelty; this must be replaced. The correct pattern is an ORDER BY of ..."},
		{"recurring_bug_openai", "RECURRING BUG (3rd occurrence): OpenAI chat/completions API — newer models (gpt-5.x, gpt-4.1+) REQUIRE the `max_completion_tokens` parameter, not the legacy `max_tokens`. Both must be provided as separate keys because the code has diverged."},
		{"correction_sprint_plans_location", "CORRECTION: Sprint plans (and all project planning docs) belong in docs/development/<sprint-line>/ inside the repo — NOT in ~/Downloads/ or any ad-hoc scratch location."},
		{"correction_no_hardcoded_pool", "CORRECTION: Do not hardcode connection pool sizes. Use configuration from environment variables with sensible defaults."},
		{"correction_sequential_epics", "CORRECTION: Do NOT parallelize epics. Epic 1 (Foundation/Documentation) must complete fully before Epic 2 (Causal Insertion) begins."},
		{"correction_planning_protocol", "At 105+ phases, 2000+ line API docs, and 60+ files per commit, the planning protocol is not overhead — it is the only thing keeping the project coherent."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reason, rejected := g.Reject(tc.content); rejected {
				t.Errorf("Reject(%.60q...) rejected (%s), want pass — the gate must not block genuine corrections", tc.content, reason)
			}
		})
	}
}

func TestCorrectionPromotionGate_DisabledPassesEverything(t *testing.T) {
	cfg := defaultCorrectionGateConfig()
	cfg.CorrectionPromotionEnabled = false
	g := NewCorrectionPromotionGate(cfg)
	if _, rejected := g.Reject(""); rejected {
		t.Error("disabled gate rejected empty content — CORRECTION_PROMOTION_ENABLED=false must pass everything")
	}
	if _, rejected := g.Reject("Build/test succeeded: anything"); rejected {
		t.Error("disabled gate rejected junk — CORRECTION_PROMOTION_ENABLED=false must pass everything")
	}
}

func TestCorrectionPromotionGate_NilGatePasses(t *testing.T) {
	var g *CorrectionPromotionGate
	if _, rejected := g.Reject("Build/test succeeded: anything"); rejected {
		t.Error("nil gate rejected — hand-built Services without a gate must promote as before")
	}
}

// A hand-built Config with a broken pattern must not kill the gate: the bad
// pattern is skipped with a warning, the rest still enforce.
func TestCorrectionPromotionGate_InvalidPatternSkipped(t *testing.T) {
	cfg := defaultCorrectionGateConfig()
	cfg.CorrectionPromotionRejectPatterns = []string{"(unclosed", "^Build/test succeeded"}
	g := NewCorrectionPromotionGate(cfg)
	if _, rejected := g.Reject("Build/test succeeded: x — this content is 40+ chars so length passes"); !rejected {
		t.Error("valid pattern stopped enforcing after an invalid sibling was skipped")
	}
	if _, rejected := g.Reject("harmless durable rule: never do X, always do Y instead"); rejected {
		t.Error("gate rejected content matching nothing")
	}
}

// MinContentLen=0 disables the length gate — only patterns fire.
func TestCorrectionPromotionGate_MinContentLenZeroDisablesLengthGate(t *testing.T) {
	cfg := defaultCorrectionGateConfig()
	cfg.CorrectionPromotionMinContentLen = 0
	g := NewCorrectionPromotionGate(cfg)
	if _, rejected := g.Reject("use X"); rejected {
		t.Error("min_content_len=0 must disable the length gate")
	}
}
