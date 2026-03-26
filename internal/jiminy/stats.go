package jiminy

import (
	"context"
	"fmt"
	"math"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// StatsCollector queries Neo4j for aggregated Jiminy guidance metrics.
type StatsCollector struct {
	driver neo4j.DriverWithContext
	cfg    config.Config
}

// NewStatsCollector creates a new StatsCollector.
func NewStatsCollector(driver neo4j.DriverWithContext, cfg config.Config) *StatsCollector {
	return &StatsCollector{driver: driver, cfg: cfg}
}

// GetGuidanceStats returns aggregated guidance outcome metrics for RSIC assessment.
func (sc *StatsCollector) GetGuidanceStats(ctx context.Context, spaceID string) (JiminyStats, error) {
	if sc.driver == nil {
		return JiminyStats{}, nil
	}

	sess := sc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (n:MemoryNode {space_id: $spaceId})-[r:GUIDANCE_OUTCOME]-()
	RETURN
		count(r) AS total,
		count(CASE WHEN r.outcome_type = 'followed' THEN 1 END) AS followed,
		count(CASE WHEN r.outcome_type = 'ignored' THEN 1 END) AS ignored,
		count(CASE WHEN r.outcome_type = 'contradicted' THEN 1 END) AS contradicted`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
		})
		if txErr != nil {
			return nil, txErr
		}

		if !res.Next(ctx) {
			return JiminyStats{}, res.Err()
		}

		rec := res.Record()
		total := toInt(rec, "total")
		followed := toInt(rec, "followed")
		ignored := toInt(rec, "ignored")
		contradicted := toInt(rec, "contradicted")

		var followRate float64
		if total > 0 {
			followRate = float64(followed) / float64(total)
		}

		return JiminyStats{
			TotalGuidanceIssued: total,
			TotalFollowed:       followed,
			TotalIgnored:        ignored,
			TotalContradicted:   contradicted,
			FollowRate:          followRate,
		}, res.Err()
	})
	if err != nil {
		return JiminyStats{}, fmt.Errorf("get guidance stats: %w", err)
	}

	stats := result.(JiminyStats)

	// Compute constraint effectiveness rate
	effRate, err := sc.computeConstraintEffectivenessRate(ctx, spaceID)
	if err == nil {
		stats.ConstraintEffRate = effRate
	}

	// Compute source diversity
	stats.SourceDiversity = sc.computeSourceDiversity(ctx, spaceID)

	return stats, nil
}

// computeConstraintEffectivenessRate returns the average effectiveness rate across all constraints.
func (sc *StatsCollector) computeConstraintEffectivenessRate(ctx context.Context, spaceID string) (float64, error) {
	sess := sc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
	OPTIONAL MATCH (c)-[r:GUIDANCE_OUTCOME]-()
	WITH c,
	     count(r) AS surfaced,
	     count(CASE WHEN r.outcome_type = 'followed' THEN 1 END) AS followed
	WHERE surfaced > 0
	RETURN avg(toFloat(followed) / toFloat(surfaced)) AS avg_effectiveness`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if txErr != nil {
			return 0.0, txErr
		}
		if !res.Next(ctx) {
			return 0.0, res.Err()
		}
		rec := res.Record()
		v, _ := rec.Get("avg_effectiveness")
		if v == nil {
			return 0.0, nil
		}
		if f, ok := v.(float64); ok {
			return f, nil
		}
		return 0.0, nil
	})
	if err != nil {
		return 0.0, err
	}
	return result.(float64), nil
}

// computeSourceDiversity measures how evenly guidance comes from different sources (0-1).
func (sc *StatsCollector) computeSourceDiversity(ctx context.Context, spaceID string) float64 {
	sess := sc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (n:MemoryNode {space_id: $spaceId})-[r:GUIDANCE_OUTCOME]-()
	WITH n.obs_type AS obs_type, count(r) AS cnt
	WHERE obs_type IS NOT NULL
	RETURN obs_type, cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if txErr != nil {
			return nil, txErr
		}

		var counts []int
		for res.Next(ctx) {
			rec := res.Record()
			cntVal, _ := rec.Get("cnt")
			if n, ok := cntVal.(int64); ok {
				counts = append(counts, int(n))
			}
		}
		return counts, res.Err()
	})
	if err != nil || result == nil {
		return 0.0
	}

	counts := result.([]int)
	if len(counts) <= 1 {
		return 0.0
	}

	// Shannon entropy normalized to [0, 1]
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0.0
	}

	entropy := 0.0
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / float64(total)
			entropy -= p * ln(p)
		}
	}
	maxEntropy := ln(float64(len(counts)))
	if maxEntropy == 0 {
		return 0.0
	}
	return entropy / maxEntropy
}

// ln computes natural logarithm; returns 0 for non-positive values.
func ln(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Log(x)
}

// toInt extracts an int from a neo4j record field.
func toInt(rec *neo4j.Record, key string) int {
	v, _ := rec.Get(key)
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
