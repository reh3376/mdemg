package api

import (
	"net/http"
	"strings"

	"mdemg/internal/symbols"
)

// handleRelationshipStats handles GET /v1/symbols/relationships?space_id=X
// Returns counts of relationships by type for a space.
func (s *Server) handleRelationshipStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id required"})
		return
	}

	stats, err := s.symbolStore.RelationshipStats(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "relationship stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": spaceID,
		"counts":   stats,
	})
}

// handleSymbolRelationships handles GET /v1/symbols/{id}/relationships
// Returns incoming and outgoing relationships for a specific symbol.
func (s *Server) handleSymbolRelationships(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Extract symbol ID from path: /v1/symbols/{id}/relationships
	path := strings.TrimPrefix(r.URL.Path, "/v1/symbols/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "relationships" || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path, expected /v1/symbols/{id}/relationships"})
		return
	}
	symbolID := parts[0]

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id required"})
		return
	}

	rels, err := s.symbolStore.QueryRelationships(r.Context(), spaceID, symbolID)
	if err != nil {
		writeInternalError(w, err, "query relationships")
		return
	}

	// Ensure JSON encodes as [] not null for empty results
	if rels == nil {
		rels = []symbols.RelationshipRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":      spaceID,
		"symbol_id":     symbolID,
		"relationships": rels,
		"count":         len(rels),
	})
}
