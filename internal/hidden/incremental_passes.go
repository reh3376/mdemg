package hidden

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// CONSOLIDATE-PERF-002: incremental forward/backward passes.
//
// The legacy passes sweep EVERY L1 node (5.2k live) and EVERY L2+ concept
// (19.6k live) each consolidation cycle even when nothing under them changed
// — ~33s/cycle × ~14 cycles/day of pure recomputation on mdemg-dev. Both
// passes are pure functions of (own embedding, member embeddings+weights,
// upper-layer embeddings), so skip-if-inputs-unchanged is exactly correct.
//
// Gates key on the timestamps the passes already write
// (last_forward_pass / last_backward_pass) versus:
//   - member-node recency  (b.updated_at)
//   - membership recency   (GENERALIZES/ABSTRACTS_TO r.created_at — catches
//     HIDDEN-CHURN-003 re-assignments of OLD nodes to new parents)
//   - cascade advancement  (a lower layer's last_forward_pass moving)
//
// Known caveat (documented in the sprint plan): weight-only drift (decay
// touching edge weights without any timestamp change) is invisible to the
// gates; it self-corrects whenever any member changes, and fully corrects on
// the explicit full path (`concepts recluster` / full_recluster).
//
// Pagination note: the legacy SKIP/LIMIT pattern cannot be combined with a
// gate — stamping nodes mid-pagination mutates the filtered set and SKIP
// then jumps over still-pending nodes. The incremental path therefore
// collects pending node_ids first, then processes id-batches.

const incrementalPassBatchSize = 50

// pendingForwardHiddenCypher selects L1 nodes whose forward inputs changed.
const pendingForwardHiddenCypher = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.last_forward_pass IS NULL
   OR EXISTS {
        MATCH (nb:MemoryNode {space_id: $spaceId, layer: 0})-[nr:GENERALIZES]->(h)
        WHERE nb.updated_at > h.last_forward_pass
           OR nr.created_at > h.last_forward_pass
      }
RETURN h.node_id AS id ORDER BY id`

// forwardHiddenByIDsCypher is the legacy L1 aggregation body re-targeted at
// an explicit id set. TestIncrementalPassBodiesMatchLegacy pins the shared
// aggregation fragment against the legacy query to prevent drift.
// fwdHiddenAggBody is the SINGLE aggregation body shared by the legacy
// (paginated) and incremental (id-batched) L1 forward passes — one source of
// truth for the math; TestLegacyPassCypherComposition pins the composed
// legacy strings byte-for-byte.
const fwdHiddenAggBody = `MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[r:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, collect({emb: b.embedding, weight: coalesce(r.weight, 1.0)}) AS neighbors
WHERE size(neighbors) > 0
WITH h, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH h, neighbors, totalWeight,
     [i IN range(0, size(h.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $alpha * coalesce(h.embedding[i], 0) + $beta * aggregated[i]
    ],
    h.last_forward_pass = datetime(),
    h.aggregation_count = size(neighbors)
RETURN count(h) AS updated`

const forwardHiddenByIDsCypher = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.node_id IN $ids
` + fwdHiddenAggBody

// pendingForwardConceptCypher selects L2+ concepts whose forward inputs
// changed — including the cascade (a member L1's last_forward_pass advanced
// past the concept's own stamp; the L1 phase runs first in ForwardPass, so
// this cycle's L1 updates are visible here).
const pendingForwardConceptCypher = `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2
  AND (c.last_forward_pass IS NULL
       OR EXISTS {
            MATCH (nh:MemoryNode {space_id: $spaceId, layer: 1})-[nr:ABSTRACTS_TO]->(c)
            WHERE nh.last_forward_pass > c.last_forward_pass
               OR nh.updated_at > c.last_forward_pass
               OR nr.created_at > c.last_forward_pass
          })
RETURN c.node_id AS id ORDER BY id`

const fwdConceptAggBody = `MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})-[r:ABSTRACTS_TO]->(c)
WHERE h.message_pass_embedding IS NOT NULL OR h.embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(h.message_pass_embedding, h.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`

const forwardConceptByIDsCypher = `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.node_id IN $ids
` + fwdConceptAggBody

// pendingBackwardHiddenCypher selects L1 nodes whose backward inputs changed:
// members below (node or membership recency) or concepts above (their
// forward stamp advanced past this node's backward stamp).
const pendingBackwardHiddenCypher = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.last_backward_pass IS NULL
   OR EXISTS {
        MATCH (nb:MemoryNode {space_id: $spaceId, layer: 0})-[nr:GENERALIZES]->(h)
        WHERE nb.updated_at > h.last_backward_pass
           OR nr.created_at > h.last_backward_pass
      }
   OR EXISTS {
        MATCH (h)-[nr:ABSTRACTS_TO]->(nc:MemoryNode)
        WHERE nc.layer >= 2
          AND (nc.last_forward_change > h.last_backward_pass
               OR nr.created_at > h.last_backward_pass)
      }
RETURN h.node_id AS id ORDER BY id`

// stampForwardChangeCypher marks nodes the INCREMENTAL forward actually
// recomputed this pass. The backward cascade keys on last_forward_change,
// not last_forward_pass: other forward writers (theme/emergent repeats)
// stamp last_forward_pass unconditionally every cycle, which made a
// stamp-based cascade select EVERY L1 (live cycle 1: backward 16.2s while
// forward dropped to 0.28s). last_forward_change moves only on real
// incremental updates, so the cascade stays proportional to actual change.
const stampForwardChangeCypher = `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.node_id IN $ids AND n.last_forward_pass >= $passStart
SET n.last_forward_change = n.last_forward_pass
RETURN count(n) AS stamped`

const backwardAggBody = `OPTIONAL MATCH (h)-[rUp:ABSTRACTS_TO]->(c:MemoryNode)
WHERE c.layer >= 2 AND (c.message_pass_embedding IS NOT NULL OR c.embedding IS NOT NULL)
WITH h, collect(coalesce(c.message_pass_embedding, c.embedding)) AS conceptEmbs
OPTIONAL MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[rDown:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, conceptEmbs, collect(b.embedding) AS baseEmbs
WHERE size(conceptEmbs) > 0 OR size(baseEmbs) > 0
WITH h, conceptEmbs, baseEmbs,
     CASE WHEN size(conceptEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN conceptEmbs | sum + e[i]) / size(conceptEmbs)
       ]
     ELSE null END AS conceptSignal,
     CASE WHEN size(baseEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN baseEmbs | sum + e[i]) / size(baseEmbs)
       ]
     ELSE null END AS baseSignal
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $selfW * coalesce(h.embedding[i], 0) +
      $baseW * coalesce(baseSignal[i], h.embedding[i]) +
      $concW * coalesce(conceptSignal[i], h.embedding[i])
    ],
    h.last_backward_pass = datetime()
RETURN count(h) AS updated`

const backwardHiddenByIDsCypher = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.node_id IN $ids
` + backwardAggBody

// collectPendingIDs runs a pending-selection query and returns the ids.
func (s *Service) collectPendingIDs(ctx context.Context, cypher, spaceID string) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	res, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for r.Next(ctx) {
			if id, ok := r.Record().Get("id"); ok {
				if str, ok := id.(string); ok {
					ids = append(ids, str)
				}
			}
		}
		return ids, r.Err()
	})
	if err != nil {
		return nil, err
	}
	ids, _ := res.([]string)
	return ids, nil
}

// processByIDBatches runs an id-targeted aggregation query over chunks of
// pending ids, returning the total updated count.
func (s *Service) processByIDBatches(ctx context.Context, cypher, spaceID string, ids []string, extra map[string]any) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	total := 0
	for start := 0; start < len(ids); start += incrementalPassBatchSize {
		end := min(start+incrementalPassBatchSize, len(ids))
		params := map[string]any{"spaceId": spaceID, "ids": ids[start:end]}
		maps.Copy(params, extra)
		updated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			r, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return 0, err
			}
			if r.Next(ctx) {
				u, _ := r.Record().Get("updated")
				return asInt(u), r.Err()
			}
			return 0, r.Err()
		})
		if err != nil {
			return total, fmt.Errorf("incremental pass batch [%d:%d]: %w", start, end, err)
		}
		total += updated.(int)
	}
	return total, nil
}

// forwardPassHiddenLayerIncremental is the gated L1 forward pass.
func (s *Service) forwardPassHiddenLayerIncremental(ctx context.Context, spaceID string) (int, error) {
	ids, err := s.collectPendingIDs(ctx, pendingForwardHiddenCypher, spaceID)
	if err != nil {
		return 0, fmt.Errorf("collect pending forward hidden: %w", err)
	}
	passStart := time.Now().UTC()
	updated, err := s.processByIDBatches(ctx, forwardHiddenByIDsCypher, spaceID, ids, map[string]any{
		"alpha": s.cfg.HiddenLayerForwardAlpha,
		"beta":  s.cfg.HiddenLayerForwardBeta,
	})
	if err != nil {
		return updated, err
	}
	if err := s.stampForwardChange(ctx, spaceID, ids, passStart); err != nil {
		return updated, fmt.Errorf("stamp forward change (hidden): %w", err)
	}
	return updated, nil
}

// forwardPassConceptLayerIncremental is the gated L2+ forward pass.
func (s *Service) forwardPassConceptLayerIncremental(ctx context.Context, spaceID string) (int, error) {
	ids, err := s.collectPendingIDs(ctx, pendingForwardConceptCypher, spaceID)
	if err != nil {
		return 0, fmt.Errorf("collect pending forward concept: %w", err)
	}
	passStart := time.Now().UTC()
	updated, err := s.processByIDBatches(ctx, forwardConceptByIDsCypher, spaceID, ids, map[string]any{
		"alpha": s.cfg.HiddenLayerForwardAlpha,
		"beta":  s.cfg.HiddenLayerForwardBeta,
	})
	if err != nil {
		return updated, err
	}
	if err := s.stampForwardChange(ctx, spaceID, ids, passStart); err != nil {
		return updated, fmt.Errorf("stamp forward change (concept): %w", err)
	}
	return updated, nil
}

// stampForwardChange sets last_forward_change on the subset of ids the
// incremental pass actually re-aggregated (their last_forward_pass advanced
// past passStart). Cheap: runs over the small pending-id set only.
func (s *Service) stampForwardChange(ctx context.Context, spaceID string, ids []string, passStart time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, stampForwardChangeCypher, map[string]any{
			"spaceId": spaceID, "ids": ids, "passStart": passStart,
		})
		if err != nil {
			return nil, err
		}
		_, _ = r.Consume(ctx)
		return nil, nil
	})
	return err
}

// backwardPassIncremental is the gated backward pass.
func (s *Service) backwardPassIncremental(ctx context.Context, spaceID string) (int, error) {
	ids, err := s.collectPendingIDs(ctx, pendingBackwardHiddenCypher, spaceID)
	if err != nil {
		return 0, fmt.Errorf("collect pending backward: %w", err)
	}
	return s.processByIDBatches(ctx, backwardHiddenByIDsCypher, spaceID, ids, map[string]any{
		"selfW": s.cfg.HiddenLayerBackwardSelf,
		"baseW": s.cfg.HiddenLayerBackwardBase,
		"concW": s.cfg.HiddenLayerBackwardConc,
	})
}
