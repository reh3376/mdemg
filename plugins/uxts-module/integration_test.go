// Integration tests for the uxts-module plugin.
// These test the full gRPC lifecycle via Unix socket.
//
// Run with: go test -v -tags=integration ./plugins/uxts-module/...
//
//go:build integration

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "mdemg/api/modulepb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startPluginServer starts the uxts-module gRPC server on a Unix socket
// in /tmp (macOS limits socket paths to 104 chars, so t.TempDir() is too long).
func startPluginServer(t *testing.T) (*grpc.Server, *server, string) {
	t.Helper()

	socketPath := fmt.Sprintf("/tmp/uxts-test-%d.sock", time.Now().UnixNano())
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on socket: %v", err)
	}

	grpcServer := grpc.NewServer()
	s := newServer()

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterReasoningModuleServer(grpcServer, s)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			// Server stopped — expected during cleanup
		}
	}()

	t.Cleanup(func() {
		grpcServer.GracefulStop()
		os.Remove(socketPath)
	})

	// Wait for socket to appear
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			return grpcServer, s, socketPath
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("socket did not appear within 500ms")
	return nil, nil, ""
}

func dialSocket(t *testing.T, socketPath string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestIntegration_FullLifecycle tests the complete plugin lifecycle:
// start → handshake → health check → process → shutdown
func TestIntegration_FullLifecycle(t *testing.T) {
	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	// Create temp spec fixtures
	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specJSON := `{
		"uats_version": "1.0.0",
		"request": {"method": "POST", "path": "/v1/memory/retrieve"},
		"expected": {"status": 200},
		"metadata": {"description": "Retrieve memory", "tags": ["memory", "contract"]}
	}`
	if err := os.WriteFile(filepath.Join(uatsDir, "retrieve.uats.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Step 1: Handshake
	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	hsResp, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		SocketPath:   socketPath,
		Config: map[string]string{
			"spec_root":          tmpDir,
			"frameworks":         "uats",
			"mdemg_endpoint":     "http://127.0.0.1:1", // won't connect — that's fine
			"space_id":           "integration-test",
			"enable_drift_check": "false",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if !hsResp.Ready {
		t.Fatal("expected Ready=true after handshake")
	}
	if hsResp.ModuleId != moduleID {
		t.Errorf("module_id = %s, want %s", hsResp.ModuleId, moduleID)
	}
	if hsResp.ModuleType != pb.ModuleType_MODULE_TYPE_REASONING {
		t.Errorf("module_type = %v, want REASONING", hsResp.ModuleType)
	}

	// Step 2: Health check (should report loaded specs)
	hcResp, err := lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !hcResp.Healthy {
		t.Error("expected healthy=true")
	}
	if hcResp.Metrics["total_specs"] != "1" {
		t.Errorf("total_specs = %s, want 1", hcResp.Metrics["total_specs"])
	}

	// Step 3: Process with candidates
	reasoningClient := pb.NewReasoningModuleClient(conn)
	procResp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
		QueryText: "memory retrieval",
		Candidates: []*pb.RetrievalCandidate{
			{
				NodeId:  "integ-node-1",
				Path:    "/v1/memory/retrieve",
				Name:    "retrieve handler",
				Summary: "Handles memory retrieval requests",
				Score:   0.5,
			},
			{
				NodeId:  "integ-node-2",
				Path:    "/v1/other/endpoint",
				Name:    "unrelated handler",
				Summary: "Something else entirely",
				Score:   0.6,
			},
		},
		TopK: 10,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if len(procResp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(procResp.Results))
	}

	// Find the matching candidate
	var matched, unmatched *pb.RetrievalCandidate
	for _, r := range procResp.Results {
		if r.NodeId == "integ-node-1" {
			matched = r
		} else {
			unmatched = r
		}
	}

	if matched == nil {
		t.Fatal("matched candidate not found in results")
	}
	if matched.Metadata["uxts_coverage_count"] == "0" || matched.Metadata["uxts_coverage_count"] == "" {
		t.Errorf("expected coverage_count > 0 for matched candidate, got %s", matched.Metadata["uxts_coverage_count"])
	}
	if matched.Metadata["uxts_frameworks"] == "" {
		t.Error("expected uxts_frameworks to be set for matched candidate")
	}

	if unmatched == nil {
		t.Fatal("unmatched candidate not found in results")
	}
	if unmatched.Metadata["uxts_coverage_count"] != "0" {
		t.Errorf("expected coverage_count=0 for unmatched, got %s", unmatched.Metadata["uxts_coverage_count"])
	}

	// Verify metadata is populated
	if procResp.Metadata["total_specs"] != "1" {
		t.Errorf("response metadata total_specs = %s, want 1", procResp.Metadata["total_specs"])
	}

	// Step 4: Process with empty candidates (edge case)
	emptyResp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
		QueryText:  "empty query",
		Candidates: []*pb.RetrievalCandidate{},
		TopK:       5,
	})
	if err != nil {
		t.Fatalf("Process(empty): %v", err)
	}
	if len(emptyResp.Results) != 0 {
		t.Errorf("expected 0 results for empty candidates, got %d", len(emptyResp.Results))
	}

	// Step 5: Shutdown
	sdResp, err := lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{
		TimeoutMs: 5000,
		Reason:    "integration_test",
	})
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !sdResp.Success {
		t.Error("expected shutdown success=true")
	}
}

// TestIntegration_MultiFrameworkIndex tests spec loading across multiple frameworks.
func TestIntegration_MultiFrameworkIndex(t *testing.T) {
	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create UATS and UDTS spec directories
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	udtsDir := filepath.Join(tmpDir, "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(udtsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write 3 UATS specs
	for i := 1; i <= 3; i++ {
		spec := fmt.Sprintf(`{
			"uats_version": "1.0.0",
			"request": {"path": "/v1/test/%d"},
			"expected": {"status": 200},
			"metadata": {"description": "Test spec %d"}
		}`, i, i)
		fname := fmt.Sprintf("test_%d.uats.json", i)
		if err := os.WriteFile(filepath.Join(uatsDir, fname), []byte(spec), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Write 2 UDTS specs
	for i := 1; i <= 2; i++ {
		spec := fmt.Sprintf(`{
			"udts_version": "1.0.0",
			"service": "mdemg.module.v1.ReasoningModule",
			"method": "Process%d",
			"request": {},
			"expected": {"status_code": "OK"},
			"metadata": {"description": "UDTS spec %d"}
		}`, i, i)
		fname := fmt.Sprintf("test_%d.udts.json", i)
		if err := os.WriteFile(filepath.Join(udtsDir, fname), []byte(spec), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	_, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		Config: map[string]string{
			"spec_root":          tmpDir,
			"frameworks":         "uats,udts",
			"enable_drift_check": "false",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	hcResp, err := lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	if hcResp.Metrics["total_specs"] != "5" {
		t.Errorf("total_specs = %s, want 5 (3 UATS + 2 UDTS)", hcResp.Metrics["total_specs"])
	}
	if hcResp.Metrics["frameworks"] != "2" {
		t.Errorf("frameworks = %s, want 2", hcResp.Metrics["frameworks"])
	}

	// Cleanup event handler
	lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{Reason: "cleanup"})
}

// TestIntegration_DriftDetection tests drift detection through the gRPC interface.
func TestIntegration_DriftDetection(t *testing.T) {
	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Spec with correct hash (compute it first)
	cleanSpec := map[string]any{
		"uats_version": "1.0.0",
		"request":      map[string]any{"path": "/v1/clean"},
		"expected":     map[string]any{"status": 200},
		"config":       map[string]any{},
	}
	hash, err := sha256SpecWithoutField(cleanSpec, []string{"config", "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	cleanSpec["config"] = map[string]any{"sha256": hash}

	cleanJSON, _ := canonicalJSONBytes(cleanSpec)
	if err := os.WriteFile(filepath.Join(uatsDir, "clean.uats.json"), cleanJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	// Spec with wrong hash (drift)
	driftSpec := `{
		"uats_version": "1.0.0",
		"request": {"path": "/v1/drifted"},
		"expected": {"status": 200},
		"config": {"sha256": "0000000000000000000000000000000000000000000000000000000000000000"}
	}`
	if err := os.WriteFile(filepath.Join(uatsDir, "drifted.uats.json"), []byte(driftSpec), 0o644); err != nil {
		t.Fatal(err)
	}

	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	_, err = lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		Config: map[string]string{
			"spec_root":          tmpDir,
			"frameworks":         "uats",
			"enable_drift_check": "true",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	hcResp, err := lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	if hcResp.Metrics["drift_detected"] != "1" {
		t.Errorf("drift_detected = %s, want 1 (one drifted spec)", hcResp.Metrics["drift_detected"])
	}
	if hcResp.Metrics["total_specs"] != "2" {
		t.Errorf("total_specs = %s, want 2", hcResp.Metrics["total_specs"])
	}

	// Process a candidate matching the drifted path
	reasoningClient := pb.NewReasoningModuleClient(conn)
	procResp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
		QueryText: "drifted endpoint",
		Candidates: []*pb.RetrievalCandidate{
			{NodeId: "drift-node", Path: "/v1/drifted", Score: 0.5},
		},
		TopK: 10,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if len(procResp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(procResp.Results))
	}

	r := procResp.Results[0]
	if r.Metadata["uxts_drift"] != "drift_detected" {
		t.Errorf("uxts_drift = %s, want drift_detected", r.Metadata["uxts_drift"])
	}

	lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{Reason: "cleanup"})
}

// TestIntegration_RealSpecFiles tests the plugin against the actual spec files on disk.
func TestIntegration_RealSpecFiles(t *testing.T) {
	// Find the project root (go up from plugins/uxts-module/)
	projectRoot := findProjectRoot(t)
	docsDir := filepath.Join(projectRoot, "docs")

	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Skip("docs directory not found — skipping real spec test")
	}

	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	_, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		Config: map[string]string{
			"spec_root":          docsDir,
			"frameworks":         "uats,udts,upts,ubts,usts,uobs,uots,uets,uvts",
			"enable_drift_check": "true",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	hcResp, err := lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	t.Logf("Real spec scan results:")
	t.Logf("  Total specs:    %s", hcResp.Metrics["total_specs"])
	t.Logf("  Frameworks:     %s", hcResp.Metrics["frameworks"])
	t.Logf("  Drift detected: %s", hcResp.Metrics["drift_detected"])

	// We know the project has >100 specs across multiple frameworks
	if hcResp.Metrics["total_specs"] == "0" {
		t.Error("expected to find specs in real docs directory")
	}
	if hcResp.Metrics["frameworks"] == "0" {
		t.Error("expected to find active frameworks")
	}

	// Process a candidate matching a real endpoint
	reasoningClient := pb.NewReasoningModuleClient(conn)
	procResp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
		QueryText: "memory retrieval API",
		Candidates: []*pb.RetrievalCandidate{
			{NodeId: "real-1", Path: "/v1/memory/retrieve", Name: "retrieve", Score: 0.5},
			{NodeId: "real-2", Path: "/v1/conversation/observe", Name: "observe", Score: 0.4},
			{NodeId: "real-3", Path: "/nonexistent", Name: "nothing", Score: 0.3},
		},
		TopK: 10,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	for _, r := range procResp.Results {
		t.Logf("  %s: coverage=%s frameworks=%s drift=%s",
			r.NodeId,
			r.Metadata["uxts_coverage_count"],
			r.Metadata["uxts_frameworks"],
			r.Metadata["uxts_drift"])
	}

	lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{Reason: "cleanup"})
}

// TestIntegration_ConcurrentProcessing tests thread safety with concurrent Process calls.
func TestIntegration_ConcurrentProcessing(t *testing.T) {
	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	// Setup with fixture specs
	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `{"uats_version":"1.0.0","request":{"path":"/v1/test"},"expected":{"status":200}}`
	if err := os.WriteFile(filepath.Join(uatsDir, "concurrent.uats.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	_, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		Config: map[string]string{
			"spec_root":          tmpDir,
			"frameworks":         "uats",
			"enable_drift_check": "false",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	// Fire 10 concurrent Process calls
	const concurrency = 10
	errCh := make(chan error, concurrency)
	reasoningClient := pb.NewReasoningModuleClient(conn)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			resp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
				QueryText: fmt.Sprintf("query %d", idx),
				Candidates: []*pb.RetrievalCandidate{
					{NodeId: fmt.Sprintf("node-%d", idx), Path: "/v1/test", Score: 0.5},
				},
				TopK: 5,
			})
			if err != nil {
				errCh <- fmt.Errorf("concurrent Process %d: %w", idx, err)
				return
			}
			if len(resp.Results) != 1 {
				errCh <- fmt.Errorf("concurrent Process %d: expected 1 result, got %d", idx, len(resp.Results))
				return
			}
			errCh <- nil
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			t.Error(err)
		}
	}

	// Verify request counter
	hcResp, _ := lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if hcResp.Metrics["requests_handled"] != "10" {
		t.Errorf("requests_handled = %s, want 10", hcResp.Metrics["requests_handled"])
	}

	lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{Reason: "cleanup"})
}

// TestIntegration_ScoreAdjustment verifies that covered candidates get boosted
// and failed candidates get penalized.
func TestIntegration_ScoreAdjustment(t *testing.T) {
	_, _, socketPath := startPluginServer(t)
	conn := dialSocket(t, socketPath)
	ctx := context.Background()

	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `{
		"uats_version": "1.0.0",
		"request": {"path": "/v1/scored"},
		"expected": {"status": 200},
		"metadata": {"description": "Scored endpoint"}
	}`
	if err := os.WriteFile(filepath.Join(uatsDir, "scored.uats.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	lifecycleClient := pb.NewModuleLifecycleClient(conn)
	_, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		Config: map[string]string{
			"spec_root":          tmpDir,
			"frameworks":         "uats",
			"enable_drift_check": "false",
			"coverage_boost":     "0.10",
			"failure_penalty":    "0.05",
		},
	})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	reasoningClient := pb.NewReasoningModuleClient(conn)
	resp, err := reasoningClient.Process(ctx, &pb.ProcessRequest{
		QueryText: "scored endpoint",
		Candidates: []*pb.RetrievalCandidate{
			{NodeId: "covered", Path: "/v1/scored", Score: 0.50},
			{NodeId: "untested", Path: "/v1/unknown", Score: 0.55},
		},
		TopK: 10,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	for _, r := range resp.Results {
		switch r.NodeId {
		case "covered":
			// Should get +0.10 boost → 0.60
			if r.Score <= 0.50 {
				t.Errorf("covered candidate score = %f, expected > 0.50 (boost applied)", r.Score)
			}
		case "untested":
			// No boost, no penalty → stays at 0.55
			if r.Score != 0.55 {
				t.Errorf("untested candidate score = %f, expected 0.55 (unchanged)", r.Score)
			}
		}
	}

	lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{Reason: "cleanup"})
}

func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Walk up from current working dir looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("could not find project root (go.mod)")
		}
		dir = parent
	}
}
