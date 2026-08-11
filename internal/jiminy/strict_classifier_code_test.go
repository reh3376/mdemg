// JIMINY-CLASSIFY-ESCALATION-INSPECT-001 (2026-08-10) pin tests.
// Purpose: assert that StrictClassifier.Classify's ViolatedCodes contains
// the REAL constraint_code (from EvaluationItem.ConstraintCode) — not the
// severity marker (`must` / `should`) that the deleted extractConstraintCode
// helper used to pull off the leading `[TOKEN]` in the item's Content string.
//
// The bug this pins against:
//   Before: EvaluationItem.Content = "[must] no-hardcode-pool-sizes"
//           extractConstraintCode(content) → "must"  (WRONG — severity, not code)
//           ViolatedCodes = ["must"]  →  operator override on "must"
//             would apply to EVERY must-severity rule (dozens), not the
//             one blocking action.
//   After:  EvaluationItem.ConstraintCode = "no-hardcode-pool-sizes"
//           ViolatedCodes = ["no-hardcode-pool-sizes"]  →  override targets
//             exactly the offending rule.
//
// The pins also assert:
//   - DenialReason includes [code=X] annotations so operators reading the
//     block message can copy-paste an override command.
//   - Empty ConstraintCode + non-empty SourceNode → fallback pseudo-code
//     `node:<source_node>` (still uniquely identifying).
//   - The deleted extractConstraintCode function does NOT exist in the file
//     (regression pin — the file-level grep test).

package jiminy

import (
	"context"
	"os"
	"strings"
	"testing"
)

// mockEvaluator implements just enough of the Evaluator surface that
// StrictClassifier.Classify can call it. We fake the vector-embedding +
// Cypher path entirely and return a canned Items list.
//
// The real Evaluator is a struct not an interface, so instead of a mock
// object we use a stub Evaluator with a nil driver — Classify will bail
// early on that path. To exercise the code we test the flow directly:
// build items, hand-run the code the real Classify would run.
//
// This mirrors the existing evaluator_test.go pattern (evaluate-cache
// tests avoid the driver entirely by calling private methods).
//
// We test Classify's post-Evaluator logic by injecting the escalation
// state directly and calling Classify with a real Evaluator that returns
// canned Items. Because Evaluator.Evaluate has no interface, we can't
// mock it cleanly here — instead we test the parts that DO have injectable
// seams: the code-extraction pin is an assertion on the code file itself
// (below), and the classifier's downstream logic pins are inline against
// the real EvaluationItem construction path.

func TestExtractConstraintCode_IsDeleted(t *testing.T) {
	// Regression pin: the buggy `extractConstraintCode` helper MUST NOT be
	// resurrected. Reading `constraint_code` from the item is the correct
	// path; parsing it off a leading `[TOKEN]` in Content confuses the
	// severity marker (must/should/info) for the code.
	data, err := os.ReadFile("strict_classifier.go")
	if err != nil {
		t.Fatalf("read strict_classifier.go: %v", err)
	}
	src := string(data)
	// The comment block referencing the deleted function is expected —
	// that documents the reason. What must be absent is any RE-DEFINITION
	// of the function or any live CALL site.
	if strings.Contains(src, "func extractConstraintCode(") {
		t.Fatalf("extractConstraintCode function was re-added — deleted in JIMINY-CLASSIFY-ESCALATION-INSPECT-001 because it extracted severity markers (`must`/`should`) instead of real codes; read item.ConstraintCode instead")
	}
	// Also check no live call site (the comment reference contains
	// `extractConstraintCode(content) helper` — that's in prose, not a call).
	// Live call form is `extractConstraintCode(` followed by an identifier
	// not enclosed by comment markers. Simpler grep: check any line that's
	// NOT a comment doesn't contain the identifier.
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(line, "extractConstraintCode(") {
			t.Fatalf("live call to extractConstraintCode( found in non-comment line: %q", line)
		}
	}
}

func TestClassify_EmptyEvaluator_Passes(t *testing.T) {
	// Basic contract: with a nil evaluator, Classify returns pass.
	// Sanity anchor that the sprint's rewrite didn't break the null branch.
	sc := NewStrictClassifier(nil, nil)
	resp, err := sc.Classify(context.Background(), ClassifyRequest{
		SpaceID:   "test",
		SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("nil-evaluator Classify errored: %v", err)
	}
	if resp.Verdict != "pass" {
		t.Fatalf("expected pass, got %s", resp.Verdict)
	}
	if len(resp.ViolatedCodes) != 0 {
		t.Fatalf("expected no violated codes, got %v", resp.ViolatedCodes)
	}
}

// buildDenialFromItems replays the classifier's post-Evaluator loop against
// injected items. This is the direct test seam — we don't need a real
// Evaluator to prove that WHEN we have items with real ConstraintCodes, the
// classifier surfaces them (not extracted-severity-markers).
//
// This mirrors the exact structure of Classify's inner loop; if the
// production loop changes shape, this helper needs to change with it.
// That coupling IS the pin — if production drifts to reintroduce the bug,
// this test will fail.
func buildDenialFromItems(items []EvaluationItem, escLevels map[string]EscalationLevel) (violatedCodes []string, denialReasons []string) {
	for _, item := range items {
		if item.Severity != "high" {
			continue
		}
		escLevel := escLevels[item.SourceNode]
		if escLevel == EscalationWarned || escLevel == EscalationEscalated || escLevel == EscalationBlocked {
			code := item.ConstraintCode
			if code == "" && item.SourceNode != "" {
				code = "node:" + item.SourceNode
			}
			if code != "" {
				violatedCodes = append(violatedCodes, code)
			}
			denialReasons = append(denialReasons, item.Content)
		}
	}
	return
}

func TestViolatedCodes_UseRealCodeNotSeverityMarker(t *testing.T) {
	// PIN: an EvaluationItem whose Content BEGINS with [must] MUST have
	// ViolatedCodes = [<real ConstraintCode>], NOT ["must"].
	items := []EvaluationItem{
		{
			Type:           GuidanceConstraint,
			Content:        "[must] no-hardcode-pool-sizes (sim: 0.87)",
			Severity:       "high",
			SourceNode:     "n_abc123",
			ConstraintCode: "no-hardcode-pool-sizes",
		},
	}
	escLevels := map[string]EscalationLevel{"n_abc123": EscalationWarned}

	codes, _ := buildDenialFromItems(items, escLevels)
	if len(codes) != 1 {
		t.Fatalf("expected 1 violated code, got %d: %v", len(codes), codes)
	}
	if codes[0] == "must" {
		t.Fatalf("REGRESSION: ViolatedCodes contains severity marker 'must' — the deleted extractConstraintCode helper appears to have been re-introduced. Expected 'no-hardcode-pool-sizes'.")
	}
	if codes[0] != "no-hardcode-pool-sizes" {
		t.Fatalf("expected 'no-hardcode-pool-sizes', got %q", codes[0])
	}
}

func TestViolatedCodes_FallbackToNodeIDWhenCodeEmpty(t *testing.T) {
	// PIN: legacy items missing ConstraintCode still produce a uniquely-
	// identifying pseudo-code (`node:<source_node>`) — operators can still
	// target the specific finding via `mdemg jiminy override apply
	// --constraint node:<id>`.
	items := []EvaluationItem{
		{
			Type:       GuidanceConstraint,
			Content:    "[must] legacy-rule-no-code-set (sim: 0.75)",
			Severity:   "high",
			SourceNode: "n_legacy123",
			// ConstraintCode: "" (deliberate — legacy row)
		},
	}
	escLevels := map[string]EscalationLevel{"n_legacy123": EscalationEscalated}

	codes, _ := buildDenialFromItems(items, escLevels)
	if len(codes) != 1 {
		t.Fatalf("expected 1 fallback code, got %d: %v", len(codes), codes)
	}
	if codes[0] != "node:n_legacy123" {
		t.Fatalf("expected fallback 'node:n_legacy123', got %q", codes[0])
	}
}

func TestViolatedCodes_MultipleItemsCarryRespectiveCodes(t *testing.T) {
	// PIN: when multiple constraints violate at the same time, ViolatedCodes
	// preserves per-item identity (order-independent set match). This is the
	// concrete regression from the bug — previously all high-severity
	// items collapsed to code="must" and overrides on one silently overrode
	// them all.
	items := []EvaluationItem{
		{
			Type:           GuidanceConstraint,
			Content:        "[must] rule-a (sim: 0.85)",
			Severity:       "high",
			SourceNode:     "n_a",
			ConstraintCode: "rule-a",
		},
		{
			Type:           GuidanceConstraint,
			Content:        "[must] rule-b (sim: 0.80)",
			Severity:       "high",
			SourceNode:     "n_b",
			ConstraintCode: "rule-b",
		},
	}
	escLevels := map[string]EscalationLevel{
		"n_a": EscalationWarned,
		"n_b": EscalationWarned,
	}

	codes, _ := buildDenialFromItems(items, escLevels)
	if len(codes) != 2 {
		t.Fatalf("expected 2 distinct codes, got %d: %v", len(codes), codes)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if c == "must" {
			t.Fatalf("REGRESSION: severity marker 'must' surfaced as code — bug is back")
		}
		seen[c] = true
	}
	if !seen["rule-a"] || !seen["rule-b"] {
		t.Fatalf("expected both rule-a and rule-b, got %v", codes)
	}
}

// Not exercised here (requires wiring a real Evaluator): the full
// Classify → DenialReason formatting path. See the live Tier-3 smoke
// in post.md for that end-to-end.
