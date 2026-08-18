package jiminy

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

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
