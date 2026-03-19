package api

import (
	"net/http"
	"strconv"
	"strings"
)

// handleDetectConstraintConflicts handles POST /v1/constraints/detect-conflicts
func (s *Server) handleDetectConstraintConflicts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	if s.conflictDetector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "constraint conflict detection is not enabled (set CONSTRAINT_CONFLICT_DETECTION_ENABLED=true)",
		})
		return
	}

	count, err := s.conflictDetector.DetectConflicts(r.Context(), req.SpaceID)
	if err != nil {
		writeInternalError(w, err, "detect conflicts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"new_conflicts": count,
	})
}

// handleListConstraintConflicts handles GET /v1/constraints/conflicts
func (s *Server) handleListConstraintConflicts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	if s.conflictDetector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "constraint conflict detection is not enabled (set CONSTRAINT_CONFLICT_DETECTION_ENABLED=true)",
		})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	conflicts, err := s.conflictDetector.ListConflicts(r.Context(), spaceID, limit)
	if err != nil {
		writeInternalError(w, err, "list conflicts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conflicts": conflicts,
		"total":     len(conflicts),
	})
}

// handleResolveConstraintConflict handles PATCH /v1/constraints/conflicts/{id}/resolve
func (s *Server) handleResolveConstraintConflict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.conflictDetector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "constraint conflict detection is not enabled (set CONSTRAINT_CONFLICT_DETECTION_ENABLED=true)",
		})
		return
	}

	// Extract conflict ID from URL path: /v1/constraints/conflicts/{id}/resolve
	// id format is "source_node_id:target_node_id"
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/v1/constraints/conflicts/")
	path = strings.TrimSuffix(path, "/resolve")
	parts := strings.SplitN(path, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid conflict id format, expected source_node_id:target_node_id"})
		return
	}

	var req struct {
		ResolutionText string `json:"resolution_text"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	err := s.conflictDetector.ResolveConflict(r.Context(), parts[0], parts[1], req.ResolutionText)
	if err != nil {
		writeInternalError(w, err, "resolve conflict")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "resolved",
	})
}
