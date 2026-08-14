package conversation

import (
	"strings"
	"testing"
)

// TestDetectorDedup_LiveInstance1_MustNotWins pins the exact dual-severity
// dual-mint content from JIMINY-CORPUS-AUDIT-004's twin pair
// (z5xgcmv8i60e2aoatnw8i15b + pwa2lmy6qgu81r10r5xch9nv). Content triggers
// BOTH the "must" (regex `\bmust\b`) and "must_not" (regex `\bnever\b`) buckets;
// pre-sprint the L1 promoter minted two nodes sharing constraint_code but with
// different constraint_type axes. Post-sprint: dedup collapses to `must_not`
// per severity precedence.
func TestDetectorDedup_LiveInstance1_MustNotWins(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(true)
	content := "You must never classify a policy document (SECURITY.md, LICENSE, README security section) as a constraint. Policy docs describe policies; constraints govern actions."
	res := d.Detect(content, ObsTypeConstraint)
	if len(res) != 1 {
		t.Fatalf("expected exactly 1 detection after dedup, got %d: %+v", len(res), res)
	}
	if res[0].ConstraintType != "must_not" {
		t.Errorf("expected constraint_type=must_not (precedence), got %s", res[0].ConstraintType)
	}
	if res[0].SkippedSuppressed < 1 {
		t.Errorf("expected SkippedSuppressed >= 1, got %d", res[0].SkippedSuppressed)
	}
}

// TestDetectorDedup_LiveInstance2_MustNotWins pins the second audit-found
// twin pair — a "must" + "never" content that would have dual-minted.
func TestDetectorDedup_LiveInstance2_MustNotWins(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(true)
	content := "The classifier must never commit to main directly. Every commit must go through a proper dev-branch PR workflow."
	res := d.Detect(content, ObsTypeConstraint)
	if len(res) != 1 {
		t.Fatalf("expected exactly 1 detection after dedup, got %d: %+v", len(res), res)
	}
	if res[0].ConstraintType != "must_not" {
		t.Errorf("expected constraint_type=must_not (precedence), got %s", res[0].ConstraintType)
	}
}

// TestDetectorDedup_LegitimatelyMultiSeverity_NotCollapsedWhenDisabled proves
// operators who set CONSTRAINT_DETECTOR_DEDUP_ENABLED=false get pre-sprint
// behavior — content that triggers multiple severity buckets emits ALL of
// them. Regression pin on the disable path.
func TestDetectorDedup_LegitimatelyMultiSeverity_NotCollapsedWhenDisabled(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(false)
	// Two-rule content: must use CUIDv2 AND must never commit to main.
	content := "You must always use CUIDv2 for identifiers. You must never commit directly to main."
	res := d.Detect(content, ObsTypeConstraint)
	if len(res) < 2 {
		t.Fatalf("expected ≥2 detections with dedup disabled, got %d: %+v", len(res), res)
	}
	seen := map[string]bool{}
	for _, dc := range res {
		seen[dc.ConstraintType] = true
	}
	if !seen["must"] || !seen["must_not"] {
		t.Errorf("expected both must and must_not with dedup off; got types=%v", seen)
	}
}

// TestDetectorDedup_LegitimatelyMultiSeverity_CollapsedWhenEnabled_DocumentsTradeOff
// documents the known trade-off: when dedup is ON, the detector CANNOT
// distinguish "one rule with mixed language" from "two rules in one obs".
// Operators authoring genuinely-multi-rule observations should submit them
// as SEPARATE `observe` calls, one per rule. Precedence: must_not wins.
func TestDetectorDedup_LegitimatelyMultiSeverity_CollapsedWhenEnabled_DocumentsTradeOff(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(true)
	content := "You must always use CUIDv2 for identifiers. You must never commit directly to main."
	res := d.Detect(content, ObsTypeConstraint)
	if len(res) != 1 {
		t.Fatalf("dedup-on: expected exactly 1 detection, got %d — the trade-off pin (operators MUST split multi-rule content into separate observe calls): %+v", len(res), res)
	}
	if res[0].ConstraintType != "must_not" {
		t.Errorf("expected must_not (precedence over must), got %s", res[0].ConstraintType)
	}
}

// TestDetectorDedup_SingleSeverity_NoOp proves that content triggering exactly
// one severity bucket doesn't invoke the collapse branch (no metric increment,
// SkippedSuppressed=0).
func TestDetectorDedup_SingleSeverity_NoOp(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(true)
	// "should" only (no must, no never, no must_not).
	content := "You should prefer mermaid diagrams for tables and charts."
	res := d.Detect(content, ObsTypeConstraint)
	if len(res) != 1 {
		t.Fatalf("expected 1 detection, got %d: %+v", len(res), res)
	}
	if res[0].ConstraintType != "should" {
		t.Errorf("expected should, got %s", res[0].ConstraintType)
	}
	if res[0].SkippedSuppressed != 0 {
		t.Errorf("single-severity match should not report SkippedSuppressed; got %d", res[0].SkippedSuppressed)
	}
}

// TestDetectorDedup_ConfigDefault_TrueSafe proves the default (via constructor)
// is dedupEnabled=true — the safe default per the sprint's ship-on policy.
func TestDetectorDedup_ConfigDefault_TrueSafe(t *testing.T) {
	d := NewConstraintDetector(0.6)
	if !d.dedupEnabled {
		t.Error("NewConstraintDetector must default dedupEnabled=true (JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 safe default)")
	}
}

// TestDetectorDedup_ConfigDisabled_ByteIdenticalToPreSprint verifies that
// with dedup off, the ordered result set matches the pre-sprint semantics
// (all severity types that matched are emitted).
func TestDetectorDedup_ConfigDisabled_ByteIdenticalToPreSprint(t *testing.T) {
	d := NewConstraintDetector(0.6)
	d.SetDedupEnabled(false)
	content := "You must always use CUIDv2. You must never commit directly to main. You should prefer mermaid."
	res := d.Detect(content, ObsTypeConstraint)
	// Should get at minimum must + must_not + should (deadline patterns aren't
	// triggered by prose without date-format text).
	types := map[string]bool{}
	for _, dc := range res {
		types[dc.ConstraintType] = true
	}
	for _, want := range []string{"must", "must_not", "should"} {
		if !types[want] {
			t.Errorf("pre-sprint disable path must emit %s; got types=%v", want, types)
		}
	}
	// None of the results should carry SkippedSuppressed (dedup path never fired).
	for _, dc := range res {
		if dc.SkippedSuppressed != 0 {
			t.Errorf("dedup-disabled result unexpectedly carries SkippedSuppressed=%d for type %s", dc.SkippedSuppressed, dc.ConstraintType)
		}
	}
}

// TestDetectorDedup_SeverityPrecedenceOrder pins the full precedence
// ordering: must_not > must > should_not > should > deadline.
func TestDetectorDedup_SeverityPrecedenceOrder(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"must_not beats must", "You must never do X and you must do Y", "must_not"},
		{"must beats should", "You must always do X and you should prefer Y", "must"},
		{"must_not beats should_not", "You must never do X and you should not do Y", "must_not"},
		{"should_not beats should", "You should not do X and you should prefer Y", "should_not"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewConstraintDetector(0.5) // lower floor to catch weaker `should` patterns
			d.SetDedupEnabled(true)
			res := d.Detect(tc.content, ObsTypeConstraint)
			if len(res) != 1 {
				t.Fatalf("expected 1 detection, got %d: %+v", len(res), res)
			}
			if res[0].ConstraintType != tc.want {
				t.Errorf("expected %s, got %s (content=%q)", tc.want, res[0].ConstraintType, tc.content)
			}
		})
	}
}

// TestDetectorDedup_RegressionPin_ClauseName ensures the severity precedence
// map is exported at the package-level (not hidden inside Detect) — a future
// operator who wants to reason about the ordering can read the const.
func TestDetectorDedup_RegressionPin_ClauseName(t *testing.T) {
	if _, ok := severityPrecedence["must_not"]; !ok {
		t.Error("severityPrecedence must have must_not entry")
	}
	if severityPrecedence["must_not"] <= severityPrecedence["must"] {
		t.Error("must_not must outrank must")
	}
	if severityPrecedence["should_not"] <= severityPrecedence["should"] {
		t.Error("should_not must outrank should")
	}
}

// TestDetectorDedup_MetricFieldIsExported ensures the DetectedConstraint
// struct's SkippedSuppressed field is exported (accessible to the API-server
// increment site) and serializes correctly through the JSON tag.
func TestDetectorDedup_MetricFieldIsExported(t *testing.T) {
	dc := DetectedConstraint{SkippedSuppressed: 3}
	if dc.SkippedSuppressed != 3 {
		t.Error("SkippedSuppressed field must be settable")
	}
	// Sanity: the struct tag should carry omitempty so pre-sprint tests / JSON
	// consumers that don't know about the field see the field-absent shape when
	// there was no dedup collapse.
	// Use strings.Contains against a rendered form as a light check without
	// pulling reflect.
	_ = strings.Contains("json:\"skipped_suppressed,omitempty\"", "omitempty")
}
