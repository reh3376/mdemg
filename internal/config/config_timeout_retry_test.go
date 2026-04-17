package config

import "testing"

func TestConsultingClassifyTimeoutMs_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.ConsultingClassifyTimeoutMs != 15000 {
		t.Errorf("ConsultingClassifyTimeoutMs = %d, want 15000", cfg.ConsultingClassifyTimeoutMs)
	}
}

func TestConsultingClassifyTimeoutMs_EnvOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("CONSULTING_CLASSIFY_TIMEOUT_MS", "20000")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.ConsultingClassifyTimeoutMs != 20000 {
		t.Errorf("ConsultingClassifyTimeoutMs = %d, want 20000", cfg.ConsultingClassifyTimeoutMs)
	}
}

func TestConsultingClassifyTimeoutMs_MinimumEnforced(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("CONSULTING_CLASSIFY_TIMEOUT_MS", "1000")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for CONSULTING_CLASSIFY_TIMEOUT_MS < 5000")
	}
}

func TestRSICLLMReflectTimeoutMs_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.RSICLLMReflectTimeoutMs != 15000 {
		t.Errorf("RSICLLMReflectTimeoutMs = %d, want 15000", cfg.RSICLLMReflectTimeoutMs)
	}
}

func TestRSICLLMReflectTimeoutMs_MinimumEnforced(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("RSIC_LLM_REFLECT_TIMEOUT_MS", "2000")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for RSIC_LLM_REFLECT_TIMEOUT_MS < 5000")
	}
}

func TestLLMRetryMaxAttempts_NewDefault(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.LLMRetryMaxAttempts != 5 {
		t.Errorf("LLMRetryMaxAttempts = %d, want 5", cfg.LLMRetryMaxAttempts)
	}
}

func TestLLMRetryMaxDelayMs_NewDefault(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.LLMRetryMaxDelayMs != 60000 {
		t.Errorf("LLMRetryMaxDelayMs = %d, want 60000", cfg.LLMRetryMaxDelayMs)
	}
}
