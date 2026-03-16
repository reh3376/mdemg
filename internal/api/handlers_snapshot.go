package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mdemg/internal/conversation"
)

// handleSnapshots handles /v1/conversation/snapshot
func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSnapshots(w, r)
	case http.MethodPost:
		s.handleCreateSnapshot(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method not allowed"})
	}
}

// handleSnapshotByID handles /v1/conversation/snapshot/{id}
func (s *Server) handleSnapshotByID(w http.ResponseWriter, r *http.Request) {
	// Extract snapshot ID from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/conversation/snapshot/")
	snapshotID := strings.TrimSuffix(path, "/")

	if snapshotID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "snapshot_id is required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetSnapshot(w, r, snapshotID)
	case http.MethodDelete:
		s.handleDeleteSnapshot(w, r, snapshotID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method not allowed"})
	}
}

// handleListSnapshots lists snapshots for a space
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	sessionID := r.URL.Query().Get("session_id") // Optional filter

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	snapshots, err := s.snapshotService.ListSnapshots(r.Context(), spaceID, sessionID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to list snapshots: " + err.Error()})
		return
	}

	response := conversation.ListSnapshotsResponse{
		Snapshots: make([]conversation.SnapshotResponse, 0, len(snapshots)),
		Count:     len(snapshots),
	}

	for _, snap := range snapshots {
		response.Snapshots = append(response.Snapshots, snap.ToResponse())
	}

	writeJSON(w, http.StatusOK, response)
}

// handleCreateSnapshot creates a new snapshot
func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req conversation.CreateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body: " + err.Error()})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id is required"})
		return
	}

	trigger := conversation.TriggerManual
	if req.Trigger != "" {
		switch req.Trigger {
		case "manual":
			trigger = conversation.TriggerManual
		case "compaction":
			trigger = conversation.TriggerCompaction
		case "session_end":
			trigger = conversation.TriggerSessionEnd
		case "error":
			trigger = conversation.TriggerError
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid trigger: must be manual, compaction, session_end, or error"})
			return
		}
	}

	snapshot := &conversation.TaskSnapshot{
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Trigger:   trigger,
		Context:   req.Context,
	}

	if err := s.snapshotService.CreateSnapshot(r.Context(), snapshot); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to create snapshot: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, snapshot.ToResponse())
}

// handleGetSnapshot retrieves a snapshot by ID
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request, snapshotID string) {
	snapshot, err := s.snapshotService.GetSnapshot(r.Context(), snapshotID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to get snapshot: " + err.Error()})
		return
	}
	if snapshot == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Snapshot not found"})
		return
	}

	writeJSON(w, http.StatusOK, snapshot.ToResponse())
}

// handleDeleteSnapshot deletes a snapshot
func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request, snapshotID string) {
	if err := s.snapshotService.DeleteSnapshot(r.Context(), snapshotID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "Snapshot not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to delete snapshot: " + err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleLatestSnapshot handles GET /v1/conversation/snapshot/latest
func (s *Server) handleLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	sessionID := r.URL.Query().Get("session_id") // Optional

	snapshot, err := s.snapshotService.GetLatestSnapshot(r.Context(), spaceID, sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to get latest snapshot: " + err.Error()})
		return
	}
	if snapshot == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "No snapshots found"})
		return
	}

	writeJSON(w, http.StatusOK, snapshot.ToResponse())
}

// handleCleanupSnapshots handles POST /v1/conversation/snapshot/cleanup
func (s *Server) handleCleanupSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method not allowed"})
		return
	}

	var req struct {
		SpaceID       string `json:"space_id"`
		RetentionDays int    `json:"retention_days,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body: " + err.Error()})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	deleted, err := s.snapshotService.CleanupOldSnapshots(r.Context(), req.SpaceID, req.RetentionDays)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to cleanup snapshots: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":        deleted,
		"retention_days": req.RetentionDays,
	})
}
