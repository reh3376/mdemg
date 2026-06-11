package api

import (
	"net/http"

	"mdemg/internal/models"
)

// handleMetaLearn handles POST /v1/memory/meta-learn
// Promotes high-value L4/L5 concepts from a source space to the global space.
func (s *Server) handleMetaLearn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.metaLearnSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "meta-learning is not enabled",
		})
		return
	}

	if s.embedder == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "embedder required for meta-learning",
		})
		return
	}

	var req models.MetaLearnRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}
	// CONFIG-DEADFLAG-001: METALEARN_MIN_LAYER / METALEARN_MIN_UPDATE_COUNT
	// were parsed but the service fell back to literals (4 / 5) when the
	// request omitted them. Config now supplies the defaults; explicit
	// request values still win.
	if req.MinLayer <= 0 {
		req.MinLayer = s.cfg.MetaLearnMinLayer
	}
	if req.MinUpdateCount <= 0 {
		req.MinUpdateCount = s.cfg.MetaLearnMinUpdateCount
	}

	resp, err := s.metaLearnSvc.Promote(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "meta-learn promote")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}
