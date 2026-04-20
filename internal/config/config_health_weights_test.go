package config

import (
	"math"
	"testing"
)

// TestRSICHealthWeight_Defaults: unset env vars yield the DH-005 hybrid
// reliability × user-impact priors. Values hard-coded here mirror
// ape.DefaultHealthWeights(); if one side changes the other must too.
func TestRSICHealthWeight_Defaults(t *testing.T) {
	setMinimalEnv(t)
	// Explicitly clear so prior test env doesn't leak in.
	for _, key := range []string{
		"RSIC_HEALTH_WEIGHT_RETRIEVAL", "RSIC_HEALTH_WEIGHT_MEMORY",
		"RSIC_HEALTH_WEIGHT_EDGE", "RSIC_HEALTH_WEIGHT_TASK",
		"RSIC_HEALTH_WEIGHT_GUIDANCE", "RSIC_HEALTH_WEIGHT_PROTOCOL",
		"RSIC_HEALTH_WEIGHT_SYNERGY",
	} {
		t.Setenv(key, "")
	}

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv(): %v", err)
	}

	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"Retrieval", cfg.RSICHealthWeightRetrieval, 0.08},
		{"Memory", cfg.RSICHealthWeightMemory, 0.15},
		{"Edge", cfg.RSICHealthWeightEdge, 0.15},
		{"Task", cfg.RSICHealthWeightTask, 0.20},
		{"Guidance", cfg.RSICHealthWeightGuidance, 0.17},
		{"Protocol", cfg.RSICHealthWeightProtocol, 0.20},
		{"Synergy", cfg.RSICHealthWeightSynergy, 0.05},
	}
	for _, tc := range cases {
		if math.Abs(tc.got-tc.want) > 0.0001 {
			t.Errorf("%s default: got %f, want %f", tc.name, tc.got, tc.want)
		}
	}
}

// TestRSICHealthWeight_EnvOverride: operator-supplied env values flow through
// unchanged (and are not re-normalised at parse time — the formula handles it).
func TestRSICHealthWeight_EnvOverride(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("RSIC_HEALTH_WEIGHT_RETRIEVAL", "0.50")
	t.Setenv("RSIC_HEALTH_WEIGHT_PROTOCOL", "0.33")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv(): %v", err)
	}
	if math.Abs(cfg.RSICHealthWeightRetrieval-0.50) > 1e-9 {
		t.Errorf("override Retrieval: got %f, want 0.50", cfg.RSICHealthWeightRetrieval)
	}
	if math.Abs(cfg.RSICHealthWeightProtocol-0.33) > 1e-9 {
		t.Errorf("override Protocol: got %f, want 0.33", cfg.RSICHealthWeightProtocol)
	}
}

// TestRSICHealthWeight_NegativeIgnored: a negative value falls back to the
// default with a warning log (negatives are nonsensical in a weighted sum).
func TestRSICHealthWeight_NegativeIgnored(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("RSIC_HEALTH_WEIGHT_RETRIEVAL", "-1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv(): %v", err)
	}
	if math.Abs(cfg.RSICHealthWeightRetrieval-0.08) > 1e-9 {
		t.Errorf("negative → default: got %f, want 0.08", cfg.RSICHealthWeightRetrieval)
	}
}

// TestRSICHealthWeight_ZeroAllowed: zero is a valid operator choice meaning
// "disable this dimension". It must NOT fall back to default.
func TestRSICHealthWeight_ZeroAllowed(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("RSIC_HEALTH_WEIGHT_SYNERGY", "0")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv(): %v", err)
	}
	if cfg.RSICHealthWeightSynergy != 0 {
		t.Errorf("zero explicitly set: got %f, want 0 (dimension disabled)", cfg.RSICHealthWeightSynergy)
	}
}

// TestRSICHealthWeight_AllZeroWarns: if every weight is zero, Validate()
// issues a warning (formula would always return 0 — almost certainly a misconfiguration).
func TestRSICHealthWeight_AllZeroWarns(t *testing.T) {
	setMinimalEnv(t)
	for _, key := range []string{
		"RSIC_HEALTH_WEIGHT_RETRIEVAL", "RSIC_HEALTH_WEIGHT_MEMORY",
		"RSIC_HEALTH_WEIGHT_EDGE", "RSIC_HEALTH_WEIGHT_TASK",
		"RSIC_HEALTH_WEIGHT_GUIDANCE", "RSIC_HEALTH_WEIGHT_PROTOCOL",
		"RSIC_HEALTH_WEIGHT_SYNERGY",
	} {
		t.Setenv(key, "0")
	}

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv(): %v", err)
	}
	warnings, _ := cfg.Validate()
	found := false
	for _, w := range warnings {
		if containsAll(w, "RSIC_HEALTH_WEIGHT", "zero") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected all-zero warning; got warnings: %v", warnings)
	}
}

// containsAll is a tiny helper avoiding a strings import ordering churn.
func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for i := 0; i+len(n) <= len(s); i++ {
			if s[i:i+len(n)] == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
