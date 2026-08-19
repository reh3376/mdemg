package jiminy

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

// TestFetchConceptCandidates_DriverNilIsSafe pins the fail-open contract:
// nil driver → nil (no panic).
func TestFetchConceptCandidates_DriverNilIsSafe(t *testing.T) {
	s := &Service{cfg: config.Config{}, driver: nil}
	got := s.fetchConceptCandidates(context.Background(), "space-x", []float32{0.1, 0.2}, 3, 0.5, 2)
	if got != nil {
		t.Fatalf("expected nil on nil driver, got %+v", got)
	}
}

// TestFetchConceptCandidates_EmptyEmbeddingIsSafe pins the fail-open contract:
// nil or empty embedding → nil (never reach Neo4j).
func TestFetchConceptCandidates_EmptyEmbeddingIsSafe(t *testing.T) {
	// Driver nil is safe because the length check on embedding comes first.
	s := &Service{cfg: config.Config{}, driver: nil}
	if got := s.fetchConceptCandidates(context.Background(), "space-x", nil, 3, 0.5, 2); got != nil {
		t.Fatalf("expected nil on nil embedding, got %+v", got)
	}
	if got := s.fetchConceptCandidates(context.Background(), "space-x", []float32{}, 3, 0.5, 2); got != nil {
		t.Fatalf("expected nil on empty embedding, got %+v", got)
	}
}

// TestFetchConceptCandidates_TopKZeroIsSafe pins the topK ≤ 0 short-circuit.
func TestFetchConceptCandidates_TopKZeroIsSafe(t *testing.T) {
	s := &Service{cfg: config.Config{}, driver: nil}
	emb := []float32{0.1, 0.2}
	if got := s.fetchConceptCandidates(context.Background(), "space-x", emb, 0, 0.5, 2); got != nil {
		t.Fatalf("expected nil on topK=0, got %+v", got)
	}
	if got := s.fetchConceptCandidates(context.Background(), "space-x", emb, -1, 0.5, 2); got != nil {
		t.Fatalf("expected nil on topK=-1, got %+v", got)
	}
}
