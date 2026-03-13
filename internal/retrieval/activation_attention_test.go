package retrieval

import (
	"testing"

	"mdemg/internal/config"
)

func TestEdgeAttentionWeights_GetEdgeAttention(t *testing.T) {
	weights := EdgeAttentionWeights{
		CoActivated: 0.85,
		Associated:  0.65,
		Generalizes: 0.65,
		AbstractsTo: 0.60,
		Temporal:    0.45,
	}

	tests := []struct {
		relType  string
		expected float64
	}{
		{"CO_ACTIVATED_WITH", 0.85},
		{"ASSOCIATED_WITH", 0.65},
		{"GENERALIZES", 0.65},
		{"ABSTRACTS_TO", 0.60},
		{"TEMPORALLY_ADJACENT", 0.45},
		{"UNKNOWN_TYPE", 0.5}, // Unknown types get neutral weight
	}

	for _, tt := range tests {
		got := weights.GetEdgeAttention(tt.relType)
		if got != tt.expected {
			t.Errorf("GetEdgeAttention(%q) = %v, want %v", tt.relType, got, tt.expected)
		}
	}
}

func TestComputeEdgeAttention_Default(t *testing.T) {
	cfg := config.Config{
		EdgeAttentionCoActivated: 0.85,
		EdgeAttentionAssociated:  0.65,
		EdgeAttentionGeneralizes: 0.65,
		EdgeAttentionAbstractsTo: 0.60,
		EdgeAttentionTemporal:    0.45,
		EdgeAttentionCodeBoost:   1.2,
		EdgeAttentionArchBoost:   1.5,
	}

	// Default query (neither code nor architecture)
	ctx := QueryContext{
		QueryText:   "what is this module?",
		IsCodeQuery: false,
		IsArchQuery: false,
	}

	weights := ComputeEdgeAttention(ctx, cfg)

	// Should match config defaults
	if weights.CoActivated != 0.85 {
		t.Errorf("Default CoActivated = %v, want 0.85", weights.CoActivated)
	}
	if weights.Associated != 0.65 {
		t.Errorf("Default Associated = %v, want 0.65", weights.Associated)
	}
	if weights.Generalizes != 0.65 {
		t.Errorf("Default Generalizes = %v, want 0.65", weights.Generalizes)
	}
}

func TestComputeEdgeAttention_CodeQuery(t *testing.T) {
	cfg := config.Config{
		EdgeAttentionCoActivated: 0.85,
		EdgeAttentionAssociated:  0.65,
		EdgeAttentionGeneralizes: 0.65,
		EdgeAttentionAbstractsTo: 0.60,
		EdgeAttentionTemporal:    0.45,
		EdgeAttentionCodeBoost:   1.2,
		EdgeAttentionArchBoost:   1.5,
	}

	ctx := QueryContext{
		QueryText:   "where is the function defined?",
		IsCodeQuery: true,
		IsArchQuery: false,
	}

	weights := ComputeEdgeAttention(ctx, cfg)

	// Code query should boost CO_ACTIVATED and reduce hierarchical
	expectedCoActivated := 0.85 * 1.2 // 1.02 -> clamped to 1.0
	if weights.CoActivated != 1.0 {
		t.Errorf("Code query CoActivated = %v, want 1.0 (clamped from %v)", weights.CoActivated, expectedCoActivated)
	}

	// Generalizes should be reduced for code queries
	expectedGeneralizes := 0.65 * 0.6 // 0.39
	if weights.Generalizes != expectedGeneralizes {
		t.Errorf("Code query Generalizes = %v, want %v", weights.Generalizes, expectedGeneralizes)
	}

	// AbstractsTo should be reduced for code queries
	expectedAbstractsTo := 0.60 * 0.5 // 0.30
	if weights.AbstractsTo != expectedAbstractsTo {
		t.Errorf("Code query AbstractsTo = %v, want %v", weights.AbstractsTo, expectedAbstractsTo)
	}
}

func TestComputeEdgeAttention_ArchitectureQuery(t *testing.T) {
	cfg := config.Config{
		EdgeAttentionCoActivated: 0.85,
		EdgeAttentionAssociated:  0.65,
		EdgeAttentionGeneralizes: 0.65,
		EdgeAttentionAbstractsTo: 0.60,
		EdgeAttentionTemporal:    0.45,
		EdgeAttentionCodeBoost:   1.2,
		EdgeAttentionArchBoost:   1.5,
	}

	ctx := QueryContext{
		QueryText:   "what is the architecture pattern?",
		IsCodeQuery: false,
		IsArchQuery: true,
	}

	weights := ComputeEdgeAttention(ctx, cfg)

	// Architecture query should boost hierarchical edges
	// Use approximate comparison for floating point
	expectedGeneralizes := 0.65 * 1.5 // 0.975
	if !approxEqual(weights.Generalizes, expectedGeneralizes, 0.0001) {
		t.Errorf("Arch query Generalizes = %v, want %v", weights.Generalizes, expectedGeneralizes)
	}

	expectedAbstractsTo := 0.60 * 1.5 // 0.90
	if !approxEqual(weights.AbstractsTo, expectedAbstractsTo, 0.0001) {
		t.Errorf("Arch query AbstractsTo = %v, want %v", weights.AbstractsTo, expectedAbstractsTo)
	}

	// CoActivated should be reduced for architecture queries
	expectedCoActivated := 0.85 * 0.8 // 0.68
	if !approxEqual(weights.CoActivated, expectedCoActivated, 0.0001) {
		t.Errorf("Arch query CoActivated = %v, want %v", weights.CoActivated, expectedCoActivated)
	}
}

// approxEqual checks if two floats are approximately equal within tolerance
func approxEqual(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

func TestSpreadingActivationWithAttention_AllEdgeTypes(t *testing.T) {
	// Create test candidates
	cands := []Candidate{
		{NodeID: "a", VectorSim: 1.0},
		{NodeID: "b", VectorSim: 0.5},
		{NodeID: "c", VectorSim: 0.3},
	}

	// Create edges of different types
	edges := []Edge{
		{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH", Weight: 0.8},
		{Src: "a", Dst: "c", RelType: "GENERALIZES", Weight: 0.8},
	}

	// Test with equal attention - both edges should contribute
	equalAttn := EdgeAttentionWeights{
		CoActivated: 0.5,
		Generalizes: 0.5,
	}

	act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, equalAttn, nil)

	// Both b and c should have increased activation from a
	if act["b"] <= 0.5 {
		t.Errorf("Node b activation = %v, expected > 0.5 (initial)", act["b"])
	}
	if act["c"] <= 0.3 {
		t.Errorf("Node c activation = %v, expected > 0.3 (initial)", act["c"])
	}
}

func TestSpreadingActivationWithAttention_CodeQueryFavorsCoActivated(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", VectorSim: 1.0},
		{NodeID: "b", VectorSim: 0.0}, // Only gets activation from edges
		{NodeID: "c", VectorSim: 0.0}, // Only gets activation from edges
	}

	edges := []Edge{
		{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH", Weight: 0.8},
		{Src: "a", Dst: "c", RelType: "GENERALIZES", Weight: 0.8},
	}

	// Code query attention: high CO_ACTIVATED, low GENERALIZES
	codeAttn := EdgeAttentionWeights{
		CoActivated: 1.0,
		Generalizes: 0.2,
	}

	act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, codeAttn, nil)

	// b (CO_ACTIVATED) should have higher activation than c (GENERALIZES)
	if act["b"] <= act["c"] {
		t.Errorf("Code query: b activation (%v) should be > c activation (%v)", act["b"], act["c"])
	}
}

func TestSpreadingActivationWithAttention_ArchQueryFavorsHierarchical(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", VectorSim: 1.0},
		{NodeID: "b", VectorSim: 0.0}, // Only gets activation from edges
		{NodeID: "c", VectorSim: 0.0}, // Only gets activation from edges
	}

	edges := []Edge{
		{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH", Weight: 0.8},
		{Src: "a", Dst: "c", RelType: "GENERALIZES", Weight: 0.8},
	}

	// Architecture query attention: low CO_ACTIVATED, high GENERALIZES
	archAttn := EdgeAttentionWeights{
		CoActivated: 0.3,
		Generalizes: 1.0,
	}

	act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, archAttn, nil)

	// c (GENERALIZES) should have higher activation than b (CO_ACTIVATED)
	if act["c"] <= act["b"] {
		t.Errorf("Arch query: c activation (%v) should be > b activation (%v)", act["c"], act["b"])
	}
}

func TestSpreadingActivationWithAttention_ZeroAttentionIgnoresEdge(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", VectorSim: 1.0},
		{NodeID: "b", VectorSim: 0.0},
	}

	edges := []Edge{
		{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH", Weight: 0.8},
	}

	// Zero attention for CO_ACTIVATED
	zeroAttn := EdgeAttentionWeights{
		CoActivated: 0.0,
	}

	act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, zeroAttn, nil)

	// b should have zero activation (no seeding, no edge contribution)
	if act["b"] != 0.0 {
		t.Errorf("Zero attention: b activation = %v, expected 0.0", act["b"])
	}
}

func TestDefaultEdgeAttention(t *testing.T) {
	attn := DefaultEdgeAttention()

	// Should only have CO_ACTIVATED enabled (matches original behavior)
	if attn.CoActivated != 1.0 {
		t.Errorf("Default CoActivated = %v, want 1.0", attn.CoActivated)
	}
	if attn.Associated != 0.0 {
		t.Errorf("Default Associated = %v, want 0.0", attn.Associated)
	}
	if attn.Generalizes != 0.0 {
		t.Errorf("Default Generalizes = %v, want 0.0", attn.Generalizes)
	}
	if attn.AbstractsTo != 0.0 {
		t.Errorf("Default AbstractsTo = %v, want 0.0", attn.AbstractsTo)
	}
	if attn.Temporal != 0.0 {
		t.Errorf("Default Temporal = %v, want 0.0", attn.Temporal)
	}
}

func TestClampWeight(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}

	for _, tt := range tests {
		got := clampWeight(tt.input)
		if got != tt.expected {
			t.Errorf("clampWeight(%v) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
