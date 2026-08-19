package jiminy

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/config"
)

// seedEffectivenessCache pre-populates the effectiveness cache for a space so
// activationEnrichLeverC's effectiveness path can be tested without a real
// PersistenceStore. Requires JiminySurfaceEffectivenessPriorWeight > 0 and
// non-nil persistence (the outer gate at effectivenessPriorRates) — set both
// on the Service before calling this helper.
func seedEffectivenessCache(s *Service, spaceID string, rates map[string]float64) {
	s.effPriorMu.Lock()
	defer s.effPriorMu.Unlock()
	if s.effPriorCache == nil {
		s.effPriorCache = make(map[string]effPriorCacheEntry)
	}
	s.effPriorCache[spaceID] = effPriorCacheEntry{rates: rates, fetchedAt: time.Now()}
}

// TestActivationEnrichLeverC_DisabledIsIdentity pins the default-off
// contract: with the flag off, the function returns its input unchanged
// (byte-identical, same ordering, same confidences). This regression-locks
// the ULTS-parity requirement — nothing about surfacing behavior changes
// unless the flag is flipped in .env.
func TestActivationEnrichLeverC_DisabledIsIdentity(t *testing.T) {
	s := &Service{
		cfg: config.Config{JiminyLeverCActivationEnabled: false},
		retriever: &mockRetriever{
			// If this activation map WERE consumed, it would reorder the
			// input — the disabled flag must prevent that.
			activation: map[string]float64{"n1": 0.0, "n2": 1.0},
		},
	}
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.9, Content: "first"},
		{SourceNodes: []string{"n2"}, Confidence: 0.1, Content: "second"},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if len(got) != 2 || got[0].SourceNodes[0] != "n1" || got[1].SourceNodes[0] != "n2" {
		t.Fatalf("disabled flag must be identity; got reordered: %+v", got)
	}
	if got[0].Confidence != 0.9 || got[1].Confidence != 0.1 {
		t.Fatalf("disabled flag must preserve confidences; got: %v %v", got[0].Confidence, got[1].Confidence)
	}
}

// TestActivationEnrichLeverC_NilRetriever pins fail-safe: nil retriever
// returns input unchanged, no panic.
func TestActivationEnrichLeverC_NilRetriever(t *testing.T) {
	s := &Service{
		cfg:       config.Config{JiminyLeverCActivationEnabled: true, JiminyLeverCActivationWeight: 0.3},
		retriever: nil,
	}
	input := []GuidanceItem{{SourceNodes: []string{"n1"}, Confidence: 0.7}}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if len(got) != 1 || got[0].Confidence != 0.7 {
		t.Fatalf("nil retriever must be identity; got: %+v", got)
	}
}

// TestActivationEnrichLeverC_EmptyActionables pins the empty-input contract.
func TestActivationEnrichLeverC_EmptyActionables(t *testing.T) {
	s := &Service{
		cfg:       config.Config{JiminyLeverCActivationEnabled: true, JiminyLeverCActivationWeight: 0.3},
		retriever: &mockRetriever{},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", nil, "q")
	if got != nil {
		t.Fatalf("empty input must return nil, got: %+v", got)
	}
}

// TestActivationEnrichLeverC_RerankReordersByBlend pins the core B1
// behavior: with activation flipping n2's Hebbian centrality high, n2
// leapfrogs n1 in the blended ordering even though n1's raw cosine is
// higher. Blend formula: (1-w)*cosine + w*activation, w=0.5.
//
//	n1: (1-0.5)*0.6 + 0.5*0.0 = 0.30
//	n2: (1-0.5)*0.5 + 0.5*1.0 = 0.75  ← wins
func TestActivationEnrichLeverC_RerankReordersByBlend(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled: true,
			JiminyLeverCActivationWeight:  0.5, // 50/50 blend for clean arithmetic
		},
		retriever: &mockRetriever{
			activation: map[string]float64{
				"n1": 0.0, // isolated
				"n2": 1.0, // fully activated
			},
		},
	}
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.6, Content: "high-cosine-low-activation"},
		{SourceNodes: []string{"n2"}, Confidence: 0.5, Content: "lower-cosine-high-activation"},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].SourceNodes[0] != "n2" {
		t.Fatalf("expected n2 to win the blend (0.75 > 0.30); got order: %s, %s", got[0].SourceNodes[0], got[1].SourceNodes[0])
	}
	if diff := got[0].Confidence - 0.75; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n2 blended confidence should be 0.75, got %f", got[0].Confidence)
	}
	if diff := got[1].Confidence - 0.30; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n1 blended confidence should be 0.30, got %f", got[1].Confidence)
	}
}

// TestActivationEnrichLeverC_ZeroWeightIsIdentity pins that w=0 short-circuits
// (defensive — someone might disable via weight instead of flag).
func TestActivationEnrichLeverC_ZeroWeightIsIdentity(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled: true,
			JiminyLeverCActivationWeight:  0.0, // disables via weight
		},
		retriever: &mockRetriever{
			activation: map[string]float64{"n1": 0.0, "n2": 1.0}, // would reorder
		},
	}
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.9},
		{SourceNodes: []string{"n2"}, Confidence: 0.1},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if got[0].SourceNodes[0] != "n1" || got[0].Confidence != 0.9 {
		t.Fatalf("zero weight must be identity; got: %+v", got)
	}
}

// TestActivationEnrichLeverC_EffectivenessOnly_ReordersByRate pins B2's
// effectiveness-only mode: activation flag OFF, we=0.5, seeded rate map that
// should flip ordering (n2 has higher effectiveness rate than n1 despite lower
// cosine). Blend: (1-0-0.5)*cosine + 0*act + 0.5*effRate
//
//	n1: 0.5*0.9 + 0.5*0.10 = 0.50
//	n2: 0.5*0.5 + 0.5*0.90 = 0.70  ← wins
func TestActivationEnrichLeverC_EffectivenessOnly_ReordersByRate(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			// Activation off
			JiminyLeverCActivationEnabled:         false,
			JiminyLeverCActivationWeight:          0,
			JiminyLeverCEffectivenessWeight:       0.5,
			JiminySurfaceEffectivenessPriorWeight: 0.3, // gate: > 0 for cache-fetch
		},
		persistence: &PersistenceStore{}, // gate: non-nil (never called via cache-hit)
	}
	seedEffectivenessCache(s, "space-x", map[string]float64{
		"n1": 0.10, // low effectiveness
		"n2": 0.90, // high effectiveness
	})
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.9, Content: "high-cosine-low-effRate"},
		{SourceNodes: []string{"n2"}, Confidence: 0.5, Content: "lower-cosine-high-effRate"},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].SourceNodes[0] != "n2" {
		t.Fatalf("expected n2 to win effectiveness blend (0.70 > 0.50); got order: %s, %s", got[0].SourceNodes[0], got[1].SourceNodes[0])
	}
	if diff := got[0].Confidence - 0.70; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n2 blended confidence should be 0.70, got %f", got[0].Confidence)
	}
	if diff := got[1].Confidence - 0.50; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n1 blended confidence should be 0.50, got %f", got[1].Confidence)
	}
}

// TestActivationEnrichLeverC_ThreeWayBlend pins the full 3-way blend:
// activation + effectiveness + cosine all contributing. wa=0.3, we=0.3 →
// cosine coefficient 0.4. Two items:
//
//	n1: cos=0.8, act=0.2, eff=0.1 → 0.4*0.8 + 0.3*0.2 + 0.3*0.1 = 0.41
//	n2: cos=0.5, act=0.9, eff=0.8 → 0.4*0.5 + 0.3*0.9 + 0.3*0.8 = 0.71  ← wins
func TestActivationEnrichLeverC_ThreeWayBlend(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled:         true,
			JiminyLeverCActivationWeight:          0.3,
			JiminyLeverCEffectivenessWeight:       0.3,
			JiminySurfaceEffectivenessPriorWeight: 0.3,
		},
		persistence: &PersistenceStore{},
		retriever: &mockRetriever{
			activation: map[string]float64{"n1": 0.2, "n2": 0.9},
		},
	}
	seedEffectivenessCache(s, "space-x", map[string]float64{"n1": 0.1, "n2": 0.8})
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.8},
		{SourceNodes: []string{"n2"}, Confidence: 0.5},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if got[0].SourceNodes[0] != "n2" {
		t.Fatalf("expected n2 to win 3-way blend (0.71 > 0.41); got order: %s, %s", got[0].SourceNodes[0], got[1].SourceNodes[0])
	}
	if diff := got[0].Confidence - 0.71; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n2 3-way blended confidence should be 0.71, got %f", got[0].Confidence)
	}
	if diff := got[1].Confidence - 0.41; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n1 3-way blended confidence should be 0.41, got %f", got[1].Confidence)
	}
}

// TestActivationEnrichLeverC_WeightClamping pins the wa+we<=1 clamp:
// operator specifies wa=0.6, we=0.6 → we clamped to 0.4 → sum=1.0 → cosine
// coefficient 0.0. Ordering follows pure activation+effectiveness.
//
//	n1: 0*0.5 + 0.6*0.1 + 0.4*0.9 = 0.42
//	n2: 0*0.9 + 0.6*0.9 + 0.4*0.1 = 0.58 (higher activation + clamped-effRate wins)
func TestActivationEnrichLeverC_WeightClamping(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled:         true,
			JiminyLeverCActivationWeight:          0.6,
			JiminyLeverCEffectivenessWeight:       0.6, // clamped to 0.4
			JiminySurfaceEffectivenessPriorWeight: 0.3,
		},
		persistence: &PersistenceStore{},
		retriever: &mockRetriever{
			activation: map[string]float64{"n1": 0.1, "n2": 0.9},
		},
	}
	seedEffectivenessCache(s, "space-x", map[string]float64{"n1": 0.9, "n2": 0.1})
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.5},
		{SourceNodes: []string{"n2"}, Confidence: 0.9},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if got[0].SourceNodes[0] != "n2" {
		t.Fatalf("expected n2 (higher activation) to win clamped blend; got order: %s, %s", got[0].SourceNodes[0], got[1].SourceNodes[0])
	}
	if diff := got[0].Confidence - 0.58; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n2 clamped-blend confidence should be 0.58, got %f", got[0].Confidence)
	}
	if diff := got[1].Confidence - 0.42; diff < -0.001 || diff > 0.001 {
		t.Fatalf("n1 clamped-blend confidence should be 0.42, got %f", got[1].Confidence)
	}
}

// TestActivationEnrichLeverC_EffectivenessNilPersistenceIsSafe pins that
// we>0 with nil persistence returns input unchanged (B1 contract preserved:
// no signal available → identity).
func TestActivationEnrichLeverC_EffectivenessNilPersistenceIsSafe(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled:         false,
			JiminyLeverCEffectivenessWeight:       0.5,
			JiminySurfaceEffectivenessPriorWeight: 0.3,
		},
		persistence: nil, // nil persistence → effectivenessPriorRates returns nil
	}
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.9},
		{SourceNodes: []string{"n2"}, Confidence: 0.1},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if got[0].SourceNodes[0] != "n1" || got[0].Confidence != 0.9 {
		t.Fatalf("nil persistence with we>0 must be identity; got: %+v", got)
	}
}

// TestActivationEnrichLeverC_ActivationErrorFailsOpen pins that a retriever
// error returns the input unchanged (Confidence + ordering preserved).
func TestActivationEnrichLeverC_ActivationErrorFailsOpen(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			JiminyLeverCActivationEnabled: true,
			JiminyLeverCActivationWeight:  0.5,
		},
		retriever: &mockRetriever{
			activationErr: context.DeadlineExceeded, // simulate any error
		},
	}
	input := []GuidanceItem{
		{SourceNodes: []string{"n1"}, Confidence: 0.9},
		{SourceNodes: []string{"n2"}, Confidence: 0.1},
	}
	got := s.activationEnrichLeverC(context.Background(), "space-x", input, "q")
	if got[0].SourceNodes[0] != "n1" || got[0].Confidence != 0.9 {
		t.Fatalf("activation error must fail open (preserve input); got: %+v", got)
	}
}
