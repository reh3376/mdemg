package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/alert"
)

// grafanaWebhookPayload represents Grafana's alert webhook format (v10+).
type grafanaWebhookPayload struct {
	Alerts []grafanaAlert `json:"alerts"`
}

type grafanaAlert struct {
	Status      string            `json:"status"` // "firing" or "resolved"
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

// handleGrafanaAlertWebhook receives Grafana alert webhook notifications and
// dispatches them through the alert subsystem for file-based user delivery.
func (s *Server) handleGrafanaAlertWebhook(w http.ResponseWriter, r *http.Request) {
	if s.alertDispatcher == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "alert dispatcher not configured"})
		return
	}

	var payload grafanaWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	for _, ga := range payload.Alerts {
		if ga.Status != "firing" {
			continue
		}

		sev := mapGrafanaSeverity(ga.Labels["severity"])
		title := ga.Labels["alertname"]
		if title == "" {
			title = "Grafana Alert"
		}
		message := ga.Annotations["summary"]
		if message == "" {
			message = ga.Annotations["description"]
		}

		s.alertDispatcher.Send(r.Context(), alert.Alert{
			Service:  "grafana",
			Severity: sev,
			Title:    title,
			Message:  message,
		})
	}

	slog.Debug("grafana webhook: processed", "alert_count", len(payload.Alerts))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func mapGrafanaSeverity(label string) alert.Severity {
	switch strings.ToLower(label) {
	case "critical":
		return alert.SeverityCritical
	case "high", "error":
		return alert.SeverityHigh
	case "medium", "warning":
		return alert.SeverityMedium
	default:
		return alert.SeverityLow
	}
}

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
