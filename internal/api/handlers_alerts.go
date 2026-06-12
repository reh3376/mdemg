package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// The Grafana alert webhook (POST /v1/alerts/grafana) was pruned in
// DORMANT-CENSUS-001: superseded by the server-native alert evaluator
// (SR-001/SNA-001); its only remaining reference was a commented-out
// backward-compat contactpoint. Alerting does not require Grafana.

// clearAlertsRequest is the body for POST /v1/alerts/clear (HOOKSYNC-001).
// Provide ids and/or all_before (RFC3339). Cleared = delivered to the
// operator, not resolved — persisting conditions re-fire via the evaluator.
type clearAlertsRequest struct {
	IDs       []string `json:"ids,omitempty"`
	AllBefore string   `json:"all_before,omitempty"`
}

// handleAlertsClear processes POST /v1/alerts/clear. Marks file-backend
// alerts cleared so hooks stop re-rendering delivered entries.
func (s *Server) handleAlertsClear(w http.ResponseWriter, r *http.Request) {
	var req clearAlertsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	var before time.Time
	if req.AllBefore != "" {
		t, err := time.Parse(time.RFC3339, req.AllBefore)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "all_before must be RFC3339"})
			return
		}
		before = t
	}
	if len(req.IDs) == 0 && before.IsZero() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide ids or all_before"})
		return
	}

	if s.alertDispatcher == nil {
		writeJSON(w, http.StatusOK, map[string]int{"cleared": 0})
		return
	}
	n, err := s.alertDispatcher.ClearAlerts(req.IDs, before)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	slog.Debug("alerts cleared", "count", n, "by_ids", len(req.IDs), "all_before", req.AllBefore)
	writeJSON(w, http.StatusOK, map[string]int{"cleared": n})
}
