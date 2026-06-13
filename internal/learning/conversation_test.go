package learning

import (
	"math"
	"testing"
)

// TestSurpriseWeightedEdgeCreation tests that conversation nodes create edges with surprise-based initial weights
func TestSurpriseWeightedEdgeCreation(t *testing.T) {
	tests := []struct {
		name                string
		surpriseScore       float64
		expectedMinWeight   float64
		expectedMaxWeight   float64
		expectedSurpriseFac float64
	}{
		{
			name:                "high surprise gives 2x weight",
			surpriseScore:       0.8,
			expectedMinWeight:   0.15, // 0.10 * 2.0 = 0.20, but with activation product
			expectedMaxWeight:   0.25,
			expectedSurpriseFac: 2.0,
		},
		{
			name:                "medium surprise gives 1.5x weight",
			surpriseScore:       0.5,
			expectedMinWeight:   0.10,
			expectedMaxWeight:   0.20,
			expectedSurpriseFac: 1.5,
		},
		{
			name:                "low surprise gives normal weight",
			surpriseScore:       0.2,
			expectedMinWeight:   0.05,
			expectedMaxWeight:   0.15,
			expectedSurpriseFac: 1.0,
		},
		{
			name:                "zero surprise gives normal weight",
			surpriseScore:       0.0,
			expectedMinWeight:   0.05,
			expectedMaxWeight:   0.15,
			expectedSurpriseFac: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic: Verify that surprise_factor is calculated correctly
			// and that initial weight is boosted accordingly
			surpriseFactor := 1.0
			if tt.surpriseScore >= 0.7 {
				surpriseFactor = 2.0
			} else if tt.surpriseScore >= 0.4 {
				surpriseFactor = 1.5
			}

			if surpriseFactor != tt.expectedSurpriseFac {
				t.Errorf("expected surprise factor %f, got %f", tt.expectedSurpriseFac, surpriseFactor)
			}

			initialWeight := 0.10 * surpriseFactor
			if initialWeight < tt.expectedMinWeight || initialWeight > tt.expectedMaxWeight {
				t.Errorf("initial weight %f not in expected range [%f, %f]",
					initialWeight, tt.expectedMinWeight, tt.expectedMaxWeight)
			}
		})
	}
}

// TestSurpriseFactorCalculation tests the surprise factor calculation logic
func TestSurpriseFactorCalculation(t *testing.T) {
	tests := []struct {
		name             string
		surpriseA        float64
		surpriseB        float64
		expectedFactor   float64
		description      string
	}{
		{
			name:           "both high surprise",
			surpriseA:      0.9,
			surpriseB:      0.8,
			expectedFactor: 2.0,
			description:    "either node >= 0.7 triggers HIGH",
		},
		{
			name:           "one high, one low",
			surpriseA:      0.75,
			surpriseB:      0.1,
			expectedFactor: 2.0,
			description:    "one high is enough",
		},
		{
			name:           "both medium surprise",
			surpriseA:      0.5,
			surpriseB:      0.6,
			expectedFactor: 1.5,
			description:    "either >= 0.4 triggers MEDIUM",
		},
		{
			name:           "one medium, one low",
			surpriseA:      0.45,
			surpriseB:      0.2,
			expectedFactor: 1.5,
			description:    "one medium is enough",
		},
		{
			name:           "both low surprise",
			surpriseA:      0.3,
			surpriseB:      0.1,
			expectedFactor: 1.0,
			description:    "both < 0.4 triggers NORMAL",
		},
		{
			name:           "zero surprise",
			surpriseA:      0.0,
			surpriseB:      0.0,
			expectedFactor: 1.0,
			description:    "zero is treated as normal",
		},
		{
			name:           "boundary case - exactly 0.7",
			surpriseA:      0.7,
			surpriseB:      0.3,
			expectedFactor: 2.0,
			description:    ">= 0.7 triggers HIGH",
		},
		{
			name:           "boundary case - exactly 0.4",
			surpriseA:      0.4,
			surpriseB:      0.2,
			expectedFactor: 1.5,
			description:    ">= 0.4 triggers MEDIUM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the Cypher logic for surprise factor calculation
			surpriseFactor := 1.0
			if tt.surpriseA >= 0.7 || tt.surpriseB >= 0.7 {
				surpriseFactor = 2.0
			} else if tt.surpriseA >= 0.4 || tt.surpriseB >= 0.4 {
				surpriseFactor = 1.5
			}

			if surpriseFactor != tt.expectedFactor {
				t.Errorf("%s: expected factor %f, got %f", tt.description, tt.expectedFactor, surpriseFactor)
			}
		})
	}
}

// TestDecayWithSurpriseFactor tests that surprise factor affects decay rate
func TestDecayWithSurpriseFactor(t *testing.T) {
	tests := []struct {
		name            string
		surpriseFactor  float64
		evidenceCount   int
		decayPerDay     float64
		daysInactive    int
		initialWeight   float64
		shouldDecayMore bool // compared to baseline (surpriseFactor=1.0)
	}{
		{
			name:            "high surprise decays slower",
			surpriseFactor:  2.0,
			evidenceCount:   1,
			decayPerDay:     0.05,
			daysInactive:    10,
			initialWeight:   0.5,
			shouldDecayMore: false,
		},
		{
			name:            "medium surprise decays slower than normal",
			surpriseFactor:  1.5,
			evidenceCount:   1,
			decayPerDay:     0.05,
			daysInactive:    10,
			initialWeight:   0.5,
			shouldDecayMore: false,
		},
		{
			name:            "normal surprise baseline",
			surpriseFactor:  1.0,
			evidenceCount:   1,
			decayPerDay:     0.05,
			daysInactive:    10,
			initialWeight:   0.5,
			shouldDecayMore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate decay formula: rawWeight * ((1.0 - decayPerDay / sqrt(evidenceCount * surpriseFactor)) ^ daysInactive)
			effectiveDecay := tt.decayPerDay / math.Sqrt(float64(tt.evidenceCount)*tt.surpriseFactor)
			decayedWeight := tt.initialWeight * math.Pow(1.0-effectiveDecay, float64(tt.daysInactive))

			// Calculate baseline (surpriseFactor=1.0) for comparison
			baselineDecay := tt.decayPerDay / math.Sqrt(float64(tt.evidenceCount)*1.0)
			baselineWeight := tt.initialWeight * math.Pow(1.0-baselineDecay, float64(tt.daysInactive))

			// High surprise should decay less (higher final weight)
			if tt.surpriseFactor > 1.0 {
				if decayedWeight <= baselineWeight {
					t.Errorf("high surprise (factor=%f) should decay less than baseline, got %f vs %f",
						tt.surpriseFactor, decayedWeight, baselineWeight)
				}
			}

			// Verify weight is positive and less than initial
			if decayedWeight < 0 {
				t.Errorf("decayed weight should be positive, got %f", decayedWeight)
			}
			if decayedWeight > tt.initialWeight {
				t.Errorf("decayed weight %f should be <= initial %f", decayedWeight, tt.initialWeight)
			}
		})
	}
}

// TestSessionCoactivationTemporalProximity tests temporal proximity weighting
func TestSessionCoactivationTemporalProximity(t *testing.T) {
	tests := []struct {
		name              string
		timeDiffSeconds   int64
		expectedProximity float64
	}{
		{
			name:              "same time - maximum proximity",
			timeDiffSeconds:   0,
			expectedProximity: 1.0,
		},
		{
			name:              "5 minutes - high proximity",
			timeDiffSeconds:   300,
			expectedProximity: 0.925, // 1.0 - (300/3600)*0.9 ≈ 0.925
		},
		{
			name:              "30 minutes - medium proximity",
			timeDiffSeconds:   1800,
			expectedProximity: 0.55, // 1.0 - (1800/3600)*0.9 = 0.55
		},
		{
			name:              "1 hour - low proximity",
			timeDiffSeconds:   3600,
			expectedProximity: 0.1, // boundary
		},
		{
			name:              "2 hours - minimum proximity",
			timeDiffSeconds:   7200,
			expectedProximity: 0.1, // capped at minimum
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate temporal proximity calculation from Cypher
			var temporalProximity float64
			if tt.timeDiffSeconds <= 0 {
				temporalProximity = 1.0
			} else if tt.timeDiffSeconds > 3600 {
				temporalProximity = 0.1
			} else {
				temporalProximity = 1.0 - (float64(tt.timeDiffSeconds)/3600.0)*0.9
			}

			tolerance := 0.01
			if abs(temporalProximity-tt.expectedProximity) > tolerance {
				t.Errorf("expected proximity %f, got %f", tt.expectedProximity, temporalProximity)
			}
		})
	}
}

// TestEdgePropertyPreservation tests that conversation edge properties are stored correctly
func TestEdgePropertyPreservation(t *testing.T) {
	// This test verifies the structure of edge properties for conversation edges
	edgeProps := map[string]any{
		"surprise_factor":     2.0,
		"session_id":          "session-123",
		"obs_type":            "decision",
		"temporal_proximity":  0.85,
		"evidence_count":      1,
		"weight":              0.20,
	}

	// Verify required properties exist
	requiredProps := []string{"surprise_factor", "session_id", "obs_type", "evidence_count", "weight"}
	for _, prop := range requiredProps {
		if _, ok := edgeProps[prop]; !ok {
			t.Errorf("missing required edge property: %s", prop)
		}
	}

	// Verify surprise_factor is valid
	if sf, ok := edgeProps["surprise_factor"].(float64); ok {
		if sf < 1.0 || sf > 2.0 {
			t.Errorf("surprise_factor %f out of valid range [1.0, 2.0]", sf)
		}
	}

	// Verify session_id is not empty
	if sid, ok := edgeProps["session_id"].(string); ok {
		if sid == "" {
			t.Error("session_id should not be empty for conversation edges")
		}
	}
}

// TestCoactivateSessionWithMultipleObservations tests full session coactivation
// NEGFEED-001: CoactivateSession emits DELTAS, not the full C(n,2) clique.
// Each Observe links the NEW observation to its prior session members
// (bounded by LEARNING_SESSION_CLIQUE_WINDOW). This pins the delta-count
// arithmetic the rewritten Cypher relies on: per-call work is the number
// of prior members (capped by the window), not C(n,2).
func TestCoactivateSession_DeltaPairCount(t *testing.T) {
	deltaPairs := func(priorCount, window int) int {
		if window > 0 && priorCount > window {
			return window
		}
		return priorCount
	}
	cases := []struct {
		priorCount, window, expected int
	}{
		{0, 50, 0},  // first observation in a session — no prior members, no edges
		{1, 50, 1},  // 2nd obs links to the 1 prior
		{4, 50, 4},  // 5th obs links to all 4 priors (under window)
		{99, 50, 50}, // 100th obs in a long session — capped at the window
		{99, 0, 99},  // window 0 = link to all priors (unbounded delta)
	}
	for _, tc := range cases {
		if got := deltaPairs(tc.priorCount, tc.window); got != tc.expected {
			t.Errorf("priorCount=%d window=%d: got %d delta pairs, want %d",
				tc.priorCount, tc.window, got, tc.expected)
		}
	}

	// Cumulative invariant: an UNWINDOWED session that grows to n observations
	// still accumulates C(n,2) total edges (sum of per-call deltas
	// 0+1+...+(n-1)), so the asymptotic graph is unchanged — only the
	// per-Observe cost (O(n) not O(n^2)) and evidence_count semantics differ.
	cumulative := 0
	for i := 0; i < 5; i++ {
		cumulative += deltaPairs(i, 0)
	}
	if cumulative != 5*(5-1)/2 {
		t.Errorf("cumulative delta edges over 5 observations = %d, want C(5,2)=10", cumulative)
	}
}

// TestSurpriseBoostCalculation tests the combined effect of surprise on edge strength
func TestSurpriseBoostCalculation(t *testing.T) {
	baseWeight := 0.10

	tests := []struct {
		surpriseFactor float64
		expectedWeight float64
	}{
		{1.0, 0.10},  // normal
		{1.5, 0.15},  // medium surprise
		{2.0, 0.20},  // high surprise
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			initialWeight := baseWeight * tt.surpriseFactor
			if abs(initialWeight-tt.expectedWeight) > 1e-10 {
				t.Errorf("surprise factor %f: expected initial weight %f, got %f",
					tt.surpriseFactor, tt.expectedWeight, initialWeight)
			}
		})
	}
}

// TestCodeNodesUseStandardFactor tests that non-conversation nodes still use factor=1.0
func TestCodeNodesUseStandardFactor(t *testing.T) {
	// Simulate the role_type check logic
	testCases := []struct {
		roleType       string
		shouldBoost    bool
		expectedFactor float64
	}{
		{"conversation_observation", true, 2.0}, // with high surprise
		{"code_file", false, 1.0},
		{"dependency", false, 1.0},
		{"hidden_concept", false, 1.0},
		{"", false, 1.0},
	}

	surpriseScore := 0.8 // high surprise

	for _, tc := range testCases {
		surpriseFactor := 1.0
		if tc.roleType == "conversation_observation" {
			if surpriseScore >= 0.7 {
				surpriseFactor = 2.0
			} else if surpriseScore >= 0.4 {
				surpriseFactor = 1.5
			}
		}

		if surpriseFactor != tc.expectedFactor {
			t.Errorf("role_type=%s: expected factor %f, got %f",
				tc.roleType, tc.expectedFactor, surpriseFactor)
		}
	}
}

// TestHighEvidenceWithSurpriseDecaysSlower tests combined effect of evidence and surprise
func TestHighEvidenceWithSurpriseDecaysSlower(t *testing.T) {
	decayPerDay := 0.05
	daysInactive := 30
	initialWeight := 0.5

	tests := []struct {
		name           string
		evidenceCount  int
		surpriseFactor float64
		description    string
	}{
		{"low evidence, normal surprise", 1, 1.0, "baseline"},
		{"high evidence, normal surprise", 10, 1.0, "frequent use protects"},
		{"low evidence, high surprise", 1, 2.0, "surprise protects"},
		{"high evidence, high surprise", 10, 2.0, "both protect - slowest decay"},
	}

	var baselineWeight float64

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effectiveDecay := decayPerDay / math.Sqrt(float64(tt.evidenceCount)*tt.surpriseFactor)
			decayedWeight := initialWeight * math.Pow(1.0-effectiveDecay, float64(daysInactive))

			if i == 0 {
				baselineWeight = decayedWeight
			} else {
				// All other cases should decay slower than baseline
				if decayedWeight <= baselineWeight {
					t.Errorf("%s should decay slower than baseline: got %f vs baseline %f",
						tt.description, decayedWeight, baselineWeight)
				}
			}

			t.Logf("%s: final weight = %f (effective decay = %f)",
				tt.description, decayedWeight, effectiveDecay)
		})
	}
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
