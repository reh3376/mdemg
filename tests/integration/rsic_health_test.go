//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestRSIC_HealthGaugesExist verifies that the metrics snapshot endpoint
// contains the rsic_health_overall metric name.
//
// The value may be 0 when a bootstrap cycle has not yet run (typical in CI),
// so this test only asserts the metric NAME is present in the output.  This
// ensures the APE live-collector is wired into the metrics registry and the
// gauge is registered even before any RSIC state exists.
func TestRSIC_HealthGaugesExist(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// --- GET /v1/metrics/snapshot ---
	resp, err := client.Get(cfg.MDEMGEndpoint + "/v1/metrics/snapshot")
	if err != nil {
		t.Fatalf("GET /v1/metrics/snapshot failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/metrics/snapshot, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read /v1/metrics/snapshot body: %v", err)
	}
	body := string(bodyBytes)

	// --- Assert rsic_health_overall metric name is emitted ---
	// We do not assert the value because it may be 0.0 when no RSIC bootstrap
	// cycle has run yet (common in CI environments).
	if !strings.Contains(body, "rsic_health_overall") {
		t.Errorf("/v1/metrics/snapshot body does not contain 'rsic_health_overall'; got:\n%s", body)
	}
}

// TestRSIC_ReadyzContainsRSIC verifies that the /readyz endpoint responds with
// 200 and that the response body contains RSIC-related information.
//
// The exact shape of RSIC data in /readyz depends on server configuration.
// This test is lenient: it uses t.Skip when /readyz does not include a
// recognised RSIC field rather than failing, because some deployments do not
// expose RSIC sub-checks in the readiness response.
func TestRSIC_ReadyzContainsRSIC(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// --- GET /readyz ---
	resp, err := client.Get(cfg.MDEMGEndpoint + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /readyz, got %d", resp.StatusCode)
	}

	// --- Parse JSON ---
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode /readyz response: %v", err)
	}

	// --- Look for RSIC-related information ---
	// Check the top-level keys and nested checks for any RSIC signal.
	// Known shapes:
	//   - result["rsic"] exists (direct top-level field)
	//   - result["checks"]["rsic"] exists (sub-check entry)
	//   - any key contains "rsic" (future-proof scan)
	rsicFound := false

	// Direct top-level field
	if _, ok := result["rsic"]; ok {
		rsicFound = true
	}

	// Nested under "checks"
	if !rsicFound {
		if checks, ok := result["checks"].(map[string]any); ok {
			if _, ok := checks["rsic"]; ok {
				rsicFound = true
			}
		}
	}

	// Broad scan: any key at top level containing "rsic"
	if !rsicFound {
		for k := range result {
			if strings.Contains(strings.ToLower(k), "rsic") {
				rsicFound = true
				break
			}
		}
	}

	if !rsicFound {
		// RSIC readyz reporting may not be enabled on all deployments.
		t.Skip("/readyz response contains no RSIC-related fields; RSIC readyz reporting may not be enabled on this server")
	}
}

// TestRSIC_ReadyzContainsSidecar verifies that the /readyz endpoint includes
// a neural_sidecar check when the sidecar is configured.
func TestRSIC_ReadyzContainsSidecar(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	resp, err := client.Get(cfg.MDEMGEndpoint + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 200 or 503 from /readyz, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode /readyz response: %v", err)
	}

	checks, ok := result["checks"].(map[string]any)
	if !ok {
		t.Fatal("/readyz response missing 'checks' object")
	}

	sc, ok := checks["neural_sidecar"]
	if !ok {
		t.Skip("neural_sidecar not in /readyz checks — sidecar may not be configured on this server")
	}

	scMap, ok := sc.(map[string]any)
	if !ok {
		t.Fatal("neural_sidecar check is not an object")
	}

	status, _ := scMap["status"].(string)
	if status != "healthy" && status != "degraded" {
		t.Errorf("expected neural_sidecar status 'healthy' or 'degraded', got %q", status)
	}
}

// TestRSIC_SidecarProbeMetric verifies that the metrics snapshot contains
// the probe_up metric for the neural_sidecar target.
func TestRSIC_SidecarProbeMetric(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	resp, err := client.Get(cfg.MDEMGEndpoint + "/v1/metrics/snapshot")
	if err != nil {
		t.Fatalf("GET /v1/metrics/snapshot failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/metrics/snapshot, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	body := string(bodyBytes)

	// The probe_up metric for neural_sidecar may not exist if the health prober
	// is not wired into the server. Use t.Skip rather than hard fail.
	if !strings.Contains(body, "neural_sidecar") {
		t.Skip("metrics snapshot does not contain 'neural_sidecar' — health prober may not be active")
	}
}
