package retrieval

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestExpandSeedsByActivation_EmptySeeds(t *testing.T) {
	// Service constructed with nil driver — no code path should call it when
	// seeds are empty (early-return before any Neo4j hit).
	s := &Service{cfg: config.Config{}}
	got, err := s.ExpandSeedsByActivation(context.Background(), "space-x", nil, "any query")
	if err != nil {
		t.Fatalf("unexpected error on empty seeds: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map on empty seeds, got %v", got)
	}
}

func TestExpandSeedsByActivation_AllSeedsFilteredEmpty(t *testing.T) {
	// Seeds with empty NodeID must be filtered before any Neo4j touch —
	// otherwise the driver-nil access would panic. This test proves the
	// filter runs and the method safely early-returns.
	s := &Service{cfg: config.Config{}}
	seeds := []ActivationSeed{{NodeID: "", Score: 0.9}, {NodeID: "", Score: 0.5}}
	got, err := s.ExpandSeedsByActivation(context.Background(), "space-x", seeds, "any query")
	if err != nil {
		t.Fatalf("unexpected error on all-empty-NodeID seeds: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map when all seeds filter out, got %v", got)
	}
}

// Note: score-clamping into [0,1] is inline arithmetic before any driver
// touch — verifiable by code inspection. A test that reaches the clamp
// path requires a live Neo4j driver (fetchOutgoingEdges dispatches there);
// live Tier-3 exercises the end-to-end path.
