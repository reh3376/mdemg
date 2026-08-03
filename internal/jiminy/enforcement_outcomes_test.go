package jiminy

import (
	"reflect"
	"testing"
)

// JIMINY-ENFORCE-004 — extractOverriddenCodes pin tests.
// The classifier's suppression path emits "[override:CODE reason=…]"
// annotations in the DenialReason string. The outcome writer parses them
// back out to emit per-code blocked_false_positive rows. This test pins
// the round-trip semantics.

func TestExtractOverriddenCodes_SingleOverride(t *testing.T) {
	reason := "override-suppressed: [override:MYRULE reason=\"false positive\"]"
	got := extractOverriddenCodes(reason)
	want := []string{"MYRULE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("single override: got %v, want %v", got, want)
	}
}

func TestExtractOverriddenCodes_MultipleOverrides(t *testing.T) {
	reason := "override-suppressed: [override:RULE-A reason=\"r1\"]; [override:RULE-B reason=\"r2\"]"
	got := extractOverriddenCodes(reason)
	want := []string{"RULE-A", "RULE-B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("multiple overrides: got %v, want %v", got, want)
	}
}

func TestExtractOverriddenCodes_PartialOverrideMessage(t *testing.T) {
	// Partial-override deny message: shipped classifier format includes
	// both the surviving deny + the (partial-override-suppressed: …) list.
	reason := `Constraint violation (warned): [must-x] blah (partial-override-suppressed: [override:MAYBE-DEP reason="not relevant"])`
	got := extractOverriddenCodes(reason)
	want := []string{"MAYBE-DEP"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("partial override: got %v, want %v", got, want)
	}
}

func TestExtractOverriddenCodes_NoOverrides(t *testing.T) {
	// Plain deny reason with no annotations returns nil.
	reason := "Constraint violation (warned): [must-x] some reason"
	got := extractOverriddenCodes(reason)
	if len(got) != 0 {
		t.Errorf("no overrides: got %v, want empty", got)
	}
}

func TestExtractOverriddenCodes_MalformedNoBracket(t *testing.T) {
	// If a "[override:" marker starts but no space/] terminator follows,
	// the extractor stops cleanly (no infinite loop, no panic).
	reason := "[override:BROKEN"
	got := extractOverriddenCodes(reason)
	if len(got) != 0 {
		t.Errorf("malformed no-bracket: got %v, want empty", got)
	}
}

func TestExtractOverriddenCodes_CodeWithBracketBeforeSpace(t *testing.T) {
	// If the ] comes BEFORE the space (e.g. "[override:CODE]"), extractor
	// takes the ] as the terminator — correct for the sparse-annotation case.
	reason := "[override:CODE]"
	got := extractOverriddenCodes(reason)
	want := []string{"CODE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bracket-before-space: got %v, want %v", got, want)
	}
}
