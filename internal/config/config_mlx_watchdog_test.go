package config

import "testing"

// TestMLXWatchdog_Defaults confirms the safe-off rollout posture: watchdog
// disabled, fail-fast on, 5s/2s probe cadence.
func TestMLXWatchdog_Defaults(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_WATCHDOG_ENABLED", "")
	t.Setenv("MLX_PROBE_INTERVAL_SEC", "")
	t.Setenv("MLX_PROBE_TIMEOUT_SEC", "")
	t.Setenv("MLX_FAIL_FAST_ENABLED", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.MLXWatchdogEnabled {
		t.Error("MLXWatchdogEnabled default = true, want false (safe-off rollout)")
	}
	if cfg.MLXProbeIntervalSec != 5 {
		t.Errorf("MLXProbeIntervalSec = %d, want 5", cfg.MLXProbeIntervalSec)
	}
	if cfg.MLXProbeTimeoutSec != 2 {
		t.Errorf("MLXProbeTimeoutSec = %d, want 2", cfg.MLXProbeTimeoutSec)
	}
	if !cfg.MLXFailFastEnabled {
		t.Error("MLXFailFastEnabled default = false, want true")
	}
}

// TestMLXWatchdog_EnableViaEnv verifies an operator can opt in.
func TestMLXWatchdog_EnableViaEnv(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_WATCHDOG_ENABLED", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if !cfg.MLXWatchdogEnabled {
		t.Error("MLXWatchdogEnabled stayed false after env override to true")
	}
}

// TestMLXProbe_RejectsZeroOrNegativeInterval verifies the lower bound is
// enforced.
func TestMLXProbe_RejectsZeroOrNegativeInterval(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_PROBE_INTERVAL_SEC", "0")

	if _, err := FromEnv(); err == nil {
		t.Error("expected error for MLX_PROBE_INTERVAL_SEC=0")
	}
}

// TestMLXProbe_RejectsTimeoutGreaterThanInterval guards against ticker stalls
// when each tick can outlive the window.
func TestMLXProbe_RejectsTimeoutGreaterThanInterval(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_PROBE_INTERVAL_SEC", "2")
	t.Setenv("MLX_PROBE_TIMEOUT_SEC", "5")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when MLX_PROBE_TIMEOUT_SEC >= MLX_PROBE_INTERVAL_SEC")
	}
}

// TestMLXProbe_RejectsTimeoutEqualToInterval — strict less-than is required.
func TestMLXProbe_RejectsTimeoutEqualToInterval(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_PROBE_INTERVAL_SEC", "3")
	t.Setenv("MLX_PROBE_TIMEOUT_SEC", "3")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when timeout equals interval")
	}
}

// TestMLXFailFast_DisableViaEnv verifies the operator escape hatch.
func TestMLXFailFast_DisableViaEnv(t *testing.T) {
	setMinimalEnv(t)
	clearLLMEnv(t)
	t.Setenv("MLX_FAIL_FAST_ENABLED", "false")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error: %v", err)
	}
	if cfg.MLXFailFastEnabled {
		t.Error("MLXFailFastEnabled = true after env override to false")
	}
}
