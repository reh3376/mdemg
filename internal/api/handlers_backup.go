package api

import (
	"log/slog"
	"net/http"
	"strings"

	"mdemg/internal/backup"
	"mdemg/internal/jobs"
)

// handleBackupTrigger routes POST /v1/backup/trigger.
func (s *Server) handleBackupTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	var req backup.TriggerRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type required (full or partial_space)"})
		return
	}

	backupID, err := s.backupSvc.Trigger(r.Context(), req)
	if err != nil {
		slog.Error("trigger backup failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup trigger failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"backup_id": backupID,
		"status":    "pending",
		"message":   "backup triggered",
	})
}

// handleBackupStatus routes GET /v1/backup/status/{id}.
func (s *Server) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	backupID := strings.TrimPrefix(r.URL.Path, "/v1/backup/status/")
	if backupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup_id required"})
		return
	}

	// Check the job queue first (for in-progress/recent jobs).
	q := jobs.GetQueue()
	if job, ok := q.GetJob(backupID); ok {
		snap := job.GetSnapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"backup_id": snap.ID,
			"status":    string(snap.Status),
			"progress":  snap.Progress,
			"result":    snap.Result,
			"error":     snap.Error,
		})
		return
	}

	// Fall back to manifest on disk.
	m, err := s.backupSvc.GetBackup(backupID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "backup not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backup_id": m.BackupID,
		"status":    "completed",
		"type":      m.Type,
		"checksum":  m.Checksum,
		"size":      m.SizeBytes,
		"created":   m.CreatedAt,
	})
}

// handleBackupList routes GET /v1/backup/list.
func (s *Server) handleBackupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	typeFilter := r.URL.Query().Get("type")
	manifests, err := s.backupSvc.ListBackups(typeFilter, 0)
	if err != nil {
		writeInternalError(w, err, "list backups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backups": manifests,
		"count":   len(manifests),
	})
}

// handleBackupManifest routes GET /v1/backup/manifest/{id}.
func (s *Server) handleBackupManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	backupID := strings.TrimPrefix(r.URL.Path, "/v1/backup/manifest/")
	if backupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup_id required"})
		return
	}

	m, err := s.backupSvc.GetBackup(backupID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "manifest not found"})
		return
	}

	writeJSON(w, http.StatusOK, m)
}

// handleBackupByID routes DELETE /v1/backup/{id}.
func (s *Server) handleBackupByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	// Extract ID: path is /v1/backup/{id}
	backupID := strings.TrimPrefix(r.URL.Path, "/v1/backup/")
	backupID = strings.TrimSuffix(backupID, "/")
	if backupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup_id required"})
		return
	}

	if err := s.backupSvc.DeleteBackup(backupID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "backup not found"})
		} else if strings.Contains(err.Error(), "keep_forever") {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "backup is protected (keep_forever)"})
		} else {
			writeInternalError(w, err, "delete backup")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "backup deleted",
		"backup_id": backupID,
	})
}

// handleBackupRestore routes POST /v1/backup/restore.
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	var req backup.RestoreRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.BackupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup_id required"})
		return
	}

	restoreID, err := s.backupSvc.Restore(r.Context(), req)
	if err != nil {
		slog.Error("restore backup failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "backup restore failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"restore_id": restoreID,
		"backup_id":  req.BackupID,
		"status":     "pending",
		"message":    "restore triggered",
	})
}

// handleRestoreStatus routes GET /v1/backup/restore/status/{id}.
func (s *Server) handleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.backupSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "backup not enabled"})
		return
	}

	restoreID := strings.TrimPrefix(r.URL.Path, "/v1/backup/restore/status/")
	if restoreID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "restore_id required"})
		return
	}

	q := jobs.GetQueue()
	job, ok := q.GetJob(restoreID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "restore job not found"})
		return
	}

	snap := job.GetSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"restore_id": snap.ID,
		"status":     string(snap.Status),
		"progress":   snap.Progress,
		"result":     snap.Result,
		"error":      snap.Error,
	})
}
