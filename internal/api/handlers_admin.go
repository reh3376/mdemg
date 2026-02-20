package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/models"
)

// handleAdminSpaces handles GET /v1/admin/spaces — list all spaces with metadata.
func (s *Server) handleAdminSpaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Parse optional query params
	q := r.URL.Query()
	prunableFilter := q.Get("prunable") // "", "true", or "false"
	limitStr := q.Get("limit")
	limit := 100
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 500 {
		limit = 500
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Two-pass query: first collect all distinct space_ids from both TapRoots
	// and MemoryNodes, then enrich with TapRoot metadata and node counts.
	// This catches orphan spaces that have MemoryNodes but no TapRoot.
	result, err := session.Run(ctx, `
		CALL {
			MATCH (t:TapRoot) RETURN t.space_id AS sid
			UNION
			MATCH (n:MemoryNode) WHERE n.space_id IS NOT NULL RETURN DISTINCT n.space_id AS sid
		}
		WITH DISTINCT sid
		OPTIONAL MATCH (t:TapRoot {space_id: sid})
		OPTIONAL MATCH (n:MemoryNode {space_id: sid})
		WITH sid, t, count(n) AS node_count
		OPTIONAL MATCH (o:MemoryNode {space_id: sid})
		WHERE o.role_type = 'conversation_observation'
		WITH sid, t, node_count, count(o) AS obs_count
		RETURN sid AS space_id,
		       coalesce(t.prunable, false) AS prunable,
		       toString(t.created_at) AS created_at,
		       toString(t.last_ingest_at) AS last_ingest_at,
		       coalesce(t.ingest_count, 0) AS ingest_count,
		       node_count, obs_count,
		       t IS NULL AS orphan_taproot
		ORDER BY sid
		LIMIT $limit
	`, map[string]any{"limit": limit})
	if err != nil {
		writeInternalError(w, err, "admin-spaces-list")
		return
	}

	var spaces []models.SpaceInfo
	prunableCount := 0

	for result.Next(ctx) {
		rec := result.Record()
		spaceID, _ := rec.Get("space_id")
		prunable, _ := rec.Get("prunable")
		createdAt, _ := rec.Get("created_at")
		lastIngestAt, _ := rec.Get("last_ingest_at")
		ingestCount, _ := rec.Get("ingest_count")
		nodeCount, _ := rec.Get("node_count")
		obsCount, _ := rec.Get("obs_count")

		sid := toString(spaceID)
		isPrunable := toBool(prunable)

		// Apply filter
		if prunableFilter == "true" && !isPrunable {
			continue
		}
		if prunableFilter == "false" && isPrunable {
			continue
		}

		if isPrunable {
			prunableCount++
		}

		spaces = append(spaces, models.SpaceInfo{
			SpaceID:          sid,
			Prunable:         isPrunable,
			Protected:        IsProtectedSpace(sid),
			CreatedAt:        toString(createdAt),
			LastIngestAt:     toString(lastIngestAt),
			IngestCount:      toInt(ingestCount),
			NodeCount:        toInt(nodeCount),
			ObservationCount: toInt(obsCount),
		})
	}

	if spaces == nil {
		spaces = []models.SpaceInfo{}
	}

	writeJSON(w, http.StatusOK, models.AdminSpacesResponse{
		Spaces:        spaces,
		Total:         len(spaces),
		PrunableCount: prunableCount,
	})
}

// handleAdminSpaceUpdate handles PATCH /v1/admin/spaces/{space_id} — update space metadata.
func (s *Server) handleAdminSpaceUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Extract space_id from path: /v1/admin/spaces/{space_id}
	spaceID := strings.TrimPrefix(r.URL.Path, "/v1/admin/spaces/")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	var req models.AdminSpaceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.Prunable == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prunable field is required"})
		return
	}

	// Guard: protected spaces cannot be marked prunable
	if *req.Prunable && IsProtectedSpace(spaceID) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("space %q is protected and cannot be marked prunable", spaceID),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Check TapRoot exists, then update
	result, err := session.Run(ctx, `
		MATCH (t:TapRoot {space_id: $spaceId})
		SET t.prunable = $prunable
		RETURN t.space_id AS space_id, t.prunable AS prunable
	`, map[string]any{
		"spaceId":  spaceID,
		"prunable": *req.Prunable,
	})
	if err != nil {
		writeInternalError(w, err, "admin-space-update")
		return
	}

	if !result.Next(ctx) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("space %q not found", spaceID),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": spaceID,
		"prunable": *req.Prunable,
		"updated":  true,
	})
}

// handleAdminSpacePrune handles POST /v1/admin/spaces/prune — execute batch pruning.
func (s *Server) handleAdminSpacePrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req models.AdminSpacePruneRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
	}

	// Defaults
	if req.BatchSize <= 0 {
		req.BatchSize = 10000
	}
	if req.MaxSpaces <= 0 {
		req.MaxSpaces = 50
	}

	// Dry-run: query candidates with node counts and return without mutating
	if req.DryRun {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		result, err := session.Run(ctx, `
			CALL {
				MATCH (t:TapRoot) WHERE coalesce(t.prunable, false) = true
				RETURN t.space_id AS sid
				UNION
				MATCH (n:MemoryNode)
				WHERE n.space_id IS NOT NULL AND NOT EXISTS { MATCH (t:TapRoot {space_id: n.space_id}) }
				RETURN DISTINCT n.space_id AS sid
			}
			WITH DISTINCT sid
			OPTIONAL MATCH (n:MemoryNode {space_id: sid})
			WITH sid, count(n) AS node_count
			RETURN sid AS space_id, node_count
			ORDER BY sid
			LIMIT $maxSpaces
		`, map[string]any{"maxSpaces": req.MaxSpaces})
		if err != nil {
			writeInternalError(w, err, "admin-prune-list")
			return
		}

		resp := models.AdminSpacePruneResponse{
			DryRun:  true,
			Results: []models.SpacePruneResult{},
		}
		for result.Next(ctx) {
			rec := result.Record()
			sid, _ := rec.Get("space_id")
			nc, _ := rec.Get("node_count")
			spaceID := toString(sid)
			if IsProtectedSpace(spaceID) {
				continue
			}
			nodeCount := toInt(nc)
			resp.Results = append(resp.Results, models.SpacePruneResult{
				SpaceID:      spaceID,
				NodesDeleted: nodeCount,
				Status:       "dry_run",
			})
			resp.TotalNodesDeleted += nodeCount
		}
		resp.SpacesPruned = len(resp.Results)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Execute pruning via shared auto-prune logic
	pruned, totalDeleted, errCount := s.runAutoSpacePrune()
	resp := models.AdminSpacePruneResponse{
		DryRun:            false,
		Results:           []models.SpacePruneResult{},
		SpacesPruned:      pruned,
		TotalNodesDeleted: totalDeleted,
		SpacesSkipped:     errCount,
	}
	writeJSON(w, http.StatusOK, resp)
}

// runAutoSpacePrune discovers and prunes eligible spaces. Used by both the
// auto-prune scheduler and the non-dry-run path of handleAdminSpacePrune.
// Returns (spacesPruned, totalNodesDeleted, errorCount).
func (s *Server) runAutoSpacePrune() (int, int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})

	result, err := session.Run(ctx, `
		CALL {
			MATCH (t:TapRoot) WHERE coalesce(t.prunable, false) = true
			RETURN t.space_id AS sid
			UNION
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL AND NOT EXISTS { MATCH (t:TapRoot {space_id: n.space_id}) }
			RETURN DISTINCT n.space_id AS sid
		}
		WITH DISTINCT sid
		RETURN sid AS space_id
		ORDER BY sid
		LIMIT 50
	`, nil)
	if err != nil {
		session.Close(ctx)
		log.Printf("[auto-prune] candidate query failed: %v", err)
		return 0, 0, 1
	}

	var candidates []string
	for result.Next(ctx) {
		rec := result.Record()
		sid, _ := rec.Get("space_id")
		spaceID := toString(sid)
		if !IsProtectedSpace(spaceID) {
			candidates = append(candidates, spaceID)
		}
	}
	session.Close(ctx)

	if len(candidates) == 0 {
		return 0, 0, 0
	}

	spacesPruned, totalDeleted, errCount := 0, 0, 0
	for _, spaceID := range candidates {
		deleted, err := s.pruneSpace(ctx, spaceID, 10000)
		if err != nil {
			log.Printf("[auto-prune] space %s failed: %v", spaceID, err)
			errCount++
			continue
		}
		spacesPruned++
		totalDeleted += deleted
		log.Printf("[auto-prune] pruned space %s: %d nodes deleted", spaceID, deleted)
	}

	// Invalidate graph metrics cache so Grafana updates immediately
	if spacesPruned > 0 {
		s.graphMetricsCache.Lock()
		s.graphMetricsCache.data = nil
		s.graphMetricsCache.updated = time.Time{}
		s.graphMetricsCache.Unlock()
	}

	return spacesPruned, totalDeleted, errCount
}

// pruneSpace deletes all nodes (and their relationships) for a space in batches.
// Also removes the TapRoot node. Returns total nodes deleted.
func (s *Server) pruneSpace(ctx context.Context, spaceID string, batchSize int) (int, error) {
	// Defense-in-depth: never prune protected
	if IsProtectedSpace(spaceID) {
		return 0, fmt.Errorf("refusing to prune protected space %q", spaceID)
	}

	totalDeleted := 0

	writeSession := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer writeSession.Close(ctx)

	// Batch-delete ALL nodes with this space_id (MemoryNodes, Observations, etc.)
	for {
		result, err := writeSession.Run(ctx, `
			MATCH (n {space_id: $spaceId})
			WITH n LIMIT $batchSize
			DETACH DELETE n
			RETURN count(*) AS deleted
		`, map[string]any{
			"spaceId":   spaceID,
			"batchSize": batchSize,
		})
		if err != nil {
			return totalDeleted, fmt.Errorf("batch delete for %s: %w", spaceID, err)
		}

		if !result.Next(ctx) {
			break
		}
		rec := result.Record()
		deletedVal, _ := rec.Get("deleted")
		deleted := toInt(deletedVal)
		totalDeleted += deleted

		if deleted < batchSize {
			break
		}
	}

	// Delete the TapRoot itself
	_, err := writeSession.Run(ctx, `
		MATCH (t:TapRoot {space_id: $spaceId})
		DETACH DELETE t
	`, map[string]any{"spaceId": spaceID})
	if err != nil {
		return totalDeleted, fmt.Errorf("delete TapRoot for %s: %w", spaceID, err)
	}

	// Delete any RSICState nodes for this space
	_, err = writeSession.Run(ctx, `
		MATCH (s:RSICState {space_id: $spaceId})
		DETACH DELETE s
	`, map[string]any{"spaceId": spaceID})
	if err != nil {
		log.Printf("[admin] prune: failed to delete RSICState for %s: %v", spaceID, err)
	}

	return totalDeleted, nil
}

// --- helpers ---

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func toInt(v any) int {
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
