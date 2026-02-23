package api

import (
	"net/http"
	"os"
	"path/filepath"

	"mdemg/internal/filewatcher"
)

// handleFileWatcherStart handles POST /v1/filewatcher/start — starts a file watcher for a space.
func (s *Server) handleFileWatcherStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req struct {
		SpaceID    string   `json:"space_id"`
		Path       string   `json:"path"`
		Extensions []string `json:"extensions,omitempty"`
		Excludes   []string `json:"excludes,omitempty"`
		DebounceMs int      `json:"debounce_ms,omitempty"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id required"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path required"})
		return
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
		return
	}

	// Validate path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path does not exist: " + absPath})
		return
	}
	if !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is not a directory: " + absPath})
		return
	}

	cfg := filewatcher.Config{
		SpaceID:    req.SpaceID,
		Path:       absPath,
		Extensions: req.Extensions,
		Excludes:   req.Excludes,
		DebounceMs: req.DebounceMs,
		OnChange:   s.handleFileWatcherChange,
	}

	// Apply defaults
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = filewatcher.DefaultExtensions
	}
	if len(cfg.Excludes) == 0 {
		cfg.Excludes = filewatcher.DefaultExcludes
	}
	if cfg.DebounceMs <= 0 {
		cfg.DebounceMs = 500
	}

	if err := s.fileWatcherMgr.AddWatcher(cfg); err != nil {
		writeInternalError(w, err, "start file watcher")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": req.SpaceID,
		"path":     absPath,
		"status":   "watching",
	})
}

// handleFileWatcherStatus handles GET /v1/filewatcher/status — lists all active file watchers.
func (s *Server) handleFileWatcherStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	watchers := s.fileWatcherMgr.GetStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"watchers": watchers,
		"count":    len(watchers),
	})
}

// handleFileWatcherStop handles POST /v1/filewatcher/stop — stops a file watcher for a space.
func (s *Server) handleFileWatcherStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id required"})
		return
	}

	s.fileWatcherMgr.RemoveWatcher(req.SpaceID)

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": req.SpaceID,
		"status":   "stopped",
	})
}
