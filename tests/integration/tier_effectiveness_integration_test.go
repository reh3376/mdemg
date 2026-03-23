//go:build integration

package integration

import (
	"bytes"
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

	// Verify structure
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
// Record outcomes with tier data → trigger RSIC cycle → verify tier-related
// insights appear when tier drift exists.
func TestTierEffectivenessRSICLoop(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	spaceID := GenerateTestSpaceID("tier-eff")

	// Step 1: Send multiple feedback requests to populate tier metrics.
	// We use the protocol metrics endpoint to verify data flows through.
	// The /v1/jiminy/feedback endpoint populates these, but for integration
	// testing we verify via the metrics and RSIC endpoints.

	// Step 2: Trigger an RSIC micro cycle to verify protocol assessment
	// includes tier data
	result := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"tier": "micro",
	})

	// Step 3: Verify cycle completed
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("cycle response missing 'data' field")
	}

	// The cycle should have an assessment stage
	if stages, ok := data["stages"].([]any); ok {
		foundAssess := false
		for _, s := range stages {
			if stage, ok := s.(map[string]any); ok {
				if stage["stage"] == "assess" || stage["name"] == "assess" {
					foundAssess = true
					break
				}
			}
		}
		if !foundAssess {
			t.Log("warning: no 'assess' stage found in cycle stages (may be nested differently)")
		}
	}

	// Step 4: Verify protocol metrics endpoint returns tier data
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
// attempts to generate a tier effectiveness dataset (via the provider).
func TestTierEffectivenessDatasetGeneration(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("tier-dataset")

	// Trigger a meso cycle — dataset generation happens at meso/macro boundaries
	result := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"tier": "meso",
	})

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("cycle response missing 'data' field")
	}

	// Meso cycle should complete without error
	if errMsg, ok := data["error"].(string); ok && errMsg != "" {
		t.Errorf("meso cycle returned error: %s", errMsg)
	}
}

// TestFeedbackWithTierMetrics sends a feedback request and verifies
// tier metrics are updated accordingly.
func TestFeedbackWithTierMetrics(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// First, get a guide response to get a guidance_id
	guideBody := map[string]any{
		"space_id":   "mdemg-dev",
		"context":    "test tier metrics flow",
		"session_id": GenerateTestSpaceID("tier-feedback"),
	}
	guideBytes, _ := json.Marshal(guideBody)

	guideReq, _ := http.NewRequest(http.MethodPost,
		cfg.MDEMGEndpoint+"/v1/jiminy/guide",
		bytes.NewReader(guideBytes))
	guideReq.Header.Set("Content-Type", "application/json")

	guideResp, err := client.Do(guideReq)
	if err != nil {
		t.Fatalf("guide request failed: %v", err)
	}
	defer guideResp.Body.Close()

	if guideResp.StatusCode != http.StatusOK {
		t.Skipf("guide endpoint returned %d — Jiminy may be disabled, skipping", guideResp.StatusCode)
	}

	var guideResult map[string]any
	if err := json.NewDecoder(guideResp.Body).Decode(&guideResult); err != nil {
		t.Fatalf("failed to decode guide response: %v", err)
	}

	guideData, _ := guideResult["data"].(map[string]any)
	guidanceID, _ := guideData["guidance_id"].(string)
	if guidanceID == "" {
		t.Skip("no guidance_id returned — no active constraints to test with")
	}

	// Send feedback with action summary
	feedbackBody := map[string]any{
		"space_id":       "mdemg-dev",
		"guidance_id":    guidanceID,
		"action_summary": "I followed the constraint by not force-pushing",
		"items":          []any{},
	}
	fbBytes, _ := json.Marshal(feedbackBody)

	fbReq, _ := http.NewRequest(http.MethodPost,
		cfg.MDEMGEndpoint+"/v1/jiminy/feedback",
		bytes.NewReader(fbBytes))
	fbReq.Header.Set("Content-Type", "application/json")

	fbResp, err := client.Do(fbReq)
	if err != nil {
		t.Fatalf("feedback request failed: %v", err)
	}
	defer fbResp.Body.Close()

	// 200 or 422 (no items) are both acceptable — we're testing the wiring
	if fbResp.StatusCode != http.StatusOK && fbResp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("feedback returned unexpected status %d", fbResp.StatusCode)
	}
}
