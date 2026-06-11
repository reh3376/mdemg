package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// Default cooler constants (used as fallbacks when config values are zero)
const (
	defaultReinforcementWindowHours    = 2
	defaultStabilityIncreasePerReinf   = 0.15
	defaultStabilityDecayRate          = 0.1
	defaultTombstoneThreshold          = 0.05
	defaultGraduationThreshold         = 0.8
)

// ContextCooler manages the graduation of volatile observations to permanent memory.
// It implements a "cooling" metaphor where new hot (volatile) observations must prove
// their value through reinforcement before becoming permanent (cold storage).
type ContextCooler struct {
	driver neo4j.DriverWithContext

	// Config-driven tuning parameters
	reinforcementWindowHours  int
	stabilityIncreasePerReinf float64
	stabilityDecayRate        float64
	tombstoneThreshold        float64
	graduationThreshold       float64
	constraintProtectFromDecay bool
	tombstoneMaxPerRun        int // RSIC-STORM-001: cap per ProcessGraduations run (0 = unlimited)
}

// NewContextCooler creates a new Context Cooler instance with config-driven parameters.
func NewContextCooler(driver neo4j.DriverWithContext, cfg config.Config) *ContextCooler {
	c := &ContextCooler{
		driver:                     driver,
		reinforcementWindowHours:   cfg.CoolerReinforcementWindowHours,
		stabilityIncreasePerReinf:  cfg.CoolerStabilityIncreasePerReinf,
		stabilityDecayRate:         cfg.CoolerStabilityDecayRate,
		tombstoneThreshold:         cfg.CoolerTombstoneThreshold,
		graduationThreshold:        cfg.CoolerGraduationThreshold,
		constraintProtectFromDecay: cfg.ConstraintProtectFromDecay,
		tombstoneMaxPerRun:         cfg.CoolerTombstoneMaxPerRun,
	}
	// Apply defaults for zero values
	if c.reinforcementWindowHours <= 0 {
		c.reinforcementWindowHours = defaultReinforcementWindowHours
	}
	if c.stabilityIncreasePerReinf <= 0 {
		c.stabilityIncreasePerReinf = defaultStabilityIncreasePerReinf
	}
	if c.stabilityDecayRate <= 0 {
		c.stabilityDecayRate = defaultStabilityDecayRate
	}
	if c.tombstoneThreshold <= 0 {
		c.tombstoneThreshold = defaultTombstoneThreshold
	}
	if c.graduationThreshold <= 0 {
		c.graduationThreshold = defaultGraduationThreshold
	}
	return c
}

// GraduationResult contains the result of a graduation check
type GraduationResult struct {
	NodeID         string  `json:"node_id"`
	Graduated      bool    `json:"graduated"`
	Tombstoned     bool    `json:"tombstoned"`
	StabilityScore float64 `json:"stability_score"`
	Reason         string  `json:"reason"`
}

// UpdateStabilityOnReinforcement increases the stability score when a node is co-activated.
// This is called by the learning service when CO_ACTIVATED_WITH edges are created/strengthened.
func (c *ContextCooler) UpdateStabilityOnReinforcement(ctx context.Context, spaceID, nodeID string) error {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Increase stability score, capped at 1.0
		// Also update updated_at to track activity
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, false) = true
			SET n.stability_score = CASE
				WHEN coalesce(n.stability_score, 0.1) + $increase > 1.0 THEN 1.0
				ELSE coalesce(n.stability_score, 0.1) + $increase
			END,
			n.updated_at = datetime(),
			n.reinforcement_count = coalesce(n.reinforcement_count, 0) + 1
			RETURN n.stability_score as newScore
		`

		params := map[string]any{
			"spaceId":  spaceID,
			"nodeId":   nodeID,
			"increase": c.stabilityIncreasePerReinf,
		}

		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if result.Next(ctx) {
			newScore, _ := result.Record().Get("newScore")
			slog.Info("Context Cooler: reinforced node", "node_id", nodeID, "new_stability", newScore)
		}

		return nil, result.Err()
	})

	return err
}

// CheckGraduation checks if a node should graduate from volatile to permanent.
// Returns true if the node graduated, false otherwise.
func (c *ContextCooler) CheckGraduation(ctx context.Context, spaceID, nodeID string) (*GraduationResult, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Check if node qualifies for graduation
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, false) = true
			WITH n, coalesce(n.stability_score, 0.1) as stability
			WHERE stability >= $graduationThreshold
			SET n.volatile = false,
			    n.graduated_at = datetime(),
			    n.updated_at = datetime()
			RETURN n.node_id as nodeId, n.stability_score as stability, true as graduated
		`

		params := map[string]any{
			"spaceId":             spaceID,
			"nodeId":              nodeID,
			"graduationThreshold": c.graduationThreshold,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			stability, _ := rec.Get("stability")
			return &GraduationResult{
				NodeID:         nodeID,
				Graduated:      true,
				StabilityScore: stability.(float64),
				Reason:         fmt.Sprintf("stability %.2f >= %.2f threshold", stability, c.graduationThreshold),
			}, nil
		}

		// Node didn't graduate - get current stability
		checkCypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			RETURN coalesce(n.stability_score, 0.1) as stability, coalesce(n.volatile, false) as volatile
		`
		checkRes, err := tx.Run(ctx, checkCypher, map[string]any{"spaceId": spaceID, "nodeId": nodeID})
		if err != nil {
			return nil, err
		}

		if checkRes.Next(ctx) {
			rec := checkRes.Record()
			stability, _ := rec.Get("stability")
			volatile, _ := rec.Get("volatile")
			return &GraduationResult{
				NodeID:         nodeID,
				Graduated:      false,
				StabilityScore: stability.(float64),
				Reason: fmt.Sprintf("stability %.2f < %.2f threshold (volatile=%v)",
					stability, c.graduationThreshold, volatile),
			}, nil
		}

		return &GraduationResult{
			NodeID: nodeID,
			Reason: "node not found",
		}, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*GraduationResult), nil
}

// ApplyDecay applies stability decay to volatile nodes that haven't been reinforced.
// Decay formula: stability = stability * (1 - decay_rate)^days_inactive
func (c *ContextCooler) ApplyDecay(ctx context.Context, spaceID string) (int, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Apply decay to volatile nodes not updated in the last day
		// Decay is proportional to days since last update
		// Neo4j uses ^ for exponentiation, not power()
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, false) = true
			  AND NOT coalesce(n.pinned, false)
			  AND n.updated_at < datetime() - duration({hours: 24})
			WITH n,
			     duration.between(n.updated_at, datetime()).days as daysInactive,
			     coalesce(n.stability_score, 0.1) as currentStability
			WITH n, daysInactive, currentStability,
			     currentStability * ((1.0 - $decayRate) ^ daysInactive) as newStability
			SET n.stability_score = newStability,
			    n.decay_applied_at = datetime()
			RETURN count(n) as decayedCount
		`

		params := map[string]any{
			"spaceId":   spaceID,
			"decayRate": c.stabilityDecayRate,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			count, _ := res.Record().Get("decayedCount")
			return count.(int64), nil
		}

		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

// ProcessGraduations checks all volatile nodes and graduates or tombstones them.
// This should be called periodically by the APE scheduler.
func (c *ContextCooler) ProcessGraduations(ctx context.Context, spaceID string) (*GraduationSummary, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		summary := &GraduationSummary{
			SpaceID:   spaceID,
			Timestamp: time.Now().UTC(),
		}

		// Step 1: Graduate nodes with stability >= threshold
		graduateCypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, true) = true
			  AND NOT coalesce(n.pinned, false)
			  AND coalesce(n.stability_score, 0.1) >= $graduationThreshold
			SET n.volatile = false,
			    n.graduated_at = datetime(),
			    n.updated_at = datetime()
			RETURN count(n) as graduatedCount
		`

		res, err := tx.Run(ctx, graduateCypher, map[string]any{
			"spaceId":             spaceID,
			"graduationThreshold": c.graduationThreshold,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("graduatedCount")
			summary.Graduated = int(count.(int64))
		}

		// Step 2: Tombstone stagnant nodes past the reinforcement window with low stability
		// Constraint-tagged observations are protected from tombstoning when configured
		windowCutoff := time.Now().UTC().Add(-time.Duration(c.reinforcementWindowHours) * time.Hour)

		constraintExclusion := ""
		if c.constraintProtectFromDecay {
			constraintExclusion = "\n\t\t\t  AND NOT any(tag IN coalesce(n.tags, []) WHERE tag STARTS WITH 'constraint:')"
		}

		// RSIC-STORM-001: cap per run. The 2026-06-11 incident tombstoned
		// 5,397 observations in ONE uncapped sweep (the backlog of
		// pre-DH-004 graduation-bug victims) when a session-start hook hit
		// the graduate endpoint — a CRITICAL node-drop alert and hours of
		// mis-attribution. Successive runs drain a large backlog at cap
		// rate instead.
		capClause := ""
		if c.tombstoneMaxPerRun > 0 {
			capClause = "\n\t\t\tWITH n LIMIT $maxPerRun"
		}
		tombstoneCypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, true) = true
			  AND NOT coalesce(n.pinned, false)
			  AND n.created_at < datetime($windowCutoff)
			  AND coalesce(n.stability_score, 0.1) < $tombstoneThreshold` + constraintExclusion + capClause + `
			SET n.is_archived = true,
			    n.archived_at = datetime(),
			    n.archive_reason = 'context_cooler_tombstone',
			    n.updated_at = datetime()
			RETURN count(n) as tombstonedCount
		`

		res, err = tx.Run(ctx, tombstoneCypher, map[string]any{
			"spaceId":            spaceID,
			"windowCutoff":       windowCutoff.Format(time.RFC3339),
			"tombstoneThreshold": c.tombstoneThreshold,
			"maxPerRun":          c.tombstoneMaxPerRun,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("tombstonedCount")
			summary.Tombstoned = int(count.(int64))
			if c.tombstoneMaxPerRun > 0 && summary.Tombstoned >= c.tombstoneMaxPerRun {
				slog.Warn("context cooler: tombstone cap reached — backlog remains and will drain on subsequent runs",
					"space_id", spaceID, "tombstoned", summary.Tombstoned, "cap", c.tombstoneMaxPerRun)
			}
		}

		// Step 3: Count remaining volatile nodes
		countCypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND coalesce(n.volatile, true) = true
			  AND NOT coalesce(n.is_archived, false)
			RETURN count(n) as volatileCount
		`

		res, err = tx.Run(ctx, countCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("volatileCount")
			summary.RemainingVolatile = int(count.(int64))
		}

		return summary, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*GraduationSummary), nil
}

// GraduationSummary contains the results of a graduation processing run
type GraduationSummary struct {
	SpaceID           string    `json:"space_id"`
	Timestamp         time.Time `json:"timestamp"`
	Graduated         int       `json:"graduated"`
	Tombstoned        int       `json:"tombstoned"`
	RemainingVolatile int       `json:"remaining_volatile"`
	DecayApplied      int       `json:"decay_applied"`
}

// GetVolatileStats returns statistics about volatile nodes
func (c *ContextCooler) GetVolatileStats(ctx context.Context, spaceID string) (*VolatileStats, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND NOT coalesce(n.is_archived, false)
			WITH n,
			     coalesce(n.volatile, true) as isVolatile,
			     coalesce(n.stability_score, 0.1) as stability
			RETURN
			  count(CASE WHEN isVolatile THEN 1 END) as volatileCount,
			  count(CASE WHEN NOT isVolatile THEN 1 END) as permanentCount,
			  avg(CASE WHEN isVolatile THEN stability END) as avgVolatileStability,
			  min(CASE WHEN isVolatile THEN stability END) as minVolatileStability,
			  max(CASE WHEN isVolatile THEN stability END) as maxVolatileStability
		`

		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		stats := &VolatileStats{SpaceID: spaceID}
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("volatileCount"); ok && v != nil {
				stats.VolatileCount = int(v.(int64))
			}
			if v, ok := rec.Get("permanentCount"); ok && v != nil {
				stats.PermanentCount = int(v.(int64))
			}
			if v, ok := rec.Get("avgVolatileStability"); ok && v != nil {
				stats.AvgVolatileStability = v.(float64)
			}
			if v, ok := rec.Get("minVolatileStability"); ok && v != nil {
				stats.MinVolatileStability = v.(float64)
			}
			if v, ok := rec.Get("maxVolatileStability"); ok && v != nil {
				stats.MaxVolatileStability = v.(float64)
			}
		}

		return stats, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.(*VolatileStats), nil
}

// VolatileStats contains statistics about volatile observations
type VolatileStats struct {
	SpaceID              string  `json:"space_id"`
	VolatileCount        int     `json:"volatile_count"`
	PermanentCount       int     `json:"permanent_count"`
	AvgVolatileStability float64 `json:"avg_volatile_stability"`
	MinVolatileStability float64 `json:"min_volatile_stability"`
	MaxVolatileStability float64 `json:"max_volatile_stability"`
}

// CalculateDecayedStability calculates what a stability score would be after decay.
// This is a pure function for testing/preview purposes (uses default decay rate).
func CalculateDecayedStability(currentStability float64, daysInactive int) float64 {
	if daysInactive <= 0 {
		return currentStability
	}
	return currentStability * math.Pow(1.0-defaultStabilityDecayRate, float64(daysInactive))
}

// CalculateReinforcementsToGraduate calculates how many reinforcements needed to graduate (uses defaults).
func CalculateReinforcementsToGraduate(currentStability float64) int {
	if currentStability >= defaultGraduationThreshold {
		return 0
	}
	needed := (defaultGraduationThreshold - currentStability) / defaultStabilityIncreasePerReinf
	return int(math.Ceil(needed))
}
