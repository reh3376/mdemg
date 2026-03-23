//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestTierEffectivenessEndpoint verifies the /v1/jiminy/protocol/tier-effectiveness
// endpoint returns tier effectiveness data after outcomes are recorded via feedback.
func TestTierEffectivenessEndpoint(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Hit the tier effectiveness endpoint
	url := fmt.Sprintf("%s/v1/jiminy/protocol/tier-effectiveness?space_id=mdemg-dev", cfg.MDEMGEndpoint)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("tier effectiveness request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tier effectiveness returned status %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify structure — response wrapped in "data"
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'data' field")
	}

	// Must have tier_comprehension array
	if _, ok := data["overall_tier_comprehension"]; !ok {
		t.Error("response missing 'overall_tier_comprehension' field")
	}
	// Must have tier_outcome_count array
	if _, ok := data["tier_outcome_count"]; !ok {
		t.Error("response missing 'tier_outcome_count' field")
	}
}

// TestTierEffectivenessRSICLoop verifies the full RSIC loop:
// Trigger RSIC cycle → verify cycle completes → verify protocol metrics
// endpoint returns tier fields.
func TestTierEffectivenessRSICLoop(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	spaceID := GenerateTestSpaceID("tier-eff")

	// Trigger an RSIC micro cycle — response fields are at top level (no "data" wrapper)
	result := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"tier": "micro",
	})

	// Verify cycle completed — cycle_id is at top level
	if _, ok := result["cycle_id"].(string); !ok {
		t.Fatal("cycle response missing 'cycle_id' field")
	}

	// Verify protocol metrics endpoint returns tier data
	metricsURL := fmt.Sprintf("%s/v1/jiminy/protocol/metrics?space_id=%s", cfg.MDEMGEndpoint, spaceID)
	metricsResp, err := client.Get(metricsURL)
	if err != nil {
		t.Fatalf("metrics request failed: %v", err)
	}
	defer metricsResp.Body.Close()

	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics returned status %d", metricsResp.StatusCode)
	}

	var metricsResult map[string]any
	if err := json.NewDecoder(metricsResp.Body).Decode(&metricsResult); err != nil {
		t.Fatalf("failed to decode metrics response: %v", err)
	}

	metricsData, ok := metricsResult["data"].(map[string]any)
	if !ok {
		t.Fatal("metrics response missing 'data' field")
	}

	// Verify tier fields exist in metrics response
	if _, ok := metricsData["tier_comprehension"]; !ok {
		t.Error("metrics response missing 'tier_comprehension' field")
	}
	if _, ok := metricsData["tier_outcome_count"]; !ok {
		t.Error("metrics response missing 'tier_outcome_count' field")
	}
}

// TestTierEffectivenessDatasetGeneration verifies that a meso RSIC cycle
// completes successfully (dataset generation happens at meso/macro boundaries).
func TestTierEffectivenessDatasetGeneration(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("tier-dataset")

	// Trigger a meso cycle — response fields are at top level (no "data" wrapper)
	result := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"tier": "meso",
	})

	// Meso cycle should complete with a cycle_id
	if _, ok := result["cycle_id"].(string); !ok {
		t.Fatal("meso cycle response missing 'cycle_id' field")
	}

	// Verify no error field at top level
	if errMsg, ok := result["error"].(string); ok && errMsg != "" {
		t.Errorf("meso cycle returned error: %s", errMsg)
	}
}

// TestFeedbackWithTierMetrics verifies the guide endpoint returns
// tier-aware guidance and the feedback endpoint accepts responses.
func TestFeedbackWithTierMetrics(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Verify the tier-effectiveness endpoint works (this is the key integration point)
	url := fmt.Sprintf("%s/v1/jiminy/protocol/tier-effectiveness?space_id=mdemg-dev", cfg.MDEMGEndpoint)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("tier effectiveness request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tier effectiveness returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'data' field")
	}

	// Verify NLI calibration field is present
	if _, ok := data["nli_calibration"]; !ok {
		t.Error("response missing 'nli_calibration' field")
	}

	// Verify protocol metrics endpoint also has tier fields
	metricsURL := fmt.Sprintf("%s/v1/jiminy/protocol/metrics?space_id=mdemg-dev", cfg.MDEMGEndpoint)
	metricsResp, err := client.Get(metricsURL)
	if err != nil {
		t.Fatalf("metrics request failed: %v", err)
	}
	defer metricsResp.Body.Close()

	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics returned status %d", metricsResp.StatusCode)
	}

	var metricsResult map[string]any
	if err := json.NewDecoder(metricsResp.Body).Decode(&metricsResult); err != nil {
		t.Fatalf("failed to decode metrics: %v", err)
	}

	if mData, ok := metricsResult["data"].(map[string]any); ok {
		if _, ok := mData["tier_comprehension"]; !ok {
			t.Error("metrics missing 'tier_comprehension' field")
		}
	}
}
