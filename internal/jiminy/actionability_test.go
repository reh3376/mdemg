package jiminy

import (
	"strings"
	"testing"

	"mdemg/internal/config"
)

func TestIsActionableType(t *testing.T) {
	for _, ty := range []GuidanceType{GuidanceConstraint, GuidanceCorrection} {
		if !isActionableType(ty) {
			t.Errorf("%q should be actionable", ty)
		}
	}
	for _, ty := range []GuidanceType{GuidancePattern, GuidanceLearning, GuidanceConcept, GuidanceSuggestion} {
		if isActionableType(ty) {
			t.Errorf("%q should be abstraction (not actionable)", ty)
		}
	}
}

func items(types ...GuidanceType) []GuidanceItem {
	out := make([]GuidanceItem, len(types))
	for i, ty := range types {
		out[i] = GuidanceItem{Type: ty, Content: string(ty), Confidence: 0.9}
	}
	return out
}

func countActionable(its []GuidanceItem) int {
	n := 0
	for _, it := range its {
		if isActionableType(it.Type) {
			n++
		}
	}
	return n
}

func TestApplyActionableComposition_DefaultPreserving(t *testing.T) {
	// Defaults: weight 1, quota 0, fraction 0, cap 1.0 → exactly a plain truncate.
	s := &Service{cfg: config.Config{
		JiminySurfaceActionableWeight:       1.0,
		JiminySurfaceMaxAbstractionFraction: 1.0,
	}}
	in := items(GuidancePattern, GuidanceLearning, GuidanceConstraint, GuidanceConcept, GuidanceCorrection)
	got := s.applyActionableComposition(in, 3)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	for i := 0; i < 3; i++ {
		if got[i].Type != in[i].Type {
			t.Errorf("default-preserving: pos %d = %q, want %q (must be a plain truncate)", i, got[i].Type, in[i].Type)
		}
	}
}

func TestApplyActionableComposition_Quota(t *testing.T) {
	// Quota 2: even though abstractions sort first, 2 actionable must survive
	// the truncation to maxItems=3.
	s := &Service{cfg: config.Config{
		JiminySurfaceActionableWeight:       1.0,
		JiminySurfaceMaxAbstractionFraction: 1.0,
		JiminySurfaceMinActionable:          2,
	}}
	// 4 abstractions first, then 2 actionables (sorted order).
	in := items(GuidancePattern, GuidanceLearning, GuidanceConcept, GuidancePattern, GuidanceConstraint, GuidanceCorrection)
	got := s.applyActionableComposition(in, 3)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if a := countActionable(got); a < 2 {
		t.Errorf("quota=2 must reserve 2 actionable slots, got %d in %v", a, typesOf(got))
	}
}

func TestApplyActionableComposition_AbstractionCap(t *testing.T) {
	// Cap 0.5, maxItems=4 → at most 2 abstraction items; abstraction-tail dropped,
	// actionable never dropped.
	s := &Service{cfg: config.Config{
		JiminySurfaceActionableWeight:       1.0,
		JiminySurfaceMaxAbstractionFraction: 0.5,
	}}
	in := items(GuidancePattern, GuidanceLearning, GuidanceConcept, GuidancePattern, GuidanceConstraint, GuidanceCorrection)
	got := s.applyActionableComposition(in, 4)
	abs := len(got) - countActionable(got)
	if abs > 2 {
		t.Errorf("cap 0.5 of 4 → ≤2 abstraction, got %d (%v)", abs, typesOf(got))
	}
	// Both actionable items must be present (never dropped).
	if countActionable(got) != 2 {
		t.Errorf("actionable items must survive the cap, got %d", countActionable(got))
	}
}

func typesOf(its []GuidanceItem) []GuidanceType {
	out := make([]GuidanceType, len(its))
	for i, it := range its {
		out[i] = it.Type
	}
	return out
}

// --- Lever B: directive-mode synthesis ---

func TestBoundDirectivePrompt(t *testing.T) {
	long := strings.Repeat("x", 100)
	// 0/negative budget = no bound.
	if got := boundDirectivePrompt(long, 0); got != long {
		t.Errorf("budget 0 must not bound; len=%d", len(got))
	}
	if got := boundDirectivePrompt(long, -5); got != long {
		t.Errorf("negative budget must not bound; len=%d", len(got))
	}
	// budget 10 tokens → 40 chars max; 100-char prompt is trimmed + marked.
	got := boundDirectivePrompt(long, 10)
	if !strings.HasPrefix(got, strings.Repeat("x", 40)) {
		t.Errorf("bounded prompt must keep the first 40 chars")
	}
	if !strings.HasSuffix(got, "[bounded]") {
		t.Errorf("bounded prompt must carry the [bounded] marker, got %q", got)
	}
	// Under budget = untouched.
	short := strings.Repeat("y", 20)
	if got := boundDirectivePrompt(short, 10); got != short {
		t.Errorf("under-budget prompt must be untouched, got %q", got)
	}
}

func TestDirectiveSynthesisInstruction_Imperative(t *testing.T) {
	// The directive block must actually instruct imperative rendering of
	// abstractions — a sanity guard so a future edit can't silently neuter it.
	if !strings.Contains(directiveSynthesisInstruction, "DIRECTIVE MODE") {
		t.Error("directive instruction must announce DIRECTIVE MODE")
	}
	for _, kw := range []string{"PATTERNS", "imperative", "(Node:"} {
		if !strings.Contains(directiveSynthesisInstruction, kw) {
			t.Errorf("directive instruction must reference %q (imperative-abstraction + citation contract)", kw)
		}
	}
}

func TestSynthesize_DirectiveMode_OffIsDefault(t *testing.T) {
	// Default-off: a zero-value SynthesisConfig leaves DirectiveMode false, so
	// the system prompt is the unmodified guidanceSystemPrompt.
	var cfg SynthesisConfig
	if cfg.DirectiveMode {
		t.Error("DirectiveMode must default to false (zero value)")
	}
}

// --- Lever C: constraint-inclusion (guards; DB path live-tested in Epic 5d) ---

func TestFetchActionableCandidates_Guards(t *testing.T) {
	s := &Service{cfg: config.Config{}} // nil driver
	if got := s.fetchActionableCandidates(nil, "sp", []float32{0.1, 0.2}, 5, 0.3); got != nil {
		t.Errorf("nil driver must return nil, got %v", got)
	}
	// topK<=0 → nil even with a (would-be) driver
	if got := s.fetchActionableCandidates(nil, "sp", []float32{0.1}, 0, 0.3); got != nil {
		t.Errorf("topK<=0 must return nil")
	}
	// empty embedding → nil
	if got := s.fetchActionableCandidates(nil, "sp", nil, 5, 0.3); got != nil {
		t.Errorf("empty embedding must return nil")
	}
}
