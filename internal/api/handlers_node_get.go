package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// NodeGetResponse is the provenance-oriented projection of a MemoryNode
// returned by GET /v1/memory/nodes/{node_id}. Shape matches the J17 agent
// briefing packet v1.1 provenance contract — every field a receiver needs to
// verify that a `src:NODE_ID` reference in surfaced guidance is a real,
// resolvable durable rule rather than noise.
//
// Omitted (deliberately): the full embedding vector (expensive to serialize,
// bytes-not-semantic for provenance), structured_data blob (JSONB too wide
// for a point-read), and co-activation neighborhood (callers that need the
// graph context use /v1/eventgraph/* instead).
type NodeGetResponse struct {
	NodeID             string  `json:"node_id"`
	SpaceID            string  `json:"space_id"`
	Path               string  `json:"path,omitempty"`
	Name               string  `json:"name,omitempty"`
	RoleType           string  `json:"role_type,omitempty"`
	ObsType            string  `json:"obs_type,omitempty"`
	Layer              int     `json:"layer"`
	Content            string  `json:"content,omitempty"`
	ConstraintCode     string  `json:"constraint_code,omitempty"`
	ConstraintType     string  `json:"constraint_type,omitempty"`
	IsInformational    bool    `json:"is_informational"`
	IsArchived         bool    `json:"is_archived"`
	ArchiveReason      string  `json:"archive_reason,omitempty"`
	ArchivedAt         *string `json:"archived_at,omitempty"`
	VerifiabilityClass string  `json:"verifiability_class,omitempty"`
	CreatedAt          *string `json:"created_at,omitempty"`
	UpdatedAt          *string `json:"updated_at,omitempty"`
	SessionID          string  `json:"session_id,omitempty"`
}

// handleGetNode handles GET /v1/memory/nodes/{node_id}?space_id=X — returns
// the provenance-oriented projection of a MemoryNode by its CUIDv2 node_id.
//
// Scoped by space_id (query param, required): the same node_id MUST NOT be
// reachable from a different space's perspective, mirroring the shipped
// space-scoping on handleNodeMeta.
//
// 200: node found, body is NodeGetResponse
// 400: missing node_id (path) or space_id (query)
// 404: no node matches (node_id, space_id)
// 405: non-GET method (dispatch in handleNodeOperation routes GET here)
// 5xx: Neo4j error (via writeInternalError — handles caller-cancel 499)
//
// Dispatched from handleNodeOperation when r.Method == GET and path has no
// /archive or /unarchive suffix. DELETE on the same path still routes to
// handleDeleteNode (unchanged — this handler is a GET-only sibling).
func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	nodeID := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID = strings.TrimSuffix(nodeID, "/")
	if nodeID == "" || strings.Contains(nodeID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "space_id query parameter is required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
RETURN n.node_id             AS node_id,
       n.space_id             AS space_id,
       n.path                 AS path,
       n.name                 AS name,
       n.role_type             AS role_type,
       n.obs_type             AS obs_type,
       coalesce(n.layer, 0)   AS layer,
       n.content              AS content,
       n.constraint_code      AS constraint_code,
       n.constraint_type      AS constraint_type,
       coalesce(n.is_informational, false) AS is_informational,
       coalesce(n.is_archived, false)      AS is_archived,
       n.archive_reason        AS archive_reason,
       n.archived_at           AS archived_at,
       n.verifiability_class   AS verifiability_class,
       n.created_at            AS created_at,
       n.updated_at            AS updated_at,
       n.session_id            AS session_id`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"nodeId":  nodeID,
			"spaceId": spaceID,
		})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			resp := &NodeGetResponse{}

			strField := func(k string) string {
				if v, ok := rec.Get(k); ok && v != nil {
					if s, ok := v.(string); ok {
						return s
					}
				}
				return ""
			}
			boolField := func(k string) bool {
				if v, ok := rec.Get(k); ok && v != nil {
					if b, ok := v.(bool); ok {
						return b
					}
				}
				return false
			}
			timeField := func(k string) *string {
				if v, ok := rec.Get(k); ok && v != nil {
					if t, ok := v.(time.Time); ok {
						s := t.Format(time.RFC3339)
						return &s
					}
				}
				return nil
			}

			resp.NodeID = strField("node_id")
			resp.SpaceID = strField("space_id")
			resp.Path = strField("path")
			resp.Name = strField("name")
			resp.RoleType = strField("role_type")
			resp.ObsType = strField("obs_type")
			if v, ok := rec.Get("layer"); ok && v != nil {
				resp.Layer = int(toInt64(v))
			}
			resp.Content = strField("content")
			resp.ConstraintCode = strField("constraint_code")
			resp.ConstraintType = strField("constraint_type")
			resp.IsInformational = boolField("is_informational")
			resp.IsArchived = boolField("is_archived")
			resp.ArchiveReason = strField("archive_reason")
			resp.ArchivedAt = timeField("archived_at")
			resp.VerifiabilityClass = strField("verifiability_class")
			resp.CreatedAt = timeField("created_at")
			resp.UpdatedAt = timeField("updated_at")
			resp.SessionID = strField("session_id")

			return resp, nil
		}
		return nil, res.Err()
	})
	if err != nil {
		writeInternalError(w, err, "node_get.get")
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":   "node not found",
			"node_id": nodeID,
			"space_id": spaceID,
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
