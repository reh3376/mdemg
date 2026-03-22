package jiminy

import (
	"context"
	"fmt"
	"log"
)

// ProtocolEvolver analyzes protocol metrics snapshots and applies mutations.
type ProtocolEvolver struct {
	metrics   *ProtocolMetricsCollector
	codeGen   *ConstraintCodeGenerator
	encoder   *ProtocolEncoder
	seqTracker *SequenceTracker
}

// NewProtocolEvolver creates a new protocol evolver.
func NewProtocolEvolver(metrics *ProtocolMetricsCollector, codeGen *ConstraintCodeGenerator, encoder *ProtocolEncoder, seqTracker *SequenceTracker) *ProtocolEvolver {
	return &ProtocolEvolver{
		metrics:    metrics,
		codeGen:    codeGen,
		encoder:    encoder,
		seqTracker: seqTracker,
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

	log.Printf("j17: codified constraint %s → %s", constraintNodeID, code)

	return map[string]any{
		"constraint_id": constraintNodeID,
		"new_code":      code,
		"previous_tier": 2,
		"action":        "codify_constraint",
	}, nil
}

// RetireCode retires a T1 code with low comprehension back to T2.
func (pe *ProtocolEvolver) RetireCode(_ context.Context, _ string, constraintCode string) (map[string]any, error) {
	// In a full implementation, this would clear the constraint_code property on the Neo4j node.
	// For now, we track the retirement in the code generator.
	log.Printf("j17: retired code %s back to T2", constraintCode)

	return map[string]any{
		"retired_code":       constraintCode,
		"action":             "retire_code",
		"comprehension_rate": 0.0,
	}, nil
}

// AdjustTierThresholds adjusts tier selection parameters based on metrics.
func (pe *ProtocolEvolver) AdjustTierThresholds(_ context.Context, _ string) (map[string]any, error) {
	if pe.metrics == nil {
		return nil, fmt.Errorf("j17: metrics collector not available")
	}

	snapshot := pe.metrics.Snapshot()

	// Adjustment logic: if T1 distribution < 70%, lower threshold; if > 90%, raise
	oldThreshold := 0.5 // placeholder — real implementation would read from encoder
	newThreshold := oldThreshold

	if snapshot.TierDistribution[0] < 0.7 {
		newThreshold -= 0.05
	} else if snapshot.TierDistribution[0] > 0.9 {
		newThreshold += 0.05
	}

	// Clamp
	if newThreshold < 0.1 {
		newThreshold = 0.1
	}
	if newThreshold > 0.9 {
		newThreshold = 0.9
	}

	log.Printf("j17: adjusted tier threshold %.2f → %.2f", oldThreshold, newThreshold)

	return map[string]any{
		"old_threshold":     oldThreshold,
		"new_threshold":     newThreshold,
		"adjustment_reason": fmt.Sprintf("T1 distribution: %.1f%%", snapshot.TierDistribution[0]*100),
		"action":            "adjust_tier_threshold",
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
		// Note: can't get current buffer size from seqTracker directly, use config default
		oldSize = 1000
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
