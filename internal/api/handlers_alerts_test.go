package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mdemg/internal/alert"
)

func TestHandleGrafanaAlertWebhook(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/alerts.json"
	disp := alert.NewDispatcher(alert.Config{
		Enabled:       true,
		CooldownSec:   0,
		AlertFilePath: filePath,
		MaxAlerts:     50,
	})

	s := &Server{alertDispatcher: disp}

	payload := grafanaWebhookPayload{
		Alerts: []grafanaAlert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "Neo4j Probe Down",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "Neo4j health probe reports database down",
				},
			},
			{
				Status: "resolved", // should be skipped
				Labels: map[string]string{
					"alertname": "Resolved Alert",
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/v1/alerts/grafana", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleGrafanaAlertWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Dispatcher fans out to backends in goroutines — wait briefly for async delivery
	time.Sleep(200 * time.Millisecond)

	af := alert.NewFileBackend(filePath, 50).ReadAlerts()
	if len(af.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(af.Alerts))
	}
	if af.Alerts[0].Service != "grafana" {
		t.Errorf("expected service 'grafana', got %q", af.Alerts[0].Service)
	}
	if af.Alerts[0].Severity != alert.SeverityCritical {
		t.Errorf("expected severity critical, got %q", af.Alerts[0].Severity)
	}
	if af.Alerts[0].Title != "Neo4j Probe Down" {
		t.Errorf("expected title 'Neo4j Probe Down', got %q", af.Alerts[0].Title)
	}
}

func TestHandleGrafanaAlertWebhook_InvalidJSON(t *testing.T) {
	s := &Server{alertDispatcher: alert.NewDispatcher(alert.Config{Enabled: true})}

	req := httptest.NewRequest("POST", "/v1/alerts/grafana", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	s.handleGrafanaAlertWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleGrafanaAlertWebhook_NoDispatcher(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest("POST", "/v1/alerts/grafana", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	s.handleGrafanaAlertWebhook(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestMapGrafanaSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  alert.Severity
	}{
		{"critical", alert.SeverityCritical},
		{"high", alert.SeverityHigh},
		{"error", alert.SeverityHigh},
		{"warning", alert.SeverityMedium},
		{"medium", alert.SeverityMedium},
		{"info", alert.SeverityLow},
		{"", alert.SeverityLow},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapGrafanaSeverity(tt.input); got != tt.want {
				t.Errorf("mapGrafanaSeverity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
