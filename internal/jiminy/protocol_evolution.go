package jiminy

import (
	"context"
	"fmt"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ProtocolEvolver analyzes protocol metrics snapshots and applies mutations.
type ProtocolEvolver struct {
	metrics     *ProtocolMetricsCollector
	codeGen     *ConstraintCodeGenerator
	encoder     *ProtocolEncoder
	seqTracker  *SequenceTracker
	driver      neo4j.DriverWithContext // GAP 4: for RetireCode Neo4j writes
	trustScorer *TrustScorer           // GAP 5: for threshold reads/writes
}

// NewProtocolEvolver creates a new protocol evolver.
func NewProtocolEvolver(metrics *ProtocolMetricsCollector, codeGen *ConstraintCodeGenerator, encoder *ProtocolEncoder, seqTracker *SequenceTracker, driver neo4j.DriverWithContext, trustScorer *TrustScorer) *ProtocolEvolver {
	return &ProtocolEvolver{
		metrics:     metrics,
		codeGen:     codeGen,
		encoder:     encoder,
		seqTracker:  seqTracker,
		driver:      driver,
		trustScorer: trustScorer,
	}
}

// CodifyConstraint generates and freezes a T1 code for a constraint currently sent as T2.
func (pe *ProtocolEvolver) CodifyConstraint(ctx context.Context, spaceID string, constraintNodeID string) (map[string]any, error) {
	if pe.codeGen == nil {
		return nil, fmt.Errorf("j17: code generator not available")
	}

	// Generate code
	code, err := pe.codeGen.GenerateCode(ctx, "constraint", constraintNodeID)
	if err != nil {
		return nil, fmt.Errorf("j17: codify constraint: %w", err)
	}

	// Gap 5: Persist the code to Neo4j (following RetireCode's pattern)
	if pe.driver != nil {
		sess := pe.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer sess.Close(ctx)
		_, writeErr := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `MATCH (n:MemoryNode)
				WHERE n.node_id = $nodeId AND n.space_id = $spaceId
				SET n.constraint_code = $code,
					n.constraint_code_assigned_at = datetime(),
					n.constraint_code_assigned_by = "mdemg-codify"
				RETURN count(n) AS updated`
			_, runErr := tx.Run(ctx, cypher, map[string]any{
				"nodeId":  constraintNodeID,
				"spaceId": spaceID,
				"code":    code,
			})
			return nil, runErr
		})
		if writeErr != nil {
			log.Printf("j17: codify Neo4j write error: %v", writeErr)
		}
	}

	log.Printf("j17: codified constraint %s → %s", constraintNodeID, code)

	return map[string]any{
		"constraint_id": constraintNodeID,
		"new_code":      code,
		"previous_tier": 2,
		"action":        "codify_constraint",
	}, nil
}

// RetireCode retires a T1 code with low comprehension back to T2.
// Clears constraint_code on the Neo4j node and removes from the codegen collision set.
func (pe *ProtocolEvolver) RetireCode(ctx context.Context, spaceID string, constraintCode string) (map[string]any, error) {
	// Write to Neo4j: clear constraint_code, mark as retired
	if pe.driver != nil {
		sess := pe.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer sess.Close(ctx)

		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `
				MATCH (n:MemoryNode)
				WHERE n.space_id = $spaceId AND n.constraint_code = $code
				SET n.constraint_code = NULL, n.constraint_code_retired = true
				RETURN count(n) AS updated
			`
			_, runErr := tx.Run(ctx, cypher, map[string]any{
				"spaceId": spaceID,
				"code":    constraintCode,
			})
			return nil, runErr
		})
		if err != nil {
			log.Printf("j17: retire code Neo4j write error: %v", err)
			// Continue — still remove from collision set
		}
	}

	// Remove from codegen collision set
	if pe.codeGen != nil {
		pe.codeGen.UnregisterCode(constraintCode)
	}

	log.Printf("j17: retired code %s back to T2", constraintCode)

	return map[string]any{
		"retired_code":       constraintCode,
		"action":             "retire_code",
		"comprehension_rate": 0.0,
	}, nil
}

// AdjustTierThresholds adjusts tier selection parameters based on metrics.
// Reads real thresholds from trustScorer and encoder, computes adjustments
// based on T1 distribution, and writes back to both.
func (pe *ProtocolEvolver) AdjustTierThresholds(_ context.Context, _ string) (map[string]any, error) {
	if pe.metrics == nil {
		return nil, fmt.Errorf("j17: metrics collector not available")
	}

	snapshot := pe.metrics.Snapshot()

	// Read current thresholds from trust scorer or use defaults
	oldHigh := 0.8
	oldLow := 0.4
	if pe.trustScorer != nil {
		oldHigh = pe.trustScorer.HighThreshold()
		oldLow = pe.trustScorer.LowThreshold()
	}

	newHigh := oldHigh
	newLow := oldLow

	// Adjustment logic based on T1 distribution
	// If T1 usage is too low (< 70%), lower the high threshold to allow more T1
	// If T1 usage is too high (> 90%), raise the high threshold to be more selective
	if snapshot.TierDistribution[0] < 0.7 {
		newHigh -= 0.05
		newLow -= 0.03
	} else if snapshot.TierDistribution[0] > 0.9 {
		newHigh += 0.05
		newLow += 0.03
	}

	// Comprehension-based adjustment: if T1 comprehension is low on sufficient data,
	// raise threshold (fewer T1 assignments); if high, slightly lower bar
	if snapshot.TierComprehension[0] > 0 && snapshot.TierOutcomeCount[0] >= 5 {
		if snapshot.TierComprehension[0] < 0.6 {
			newHigh += 0.05 // T1 failing — make T1 harder to reach
		} else if snapshot.TierComprehension[0] > 0.9 && snapshot.TierOutcomeCount[0] >= 20 {
			newHigh -= 0.02 // T1 working great, slightly lower bar
		}
	}
	// If T2 comprehension significantly below T3, push more to T3
	if snapshot.TierComprehension[1] > 0 && snapshot.TierComprehension[2] > 0 {
		if snapshot.TierComprehension[1] < snapshot.TierComprehension[2]-0.15 {
			newLow += 0.03
		}
	}

	// Clamp: high in [0.3, 0.95], low >= 0.1 and low < high - 0.1
	if newHigh < 0.3 {
		newHigh = 0.3
	}
	if newHigh > 0.95 {
		newHigh = 0.95
	}
	if newLow < 0.1 {
		newLow = 0.1
	}
	if newLow >= newHigh-0.1 {
		newLow = newHigh - 0.15
		if newLow < 0.1 {
			newLow = 0.1
		}
	}

	// Write back to trust scorer
	if pe.trustScorer != nil {
		pe.trustScorer.SetThresholds(newHigh, newLow)
	}

	// Write back to encoder
	if pe.encoder != nil {
		pe.encoder.SetTierThresholds(newHigh, newLow)
	}

	log.Printf("j17: adjusted tier thresholds high %.2f → %.2f, low %.2f → %.2f",
		oldHigh, newHigh, oldLow, newLow)

	return map[string]any{
		"old_high_threshold": oldHigh,
		"new_high_threshold": newHigh,
		"old_low_threshold":  oldLow,
		"new_low_threshold":  newLow,
		"adjustment_reason":  fmt.Sprintf("T1 distribution: %.1f%%", snapshot.TierDistribution[0]*100),
		"action":             "adjust_tier_threshold",
		"t1_comprehension":   snapshot.TierComprehension[0],
		"t2_comprehension":   snapshot.TierComprehension[1],
		"t3_comprehension":   snapshot.TierComprehension[2],
	}, nil
}

// AdjustReplayBuffer adjusts the replay buffer size based on replay frequency.
func (pe *ProtocolEvolver) AdjustReplayBuffer(_ context.Context, _ string) (map[string]any, error) {
	if pe.metrics == nil {
		return nil, fmt.Errorf("j17: metrics collector not available")
	}

	snapshot := pe.metrics.Snapshot()
	oldSize := 1000 // default
	if pe.seqTracker != nil {
		oldSize = pe.seqTracker.BufferSize()
	}

	newSize := oldSize
	if snapshot.ReplayFrequencyPerHour > 5.0 {
		// High replay frequency → increase buffer
		newSize = int(float64(oldSize) * 1.25)
		if newSize > 5000 {
			newSize = 5000
		}
	} else if snapshot.ReplayFrequencyPerHour < 1.0 {
		// Low replay frequency → decrease buffer
		newSize = int(float64(oldSize) * 0.9)
		if newSize < 500 {
			newSize = 500
		}
	}

	// Apply the new buffer size
	if pe.seqTracker != nil && newSize != oldSize {
		pe.seqTracker.Resize(newSize)
	}

	log.Printf("j17: adjusted replay buffer %d → %d (replay freq: %.1f/hr)",
		oldSize, newSize, snapshot.ReplayFrequencyPerHour)

	return map[string]any{
		"old_size":          oldSize,
		"new_size":          newSize,
		"replay_freq_before": snapshot.ReplayFrequencyPerHour,
		"action":            "adjust_replay_buffer",
	}, nil
}

// NeedsDeepAnalysis returns true when macro-level protocol analysis is warranted.
func (pe *ProtocolEvolver) NeedsDeepAnalysis() bool {
	if pe.metrics == nil {
		return false
	}

	snapshot := pe.metrics.Snapshot()

	// Comprehension ceiling with outliers
	if snapshot.AvgComprehension > 0.95 && len(snapshot.CodeComprehension) > 0 {
		for _, rate := range snapshot.CodeComprehension {
			if rate < 0.7 {
				return true // outlier needs attention
			}
		}
	}

	return false
}
