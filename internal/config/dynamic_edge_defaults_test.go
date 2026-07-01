package config

import "testing"

// RETRIEVAL-TYPED-EDGES-002: the vector-index dynamic-edge knobs default sanely.
// MinLayer=1 is the key change (the old L5SourceMinLayer=3 missed the abundant
// L1/L2 concept layers a fresh corpus actually has).
func TestDynamicEdgeVectorDefaults(t *testing.T) {
	setMinimalEnv(t)
	for _, e := range []string{"DYNAMIC_EDGE_MIN_LAYER", "DYNAMIC_EDGE_TOPK", "DYNAMIC_EDGE_SIM_THRESHOLD", "DYNAMIC_EDGE_OVERSAMPLE"} {
		t.Setenv(e, "")
	}
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.DynamicEdgeMinLayer != 1 {
		t.Errorf("DynamicEdgeMinLayer = %d, want 1 (connect the abundant concept layers)", cfg.DynamicEdgeMinLayer)
	}
	if cfg.DynamicEdgeTopK != 10 {
		t.Errorf("DynamicEdgeTopK = %d, want 10", cfg.DynamicEdgeTopK)
	}
	if cfg.DynamicEdgeSimThreshold != 0.30 {
		t.Errorf("DynamicEdgeSimThreshold = %v, want 0.30", cfg.DynamicEdgeSimThreshold)
	}
	if cfg.DynamicEdgeOversample != 8 {
		t.Errorf("DynamicEdgeOversample = %d, want 8", cfg.DynamicEdgeOversample)
	}
}
