package api

import (
	"net/http"
)

// handleConstraintEffectiveness handles GET /v1/constraints/effectiveness
func (s *Server) handleConstraintEffectiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	if s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	results, err := s.jiminySvc.GetConstraintEffectiveness(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "constraint effectiveness")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"constraints": results,
		"total":       len(results),
	})
}
