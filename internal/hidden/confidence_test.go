package hidden

import (
	"math"
	"testing"
)

// HEBB-ETA-001 — property tests for ComputeActivationConfidence.

func TestComputeConfidence_ClampBounds(t *testing.T) {
	cfg := DefaultConfidenceConfig()
	// Extreme negative input → floor 0.05
	c := ComputeActivationConfidence(0, 1e12, nil, cfg)
	if c < 0.05 {
		t.Errorf("floor violated: got %f, want >= 0.05", c)
	}
	// Extreme positive → still <= 1.0
	c = ComputeActivationConfidence(math.MaxInt32, 0, nil, cfg)
	if c > 1.0 {
		t.Errorf("ceiling violated: got %f, want <= 1.0", c)
	}
}

func TestComputeConfidence_MonotonicInReinforceCount(t *testing.T) {
	cfg := DefaultConfidenceConfig()
	prev := 0.0
	for _, n := range []int64{0, 1, 10, 100, 1000, 10000} {
		c := ComputeActivationConfidence(n, 0, nil, cfg)
		if c < prev {
			t.Errorf("confidence must be monotonic in reinforceCount: n=%d c=%f prev=%f", n, c, prev)
		}
		prev = c
	}
}

func TestComputeConfidence_DecaysWithAge(t *testing.T) {
	cfg := DefaultConfidenceConfig() // HalfLifeSec=604800
	recent := ComputeActivationConfidence(10, 0, nil, cfg)
	oneHL := ComputeActivationConfidence(10, cfg.HalfLifeSec, nil, cfg)
	twoHL := ComputeActivationConfidence(10, cfg.HalfLifeSec*2, nil, cfg)
	if !(recent > oneHL && oneHL > twoHL) {
		t.Errorf("confidence must decay with age: recent=%f 1hl=%f 2hl=%f", recent, oneHL, twoHL)
	}
}

func TestComputeConfidence_PenalizedByVariance(t *testing.T) {
	cfg := DefaultConfidenceConfig()
	stable := ComputeActivationConfidence(10, 0, []float64{0.5, 0.5, 0.5, 0.5}, cfg)
	unstable := ComputeActivationConfidence(10, 0, []float64{0.0, 1.0, 0.0, 1.0}, cfg)
	if unstable >= stable {
		t.Errorf("unstable surprise history must reduce confidence: stable=%f unstable=%f", stable, unstable)
	}
}

func TestComputeConfidence_EmptyHistoryNoPenalty(t *testing.T) {
	cfg := DefaultConfidenceConfig()
	// nil and single-sample histories must produce identical outputs — variance is 0
	c0 := ComputeActivationConfidence(10, 0, nil, cfg)
	c1 := ComputeActivationConfidence(10, 0, []float64{0.5}, cfg)
	if math.Abs(c0-c1) > 1e-9 {
		t.Errorf("nil vs single-sample history must be identical: nil=%f one=%f", c0, c1)
	}
}

func TestComputeConfidence_ZeroHalfLifeDisablesRecency(t *testing.T) {
	// With HalfLifeSec=0 the recency term is skipped entirely — the function is
	// still safe (no divide-by-zero) and confidence depends on reinforce+variance only.
	cfg := ConfidenceConfig{Alpha: 1.0, Beta: 0.5, Gamma: 0.3, HalfLifeSec: 0}
	c := ComputeActivationConfidence(10, 100, nil, cfg)
	if math.IsNaN(c) || math.IsInf(c, 0) {
		t.Errorf("HalfLifeSec=0 must not produce NaN/Inf: got %f", c)
	}
	if c < 0.05 || c > 1.0 {
		t.Errorf("HalfLifeSec=0 must still clamp: got %f", c)
	}
}

func TestDefaultConfidenceConfig_Values(t *testing.T) {
	// Pin the shipped defaults — future config bumps should be explicit.
	d := DefaultConfidenceConfig()
	if d.Alpha != 1.0 || d.Beta != 0.5 || d.Gamma != 0.3 || d.HalfLifeSec != 604800 {
		t.Errorf("HEBB-ETA-001 shipped defaults changed: got %+v", d)
	}
}
