package api

import (
	"net/http"
	"strconv"

	"mdemg/internal/config"
)

// handleAdminConfig serves GET /v1/admin/config.
// Returns the effective configuration with source attribution.
func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	yamlPath := config.FindConfigFile()
	sources := config.EffectiveConfig(yamlPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"config":    sources,
		"yaml_path": yamlPath,
	})
}

// handleAdminLogs serves GET /v1/admin/logs.
// Returns recent log entries from the in-process ring buffer.
// Query params: limit (default 200, max 500).
// Level filtering and text search are client-side.
func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.logBuffer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "log buffer not initialized"})
		return
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	entries := s.logBuffer.Entries(limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
		"seq":     s.logBuffer.Seq(),
	})
}
