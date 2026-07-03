package config

import "testing"

// JIMINY-CORPUS-001 Epic 3: repetition-control knobs default sanely (shipped
// enabled with conservative thresholds), override via env, and disable cleanly.

func TestJiminySurfaceRepetitionDefaults(t *testing.T) {
	setMinimalEnv(t)
	for _, e := range []string{
		"JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT",
		"JIMINY_SURFACE_COOLDOWN_CAPACITY",
		"JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT",
		"JIMINY_SURFACE_EFFECTIVENESS_PRIOR_TTL_SEC",
		"JIMINY_SURFACE_EFFECTIVENESS_PRIOR_MIN_SAMPLES",
	} {
		t.Setenv(e, "")
	}
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminySurfaceCooldownIgnoredCount != 3 {
		t.Errorf("JiminySurfaceCooldownIgnoredCount = %d, want 3 (ships enabled, conservative)", cfg.JiminySurfaceCooldownIgnoredCount)
	}
	if cfg.JiminySurfaceCooldownCapacity != 5000 {
		t.Errorf("JiminySurfaceCooldownCapacity = %d, want 5000", cfg.JiminySurfaceCooldownCapacity)
	}
	if cfg.JiminySurfaceEffectivenessPriorWeight != 0.3 {
		t.Errorf("JiminySurfaceEffectivenessPriorWeight = %v, want 0.3 (ships enabled, soft)", cfg.JiminySurfaceEffectivenessPriorWeight)
	}
	if cfg.JiminySurfaceEffectivenessPriorTTLSec != 300 {
		t.Errorf("JiminySurfaceEffectivenessPriorTTLSec = %d, want 300", cfg.JiminySurfaceEffectivenessPriorTTLSec)
	}
	if cfg.JiminySurfaceEffectivenessPriorMinSamples != 5 {
		t.Errorf("JiminySurfaceEffectivenessPriorMinSamples = %d, want 5", cfg.JiminySurfaceEffectivenessPriorMinSamples)
	}
}

func TestJiminySurfaceRepetitionOverrides(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT", "7")
	t.Setenv("JIMINY_SURFACE_COOLDOWN_CAPACITY", "123")
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT", "0.6")
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_TTL_SEC", "60")
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_MIN_SAMPLES", "10")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminySurfaceCooldownIgnoredCount != 7 {
		t.Errorf("cooldown count override = %d, want 7", cfg.JiminySurfaceCooldownIgnoredCount)
	}
	if cfg.JiminySurfaceCooldownCapacity != 123 {
		t.Errorf("cooldown capacity override = %d, want 123", cfg.JiminySurfaceCooldownCapacity)
	}
	if cfg.JiminySurfaceEffectivenessPriorWeight != 0.6 {
		t.Errorf("prior weight override = %v, want 0.6", cfg.JiminySurfaceEffectivenessPriorWeight)
	}
	if cfg.JiminySurfaceEffectivenessPriorTTLSec != 60 {
		t.Errorf("prior TTL override = %d, want 60", cfg.JiminySurfaceEffectivenessPriorTTLSec)
	}
	if cfg.JiminySurfaceEffectivenessPriorMinSamples != 10 {
		t.Errorf("prior min samples override = %d, want 10", cfg.JiminySurfaceEffectivenessPriorMinSamples)
	}
}

func TestJiminySurfaceRepetitionDisableAndClamps(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT", "0")     // explicit disable
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT", "0") // explicit disable
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminySurfaceCooldownIgnoredCount != 0 {
		t.Errorf("cooldown 0 must disable, got %d", cfg.JiminySurfaceCooldownIgnoredCount)
	}
	if cfg.JiminySurfaceEffectivenessPriorWeight != 0 {
		t.Errorf("prior weight 0 must disable, got %v", cfg.JiminySurfaceEffectivenessPriorWeight)
	}

	// Out-of-range values clamp to safe semantics.
	t.Setenv("JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT", "-2")         // negative → disabled
	t.Setenv("JIMINY_SURFACE_COOLDOWN_CAPACITY", "-1")              // non-positive → default bound
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT", "1.5")    // >1 → clamp to 1
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_TTL_SEC", "0")     // non-positive → default
	t.Setenv("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_MIN_SAMPLES", "0") // <1 → floor 1
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminySurfaceCooldownIgnoredCount != 0 {
		t.Errorf("negative cooldown count must clamp to 0 (disabled), got %d", cfg.JiminySurfaceCooldownIgnoredCount)
	}
	if cfg.JiminySurfaceCooldownCapacity != 5000 {
		t.Errorf("non-positive capacity must fall back to 5000, got %d", cfg.JiminySurfaceCooldownCapacity)
	}
	if cfg.JiminySurfaceEffectivenessPriorWeight != 1.0 {
		t.Errorf("weight >1 must clamp to 1.0, got %v", cfg.JiminySurfaceEffectivenessPriorWeight)
	}
	if cfg.JiminySurfaceEffectivenessPriorTTLSec != 300 {
		t.Errorf("non-positive TTL must fall back to 300, got %d", cfg.JiminySurfaceEffectivenessPriorTTLSec)
	}
	if cfg.JiminySurfaceEffectivenessPriorMinSamples != 1 {
		t.Errorf("min samples <1 must floor at 1, got %d", cfg.JiminySurfaceEffectivenessPriorMinSamples)
	}
}
