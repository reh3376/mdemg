package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// TestIngestionPluginHandler implements the INGESTION module interfaces
type TestIngestionPluginHandler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer

	mu        sync.Mutex
	startTime time.Time
	config    map[string]string
}

var requestCounter uint64

// NewTestIngestionPluginHandler creates a new handler instance
func NewTestIngestionPluginHandler() *TestIngestionPluginHandler {
	return &TestIngestionPluginHandler{
		startTime: time.Now(),
		config:    make(map[string]string),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *TestIngestionPluginHandler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Store configuration from manifest
	h.mu.Lock()
	h.config = req.Config
	h.mu.Unlock()

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"test-ingestion-plugin://", "application/x-test-ingestion-plugin"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *TestIngestionPluginHandler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *TestIngestionPluginHandler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Ingestion RPCs ============

// Matches checks if this module can handle the given source.
func (h *TestIngestionPluginHandler) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Customize matching logic for your data source
	matches := strings.HasPrefix(req.SourceUri, "test-ingestion-plugin://") ||
		req.ContentType == "application/x-test-ingestion-plugin"

	confidence := float32(0.0)
	reason := "not a supported source"
	if matches {
		confidence = 1.0
		reason = "matches test-ingestion-plugin:// or application/x-test-ingestion-plugin"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse converts source content into MDEMG observations.
func (h *TestIngestionPluginHandler) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement your parsing logic here
	// Convert the raw content into structured observations

	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-%d", moduleID, time.Now().UnixNano()),
		Path:        req.SourceUri,
		Name:        "Parsed Item", // TODO: Extract a meaningful name
		Content:     string(req.Content),
		ContentType: req.ContentType,
		Tags:        []string{moduleID}, // TODO: Add relevant tags
		Metadata:    map[string]string{"source_uri": req.SourceUri},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at": time.Now().Format(time.RFC3339),
		},
	}, nil
}

// Sync performs incremental synchronization with an external source.
func (h *TestIngestionPluginHandler) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement incremental sync logic
	// Use req.Cursor to resume from the last position
	// Fetch items from your external source in batches

	// Example: Send a single response (replace with your sync logic)
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-sync-%d", moduleID, time.Now().UnixNano()),
		Path:        fmt.Sprintf("%s://sync", moduleID),
		Name:        "sync-observation",
		Content:     "TODO: Replace with synced content",
		ContentType: "text/plain",
		Tags:        []string{moduleID, "sync"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	if err := stream.Send(&pb.SyncResponse{
		Observations: []*pb.Observation{obs},
		Cursor:       "cursor-1", // TODO: Return actual cursor for incremental sync
		HasMore:      false,
		Stats: &pb.SyncStats{
			ItemsProcessed: 1,
			ItemsCreated:   1,
		},
	}); err != nil {
		return err
	}

	return nil
}
