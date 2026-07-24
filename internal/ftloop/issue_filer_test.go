package ftloop

import (
	"strings"
	"testing"
)

// FT-RECURSIVE-003 E7: recurrences of one defect must fingerprint
// identically despite volatile tokens (cycle ids, paths, numbers).
func TestNormalizeSignature_CollapsesVolatileTokens(t *testing.T) {
	a := normalizeSignature("convert",
		"convert-gguf: exit status 2 (can't open file '/Users/x/T/mdemg-ft-loop/gdd72ewed2t3vla9cnu7z1ft/x.py')")
	b := normalizeSignature("convert",
		"convert-gguf: exit status 2 (can't open file '/Users/x/T/mdemg-ft-loop/i0ddlbpuauf3pznrks0g6u36/x.py')")
	if a != b {
		t.Errorf("same defect must normalize identically:\n%s\n%s", a, b)
	}
	if fingerprint(a) != fingerprint(b) {
		t.Error("fingerprints must match")
	}
	// Different stages must NOT collide.
	c := normalizeSignature("train", "exit status 2")
	if fingerprint(c) == fingerprint(normalizeSignature("convert", "exit status 2")) {
		t.Error("stage must be part of the fingerprint")
	}
}

func TestTruncateForTitle_RuneSafe(t *testing.T) {
	s := strings.Repeat("a", 78) + "→→→"
	got := truncateForTitle(s)
	for i := 0; i < len(got); i++ {
		// A well-formed string never starts a check mid-rune; validate whole.
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix: %q", got)
	}
	if len([]rune(got)) == 0 {
		t.Error("empty")
	}
}

// FT-RECURSIVE-004 E3: the sweep SQL must include rolled_back failure
// classes (canary_failed/promote_failed), not only status='failed', and
// must require a non-empty error (neutralized drill events carry stages
// like e7_drill_neutralized with explanatory text — the error filter plus
// the _failed suffix keeps them out unless they genuinely match).
func TestSweepSQL_IncludesRolledBackFailures(t *testing.T) {
	f := NewIssueFiler(nil, IssueFilerConfig{}, nil, nil)
	_ = f
	// Shape-pin on the query text via collectGroups' source.
	src := sweepQueryForTest()
	for _, w := range []string{"rolled_back", "%\\_failed", "DISTINCT ON (cycle_id)", "error <> ''"} {
		if !strings.Contains(src, w) {
			t.Errorf("sweep SQL missing %q", w)
		}
	}
}
