// Package main implements an APE module that schedules codebase re-ingestion
// for stale spaces based on freshness tracking.
//
// The module:
// 1. Queries the batch freshness endpoint for configured spaces
// 2. Identifies stale spaces (last_ingest_at > threshold)
// 3. Triggers ingestion for stale spaces via the ingest/trigger endpoint
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const moduleID = "ingest-scheduler"
const moduleVersion = "1.0.0"

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time

	// Configuration
	cronExpression      string
	staleThresholdHours int
	mdemgEndpoint       string
	spaceRepoMap        map[string]string // space_id -> repo_path
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
		startTime:           time.Now(),
		cronExpression:      "0 */6 * * *", // Default: every 6 hours
		staleThresholdHours: 24,
		mdemgEndpoint:       "http://localhost:9999",
		spaceRepoMap:        make(map[string]string),
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

	// Parse config
	if cron, ok := req.Config["cron_expression"]; ok && cron != "" {
		s.cronExpression = cron
		slog.Info("config applied", "module", moduleID, "cron_expression", s.cronExpression)
	}

	if threshold, ok := req.Config["stale_threshold_hours"]; ok {
		if t, err := strconv.Atoi(threshold); err == nil {
			s.staleThresholdHours = t
			slog.Info("config applied", "module", moduleID, "stale_threshold_hours", s.staleThresholdHours)
		}
	}

	if endpoint, ok := req.Config["mdemg_endpoint"]; ok && endpoint != "" {
		s.mdemgEndpoint = endpoint
		slog.Info("config applied", "module", moduleID, "mdemg_endpoint", s.mdemgEndpoint)
	}

	// Parse space_repo_map (format: "space1=/path1,space2=/path2")
	if repoMap, ok := req.Config["space_repo_map"]; ok && repoMap != "" {
		for _, pair := range strings.Split(repoMap, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				s.spaceRepoMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		slog.Info("loaded space-repo mappings", "module", moduleID, "count", len(s.spaceRepoMap))
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"scheduled_ingest", "freshness_check"},
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
		"uptime":                uptime,
		"executions_total":      strconv.FormatInt(executions, 10),
		"stale_threshold_hours": strconv.Itoa(s.staleThresholdHours),
		"cron_expression":       s.cronExpression,
		"configured_spaces":     strconv.Itoa(len(s.spaceRepoMap)),
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
		CronExpression:     s.cronExpression,
		EventTriggers:      []string{"source_changed", "manual_sync"},
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

	var stats *pb.ExecuteStats
	var message string
	var ingestErr string

	switch req.Trigger {
	case "event:source_changed":
		// Source changed event - check if specific space needs re-ingest
		spaceID := req.Context["space_id"]
		if spaceID != "" {
			slog.Info("source_changed event", "module", moduleID, "space_id", spaceID)
			if repoPath, ok := s.spaceRepoMap[spaceID]; ok {
				triggered, err := s.triggerIngest(spaceID, repoPath, true)
				if err != nil {
					ingestErr = err.Error()
				} else if triggered {
					stats = &pb.ExecuteStats{NodesCreated: 1}
					message = fmt.Sprintf("Triggered incremental ingest for space %s", spaceID)
				} else {
					message = fmt.Sprintf("Space %s is fresh, no ingest needed", spaceID)
				}
			} else {
				message = fmt.Sprintf("No repo path configured for space %s", spaceID)
			}
		} else {
			message = "source_changed event without space_id context"
		}

	case "event:manual_sync":
		// Manual sync - force re-ingest for specified space
		spaceID := req.Context["space_id"]
		if spaceID != "" {
			if repoPath, ok := s.spaceRepoMap[spaceID]; ok {
				triggered, err := s.triggerIngest(spaceID, repoPath, false) // Full ingest
				if err != nil {
					ingestErr = err.Error()
				} else if triggered {
					stats = &pb.ExecuteStats{NodesCreated: 1}
					message = fmt.Sprintf("Triggered full ingest for space %s", spaceID)
				}
			} else {
				message = fmt.Sprintf("No repo path configured for space %s", spaceID)
			}
		} else {
			message = "manual_sync event without space_id context"
		}

	case "schedule":
		// Scheduled check - query freshness and trigger for stale spaces
		staleCount, totalCount, err := s.checkAndTriggerStaleSpaces()
		if err != nil {
			ingestErr = err.Error()
		} else {
			stats = &pb.ExecuteStats{
				NodesCreated: int32(staleCount),
				DurationMs:   time.Since(start).Milliseconds(),
			}
			message = fmt.Sprintf("Checked %d spaces, triggered ingest for %d stale spaces",
				totalCount, staleCount)
		}

	default:
		message = fmt.Sprintf("Unknown trigger: %s", req.Trigger)
	}

	if stats == nil {
		stats = &pb.ExecuteStats{
			DurationMs: time.Since(start).Milliseconds(),
		}
	} else {
		stats.DurationMs = time.Since(start).Milliseconds()
	}

	slog.Info("task completed", "module", moduleID, "task_id", req.TaskId, "duration", time.Since(start), "message", message)

	return &pb.ExecuteResponse{
		Success: ingestErr == "",
		Message: message,
		Error:   ingestErr,
		Stats:   stats,
	}, nil
}

// checkAndTriggerStaleSpaces queries freshness for all configured spaces
// and triggers ingest for those that are stale.
func (s *server) checkAndTriggerStaleSpaces() (staleCount, totalCount int, err error) {
	if len(s.spaceRepoMap) == 0 {
		return 0, 0, nil
	}

	// Build list of space IDs
	spaceIDs := make([]string, 0, len(s.spaceRepoMap))
	for spaceID := range s.spaceRepoMap {
		spaceIDs = append(spaceIDs, spaceID)
	}
	totalCount = len(spaceIDs)

	// Query batch freshness
	freshnessData, err := s.queryBatchFreshness(spaceIDs)
	if err != nil {
		return 0, totalCount, fmt.Errorf("freshness query failed: %w", err)
	}

	// Check each space and trigger if stale
	for _, f := range freshnessData {
		spaceID, ok := f["space_id"].(string)
		if !ok {
			continue
		}

		isStale, ok := f["is_stale"].(bool)
		if !ok {
			continue
		}

		if isStale {
			repoPath, hasPath := s.spaceRepoMap[spaceID]
			if !hasPath {
				continue
			}

			slog.Info("space is stale, triggering incremental ingest", "module", moduleID, "space_id", spaceID)
			triggered, err := s.triggerIngest(spaceID, repoPath, true)
			if err != nil {
				slog.Error("failed to trigger ingest", "module", moduleID, "space_id", spaceID, "error", err)
			} else if triggered {
				staleCount++
			}
		}
	}

	return staleCount, totalCount, nil
}

// queryBatchFreshness calls the batch freshness endpoint.
func (s *server) queryBatchFreshness(spaceIDs []string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/v1/memory/freshness?space_ids=%s",
		strings.TrimRight(s.mdemgEndpoint, "/"),
		strings.Join(spaceIDs, ","))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("freshness query returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Spaces []map[string]any `json:"spaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode freshness response: %w", err)
	}

	return result.Spaces, nil
}

// triggerIngest calls the ingest/trigger endpoint to start a background ingest job.
func (s *server) triggerIngest(spaceID, repoPath string, incremental bool) (bool, error) {
	payload := map[string]any{
		"space_id":    spaceID,
		"path":        repoPath,
		"incremental": incremental,
	}

	if incremental {
		payload["since_commit"] = "HEAD~1"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("%s/v1/memory/ingest/trigger", strings.TrimRight(s.mdemgEndpoint, "/"))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("ingest trigger returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		JobID   string `json:"job_id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode trigger response: %w", err)
	}

	slog.Info("ingest job created", "module", moduleID, "job_id", result.JobID, "status", result.Status)
	return true, nil
}
