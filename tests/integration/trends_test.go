//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestTrends_Endpoint_ReturnsArray(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	url := cfg.MDEMGEndpoint + "/v1/metrics/trends?space_id=test-trends&metric=rsic_health_overall&from=2020-01-01&to=2020-01-02"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET trends failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response as array: %v", err)
	}

	// result should be a non-nil slice (possibly empty for a range with no data)
	if result == nil {
		t.Fatalf("expected non-nil slice, got nil")
	}
}

func TestTrends_Endpoint_MissingSpaceID(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	url := cfg.MDEMGEndpoint + "/v1/metrics/trends?metric=foo"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET trends failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing space_id, got %d", resp.StatusCode)
	}
}

func TestTrends_Endpoint_MissingMetric(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	url := cfg.MDEMGEndpoint + "/v1/metrics/trends?space_id=x"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET trends failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing metric, got %d", resp.StatusCode)
	}
}

func TestTrends_Endpoint_InvalidDate(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	url := cfg.MDEMGEndpoint + "/v1/metrics/trends?space_id=x&metric=y&from=bad"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET trends failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid date, got %d", resp.StatusCode)
	}
}

func TestTrends_Endpoint_MethodNotAllowed(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	url := cfg.MDEMGEndpoint + "/v1/metrics/trends"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST trends failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", resp.StatusCode)
	}
}
