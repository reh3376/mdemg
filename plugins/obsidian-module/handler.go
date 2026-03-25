package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// ObsidianHandler implements the INGESTION module interfaces for Obsidian vaults.
type ObsidianHandler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer

	mu          sync.RWMutex
	startTime   time.Time
	config      map[string]string
	vaultPath   string
	lastSyncAt  time.Time
	mtimeIndex  map[string]time.Time
	excludeDirs []string
}

var requestCounter uint64

// NewObsidianHandler creates a new handler instance.
func NewObsidianHandler() *ObsidianHandler {
	return &ObsidianHandler{
		startTime:  time.Now(),
		config:     make(map[string]string),
		mtimeIndex: make(map[string]time.Time),
	}
}

// ============ Lifecycle RPCs ============

func (h *ObsidianHandler) Handshake(_ context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "mdemg_version", req.MdemgVersion)

	h.mu.Lock()
	h.config = req.Config

	// Extract vault path from config, fallback to env var
	if vp, ok := req.Config["vault_path"]; ok && vp != "" {
		h.vaultPath = vp
	} else if vp := os.Getenv("OBSIDIAN_VAULT_PATH"); vp != "" {
		h.vaultPath = vp
	}

	// Validate vault path if set
	if h.vaultPath != "" {
		if _, err := os.Stat(h.vaultPath); err != nil {
			slog.Warn("vault path not accessible", "path", h.vaultPath, "error", err)
		} else {
			slog.Info("vault path configured", "path", h.vaultPath)
		}
	}

	// Parse exclude dirs from config
	if excl, ok := req.Config["exclude_dirs"]; ok && excl != "" {
		h.excludeDirs = strings.Split(excl, ",")
		for i := range h.excludeDirs {
			h.excludeDirs[i] = strings.TrimSpace(h.excludeDirs[i])
		}
	}
	h.mu.Unlock()

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"obsidian://", "text/markdown", "application/vnd.obsidian.md"},
		Ready:         true,
	}, nil
}

func (h *ObsidianHandler) HealthCheck(_ context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.RLock()
	vaultPath := h.vaultPath
	lastSync := h.lastSyncAt
	h.mu.RUnlock()

	metrics := map[string]string{
		"uptime":           time.Since(h.startTime).String(),
		"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		"vault_configured": fmt.Sprintf("%t", vaultPath != ""),
	}
	if vaultPath != "" {
		metrics["vault_path"] = vaultPath
		if _, err := os.Stat(vaultPath); err == nil {
			metrics["vault_accessible"] = "true"
		} else {
			metrics["vault_accessible"] = "false"
		}
	}
	if !lastSync.IsZero() {
		metrics["last_sync"] = lastSync.Format(time.RFC3339)
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: metrics,
	}, nil
}

func (h *ObsidianHandler) Shutdown(_ context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "reason", req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Ingestion RPCs ============

// Matches checks if the content looks like an Obsidian markdown note.
// Returns high confidence if wikilinks, Obsidian-style tags, or frontmatter are detected.
func (h *ObsidianHandler) Matches(_ context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// Direct URI match
	if strings.HasPrefix(req.SourceUri, "obsidian://") {
		return &pb.MatchResponse{
			Matches:    true,
			Confidence: 1.0,
			Reason:     "obsidian:// URI scheme",
		}, nil
	}

	// Content type match
	if req.ContentType == "application/vnd.obsidian.md" {
		return &pb.MatchResponse{
			Matches:    true,
			Confidence: 1.0,
			Reason:     "Obsidian content type",
		}, nil
	}

	// Must be a markdown file
	ext := strings.ToLower(filepath.Ext(req.SourceUri))
	if ext != ".md" && ext != ".markdown" {
		return &pb.MatchResponse{
			Matches:    false,
			Confidence: 0,
			Reason:     "not a markdown file",
		}, nil
	}

	// Check metadata hints from caller (content_preview may be passed)
	confidence := float32(0.3) // Base confidence for any .md file
	var reasons []string
	reasons = append(reasons, "markdown file")

	if preview, ok := req.Metadata["content_preview"]; ok {
		if wikilinkRe.MatchString(preview) {
			confidence += 0.4
			reasons = append(reasons, "contains wikilinks")
		}
		if frontmatterRe.MatchString(preview) {
			confidence += 0.15
			reasons = append(reasons, "has YAML frontmatter")
		}
		if tagRe.MatchString(preview) {
			confidence += 0.15
			reasons = append(reasons, "contains #tags")
		}
	}

	// Vault path hint: if inside a known vault directory
	if _, ok := req.Metadata["obsidian_vault"]; ok {
		confidence += 0.3
		reasons = append(reasons, "inside Obsidian vault")
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return &pb.MatchResponse{
		Matches:    confidence >= 0.4,
		Confidence: confidence,
		Reason:     strings.Join(reasons, ", "),
	}, nil
}

// Parse converts an Obsidian markdown note into observations and edges.
func (h *ObsidianHandler) Parse(_ context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	content := string(req.Content)
	filename := filepath.Base(req.SourceUri)
	note := ParseNote(content, filename)

	// Generate deterministic node ID from source path
	nodeID := obsidianNodeID(req.SourceUri)

	// Build metadata from frontmatter
	metadata := map[string]string{
		"source":   "obsidian",
		"filename": filename,
	}
	for k, v := range note.FrontmatterStrings() {
		metadata["fm_"+k] = v
	}
	if len(note.Aliases) > 0 {
		metadata["aliases"] = strings.Join(note.Aliases, ", ")
	}
	if len(note.Embeds) > 0 {
		metadata["embeds"] = strings.Join(note.Embeds, ",")
	}

	// Build tags list: module tag + extracted tags
	tags := []string{"obsidian"}
	tags = append(tags, note.Tags...)

	obs := &pb.Observation{
		NodeId:      nodeID,
		Path:        req.SourceUri,
		Name:        note.Title,
		Content:     note.CleanContent(),
		ContentType: "text/markdown",
		Tags:        tags,
		Metadata:    metadata,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	// Build edges for wikilinks
	var edges []*pb.Edge
	seen := make(map[string]bool)

	for _, wl := range note.Wikilinks {
		targetID := obsidianNodeID(wl.Target)
		edgeKey := nodeID + "->" + targetID
		if seen[edgeKey] {
			continue
		}
		seen[edgeKey] = true

		props := map[string]string{
			"link_type": "wikilink",
		}
		if wl.Section != "" {
			props["section"] = wl.Section
		}
		if wl.Alias != "" {
			props["alias"] = wl.Alias
		}

		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   targetID,
			RelType:    "WIKILINK",
			Weight:     0.8,
			Properties: props,
		})
	}

	// Build edges for embeds (stronger relationship)
	for _, embed := range note.Embeds {
		targetID := obsidianNodeID(embed)
		edgeKey := nodeID + "->embed->" + targetID
		if seen[edgeKey] {
			continue
		}
		seen[edgeKey] = true

		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   targetID,
			RelType:    "EMBEDS",
			Weight:     0.9,
			Properties: map[string]string{"link_type": "embed"},
		})
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Edges:        edges,
		Metadata: map[string]string{
			"parsed_at":      time.Now().Format(time.RFC3339),
			"wikilink_count": fmt.Sprintf("%d", len(note.Wikilinks)),
			"tag_count":      fmt.Sprintf("%d", len(note.Tags)),
			"embed_count":    fmt.Sprintf("%d", len(note.Embeds)),
		},
	}, nil
}

// Sync walks an Obsidian vault directory and streams all notes as observations.
func (h *ObsidianHandler) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	ctx := stream.Context()

	// Resolve vault path: request config → stored → env var
	vaultPath := h.resolveVaultPath(req.Config)
	if vaultPath == "" {
		return fmt.Errorf("vault path not configured: set vault_path in config or OBSIDIAN_VAULT_PATH env var")
	}

	// Validate vault path
	if _, err := os.Stat(vaultPath); err != nil {
		return fmt.Errorf("vault path not accessible: %w", err)
	}

	slog.Info("sync starting", "vault_path", vaultPath, "cursor", req.Cursor)

	// Walk the vault
	h.mu.RLock()
	excludeDirs := h.excludeDirs
	h.mu.RUnlock()

	walker := NewVaultWalker(vaultPath, excludeDirs)
	files, err := walker.Walk()
	if err != nil {
		return fmt.Errorf("vault walk failed: %w", err)
	}

	// Filter by cursor (RFC3339 mtime-based incremental sync)
	if req.Cursor != "" {
		cursorTime, err := time.Parse(time.RFC3339, req.Cursor)
		if err == nil {
			files = filterModifiedSince(files, cursorTime)
			slog.Info("incremental sync", "since", req.Cursor, "files_after_filter", len(files))
		} else {
			slog.Warn("invalid cursor, doing full sync", "cursor", req.Cursor, "error", err)
		}
	}

	var processed, created, updated, skipped int32
	const batchSize = 50
	var batchObs []*pb.Observation
	var batchEdges []*pb.Edge

	for i, file := range files {
		// Check context cancellation
		select {
		case <-ctx.Done():
			slog.Warn("sync cancelled", "processed", processed)
			return ctx.Err()
		default:
		}

		// Skip if unchanged since last sync
		h.mu.RLock()
		lastMtime, known := h.mtimeIndex[file.RelPath]
		h.mu.RUnlock()
		if known && !file.ModTime.After(lastMtime) {
			skipped++
			processed++
			continue
		}

		// Read file content
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			slog.Warn("failed to read file", "path", file.RelPath, "error", err)
			skipped++
			processed++
			continue
		}

		// Parse note
		note := ParseNote(string(content), filepath.Base(file.RelPath))
		obs := h.noteToObservation(note, file)
		edges := h.noteToEdges(note, file)

		batchObs = append(batchObs, obs)
		batchEdges = append(batchEdges, edges...)

		if known {
			updated++
		} else {
			created++
		}
		processed++

		// Update mtime index
		h.mu.Lock()
		h.mtimeIndex[file.RelPath] = file.ModTime
		h.mu.Unlock()

		// Send batch when full or at end
		if len(batchObs) >= batchSize || i == len(files)-1 {
			hasMore := i < len(files)-1
			cursor := file.ModTime.Format(time.RFC3339)

			if err := stream.Send(&pb.SyncResponse{
				Observations: batchObs,
				Edges:        batchEdges,
				Cursor:       cursor,
				HasMore:      hasMore,
				Stats: &pb.SyncStats{
					ItemsProcessed: processed,
					ItemsCreated:   created,
					ItemsUpdated:   updated,
					ItemsSkipped:   skipped,
				},
			}); err != nil {
				return err
			}

			batchObs = nil
			batchEdges = nil
		}
	}

	// Update last sync time
	h.mu.Lock()
	h.lastSyncAt = time.Now()
	h.mu.Unlock()

	slog.Info("sync complete", "processed", processed, "created", created, "updated", updated, "skipped", skipped)
	return nil
}

// resolveVaultPath resolves the vault path from request config, stored config, or env var.
func (h *ObsidianHandler) resolveVaultPath(config map[string]string) string {
	if vp, ok := config["vault_path"]; ok && vp != "" {
		return vp
	}
	h.mu.RLock()
	vp := h.vaultPath
	h.mu.RUnlock()
	if vp != "" {
		return vp
	}
	return os.Getenv("OBSIDIAN_VAULT_PATH")
}

// noteToObservation converts a parsed note into a proto Observation.
func (h *ObsidianHandler) noteToObservation(note ObsidianNote, file VaultFile) *pb.Observation {
	nodeID := obsidianNodeID(file.RelPath)

	metadata := map[string]string{
		"source":   "obsidian",
		"filename": filepath.Base(file.RelPath),
		"rel_path": file.RelPath,
		"mod_time": file.ModTime.Format(time.RFC3339),
		"size":     fmt.Sprintf("%d", file.Size),
	}
	for k, v := range note.FrontmatterStrings() {
		metadata["fm_"+k] = v
	}
	if len(note.Aliases) > 0 {
		metadata["aliases"] = strings.Join(note.Aliases, ", ")
	}
	if note.CreatedAt != "" {
		metadata["created_at"] = note.CreatedAt
	}

	tags := []string{"obsidian"}
	tags = append(tags, note.Tags...)

	return &pb.Observation{
		NodeId:      nodeID,
		Path:        file.RelPath,
		Name:        note.Title,
		Content:     note.CleanContent(),
		ContentType: "text/markdown",
		Tags:        tags,
		Metadata:    metadata,
		Timestamp:   file.ModTime.Format(time.RFC3339),
		Source:      moduleID,
	}
}

// noteToEdges builds wikilink and embed edges for a parsed note.
func (h *ObsidianHandler) noteToEdges(note ObsidianNote, file VaultFile) []*pb.Edge {
	nodeID := obsidianNodeID(file.RelPath)
	var edges []*pb.Edge
	seen := make(map[string]bool)

	for _, wl := range note.Wikilinks {
		targetID := obsidianNodeID(wl.Target)
		edgeKey := nodeID + "->" + targetID
		if seen[edgeKey] {
			continue
		}
		seen[edgeKey] = true

		props := map[string]string{"link_type": "wikilink", "source": "obsidian"}
		if wl.Section != "" {
			props["section"] = wl.Section
		}
		if wl.Alias != "" {
			props["alias"] = wl.Alias
		}

		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   targetID,
			RelType:    "WIKILINK",
			Weight:     0.8,
			Properties: props,
		})
	}

	for _, embed := range note.Embeds {
		targetID := obsidianNodeID(embed)
		edgeKey := nodeID + "->embed->" + targetID
		if seen[edgeKey] {
			continue
		}
		seen[edgeKey] = true

		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   targetID,
			RelType:    "EMBEDS",
			Weight:     0.9,
			Properties: map[string]string{"link_type": "embed", "source": "obsidian"},
		})
	}

	return edges
}

// obsidianNodeID generates a deterministic node ID from a file path or note name.
func obsidianNodeID(path string) string {
	normalized := strings.ToLower(strings.TrimSpace(path))
	normalized = strings.TrimSuffix(normalized, ".md")
	normalized = strings.TrimSuffix(normalized, ".markdown")
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("obsidian-%x", hash[:8])
}
