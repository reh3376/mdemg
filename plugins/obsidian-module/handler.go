package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
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

	mu        sync.Mutex
	startTime time.Time
	config    map[string]string
}

var requestCounter uint64

// NewObsidianHandler creates a new handler instance.
func NewObsidianHandler() *ObsidianHandler {
	return &ObsidianHandler{
		startTime: time.Now(),
		config:    make(map[string]string),
	}
}

// ============ Lifecycle RPCs ============

func (h *ObsidianHandler) Handshake(_ context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	h.mu.Lock()
	h.config = req.Config
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
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		},
	}, nil
}

func (h *ObsidianHandler) Shutdown(_ context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
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
	for k, v := range note.Frontmatter {
		metadata["fm_"+k] = v
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
		Content:     note.Content,
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

// Sync walks an Obsidian vault directory and returns all notes.
func (h *ObsidianHandler) Sync(_ *pb.SyncRequest, _ pb.IngestionModule_SyncServer) error {
	// Vault-level sync requires filesystem access — delegated to the CLI
	// ingest command which calls Parse() per file. This is a no-op placeholder.
	return nil
}

// obsidianNodeID generates a deterministic node ID from a file path or note name.
func obsidianNodeID(path string) string {
	normalized := strings.ToLower(strings.TrimSpace(path))
	normalized = strings.TrimSuffix(normalized, ".md")
	normalized = strings.TrimSuffix(normalized, ".markdown")
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("obsidian-%x", hash[:8])
}
