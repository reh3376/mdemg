package api

import (
	"net/http"
	"strconv"
	"time"
)

// enforcementEvent represents a logged guardrail enforcement event.
type enforcementEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	SpaceID      string    `json:"space_id"`
	Status       string    `json:"status"` // "Block", "Warning", "Pass"
	FilesChanged []string  `json:"files_changed"`
	Violations   int       `json:"violation_count"`
	Warnings     int       `json:"warning_count"`
	LatencyMs    float64   `json:"latency_ms"`
}

// enforcementEventLog is an in-memory ring buffer of recent enforcement events.
type enforcementEventLog struct {
	events []enforcementEvent
	maxLen int
}

func newEnforcementEventLog(maxLen int) *enforcementEventLog {
	if maxLen <= 0 {
		maxLen = 1000
	}
	return &enforcementEventLog{
		events: make([]enforcementEvent, 0, maxLen),
		maxLen: maxLen,
	}
}

func (l *enforcementEventLog) Add(e enforcementEvent) {
	if len(l.events) >= l.maxLen {
		// Shift out oldest
		l.events = l.events[1:]
	}
	l.events = append(l.events, e)
}

func (l *enforcementEventLog) Recent(n int) []enforcementEvent {
	if n <= 0 || n > len(l.events) {
		n = len(l.events)
	}
	start := len(l.events) - n
	if start < 0 {
		start = 0
	}
	result := make([]enforcementEvent, n)
	copy(result, l.events[start:])
	return result
}

// handleGuardrailEvents handles GET /v1/guardrail/events
// Returns recent enforcement events from the in-memory log.
func (s *Server) handleGuardrailEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Parse limit parameter (CONFIG-DEADFLAG-001: PAGINATION_DEF_LIMIT,
	// default 50 — the literal previously always won)
	limit := s.cfg.PaginationDefLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if s.enforcementLog == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"events": []enforcementEvent{},
			"total":  0,
		})
		return
	}

	events := s.enforcementLog.Recent(limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  len(events),
	})
}
