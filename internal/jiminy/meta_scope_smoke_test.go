//go:build metascope_smoke

package jiminy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestMetaScopeSmoke — JIMINY-CLASSIFIER-META-SCOPE-001 Epic 4 live Tier-3
// fixture set. Runs 6 real content examples (4 flip targets + 2 counter-
// fixtures) under two configurations: baseline (3 shipped credits ON,
// MentionVsPerform OFF) and fix (all 4 credits ON). Reports flip count +
// any counter-fixture regressions.
//
// Run: go test -tags metascope_smoke ./internal/jiminy/ -run TestMetaScopeSmoke -v
// Requires: llama-server reachable at LLM_ENDPOINT (default http://127.0.0.1:8102/v1).
//
// Success: ≥4 clean flips (`ignored`→`not_applicable`) on the 4 non-counter
// fixtures; 0 regressions on the 2 counter fixtures.
func TestMetaScopeSmoke(t *testing.T) {
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8102/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "mdemg-llm-v1"
	}
	fixtures := []struct {
		name        string
		constraint  string
		action      string
		oldExpected string
		newExpected string
		isCounter   bool
	}{
		{
			name:        "F1 doc-edit adds CLAUDE.md pin quoting commit rule",
			constraint:  "never commit directly to main. All commits must go through a proper branching and pull request workflow; the main branch should not be modified directly.",
			action:      "Edited CLAUDE.md: added a pin quoting the never-commit-directly-to-main rule under the JIMINY-ENFORCE lineage.",
			oldExpected: "ignored",
			newExpected: "not_applicable",
		},
		{
			name:        "F2 sprint plan describes CUIDv2 generate",
			constraint:  "All unique identifiers in MDEMG must use CUIDv2 (nrednav/cuid2), NOT UUID. Every new identifier at record-mint time must generate CUIDv2.",
			action:      "Edited docs/development/id-strategy/sprint_plan.md: wrote a section describing 'generate CUIDv2 at record-mint time' as the identity mechanism.",
			oldExpected: "ignored",
			newExpected: "not_applicable",
		},
		{
			name:        "F3 sprint plan describes ALTER TABLE migration",
			constraint:  "Never modify production database schemas without a migration file. ALTER TABLE and DROP TABLE are prohibited outside of migrations.",
			action:      "Edited docs/development/schema-refactor/sprint_plan.md: authored a plan describing an ALTER TABLE constraints ADD COLUMN role_type migration approach.",
			oldExpected: "ignored",
			newExpected: "not_applicable",
		},
		{
			name:        "F4 (counter) doc-authoring perform — mermaid in feature doc",
			constraint:  "Always create tables and charts in markdown files with mermaid diagrams, not ASCII.",
			action:      "Edited docs/features/jiminy-actionability.md: added a section containing a mermaid diagram block visualizing the compliance credit flow.",
			oldExpected: "followed",
			newExpected: "followed",
			isCounter:   true,
		},
		{
			name:        "F5 CLAUDE.md prose quotes pre-commit lint rule",
			constraint:  "Mandatory workflow rule: Linting must be checked and pass before changes can be committed.",
			action:      "Edited docs/features/jiminy-actionability.md: added prose text quoting the pre-commit lint rule verbatim.",
			oldExpected: "ignored",
			newExpected: "not_applicable",
		},
		{
			name:        "F6 (counter) actual git commit performed",
			constraint:  "never commit directly to main. All commits must go through a proper branching and pull request workflow.",
			action:      "Ran `git commit -m 'meta-scope-001: draft clause' && git push origin reh3376_dev01`.",
			oldExpected: "followed",
			newExpected: "followed",
			isCounter:   true,
		},
	}

	baseCfg := OutcomeClassifierConfig{
		LLMEnabled:             true,
		LLMProvider:            "openai",
		LLMModel:               model,
		LLMBaseURL:             endpoint,
		NonViolationCredit:     true,
		ContextMismatchCredit:  true,
		MechanismScopeCredit:   true,
		MentionVsPerformCredit: false,
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: 0.10,
		MaxTokens:              500,
	}
	fixCfg := baseCfg
	fixCfg.MentionVsPerformCredit = true

	baseOC := NewOutcomeClassifier(nil, baseCfg)
	fixOC := NewOutcomeClassifier(nil, fixCfg)

	type result struct {
		Name        string `json:"name"`
		IsCounter   bool   `json:"is_counter"`
		OldExpected string `json:"old_expected"`
		NewExpected string `json:"new_expected"`
		BaseVerdict string `json:"base_verdict"`
		FixVerdict  string `json:"fix_verdict"`
		Result      string `json:"result"`
	}
	rows := []result{}
	flips := 0
	regressions := 0
	for _, f := range fixtures {
		t.Logf("running: %s", f.name)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		item := GuidanceItem{Content: f.constraint, Type: GuidanceConstraint}
		// Bypass tier1 (no embedder in this smoke); call llmClassify directly
		// with a similarity of 0.4 (mid-band — LLM tier-2 is the decision maker).
		base := baseOC.llmClassify(ctx, item, f.action, 0.4, false, "")
		fix := fixOC.llmClassify(ctx, item, f.action, 0.4, false, "")
		cancel()
		vb := string(base.Outcome)
		vf := string(fix.Outcome)
		row := result{
			Name:        f.name,
			IsCounter:   f.isCounter,
			OldExpected: f.oldExpected,
			NewExpected: f.newExpected,
			BaseVerdict: vb,
			FixVerdict:  vf,
		}
		switch {
		case !f.isCounter && vb == "ignored" && vf == "not_applicable":
			flips++
			row.Result = "FLIP_OK"
		case f.isCounter && vb == vf:
			row.Result = "COUNTER_STABLE"
		case f.isCounter && vf == "not_applicable" && f.newExpected != "not_applicable":
			regressions++
			row.Result = "REGRESSION_OVERCORRECTION"
		default:
			row.Result = fmt.Sprintf("PARTIAL base=%s fix=%s", vb, vf)
		}
		rows = append(rows, row)
	}
	summary := map[string]any{"flips": flips, "regressions": regressions, "fixtures": rows}
	out, _ := json.MarshalIndent(summary, "", "  ")
	t.Logf("\n=== META-SCOPE-001 Tier-3 Smoke Result ===\n%s\n", string(out))

	// Live smoke calibration (2026-08-14 initial runs):
	// Two runs against local mdemg-llm-v1 with the same 6 fixtures produced
	// inconsistent verdicts — the LLM's classification of ambiguous doc-edit
	// actions is high-variance across runs, and the shipped 3-credit CONTEXT-002
	// baseline already routes most fixtures appropriately.
	//
	// Reproducible finding: the MechanismScopeCredit clause (CONTEXT-002) is
	// already doing most of the work; the mention-vs-perform clause adds
	// marginal refinement only on unambiguous mention-only cases (F1 pin
	// quoting rule, F5 prose quoting rule) — and even those are LLM-run-
	// dependent. 0 counter-fixture regressions across both runs = safety
	// envelope intact.
	//
	// Decision: ship the code + tests + docs as DORMANT capability
	// (default-off in code AND .env). Do not flip the .env flag until the
	// CEILING-BREAK-2 T+168h passive re-check shows evidence CONTEXT-002
	// alone underdelivers.
	//
	// This smoke is now informational only — it does NOT fail the build,
	// only reports variance for operator awareness. Regression on
	// counter-fixtures (over-correction on real perform actions) IS a fail.
	if regressions > 0 {
		t.Errorf("counter-fixture regression detected: %d over-corrections (expected 0) — over-correction is the fail condition", regressions)
	}
	t.Logf("META-SCOPE-001 smoke: flips=%d (LLM variance is high; ship-dormant decision documented above)", flips)
}
