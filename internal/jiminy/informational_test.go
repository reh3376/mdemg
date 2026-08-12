// Sprint JIMINY-INFORMATIONAL-CATEGORY-001 (2026-08-12) — pin tests.

package jiminy

import (
	"context"
	"testing"
)

// TestLoadInformationalNodeSet_NilDriverReturnsEmpty pins the safe-default
// behavior: when the Service has no Neo4j driver (unit-test setup, disabled
// jiminy, or driver-open failure), the loader returns an EMPTY set — not
// nil — so the RecordOutcome override loop treats every source_node as NOT
// informational and grades normally. False positives (marking something
// informational that isn't) are the failure mode we're WILLING to have
// (agent grades a real rule); false negatives (silently skipping items
// due to unavailability) would corrupt the actionable follow-rate signal.
func TestLoadInformationalNodeSet_NilDriverReturnsEmpty(t *testing.T) {
	s := &Service{driver: nil}
	got := s.loadInformationalNodeSet(context.Background(), []string{"n1", "n2", "n3"})
	if got == nil {
		t.Fatal("nil-driver call returned nil map; expected empty map so callers can safely map-index without a nil check")
	}
	if len(got) != 0 {
		t.Fatalf("nil-driver call returned non-empty map (%d entries); expected empty", len(got))
	}
	// map-indexing on the empty map returns the zero value (false) — verify.
	if got["n1"] {
		t.Fatal("empty map returned true for missing key; would incorrectly override outcome to NotApplicable")
	}
}

// TestLoadInformationalNodeSet_EmptyInputReturnsEmpty pins the short-circuit
// when the caller has no source_node_ids to look up (guidance items with
// empty SourceNodes lists — legitimate for anonymous/generated items).
func TestLoadInformationalNodeSet_EmptyInputReturnsEmpty(t *testing.T) {
	s := &Service{driver: nil}
	got := s.loadInformationalNodeSet(context.Background(), []string{})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty-input call: expected empty map, got nil=%v len=%d", got == nil, len(got))
	}
	got = s.loadInformationalNodeSet(context.Background(), nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("nil-input call: expected empty map, got nil=%v len=%d", got == nil, len(got))
	}
	// Skip-blanks — a slice of empty strings should not query.
	got = s.loadInformationalNodeSet(context.Background(), []string{"", "", ""})
	if got == nil || len(got) != 0 {
		t.Fatalf("all-blank input: expected empty map, got nil=%v len=%d", got == nil, len(got))
	}
}

// TestInformationalOverride_LogicMirror pins the OVERRIDE decision that
// RecordOutcome makes in-line (service.go around line 1899): when an item's
// source node is in the informational set AND the outcome is not already
// NotApplicable, the outcome MUST be overridden to NotApplicable.
//
// This is a logic-mirror test — the real check is embedded in the middle
// of a Neo4j-dependent function; testing the logic shape here catches
// accidental inversion of the condition or wrong-outcome selection during
// future refactors.
func TestInformationalOverride_LogicMirror(t *testing.T) {
	// Reproduce the exact predicate + override from service.go RecordOutcome.
	overrideIfInformational := func(sourceNodes []string, informationalSet map[string]bool, in GuidanceOutcome) GuidanceOutcome {
		outcome := in
		if len(informationalSet) > 0 && len(sourceNodes) > 0 && outcome != OutcomeNotApplicable {
			for _, nodeID := range sourceNodes {
				if informationalSet[nodeID] {
					outcome = OutcomeNotApplicable
					break
				}
			}
		}
		return outcome
	}

	cases := []struct {
		name  string
		src   []string
		info  map[string]bool
		in    GuidanceOutcome
		want  GuidanceOutcome
	}{
		{"informational source overrides Followed → NotApplicable",
			[]string{"n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomeFollowed, OutcomeNotApplicable},
		{"informational source overrides Ignored → NotApplicable",
			[]string{"n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomeIgnored, OutcomeNotApplicable},
		{"informational source overrides PartialCompliance → NotApplicable",
			[]string{"n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomePartialCompliance, OutcomeNotApplicable},
		{"informational source overrides Contradicted → NotApplicable",
			[]string{"n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomeContradicted, OutcomeNotApplicable},
		{"NotApplicable in stays NotApplicable (no double-override)",
			[]string{"n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomeNotApplicable, OutcomeNotApplicable},
		{"non-informational source doesn't override",
			[]string{"n_actionable"}, map[string]bool{"n_meta_1": true}, OutcomeFollowed, OutcomeFollowed},
		{"empty informational set doesn't override",
			[]string{"n_meta_1"}, map[string]bool{}, OutcomeFollowed, OutcomeFollowed},
		{"empty source list doesn't override",
			[]string{}, map[string]bool{"n_meta_1": true}, OutcomeIgnored, OutcomeIgnored},
		{"multi-source: any informational source triggers override",
			[]string{"n_actionable", "n_meta_1"}, map[string]bool{"n_meta_1": true}, OutcomeFollowed, OutcomeNotApplicable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := overrideIfInformational(tc.src, tc.info, tc.in)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
