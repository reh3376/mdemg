package api

import (
	"encoding/json"
	"net/http"
)

// handleStaleEdgeStats returns statistics about stale edges in a space.
// GET /v1/memory/edges/stale/stats?space_id=xxx
func (s *Server) handleStaleEdgeStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	stats, err := s.retriever.GetStaleEdgeStats(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "stale edge stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleRefreshStaleEdges triggers a refresh of stale edges in a space.
// POST /v1/memory/edges/stale/refresh
// Body: {"space_id": "xxx"}
func (s *Server) handleRefreshStaleEdges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	refreshed, err := s.retriever.RefreshAllStaleEdges(r.Context(), req.SpaceID)
	if err != nil {
		writeInternalError(w, err, "refresh stale edges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":        req.SpaceID,
		"edges_refreshed": refreshed,
	})
}
