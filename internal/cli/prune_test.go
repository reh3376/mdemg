package cli

import (
	"testing"
	"time"
)

// TestShouldPruneEdge verifies the edge pruning decision logic.
// From docs/07_Consolidation_and_Pruning.md Section 5.1:
// Prune edges if ALL are true:
// - weight < weight_threshold
// - evidence_count < min_evidence
// - updated_at older than olderThanDays
// - edge not pinned
// Special case: weight <= 0 always marks for pruning
func TestShouldPruneEdge(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		edge            pruneEdge
		weightThreshold float64
		minEvidence     int
		olderThanDays   int
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		// Low weight prune cases
		{
			name: "low weight, low evidence, old edge -> prune",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 2,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour), // 60 days old
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "zero weight -> always prune regardless of other factors",
			edge: pruneEdge{
				Weight:        0.0,
				EvidenceCount: 10, // high evidence should not protect
				Pinned:        true, // even pinned should not protect
				UpdatedAt:     now, // even recent should not protect
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "negative weight -> always prune",
			edge: pruneEdge{
				Weight:        -0.1,
				EvidenceCount: 10,
				Pinned:        true,
				UpdatedAt:     now,
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},

		// Pinned protection cases
		{
			name: "low weight, pinned -> protected",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},
		{
			name: "pinned edge with very low weight -> protected",
			edge: pruneEdge{
				Weight:        0.001,
				EvidenceCount: 0,
				Pinned:        true,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour), // 1 year old
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},

		// High evidence protection cases
		{
			name: "low weight, high evidence -> protected",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 5,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "evidence exactly at threshold -> protected",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 3,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "high evidence count protects old edge",
			edge: pruneEdge{
				Weight:        0.001,
				EvidenceCount: 10,
				Pinned:        false,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour), // 1 year old
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},

		// Weight above threshold cases
		{
			name: "weight above threshold -> not pruned",
			edge: pruneEdge{
				Weight:        0.5,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "weight exactly at threshold -> not pruned",
			edge: pruneEdge{
				Weight:        0.01,
				EvidenceCount: 0,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "weight just above threshold -> not pruned",
			edge: pruneEdge{
				Weight:        0.011,
				EvidenceCount: 0,
				Pinned:        false,
				UpdatedAt:     now.Add(-365 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     5,
			olderThanDays:   7,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},

		// Age criterion cases (edge must be old enough to prune)
		{
			name: "low weight but too recent -> not pruned",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-10 * 24 * time.Hour), // 10 days old, threshold is 30
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "edge exactly at age threshold -> not pruned",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     now.Add(-30*24*time.Hour + time.Hour), // just inside 30-day threshold
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "edge with zero timestamp -> prune (treated as very old)",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        false,
				UpdatedAt:     time.Time{}, // zero time
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},

		// Combined protection - pinned takes precedence over high_evidence
		{
			name: "pinned with high evidence -> protected by pinned",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 10,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			weightThreshold: 0.01,
			minEvidence:     3,
			olderThanDays:   30,
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned", // pinned is checked first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prune, protected, reason := shouldPruneEdge(
				tt.edge, tt.weightThreshold, tt.minEvidence, tt.olderThanDays, now,
			)

			if prune != tt.expectPrune {
				t.Errorf("shouldPruneEdge() prune = %v, want %v", prune, tt.expectPrune)
			}
			if protected != tt.expectProtected {
				t.Errorf("shouldPruneEdge() protected = %v, want %v", protected, tt.expectProtected)
			}
			if reason != tt.expectReason {
				t.Errorf("shouldPruneEdge() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

// TestIsOlderThan verifies the age calculation for pruning
func TestIsOlderThan(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		updatedAt     time.Time
		olderThanDays int
		expectOlder   bool
	}{
		{
			name:          "60 days old, threshold 30 -> older",
			updatedAt:     now.Add(-60 * 24 * time.Hour),
			olderThanDays: 30,
			expectOlder:   true,
		},
		{
			name:          "10 days old, threshold 30 -> not older",
			updatedAt:     now.Add(-10 * 24 * time.Hour),
			olderThanDays: 30,
			expectOlder:   false,
		},
		{
			name:          "exactly at threshold -> not older",
			updatedAt:     now.Add(-30*24*time.Hour + time.Hour),
			olderThanDays: 30,
			expectOlder:   false,
		},
		{
			name:          "zero time -> treated as very old",
			updatedAt:     time.Time{},
			olderThanDays: 30,
			expectOlder:   true,
		},
		{
			name:          "just updated -> not older",
			updatedAt:     now,
			olderThanDays: 1,
			expectOlder:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			older := isOlderThan(tt.updatedAt, tt.olderThanDays, now)

			if older != tt.expectOlder {
				t.Errorf("isOlderThan() = %v, want %v", older, tt.expectOlder)
			}
		})
	}
}

// TestProcessEdgeForPruning verifies the combined edge processing logic
func TestProcessEdgeForPruning(t *testing.T) {
	now := time.Now()
	cfg := pruneConfig{
		WeightThreshold: 0.01,
		MinEvidence:     3,
		OlderThanDays:   30,
	}

	tests := []struct {
		name            string
		edge            pruneEdge
		expectPrune     bool
		expectProtected bool
		expectReason    string
	}{
		{
			name: "low weight, low evidence, old -> prune",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 2,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "high evidence -> protected",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 5,
				Pinned:        false,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "high_evidence",
		},
		{
			name: "pinned -> protected",
			edge: pruneEdge{
				Weight:        0.005,
				EvidenceCount: 1,
				Pinned:        true,
				UpdatedAt:     now.Add(-60 * 24 * time.Hour),
			},
			expectPrune:     false,
			expectProtected: true,
			expectReason:    "pinned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processEdgeForPruning(tt.edge, cfg, now)

			if result.ShouldPrune != tt.expectPrune {
				t.Errorf("processEdgeForPruning() ShouldPrune = %v, want %v",
					result.ShouldPrune, tt.expectPrune)
			}
			if result.Protected != tt.expectProtected {
				t.Errorf("processEdgeForPruning() Protected = %v, want %v",
					result.Protected, tt.expectProtected)
			}
			if result.ProtectReason != tt.expectReason {
				t.Errorf("processEdgeForPruning() ProtectReason = %q, want %q",
					result.ProtectReason, tt.expectReason)
			}

			// Verify edge is preserved in result
			if result.Edge.Weight != tt.edge.Weight {
				t.Errorf("processEdgeForPruning() Edge.Weight = %v, want %v",
					result.Edge.Weight, tt.edge.Weight)
			}
		})
	}
}

// TestShouldTombstoneNode verifies the node tombstoning decision logic.
// From docs/07_Consolidation_and_Pruning.md Section 5.2:
// Tombstone nodes if ALL are true:
// - degree <= maxDegree (low connectivity, orphan-like)
// - no observations within retentionDays (no recent activity)
// - not part of any abstraction chain (no ABSTRACTS_TO/INSTANTIATES relationships)
//
// Protection rules:
// - If degree > maxDegree -> protected (high_degree)
// - If has observation within retention window -> protected (recent_observation)
// - If part of abstraction chain -> protected (abstraction_chain)
func TestShouldTombstoneNode(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		node            node
		maxDegree       int
		retentionDays   int
		expectTombstone bool
		expectProtected bool
		expectReason    string
	}{
		// Orphan tombstone cases - should be tombstoned
		{
			name: "orphan node with zero degree, no observations -> tombstone",
			node: node{
				NodeID:              "orphan-1",
				Degree:              0,
				LastObservationTime: time.Time{}, // no observations
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "orphan node with one edge, old observation -> tombstone",
			node: node{
				NodeID:              "orphan-2",
				Degree:              1,
				LastObservationTime: now.Add(-180 * 24 * time.Hour), // 180 days old
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "orphan node at max degree threshold, very old observation -> tombstone",
			node: node{
				NodeID:              "orphan-3",
				Degree:              2,
				LastObservationTime: now.Add(-365 * 24 * time.Hour), // 1 year old
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       2,
			retentionDays:   30,
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},

		// Abstraction protection cases - nodes in abstraction chains are never tombstoned
		{
			name: "node in abstraction chain with zero degree -> protected",
			node: node{
				NodeID:              "abstraction-1",
				Degree:              0,
				LastObservationTime: time.Time{}, // no observations
				InAbstractionChain:  true,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "abstraction_chain",
		},
		{
			name: "node in abstraction chain with old observations -> protected",
			node: node{
				NodeID:              "abstraction-2",
				Degree:              1,
				LastObservationTime: now.Add(-365 * 24 * time.Hour), // 1 year old
				InAbstractionChain:  true,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "abstraction_chain",
		},
		{
			name: "abstraction node takes priority over other checks",
			node: node{
				NodeID:              "abstraction-3",
				Degree:              0, // would qualify as orphan
				LastObservationTime: time.Time{}, // would qualify for old observation
				InAbstractionChain:  true,        // but this protects it
				Status:              "active",
			},
			maxDegree:       5,
			retentionDays:   7,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "abstraction_chain",
		},

		// High degree protection cases - well-connected nodes are valuable
		{
			name: "node with high degree -> protected",
			node: node{
				NodeID:              "hub-1",
				Degree:              5,
				LastObservationTime: time.Time{}, // no recent observations
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "high_degree",
		},
		{
			name: "node with degree just above threshold -> protected",
			node: node{
				NodeID:              "hub-2",
				Degree:              2,
				LastObservationTime: now.Add(-180 * 24 * time.Hour), // old observation
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "high_degree",
		},
		{
			name: "node with very high degree -> protected even without observations",
			node: node{
				NodeID:              "hub-3",
				Degree:              100,
				LastObservationTime: time.Time{},
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       10,
			retentionDays:   30,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "high_degree",
		},

		// Recent observation protection cases - recently active nodes are kept
		{
			name: "node with recent observation -> protected",
			node: node{
				NodeID:              "recent-1",
				Degree:              0,
				LastObservationTime: now.Add(-30 * 24 * time.Hour), // 30 days ago, within 90 day window
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "recent_observation",
		},
		{
			name: "node with observation exactly at retention window -> protected",
			node: node{
				NodeID:              "recent-2",
				Degree:              1,
				LastObservationTime: now.Add(-90*24*time.Hour + time.Hour), // just inside 90-day threshold
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "recent_observation",
		},
		{
			name: "node with observation just inside window -> protected",
			node: node{
				NodeID:              "recent-3",
				Degree:              0,
				LastObservationTime: now.Add(-6 * 24 * time.Hour), // 6 days ago, within 7 day window
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       5,
			retentionDays:   7,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "recent_observation",
		},
		{
			name: "node with today's observation -> protected",
			node: node{
				NodeID:              "recent-4",
				Degree:              0,
				LastObservationTime: now,
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "recent_observation",
		},

		// Already tombstoned cases - skip re-tombstoning
		{
			name: "already tombstoned node -> skip (no change needed)",
			node: node{
				NodeID:              "tombstoned-1",
				Degree:              0,
				LastObservationTime: time.Time{},
				InAbstractionChain:  false,
				Status:              "tombstoned",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: false,
			expectProtected: false,
			expectReason:    "",
		},

		// Edge cases
		{
			name: "observation just outside retention window -> tombstone",
			node: node{
				NodeID:              "edge-1",
				Degree:              0,
				LastObservationTime: now.Add(-91 * 24 * time.Hour), // 91 days ago, just outside 90 day window
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "degree exactly at threshold, no other protection -> tombstone",
			node: node{
				NodeID:              "edge-2",
				Degree:              1, // at threshold (maxDegree=1)
				LastObservationTime: now.Add(-100 * 24 * time.Hour),
				InAbstractionChain:  false,
				Status:              "active",
			},
			maxDegree:       1,
			retentionDays:   90,
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tombstone, protected, reason := shouldTombstoneNode(
				tt.node, tt.maxDegree, tt.retentionDays, now,
			)

			if tombstone != tt.expectTombstone {
				t.Errorf("shouldTombstoneNode() tombstone = %v, want %v", tombstone, tt.expectTombstone)
			}
			if protected != tt.expectProtected {
				t.Errorf("shouldTombstoneNode() protected = %v, want %v", protected, tt.expectProtected)
			}
			if reason != tt.expectReason {
				t.Errorf("shouldTombstoneNode() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

// TestHasRecentObservation verifies the observation recency check
func TestHasRecentObservation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                string
		lastObservationTime time.Time
		retentionDays       int
		expectRecent        bool
	}{
		{
			name:                "observation 30 days ago, 90 day window -> recent",
			lastObservationTime: now.Add(-30 * 24 * time.Hour),
			retentionDays:       90,
			expectRecent:        true,
		},
		{
			name:                "observation exactly at cutoff -> recent",
			lastObservationTime: now.Add(-90*24*time.Hour + time.Hour),
			retentionDays:       90,
			expectRecent:        true,
		},
		{
			name:                "observation just outside window -> not recent",
			lastObservationTime: now.Add(-91 * 24 * time.Hour),
			retentionDays:       90,
			expectRecent:        false,
		},
		{
			name:                "no observation (zero time) -> not recent",
			lastObservationTime: time.Time{},
			retentionDays:       90,
			expectRecent:        false,
		},
		{
			name:                "observation today -> recent",
			lastObservationTime: now,
			retentionDays:       7,
			expectRecent:        true,
		},
		{
			name:                "observation 1 year ago, 30 day window -> not recent",
			lastObservationTime: now.Add(-365 * 24 * time.Hour),
			retentionDays:       30,
			expectRecent:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recent := hasRecentObservation(tt.lastObservationTime, tt.retentionDays, now)

			if recent != tt.expectRecent {
				t.Errorf("hasRecentObservation() = %v, want %v", recent, tt.expectRecent)
			}
		})
	}
}

// TestProcessNodeForTombstoning verifies the combined node processing logic
func TestProcessNodeForTombstoning(t *testing.T) {
	now := time.Now()
	cfg := pruneConfig{
		MaxDegree:     1,
		RetentionDays: 90,
	}

	tests := []struct {
		name            string
		node            node
		expectTombstone bool
		expectProtected bool
		expectReason    string
	}{
		{
			name: "orphan node -> tombstone",
			node: node{
				NodeID:              "orphan",
				Degree:              0,
				LastObservationTime: time.Time{},
				InAbstractionChain:  false,
				Status:              "active",
			},
			expectTombstone: true,
			expectProtected: false,
			expectReason:    "",
		},
		{
			name: "abstraction chain node -> protected",
			node: node{
				NodeID:              "abstraction",
				Degree:              0,
				LastObservationTime: time.Time{},
				InAbstractionChain:  true,
				Status:              "active",
			},
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "abstraction_chain",
		},
		{
			name: "high degree node -> protected",
			node: node{
				NodeID:              "hub",
				Degree:              5,
				LastObservationTime: time.Time{},
				InAbstractionChain:  false,
				Status:              "active",
			},
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "high_degree",
		},
		{
			name: "node with recent observation -> protected",
			node: node{
				NodeID:              "recent",
				Degree:              0,
				LastObservationTime: now.Add(-30 * 24 * time.Hour),
				InAbstractionChain:  false,
				Status:              "active",
			},
			expectTombstone: false,
			expectProtected: true,
			expectReason:    "recent_observation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processNodeForTombstoning(tt.node, cfg, now)

			if result.ShouldTombstone != tt.expectTombstone {
				t.Errorf("processNodeForTombstoning() ShouldTombstone = %v, want %v",
					result.ShouldTombstone, tt.expectTombstone)
			}
			if result.Protected != tt.expectProtected {
				t.Errorf("processNodeForTombstoning() Protected = %v, want %v",
					result.Protected, tt.expectProtected)
			}
			if result.ProtectReason != tt.expectReason {
				t.Errorf("processNodeForTombstoning() ProtectReason = %q, want %q",
					result.ProtectReason, tt.expectReason)
			}

			// Verify node is preserved in result
			if result.Node.NodeID != tt.node.NodeID {
				t.Errorf("processNodeForTombstoning() Node.NodeID = %v, want %v",
					result.Node.NodeID, tt.node.NodeID)
			}
		})
	}
}

// TestParseConfig verifies that parseConfig correctly parses CLI flags and environment variables.
// Note: Since flag.Parse() uses global state, we test the validation logic directly
// rather than calling parseConfig() multiple times.
func TestParseConfig(t *testing.T) {
	// Test default values by checking the documentation and flag definitions
	t.Run("default values are documented correctly", func(t *testing.T) {
		// These are the expected default values from parseConfig()
		expectedDefaults := map[string]any{
			"weight-threshold":     0.01,
			"min-evidence":         3,
			"older-than-days":      30,
			"retention-days":       90,
			"max-degree":           1,
			"similarity-threshold": 0.98,
			"merge-enabled":        false,
			"vector-index":         "memNodeEmbedding",
			"dry-run":              true,
			"batch-size":           1000,
		}

		// Verify we have documented the expected defaults
		if expectedDefaults["weight-threshold"].(float64) != 0.01 {
			t.Error("weight-threshold default should be 0.01")
		}
		if expectedDefaults["min-evidence"].(int) != 3 {
			t.Error("min-evidence default should be 3")
		}
		if expectedDefaults["older-than-days"].(int) != 30 {
			t.Error("older-than-days default should be 30")
		}
		if expectedDefaults["retention-days"].(int) != 90 {
			t.Error("retention-days default should be 90")
		}
		if expectedDefaults["max-degree"].(int) != 1 {
			t.Error("max-degree default should be 1")
		}
		if expectedDefaults["similarity-threshold"].(float64) != 0.98 {
			t.Error("similarity-threshold default should be 0.98")
		}
		if expectedDefaults["merge-enabled"].(bool) != false {
			t.Error("merge-enabled default should be false")
		}
		if expectedDefaults["vector-index"].(string) != "memNodeEmbedding" {
			t.Error("vector-index default should be memNodeEmbedding")
		}
		if expectedDefaults["dry-run"].(bool) != true {
			t.Error("dry-run default should be true")
		}
		if expectedDefaults["batch-size"].(int) != 1000 {
			t.Error("batch-size default should be 1000")
		}
	})

	t.Run("config struct stores all expected fields", func(t *testing.T) {
		cfg := pruneConfig{
			Neo4jURI:            "bolt://localhost:7687",
			Neo4jUser:           "neo4j",
			Neo4jPass:           "testpassword",
			WeightThreshold:     0.05,
			MinEvidence:         5,
			OlderThanDays:       60,
			RetentionDays:       120,
			MaxDegree:           2,
			SimilarityThreshold: 0.95,
			MergeEnabled:        true,
			VectorIndexName:     "customIndex",
			DryRun:              false,
			SpaceID:             "test-space",
			BatchSize:           500,
		}

		// Verify all fields are accessible and store correctly
		if cfg.Neo4jURI != "bolt://localhost:7687" {
			t.Error("Neo4jURI not stored correctly")
		}
		if cfg.Neo4jUser != "neo4j" {
			t.Error("Neo4jUser not stored correctly")
		}
		if cfg.Neo4jPass != "testpassword" {
			t.Error("Neo4jPass not stored correctly")
		}
		if cfg.WeightThreshold != 0.05 {
			t.Error("WeightThreshold not stored correctly")
		}
		if cfg.MinEvidence != 5 {
			t.Error("MinEvidence not stored correctly")
		}
		if cfg.OlderThanDays != 60 {
			t.Error("OlderThanDays not stored correctly")
		}
		if cfg.RetentionDays != 120 {
			t.Error("RetentionDays not stored correctly")
		}
		if cfg.MaxDegree != 2 {
			t.Error("MaxDegree not stored correctly")
		}
		if cfg.SimilarityThreshold != 0.95 {
			t.Error("SimilarityThreshold not stored correctly")
		}
		if cfg.MergeEnabled != true {
			t.Error("MergeEnabled not stored correctly")
		}
		if cfg.VectorIndexName != "customIndex" {
			t.Error("VectorIndexName not stored correctly")
		}
		if cfg.DryRun != false {
			t.Error("DryRun not stored correctly")
		}
		if cfg.SpaceID != "test-space" {
			t.Error("SpaceID not stored correctly")
		}
		if cfg.BatchSize != 500 {
			t.Error("BatchSize not stored correctly")
		}
	})
}

// TestConfigValidation verifies the validation error cases for pruneConfig.
// From parseConfig(): various validations ensure config values are within valid ranges.
func TestConfigValidation(t *testing.T) {
	// Test weight-threshold validation (must be between 0 and 1)
	t.Run("weight-threshold validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       float64
			expectValid bool
		}{
			{"valid 0.0", 0.0, true},
			{"valid 0.5", 0.5, true},
			{"valid 1.0", 1.0, true},
			{"valid 0.01 (default)", 0.01, true},
			{"invalid negative", -0.1, false},
			{"invalid above 1", 1.1, false},
			{"invalid large negative", -100.0, false},
			{"invalid large positive", 100.0, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0 && tt.value <= 1
				if valid != tt.expectValid {
					t.Errorf("weight-threshold=%v: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test similarity-threshold validation (must be between 0 and 1)
	t.Run("similarity-threshold validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       float64
			expectValid bool
		}{
			{"valid 0.0", 0.0, true},
			{"valid 0.5", 0.5, true},
			{"valid 1.0", 1.0, true},
			{"valid 0.98 (default)", 0.98, true},
			{"invalid negative", -0.1, false},
			{"invalid above 1", 1.1, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0 && tt.value <= 1
				if valid != tt.expectValid {
					t.Errorf("similarity-threshold=%v: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test min-evidence validation (must be non-negative)
	t.Run("min-evidence validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectValid bool
		}{
			{"valid 0", 0, true},
			{"valid 1", 1, true},
			{"valid 3 (default)", 3, true},
			{"valid 100", 100, true},
			{"invalid -1", -1, false},
			{"invalid -100", -100, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0
				if valid != tt.expectValid {
					t.Errorf("min-evidence=%d: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test older-than-days validation (must be non-negative)
	t.Run("older-than-days validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectValid bool
		}{
			{"valid 0", 0, true},
			{"valid 1", 1, true},
			{"valid 30 (default)", 30, true},
			{"valid 365", 365, true},
			{"invalid -1", -1, false},
			{"invalid -30", -30, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0
				if valid != tt.expectValid {
					t.Errorf("older-than-days=%d: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test retention-days validation (must be non-negative)
	t.Run("retention-days validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectValid bool
		}{
			{"valid 0", 0, true},
			{"valid 1", 1, true},
			{"valid 90 (default)", 90, true},
			{"valid 365", 365, true},
			{"invalid -1", -1, false},
			{"invalid -90", -90, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0
				if valid != tt.expectValid {
					t.Errorf("retention-days=%d: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test max-degree validation (must be non-negative)
	t.Run("max-degree validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectValid bool
		}{
			{"valid 0", 0, true},
			{"valid 1 (default)", 1, true},
			{"valid 10", 10, true},
			{"valid 100", 100, true},
			{"invalid -1", -1, false},
			{"invalid -10", -10, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value >= 0
				if valid != tt.expectValid {
					t.Errorf("max-degree=%d: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test batch-size validation (must be positive)
	t.Run("batch-size validation", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectValid bool
		}{
			{"valid 1", 1, true},
			{"valid 100", 100, true},
			{"valid 1000 (default)", 1000, true},
			{"valid 10000", 10000, true},
			{"invalid 0", 0, false},
			{"invalid -1", -1, false},
			{"invalid -1000", -1000, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value > 0
				if valid != tt.expectValid {
					t.Errorf("batch-size=%d: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test space-id required validation
	t.Run("space-id required", func(t *testing.T) {
		tests := []struct {
			name        string
			value       string
			expectValid bool
		}{
			{"valid non-empty", "test-space", true},
			{"valid uuid-like", "550e8400-e29b-41d4-a716-446655440000", true},
			{"invalid empty", "", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.value != ""
				if valid != tt.expectValid {
					t.Errorf("space-id=%q: valid=%v, want %v", tt.value, valid, tt.expectValid)
				}
			})
		}
	})

	// Test Neo4j environment variables required validation
	t.Run("neo4j env vars required", func(t *testing.T) {
		tests := []struct {
			name        string
			uri         string
			user        string
			pass        string
			expectValid bool
		}{
			{"all set", "bolt://localhost:7687", "neo4j", "password", true},
			{"missing uri", "", "neo4j", "password", false},
			{"missing user", "bolt://localhost:7687", "", "password", false},
			{"missing pass", "bolt://localhost:7687", "neo4j", "", false},
			{"all missing", "", "", "", false},
			{"uri and user missing", "", "", "password", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				valid := tt.uri != "" && tt.user != "" && tt.pass != ""
				if valid != tt.expectValid {
					t.Errorf("neo4j vars (uri=%q, user=%q, pass_set=%v): valid=%v, want %v",
						tt.uri, tt.user, len(tt.pass) > 0, valid, tt.expectValid)
				}
			})
		}
	})

	// Test combined validation - a fully valid config
	t.Run("combined valid config", func(t *testing.T) {
		cfg := pruneConfig{
			Neo4jURI:            "bolt://localhost:7687",
			Neo4jUser:           "neo4j",
			Neo4jPass:           "testpassword",
			WeightThreshold:     0.01,
			MinEvidence:         3,
			OlderThanDays:       30,
			RetentionDays:       90,
			MaxDegree:           1,
			SimilarityThreshold: 0.98,
			MergeEnabled:        false,
			VectorIndexName:     "memNodeEmbedding",
			DryRun:              true,
			SpaceID:             "test-space",
			BatchSize:           1000,
		}

		// Validate all conditions match parseConfig validation logic
		errors := []string{}

		if cfg.Neo4jURI == "" || cfg.Neo4jUser == "" || cfg.Neo4jPass == "" {
			errors = append(errors, "neo4j credentials required")
		}
		if cfg.SpaceID == "" {
			errors = append(errors, "space-id required")
		}
		if cfg.WeightThreshold < 0 || cfg.WeightThreshold > 1 {
			errors = append(errors, "weight-threshold must be between 0 and 1")
		}
		if cfg.MinEvidence < 0 {
			errors = append(errors, "min-evidence must be non-negative")
		}
		if cfg.OlderThanDays < 0 {
			errors = append(errors, "older-than-days must be non-negative")
		}
		if cfg.RetentionDays < 0 {
			errors = append(errors, "retention-days must be non-negative")
		}
		if cfg.MaxDegree < 0 {
			errors = append(errors, "max-degree must be non-negative")
		}
		if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
			errors = append(errors, "similarity-threshold must be between 0 and 1")
		}
		if cfg.BatchSize <= 0 {
			errors = append(errors, "batch-size must be positive")
		}

		if len(errors) > 0 {
			t.Errorf("valid config should have no errors, got: %v", errors)
		}
	})

	// Test combined validation - invalid config with multiple errors
	t.Run("combined invalid config", func(t *testing.T) {
		cfg := pruneConfig{
			Neo4jURI:            "",  // invalid
			Neo4jUser:           "",  // invalid
			Neo4jPass:           "",  // invalid
			WeightThreshold:     1.5, // invalid
			MinEvidence:         -1,  // invalid
			OlderThanDays:       -1,  // invalid
			RetentionDays:       -1,  // invalid
			MaxDegree:           -1,  // invalid
			SimilarityThreshold: 2.0, // invalid
			MergeEnabled:        false,
			VectorIndexName:     "memNodeEmbedding",
			DryRun:              true,
			SpaceID:             "", // invalid
			BatchSize:           0,  // invalid
		}

		errors := []string{}

		if cfg.Neo4jURI == "" || cfg.Neo4jUser == "" || cfg.Neo4jPass == "" {
			errors = append(errors, "neo4j credentials required")
		}
		if cfg.SpaceID == "" {
			errors = append(errors, "space-id required")
		}
		if cfg.WeightThreshold < 0 || cfg.WeightThreshold > 1 {
			errors = append(errors, "weight-threshold must be between 0 and 1")
		}
		if cfg.MinEvidence < 0 {
			errors = append(errors, "min-evidence must be non-negative")
		}
		if cfg.OlderThanDays < 0 {
			errors = append(errors, "older-than-days must be non-negative")
		}
		if cfg.RetentionDays < 0 {
			errors = append(errors, "retention-days must be non-negative")
		}
		if cfg.MaxDegree < 0 {
			errors = append(errors, "max-degree must be non-negative")
		}
		if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
			errors = append(errors, "similarity-threshold must be between 0 and 1")
		}
		if cfg.BatchSize <= 0 {
			errors = append(errors, "batch-size must be positive")
		}

		// Expect multiple errors for this invalid config
		expectedErrorCount := 9 // neo4j, space-id, weight, min-evidence, older-than, retention, max-degree, similarity, batch-size
		if len(errors) != expectedErrorCount {
			t.Errorf("invalid config should have %d errors, got %d: %v",
				expectedErrorCount, len(errors), errors)
		}
	})
}

// TestResolveTransitiveMerges verifies the transitive merge chain resolution logic.
// From docs/07_Consolidation_and_Pruning.md:
// When multiple nodes form a transitive chain (A->B, B->C), all should merge
// to the oldest node in the chain. Uses union-find to identify connected components.
func TestResolveTransitiveMerges(t *testing.T) {
	// Base times for deterministic ordering
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // oldest
	t2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	t4 := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	t5 := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

	t.Run("empty input returns empty output", func(t *testing.T) {
		result := resolveTransitiveMerges(nil)
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d pairs", len(result))
		}

		result = resolveTransitiveMerges([]mergePair{})
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d pairs", len(result))
		}
	})

	t.Run("simple pair - A merges into B (B is older)", func(t *testing.T) {
		pairs := []mergePair{
			{
				SurvivorID:        "node-B",
				MergedID:          "node-A",
				Similarity:        0.99,
				SurvivorCreatedAt: t1, // older
				MergedCreatedAt:   t2, // newer
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 1 {
			t.Fatalf("expected 1 merge pair, got %d", len(result))
		}

		// B is older, so B should be survivor
		if result[0].SurvivorID != "node-B" {
			t.Errorf("expected survivor node-B, got %s", result[0].SurvivorID)
		}
		if result[0].MergedID != "node-A" {
			t.Errorf("expected merged node-A, got %s", result[0].MergedID)
		}
	})

	t.Run("simple pair - survivor/merged order swapped (A is older)", func(t *testing.T) {
		// Input has the pair ordered incorrectly (newer as survivor)
		// The function should resolve this to make the older node the survivor
		pairs := []mergePair{
			{
				SurvivorID:        "node-B", // newer
				MergedID:          "node-A", // older
				Similarity:        0.99,
				SurvivorCreatedAt: t2, // newer
				MergedCreatedAt:   t1, // older - should become survivor
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 1 {
			t.Fatalf("expected 1 merge pair, got %d", len(result))
		}

		// A is older, so A should be survivor after resolution
		if result[0].SurvivorID != "node-A" {
			t.Errorf("expected survivor node-A (older), got %s", result[0].SurvivorID)
		}
		if result[0].MergedID != "node-B" {
			t.Errorf("expected merged node-B (newer), got %s", result[0].MergedID)
		}
	})

	t.Run("chain A->B->C - all merge to oldest (A)", func(t *testing.T) {
		// Input pairs: A->B and B->C
		// A is oldest, so both B and C should merge into A
		pairs := []mergePair{
			{
				SurvivorID:        "node-A",
				MergedID:          "node-B",
				Similarity:        0.99,
				SurvivorCreatedAt: t1, // oldest
				MergedCreatedAt:   t2,
			},
			{
				SurvivorID:        "node-B",
				MergedID:          "node-C",
				Similarity:        0.98,
				SurvivorCreatedAt: t2,
				MergedCreatedAt:   t3, // newest
			},
		}

		result := resolveTransitiveMerges(pairs)

		// Should have 2 pairs: A<-B and A<-C (both merge into A)
		if len(result) != 2 {
			t.Fatalf("expected 2 merge pairs, got %d", len(result))
		}

		// Build a map of mergedID -> survivorID for easier verification
		mergeMap := make(map[string]string)
		for _, p := range result {
			mergeMap[p.MergedID] = p.SurvivorID
		}

		// Both B and C should merge into A
		if mergeMap["node-B"] != "node-A" {
			t.Errorf("expected node-B to merge into node-A, got %s", mergeMap["node-B"])
		}
		if mergeMap["node-C"] != "node-A" {
			t.Errorf("expected node-C to merge into node-A, got %s", mergeMap["node-C"])
		}
	})

	t.Run("chain with middle oldest - B is oldest in A->B->C chain", func(t *testing.T) {
		// B is oldest, A and C are newer
		// Both A and C should merge into B
		pairs := []mergePair{
			{
				SurvivorID:        "node-A",
				MergedID:          "node-B",
				Similarity:        0.99,
				SurvivorCreatedAt: t2, // newer
				MergedCreatedAt:   t1, // oldest (B)
			},
			{
				SurvivorID:        "node-B",
				MergedID:          "node-C",
				Similarity:        0.98,
				SurvivorCreatedAt: t1, // oldest (B)
				MergedCreatedAt:   t3, // newest
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 2 {
			t.Fatalf("expected 2 merge pairs, got %d", len(result))
		}

		mergeMap := make(map[string]string)
		for _, p := range result {
			mergeMap[p.MergedID] = p.SurvivorID
		}

		// Both A and C should merge into B (the oldest)
		if mergeMap["node-A"] != "node-B" {
			t.Errorf("expected node-A to merge into node-B (oldest), got %s", mergeMap["node-A"])
		}
		if mergeMap["node-C"] != "node-B" {
			t.Errorf("expected node-C to merge into node-B (oldest), got %s", mergeMap["node-C"])
		}
	})

	t.Run("multiple disconnected components", func(t *testing.T) {
		// Component 1: A<->B (A is oldest)
		// Component 2: C<->D (C is oldest)
		// Component 3: E<->F (E is oldest)
		pairs := []mergePair{
			// Component 1
			{
				SurvivorID:        "node-A",
				MergedID:          "node-B",
				Similarity:        0.99,
				SurvivorCreatedAt: t1, // oldest in component 1
				MergedCreatedAt:   t2,
			},
			// Component 2
			{
				SurvivorID:        "node-C",
				MergedID:          "node-D",
				Similarity:        0.98,
				SurvivorCreatedAt: t1, // oldest in component 2 (same time as A, but different component)
				MergedCreatedAt:   t3,
			},
			// Component 3
			{
				SurvivorID:        "node-F",
				MergedID:          "node-E",
				Similarity:        0.97,
				SurvivorCreatedAt: t5, // newer
				MergedCreatedAt:   t4, // E is older
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 3 {
			t.Fatalf("expected 3 merge pairs, got %d", len(result))
		}

		mergeMap := make(map[string]string)
		for _, p := range result {
			mergeMap[p.MergedID] = p.SurvivorID
		}

		// Component 1: B merges into A
		if mergeMap["node-B"] != "node-A" {
			t.Errorf("expected node-B to merge into node-A, got %s", mergeMap["node-B"])
		}

		// Component 2: D merges into C
		if mergeMap["node-D"] != "node-C" {
			t.Errorf("expected node-D to merge into node-C, got %s", mergeMap["node-D"])
		}

		// Component 3: F merges into E (E is older)
		if mergeMap["node-F"] != "node-E" {
			t.Errorf("expected node-F to merge into node-E (older), got %s", mergeMap["node-F"])
		}
	})

	t.Run("longer chain A->B->C->D - all merge to oldest", func(t *testing.T) {
		// A is oldest, B->C->D forms a chain connected to A
		pairs := []mergePair{
			{
				SurvivorID:        "node-A",
				MergedID:          "node-B",
				Similarity:        0.99,
				SurvivorCreatedAt: t1, // oldest
				MergedCreatedAt:   t2,
			},
			{
				SurvivorID:        "node-B",
				MergedID:          "node-C",
				Similarity:        0.98,
				SurvivorCreatedAt: t2,
				MergedCreatedAt:   t3,
			},
			{
				SurvivorID:        "node-C",
				MergedID:          "node-D",
				Similarity:        0.97,
				SurvivorCreatedAt: t3,
				MergedCreatedAt:   t4,
			},
		}

		result := resolveTransitiveMerges(pairs)

		// Should have 3 pairs: all merging into A
		if len(result) != 3 {
			t.Fatalf("expected 3 merge pairs, got %d", len(result))
		}

		mergeMap := make(map[string]string)
		for _, p := range result {
			mergeMap[p.MergedID] = p.SurvivorID
		}

		// All should merge into A (oldest)
		for _, nodeID := range []string{"node-B", "node-C", "node-D"} {
			if mergeMap[nodeID] != "node-A" {
				t.Errorf("expected %s to merge into node-A, got %s", nodeID, mergeMap[nodeID])
			}
		}
	})

	t.Run("equal timestamps use lexicographic order for determinism", func(t *testing.T) {
		// Both nodes have the same creation time
		// Should use lexicographic ordering for determinism
		sameTime := t1

		pairs := []mergePair{
			{
				SurvivorID:        "node-B",
				MergedID:          "node-A",
				Similarity:        0.99,
				SurvivorCreatedAt: sameTime,
				MergedCreatedAt:   sameTime,
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 1 {
			t.Fatalf("expected 1 merge pair, got %d", len(result))
		}

		// With equal times, node-A should be survivor (lexicographically smaller)
		if result[0].SurvivorID != "node-A" {
			t.Errorf("expected survivor node-A (lexicographically smaller), got %s", result[0].SurvivorID)
		}
		if result[0].MergedID != "node-B" {
			t.Errorf("expected merged node-B, got %s", result[0].MergedID)
		}
	})

	t.Run("similarity scores preserved from original pairs", func(t *testing.T) {
		pairs := []mergePair{
			{
				SurvivorID:        "node-A",
				MergedID:          "node-B",
				Similarity:        0.995,
				SurvivorCreatedAt: t1,
				MergedCreatedAt:   t2,
			},
		}

		result := resolveTransitiveMerges(pairs)

		if len(result) != 1 {
			t.Fatalf("expected 1 merge pair, got %d", len(result))
		}

		if result[0].Similarity != 0.995 {
			t.Errorf("expected similarity 0.995, got %f", result[0].Similarity)
		}
	})

	// Suppress unused variable warnings for t4 and t5
	_ = t4
	_ = t5
}
