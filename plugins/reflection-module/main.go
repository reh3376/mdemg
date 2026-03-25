package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const moduleID = "reflection-module"
const moduleVersion = "1.0.0"

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	socketPath := flag.String("socket", "", "Unix socket path")
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
		slog.Error("failed to listen on socket", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "module", moduleID, "socket", *socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	s := &server{
		startTime: time.Now(),
	}

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterAPEModuleServer(grpcServer, s)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal", "module", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}

// Handshake implements ModuleLifecycle.Handshake
func (s *server) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "module", moduleID, "mdemg_version", req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"reflection", "session_summary"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle.HealthCheck
func (s *server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	executions := s.executionsTotal
	lastExec := s.lastExecution
	s.mu.Unlock()

	uptime := time.Since(s.startTime).String()

	metrics := map[string]string{
		"uptime":           uptime,
		"executions_total": strconv.FormatInt(executions, 10),
	}
	if !lastExec.IsZero() {
		metrics["last_execution"] = lastExec.Format(time.RFC3339)
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: metrics,
	}, nil
}

// Shutdown implements ModuleLifecycle.Shutdown
func (s *server) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "module", moduleID, "reason", req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "shutting down gracefully",
	}, nil
}

// GetSchedule implements APEModule.GetSchedule
func (s *server) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		CronExpression:     "0 * * * *", // Every hour
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: 300, // Minimum 5 minutes between runs
	}, nil
}

// Execute implements APEModule.Execute
func (s *server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	s.mu.Lock()
	s.executionsTotal++
	s.lastExecution = start
	execNum := s.executionsTotal
	s.mu.Unlock()

	slog.Info("executing task", "module", moduleID, "task_id", req.TaskId, "trigger", req.Trigger, "execution", execNum)

	// Simulate some work
	// In a real implementation, this would:
	// 1. Query MDEMG for recent observations
	// 2. Analyze patterns and generate reflections
	// 3. Create summary nodes or update existing ones

	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "event:session_end":
		message = "Session reflection completed - analyzed recent activity"
		stats = &pb.ExecuteStats{
			NodesCreated: 0, // Would create reflection nodes
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	case "event:consolidate":
		message = "Post-consolidation reflection completed"
		stats = &pb.ExecuteStats{
			NodesCreated: 0,
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	case "schedule":
		message = "Scheduled reflection completed - periodic health check"
		stats = &pb.ExecuteStats{
			NodesCreated: 0,
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	default:
		message = "Generic execution completed"
		stats = &pb.ExecuteStats{
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	slog.Info("task completed", "module", moduleID, "task_id", req.TaskId, "duration", time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}
