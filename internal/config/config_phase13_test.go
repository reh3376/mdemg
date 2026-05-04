package config

import "testing"

// TestPhase13_Defaults documents the production-on rollout posture (Phase 13.1
// flipped this from safe-off to default-on after the embedding-heavy preset
// passed full 120q UVTS A/B with mean +0.023 and 30 improvements). RRF k=60,
// structural hops=2, all per-column enable knobs default to true. Default
// weights are the Phase 13.1 winning preset (embedding-heavy: 0.50/0.20/0.15/0.15).
func TestPhase13_Defaults(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if !cfg.RetrievalColumnVotingEnabled {
		t.Error("RetrievalColumnVotingEnabled default = false, want true (post-Phase-13.1 default)")
	}
	if cfg.RetrievalRRFK != 60 {
		t.Errorf("RetrievalRRFK = %d, want 60 (Cormack et al.)", cfg.RetrievalRRFK)
	}
	if cfg.RetrievalColumnTimeoutFrac != 0.8 {
		t.Errorf("RetrievalColumnTimeoutFrac = %v, want 0.8", cfg.RetrievalColumnTimeoutFrac)
	}
	if cfg.RetrievalStructuralHops != 2 {
		t.Errorf("RetrievalStructuralHops = %d, want 2", cfg.RetrievalStructuralHops)
	}
	if !cfg.RetrievalColumnEmbeddingEnabled {
		t.Error("RetrievalColumnEmbeddingEnabled default = false, want true")
	}
	if !cfg.RetrievalColumnBM25Enabled {
		t.Error("RetrievalColumnBM25Enabled default = false, want true")
	}
	if !cfg.RetrievalColumnGraphEnabled {
		t.Error("RetrievalColumnGraphEnabled default = false, want true")
	}
	if !cfg.RetrievalColumnStructuralEnabled {
		t.Error("RetrievalColumnStructuralEnabled default = false, want true")
	}
	// Phase 13.1 winning preset weights
	if cfg.RetrievalColumnWeightEmbedding != 0.50 {
		t.Errorf("RetrievalColumnWeightEmbedding = %v, want 0.50 (Phase 13.1 embedding-heavy)", cfg.RetrievalColumnWeightEmbedding)
	}
	if cfg.RetrievalColumnWeightBM25 != 0.20 {
		t.Errorf("RetrievalColumnWeightBM25 = %v, want 0.20 (Phase 13.1 embedding-heavy)", cfg.RetrievalColumnWeightBM25)
	}
	if cfg.RetrievalColumnWeightGraph != 0.15 {
		t.Errorf("RetrievalColumnWeightGraph = %v, want 0.15 (Phase 13.1 embedding-heavy)", cfg.RetrievalColumnWeightGraph)
	}
	if cfg.RetrievalColumnWeightStructural != 0.15 {
		t.Errorf("RetrievalColumnWeightStructural = %v, want 0.15 (Phase 13.1 embedding-heavy)", cfg.RetrievalColumnWeightStructural)
	}
}

// TestPhase13_FlagFlip verifies an operator can opt out via .env (the default
// is true post-Phase-13.1, so the flip-test exercises the opt-out path now).
func TestPhase13_FlagFlip(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("RETRIEVAL_COLUMN_VOTING_ENABLED", "false")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.RetrievalColumnVotingEnabled {
		t.Error("RetrievalColumnVotingEnabled stayed true after env override to false")
	}
}

// TestPhase13_RejectsInvalidRRFK_Zero exercises the bound enforcement.
func TestPhase13_RejectsInvalidRRFK_Zero(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("RETRIEVAL_RRF_K", "0")
	if _, err := FromEnv(); err == nil {
		t.Error("expected error for RETRIEVAL_RRF_K=0")
	}
}

// TestPhase13_RejectsInvalidColumnTimeoutFrac exercises the (0, 1] guard.
func TestPhase13_RejectsInvalidColumnTimeoutFrac(t *testing.T) {
	for _, bad := range []string{"-0.1", "0", "1.5", "2.0"} {
		t.Run("frac="+bad, func(t *testing.T) {
			setMinimalEnv(t)
			clearLLMEnv(t)
			t.Setenv("RETRIEVAL_COLUMN_TIMEOUT_FRACTION", bad)
			if _, err := FromEnv(); err == nil {
				t.Errorf("expected error for RETRIEVAL_COLUMN_TIMEOUT_FRACTION=%s", bad)
			}
		})
	}
}

// TestPhase13_RejectsStructuralHopsOutOfRange exercises the [1, 9] guard.
func TestPhase13_RejectsStructuralHopsOutOfRange(t *testing.T) {
	for _, bad := range []string{"0", "10", "100"} {
		t.Run("hops="+bad, func(t *testing.T) {
			setMinimalEnv(t)
			clearLLMEnv(t)
			t.Setenv("RETRIEVAL_STRUCTURAL_HOPS", bad)
			if _, err := FromEnv(); err == nil {
				t.Errorf("expected error for RETRIEVAL_STRUCTURAL_HOPS=%s", bad)
			}
		})
	}
}

// TestPhase13_PerColumnSuppressionKnobsRespected verifies operators can
// disable individual columns without touching code (e.g. when a column's
// backing index is known-broken in production).
func TestPhase13_PerColumnSuppressionKnobsRespected(t *testing.T) {
	cases := []struct {
		envKey string
		field  func(c Config) bool
	}{
		{"RETRIEVAL_COLUMN_EMBEDDING_ENABLED", func(c Config) bool { return c.RetrievalColumnEmbeddingEnabled }},
		{"RETRIEVAL_COLUMN_BM25_ENABLED", func(c Config) bool { return c.RetrievalColumnBM25Enabled }},
		{"RETRIEVAL_COLUMN_GRAPH_ENABLED", func(c Config) bool { return c.RetrievalColumnGraphEnabled }},
		{"RETRIEVAL_COLUMN_STRUCTURAL_ENABLED", func(c Config) bool { return c.RetrievalColumnStructuralEnabled }},
	}
	for _, c := range cases {
		t.Run(c.envKey, func(t *testing.T) {
			setMinimalEnv(t)
			clearLLMEnv(t)
			t.Setenv(c.envKey, "false")
			cfg, err := FromEnv()
			if err != nil {
				t.Fatalf("FromEnv() err=%v", err)
			}
			if c.field(cfg) {
				t.Errorf("%s=false didn't disable column", c.envKey)
			}
		})
	}
}
