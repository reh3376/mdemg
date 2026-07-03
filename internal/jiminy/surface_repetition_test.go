package jiminy

import (
	"context"
	"sort"
	"testing"
	"time"

	"mdemg/internal/config"
)

// JIMINY-CORPUS-001 Epic 3 — repetition control for guidance surfacing.
// Lever A: per-session surfacing cooldown. Lever B: effectiveness prior.

// --- Lever A: SurfaceCooldownTracker unit tests ---

func TestSurfaceCooldownTracker_IgnoredIncrementsAndFollowedResets(t *testing.T) {
	ct := NewSurfaceCooldownTracker(10)

	for i := 1; i <= 3; i++ {
		ct.RecordOutcome("s1", "n1", OutcomeIgnored)
		if count, _ := ct.State("s1", "n1"); count != i {
			t.Fatalf("after %d ignores count = %d, want %d", i, count, i)
		}
	}

	// A followed outcome releases the counter entirely.
	ct.RecordOutcome("s1", "n1", OutcomeFollowed)
	if count, _ := ct.State("s1", "n1"); count != 0 {
		t.Errorf("followed must reset counter, got %d", count)
	}

	// Partial compliance releases too.
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)
	ct.RecordOutcome("s1", "n1", OutcomePartialCompliance)
	if count, _ := ct.State("s1", "n1"); count != 0 {
		t.Errorf("partial_compliance must reset counter, got %d", count)
	}
}

func TestSurfaceCooldownTracker_ContradictedAndNotApplicableAreNoOps(t *testing.T) {
	ct := NewSurfaceCooldownTracker(10)
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)

	// Contradicted = actively violated = highly relevant → must NOT count
	// toward cooldown (escalation owns that path) and must NOT release.
	ct.RecordOutcome("s1", "n1", OutcomeContradicted)
	if count, _ := ct.State("s1", "n1"); count != 2 {
		t.Errorf("contradicted must be a no-op, got count %d, want 2", count)
	}
	ct.RecordOutcome("s1", "n1", OutcomeNotApplicable)
	if count, _ := ct.State("s1", "n1"); count != 2 {
		t.Errorf("not_applicable must be a no-op, got count %d, want 2", count)
	}
}

func TestSurfaceCooldownTracker_PerSessionIsolation(t *testing.T) {
	ct := NewSurfaceCooldownTracker(10)
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)

	if count, _ := ct.State("s2", "n1"); count != 0 {
		t.Errorf("cooldown must be per-session: session s2 count = %d, want 0", count)
	}
}

func TestSurfaceCooldownTracker_CapacityBound(t *testing.T) {
	ct := NewSurfaceCooldownTracker(3)
	ct.RecordOutcome("s1", "n1", OutcomeIgnored)
	ct.RecordOutcome("s1", "n2", OutcomeIgnored)
	ct.RecordOutcome("s1", "n3", OutcomeIgnored)
	ct.RecordOutcome("s1", "n4", OutcomeIgnored) // evicts least-recently-updated (n1)

	if got := ct.Size(); got != 3 {
		t.Fatalf("size = %d, want capacity bound 3", got)
	}
	if count, _ := ct.State("s1", "n1"); count != 0 {
		t.Errorf("LRU-evicted entry must restart at 0 (fail-open), got %d", count)
	}
	if count, _ := ct.State("s1", "n4"); count != 1 {
		t.Errorf("newest entry count = %d, want 1", count)
	}
}

func TestSurfaceCooldownTracker_NilAndEmptyGuards(t *testing.T) {
	var ct *SurfaceCooldownTracker
	ct.RecordOutcome("s1", "n1", OutcomeIgnored) // must not panic
	if count, _ := ct.State("s1", "n1"); count != 0 {
		t.Errorf("nil tracker State = %d, want 0", count)
	}
	if ct.Size() != 0 {
		t.Errorf("nil tracker Size != 0")
	}

	real := NewSurfaceCooldownTracker(10)
	real.RecordOutcome("", "n1", OutcomeIgnored)
	real.RecordOutcome("s1", "", OutcomeIgnored)
	if real.Size() != 0 {
		t.Errorf("empty session/node ids must not be tracked, size = %d", real.Size())
	}
}

// --- Lever A: applySurfaceCooldown ---

func cooldownService(threshold int, extra func(*config.Config)) *Service {
	cfg := config.Config{
		JiminySurfaceActionableWeight:       1.0,
		JiminySurfaceMaxAbstractionFraction: 1.0,
		JiminySurfaceCooldownIgnoredCount:   threshold,
		JiminySurfaceCooldownCapacity:       100,
	}
	if extra != nil {
		extra(&cfg)
	}
	return NewService(cfg, nil, nil, nil)
}

func itemWithNode(ty GuidanceType, nodeID string) GuidanceItem {
	return GuidanceItem{Type: ty, Content: string(ty) + ":" + nodeID, Confidence: 0.9, Priority: "medium", SourceNodes: []string{nodeID}}
}

func nodeIDsOf(its []GuidanceItem) []string {
	out := make([]string, 0, len(its))
	for _, it := range its {
		if len(it.SourceNodes) > 0 {
			out = append(out, it.SourceNodes[0])
		}
	}
	return out
}

func TestApplySurfaceCooldown_SuppressedOnFourthAfterThreeIgnores(t *testing.T) {
	s := cooldownService(3, nil)
	in := []GuidanceItem{itemWithNode(GuidanceConstraint, "hot"), itemWithNode(GuidancePattern, "fresh")}

	// Two ignores: below threshold → still surfaced.
	s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	if got := s.applySurfaceCooldown("s1", in, 10); len(got) != 2 {
		t.Fatalf("below threshold must not suppress, got %v", nodeIDsOf(got))
	}

	// Third consecutive ignore reaches the threshold → suppressed on the next surface.
	s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	got := s.applySurfaceCooldown("s1", in, 10)
	if len(got) != 1 || got[0].SourceNodes[0] != "fresh" {
		t.Fatalf("node ignored 3x must be suppressed on the 4th surface, got %v", nodeIDsOf(got))
	}
}

func TestApplySurfaceCooldown_FollowedReleasesSuppression(t *testing.T) {
	s := cooldownService(3, nil)
	in := []GuidanceItem{itemWithNode(GuidanceConstraint, "hot"), itemWithNode(GuidancePattern, "fresh")}

	for i := 0; i < 3; i++ {
		s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	}
	if got := s.applySurfaceCooldown("s1", in, 10); len(got) != 1 {
		t.Fatalf("precondition: hot must be suppressed, got %v", nodeIDsOf(got))
	}

	// A followed outcome resets the counter → surfaces again.
	s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeFollowed)
	if got := s.applySurfaceCooldown("s1", in, 10); len(got) != 2 {
		t.Errorf("followed must release the cooldown, got %v", nodeIDsOf(got))
	}
}

func TestApplySurfaceCooldown_PerSession(t *testing.T) {
	s := cooldownService(3, nil)
	in := []GuidanceItem{itemWithNode(GuidanceConstraint, "hot"), itemWithNode(GuidancePattern, "fresh")}

	for i := 0; i < 3; i++ {
		s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	}
	// Another session is unaffected.
	if got := s.applySurfaceCooldown("s2", in, 10); len(got) != 2 {
		t.Errorf("cooldown must be per-session: s2 got %v", nodeIDsOf(got))
	}
	// The cooled session suppresses the hot node.
	got := s.applySurfaceCooldown("s1", in, 10)
	if len(got) != 1 || got[0].SourceNodes[0] != "fresh" {
		t.Errorf("s1 must suppress hot, got %v", nodeIDsOf(got))
	}
}

func TestApplySurfaceCooldown_AllCooledFallbackLeastRecentlyIgnored(t *testing.T) {
	// Never suppress ALL guidance: when everything is cooled, surface the
	// least-recently-ignored items instead (oldest lastIgnoredAt first).
	s := cooldownService(3, nil)
	in := []GuidanceItem{itemWithNode(GuidanceConstraint, "older"), itemWithNode(GuidanceConstraint, "newer")}

	for i := 0; i < 3; i++ {
		s.surfaceCooldown.RecordOutcome("s1", "older", OutcomeIgnored)
	}
	time.Sleep(2 * time.Millisecond) // ensure distinct lastIgnoredAt ordering
	for i := 0; i < 3; i++ {
		s.surfaceCooldown.RecordOutcome("s1", "newer", OutcomeIgnored)
	}

	got := s.applySurfaceCooldown("s1", in, 10)
	if len(got) != 2 {
		t.Fatalf("all-cooled fallback must surface everything, got %v", nodeIDsOf(got))
	}
	if got[0].SourceNodes[0] != "older" {
		t.Errorf("all-cooled fallback must order least-recently-ignored first, got %v", nodeIDsOf(got))
	}
}

func TestApplySurfaceCooldown_QuotaStillMetWhenActionablesCooled(t *testing.T) {
	// Min-actionable quota 2; both actionable nodes are cooled but abstractions
	// are not. The cooled actionables must be released (de-prioritised, appended)
	// so the downstream composition quota stays satisfiable end-to-end.
	s := cooldownService(3, func(c *config.Config) {
		c.JiminySurfaceMinActionable = 2
	})
	in := []GuidanceItem{
		itemWithNode(GuidanceConstraint, "act1"),
		itemWithNode(GuidanceCorrection, "act2"),
		itemWithNode(GuidancePattern, "abs1"),
		itemWithNode(GuidancePattern, "abs2"),
		itemWithNode(GuidancePattern, "abs3"),
	}
	for _, n := range []string{"act1", "act2"} {
		for i := 0; i < 3; i++ {
			s.surfaceCooldown.RecordOutcome("s1", n, OutcomeIgnored)
		}
	}

	maxItems := 3
	afterCooldown := s.applySurfaceCooldown("s1", in, maxItems)
	if a := countActionable(afterCooldown); a < 2 {
		t.Fatalf("cooldown must release cooled actionables to keep quota satisfiable, got %d actionable in %v", a, nodeIDsOf(afterCooldown))
	}
	// Released actionables come back de-prioritised (appended after active items).
	if isActionableType(afterCooldown[0].Type) {
		t.Errorf("released cooled actionables must be de-prioritised, got %v", nodeIDsOf(afterCooldown))
	}

	// End-to-end through the composition: the quota is actually met.
	final := s.applyActionableComposition(afterCooldown, maxItems)
	if a := countActionable(final); a < 2 {
		t.Errorf("min-actionable quota must still be met when all actionables are cooled, got %d in %v", a, nodeIDsOf(final))
	}
}

func TestApplySurfaceCooldown_DisabledOrNoSessionIsNoOp(t *testing.T) {
	in := []GuidanceItem{itemWithNode(GuidanceConstraint, "hot")}

	// threshold 0 = disabled → tracker not even constructed, list untouched.
	s0 := cooldownService(0, nil)
	if s0.surfaceCooldown != nil {
		t.Errorf("threshold 0 must not construct a cooldown tracker")
	}
	if got := s0.applySurfaceCooldown("s1", in, 10); len(got) != 1 {
		t.Errorf("disabled cooldown must be a no-op")
	}

	// Empty session id → no-op (nothing to key on).
	s := cooldownService(3, nil)
	for i := 0; i < 3; i++ {
		s.surfaceCooldown.RecordOutcome("s1", "hot", OutcomeIgnored)
	}
	if got := s.applySurfaceCooldown("", in, 10); len(got) != 1 {
		t.Errorf("empty session must be a no-op")
	}

	// Zero-value Service (nil tracker, zero config) → no-op, no panic.
	zv := &Service{cfg: config.Config{}}
	if got := zv.applySurfaceCooldown("s1", in, 10); len(got) != 1 {
		t.Errorf("zero-value service must be a no-op")
	}
}

// --- Lever A: feedback-path integration (Tier 2, in-process) ---

func TestRecordOutcome_FeedsSurfaceCooldown(t *testing.T) {
	s := cooldownService(3, nil)
	item := itemWithNode(GuidanceConstraint, "hot")

	for i := 1; i <= 3; i++ {
		gid := "g-cooldown-" + string(rune('0'+i))
		s.tracker.Track(gid, []GuidanceItem{item})
		if _, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
			GuidanceID:    gid,
			SessionID:     "s1",
			SpaceID:       "sp",
			ActionSummary: "did something unrelated",
			Outcome:       OutcomeIgnored, // explicit → bypasses classifier
		}); err != nil {
			t.Fatalf("RecordOutcome: %v", err)
		}
	}
	if count, _ := s.surfaceCooldown.State("s1", "hot"); count != 3 {
		t.Fatalf("feedback path must feed the cooldown tracker, count = %d, want 3", count)
	}

	// And a followed outcome through the same path releases it.
	s.tracker.Track("g-release", []GuidanceItem{item})
	if _, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    "g-release",
		SessionID:     "s1",
		SpaceID:       "sp",
		ActionSummary: "complied",
		Outcome:       OutcomeFollowed,
	}); err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	if count, _ := s.surfaceCooldown.State("s1", "hot"); count != 0 {
		t.Errorf("followed via feedback path must release, count = %d", count)
	}
}

// --- Lever B: effectiveness prior ---

func effPriorService(weight float64) *Service {
	return NewService(config.Config{
		JiminySurfaceActionableWeight:             1.0,
		JiminySurfaceMaxAbstractionFraction:       1.0,
		JiminySurfaceEffectivenessPriorWeight:     weight,
		JiminySurfaceEffectivenessPriorTTLSec:     300,
		JiminySurfaceEffectivenessPriorMinSamples: 5,
	}, nil, nil, nil)
}

func TestEffectivenessPriorMultiplier_Formula(t *testing.T) {
	s := effPriorService(0.3)
	rates := map[string]float64{"good": 0.9, "bad": 0.1}

	// (1-w) + w·rate
	if got, want := s.effectivenessPriorMultiplier(rates, itemWithNode(GuidanceConstraint, "good")), 0.7+0.3*0.9; !closeTo(got, want) {
		t.Errorf("good multiplier = %v, want %v", got, want)
	}
	if got, want := s.effectivenessPriorMultiplier(rates, itemWithNode(GuidanceConstraint, "bad")), 0.7+0.3*0.1; !closeTo(got, want) {
		t.Errorf("bad multiplier = %v, want %v", got, want)
	}
	// No data → neutral (never penalised).
	if got := s.effectivenessPriorMultiplier(rates, itemWithNode(GuidanceConstraint, "unknown")); got != 1.0 {
		t.Errorf("no-data node must be neutral, got %v", got)
	}
	// Soft floor: even rate 0 keeps (1-w) of the key — never a hard drop.
	rates["zero"] = 0.0
	if got := s.effectivenessPriorMultiplier(rates, itemWithNode(GuidanceConstraint, "zero")); !closeTo(got, 0.7) {
		t.Errorf("rate 0 must floor at 1-w = 0.7, got %v", got)
	}
	// Multi-source: the worst-performing cited node drives the down-weight.
	multi := GuidanceItem{Type: GuidanceConstraint, SourceNodes: []string{"good", "bad"}}
	if got, want := s.effectivenessPriorMultiplier(rates, multi), 0.7+0.3*0.1; !closeTo(got, want) {
		t.Errorf("multi-source must use the minimum rate, got %v want %v", got, want)
	}
}

func closeTo(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

func TestSortGuidanceItems_EffectivenessPriorReRanks(t *testing.T) {
	s := effPriorService(0.5)
	rates := map[string]float64{"effective": 0.9, "ignored": 0.05}

	items := []GuidanceItem{
		{Type: GuidanceConstraint, Priority: "medium", Confidence: 0.80, SourceNodes: []string{"ignored"}},
		{Type: GuidanceConstraint, Priority: "medium", Confidence: 0.75, SourceNodes: []string{"effective"}},
		{Type: GuidanceConstraint, Priority: "high", Confidence: 0.10, SourceNodes: []string{"ignored"}},
	}
	s.sortGuidanceItems(items, rates)

	if items[0].Priority != "high" {
		t.Fatalf("priority must still dominate the prior, got %q first", items[0].Priority)
	}
	// ignored: 0.80·(0.5+0.5·0.05)=0.42 < effective: 0.75·(0.5+0.5·0.9)=0.7125
	if items[1].SourceNodes[0] != "effective" {
		t.Errorf("higher-effectiveness node must out-rank the chronically-ignored one, got %v", nodeIDsOf(items))
	}
}

func TestSortGuidanceItems_WeightZeroIsByteIdentical(t *testing.T) {
	// Weight 0 must produce EXACTLY the pre-Epic-3 ordering — mirror the
	// JIMINY-ACTIONABILITY-001 zero-config no-op discipline.
	s := effPriorService(0)
	rates := map[string]float64{"a": 0.0, "b": 1.0} // present but must be ignored at w=0

	mk := func() []GuidanceItem {
		return []GuidanceItem{
			{Type: GuidancePattern, Priority: "medium", Confidence: 0.80, Content: "p1", SourceNodes: []string{"a"}},
			{Type: GuidanceConstraint, Priority: "high", Confidence: 0.60, Content: "c1", SourceNodes: []string{"b"}},
			{Type: GuidanceLearning, Priority: "medium", Confidence: 0.70, Content: "l1", SourceNodes: []string{"b"}},
			{Type: GuidanceCorrection, Priority: "low", Confidence: 0.90, Content: "x1", SourceNodes: []string{"a"}},
		}
	}

	got := mk()
	s.sortGuidanceItems(got, rates)

	// Baseline: the pre-Epic-3 comparator (no prior term).
	want := mk()
	sort.Slice(want, func(i, j int) bool {
		pi, pj := priorityRank(want[i].Priority), priorityRank(want[j].Priority)
		if pi != pj {
			return pi < pj
		}
		ki := s.guidanceSortKey(want[i]) * s.guidanceTypeWeight(want[i].Type)
		kj := s.guidanceSortKey(want[j]) * s.guidanceTypeWeight(want[j].Type)
		return ki > kj
	})

	for i := range want {
		if got[i].Content != want[i].Content {
			t.Fatalf("weight 0 must be byte-identical to the no-prior sort: pos %d = %q, want %q", i, got[i].Content, want[i].Content)
		}
	}
}

func TestEffectivenessPriorRates_DisabledPaths(t *testing.T) {
	// Weight 0 → nil without touching persistence.
	if got := effPriorService(0).effectivenessPriorRates(context.Background(), "sp"); got != nil {
		t.Errorf("weight 0 must return nil rates")
	}
	// Weight > 0 but persistence nil (no driver) → nil, never an error.
	if got := effPriorService(0.3).effectivenessPriorRates(context.Background(), "sp"); got != nil {
		t.Errorf("nil persistence must return nil rates")
	}
	// Empty space → nil.
	if got := effPriorService(0.3).effectivenessPriorRates(context.Background(), ""); got != nil {
		t.Errorf("empty space must return nil rates")
	}
	// Zero-value Service → no panic, nil.
	zv := &Service{cfg: config.Config{JiminySurfaceEffectivenessPriorWeight: 0.3}}
	if got := zv.effectivenessPriorRates(context.Background(), "sp"); got != nil {
		t.Errorf("zero-value service must return nil rates")
	}
}
