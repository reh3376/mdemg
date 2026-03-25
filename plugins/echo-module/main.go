package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

var (
	socketPath     = flag.String("socket", "", "Unix socket path")
	requestCounter uint64
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	flag.Parse()

	if *socketPath == "" {
		slog.Error("missing required flag", "flag", "--socket")
		os.Exit(1)
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Create gRPC server
	server := grpc.NewServer()

	// Register services
	echoModule := &EchoModule{
		startTime: time.Now(),
	}
	pb.RegisterModuleLifecycleServer(server, echoModule)
	pb.RegisterIngestionModuleServer(server, echoModule)

	slog.Info("echo module listening", "socket", *socketPath)

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("shutting down")
		server.GracefulStop()
	}()

	// Serve
	if err := server.Serve(listener); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// EchoModule implements the module interfaces for testing
type EchoModule struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer
	startTime time.Time
}

// Handshake implements ModuleLifecycle
func (m *EchoModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "mdemg_version", req.MdemgVersion, "socket", req.SocketPath)

	return &pb.HandshakeResponse{
		ModuleId:      "echo-module",
		ModuleVersion: "1.0.0",
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"echo://", "text/plain"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle
func (m *EchoModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	uptime := time.Since(m.startTime).String()
	requests := atomic.LoadUint64(&requestCounter)

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           uptime,
			"requests_handled": fmt.Sprintf("%d", requests),
		},
	}, nil
}

// Shutdown implements ModuleLifecycle
func (m *EchoModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "reason", req.Reason, "timeout_ms", req.TimeoutMs)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// Matches implements IngestionModule
func (m *EchoModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	matches := strings.HasPrefix(req.SourceUri, "echo://") || req.ContentType == "text/plain"
	confidence := float32(0.0)
	reason := "not an echo source"

	if matches {
		confidence = 1.0
		reason = "matches echo:// or text/plain"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse implements IngestionModule
func (m *EchoModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// Echo module just creates a single observation from the content
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("echo-%d", time.Now().UnixNano()),
		Path:        req.SourceUri,
		Name:        "echo-observation",
		Content:     string(req.Content),
		ContentType: req.ContentType,
		Tags:        []string{"echo", "test"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "echo-module",
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at": time.Now().Format(time.RFC3339),
			"bytes":     fmt.Sprintf("%d", len(req.Content)),
		},
	}, nil
}

// Sync implements IngestionModule (streaming)
func (m *EchoModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	// Echo module doesn't support real sync, just sends a single response
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("echo-sync-%d", time.Now().UnixNano()),
		Path:        "echo://sync",
		Name:        "echo-sync-observation",
		Content:     "sync test",
		ContentType: "text/plain",
		Tags:        []string{"echo", "sync"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "echo-module",
	}

	if err := stream.Send(&pb.SyncResponse{
		Observations: []*pb.Observation{obs},
		Cursor:       "cursor-1",
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
