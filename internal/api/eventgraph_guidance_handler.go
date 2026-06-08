// Sprint EVENTGRAPH-002 Epic 3 — guidance-outcome federation API handler.
//
// POST /v1/eventgraph/guidance-outcome-neighborhood orchestrates a graph walk
// from a seed constraint + a TSDB query for guidance outcomes (constraint_outcomes)
// whose constraint_code is present in the neighborhood. Same auth + gating
// convention as the reinforcement endpoint.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"mdemg/internal/eventgraph"
)

// GuidanceOutcomeNeighborhoodRequest is the body of POST
// /v1/eventgraph/guidance-outcome-neighborhood. Same optional-field convention
// as the reinforcement endpoint — nil fields fall back to the shared
// EventGraphFederationDefault* / EventGraphMaxEventsPerQuery config.
type GuidanceOutcomeNeighborhoodRequest struct {
	SpaceID      string `json:"space_id"`
	SeedNodeID   string `json:"seed_node_id"`
	Hops         *int   `json:"hops,omitempty"`
	SinceSeconds *int64 `json:"since_seconds,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
}

// eventgraphGate enforces the method/enabled/service preconditions shared by all
// federation endpoints. Returns false (after writing the error response) when the
// request should not proceed. Single source for the gate — both the reinforcement
// and guidance-outcome handlers call it.
func (s *Server) eventgraphGate(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return false
	}
	if !s.cfg.EventGraphEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":  "eventgraph disabled",
			"reason": "EVENTGRAPH_ENABLED=false; set to true to enable",
		})
		return false
	}
	if s.eventgraphService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "eventgraph service not initialized (TSDB unavailable at boot?)",
		})
		return false
	}
	return true
}

// resolveFederationDefaults applies config defaults + validation for the shared
// hops/since/limit federation parameters. Returns the resolved values and true on
// success; writes a 400 and returns false on invalid hops. Single source for the
// default-application logic so there is exactly one place to change the rules.
func (s *Server) resolveFederationDefaults(w http.ResponseWriter, hopsP *int, sinceP *int64, limitP *int) (hops int, since time.Duration, limit int, ok bool) {
	hops = s.cfg.EventGraphFederationDefaultHops
	if hopsP != nil {
		hops = *hopsP
	}
	if hops < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hops must be ≥ 0"})
		return 0, 0, 0, false
	}
	// Cap hops at 2× the configured default as the ceiling — operators can raise
	// the default itself if they need deeper walks.
	if maxHops := s.cfg.EventGraphFederationDefaultHops * 2; maxHops > 0 && hops > maxHops {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "hops exceeds server ceiling",
			"ceiling": "2 × EVENTGRAPH_FEDERATION_DEFAULT_HOPS",
		})
		return 0, 0, 0, false
	}

	since = time.Duration(s.cfg.EventGraphFederationDefaultLookbackHours) * time.Hour
	if sinceP != nil {
		since = time.Duration(*sinceP) * time.Second
	}

	limit = s.cfg.EventGraphMaxEventsPerQuery
	if limitP != nil && *limitP > 0 && *limitP < s.cfg.EventGraphMaxEventsPerQuery {
		limit = *limitP
	}
	return hops, since, limit, true
}

// handleEventgraphGuidanceOutcomeNeighborhood implements POST
// /v1/eventgraph/guidance-outcome-neighborhood. Returns the federation result
// (guidance outcomes + neighborhood node IDs + constraint codes + truncation).
func (s *Server) handleEventgraphGuidanceOutcomeNeighborhood(w http.ResponseWriter, r *http.Request) {
	if !s.eventgraphGate(w, r) {
		return
	}

	var req GuidanceOutcomeNeighborhoodRequest
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

	result, err := s.eventgraphService.GuidanceOutcomesInNeighborhood(r.Context(), eventgraph.GuidanceOutcomeRequest{
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
