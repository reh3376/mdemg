package conversation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// DedupThreshold is the cosine similarity threshold above which
// two observations are considered near-duplicates.
const DedupThreshold = 0.95

// DedupResult describes the outcome of a deduplication check.
type DedupResult struct {
	IsDuplicate    bool    `json:"is_duplicate"`
	DuplicateOfID  string  `json:"duplicate_of_id,omitempty"`
	Similarity     float64 `json:"similarity,omitempty"`
	Action         string  `json:"action,omitempty"` // "skip" or "merge" or ""
}

// CheckDuplicate checks if a new observation's embedding is a near-duplicate
// of any existing observation in the same session/space.
// Returns a DedupResult indicating whether to skip or merge.
func CheckDuplicate(ctx context.Context, driver neo4j.DriverWithContext, spaceID, sessionID string, embedding []float32, threshold float64) (*DedupResult, error) {
	if len(embedding) == 0 {
		return &DedupResult{IsDuplicate: false}, nil
	}

	if threshold <= 0 {
		threshold = DedupThreshold
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build session filter
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = " AND n.session_id = $sessionId"
	}

	// Fetch recent observations with embeddings from the same session/space
	cypher := fmt.Sprintf(`
MATCH (n:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
WHERE n.layer = 0
  AND n.embedding IS NOT NULL%s
RETURN n.node_id AS nodeId, n.embedding AS embedding
ORDER BY n.created_at DESC
LIMIT 50`, sessionFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"sessionId": sessionID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		bestSim := 0.0
		bestNodeID := ""

		for res.Next(ctx) {
			rec := res.Record()
			nodeID := asString(rec, "nodeId")
			existingEmb := asFloat32Slice(rec, "embedding")
			if len(existingEmb) == 0 {
				continue
			}

			sim := cosineSimilarity(embedding, existingEmb)
			if sim > bestSim {
				bestSim = sim
				bestNodeID = nodeID
			}
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		return &DedupResult{
			IsDuplicate:   bestSim >= threshold,
			DuplicateOfID: bestNodeID,
			Similarity:    bestSim,
			Action:        dedupAction(bestSim, threshold),
		}, nil
	})

	if err != nil {
		return nil, fmt.Errorf("dedup check: %w", err)
	}

	return result.(*DedupResult), nil
}

// dedupAction determines what to do based on similarity.
func dedupAction(similarity, threshold float64) string {
	if similarity >= threshold {
		return "skip"
	}
	return ""
}

// MergeDuplicateObservation updates the existing observation's metadata
// to record that a duplicate was attempted, incrementing a counter.
func MergeDuplicateObservation(ctx context.Context, driver neo4j.DriverWithContext, existingNodeID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
SET n.duplicate_count = coalesce(n.duplicate_count, 0) + 1,
    n.updated_at = datetime()
RETURN n.node_id AS nodeId`
		_, err := tx.Run(ctx, cypher, map[string]any{"nodeId": existingNodeID})
		return nil, err
	})

	if err != nil {
		slog.Warn("failed to merge duplicate observation", "node_id", existingNodeID, "error", err)
	}
	return err
}

// asFloat32Slice extracts a []float32 from a neo4j record value.
func asFloat32Slice(rec *neo4j.Record, key string) []float32 {
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case []any:
		result := make([]float32, len(v))
		for i, elem := range v {
			switch f := elem.(type) {
			case float64:
				result[i] = float32(f)
			case float32:
				result[i] = f
			}
		}
		return result
	case []float64:
		result := make([]float32, len(v))
		for i, f := range v {
			result[i] = float32(f)
		}
		return result
	case []float32:
		return v
	}
	return nil
}
