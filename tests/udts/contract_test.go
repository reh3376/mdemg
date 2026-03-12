// Package udts runs UDTS (Universal DevSpace Test Specification) contract tests
// against gRPC services. Set UDTS_TARGET (e.g. localhost:50051) to run; tests skip if unset.
package udts

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	devspacepb "mdemg/api/devspacepb"
	modulepb "mdemg/api/modulepb"
	pb "mdemg/api/transferpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Spec represents a minimal UDTS spec for running a gRPC test.
type Spec struct {
	UDTSVersion string          `json:"udts_version"`
	Service     string          `json:"service"`
	Method      string          `json:"method"`
	Request     json.RawMessage `json:"request"`
	Expected    struct {
		StatusCode string `json:"status_code"`
		HasField   string `json:"response_has_field"`
	} `json:"expected"`
	Config struct {
		TimeoutMs   int    `json:"timeout_ms"`
		ProtoSHA256 string `json:"proto_sha256"`
	} `json:"config"`
}

func getTarget(t *testing.T) string {
	t.Helper()
	target := os.Getenv("UDTS_TARGET")
	if target == "" {
		t.Skip("UDTS_TARGET not set (e.g. localhost:50051); start space-transfer serve first")
	}
	return target
}

func loadSpec(t *testing.T, name string) *Spec {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "api", "api-spec", "udts", "specs", name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("docs", "api", "api-spec", "udts", "specs", name)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("UDTS spec not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return &s
}

// verifyProtoHash checks proto_sha256 in spec against the given proto file when set.
func verifyProtoHash(t *testing.T, spec *Spec, protoRelPath string) {
	t.Helper()
	if spec.Config.ProtoSHA256 == "" {
		return
	}
	if protoRelPath == "" {
		protoRelPath = "space-transfer.proto"
	}
	for _, base := range []string{filepath.Join("..", ".."), "."} {
		protoPath := filepath.Join(base, "api", "proto", protoRelPath)
		if _, err := os.Stat(protoPath); err == nil {
			data, err := os.ReadFile(protoPath)
			if err != nil {
				t.Fatalf("read proto for hash: %v", err)
			}
			sum := sha256.Sum256(data)
			got := fmt.Sprintf("%x", sum)
			if got != spec.Config.ProtoSHA256 {
				t.Errorf("proto hash mismatch: spec has %s, current proto is %s (update spec or proto)", spec.Config.ProtoSHA256, got)
			}
			return
		}
	}
	t.Fatalf("proto file not found: api/proto/%s", protoRelPath)
}

// TestSpaceTransferListSpaces runs the ListSpaces contract from UDTS spec.
func TestSpaceTransferListSpaces(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "space_transfer_list_spaces.udts.json")
	verifyProtoHash(t, spec, "space-transfer.proto")
	if spec.Service != "mdemg.transfer.v1.SpaceTransfer" || spec.Method != "ListSpaces" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := pb.NewSpaceTransferClient(conn)
	resp, err := client.ListSpaces(ctx, &pb.ListSpacesRequest{})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("ListSpaces: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	if spec.Expected.StatusCode != "OK" {
		t.Fatalf("expected status %s but got OK", spec.Expected.StatusCode)
	}
	if spec.Expected.HasField == "spaces" && resp.Spaces == nil {
		t.Error("expected response to have 'spaces' field (non-nil)")
	}
}

// TestSpaceTransferSpaceInfo runs the SpaceInfo contract from UDTS spec.
func TestSpaceTransferSpaceInfo(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "space_transfer_space_info.udts.json")
	verifyProtoHash(t, spec, "space-transfer.proto")
	if spec.Service != "mdemg.transfer.v1.SpaceTransfer" || spec.Method != "SpaceInfo" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := pb.NewSpaceTransferClient(conn)
	resp, err := client.SpaceInfo(ctx, &pb.SpaceInfoRequest{SpaceId: "demo"})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("SpaceInfo: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	if spec.Expected.StatusCode != "OK" {
		t.Fatalf("expected status %s but got OK", spec.Expected.StatusCode)
	}
	if spec.Expected.HasField == "summary" && resp.Summary == nil {
		t.Error("expected response to have 'summary' field (non-nil)")
	}
}

// TestDevSpaceRegisterAgent runs the RegisterAgent contract from UDTS spec.
func TestDevSpaceRegisterAgent(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "devspace_register_agent.udts.json")
	verifyProtoHash(t, spec, "devspace.proto")
	if spec.Service != "mdemg.devspace.v1.DevSpace" || spec.Method != "RegisterAgent" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := devspacepb.NewDevSpaceClient(conn)
	resp, err := client.RegisterAgent(ctx, &devspacepb.RegisterAgentRequest{
		DevSpaceId: "udts-test-space",
		AgentId:    "udts-agent-1",
		Metadata:   map[string]string{},
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("RegisterAgent: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	if spec.Expected.StatusCode != "OK" {
		t.Fatalf("expected status %s but got OK", spec.Expected.StatusCode)
	}
	if spec.Expected.HasField == "ok" && !resp.Ok {
		t.Error("expected response to have ok true")
	}
}

// TestDevSpaceListExports runs the ListExports contract from UDTS spec.
func TestDevSpaceListExports(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "devspace_list_exports.udts.json")
	verifyProtoHash(t, spec, "devspace.proto")
	if spec.Service != "mdemg.devspace.v1.DevSpace" || spec.Method != "ListExports" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := devspacepb.NewDevSpaceClient(conn)
	resp, err := client.ListExports(ctx, &devspacepb.ListExportsRequest{DevSpaceId: "udts-test-space"})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("ListExports: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	if spec.Expected.StatusCode != "OK" {
		t.Fatalf("expected status %s but got OK", spec.Expected.StatusCode)
	}
	// In proto3, empty repeated fields are indistinguishable from unset fields on the wire.
	// A successful response with status OK is sufficient; exports can be nil or empty for an empty list.
	if spec.Expected.HasField == "exports" && resp == nil {
		t.Error("expected non-nil response")
	}
}

// TestDevSpacePullExport runs the PullExport contract (NOT_FOUND for unknown export).
func TestDevSpacePullExport(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "devspace_pull_export.udts.json")
	verifyProtoHash(t, spec, "devspace.proto")
	if spec.Service != "mdemg.devspace.v1.DevSpace" || spec.Method != "PullExport" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := devspacepb.NewDevSpaceClient(conn)
	stream, err := client.PullExport(ctx, &devspacepb.PullExportRequest{
		DevSpaceId: "udts-test-space",
		ExportId:   "nonexistent-id",
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("PullExport: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	// If no error, stream is valid; consume one chunk to see if server sends then errors
	_, err = stream.Recv()
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("PullExport stream: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	// We expected NOT_FOUND but got a chunk; that's wrong for nonexistent-id
	if spec.Expected.StatusCode == "NOT_FOUND" {
		t.Error("expected NOT_FOUND for nonexistent export_id but received stream chunk")
	}
}

// TestSpaceTransferExportDelta runs the Export (delta) contract (Phase 4).
func TestSpaceTransferExportDelta(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "space_transfer_export_delta.udts.json")
	verifyProtoHash(t, spec, "space-transfer.proto")
	if spec.Service != "mdemg.transfer.v1.SpaceTransfer" || spec.Method != "Export" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 15 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := pb.NewSpaceTransferClient(conn)
	stream, err := client.Export(ctx, &pb.ExportRequest{
		SpaceId:        "demo",
		SinceTimestamp: "2020-01-01T00:00:00Z",
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Export: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	var lastChunk *pb.SpaceChunk
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Export Recv: %v", err)
		}
		lastChunk = chunk
	}
	if lastChunk == nil {
		t.Fatal("Export stream produced no chunks")
	}
	if lastChunk.ChunkType != pb.ChunkType_CHUNK_TYPE_SUMMARY {
		t.Errorf("last chunk type: got %v, want SUMMARY", lastChunk.ChunkType)
	}
	if sum := lastChunk.GetSummary(); sum != nil && sum.NextCursor == "" {
		t.Error("Phase 4 delta export: expected summary to have next_cursor")
	}
}

// TestDevSpaceConnect runs the Connect contract (Phase 3 bidi stream).
func TestDevSpaceConnect(t *testing.T) {
	target := getTarget(t)
	spec := loadSpec(t, "devspace_connect.udts.json")
	verifyProtoHash(t, spec, "devspace.proto")
	if spec.Service != "mdemg.devspace.v1.DevSpace" || spec.Method != "Connect" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer conn.Close()

	client := devspacepb.NewDevSpaceClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Connect: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}
	if err := stream.Send(&devspacepb.AgentMessage{
		DevSpaceId: "udts-test-space",
		AgentId:    "udts-connect-agent",
	}); err != nil {
		t.Fatalf("Connect Send: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("Connect CloseSend: %v", err)
	}
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Connect Recv: %v", err)
		}
	}
}

// getModuleTarget returns the Unix socket target for uxts-module UDTS tests.
// Set UDTS_MODULE_SOCKET (e.g. /tmp/mdemg-plugins/mdemg-uxts-module.sock) to run.
func getModuleTarget(t *testing.T) string {
	t.Helper()
	socketPath := os.Getenv("UDTS_MODULE_SOCKET")
	if socketPath == "" {
		t.Skip("UDTS_MODULE_SOCKET not set; start uxts-module first")
	}
	return socketPath
}

func dialModule(t *testing.T, socketPath string, _ time.Duration) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial module socket %s: %v", socketPath, err)
	}
	return conn
}

// TestUxTSModuleHandshake validates the Handshake RPC contract.
func TestUxTSModuleHandshake(t *testing.T) {
	socketPath := getModuleTarget(t)
	spec := loadSpec(t, "uxts_module_handshake.udts.json")
	verifyProtoHash(t, spec, "mdemg-module.proto")
	if spec.Service != "mdemg.module.v1.ModuleLifecycle" || spec.Method != "Handshake" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}

	conn := dialModule(t, socketPath, timeout)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := modulepb.NewModuleLifecycleClient(conn)
	resp, err := client.Handshake(ctx, &modulepb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		SocketPath:   socketPath,
		Config: map[string]string{
			"spec_root":          "./docs",
			"frameworks":         "uats",
			"enable_drift_check": "false",
		},
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Handshake: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}

	if spec.Expected.HasField == "module_id" && resp.ModuleId == "" {
		t.Error("expected response to have module_id")
	}
	if resp.ModuleType != modulepb.ModuleType_MODULE_TYPE_REASONING {
		t.Errorf("expected REASONING type, got %v", resp.ModuleType)
	}
}

// TestUxTSModuleHealthCheck validates the HealthCheck RPC contract.
func TestUxTSModuleHealthCheck(t *testing.T) {
	socketPath := getModuleTarget(t)
	spec := loadSpec(t, "uxts_module_healthcheck.udts.json")
	verifyProtoHash(t, spec, "mdemg-module.proto")
	if spec.Service != "mdemg.module.v1.ModuleLifecycle" || spec.Method != "HealthCheck" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}

	conn := dialModule(t, socketPath, timeout)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := modulepb.NewModuleLifecycleClient(conn)
	resp, err := client.HealthCheck(ctx, &modulepb.HealthCheckRequest{})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("HealthCheck: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}

	if spec.Expected.HasField == "healthy" && !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if resp.Metrics == nil || resp.Metrics["total_specs"] == "" {
		t.Error("expected metrics to include total_specs")
	}
}

// TestUxTSModuleShutdown validates the Shutdown RPC contract.
func TestUxTSModuleShutdown(t *testing.T) {
	socketPath := getModuleTarget(t)
	spec := loadSpec(t, "uxts_module_shutdown.udts.json")
	verifyProtoHash(t, spec, "mdemg-module.proto")
	if spec.Service != "mdemg.module.v1.ModuleLifecycle" || spec.Method != "Shutdown" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}

	conn := dialModule(t, socketPath, timeout)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := modulepb.NewModuleLifecycleClient(conn)
	resp, err := client.Shutdown(ctx, &modulepb.ShutdownRequest{
		TimeoutMs: 5000,
		Reason:    "udts_contract_test",
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Shutdown: %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}

	if spec.Expected.HasField == "success" && !resp.Success {
		t.Error("expected success=true")
	}
}

// TestUxTSModuleProcessEmpty validates Process with empty candidates.
func TestUxTSModuleProcessEmpty(t *testing.T) {
	socketPath := getModuleTarget(t)
	spec := loadSpec(t, "uxts_module_process_empty.udts.json")
	verifyProtoHash(t, spec, "mdemg-module.proto")
	if spec.Service != "mdemg.module.v1.ReasoningModule" || spec.Method != "Process" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}

	conn := dialModule(t, socketPath, timeout)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := modulepb.NewReasoningModuleClient(conn)
	resp, err := client.Process(ctx, &modulepb.ProcessRequest{
		QueryText:  "empty test",
		Candidates: []*modulepb.RetrievalCandidate{},
		TopK:       10,
		Context:    map[string]string{},
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Process(empty): %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}

	if spec.Expected.HasField == "metadata" && resp.Metadata == nil {
		t.Error("expected response to have metadata")
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results for empty candidates, got %d", len(resp.Results))
	}
}

// TestUxTSModuleProcessWithCandidates validates Process annotates candidates.
func TestUxTSModuleProcessWithCandidates(t *testing.T) {
	socketPath := getModuleTarget(t)
	spec := loadSpec(t, "uxts_module_process_with_candidates.udts.json")
	verifyProtoHash(t, spec, "mdemg-module.proto")
	if spec.Service != "mdemg.module.v1.ReasoningModule" || spec.Method != "Process" {
		t.Fatalf("spec mismatch: %s / %s", spec.Service, spec.Method)
	}

	timeout := 10 * time.Second
	if spec.Config.TimeoutMs > 0 {
		timeout = time.Duration(spec.Config.TimeoutMs) * time.Millisecond
	}

	conn := dialModule(t, socketPath, timeout)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := modulepb.NewReasoningModuleClient(conn)
	resp, err := client.Process(ctx, &modulepb.ProcessRequest{
		QueryText: "memory retrieval test",
		Candidates: []*modulepb.RetrievalCandidate{
			{
				NodeId:  "udts-node-1",
				Path:    "/v1/memory/retrieve",
				Name:    "retrieve handler",
				Summary: "Handles memory retrieval requests",
				Score:   0.5,
			},
			{
				NodeId:  "udts-node-2",
				Path:    "/v1/other",
				Name:    "other handler",
				Summary: "Unrelated endpoint",
				Score:   0.4,
			},
		},
		TopK:    10,
		Context: map[string]string{},
	})
	if err != nil {
		st, _ := status.FromError(err)
		if spec.Expected.StatusCode != st.Code().String() {
			t.Fatalf("Process(with candidates): %v (expected status %s)", err, spec.Expected.StatusCode)
		}
		return
	}

	if spec.Expected.HasField == "results" && len(resp.Results) == 0 {
		t.Error("expected results to be non-empty")
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}

	// Verify candidates have UxTS metadata annotations
	for _, r := range resp.Results {
		if r.Metadata == nil {
			t.Errorf("candidate %s: expected metadata to be set", r.NodeId)
			continue
		}
		if _, ok := r.Metadata["uxts_coverage_count"]; !ok {
			t.Errorf("candidate %s: expected uxts_coverage_count metadata", r.NodeId)
		}
	}
}
