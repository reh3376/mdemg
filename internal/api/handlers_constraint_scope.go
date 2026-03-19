package api

import (
	"net/http"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// handleConstraintScopeUpdate handles PATCH /v1/constraints/scope/{node_id}
// Allows manual override of a constraint's scope (file path glob pattern).
func (s *Server) handleConstraintScopeUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Extract node_id from URL: /v1/constraints/scope/{node_id}
	nodeID := strings.TrimPrefix(r.URL.Path, "/v1/constraints/scope/")
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required in path"})
		return
	}

	var req struct {
		Scope string `json:"scope"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	sess := s.driver.NewSession(r.Context(), neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(r.Context()) //nolint:errcheck

	_, err := neo4j.ExecuteWrite(r.Context(), sess, func(tx neo4j.ManagedTransaction) (any, error) {
		_, txErr := tx.Run(r.Context(), `
			MATCH (n:MemoryNode {node_id: $nodeID})
			WHERE n.constraint_type IS NOT NULL
			SET n.scope = $scope
			RETURN n.node_id`,
			map[string]any{
				"nodeID": nodeID,
				"scope":  req.Scope,
			})
		return nil, txErr
	})
	if err != nil {
		writeInternalError(w, err, "update constraint scope")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}
