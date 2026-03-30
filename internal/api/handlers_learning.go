package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// handleNegativeFeedback processes POST /v1/learning/negative-feedback
func (s *Server) handleNegativeFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		SpaceID         string   `json:"space_id"`
		QueryNodeIDs    []string `json:"query_node_ids"`
		RejectedNodeIDs []string `json:"rejected_node_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}
	if len(req.QueryNodeIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query_node_ids must be non-empty"})
		return
	}
	if len(req.RejectedNodeIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rejected_node_ids must be non-empty"})
		return
	}

	result, err := s.learner.ApplyNegativeFeedback(r.Context(), req.SpaceID, req.QueryNodeIDs, req.RejectedNodeIDs)
	if err != nil {
		writeInternalError(w, err, "learning.negative_feedback")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleFrontierDetection processes GET /v1/memory/frontiers
func (s *Server) handleFrontierDetection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id query parameter is required"})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 100 {
		limit = 100
	}

	frontiers, err := s.queryFrontiers(r.Context(), spaceID, limit)
	if err != nil {
		writeInternalError(w, err, "learning.frontier_detection")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"frontiers": frontiers,
		"count":     len(frontiers),
	})
}

type frontierNode struct {
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	Layer         int    `json:"layer"`
	Summary       string `json:"summary"`
	OutgoingEdges int    `json:"outgoing_edges"`
	Evidence      int    `json:"evidence"`
}

func (s *Server) queryFrontiers(ctx context.Context, spaceID string, limit int) ([]frontierNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	minEvidence := s.cfg.FrontierMinEvidence
	if minEvidence <= 0 {
		minEvidence = 3
	}
	maxOutgoing := s.cfg.FrontierMaxOutgoing
	if maxOutgoing <= 0 {
		maxOutgoing = 2
	}

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 3
  AND coalesce(n.version, coalesce(n.update_count, 0)) >= $minEvidence
WITH n
// Count outgoing edges (exclude ABSTRACTS_TO which are structural)
OPTIONAL MATCH (n)-[out]->()
WHERE type(out) <> 'ABSTRACTS_TO'
WITH n, count(out) AS outDegree
WHERE outDegree <= $maxOutgoing
// Ensure no L5 parent exists
WITH n, outDegree
WHERE NOT EXISTS {
    MATCH (l5:MemoryNode {space_id: $spaceId})-[:ABSTRACTS_TO]->(n)
    WHERE l5.layer = 5
}
RETURN n.node_id AS node_id,
       coalesce(n.name, '') AS name,
       n.layer AS layer,
       coalesce(n.summary, '') AS summary,
       outDegree AS outgoing_edges,
       coalesce(n.version, coalesce(n.update_count, 0)) AS evidence
ORDER BY evidence DESC, outDegree ASC
LIMIT $limit`

	params := map[string]any{
		"spaceId":     spaceID,
		"minEvidence": minEvidence,
		"maxOutgoing": maxOutgoing,
		"limit":       limit,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		frontiers := make([]frontierNode, 0)
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			layer, _ := rec.Get("layer")
			summary, _ := rec.Get("summary")
			outEdges, _ := rec.Get("outgoing_edges")
			evidence, _ := rec.Get("evidence")

			fn := frontierNode{
				NodeID:  nodeID.(string),
				Name:    name.(string),
				Summary: summary.(string),
			}
			if l, ok := layer.(int64); ok {
				fn.Layer = int(l)
			}
			if o, ok := outEdges.(int64); ok {
				fn.OutgoingEdges = int(o)
			}
			if e, ok := evidence.(int64); ok {
				fn.Evidence = int(e)
			}
			frontiers = append(frontiers, fn)
		}
		return frontiers, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]frontierNode), nil
}
