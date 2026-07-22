package config

import "testing"

// GUARDRAIL-PRODUCER-001: default-off (flipped in .env only after live
// smoke, per the JIMINY-CONTRADICTED-BRIDGE-001 contract), concurrency
// floor 1.
func TestGuardrailProducer_Defaults(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("GUARDRAIL_PRODUCER_ENABLED", "")
	t.Setenv("GUARDRAIL_PRODUCER_MAX_CONCURRENT", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.GuardrailProducerEnabled {
		t.Error("GuardrailProducerEnabled default = true, want false (flip after live smoke)")
	}
	if cfg.GuardrailProducerMaxConcurrent != 1 {
		t.Errorf("GuardrailProducerMaxConcurrent = %d, want 1", cfg.GuardrailProducerMaxConcurrent)
	}
}

func TestGuardrailProducer_MaxConcurrentFloor(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("GUARDRAIL_PRODUCER_MAX_CONCURRENT", "0")

	if _, err := FromEnv(); err == nil {
		t.Error("expected error for GUARDRAIL_PRODUCER_MAX_CONCURRENT=0, got nil")
	}
}

func TestGuardrailIncludeCorrections_Default(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("GUARDRAIL_INCLUDE_CORRECTIONS", "")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.GuardrailIncludeCorrections {
		t.Error("GuardrailIncludeCorrections default = true, want false (flip after live smoke)")
	}
}
