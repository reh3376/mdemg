package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// TestApePluginHandler implements the APE module interfaces
type TestApePluginHandler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
}

// NewTestApePluginHandler creates a new handler instance
func NewTestApePluginHandler() *TestApePluginHandler {
	return &TestApePluginHandler{
		startTime: time.Now(),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *TestApePluginHandler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"background_task"}, // TODO: Describe your capabilities
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *TestApePluginHandler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	executions := h.executionsTotal
	lastExec := h.lastExecution
	h.mu.Unlock()

	metrics := map[string]string{
		"uptime":           time.Since(h.startTime).String(),
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

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *TestApePluginHandler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ APE RPCs ============

// GetSchedule returns the module's preferred execution schedule.
func (h *TestApePluginHandler) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		// TODO: Customize your schedule
		CronExpression:     "0 * * * *", // Every hour
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: 300, // Minimum 5 minutes between runs
	}, nil
}

// Execute runs a background task.
func (h *TestApePluginHandler) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	h.mu.Lock()
	h.executionsTotal++
	h.lastExecution = start
	execNum := h.executionsTotal
	h.mu.Unlock()

	log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
		moduleID, req.TaskId, req.Trigger, execNum)

	// TODO: Implement your background task logic here
	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "event:session_end":
		// TODO: Handle session end event
		message, stats = h.handleSessionEnd(ctx)

	case "event:consolidate":
		// TODO: Handle consolidation event
		message, stats = h.handleConsolidate(ctx)

	case "schedule":
		// TODO: Handle scheduled execution
		message, stats = h.handleScheduled(ctx)

	default:
		message = "Unknown trigger, no action taken"
		stats = &pb.ExecuteStats{DurationMs: time.Since(start).Milliseconds()}
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	log.Printf("%s: task %s completed in %v", moduleID, req.TaskId, time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}

// ============ Task Handlers (Customize these) ============

func (h *TestApePluginHandler) handleSessionEnd(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement session reflection logic
	// - Query recent observations
	// - Generate summary nodes
	// - Create relationship edges

	return "Session processing completed", &pb.ExecuteStats{
		NodesCreated: 0,
		NodesUpdated: 0,
		EdgesCreated: 0,
	}
}

func (h *TestApePluginHandler) handleConsolidate(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement post-consolidation logic

	return "Post-consolidation processing completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}

func (h *TestApePluginHandler) handleScheduled(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement periodic maintenance
	// - Cleanup stale data
	// - Recalculate statistics
	// - Detect patterns

	return "Scheduled maintenance completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}
