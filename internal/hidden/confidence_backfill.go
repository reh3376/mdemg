package hidden

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type confidenceRow struct {
	nodeID     string
	confidence float64
}

// HEBB-ETA-001 — one-shot backfill for ActivationConfidence.
//
// Reads each non-archived MemoryNode in the space, computes confidence from
// (reinforcement_count, last_activated_at, surprise_score), writes to
// n.activation_confidence + n.activation_confidence_updated_at. Idempotent.
//
// Reinforcement count = SUM(evidence_count) over incoming CO_ACTIVATED_WITH edges.
// Last activated = MAX(last_activated_at) over the same edges, falling back to
// the node's own created_at. Surprise history = the node's own surprise_score
// (single-value → no variance penalty; a fuller history from reinforcement_events
// would be a follow-up sprint).
//
// Returns the number of nodes for which confidence was written (dry-run returns
// the number that WOULD have been written).
func BackfillActivationConfidence(
	ctx context.Context,
	driver neo4j.DriverWithContext,
	spaceID string,
	cfg ConfidenceConfig,
	batchSize int,
	dryRun bool,
) (int, error) {
	if spaceID == "" {
		return 0, fmt.Errorf("BackfillActivationConfidence: empty space_id")
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// One pass — the fetch query is cheap and Neo4j streams. If the space grows past
	// ~1M nodes this will need pagination; document as a follow-up.
	fetchCypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE NOT coalesce(n.is_archived, false)
OPTIONAL MATCH ()-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(n)
WITH n,
     coalesce(sum(r.evidence_count), 0) AS reinforceCount,
     coalesce(max(r.last_activated_at), n.created_at) AS lastActivatedAt,
     coalesce(n.surprise_score, 0.0) AS surpriseScore
RETURN n.node_id AS node_id, reinforceCount, lastActivatedAt, surpriseScore
`

	res, err := sess.Run(ctx, fetchCypher, map[string]any{"spaceId": spaceID})
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}

	var batch []confidenceRow
	total := 0
	now := time.Now()

	for res.Next(ctx) {
		rec := res.Record()
		nodeID, _ := rec.Get("node_id")
		reinforce, _ := rec.Get("reinforceCount")
		lastActRaw, _ := rec.Get("lastActivatedAt")
		surprise, _ := rec.Get("surpriseScore")

		nodeIDStr, ok := nodeID.(string)
		if !ok || nodeIDStr == "" {
			continue
		}
		reinforceI, _ := reinforce.(int64)

		var ageSec float64
		if t, ok := lastActRaw.(time.Time); ok && !t.IsZero() {
			ageSec = now.Sub(t).Seconds()
			if ageSec < 0 {
				ageSec = 0
			}
		} else {
			// no activity → treat as very old
			ageSec = cfg.HalfLifeSec * 10
		}

		var history []float64
		if s, ok := surprise.(float64); ok && s > 0 {
			history = []float64{s}
		}

		c := ComputeActivationConfidence(reinforceI, ageSec, history, cfg)
		batch = append(batch, confidenceRow{nodeID: nodeIDStr, confidence: c})

		if len(batch) >= batchSize {
			if !dryRun {
				if err := writeConfidenceBatch(ctx, sess, spaceID, batch); err != nil {
					return total, fmt.Errorf("write batch: %w", err)
				}
			}
			total += len(batch)
			slog.Info("confidence backfill batch flushed", "space_id", spaceID, "total", total, "dry_run", dryRun)
			batch = batch[:0]
		}
	}
	if err := res.Err(); err != nil {
		return total, fmt.Errorf("stream: %w", err)
	}
	if len(batch) > 0 {
		if !dryRun {
			if err := writeConfidenceBatch(ctx, sess, spaceID, batch); err != nil {
				return total, fmt.Errorf("write final batch: %w", err)
			}
		}
		total += len(batch)
	}

	return total, nil
}

func writeConfidenceBatch(ctx context.Context, sess neo4j.SessionWithContext, spaceID string, batch []confidenceRow) error {
	rows := make([]map[string]any, 0, len(batch))
	for _, r := range batch {
		rows = append(rows, map[string]any{"node_id": r.nodeID, "confidence": r.confidence})
	}
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		writeCypher := `
UNWIND $rows AS row
MATCH (n:MemoryNode {space_id: $spaceId, node_id: row.node_id})
SET n.activation_confidence = row.confidence,
    n.activation_confidence_updated_at = datetime()
RETURN count(n) AS written
`
		_, err := tx.Run(ctx, writeCypher, map[string]any{"spaceId": spaceID, "rows": rows})
		return nil, err
	})
	return err
}
