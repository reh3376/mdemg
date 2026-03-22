package cli

import (
	"testing"
)

func TestSpeedPresets_AllPresetsExist(t *testing.T) {
	for _, name := range []string{"fast", "balanced", "thorough"} {
		if _, ok := speedPresets[name]; !ok {
			t.Errorf("speedPresets missing expected key %q", name)
		}
	}
}

func TestSpeedPreset_Fast(t *testing.T) {
	sp, ok := speedPresets["fast"]
	if !ok {
		t.Fatal("speedPresets missing 'fast'")
	}
	if sp.workers != 8 {
		t.Errorf("fast.workers = %d, want 8", sp.workers)
	}
	if sp.batchSize != 250 {
		t.Errorf("fast.batchSize = %d, want 250", sp.batchSize)
	}
	if sp.llmSummary != false {
		t.Error("fast.llmSummary = true, want false")
	}
	if sp.symbols != false {
		t.Error("fast.symbols = true, want false")
	}
	if sp.consolidate != false {
		t.Error("fast.consolidate = true, want false")
	}
	if sp.delay != 0 {
		t.Errorf("fast.delay = %d, want 0", sp.delay)
	}
	if sp.llmBatch != 0 {
		t.Errorf("fast.llmBatch = %d, want 0", sp.llmBatch)
	}
}

func TestSpeedPreset_Balanced(t *testing.T) {
	sp, ok := speedPresets["balanced"]
	if !ok {
		t.Fatal("speedPresets missing 'balanced'")
	}
	if sp.workers != 4 {
		t.Errorf("balanced.workers = %d, want 4", sp.workers)
	}
	if sp.batchSize != 100 {
		t.Errorf("balanced.batchSize = %d, want 100", sp.batchSize)
	}
	if sp.llmSummary != true {
		t.Error("balanced.llmSummary = false, want true")
	}
	if sp.llmBatch != 10 {
		t.Errorf("balanced.llmBatch = %d, want 10", sp.llmBatch)
	}
	if sp.symbols != true {
		t.Error("balanced.symbols = false, want true")
	}
	if sp.consolidate != true {
		t.Error("balanced.consolidate = false, want true")
	}
	if sp.delay != 50 {
		t.Errorf("balanced.delay = %d, want 50", sp.delay)
	}
}

func TestSpeedPreset_Thorough(t *testing.T) {
	sp, ok := speedPresets["thorough"]
	if !ok {
		t.Fatal("speedPresets missing 'thorough'")
	}
	if sp.workers != 8 {
		t.Errorf("thorough.workers = %d, want 8", sp.workers)
	}
	if sp.batchSize != 200 {
		t.Errorf("thorough.batchSize = %d, want 200", sp.batchSize)
	}
	if sp.llmSummary != true {
		t.Error("thorough.llmSummary = false, want true")
	}
	if sp.llmBatch != 20 {
		t.Errorf("thorough.llmBatch = %d, want 20", sp.llmBatch)
	}
	if sp.symbols != true {
		t.Error("thorough.symbols = false, want true")
	}
	if sp.consolidate != true {
		t.Error("thorough.consolidate = false, want true")
	}
	if sp.delay != 25 {
		t.Errorf("thorough.delay = %d, want 25", sp.delay)
	}
}

func TestBuildInitialIngestConfig_WithSpeedEnv(t *testing.T) {
	t.Setenv("INGEST_SPEED", "fast")

	cfg := buildInitialIngestConfig(".", "test", "openai", "gpt-4o-mini")

	if cfg.workers != 8 {
		t.Errorf("workers = %d, want 8", cfg.workers)
	}
	if cfg.batchSize != 250 {
		t.Errorf("batchSize = %d, want 250", cfg.batchSize)
	}
	if cfg.extractSymbols != false {
		t.Error("extractSymbols = true, want false")
	}
	if cfg.consolidate != false {
		t.Error("consolidate = true, want false")
	}
	if cfg.delay != 0 {
		t.Errorf("delay = %d, want 0", cfg.delay)
	}
	if cfg.llmSummary != false {
		t.Error("llmSummary = true, want false")
	}
}

func TestBuildInitialIngestConfig_SpeedPreservesLLMProvider(t *testing.T) {
	t.Setenv("INGEST_SPEED", "fast")

	cfg := buildInitialIngestConfig(".", "test", "openai", "gpt-4o-mini")

	if cfg.llmSummaryProvider != "openai" {
		t.Errorf("llmSummaryProvider = %q, want %q", cfg.llmSummaryProvider, "openai")
	}
	if cfg.llmSummaryModel != "gpt-4o-mini" {
		t.Errorf("llmSummaryModel = %q, want %q", cfg.llmSummaryModel, "gpt-4o-mini")
	}
}
