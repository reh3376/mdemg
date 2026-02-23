package cli

import (
	"math"
	"testing"
	"time"
)

// TestCalculateDecay verifies the exponential decay formula:
// w_new = w_old * exp(-decay_rate * days_since_activation)
func TestCalculateDecay(t *testing.T) {
	tests := []struct {
		name           string
		edge           edge
		decayRate      float64
		daysSince      float64
		expectedWeight float64
		tolerance      float64
	}{
		{
			name: "10 days with 0.1 decay rate",
			edge: edge{
				Weight:        1.0,
				LastActivated: time.Now().Add(-10 * 24 * time.Hour),
			},
			decayRate:      0.1,
			daysSince:      10.0,
			expectedWeight: math.Exp(-0.1 * 10), // ~0.3679
			tolerance:      0.01,
		},
		{
			name: "7 days with 0.1 decay rate",
			edge: edge{
				Weight:        1.0,
				LastActivated: time.Now().Add(-7 * 24 * time.Hour),
			},
			decayRate:      0.1,
			daysSince:      7.0,
			expectedWeight: math.Exp(-0.1 * 7), // ~0.4966
			tolerance:      0.01,
		},
		{
			name: "0 days should not decay",
			edge: edge{
				Weight:        1.0,
				LastActivated: time.Now(),
			},
			decayRate:      0.1,
			daysSince:      0.0,
			expectedWeight: 1.0,
			tolerance:      0.001,
		},
		{
			name: "high decay rate",
			edge: edge{
				Weight:        0.5,
				LastActivated: time.Now().Add(-5 * 24 * time.Hour),
			},
			decayRate:      0.2,
			daysSince:      5.0,
			expectedWeight: 0.5 * math.Exp(-0.2*5), // ~0.1839
			tolerance:      0.01,
		},
		{
			name: "zero weight edge",
			edge: edge{
				Weight:        0.0,
				LastActivated: time.Now().Add(-10 * 24 * time.Hour),
			},
			decayRate:      0.1,
			daysSince:      10.0,
			expectedWeight: 0.0,
			tolerance:      0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newWeight, _ := calculateDecay(tt.edge, tt.decayRate, time.Now())

			if math.Abs(newWeight-tt.expectedWeight) > tt.tolerance {
				t.Errorf("calculateDecay() = %v, want %v (tolerance %v)",
					newWeight, tt.expectedWeight, tt.tolerance)
			}
		})
	}
}

// TestCalculateDecayPercent verifies the decay percentage calculation
func TestCalculateDecayPercent(t *testing.T) {
	edge := edge{
		Weight:        1.0,
		LastActivated: time.Now().Add(-10 * 24 * time.Hour),
	}

	_, decayPercent := calculateDecay(edge, 0.1, time.Now())

	// After 10 days with 0.1 decay rate: exp(-1) ≈ 0.3679
	// Decay percent should be ~63.21%
	expectedPercent := (1 - math.Exp(-1)) * 100 // ~63.21

	if math.Abs(decayPercent-expectedPercent) > 1.0 {
		t.Errorf("decayPercent = %.2f%%, want ~%.2f%%", decayPercent, expectedPercent)
	}
}

// TestDaysSinceActivation verifies days calculation from timestamp
func TestDaysSinceActivation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		lastActivated time.Time
		expectedDays  float64
		tolerance     float64
	}{
		{
			name:          "10 days ago",
			lastActivated: now.Add(-10 * 24 * time.Hour),
			expectedDays:  10.0,
			tolerance:     0.01,
		},
		{
			name:          "1 day ago",
			lastActivated: now.Add(-24 * time.Hour),
			expectedDays:  1.0,
			tolerance:     0.01,
		},
		{
			name:          "12 hours ago",
			lastActivated: now.Add(-12 * time.Hour),
			expectedDays:  0.5,
			tolerance:     0.01,
		},
		{
			name:          "zero time returns 0",
			lastActivated: time.Time{},
			expectedDays:  0.0,
			tolerance:     0.001,
		},
		{
			name:          "future time returns 0",
			lastActivated: now.Add(24 * time.Hour),
			expectedDays:  0.0,
			tolerance:     0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysSinceActivation(tt.lastActivated, now)

			if math.Abs(days-tt.expectedDays) > tt.tolerance {
				t.Errorf("daysSinceActivation() = %v, want %v (tolerance %v)",
					days, tt.expectedDays, tt.tolerance)
			}
		})
	}
}

// TestShouldPrune verifies pruning decision logic with protection rules
func TestShouldPrune(t *testing.T) {
	tests := []struct {
		name            string
		weight          float64
		evidenceCount   int
		pinned          bool
		pruneThreshold  float64
		minEvidence     int
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		{
			name:            "low weight, low evidence -> prune",
			weight:          0.005,
			evidenceCount:   2,
			pinned:          false,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name:            "low weight, high evidence -> protected",
			weight:          0.005,
			evidenceCount:   5,
			pinned:          false,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name:            "low weight, low evidence, pinned -> protected",
			weight:          0.005,
			evidenceCount:   1,
			pinned:          true,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},
		{
			name:            "high weight -> not pruned, not protected",
			weight:          0.5,
			evidenceCount:   1,
			pinned:          false,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name:            "zero weight -> always prune",
			weight:          0.0,
			evidenceCount:   10, // high evidence should not protect zero weight
			pinned:          true, // even pinned edges with zero weight get pruned
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name:            "negative weight -> always prune",
			weight:          -0.1,
			evidenceCount:   10,
			pinned:          true,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name:            "weight exactly at threshold -> not pruned",
			weight:          0.01,
			evidenceCount:   0,
			pinned:          false,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name:            "weight just below threshold, evidence exactly at min -> protected",
			weight:          0.009,
			evidenceCount:   3,
			pinned:          false,
			pruneThreshold:  0.01,
			minEvidence:     3,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prune, protected, reason := shouldPrune(
				tt.weight, tt.evidenceCount, tt.pinned,
				tt.pruneThreshold, tt.minEvidence,
			)

			if prune != tt.expectPrune {
				t.Errorf("shouldPrune() prune = %v, want %v", prune, tt.expectPrune)
			}
			if protected != tt.expectProtected {
				t.Errorf("shouldPrune() protected = %v, want %v", protected, tt.expectProtected)
			}
			if reason != tt.expectReason {
				t.Errorf("shouldPrune() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

// TestProcessEdge verifies the combined decay and pruning logic
func TestProcessEdge(t *testing.T) {
	now := time.Now()
	cfg := decayConfig{
		DecayRate:      0.1,
		PruneThreshold: 0.01,
		MinEvidence:    3,
	}

	tests := []struct {
		name            string
		edge            edge
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		{
			name: "decay to below threshold, low evidence -> prune",
			edge: edge{
				Weight:        0.02,
				EvidenceCount: 2,
				Pinned:        false,
				LastActivated: now.Add(-10 * 24 * time.Hour),
			},
			// After decay: 0.02 * exp(-1) ≈ 0.00736 < 0.01
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "decay to below threshold, high evidence -> protected",
			edge: edge{
				Weight:        0.02,
				EvidenceCount: 5,
				Pinned:        false,
				LastActivated: now.Add(-10 * 24 * time.Hour),
			},
			// After decay: 0.02 * exp(-1) ≈ 0.00736 < 0.01
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "decay stays above threshold",
			edge: edge{
				Weight:        0.5,
				EvidenceCount: 0,
				Pinned:        false,
				LastActivated: now.Add(-7 * 24 * time.Hour),
			},
			// After decay: 0.5 * exp(-0.7) ≈ 0.248 > 0.01
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processEdge(tt.edge, cfg, now)

			if result.ShouldPrune != tt.expectPrune {
				t.Errorf("processEdge() ShouldPrune = %v, want %v", result.ShouldPrune, tt.expectPrune)
			}
			if result.Protected != tt.expectProtected {
				t.Errorf("processEdge() Protected = %v, want %v", result.Protected, tt.expectProtected)
			}
			if result.ProtectReason != tt.expectReason {
				t.Errorf("processEdge() ProtectReason = %q, want %q", result.ProtectReason, tt.expectReason)
			}

			// Verify decay was applied
			if result.NewWeight >= result.OldWeight && tt.edge.LastActivated.Before(now) {
				t.Errorf("processEdge() NewWeight %v should be less than OldWeight %v after decay",
					result.NewWeight, result.OldWeight)
			}
		})
	}
}

// TestExpDecayFormula directly tests the mathematical accuracy of exp(-0.1 * 10)
func TestExpDecayFormula(t *testing.T) {
	// Verify exp(-0.1 * 10) ≈ 0.3679
	decayFactor := math.Exp(-0.1 * 10)
	expected := 0.36787944117144233 // e^(-1)

	if math.Abs(decayFactor-expected) > 1e-10 {
		t.Errorf("exp(-0.1 * 10) = %v, want %v", decayFactor, expected)
	}
}
