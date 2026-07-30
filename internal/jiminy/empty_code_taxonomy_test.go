package jiminy

// JIMINY-CODE-BACKFILL-001: pin test — the empty-`constraint_code` classes in
// constraint_outcomes are DELIBERATE for a documented subset of guidance types.
// The taxonomy below must stay in sync with:
//   - the CLAUDE.md pin "empty constraint_code is expected for …"
//   - the defensive backfill in service.go::RecordOutcome (only fires for
//     GuidanceConstraint items with an empty code)
//   - the docs/development/jiminy-code-backfill-001/post.md investigation
//
// This test pins the semantic: guidance types that DON'T map to a codified
// constraint node MUST NOT be backfilled with a code. Changing the taxonomy
// requires updating the doc pin and (potentially) the follow-up
// CORRECTION-CODE-GEN-001 spec.

import "testing"

// guidanceTypesWithoutCodes enumerates GuidanceType values that legitimately
// have empty constraint_code in constraint_outcomes. Codes only apply to
// role_type='constraint' Neo4j nodes; guidance items whose source isn't a
// constraint node have no code to carry.
//
// Correction is INCLUDED for now because role_type='correction' nodes don't
// carry constraint_code today (CORRECTION-CODE-GEN-001 follow-up).
// When corrections DO get codes, remove GuidanceCorrection from this list
// and update the CLAUDE.md pin in the same PR.
var guidanceTypesWithoutCodes = map[GuidanceType]string{
	GuidancePattern:    "concept-abstracted; no codified constraint",
	GuidanceConcept:    "L2+ emergent-concept; no codified constraint",
	GuidanceLearning:   "insight/context/technical_note obs_type; no codified constraint",
	GuidanceDecision:   "decision/task obs_type; no codified constraint",
	GuidanceCorrection: "correction nodes don't carry constraint_code today (CORRECTION-CODE-GEN-001 follow-up)",
	GuidancePreference: "user preference obs; no codified constraint",
	GuidanceRisk:       "error/blocker obs; no codified constraint",
	GuidanceConflict:   "cross-source conflict summary; no codified constraint",
}

// guidanceTypesWithCodes enumerates GuidanceType values that SHOULD carry a
// non-empty constraint_code when the source is a real constraint node.
// Currently just GuidanceConstraint.
var guidanceTypesWithCodes = map[GuidanceType]string{
	GuidanceConstraint: "role_type='constraint' L1 nodes carry constraint_code",
}

func TestEmptyConstraintCodeTaxonomy_Disjoint(t *testing.T) {
	// A type must be classified as either "with code" or "without code",
	// never both. Ensures the taxonomy remains a partition.
	for gt := range guidanceTypesWithCodes {
		if _, dup := guidanceTypesWithoutCodes[gt]; dup {
			t.Errorf("guidance type %q appears in both with-codes AND without-codes taxonomies — must be a partition", gt)
		}
	}
}

func TestEmptyConstraintCodeTaxonomy_Complete(t *testing.T) {
	// Every GuidanceType enum value must be classified. New types added to
	// types.go MUST be added to one of the two taxonomy maps above (with a
	// justification comment). Missing entries would silently allow the
	// empty-code class to grow again.
	allTypes := []GuidanceType{
		GuidanceConstraint, GuidanceCorrection, GuidancePattern,
		GuidanceConcept, GuidanceLearning, GuidanceDecision,
		GuidancePreference, GuidanceRisk, GuidanceConflict,
	}
	for _, gt := range allTypes {
		_, hasCode := guidanceTypesWithCodes[gt]
		_, noCode := guidanceTypesWithoutCodes[gt]
		if !hasCode && !noCode {
			t.Errorf("guidance type %q not classified — add to guidanceTypesWithCodes or guidanceTypesWithoutCodes with a justification", gt)
		}
	}
}
