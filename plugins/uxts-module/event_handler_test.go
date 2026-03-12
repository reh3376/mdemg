package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewEventHandler(t *testing.T) {
	idx := NewSpecIndex()
	rc := NewResultClient("http://localhost:9999", "test-space")
	cfg := pluginConfig{
		specRoot:  "./docs",
		frameworks: "uats",
		enableDrift: true,
	}

	eh := NewEventHandler(idx, rc, cfg)
	if eh.specIndex != idx {
		t.Error("specIndex not set")
	}
	if eh.resultClient != rc {
		t.Error("resultClient not set")
	}
}

func TestEventHandler_StartStop(t *testing.T) {
	idx := NewSpecIndex()
	rc := NewResultClient("http://127.0.0.1:1", "test-space")
	cfg := pluginConfig{specRoot: t.TempDir(), frameworks: "uats", enableDrift: false}

	eh := NewEventHandler(idx, rc, cfg)

	started := make(chan struct{})
	go func() {
		close(started)
		eh.Start()
	}()
	<-started

	// Give it a moment to enter the loop
	time.Sleep(50 * time.Millisecond)
	eh.Stop()
	// Should not hang — Stop() closes the channel
}

func TestEventHandler_PollRefreshesIndex(t *testing.T) {
	// Create a mock MDEMG server that returns distribution data
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"stats": map[string]any{
				"observation_count": 42,
			},
		})
	}))
	defer srv.Close()

	// Create temp spec dir with a spec file
	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specJSON := `{"uats_version":"1.0.0","request":{"path":"/v1/test"},"expected":{"status":200}}`
	if err := os.WriteFile(filepath.Join(uatsDir, "poll_test.uats.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	rc := NewResultClient(srv.URL, "test-space")
	cfg := pluginConfig{
		specRoot:       tmpDir,
		frameworks:     "uats",
		mdemgEndpoint:  srv.URL,
		spaceID:        "test-space",
		enableDrift:    false,
	}

	eh := NewEventHandler(idx, rc, cfg)
	eh.poll()

	// After poll, spec index should have loaded the spec
	if idx.TotalSpecs() != 1 {
		t.Errorf("expected 1 spec after poll, got %d", idx.TotalSpecs())
	}
}

func TestEventHandler_PollReportsDrift(t *testing.T) {
	var driftReported atomic.Int32

	// Mock MDEMG server: distribution + observe endpoints
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/memory/distribution":
			json.NewEncoder(w).Encode(map[string]any{
				"stats": map[string]any{"observation_count": 1},
			})
		case "/v1/conversation/observe":
			driftReported.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Create temp spec with a bad hash (will trigger drift)
	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specJSON := `{
		"uats_version": "1.0.0",
		"request": {"path": "/v1/test"},
		"expected": {"status": 200},
		"config": {"sha256": "wrong_hash_value"}
	}`
	if err := os.WriteFile(filepath.Join(uatsDir, "drift.uats.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	rc := NewResultClient(srv.URL, "test-space")
	cfg := pluginConfig{
		specRoot:      tmpDir,
		frameworks:    "uats",
		mdemgEndpoint: srv.URL,
		spaceID:       "test-space",
		enableDrift:   true,
	}

	eh := NewEventHandler(idx, rc, cfg)
	eh.poll()

	if idx.DriftCount() == 0 {
		t.Error("expected drift to be detected")
	}
	if driftReported.Load() == 0 {
		t.Error("expected drift to be reported via observe endpoint")
	}
}

func TestEventHandler_PollServerUnavailable(t *testing.T) {
	idx := NewSpecIndex()
	rc := NewResultClient("http://127.0.0.1:1", "test-space")
	cfg := pluginConfig{
		specRoot:      t.TempDir(),
		frameworks:    "uats",
		mdemgEndpoint: "http://127.0.0.1:1",
		enableDrift:   false,
	}

	eh := NewEventHandler(idx, rc, cfg)

	// Should not panic when server is unreachable
	eh.poll()

	if idx.TotalSpecs() != 0 {
		t.Errorf("expected 0 specs when server unavailable, got %d", idx.TotalSpecs())
	}
}

func TestFindDriftedSpecs(t *testing.T) {
	idx := NewSpecIndex()
	idx.mu.Lock()
	idx.byFramework["uats"] = []*SpecEntry{
		{FileName: "clean.uats.json", DriftState: "clean"},
		{FileName: "drifted1.uats.json", DriftState: "drift_detected"},
		{FileName: "drifted2.uats.json", DriftState: "drift_detected"},
	}
	idx.mu.Unlock()

	rc := NewResultClient("http://localhost:9999", "test-space")
	cfg := pluginConfig{}
	eh := NewEventHandler(idx, rc, cfg)

	drifted := eh.findDriftedSpecs("uats")
	if len(drifted) != 2 {
		t.Errorf("expected 2 drifted specs, got %d", len(drifted))
	}

	drifted = eh.findDriftedSpecs("nonexistent")
	if len(drifted) != 0 {
		t.Errorf("expected 0 for nonexistent framework, got %d", len(drifted))
	}
}
