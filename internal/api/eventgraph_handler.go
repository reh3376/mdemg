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
	"time"

	"mdemg/internal/eventgraph"
)

// ReinforcementNeighborhoodRequest is the body of POST
// /v1/eventgraph/reinforcement-neighborhood. All fields except space_id +
// seed_node_id have server-side defaults (cfg.EventGraphFederationDefault*
// + cfg.EventGraphMaxEventsPerQuery).
type ReinforcementNeighborhoodRequest struct {
	SpaceID      string `json:"space_id"`
	SeedNodeID   string `json:"seed_node_id"`
	Hops         *int   `json:"hops,omitempty"`         // nil → server default
	SinceSeconds *int64 `json:"since_seconds,omitempty"` // nil → server default (lookback hours × 3600)
	Limit        *int   `json:"limit,omitempty"`         // nil → server default
}

// handleEventgraphReinforcementNeighborhood implements POST
// /v1/eventgraph/reinforcement-neighborhood. Returns the federation result
// (events + neighborhood IDs + truncation flag).
func (s *Server) handleEventgraphReinforcementNeighborhood(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.cfg.EventGraphEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":  "eventgraph disabled",
			"reason": "EVENTGRAPH_ENABLED=false; set to true to enable",
		})
		return
	}
	if s.eventgraphService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "eventgraph service not initialized (TSDB unavailable at boot?)",
		})
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

	// Apply defaults / clamps from config.
	hops := s.cfg.EventGraphFederationDefaultHops
	if req.Hops != nil {
		hops = *req.Hops
	}
	if hops < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hops must be ≥ 0"})
		return
	}
	// Cap hops at a reasonable max to prevent runaway Cypher walks. Use 2×
	// the configured default as the ceiling — operators can raise the
	// default itself if they need deeper walks.
	if maxHops := s.cfg.EventGraphFederationDefaultHops * 2; maxHops > 0 && hops > maxHops {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "hops exceeds server ceiling",
			"ceiling": "2 × EVENTGRAPH_FEDERATION_DEFAULT_HOPS",
		})
		return
	}

	since := time.Duration(s.cfg.EventGraphFederationDefaultLookbackHours) * time.Hour
	if req.SinceSeconds != nil {
		since = time.Duration(*req.SinceSeconds) * time.Second
	}

	limit := s.cfg.EventGraphMaxEventsPerQuery
	if req.Limit != nil && *req.Limit > 0 && *req.Limit < s.cfg.EventGraphMaxEventsPerQuery {
		limit = *req.Limit
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
