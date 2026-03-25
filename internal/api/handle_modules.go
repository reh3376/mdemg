package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/models"
)

// handleModules handles GET /v1/modules - lists all loaded plugin modules
func (s *Server) handleModules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.pluginMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"enabled": false,
				"modules": []any{},
			},
		})
		return
	}

	modules := s.pluginMgr.ListModules()

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"enabled": true,
			"modules": modules,
		},
	})
}

// handleModuleSync handles POST /v1/modules/{id}/sync - triggers a sync on an ingestion module
func (s *Server) handleModuleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.pluginMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "plugin system not enabled"})
		return
	}

	// Extract module ID from path: /v1/modules/{id}/sync
	path := strings.TrimPrefix(r.URL.Path, "/v1/modules/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "sync" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
		return
	}
	moduleID := parts[0]

	// Get the module
	modInfo, ok := s.pluginMgr.GetModule(moduleID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "module not found"})
		return
	}

	if modInfo.IngestionClient == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "module is not an ingestion module"})
		return
	}

	// Parse request body for sync config
	var req struct {
		SourceID string            `json:"source_id"`
		Cursor   string            `json:"cursor"`
		Config   map[string]string `json:"config"`
		Ingest   bool              `json:"ingest"`   // If true, store observations in MDEMG
		SpaceID  string            `json:"space_id"` // Required if ingest=true
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}

	// Default source ID based on module
	if req.SourceID == "" {
		req.SourceID = moduleID + "://issues"
	}

	// Validate ingest requirements
	if req.Ingest && req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id required when ingest=true"})
		return
	}

	// Call Sync on the module (10 minute timeout for large syncs)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	stream, err := modInfo.IngestionClient.Sync(ctx, &pb.SyncRequest{
		SourceId: req.SourceID,
		Cursor:   req.Cursor,
		Config:   req.Config,
	})
	if err != nil {
		writeInternalError(w, err, "module sync")
		return
	}

	// Collect all observations from the stream
	var allObs []map[string]any
	var ingestBatch []models.BatchIngestItem
	var lastCursor string
	var stats *pb.SyncStats
	var ingestCount, ingestErrors int

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeInternalError(w, err, "module sync stream")
			return
		}

		// Convert observations to response format
		for _, obs := range resp.Observations {
			allObs = append(allObs, map[string]any{
				"node_id":      obs.NodeId,
				"path":         obs.Path,
				"name":         obs.Name,
				"content_type": obs.ContentType,
				"tags":         obs.Tags,
				"timestamp":    obs.Timestamp,
				"source":       obs.Source,
				"metadata":     obs.Metadata,
			})

			// Build ingest request if ingesting
			if req.Ingest {
				ingestItem := models.BatchIngestItem{
					NodeID:    obs.NodeId,
					Path:      obs.Path,
					Name:      obs.Name,
					Timestamp: obs.Timestamp,
					Source:    obs.Source,
					Tags:      obs.Tags,
					Content:   obs.Content,
				}
				ingestBatch = append(ingestBatch, ingestItem)

				// Batch ingest every 100 items to avoid memory issues
				if len(ingestBatch) >= 100 {
					batchReq := models.BatchIngestRequest{SpaceID: req.SpaceID, Observations: ingestBatch}
					// Generate embeddings
					if s.embedder != nil {
						for i := range batchReq.Observations {
							text := fmt.Sprintf("%s: %v", batchReq.Observations[i].Name, batchReq.Observations[i].Content)
							emb, err := s.embedder.Embed(r.Context(), text)
							if err == nil {
								batchReq.Observations[i].Embedding = emb
							}
						}
					}
					_, err := s.retriever.BatchIngestObservations(r.Context(), batchReq)
					if err != nil {
						slog.Error("batch ingest failed", "error", err)
						ingestErrors += len(ingestBatch)
					} else {
						ingestCount += len(ingestBatch)
					}
					ingestBatch = nil
				}
			}
		}

		lastCursor = resp.Cursor
		stats = resp.Stats

		if !resp.HasMore {
			break
		}
	}

	// Ingest remaining batch
	if req.Ingest && len(ingestBatch) > 0 {
		batchReq := models.BatchIngestRequest{SpaceID: req.SpaceID, Observations: ingestBatch}
		if s.embedder != nil {
			for i := range batchReq.Observations {
				text := fmt.Sprintf("%s: %v", batchReq.Observations[i].Name, batchReq.Observations[i].Content)
				emb, err := s.embedder.Embed(r.Context(), text)
				if err == nil {
					batchReq.Observations[i].Embedding = emb
				}
			}
		}
		_, err := s.retriever.BatchIngestObservations(r.Context(), batchReq)
		if err != nil {
			slog.Error("batch ingest failed", "error", err)
			ingestErrors += len(ingestBatch)
		} else {
			ingestCount += len(ingestBatch)
		}
	}

	result := map[string]any{
		"cursor": lastCursor,
		"count":  len(allObs),
	}

	// Only include observations if not ingesting (to avoid huge responses)
	if !req.Ingest {
		result["observations"] = allObs
	}

	if stats != nil {
		result["stats"] = map[string]any{
			"items_processed": stats.ItemsProcessed,
			"items_created":   stats.ItemsCreated,
			"items_updated":   stats.ItemsUpdated,
			"items_skipped":   stats.ItemsSkipped,
		}
	}

	if req.Ingest {
		result["ingested"] = ingestCount
		result["ingest_errors"] = ingestErrors
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
