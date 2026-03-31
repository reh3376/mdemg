package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"mdemg/internal/config"
)

// handleAdminConfig serves GET and PATCH /v1/admin/config.
// GET returns the effective configuration with source attribution.
// PATCH updates YAML config values.
func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		yamlPath := config.FindConfigFile()
		sources := config.EffectiveConfig(yamlPath)
		writeJSON(w, http.StatusOK, map[string]any{
			"config":    sources,
			"yaml_path": yamlPath,
		})
	case http.MethodPatch:
		var body struct {
			Updates map[string]string `json:"updates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		if len(body.Updates) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no updates provided"})
			return
		}
		yamlPath := config.FindConfigFile()
		if yamlPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no YAML config file found"})
			return
		}
		// Reject updates to env-sourced fields.
		sources := config.EffectiveConfig(yamlPath)
		for key := range body.Updates {
			for _, src := range sources {
				if src.Key == key && src.Source == "env" {
					writeJSON(w, http.StatusBadRequest, map[string]string{
						"error": "key '" + key + "' is set via environment variable and cannot be changed in YAML",
					})
					return
				}
			}
		}
		errs, err := config.UpdateYAMLConfig(yamlPath, body.Updates)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if len(errs) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
			return
		}
		// Return updated config.
		newSources := config.EffectiveConfig(yamlPath)
		writeJSON(w, http.StatusOK, map[string]any{
			"config":    newSources,
			"yaml_path": yamlPath,
			"updated":   len(body.Updates),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
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

// handleRSICStart serves POST /v1/admin/rsic/start.
func (s *Server) handleRSICStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.rsicWatchdog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RSIC watchdog not initialized"})
		return
	}
	if s.rsicWatchdog.Running() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}
	s.rsicWatchdog.Restart()
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// handleRSICStop serves POST /v1/admin/rsic/stop.
func (s *Server) handleRSICStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.rsicWatchdog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RSIC watchdog not initialized"})
		return
	}
	if !s.rsicWatchdog.Running() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_stopped"})
		return
	}
	s.rsicWatchdog.Stop()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleRSICRestart serves POST /v1/admin/rsic/restart.
func (s *Server) handleRSICRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.rsicWatchdog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RSIC watchdog not initialized"})
		return
	}
	s.rsicWatchdog.Restart()
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// handleServerRestart serves POST /v1/admin/restart.
// Triggers a graceful server restart via re-exec.
func (s *Server) handleServerRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})

	go func() {
		time.Sleep(500 * time.Millisecond)
		binary, err := exec.LookPath(os.Args[0])
		if err != nil {
			binary = os.Args[0]
		}
		_ = execRestart(binary, os.Args, os.Environ())
	}()
}
