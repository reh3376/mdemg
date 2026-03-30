package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	pb "mdemg/api/transferpb"
	"mdemg/internal/transfer"

	"google.golang.org/protobuf/encoding/protojson"
)

// ── Request/Response types for transfer endpoints ──

// SpaceExportRequest is the request body for POST /v1/admin/spaces/export.
type SpaceExportRequest struct {
	SpaceID           string   `json:"space_id"`
	Profile           string   `json:"profile,omitempty"`
	ChunkSize         int      `json:"chunk_size,omitempty"`
	IncludeEmbeddings *bool    `json:"include_embeddings,omitempty"`
	ObsTypes          []string `json:"obs_types,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	ExcludeVolatile   bool     `json:"exclude_volatile,omitempty"`
	OnlyPinned        bool     `json:"only_pinned,omitempty"`
	MinLayer          int      `json:"min_layer,omitempty"`
	MaxLayer          int      `json:"max_layer,omitempty"`
	SinceTimestamp    string   `json:"since_timestamp,omitempty"`
	SinceCursor       string   `json:"since_cursor,omitempty"`
	NoObservations    bool     `json:"no_observations,omitempty"`
	NoSymbols         bool     `json:"no_symbols,omitempty"`
	NoLearnedEdges    bool     `json:"no_learned_edges,omitempty"`
}

// SpaceExportResponse is the response for POST /v1/admin/spaces/export.
type SpaceExportResponse struct {
	SpaceID string                  `json:"space_id"`
	Profile string                  `json:"profile"`
	Header  transfer.MdemgFileHeader `json:"header"`
	Chunks  []json.RawMessage       `json:"chunks"`
	Summary SpaceExportSummary      `json:"summary"`
}

// SpaceExportSummary is the summary section of the export response.
type SpaceExportSummary struct {
	NodesExported        int64  `json:"nodes_exported"`
	EdgesExported        int64  `json:"edges_exported"`
	ObservationsExported int64  `json:"observations_exported"`
	SymbolsExported      int64  `json:"symbols_exported"`
	DurationMs           int64  `json:"duration_ms"`
	NextCursor           string `json:"next_cursor,omitempty"`
}

// SpaceExportPreviewResponse is the response for GET /v1/admin/spaces/export/preview.
type SpaceExportPreviewResponse struct {
	SpaceID               string         `json:"space_id"`
	Profile               string         `json:"profile"`
	EstimatedNodes        int64          `json:"estimated_nodes"`
	EstimatedEdges        int64          `json:"estimated_edges"`
	EstimatedObservations int64          `json:"estimated_observations"`
	EstimatedSymbols      int64          `json:"estimated_symbols"`
	FiltersApplied        map[string]any `json:"filters_applied"`
}

// SpaceImportRequest is the request body for POST /v1/admin/spaces/import.
type SpaceImportRequest struct {
	SpaceID  string            `json:"space_id,omitempty"`
	Conflict string            `json:"conflict,omitempty"`
	Chunks   []json.RawMessage `json:"chunks"`
}

// SpaceImportResponse is the response for POST /v1/admin/spaces/import.
type SpaceImportResponse struct {
	SpaceID             string   `json:"space_id"`
	NodesCreated        int32    `json:"nodes_created"`
	NodesSkipped        int32    `json:"nodes_skipped"`
	NodesOverwritten    int32    `json:"nodes_overwritten"`
	EdgesCreated        int32    `json:"edges_created"`
	EdgesSkipped        int32    `json:"edges_skipped"`
	EdgesMerged         int32    `json:"edges_merged"`
	ObservationsCreated int32    `json:"observations_created"`
	SymbolsCreated      int32    `json:"symbols_created"`
	Warnings            []string `json:"warnings"`
	DurationMs          int64    `json:"duration_ms"`
}

// handleSpaceExport handles POST /v1/admin/spaces/export.
func (s *Server) handleSpaceExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req SpaceExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	// Build export config from profile
	profile := req.Profile
	if profile == "" {
		profile = transfer.ProfileFull
	}
	cfg, err := transfer.ExportConfigForProfile(req.SpaceID, profile)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Apply request overrides
	if req.ChunkSize > 0 {
		cfg.ChunkSize = req.ChunkSize
	}
	if req.IncludeEmbeddings != nil {
		cfg.IncludeEmbeddings = *req.IncludeEmbeddings
	}
	if len(req.ObsTypes) > 0 {
		cfg.ObsTypes = req.ObsTypes
	}
	if len(req.Tags) > 0 {
		cfg.Tags = req.Tags
	}
	if req.ExcludeVolatile {
		cfg.ExcludeVolatile = true
	}
	if req.OnlyPinned {
		cfg.OnlyPinned = true
	}
	if req.MinLayer > 0 {
		cfg.MinLayer = req.MinLayer
	}
	if req.MaxLayer > 0 {
		cfg.MaxLayer = req.MaxLayer
	}
	if req.SinceTimestamp != "" {
		cfg.SinceTimestamp = req.SinceTimestamp
	}
	if req.SinceCursor != "" {
		cfg.SinceCursor = req.SinceCursor
		if cfg.SinceTimestamp == "" {
			cfg.SinceTimestamp = cfg.SinceCursor
		}
	}
	if req.NoObservations {
		cfg.IncludeObservations = false
	}
	if req.NoSymbols {
		cfg.IncludeSymbols = false
	}
	if req.NoLearnedEdges {
		cfg.IncludeLearnedEdges = false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	exporter := transfer.NewExporter(s.driver)
	result, err := exporter.Export(ctx, cfg)
	if err != nil {
		writeInternalError(w, err, "space-export")
		return
	}

	// Serialize chunks to json.RawMessage using protojson
	marshaler := protojson.MarshalOptions{
		EmitDefaultValues: false,
		UseProtoNames:     true,
	}
	chunks := make([]json.RawMessage, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		b, err := marshaler.Marshal(chunk)
		if err != nil {
			slog.Error("space-export: marshal chunk failed", "sequence", chunk.Sequence, "error", err)
			writeInternalError(w, err, "space-export-marshal")
			return
		}
		chunks = append(chunks, json.RawMessage(b))
	}

	// Extract summary from last chunk
	var summary SpaceExportSummary
	if len(result.Chunks) > 0 {
		last := result.Chunks[len(result.Chunks)-1]
		if last.Summary != nil {
			summary = SpaceExportSummary{
				NodesExported:        last.Summary.NodesExported,
				EdgesExported:        last.Summary.EdgesExported,
				ObservationsExported: last.Summary.ObservationsExported,
				SymbolsExported:      last.Summary.SymbolsExported,
				DurationMs:           last.Summary.DurationMs,
				NextCursor:           last.Summary.NextCursor,
			}
		}
	}

	writeJSON(w, http.StatusOK, SpaceExportResponse{
		SpaceID: req.SpaceID,
		Profile: profile,
		Header: transfer.MdemgFileHeader{
			Format:  "mdemg-space-transfer",
			Version: "1.0.0",
		},
		Chunks:  chunks,
		Summary: summary,
	})
}

// handleSpaceExportPreview handles GET /v1/admin/spaces/export/preview.
func (s *Server) handleSpaceExportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	q := r.URL.Query()
	spaceID := q.Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	profile := q.Get("profile")
	if profile == "" {
		profile = transfer.ProfileFull
	}
	cfg, err := transfer.ExportConfigForProfile(spaceID, profile)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Use MetadataOnly for lightweight counting
	cfg.MetadataOnly = true

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	exporter := transfer.NewExporter(s.driver)
	result, err := exporter.Export(ctx, cfg)
	if err != nil {
		writeInternalError(w, err, "space-export-preview")
		return
	}

	// Extract counts from metadata chunk
	var nodes, edges, obs, syms int64
	for _, chunk := range result.Chunks {
		if chunk.Metadata != nil {
			nodes = chunk.Metadata.TotalNodes
			edges = chunk.Metadata.TotalEdges
			obs = chunk.Metadata.TotalObservations
			syms = chunk.Metadata.TotalSymbols
			break
		}
	}

	// Build filters_applied from config
	filters := map[string]any{
		"exclude_volatile": cfg.ExcludeVolatile,
		"only_pinned":      cfg.OnlyPinned,
		"min_layer":        cfg.MinLayer,
		"max_layer":        cfg.MaxLayer,
	}
	if len(cfg.ObsTypes) > 0 {
		filters["obs_types"] = cfg.ObsTypes
	} else {
		filters["obs_types"] = []string{}
	}
	if len(cfg.Tags) > 0 {
		filters["tags"] = cfg.Tags
	}

	writeJSON(w, http.StatusOK, SpaceExportPreviewResponse{
		SpaceID:               spaceID,
		Profile:               profile,
		EstimatedNodes:        nodes,
		EstimatedEdges:        edges,
		EstimatedObservations: obs,
		EstimatedSymbols:      syms,
		FiltersApplied:        filters,
	})
}

// handleSpaceImport handles POST /v1/admin/spaces/import.
func (s *Server) handleSpaceImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req SpaceImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Chunks == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chunks field is required"})
		return
	}

	// Parse conflict mode
	var conflictMode pb.ConflictMode
	switch req.Conflict {
	case "", "skip":
		conflictMode = pb.ConflictMode_CONFLICT_SKIP
	case "overwrite":
		conflictMode = pb.ConflictMode_CONFLICT_OVERWRITE
	case "error":
		conflictMode = pb.ConflictMode_CONFLICT_ERROR
	case "crdt":
		conflictMode = pb.ConflictMode_CONFLICT_CRDT
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid conflict mode %q (valid: skip, overwrite, error, crdt)", req.Conflict),
		})
		return
	}

	// Unmarshal chunks from JSON to protobuf
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	chunks := make([]*pb.SpaceChunk, 0, len(req.Chunks))
	for i, raw := range req.Chunks {
		chunk := &pb.SpaceChunk{}
		if err := unmarshaler.Unmarshal(raw, chunk); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid chunk %d: %v", i, err),
			})
			return
		}
		chunks = append(chunks, chunk)
	}

	// Remap space_id if target specified
	targetSpaceID := req.SpaceID
	if targetSpaceID != "" {
		for _, chunk := range chunks {
			chunk.SpaceId = targetSpaceID
			if chunk.Metadata != nil {
				chunk.Metadata.SpaceId = targetSpaceID
			}
			if chunk.Nodes != nil {
				for _, n := range chunk.Nodes.Nodes {
					n.SpaceId = targetSpaceID
				}
			}
			if chunk.Edges != nil {
				for _, e := range chunk.Edges.Edges {
					e.SpaceId = targetSpaceID
				}
			}
			if chunk.Observations != nil {
				for _, o := range chunk.Observations.Observations {
					o.SpaceId = targetSpaceID
				}
			}
			if chunk.Symbols != nil {
				for _, sym := range chunk.Symbols.Symbols {
					sym.SpaceId = targetSpaceID
				}
			}
		}
	} else if len(chunks) > 0 {
		targetSpaceID = chunks[0].SpaceId
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Handle empty chunks: valid request, 0 work done
	if len(chunks) == 0 {
		writeJSON(w, http.StatusOK, SpaceImportResponse{
			SpaceID:  targetSpaceID,
			Warnings: []string{},
		})
		return
	}

	// Validate schema compatibility
	if err := transfer.ValidateImport(ctx, s.driver, chunks); err != nil {
		slog.Error("import validation failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "import validation failed"})
		return
	}

	// Import
	importer := transfer.NewImporter(s.driver, conflictMode)
	importResult, err := importer.Import(ctx, chunks)
	if err != nil {
		// Conflict errors return 409
		if conflictMode == pb.ConflictMode_CONFLICT_ERROR {
			slog.Error("import conflict", "error", err)
			writeJSON(w, http.StatusConflict, map[string]string{"error": "import conflict: one or more entities already exist"})
			return
		}
		writeInternalError(w, err, "space-import")
		return
	}

	warnings := importResult.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	writeJSON(w, http.StatusOK, SpaceImportResponse{
		SpaceID:             targetSpaceID,
		NodesCreated:        importResult.NodesCreated,
		NodesSkipped:        importResult.NodesSkipped,
		NodesOverwritten:    importResult.NodesOverwritten,
		EdgesCreated:        importResult.EdgesCreated,
		EdgesSkipped:        importResult.EdgesSkipped,
		EdgesMerged:         importResult.EdgesMerged,
		ObservationsCreated: importResult.ObservationsCreated,
		SymbolsCreated:      importResult.SymbolsCreated,
		Warnings:            warnings,
		DurationMs:          importResult.Duration.Milliseconds(),
	})
}
