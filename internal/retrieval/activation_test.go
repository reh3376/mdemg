package retrieval

import (
	"testing"

	"mdemg/internal/config"
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

// RETRIEVAL-TYPED-EDGES-001 Epic 1: the typed semantic-edge attention weights are
// config-driven (were hardcoded in ComputeEdgeAttention), and the defaults match
// the prior literals (no behavior change at default config).
func TestComputeEdgeAttention_SemanticWeightsConfigDriven(t *testing.T) {
	// Defaults preserve the prior hardcoded literals.
	def := config.Config{
		EdgeAttentionAnalogousTo:   0.55,
		EdgeAttentionBridges:       0.60,
		EdgeAttentionComposesWith:  0.50,
		EdgeAttentionContrastsWith: 0.40,
		EdgeAttentionInfluences:    0.45,
		EdgeAttentionDefinesSymbol: 0.70,
		EdgeAttentionThemeOf:       0.65,
	}
	w := ComputeEdgeAttention(QueryContext{}, def)
	for name, got := range map[string]float64{
		"AnalogousTo": w.AnalogousTo, "Bridges": w.Bridges, "ComposesWith": w.ComposesWith,
		"ContrastsWith": w.ContrastsWith, "Influences": w.Influences,
		"DefinesSymbol": w.DefinesSymbol, "ThemeOf": w.ThemeOf,
	} {
		want := map[string]float64{
			"AnalogousTo": 0.55, "Bridges": 0.60, "ComposesWith": 0.50,
			"ContrastsWith": 0.40, "Influences": 0.45, "DefinesSymbol": 0.70, "ThemeOf": 0.65,
		}[name]
		if got != want {
			t.Errorf("%s default = %v, want %v", name, got, want)
		}
	}

	// Custom config values flow through (proves they are NOT hardcoded).
	custom := config.Config{
		EdgeAttentionAnalogousTo:   0.91,
		EdgeAttentionBridges:       0.92,
		EdgeAttentionComposesWith:  0.93,
		EdgeAttentionContrastsWith: 0.94,
		EdgeAttentionInfluences:    0.95,
		EdgeAttentionDefinesSymbol: 0.96,
		EdgeAttentionThemeOf:       0.97,
	}
	c := ComputeEdgeAttention(QueryContext{}, custom)
	if c.AnalogousTo != 0.91 || c.Bridges != 0.92 || c.ThemeOf != 0.97 {
		t.Errorf("custom weights not honored: analogous=%v bridges=%v themeof=%v", c.AnalogousTo, c.Bridges, c.ThemeOf)
	}
}
