package hidden

import (
	"strings"
	"testing"

	"mdemg/internal/config"
)

// JIMINY-CORPUS-001 Epic 1: the ConvObs→constraint promotion gate must
// reject every diagnosed junk class while passing genuine durable rules.

// defaultGateConfig mirrors the FromEnv defaults without reading the
// environment (config-level defaults are pinned in internal/config tests).
func defaultGateConfig() config.Config {
	return config.Config{
		ConstraintPromotionGateEnabled:    true,
		ConstraintPromotionDenyObsTypes:   strings.Split(config.DefaultConstraintPromotionDenyObsTypes, ","),
		ConstraintPromotionRejectPatterns: config.DefaultConstraintPromotionRejectPatterns(),
	}
}

func TestConstraintPromotionGate_RejectsJunkClasses(t *testing.T) {
	g := NewConstraintPromotionGate(defaultGateConfig())

	cases := []struct {
		name    string
		obsType string
		content string
	}{
		// Fabricated tool-status observations (pre-HOOKWIRE-001 class).
		// Live shape: obs_type=progress; content also caught pattern-wise.
		{"build_test_succeeded_provenance", "progress", "Build/test succeeded: gh pr comment 433 --body ..."},
		{"build_test_succeeded_pattern_only", "", "Build/test succeeded: cat > /tmp/x_test.go <<'EOF' ..."},
		// Bash-error observations. Live shape: obs_type=error.
		{"bash_error_provenance", "error", "Bash error in command: go build ./... && golangci-lint run ..."},
		{"bash_error_pattern_only", "", "Bash error in command: python3 docs/tests/ults/runners/ults_runner.py verify-hashes"},
		// PR status notes.
		{"pr_status_approved_merged", "decision", "PR #433 approved & merged. All required checks passed before merge."},
		{"pr_status_approved_and_merged", "learning", "The sprint PR was approved and merged after review."},
		// Sprint/phase status notes. Live shape: mostly obs_type=progress/task.
		{"sprint_status_provenance", "progress", "Sprint TSDB-CONSUME-001 complete (PR #441). V0025 retention+compression ..."},
		{"sprint_status_pattern_only", "insight", "Sprint BACKUP-RESTORE-VERIFY-001 complete, PR #434. The live round-trip caught ..."},
		{"phase_status_pattern_only", "decision", "Phase 80 COMPLETED: CMS ANN Meta-Cognition & Self-Improvement Enforcement."},
		{"task_status_provenance", "task", "OPEN MUST-FIX DEFECT — HIDDEN-CHURN-003 (incremental hidden-layer clustering)."},
		// Doc/template dumps. Live shape: obs_type=decision/learning.
		{"markdown_doc_dump", "decision", "# CMS Endpoints (Conversation Memory System)\n\n## MANDATORY: Use CMS on every session"},
		{"template_dump", "decision", "# INGESTION Plugin Template\n\n## manifest.json\n{\"id\":\"PLUGIN_ID\"}"},
		{"reference_dump", "decision", "# Plugin Troubleshooting Reference\n\n## \"binary not found\""},
		{"phase_spec_dump", "decision", "PHASE 105 SPEC: Global Meta-Learning — Cross-space promotion of L4/5 concepts."},
		{"skill_dump", "decision", "SKILL: CMS Self-Improvement — Trigger Conditions"},
		{"sprint_plan_dump", "learning", "SPRINT PLAN FORMAT v1.0 — 12 SECTIONS (7 required + 5 if-applicable):"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, rejected := g.Reject(tc.obsType, tc.content)
			if !rejected {
				t.Errorf("Reject(%q, %.60q...) = pass, want rejected", tc.obsType, tc.content)
			} else if reason == "" {
				t.Error("rejected with empty reason — rejections must be attributable")
			}
		})
	}
}

func TestConstraintPromotionGate_PassesGenuineConstraints(t *testing.T) {
	g := NewConstraintPromotionGate(defaultGateConfig())

	cases := []struct {
		name    string
		obsType string
		content string
	}{
		{"never_commit_main", "constraint", "NEVER commit directly to main branch. All development work happens on dev branches."},
		{"always_lint", "correction", "Mandatory workflow rule: Linting must be checked and pass before changes can be committed."},
		{"cuidv2_rule", "constraint", "[must] All unique identifiers in MDEMG must use CUIDv2 (nrednav/cuid2), NOT UUID v4."},
		{"env_files", "constraint", "[must] Never commit .env files to git"},
		{"user_preference_rule", "preference", "USER PREFERENCE: When planning complex features, ALWAYS use the most advanced coding model available."},
		{"untyped_genuine", "", "Never modify production database schemas without a migration file."},
		{"learning_with_rule", "learning", "Precedent-driven divergence incident: UUID v4 was used despite the CUIDv2 requirement. Never repeat."},
		// A durable rule that MENTIONS sprints/PRs must not trip the status patterns.
		{"rule_mentioning_sprint", "constraint", "All sprint development plans MUST follow the standardized 12-section format (v1.0)."},
		{"rule_mentioning_pr", "constraint", "RULE: ALWAYS add a detailed sprint summary as a PR comment every time a new PR is created."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reason, rejected := g.Reject(tc.obsType, tc.content); rejected {
				t.Errorf("Reject(%q, %.60q...) rejected (%s), want pass — the gate must not change genuine promotion", tc.obsType, tc.content, reason)
			}
		})
	}
}

func TestConstraintPromotionGate_DisabledPassesEverything(t *testing.T) {
	cfg := defaultGateConfig()
	cfg.ConstraintPromotionGateEnabled = false
	g := NewConstraintPromotionGate(cfg)
	if _, rejected := g.Reject("progress", "Build/test succeeded: anything"); rejected {
		t.Error("disabled gate rejected — CONSTRAINT_PROMOTION_GATE_ENABLED=false must pass everything")
	}
}

func TestConstraintPromotionGate_NilGatePasses(t *testing.T) {
	var g *ConstraintPromotionGate
	if _, rejected := g.Reject("progress", "Build/test succeeded: anything"); rejected {
		t.Error("nil gate rejected — hand-built Services without a gate must promote as before")
	}
}

func TestConstraintPromotionGate_ObsTypeMatchIsCaseInsensitive(t *testing.T) {
	g := NewConstraintPromotionGate(defaultGateConfig())
	if reason, rejected := g.Reject("Progress", "some transient status text"); !rejected {
		t.Error("obs_type 'Progress' passed, want case-insensitive deny match")
	} else if reason != "obs_type:progress" {
		t.Errorf("reason = %q, want obs_type:progress", reason)
	}
}

// A hand-built Config with a broken pattern must not kill the gate: the bad
// pattern is skipped with a warning, the rest still enforce.
func TestConstraintPromotionGate_InvalidPatternSkipped(t *testing.T) {
	cfg := defaultGateConfig()
	cfg.ConstraintPromotionRejectPatterns = []string{"(unclosed", "^Build/test succeeded"}
	g := NewConstraintPromotionGate(cfg)
	if _, rejected := g.Reject("", "Build/test succeeded: x"); !rejected {
		t.Error("valid pattern stopped enforcing after an invalid sibling was skipped")
	}
	if _, rejected := g.Reject("", "harmless durable rule: never do X"); rejected {
		t.Error("gate rejected content matching nothing")
	}
}
