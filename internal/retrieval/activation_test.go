package retrieval

import (
	"testing"
)

func TestActivationSeeding_BM25OnlyUsesRRF(t *testing.T) {
	// BM25-only candidate: VectorSim=0, BM25Score=0.3, RRFScore=0.15
	// Should use RRFScore (0.15), not BM25Score (0.3)
	cands := []Candidate{
		{NodeID: "bm25-only", VectorSim: 0, BM25Score: 0.3, RRFScore: 0.15},
	}
	act := SpreadingActivation(cands, nil, 1, 0.0, nil)
	if got := act["bm25-only"]; got != 0.15 {
		t.Errorf("BM25-only candidate activation = %v, want 0.15 (RRFScore)", got)
	}
}

func TestActivationSeeding_VectorOnlyUsesRRF(t *testing.T) {
	// Vector-only candidate: VectorSim=0.8, BM25Score=0, RRFScore=0.12
	// Should use RRFScore (0.12), not VectorSim (0.8)
	cands := []Candidate{
		{NodeID: "vec-only", VectorSim: 0.8, BM25Score: 0, RRFScore: 0.12},
	}
	act := SpreadingActivation(cands, nil, 1, 0.0, nil)
	if got := act["vec-only"]; got != 0.12 {
		t.Errorf("vector-only candidate activation = %v, want 0.12 (RRFScore)", got)
	}
}

func TestActivationSeeding_FallbackWhenNoRRF(t *testing.T) {
	// Candidate without RRF score: should fall back to max(VectorSim, BM25Score)
	cands := []Candidate{
		{NodeID: "no-rrf", VectorSim: 0.6, BM25Score: 0.4, RRFScore: 0},
	}
	act := SpreadingActivation(cands, nil, 1, 0.0, nil)
	if got := act["no-rrf"]; got != 0.6 {
		t.Errorf("no-RRF candidate activation = %v, want 0.6 (max of VectorSim, BM25Score)", got)
	}
}

func TestActivationSeeding_FallbackBM25Higher(t *testing.T) {
	// Fallback where BM25Score > VectorSim
	cands := []Candidate{
		{NodeID: "bm25-higher", VectorSim: 0.2, BM25Score: 0.5, RRFScore: 0},
	}
	act := SpreadingActivation(cands, nil, 1, 0.0, nil)
	if got := act["bm25-higher"]; got != 0.5 {
		t.Errorf("fallback activation = %v, want 0.5 (BM25Score > VectorSim)", got)
	}
}
