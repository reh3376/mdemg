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

	// JIMINY-SIGNAL-001 note: the follow-rate from this Cypher is INFLATED —
	// count(DISTINCT CASE WHEN outcome='followed' THEN guidance_id END) credits a
	// guidance_id to the "followed" numerator if ANY of its edges is followed,
	// while the denominator is the distinct-id count, so multi-outcome ids are
	// double-credited (live: 0.725 vs the honest constraint_outcomes ~0.27). The
	// assessor now OVERRIDES js.FollowRate with the windowed TSDB rate
	// (DatasetProvider.GuidanceEffectiveness); this path is retained only as the
	// fallback when TSDB is unavailable. Follow-up: de-inflate or retire this
	// follow-rate computation once the TSDB path is proven in production.
	cypher := `
	MATCH (n:MemoryNode {space_id: $spaceId})-[r:GUIDANCE_OUTCOME]-()
	RETURN
		count(DISTINCT r.guidance_id) AS total,
		count(DISTINCT CASE WHEN r.outcome_type = 'followed' THEN r.guidance_id END) AS followed,
		count(DISTINCT CASE WHEN r.outcome_type = 'partial_compliance' THEN r.guidance_id END) AS partial_compliance,
		count(DISTINCT CASE WHEN r.outcome_type = 'ignored' THEN r.guidance_id END) AS ignored,
		count(DISTINCT CASE WHEN r.outcome_type = 'contradicted' THEN r.guidance_id END) AS contradicted`

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
		partial := toInt(rec, "partial_compliance")
		ignored := toInt(rec, "ignored")
		contradicted := toInt(rec, "contradicted")

		// Follow rate: followed = full credit, partial_compliance = half credit
		var followRate float64
		if total > 0 {
			followRate = (float64(followed) + 0.5*float64(partial)) / float64(total)
		}

		return JiminyStats{
			TotalGuidanceIssued:    total,
			TotalFollowed:          followed,
			TotalPartialCompliance: partial,
			TotalIgnored:           ignored,
			TotalContradicted:      contradicted,
			FollowRate:             followRate,
		}, res.Err()
	})
	if err != nil {
		return JiminyStats{}, fmt.Errorf("get guidance stats: %w", err)
	}

	stats := result.(JiminyStats)

	// Compute constraint effectiveness rate (all-time, from Neo4j)
	// The 30d/windowed metric is now computed dynamically by Grafana from the
	// constraint_outcomes TSDB table, respecting the user's selected time range.
	effRate, hasData, err := sc.computeConstraintEffectivenessRate(ctx, spaceID)
	if err == nil {
		stats.ConstraintEffRate = effRate
		stats.ConstraintDataAvail = hasData
	}

	// Compute source diversity
	stats.SourceDiversity = sc.computeSourceDiversity(ctx, spaceID)

	return stats, nil
}

// computeConstraintEffectivenessRate returns the volume-weighted effectiveness rate
// across constraints with sufficient data (>= 5 surfaced outcomes). Low-volume
// constraints are excluded to reduce noise. Each qualifying constraint contributes
// proportionally to its surfaced count.
//
// Returns (rate, hasData, error) where hasData distinguishes "no qualifying
// constraints" from "0% effective". Time-windowed effectiveness is computed
// dynamically by Grafana from the constraint_outcomes TSDB table.
func (sc *StatsCollector) computeConstraintEffectivenessRate(ctx context.Context, spaceID string) (float64, bool, error) {
	sess := sc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})-[r:GUIDANCE_OUTCOME]-()
	WHERE NOT coalesce(c.is_archived, false)  // JIMINY-ARCHIVED-CODE-FILTER-001
	WITH c, count(r) AS surfaced,
	     sum(CASE WHEN r.outcome_type = 'followed' THEN 1.0 ELSE 0.0 END) AS followed,
	     sum(CASE WHEN r.outcome_type = 'partial_compliance' THEN 0.5 ELSE 0.0 END) AS partial_half
	WHERE surfaced >= 5
	WITH sum(followed) + sum(partial_half) AS effective, toFloat(sum(surfaced)) AS total
	WHERE total > 0
	RETURN effective / total AS weighted_effectiveness`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if txErr != nil {
			return nil, txErr
		}
		if !res.Next(ctx) {
			return nil, res.Err()
		}
		rec := res.Record()
		v, _ := rec.Get("weighted_effectiveness")
		if v == nil {
			return nil, nil
		}
		if f, ok := v.(float64); ok {
			return &f, nil
		}
		return nil, nil
	})
	if err != nil {
		return 0.0, false, err
	}
	if result == nil {
		// No qualifying constraints met the threshold — no data, not "0% effective"
		return 0.0, false, nil
	}
	f := *result.(*float64)
	return f, true, nil
}

// computeSourceDiversity measures how evenly guidance comes from different sources (0-1).
func (sc *StatsCollector) computeSourceDiversity(ctx context.Context, spaceID string) float64 {
	sess := sc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (n:MemoryNode {space_id: $spaceId})-[r:GUIDANCE_OUTCOME]-()
	WITH COALESCE(r.guidance_type, n.obs_type) AS source_type, count(r) AS cnt
	WHERE source_type IS NOT NULL
	RETURN source_type, cnt`

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
