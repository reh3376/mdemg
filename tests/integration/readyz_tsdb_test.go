//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestReadyz_ContainsTSDBCheck(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	resp, err := client.Get(cfg.MDEMGEndpoint + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /readyz, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode /readyz response: %v", err)
	}

	// Look for checks.timescaledb in the response
	checks, ok := result["checks"].(map[string]any)
	if !ok {
		t.Skip("no 'checks' object in /readyz response; TSDB may not be enabled on this server")
	}

	_, hasTSDB := checks["timescaledb"]
	if !hasTSDB {
		t.Skip("no 'timescaledb' check in /readyz response; TSDB not enabled on this server")
	}
}

func TestReadyz_TSDBSchemaVersion(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	resp, err := client.Get(cfg.MDEMGEndpoint + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /readyz, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode /readyz response: %v", err)
	}

	checks, ok := result["checks"].(map[string]any)
	if !ok {
		t.Skip("no 'checks' object in /readyz response")
	}

	tsdbCheck, ok := checks["timescaledb"].(map[string]any)
	if !ok {
		t.Skip("no 'timescaledb' check in /readyz response; TSDB not enabled")
	}

	// Verify status is "healthy"
	status, _ := tsdbCheck["status"].(string)
	if status != "healthy" {
		t.Errorf("expected timescaledb status 'healthy', got %q", status)
	}

	// Verify message contains "schema version"
	message := fmt.Sprintf("%v", tsdbCheck["message"])
	if !strings.Contains(strings.ToLower(message), "schema version") {
		t.Errorf("expected timescaledb message to contain 'schema version', got %q", message)
	}
}
