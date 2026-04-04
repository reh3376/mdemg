package cli

import (
	"math"
	"testing"
	"time"
)

// TestCalculateDecay verifies the evidence-weighted decay formula:
// w_new = w_old * (1 - rate/sqrt(evidence))^days
func TestCalculateDecay(t *testing.T) {
	tests := []struct {
		name           string
		edge           edge
		decayRate      float64
		expectedWeight float64
		tolerance      float64
	}{
		{
			name: "10 days, evidence=1, rate=0.1",
			edge: edge{
				Weight:        1.0,
				EvidenceCount: 1,
				LastActivated: time.Now().Add(-10 * 24 * time.Hour),
			},
			decayRate:      0.1,
			expectedWeight: math.Pow(1.0-0.1, 10), // (0.9)^10 ≈ 0.3487
			tolerance:      0.01,
		},
		{
			name: "10 days, evidence=4, rate=0.1 — high evidence decays slower",
			edge: edge{
				Weight:        1.0,
				EvidenceCount: 4,
				LastActivated: time.Now().Add(-10 * 24 * time.Hour),
			},
			decayRate:      0.1,
			expectedWeight: math.Pow(1.0-0.1/2.0, 10), // (0.95)^10 ≈ 0.5987
			tolerance:      0.01,
		},
		{
			name: "0 days should not decay",
			edge: edge{
				Weight:        1.0,
				EvidenceCount: 1,
				LastActivated: time.Now(),
			},
			decayRate:      0.1,
			expectedWeight: 1.0,
			tolerance:      0.001,
		},
		{
			name: "zero weight edge hits floor",
			edge: edge{
				Weight:        0.0,
				EvidenceCount: 1,
				LastActivated: time.Now().Add(-10 * 24 * time.Hour),
			},
			decayRate:      0.1,
			expectedWeight: minEdgeWeight,
			tolerance:      0.001,
		},
		{
			name: "decayed weight clamped to floor",
			edge: edge{
				Weight:        0.002,
				EvidenceCount: 1,
				LastActivated: time.Now().Add(-30 * 24 * time.Hour),
			},
			decayRate:      0.1,
			expectedWeight: minEdgeWeight, // would decay well below floor
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
	e := edge{
		Weight:        1.0,
		EvidenceCount: 1,
		LastActivated: time.Now().Add(-10 * 24 * time.Hour),
	}

	_, decayPercent := calculateDecay(e, 0.1, time.Now())

	// New formula: (1 - 0.1/sqrt(1))^10 = (0.9)^10 ≈ 0.3487
	// Decay percent ≈ 65.13%
	expectedPercent := (1 - math.Pow(0.9, 10)) * 100

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
		DecayRate:       0.1,
		PruneThreshold:  0.01,
		MinEvidence:     3,
		MaxDecayPercent: 50.0,
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
				Weight:        0.015,
				EvidenceCount: 1,
				Pinned:        false,
				LastActivated: now.Add(-10 * 24 * time.Hour),
			},
			// MaxDecayPercent=50%: 0.015 * 0.5 = 0.0075 < 0.01
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "high evidence protects even after decay",
			edge: edge{
				Weight:        0.02,
				EvidenceCount: 5,
				Pinned:        false,
				LastActivated: now.Add(-10 * 24 * time.Hour),
			},
			// evidence=5: effectiveRate = 0.1/sqrt(5) ≈ 0.0447
			// 0.02 * (0.955)^10 ≈ 0.0127 > 0.01 (but even if below, protected by evidence)
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "decay stays above threshold",
			edge: edge{
				Weight:        0.5,
				EvidenceCount: 1,
				Pinned:        false,
				LastActivated: now.Add(-7 * 24 * time.Hour),
			},
			// 0.5 * (0.9)^7 ≈ 0.239 > 0.01
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "max-decay-percent caps extreme decay",
			edge: edge{
				Weight:        0.5,
				EvidenceCount: 1,
				Pinned:        false,
				LastActivated: now.Add(-30 * 24 * time.Hour),
			},
			// Uncapped: 0.5 * (0.9)^30 ≈ 0.021 (95.8% decay)
			// Capped at 50%: 0.5 * 0.5 = 0.25 > 0.01
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processEdge(tt.edge, cfg, now)

			if result.ShouldPrune != tt.expectPrune {
				t.Errorf("processEdge() ShouldPrune = %v, want %v (NewWeight=%.6f)",
					result.ShouldPrune, tt.expectPrune, result.NewWeight)
			}
			if result.Protected != tt.expectProtected {
				t.Errorf("processEdge() Protected = %v, want %v", result.Protected, tt.expectProtected)
			}
			if result.ProtectReason != tt.expectReason {
				t.Errorf("processEdge() ProtectReason = %q, want %q", result.ProtectReason, tt.expectReason)
			}

			// Verify decay was applied (weight should decrease, but not below floor)
			if result.OldWeight > 0 && result.NewWeight > result.OldWeight && tt.edge.LastActivated.Before(now) {
				t.Errorf("processEdge() NewWeight %v should be <= OldWeight %v after decay",
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
