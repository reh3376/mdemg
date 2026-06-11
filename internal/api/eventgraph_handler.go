// Sprint EVENTGRAPH-001 Epic 5 — federation API handler.
//
// POST /v1/eventgraph/reinforcement-neighborhood orchestrates a graph walk
// from a seed node + a TSDB query for reinforcement events touching the
// resulting neighborhood. Same auth convention as /v1/admin/breakers:
// gated when AUTH_API_KEYS is set, permissive when not.
package api

import (
	"encoding/json"
	"net/http"

	"mdemg/internal/eventgraph"
)

// ReinforcementNeighborhoodRequest is the body of POST
// /v1/eventgraph/reinforcement-neighborhood. All fields except space_id +
// seed_node_id have server-side defaults (cfg.EventGraphFederationDefault*
// + cfg.EventGraphMaxEventsPerQuery).
type ReinforcementNeighborhoodRequest struct {
	SpaceID      string `json:"space_id"`
	SeedNodeID   string `json:"seed_node_id"`
	Hops         *int   `json:"hops,omitempty"`          // nil → server default
	SinceSeconds *int64 `json:"since_seconds,omitempty"` // nil → server default (lookback hours × 3600)
	Limit        *int   `json:"limit,omitempty"`         // nil → server default
}

// handleEventgraphReinforcementNeighborhood implements POST
// /v1/eventgraph/reinforcement-neighborhood. Returns the federation result
// (events + neighborhood IDs + truncation flag).
func (s *Server) handleEventgraphReinforcementNeighborhood(w http.ResponseWriter, r *http.Request) {
	if !s.eventgraphGate(w, r) {
		return
	}

	var req ReinforcementNeighborhoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}
	if req.SeedNodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seed_node_id is required"})
		return
	}

	hops, since, limit, ok := s.resolveFederationDefaults(w, req.Hops, req.SinceSeconds, req.Limit)
	if !ok {
		return
	}

	result, err := s.eventgraphService.EventsInGraphNeighborhood(r.Context(), eventgraph.FederationRequest{
		SpaceID:    req.SpaceID,
		SeedNodeID: req.SeedNodeID,
		Hops:       hops,
		Since:      since,
		Limit:      limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "federation query failed",
			"detail": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
