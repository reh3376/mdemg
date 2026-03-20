package api

import (
	"net/http"
)

// handleNeuralStatus handles GET /v1/neural/status
func (s *Server) handleNeuralStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sidecar_enabled":         s.cfg.SidecarEnabled,
		"neural_rerank_enabled":   s.cfg.NeuralRerankEnabled,
		"data_collection_enabled": s.cfg.NeuralDataCollection,
		"rerank_url":              s.cfg.NeuralRerankURL,
	})
}
