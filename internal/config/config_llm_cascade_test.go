package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearLLMEnv unsets all LLM-related env vars to ensure clean cascade testing.
func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"LLM_PROVIDER", "LLM_MODEL",
		"RERANK_PROVIDER", "RERANK_MODEL",
		"LLM_SUMMARY_PROVIDER", "LLM_SUMMARY_MODEL",
		"SYNTHESIS_PROVIDER", "SYNTHESIS_MODEL",
		"INTENT_PROVIDER", "INTENT_MODEL",
		"EMERGENCE_PROVIDER", "EMERGENCE_MODEL",
		"GUARDRAIL_PROVIDER", "GUARDRAIL_MODEL",
		"METALEARN_PROVIDER", "METALEARN_MODEL",
		"EMBEDDING_PROVIDER", "OLLAMA_MODEL",
	} {
		t.Setenv(key, "")
	}
}

// setMinimalEnv sets the minimum env vars needed for FromEnv to succeed.
func setMinimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "test")
}

func TestLLMCascade_AllDefaultToOllama(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	// Top-level defaults
	if cfg.LLMProvider != "ollama" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "ollama")
	}
	if cfg.LLMModel != "llama3.2:3b-instruct-fp16" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "llama3.2:3b-instruct-fp16")
	}

	// All features should cascade from top-level
	features := []struct {
		name     string
		provider string
		model    string
	}{
		{"Rerank", cfg.RerankProvider, cfg.RerankModel},
		{"LLMSummary", cfg.LLMSummaryProvider, cfg.LLMSummaryModel},
		{"Synthesis", cfg.SynthesisProvider, cfg.SynthesisModel},
		{"Intent", cfg.IntentProvider, cfg.IntentModel},
		{"Emergence", cfg.EmergenceProvider, cfg.EmergenceModel},
		{"Guardrail", cfg.GuardrailProvider, cfg.GuardrailModel},
	}

	for _, f := range features {
		if f.provider != "ollama" {
			t.Errorf("%s.Provider = %q, want %q", f.name, f.provider, "ollama")
		}
		if f.model != "llama3.2:3b-instruct-fp16" {
			t.Errorf("%s.Model = %q, want %q", f.name, f.model, "llama3.2:3b-instruct-fp16")
		}
	}
}

func TestLLMCascade_TopLevelOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_MODEL", "gpt-4o")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.SynthesisProvider != "openai" {
		t.Errorf("SynthesisProvider = %q, want %q", cfg.SynthesisProvider, "openai")
	}
	if cfg.SynthesisModel != "gpt-4o" {
		t.Errorf("SynthesisModel = %q, want %q", cfg.SynthesisModel, "gpt-4o")
	}
	if cfg.IntentProvider != "openai" {
		t.Errorf("IntentProvider = %q, want %q", cfg.IntentProvider, "openai")
	}
	if cfg.EmergenceProvider != "openai" {
		t.Errorf("EmergenceProvider = %q, want %q", cfg.EmergenceProvider, "openai")
	}
}

func TestLLMCascade_PerFeatureOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// Top-level is ollama (default), but Synthesis overrides to openai
	t.Setenv("SYNTHESIS_PROVIDER", "openai")
	t.Setenv("SYNTHESIS_MODEL", "gpt-4o-mini")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	// Synthesis should use its explicit override
	if cfg.SynthesisProvider != "openai" {
		t.Errorf("SynthesisProvider = %q, want %q", cfg.SynthesisProvider, "openai")
	}
	if cfg.SynthesisModel != "gpt-4o-mini" {
		t.Errorf("SynthesisModel = %q, want %q", cfg.SynthesisModel, "gpt-4o-mini")
	}

	// Intent should still cascade from top-level (ollama)
	if cfg.IntentProvider != "ollama" {
		t.Errorf("IntentProvider = %q, want %q (should cascade from top-level)", cfg.IntentProvider, "ollama")
	}
}

func TestLLMCascade_MetaLearnDoubleInherit(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// LLM_PROVIDER=ollama (default), no EMERGENCE_PROVIDER, no METALEARN_PROVIDER
	// MetaLearn → Emergence → LLM_PROVIDER

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.MetaLearnProvider != "ollama" {
		t.Errorf("MetaLearnProvider = %q, want %q", cfg.MetaLearnProvider, "ollama")
	}
	if cfg.MetaLearnModel != "llama3.2:3b-instruct-fp16" {
		t.Errorf("MetaLearnModel = %q, want %q", cfg.MetaLearnModel, "llama3.2:3b-instruct-fp16")
	}
}

func TestEmbedding_DefaultOllama(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.EmbeddingProvider != "ollama" {
		t.Errorf("EmbeddingProvider = %q, want %q", cfg.EmbeddingProvider, "ollama")
	}
}

func TestOllamaModel_DefaultQwen(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.OllamaModel != "qwen3-embedding:4b" {
		t.Errorf("OllamaModel = %q, want %q", cfg.OllamaModel, "qwen3-embedding:4b")
	}
}

func TestBackwardCompat_ExplicitOpenAI(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// Simulate old config: explicit openai for everything
	t.Setenv("EMBEDDING_PROVIDER", "openai")
	t.Setenv("RERANK_PROVIDER", "openai")
	t.Setenv("RERANK_MODEL", "gpt-4o-mini")
	t.Setenv("SYNTHESIS_PROVIDER", "openai")
	t.Setenv("SYNTHESIS_MODEL", "gpt-4o-mini")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.EmbeddingProvider != "openai" {
		t.Errorf("EmbeddingProvider = %q, want %q", cfg.EmbeddingProvider, "openai")
	}
	if cfg.RerankProvider != "openai" {
		t.Errorf("RerankProvider = %q, want %q", cfg.RerankProvider, "openai")
	}
	if cfg.SynthesisProvider != "openai" {
		t.Errorf("SynthesisProvider = %q, want %q", cfg.SynthesisProvider, "openai")
	}
}

func TestYAMLConfig_LLMSection(t *testing.T) {
	// Write a YAML config with LLM section
	dir := t.TempDir()
	mdemgDir := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `neo4j:
  uri: bolt://localhost:7687
  user: neo4j
llm:
  provider: ollama
  model: llama3.2:3b-instruct-fp16
embedding:
  provider: ollama
  model: qwen3-embedding:4b
`
	configPath := filepath.Join(mdemgDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Clear env vars to let YAML values apply
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_MODEL", "")

	if err := LoadYAMLConfig(configPath); err != nil {
		t.Fatalf("LoadYAMLConfig error: %v", err)
	}

	if got := os.Getenv("LLM_PROVIDER"); got != "ollama" {
		t.Errorf("LLM_PROVIDER = %q, want %q", got, "ollama")
	}
	if got := os.Getenv("LLM_MODEL"); got != "llama3.2:3b-instruct-fp16" {
		t.Errorf("LLM_MODEL = %q, want %q", got, "llama3.2:3b-instruct-fp16")
	}
}

func TestYAMLConfig_LLMSection_GenerateRoundtrip(t *testing.T) {
	opts := InitOptions{
		Neo4jURI:          "bolt://localhost:7687",
		Neo4jUser:         "neo4j",
		ServerPort:        9999,
		LLMProvider:       "ollama",
		LLMModel:          "llama3.2:3b-instruct-fp16",
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "qwen3-embedding:4b",
		EmbeddingEndpoint: "http://localhost:11434",
		SchemaVersion:     17,
	}

	data, err := GenerateConfigYAML(opts)
	if err != nil {
		t.Fatalf("GenerateConfigYAML error: %v", err)
	}

	yamlStr := string(data)
	if !contains(yamlStr, "provider: ollama") {
		t.Error("generated YAML missing 'provider: ollama'")
	}
	if !contains(yamlStr, "model: llama3.2:3b-instruct-fp16") {
		t.Error("generated YAML missing 'model: llama3.2:3b-instruct-fp16'")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && // avoid trivial match
		len(s) >= len(substr) &&
		findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
