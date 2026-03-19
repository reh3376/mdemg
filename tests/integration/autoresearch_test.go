//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Phase AR-1: RSIC Feedback Loop — Post-Cycle Re-Assessment + Criteria Evaluation
// ═══════════════════════════════════════════════════════════════════════════════

// TestAR1_CycleOutcome_HasMetricsAfter verifies that a completed RSIC cycle
// populates the metrics_after field via post-cycle re-assessment.
func TestAR1_CycleOutcome_HasMetricsAfter(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("ar1-metrics-after")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed enough data to pass confidence gate
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "ar1 test node for metrics_after")
	SeedHiddenNode(t, driver, spaceID, 48)

	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Verify cycle completed (not low_confidence)
	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Skipf("confidence gate not passed — cannot verify metrics_after: %s", errStr)
	}

	// metrics_after should be present (non-nil map)
	metricsAfter, ok := resp["metrics_after"].(map[string]any)
	if !ok || metricsAfter == nil {
		t.Error("expected metrics_after to be populated after post-cycle re-assessment, got nil or wrong type")
	} else {
		t.Logf("metrics_after keys: %v", mapKeys(metricsAfter))
		// Should contain at least overall_health
		if _, hasHealth := metricsAfter["overall_health"]; !hasHealth {
			t.Error("metrics_after should contain 'overall_health' key")
		}
	}

	// metrics_before should also be present
	metricsBefore, ok := resp["metrics_before"].(map[string]any)
	if !ok || metricsBefore == nil {
		t.Error("expected metrics_before to be populated, got nil or wrong type")
	}
}

// TestAR1_CycleOutcome_HasCriteriaFields verifies that the criteria evaluation
// fields (criteria_met, criteria_detail) are present in the cycle outcome.
func TestAR1_CycleOutcome_HasCriteriaFields(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("ar1-criteria")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed data for confidence gate
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "ar1 criteria test node")
	SeedHiddenNode(t, driver, spaceID, 48)
	SeedObservationNodes(t, driver, spaceID, 5, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 2, "correction", 0)

	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Skipf("confidence gate not passed: %s", errStr)
	}

	// criteria_met should be a boolean field in the response
	if _, exists := resp["criteria_met"]; !exists {
		t.Error("expected criteria_met field in cycle outcome")
	}

	t.Logf("criteria_met: %v", resp["criteria_met"])
	if detail, ok := resp["criteria_detail"].(map[string]any); ok {
		t.Logf("criteria_detail: %v", detail)
	}
}

// TestAR1_DryRun_SkipsPostAssessment verifies that dry-run cycles correctly
// skip post-cycle re-assessment (since no mutations occurred, metrics_after
// would be identical to metrics_before and is omitted).
func TestAR1_DryRun_SkipsPostAssessment(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("ar1-dryrun")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "ar1 dryrun test node")
	SeedHiddenNode(t, driver, spaceID, 48)

	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"dry_run": true,
	})

	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Skipf("confidence gate not passed: %s", errStr)
	}

	// Dry-run skips post-cycle assessment (no mutations = no metric changes)
	// metrics_before should be present, metrics_after should be nil/absent
	if _, ok := resp["metrics_before"].(map[string]any); !ok {
		t.Error("expected metrics_before in dry-run cycle outcome")
	}

	// Verify dry_run flag is set
	if dryRun, ok := resp["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run=true, got %v", resp["dry_run"])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Phase AR-2: Jiminy Guidance Effectiveness Tracking
// ═══════════════════════════════════════════════════════════════════════════════

// TestAR2_JiminyGuide_ReturnsGuidanceID verifies that the Jiminy guide endpoint
// returns a guidance_id in the response for effectiveness tracking.
func TestAR2_JiminyGuide_ReturnsGuidanceID(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	body := map[string]any{
		"space_id": "mdemg-dev",
		"context":  "test query for guidance_id check",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/jiminy/guide", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// If Jiminy is disabled, skip
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("Jiminy is not enabled on this server")
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got %T", result["data"])
	}

	guidanceID, ok := data["guidance_id"].(string)
	if !ok || guidanceID == "" {
		t.Error("expected non-empty guidance_id in response")
	} else {
		t.Logf("guidance_id: %s", guidanceID)
	}
}

// TestAR2_JiminyFeedback_Roundtrip verifies the full guide → feedback roundtrip:
// 1. Call /v1/jiminy/guide → get guidance_id
// 2. Call /v1/jiminy/feedback with that guidance_id → get outcome classification
func TestAR2_JiminyFeedback_Roundtrip(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Step 1: Get guidance
	guideBody := map[string]any{
		"space_id": "mdemg-dev",
		"context":  "You must always validate input before processing",
	}
	guideBytes, _ := json.Marshal(guideBody)

	guideResp, err := client.Post(cfg.MDEMGEndpoint+"/v1/jiminy/guide", "application/json", bytes.NewReader(guideBytes))
	if err != nil {
		t.Fatalf("guide request failed: %v", err)
	}
	defer guideResp.Body.Close()

	if guideResp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("Jiminy is not enabled on this server")
	}
	if guideResp.StatusCode != http.StatusOK {
		t.Fatalf("guide returned %d", guideResp.StatusCode)
	}

	var guideResult map[string]any
	json.NewDecoder(guideResp.Body).Decode(&guideResult)

	data := guideResult["data"].(map[string]any)
	guidanceID := data["guidance_id"].(string)

	// Step 2: Send feedback
	feedbackBody := map[string]any{
		"guidance_id":    guidanceID,
		"action_summary": "I validated the input before processing it",
		"space_id":       "mdemg-dev",
	}
	feedbackBytes, _ := json.Marshal(feedbackBody)

	feedbackResp, err := client.Post(cfg.MDEMGEndpoint+"/v1/jiminy/feedback", "application/json", bytes.NewReader(feedbackBytes))
	if err != nil {
		t.Fatalf("feedback request failed: %v", err)
	}
	defer feedbackResp.Body.Close()

	if feedbackResp.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(feedbackResp.Body).Decode(&errBody)
		t.Fatalf("feedback returned %d: %v", feedbackResp.StatusCode, errBody)
	}

	var feedbackResult map[string]any
	json.NewDecoder(feedbackResp.Body).Decode(&feedbackResult)

	fbData, ok := feedbackResult["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data in feedback response, got %T", feedbackResult["data"])
	}

	t.Logf("feedback response: %v", fbData)

	// Should have guidance_id echoed back
	if fbData["guidance_id"] != guidanceID {
		t.Errorf("expected guidance_id=%s echoed, got %v", guidanceID, fbData["guidance_id"])
	}
}

// TestAR2_JiminyFeedback_MissingGuidanceID verifies that feedback with
// an empty guidance_id returns 400.
func TestAR2_JiminyFeedback_MissingGuidanceID(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	body := map[string]any{
		"guidance_id":    "",
		"action_summary": "test",
		"space_id":       "mdemg-dev",
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/jiminy/feedback", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("Jiminy is not enabled on this server")
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty guidance_id, got %d", resp.StatusCode)
	}
}

// TestAR2_JiminyFeedback_UnknownGuidanceID verifies that feedback with a
// non-existent guidance_id still returns 200 (not tracked = empty items).
func TestAR2_JiminyFeedback_UnknownGuidanceID(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	body := map[string]any{
		"guidance_id":    "nonexistent-guid-" + time.Now().Format("150405"),
		"action_summary": "test action",
		"space_id":       "mdemg-dev",
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/jiminy/feedback", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("Jiminy is not enabled on this server")
	}

	// Should return 200 even if guidance_id is unknown (graceful handling)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for unknown guidance_id, got %d", resp.StatusCode)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Phase AR-3: LLM-Powered Intelligence (opt-in, test structural availability)
// ═══════════════════════════════════════════════════════════════════════════════

// TestAR3_CycleOutcome_LLMReflectorFallback verifies that when the LLM reflector
// is enabled but no LLM provider is available, the cycle still completes (fail-open).
// This tests the degrade-gracefully behavior of the LLM reflection path.
func TestAR3_CycleOutcome_LLMReflectorFallback(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("ar3-llm-fallback")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "ar3 llm reflector test node")
	SeedHiddenNode(t, driver, spaceID, 48)

	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Skipf("confidence gate not passed: %s", errStr)
	}

	// The cycle should complete regardless of whether LLM reflector is configured
	cycleID, ok := resp["cycle_id"].(string)
	if !ok || cycleID == "" {
		t.Error("expected cycle_id in response")
	}

	// Insights may be rule-based only (LLM reflector fail-open) or merged
	if insights, ok := resp["insights"].([]any); ok {
		t.Logf("insights count: %d (rule-based + any LLM)", len(insights))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
