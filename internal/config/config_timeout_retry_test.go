package config

import "testing"

func TestConsultingClassifyTimeoutMs_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// DH-004 E4.1: bumped 15000 -> 30000 to match JIMINY_SYNTHESIS_TIMEOUT_MS
	// and survive typical gpt-5.4-mini latency under load without tripping the
	// circuit breaker on a single slow call.
	t.Setenv("CONSULTING_CLASSIFY_TIMEOUT_MS", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.ConsultingClassifyTimeoutMs != 30000 {
		t.Errorf("ConsultingClassifyTimeoutMs = %d, want 30000 (DH-004 raised from 15000)", cfg.ConsultingClassifyTimeoutMs)
	}
}

// DH-004 E4.2: LLM_RETRY_DEADLINE_ENABLED defaults true.
func TestLLMRetryOnDeadline_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("LLM_RETRY_DEADLINE_ENABLED", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if !cfg.LLMRetryOnDeadline {
		t.Error("LLMRetryOnDeadline default = false, want true")
	}
}

func TestLLMRetryOnDeadline_EnvOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("LLM_RETRY_DEADLINE_ENABLED", "false")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.LLMRetryOnDeadline {
		t.Error("LLMRetryOnDeadline = true after env override to false, want false")
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

// DH-004 E3: J17 sidecar timeout default raised 200→1000ms and given a
// 100ms floor.
func TestJ17SidecarTimeoutMs_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_SIDECAR_TIMEOUT_MS", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17SidecarTimeoutMs != 1000 {
		t.Errorf("J17SidecarTimeoutMs = %d, want 1000 (DH-004 raised from 200)", cfg.J17SidecarTimeoutMs)
	}
}

func TestJ17SidecarTimeoutMs_EnvOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_SIDECAR_TIMEOUT_MS", "2500")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17SidecarTimeoutMs != 2500 {
		t.Errorf("J17SidecarTimeoutMs = %d, want 2500", cfg.J17SidecarTimeoutMs)
	}
}

func TestJ17SidecarTimeoutMs_FloorEnforced(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	// Values below 100ms are almost certainly misconfigurations — clamp, not error.
	t.Setenv("J17_SIDECAR_TIMEOUT_MS", "50")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17SidecarTimeoutMs != 100 {
		t.Errorf("J17SidecarTimeoutMs = %d, want 100 (floor)", cfg.J17SidecarTimeoutMs)
	}
}

// DASHBOARD-TRUTH-001: min-sample floor for the NLI calibration bias alert.
func TestJ17NLICalibrationMinSamples_Default(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_NLI_CALIBRATION_MIN_SAMPLES", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17NLICalibrationMinSamples != 50 {
		t.Errorf("J17NLICalibrationMinSamples = %d, want 50", cfg.J17NLICalibrationMinSamples)
	}
}

func TestJ17NLICalibrationMinSamples_EnvOverride(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_NLI_CALIBRATION_MIN_SAMPLES", "100")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17NLICalibrationMinSamples != 100 {
		t.Errorf("J17NLICalibrationMinSamples = %d, want 100", cfg.J17NLICalibrationMinSamples)
	}
}

func TestJ17NLICalibrationMinSamples_NegativeClampedToZero(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_NLI_CALIBRATION_MIN_SAMPLES", "-5")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17NLICalibrationMinSamples != 0 {
		t.Errorf("J17NLICalibrationMinSamples = %d, want 0 (negative clamps to disabled)", cfg.J17NLICalibrationMinSamples)
	}
}

func TestJ17SidecarTimeoutMs_FloorBoundaryValuesAllowed(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("J17_SIDECAR_TIMEOUT_MS", "100")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.J17SidecarTimeoutMs != 100 {
		t.Errorf("J17SidecarTimeoutMs = %d, want 100 (exactly at floor, no clamp)", cfg.J17SidecarTimeoutMs)
	}
}
