package cli

import (
	"strings"
	"testing"

	"mdemg/internal/config"
)

// CONFIG-DEADFLAG-001: env-parsed LLM_SUMMARY_* values fill unpinned settings;
// values pinned by an explicit flag or speed preset always win.
func TestApplyLLMSummaryEnvDefaults(t *testing.T) {
	appCfg := &config.Config{
		LLMSummaryEnabled:   false,
		LLMSummaryMaxTokens: 200,
		LLMSummaryBatchSize: 25,
		LLMSummaryTimeoutMs: 45000,
		LLMSummaryCacheSize: 1000,
	}

	// Nothing pinned — env config supplies every value.
	cfg := &ingestConfig{llmSummary: true, llmSummaryBatch: 10, llmSummaryMaxTokens: 150, llmSummaryTimeoutMs: 30000, llmSummaryCacheSize: 5000}
	applyLLMSummaryEnvDefaults(cfg, appCfg)
	if cfg.llmSummary != false {
		t.Error("unpinned llmSummary should take env value false")
	}
	if cfg.llmSummaryBatch != 25 || cfg.llmSummaryMaxTokens != 200 || cfg.llmSummaryTimeoutMs != 45000 || cfg.llmSummaryCacheSize != 1000 {
		t.Errorf("unpinned values should take env values, got batch=%d maxTokens=%d timeout=%d cache=%d",
			cfg.llmSummaryBatch, cfg.llmSummaryMaxTokens, cfg.llmSummaryTimeoutMs, cfg.llmSummaryCacheSize)
	}

	// Everything pinned — env config must not override.
	cfg = &ingestConfig{
		llmSummary: true, llmSummaryBatch: 10, llmSummaryMaxTokens: 150, llmSummaryTimeoutMs: 30000, llmSummaryCacheSize: 5000,
		llmSummaryPinned: true, llmSummaryBatchPinned: true, llmSummaryMaxTokensPinned: true,
		llmSummaryTimeoutMsPinned: true, llmSummaryCacheSizePinned: true,
	}
	applyLLMSummaryEnvDefaults(cfg, appCfg)
	if !cfg.llmSummary || cfg.llmSummaryBatch != 10 || cfg.llmSummaryMaxTokens != 150 || cfg.llmSummaryTimeoutMs != 30000 || cfg.llmSummaryCacheSize != 5000 {
		t.Errorf("pinned values must win over env config, got enabled=%v batch=%d maxTokens=%d timeout=%d cache=%d",
			cfg.llmSummary, cfg.llmSummaryBatch, cfg.llmSummaryMaxTokens, cfg.llmSummaryTimeoutMs, cfg.llmSummaryCacheSize)
	}
}

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

func TestGenerateSummary_DocCommentEnrichment(t *testing.T) {
	elem := codeElement{
		Name: "Classify",
		Kind: "function",
		Symbols: []ingestSymbol{
			{
				Name:       "Classify",
				Type:       "function",
				Exported:   true,
				DocComment: "Classify determines the outcome of a guidance item given an action summary.",
			},
		},
	}
	got := generateSummary(elem)
	if !strings.Contains(got, "Classify determines the outcome") {
		t.Errorf("should include DocComment, got: %s", got)
	}
}

func TestGenerateSummary_NoDocComment(t *testing.T) {
	elem := codeElement{
		Name: "Classify",
		Kind: "function",
		Symbols: []ingestSymbol{
			{
				Name:     "Classify",
				Type:     "function",
				Exported: true,
			},
		},
	}
	got := generateSummary(elem)
	// Without DocComment, summary includes symbol names but no doc comment text
	if strings.Contains(got, "determines the outcome") {
		t.Errorf("should NOT include DocComment text when absent, got: %s", got)
	}
	if !strings.HasPrefix(got, "Function Classify") {
		t.Errorf("should start with function name, got: %s", got)
	}
}

func TestGenerateSummary_DocCommentTruncation(t *testing.T) {
	longDoc := strings.Repeat("A detailed description. ", 30) // ~720 chars
	elem := codeElement{
		Name: "Process",
		Kind: "function",
		Symbols: []ingestSymbol{
			{
				Name:       "Process",
				Type:       "function",
				Exported:   true,
				DocComment: longDoc,
			},
		},
	}
	got := generateSummary(elem)
	// The DocComment portion should be capped at 400 chars.
	// Total summary = "Function Process. " + 400-char doc = ~418 chars, well under 700 max.
	if len(got) > 700 {
		t.Errorf("summary should be under 700 chars, got %d", len(got))
	}
	// Should contain the start of the doc comment
	if !strings.Contains(got, "A detailed description") {
		t.Errorf("should include truncated DocComment, got: %s", got)
	}
}

func TestGenerateSummary_DocCommentOnlyMatchingExported(t *testing.T) {
	elem := codeElement{
		Name: "Service",
		Kind: "struct",
		Symbols: []ingestSymbol{
			{
				Name:       "helper",
				Type:       "function",
				Exported:   false,
				DocComment: "helper is an internal utility function.",
			},
			{
				Name:       "Service",
				Type:       "struct",
				Exported:   true,
				DocComment: "Service orchestrates the core business logic.",
			},
		},
	}
	got := generateSummary(elem)
	if !strings.Contains(got, "Service orchestrates") {
		t.Errorf("should include matching exported symbol DocComment, got: %s", got)
	}
	if strings.Contains(got, "helper is an internal") {
		t.Errorf("should NOT include non-matching symbol DocComment, got: %s", got)
	}
}
