package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mdemg/internal/jobs"
	"mdemg/internal/tsdb"
)

type trainingDataExportRequest struct {
	SpaceID    string   `json:"space_id"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Tables     []string `json:"tables"`
	InstanceID string   `json:"instance_id"`
}

// handleTrainingDataExport triggers an async training data export.
// POST /v1/training-data/export
func (s *Server) handleTrainingDataExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.tsdbClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "TSDB not configured"})
		return
	}

	var req trainingDataExportRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id required"})
		return
	}

	// Parse time range
	var fromTime, toTime time.Time
	var err error
	if req.From != "" {
		fromTime, err = time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid from: %v", err)})
			return
		}
	} else {
		fromTime = time.Now().AddDate(0, -6, 0)
	}
	if req.To != "" {
		toTime, err = time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid to: %v", err)})
			return
		}
	} else {
		toTime = time.Now().UTC()
	}

	if len(req.Tables) == 0 {
		req.Tables = []string{"llm_interactions", "retrieval_events", "embedding_events"}
	}

	if req.InstanceID == "" {
		req.InstanceID = "api-" + req.SpaceID
	}

	now := time.Now().UTC()
	exportID := fmt.Sprintf("exp-%s-%s", req.InstanceID, now.Format("20060102-150405"))

	outputDir := filepath.Join(os.TempDir(), "mdemg-exports")
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		writeInternalError(w, err, "create export dir")
		return
	}
	outputPath := filepath.Join(outputDir, exportID+".tar.gz")

	cfg := tsdb.ExportConfig{
		SpaceID:    req.SpaceID,
		From:       fromTime,
		To:         toTime,
		Tables:     req.Tables,
		InstanceID: req.InstanceID,
		OutputPath: outputPath,
	}

	q := jobs.GetQueue()
	job, jobCtx := q.CreateJob(exportID, "training-data-export", map[string]any{
		"space_id":    req.SpaceID,
		"tables":      req.Tables,
		"output_path": outputPath,
	})
	q.StartJob(exportID)

	// Run export in background goroutine
	go func() {
		pool := s.tsdbClient.Pool()
		result, err := tsdb.RunExport(jobCtx, pool, cfg, s.cfg.MdemgVersion, "")
		if err != nil {
			slog.Error("training data export failed", "export_id", exportID, "error", err)
			job.Fail(err)
			return
		}

		job.Complete(map[string]any{
			"export_id":   result.Manifest.ExportID,
			"total_rows":  result.TotalRows,
			"tables":      len(result.Manifest.Tables),
			"output_path": result.OutputPath,
			"privacy":     result.Manifest.Quality.PrivacyScrubViolations,
		})
		slog.Info("training data export completed", "export_id", exportID, "rows", result.TotalRows)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"export_id": exportID,
		"status":    "pending",
		"message":   "training data export started",
	})
}

// handleTrainingDataStatus returns the status of a training data export job.
// GET /v1/training-data/status/{id}
func (s *Server) handleTrainingDataStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	exportID := strings.TrimPrefix(r.URL.Path, "/v1/training-data/status/")
	if exportID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "export_id required"})
		return
	}

	q := jobs.GetQueue()
	job, ok := q.GetJob(exportID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "export not found"})
		return
	}

	snap := job.GetSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"export_id": snap.ID,
		"status":    string(snap.Status),
		"progress":  snap.Progress,
		"result":    snap.Result,
		"error":     snap.Error,
	})
}

// handleTrainingDataDownload serves a completed export archive.
// GET /v1/training-data/download/{id}
func (s *Server) handleTrainingDataDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	exportID := strings.TrimPrefix(r.URL.Path, "/v1/training-data/download/")
	if exportID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "export_id required"})
		return
	}

	q := jobs.GetQueue()
	job, ok := q.GetJob(exportID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "export not found"})
		return
	}

	snap := job.GetSnapshot()
	if snap.Status != jobs.StatusCompleted {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  "export not ready",
			"status": string(snap.Status),
		})
		return
	}

	outputPath, _ := snap.Result["output_path"].(string)
	if outputPath == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "output path not found"})
		return
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusGone, map[string]any{"error": "export file no longer available"})
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(outputPath)))
	http.ServeFile(w, r, outputPath)
}
