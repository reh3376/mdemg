package api

import (
	"context"
	"net/http"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// NodeMetaResponse represents metadata for a MemoryNode, used for content-hash
// change detection during .md file ingestion.
type NodeMetaResponse struct {
	NodeID         string  `json:"node_id"`
	Path           string  `json:"path"`
	ContentHash    string  `json:"content_hash,omitempty"`
	FileSize       int64   `json:"file_size,omitempty"`
	LineCount      int     `json:"line_count,omitempty"`
	LastIngestedAt *string `json:"last_ingested_at,omitempty"`
	Status         string  `json:"status"`
}

// handleNodeMeta handles GET /v1/memory/node/meta?space_id=...&path=...
// Returns node metadata for content-hash comparison during .md file ingestion.
func (s *Server) handleNodeMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	path := r.URL.Query().Get("path")

	if spaceID == "" || path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "space_id and path query parameters are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id:$spaceId, path:$path})
RETURN n.node_id AS node_id,
       n.path AS path,
       n.content_hash AS content_hash,
       n.file_size AS file_size,
       n.line_count AS line_count,
       n.last_ingested_at AS last_ingested_at,
       n.status AS status`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"path":    path,
		})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			resp := NodeMetaResponse{
				Status: "active",
			}

			if v, ok := rec.Get("node_id"); ok && v != nil {
				resp.NodeID, _ = v.(string)
			}
			if v, ok := rec.Get("path"); ok && v != nil {
				resp.Path, _ = v.(string)
			}
			if v, ok := rec.Get("content_hash"); ok && v != nil {
				resp.ContentHash, _ = v.(string)
			}
			if v, ok := rec.Get("file_size"); ok && v != nil {
				resp.FileSize = toInt64(v)
			}
			if v, ok := rec.Get("line_count"); ok && v != nil {
				resp.LineCount = int(toInt64(v))
			}
			if v, ok := rec.Get("last_ingested_at"); ok && v != nil {
				s := v.(time.Time).Format(time.RFC3339)
				resp.LastIngestedAt = &s
			}
			if v, ok := rec.Get("status"); ok && v != nil {
				resp.Status, _ = v.(string)
			}

			return &resp, nil
		}

		return nil, res.Err()
	})
	if err != nil {
		writeInternalError(w, err, "node_meta.get")
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "node not found",
			"path":  path,
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

