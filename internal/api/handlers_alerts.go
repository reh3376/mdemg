package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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
