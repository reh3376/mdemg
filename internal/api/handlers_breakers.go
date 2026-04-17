package api

import (
	"encoding/json"
	"net/http"

	"mdemg/internal/circuitbreaker"
)

// BreakerStatus is the serializable view of a single circuit breaker.
type BreakerStatus struct {
	Name      string `json:"name"`
	State     string `json:"state"` // "closed", "open", "half-open"
	Failures  int    `json:"failures"`
	Successes int    `json:"successes"`
	Total     int64  `json:"total"`
}

// BreakersListResponse lists all registered circuit breakers and their states.
type BreakersListResponse struct {
	Breakers []BreakerStatus `json:"breakers"`
	Count    int             `json:"count"`
}

// BreakerResetRequest is the body of POST /v1/admin/breakers/reset.
type BreakerResetRequest struct {
	Name string `json:"name"`
}

// BreakerResetResponse summarizes the result of a reset call.
type BreakerResetResponse struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	WasState string `json:"was_state"` // pre-reset state
	NowState string `json:"now_state"` // post-reset state (always "closed")
}

// handleBreakersList implements GET /v1/admin/breakers.
// Returns the current state and counts for every registered breaker.
func (s *Server) handleBreakersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.cbRegistry == nil {
		writeJSON(w, http.StatusOK, BreakersListResponse{Breakers: []BreakerStatus{}, Count: 0})
		return
	}

	all := s.cbRegistry.All()
	out := make([]BreakerStatus, 0, len(all))
	for name, b := range all {
		fail, succ, total := b.Counts()
		out = append(out, BreakerStatus{
			Name:      name,
			State:     b.State().String(),
			Failures:  fail,
			Successes: succ,
			Total:     total,
		})
	}
	writeJSON(w, http.StatusOK, BreakersListResponse{Breakers: out, Count: len(out)})
}

// handleBreakersReset implements POST /v1/admin/breakers/reset.
// Forces a named breaker to StateClosed regardless of its current state.
// DH-004 E4.3: operator escape hatch for breakers that have tripped open
// after a transient incident but haven't auto-recovered yet.
func (s *Server) handleBreakersReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.cbRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "breaker registry not configured"})
		return
	}

	var req BreakerResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	all := s.cbRegistry.All()
	b, ok := all[req.Name]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":     "unknown breaker",
			"name":      req.Name,
			"available": namesOf(all),
		})
		return
	}

	wasState := b.State().String()
	b.Reset()
	writeJSON(w, http.StatusOK, BreakerResetResponse{
		Name:     req.Name,
		OK:       true,
		WasState: wasState,
		NowState: b.State().String(),
	})
}

func namesOf(m map[string]*circuitbreaker.Breaker) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
