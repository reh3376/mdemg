package jiminy

import (
	"testing"
)

// JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 — pin tests for the content-quality
// gate on contradicted-draft emission.

func TestBridgeGate_NilPassesEverything(t *testing.T) {
	var g *ContradictedBridgeGate
	if reason, rejected := g.Reject(GuidanceConstraint, "any content"); rejected {
		t.Errorf("nil gate must fail-open, got rejected reason=%q", reason)
	}
}

func TestBridgeGate_DisabledPassesEverything(t *testing.T) {
	g := NewContradictedBridgeGate(false, []GuidanceType{"constraint"}, []string{`^Bash error`})
	if reason, rejected := g.Reject(GuidanceLearning, "Bash error in command: foo"); rejected {
		t.Errorf("disabled gate must fail-open, got rejected reason=%q", reason)
	}
}

func TestBridgeGate_RejectsNonAllowedType(t *testing.T) {
	g := NewContradictedBridgeGate(true, []GuidanceType{GuidanceConstraint, GuidanceCorrection}, nil)
	// Pattern-type guidance (abstraction) → rejected by type filter.
	reason, rejected := g.Reject(GuidancePattern, "some content")
	if !rejected {
		t.Error("pattern-type guidance MUST be rejected — contradiction is meaningless for abstractions")
	}
	if reason != "type:pattern" {
		t.Errorf("reason must name the type, got %q", reason)
	}
}

func TestBridgeGate_AcceptsAllowedType(t *testing.T) {
	g := NewContradictedBridgeGate(true, []GuidanceType{GuidanceConstraint, GuidanceCorrection}, nil)
	if _, rejected := g.Reject(GuidanceConstraint, "NEVER commit to main"); rejected {
		t.Error("constraint-type guidance with clean content must pass")
	}
	if _, rejected := g.Reject(GuidanceCorrection, "Use CUIDv2 not UUID"); rejected {
		t.Error("correction-type guidance with clean content must pass")
	}
}

func TestBridgeGate_RejectsMatchingPattern(t *testing.T) {
	// Constraint-typed but the content is a Bash error → pattern filter fires.
	// This is the live class from the mdemg-dev queue.
	g := NewContradictedBridgeGate(true,
		[]GuidanceType{GuidanceConstraint, GuidanceCorrection},
		[]string{`^Bash error`, `^Build/test succeeded`})
	reason, rejected := g.Reject(GuidanceConstraint, "Bash error in command: grep -n 'foo'")
	if !rejected {
		t.Error("Bash-error content MUST be rejected regardless of guidance type")
	}
	if reason != "pattern:^Bash error" {
		t.Errorf("reason must name the matched pattern, got %q", reason)
	}
}

func TestBridgeGate_MultiplePatternsBothCatch(t *testing.T) {
	g := NewContradictedBridgeGate(true,
		nil, // type filter off — pattern filter only
		[]string{`^Bash error`, `^Build/test succeeded`})
	if _, rejected := g.Reject(GuidanceConstraint, "Build/test succeeded: all green"); !rejected {
		t.Error("first-registered matching pattern must fire (Build/test)")
	}
	if _, rejected := g.Reject(GuidanceConstraint, "Bash error in command: rm"); !rejected {
		t.Error("second-registered matching pattern must fire (Bash error)")
	}
}

func TestBridgeGate_InvalidRegexSkipsWithoutDisablingGate(t *testing.T) {
	// A single bad regex must NOT disable the whole gate; the valid regexes stay.
	g := NewContradictedBridgeGate(true,
		[]GuidanceType{GuidanceConstraint},
		[]string{`[invalid regex(`, `^Bash error`})
	// Valid pattern still fires.
	if _, rejected := g.Reject(GuidanceConstraint, "Bash error in command: foo"); !rejected {
		t.Error("valid regex must still catch even when a sibling regex is invalid")
	}
	// Non-matching content passes.
	if _, rejected := g.Reject(GuidanceConstraint, "always use CUIDv2"); rejected {
		t.Error("clean content must pass through the valid regex")
	}
}

func TestBridgeGate_TypeFilterOffOnlyPatternFilter(t *testing.T) {
	// AllowedTypes nil → only pattern filter runs; ANY type passes if content clean.
	g := NewContradictedBridgeGate(true, nil, []string{`^Bash error`})
	if _, rejected := g.Reject(GuidancePattern, "some abstract wisdom"); rejected {
		t.Error("with type filter off, non-matching pattern-type must pass")
	}
	if _, rejected := g.Reject(GuidancePattern, "Bash error: nope"); !rejected {
		t.Error("with type filter off, pattern filter must still fire")
	}
}

func TestBridgeGate_TypeFilterFiresBeforePatternFilter(t *testing.T) {
	// A pattern-type guidance with bash-error content should be caught by TYPE
	// (checked first) — not pattern — so operators can distinguish the two
	// rejection classes from the reason string.
	g := NewContradictedBridgeGate(true,
		[]GuidanceType{GuidanceConstraint},
		[]string{`^Bash error`})
	reason, rejected := g.Reject(GuidancePattern, "Bash error in command: foo")
	if !rejected {
		t.Fatal("must reject")
	}
	if reason != "type:pattern" {
		t.Errorf("type filter must fire first — expected 'type:pattern', got %q", reason)
	}
}

// Real content from the mdemg-dev queue that fired the HITL curation alert.
// This is a regression pin — if the gate stops catching these, the alert
// will re-fire on the same class of noise.
func TestBridgeGate_LiveMdemgDevNoiseClass(t *testing.T) {
	g := NewContradictedBridgeGate(true,
		[]GuidanceType{GuidanceConstraint, GuidanceCorrection},
		[]string{
			`^Bash error`,
			`(?i)^Phase\s+\d+:\s+.*\b(?:analysis|gap analysis|synthesis)\b`,
		})
	cases := []struct {
		name   string
		gtype  GuidanceType
		text   string
		reject bool
	}{
		{"bash-error-as-pattern-type", GuidancePattern, "Bash error in command: grep -n 'ubts'", true},           // rejected by TYPE (checked first)
		{"bash-error-as-constraint-type", GuidanceConstraint, "Bash error in command: sed -n '252,275p'", true}, // rejected by PATTERN
		{"phase-analysis-as-constraint", GuidanceConstraint, "Phase 92: Full system gap analysis for deployable package", true},
		{"learning-vision-narrative", GuidanceLearning, "Details MDEMG's vision as an emergent long-term memory system", true}, // TYPE
		{"real-constraint-passes", GuidanceConstraint, "CONSTRAINT: NEVER commit directly to main branch", false},
		{"real-correction-passes", GuidanceCorrection, "After merging a PR to main via --admin, if the dev branch has more work, rebase first", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, rejected := g.Reject(tc.gtype, tc.text)
			if rejected != tc.reject {
				t.Errorf("expected rejected=%v, got %v", tc.reject, rejected)
			}
		})
	}
}
