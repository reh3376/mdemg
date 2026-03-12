package main

import (
	"context"
	"testing"

	pb "mdemg/api/modulepb"
)

func TestHandshake(t *testing.T) {
	s := newServer()

	resp, err := s.Handshake(context.Background(), &pb.HandshakeRequest{
		MdemgVersion: "1.0.0",
		SocketPath:   "/tmp/test.sock",
		Config: map[string]string{
			"spec_root":        t.TempDir(),
			"frameworks":       "uats",
			"mdemg_endpoint":   "http://localhost:9999",
			"space_id":         "test-space",
			"enable_drift_check": "false",
			"coverage_boost":   "0.03",
			"failure_penalty":  "0.10",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.ModuleId != moduleID {
		t.Errorf("expected module_id=%s, got %s", moduleID, resp.ModuleId)
	}
	if resp.ModuleVersion != moduleVersion {
		t.Errorf("expected version=%s, got %s", moduleVersion, resp.ModuleVersion)
	}
	if resp.ModuleType != pb.ModuleType_MODULE_TYPE_REASONING {
		t.Errorf("expected REASONING type, got %v", resp.ModuleType)
	}
	if !resp.Ready {
		t.Error("expected ready=true")
	}

	// Verify config was parsed
	if s.config.spaceID != "test-space" {
		t.Errorf("expected space_id=test-space, got %s", s.config.spaceID)
	}
	if s.config.coverageBoost != 0.03 {
		t.Errorf("expected coverage_boost=0.03, got %f", s.config.coverageBoost)
	}
	if s.config.failurePenalty != 0.10 {
		t.Errorf("expected failure_penalty=0.10, got %f", s.config.failurePenalty)
	}

	// Stop event handler to avoid goroutine leak
	if s.eventHandler != nil {
		s.eventHandler.Stop()
	}
}

func TestHealthCheck(t *testing.T) {
	s := newServer()

	resp, err := s.HealthCheck(context.Background(), &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if resp.Status != "ready" {
		t.Errorf("expected status=ready, got %s", resp.Status)
	}
	if resp.Metrics["total_specs"] != "0" {
		t.Errorf("expected total_specs=0, got %s", resp.Metrics["total_specs"])
	}
	if resp.Metrics["requests_handled"] != "0" {
		t.Errorf("expected requests_handled=0, got %s", resp.Metrics["requests_handled"])
	}
}

func TestShutdown(t *testing.T) {
	s := newServer()

	resp, err := s.Shutdown(context.Background(), &pb.ShutdownRequest{
		TimeoutMs: 5000,
		Reason:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}
