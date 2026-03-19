package api

import (
	"net/http"

	"mdemg/internal/metrics"
)

// handleDeterminismScore handles GET /v1/metrics/determinism
func (s *Server) handleDeterminismScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if !s.cfg.DeterminismScoringEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "determinism scoring is not enabled (set DETERMINISM_SCORING_ENABLED=true)",
		})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	score, err := metrics.ComputeDeterminismScore(r.Context(), s.driver, spaceID)
	if err != nil {
		writeInternalError(w, err, "determinism score")
		return
	}

	writeJSON(w, http.StatusOK, score)
}
