package jiminy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// findContradictions queries CONTRADICTS edges near the given node IDs.
func (s *Service) findContradictions(ctx context.Context, spaceID string, nodeIDs []string) ([]contradictionMatch, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
	MATCH (a:MemoryNode {space_id: $spaceId})-[c:CONTRADICTS]->(b:MemoryNode {space_id: $spaceId})
	WHERE a.node_id IN $nodeIds OR b.node_id IN $nodeIds
	RETURN a.node_id AS src_id, a.name AS src_name,
	       b.node_id AS dst_id, b.name AS dst_name,
	       c.weight AS weight, c.evidence_count AS evidence_count
	ORDER BY c.evidence_count DESC LIMIT 5`

	var matches []contradictionMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeIds": nodeIDs,
		})
		if err != nil {
			return nil, err
		}

		for res.Next(ctx) {
			rec := res.Record()
			srcID, _ := rec.Get("src_id")
			srcName, _ := rec.Get("src_name")
			dstID, _ := rec.Get("dst_id")
			dstName, _ := rec.Get("dst_name")
			weight, _ := rec.Get("weight")
			evidence, _ := rec.Get("evidence_count")

			matches = append(matches, contradictionMatch{
				SourceNodeID: asString(srcID),
				SourceName:   asString(srcName),
				TargetNodeID: asString(dstID),
				TargetName:   asString(dstName),
				Weight:       asFloat64(weight),
				Evidence:     int(asFloat64(evidence)),
			})
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("contradiction query: %w", err)
	}

	slog.Info("jiminy: found contradictions", "count", len(matches), "node_count", len(nodeIDs), "space_id", spaceID)
	return matches, nil
}
