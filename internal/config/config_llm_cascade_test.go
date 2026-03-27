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

func TestLLMCascade_AllDefaultToOpenAI(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	// Top-level defaults (now openai/gpt-5-nano)
	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "openai")
	}
	if cfg.LLMModel != "gpt-5-nano" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "gpt-5-nano")
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
		if f.provider != "openai" {
			t.Errorf("%s.Provider = %q, want %q", f.name, f.provider, "openai")
		}
		if f.model != "gpt-5-nano" {
			t.Errorf("%s.Model = %q, want %q", f.name, f.model, "gpt-5-nano")
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
	// Top-level is openai (default), Synthesis overrides to ollama
	t.Setenv("SYNTHESIS_PROVIDER", "ollama")
	t.Setenv("SYNTHESIS_MODEL", "llama3.2:3b-instruct-fp16")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	// Synthesis should use its explicit override
	if cfg.SynthesisProvider != "ollama" {
		t.Errorf("SynthesisProvider = %q, want %q", cfg.SynthesisProvider, "ollama")
	}
	if cfg.SynthesisModel != "llama3.2:3b-instruct-fp16" {
		t.Errorf("SynthesisModel = %q, want %q", cfg.SynthesisModel, "llama3.2:3b-instruct-fp16")
	}

	// Intent should still cascade from top-level (openai)
	if cfg.IntentProvider != "openai" {
		t.Errorf("IntentProvider = %q, want %q (should cascade from top-level)", cfg.IntentProvider, "openai")
	}
}

func TestLLMCascade_MetaLearnDoubleInherit(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// LLM_PROVIDER=openai (default), no EMERGENCE_PROVIDER, no METALEARN_PROVIDER
	// MetaLearn → Emergence → LLM_PROVIDER

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.MetaLearnProvider != "openai" {
		t.Errorf("MetaLearnProvider = %q, want %q", cfg.MetaLearnProvider, "openai")
	}
	if cfg.MetaLearnModel != "gpt-5-nano" {
		t.Errorf("MetaLearnModel = %q, want %q", cfg.MetaLearnModel, "gpt-5-nano")
	}
}

func TestEmbedding_DefaultOpenAI(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.EmbeddingProvider != "openai" {
		t.Errorf("EmbeddingProvider = %q, want %q", cfg.EmbeddingProvider, "openai")
	}
}

func TestOllamaModel_DefaultQwen(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}

	if cfg.OllamaModel != "qwen3-embedding:8b" {
		t.Errorf("OllamaModel = %q, want %q", cfg.OllamaModel, "qwen3-embedding:8b")
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

func TestGenerateConfigYAML_JiminyEnabled(t *testing.T) {
	opts := InitOptions{
		Neo4jURI:       "bolt://localhost:7687",
		Neo4jUser:      "neo4j",
		ServerPort:     9999,
		LLMProvider:    "openai",
		LLMModel:       "gpt-5-nano",
		JiminyEnabled:  true,
		JiminyProvider: "openai",
		JiminyModel:    "gpt-5.4-nano",
	}

	data, err := GenerateConfigYAML(opts)
	if err != nil {
		t.Fatalf("GenerateConfigYAML error: %v", err)
	}

	yamlStr := string(data)
	if !contains(yamlStr, "enabled: true") {
		t.Error("generated YAML missing jiminy 'enabled: true'")
	}
	if !contains(yamlStr, "synthesis_enabled: true") {
		t.Error("generated YAML missing 'synthesis_enabled: true'")
	}
	if !contains(yamlStr, "synthesis_model: gpt-5.4-nano") {
		t.Error("generated YAML missing 'synthesis_model: gpt-5.4-nano'")
	}
	if !contains(yamlStr, "evaluate_enabled: true") {
		t.Error("generated YAML missing 'evaluate_enabled: true'")
	}
	if !contains(yamlStr, "evaluate_llm_model: gpt-5.4-nano") {
		t.Error("generated YAML missing 'evaluate_llm_model: gpt-5.4-nano'")
	}
}

func TestGenerateConfigYAML_JiminyDeclined(t *testing.T) {
	opts := InitOptions{
		Neo4jURI:      "bolt://localhost:7687",
		Neo4jUser:     "neo4j",
		ServerPort:    9999,
		LLMProvider:   "openai",
		LLMModel:      "gpt-5-nano",
		JiminyEnabled: false,
	}

	data, err := GenerateConfigYAML(opts)
	if err != nil {
		t.Fatalf("GenerateConfigYAML error: %v", err)
	}

	yamlStr := string(data)
	// Must contain jiminy section with explicit false
	if !contains(yamlStr, "jiminy:") {
		t.Error("generated YAML missing 'jiminy:' section")
	}
	// Must NOT contain synthesis_enabled (since Jiminy is disabled)
	if contains(yamlStr, "synthesis_enabled") {
		t.Error("generated YAML should not contain 'synthesis_enabled' when Jiminy declined")
	}
}

func TestFlattenYAML_JiminyAlwaysEmitsEnabled(t *testing.T) {
	// When Jiminy is disabled, flattenYAML must still emit jiminy.enabled=false
	// so the server default (true) doesn't silently re-enable it.
	yamlContent := `jiminy:
  enabled: false
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JIMINY_ENABLED", "")

	if err := LoadYAMLConfig(configPath); err != nil {
		t.Fatalf("LoadYAMLConfig error: %v", err)
	}

	if got := os.Getenv("JIMINY_ENABLED"); got != "false" {
		t.Errorf("JIMINY_ENABLED = %q, want %q", got, "false")
	}
}

func TestFlattenYAML_JiminyAbsentDoesNotOverride(t *testing.T) {
	// When YAML has no jiminy section, JIMINY_ENABLED must NOT be set,
	// allowing the .env or default (true) to take effect.
	yamlContent := `space_id: test-space
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JIMINY_ENABLED", "")

	if err := LoadYAMLConfig(configPath); err != nil {
		t.Fatalf("LoadYAMLConfig error: %v", err)
	}

	if got := os.Getenv("JIMINY_ENABLED"); got != "" {
		t.Errorf("JIMINY_ENABLED = %q, want empty (not set by YAML)", got)
	}
}
