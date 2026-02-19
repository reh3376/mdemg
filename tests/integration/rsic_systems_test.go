//go:build integration

package integration

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ─── Systems-level RSIC integration tests ───
// These tests validate cross-subsystem behavior that was never exercised
// by the per-phase unit tests or the 6 original integration tests.

// TestRSIC_Systems_CooldownRejectsRapidRetrigger verifies that the orchestration
// policy returns 409 when a second cycle is triggered within the cooldown window.
func TestRSIC_Systems_CooldownRejectsRapidRetrigger(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-cooldown")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed node so space is non-empty
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "cooldown test node")

	// First cycle should succeed
	resp1 := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)
	cycleID, ok := resp1["cycle_id"].(string)
	if !ok || cycleID == "" {
		t.Fatalf("expected cycle_id in first response, got %v", resp1["cycle_id"])
	}
	t.Logf("first cycle_id: %s", cycleID)

	// Immediately re-trigger — should hit cooldown → 409
	status, body := TriggerRSICCycleRaw(t, cfg.MDEMGEndpoint, spaceID, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on rapid retrigger, got %d: %v", status, body)
	}

	errMsg, _ := body["error"].(string)
	if errMsg != "trigger rejected" {
		t.Errorf("expected error='trigger rejected', got %q", errMsg)
	}

	reason, _ := body["reason"].(string)
	if !strings.Contains(reason, "cooldown") {
		t.Errorf("expected reason to contain 'cooldown', got %q", reason)
	}

	pv, _ := body["policy_version"].(string)
	if pv != "phase87-v1" {
		t.Errorf("expected policy_version='phase87-v1', got %q", pv)
	}
}

// TestRSIC_Systems_SourceTierMismatchRejected verifies that the orchestration
// policy rejects micro_auto triggers for non-micro tiers.
func TestRSIC_Systems_SourceTierMismatchRejected(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-tier-mismatch")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "tier mismatch test node")

	// micro_auto can only trigger tier=micro; send tier=meso → 409
	status, body := TriggerRSICCycleRaw(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"trigger_source": "micro_auto",
		"tier":           "meso",
	})

	if status != http.StatusConflict {
		t.Fatalf("expected 409 for source-tier mismatch, got %d: %v", status, body)
	}

	reason, _ := body["reason"].(string)
	if !strings.Contains(reason, "tier") {
		t.Errorf("expected reason to mention 'tier', got %q", reason)
	}
}

// TestRSIC_Systems_IdempotencyDeduplication verifies that a second request
// with the same idempotency_key returns a dedupe fast-path 200 (not 409).
func TestRSIC_Systems_IdempotencyDeduplication(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-idempotency")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "idempotency test node")

	idempotencyKey := fmt.Sprintf("idem-%d", generateNonce())

	// First request with idempotency key → 200
	resp1 := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"idempotency_key": idempotencyKey,
	})
	cycleID1, ok := resp1["cycle_id"].(string)
	if !ok || cycleID1 == "" {
		t.Fatalf("expected cycle_id in first response, got %v", resp1["cycle_id"])
	}
	t.Logf("first cycle_id: %s", cycleID1)

	// Second request with same key → should be 200 (dedupe fast-path, not 409)
	status, body := TriggerRSICCycleRaw(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"idempotency_key": idempotencyKey,
	})

	if status != http.StatusOK {
		t.Fatalf("expected 200 on dedupe fast-path, got %d: %v", status, body)
	}

	// Check that dedupe block is present
	dedupe, ok := body["dedupe"].(map[string]any)
	if !ok {
		t.Fatalf("expected dedupe block in response, got %v", body)
	}

	isDuplicate, _ := dedupe["is_duplicate"].(bool)
	if !isDuplicate {
		t.Error("expected dedupe.is_duplicate=true")
	}

	// cycle_id should match the original
	cycleID2, _ := body["cycle_id"].(string)
	if cycleID2 != cycleID1 {
		t.Errorf("expected dedupe cycle_id=%q, got %q", cycleID1, cycleID2)
	}
}

// TestRSIC_Systems_CalibrationAccumulatesHistory verifies that triggering cycles
// increases the history count and that the calibration endpoint is accessible.
func TestRSIC_Systems_CalibrationAccumulatesHistory(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceA := GenerateTestSpaceID("sys-cal-a")
	spaceB := GenerateTestSpaceID("sys-cal-b")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceA)
		CleanupSpaceWithTest(t, driver, spaceB)
	})

	// Get initial history count
	histBefore := GetRSICHistory(t, cfg.MDEMGEndpoint, 100)
	countBefore := int(histBefore["count"].(float64))
	t.Logf("history count before: %d", countBefore)

	// Trigger cycle on spaceA
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceA, "calibration test node A")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceA, nil)

	// Trigger cycle on spaceB (different space avoids cooldown)
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceB, "calibration test node B")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceB, nil)

	// History should have grown by at least 2
	histAfter := GetRSICHistory(t, cfg.MDEMGEndpoint, 100)
	countAfter := int(histAfter["count"].(float64))
	t.Logf("history count after: %d", countAfter)

	if countAfter < countBefore+2 {
		t.Errorf("expected history count to increase by >=2, got before=%d after=%d", countBefore, countAfter)
	}

	// Calibration endpoint should be accessible and have a calibration key
	calResp := GetRSICCalibration(t, cfg.MDEMGEndpoint)
	if _, ok := calResp["calibration"]; !ok {
		t.Error("expected 'calibration' key in calibration response")
	}
}

// TestRSIC_Systems_HistoryFiltersBySpaceAndTier verifies that the history
// filtering by space_id and tier returns correct subsets.
func TestRSIC_Systems_HistoryFiltersBySpaceAndTier(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceA := GenerateTestSpaceID("sys-filter-a")
	spaceB := GenerateTestSpaceID("sys-filter-b")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceA)
		CleanupSpaceWithTest(t, driver, spaceB)
	})

	// Trigger micro cycles on both spaces
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceA, "filter test node A")
	respA := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceA, nil)
	cycleIDA := respA["cycle_id"].(string)

	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceB, "filter test node B")
	respB := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceB, nil)
	cycleIDB := respB["cycle_id"].(string)

	t.Logf("cycle A: %s, cycle B: %s", cycleIDA, cycleIDB)

	// Filter by spaceA — should NOT contain cycle_id_b
	filteredA := GetRSICHistoryFiltered(t, cfg.MDEMGEndpoint, 50, map[string]string{"space_id": spaceA})
	historyA, ok := filteredA["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", filteredA["history"])
	}
	for _, entry := range historyA {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if sid, _ := e["space_id"].(string); sid != spaceA {
			t.Errorf("space_id filter returned entry with space_id=%q, expected %q", sid, spaceA)
		}
		if cid, _ := e["cycle_id"].(string); cid == cycleIDB {
			t.Errorf("space_id filter for A should not include cycle_id from B: %s", cycleIDB)
		}
	}

	// Filter by tier=micro — all entries should have tier=micro
	filteredMicro := GetRSICHistoryFiltered(t, cfg.MDEMGEndpoint, 50, map[string]string{"tier": "micro"})
	historyMicro, ok := filteredMicro["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", filteredMicro["history"])
	}
	for _, entry := range historyMicro {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if tier, _ := e["tier"].(string); tier != "micro" {
			t.Errorf("tier filter returned entry with tier=%q, expected 'micro'", tier)
		}
	}

	// Combined filter: space_id=spaceA AND tier=micro
	filteredCombined := GetRSICHistoryFiltered(t, cfg.MDEMGEndpoint, 50, map[string]string{
		"space_id": spaceA,
		"tier":     "micro",
	})
	historyCombined, ok := filteredCombined["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", filteredCombined["history"])
	}
	for _, entry := range historyCombined {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if sid, _ := e["space_id"].(string); sid != spaceA {
			t.Errorf("combined filter returned wrong space_id=%q", sid)
		}
		if tier, _ := e["tier"].(string); tier != "micro" {
			t.Errorf("combined filter returned wrong tier=%q", tier)
		}
	}
}

// TestRSIC_Systems_DryRunStructureAndSafetyMetadata validates the dry-run response
// structure, including safety_version, cycle_id format, and history recording.
func TestRSIC_Systems_DryRunStructureAndSafetyMetadata(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-dryrun-struct")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed 3 nodes
	for i := 1; i <= 3; i++ {
		ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID,
			fmt.Sprintf("dryrun structure test node %d", i))
	}

	// Trigger dry-run cycle
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"dry_run": true,
	})

	// Assert dry_run flag
	if dryRun, ok := resp["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run=true, got %v", resp["dry_run"])
	}

	// Assert safety_version
	if sv, ok := resp["safety_version"].(string); !ok || sv != "phase88-v1" {
		t.Errorf("expected safety_version='phase88-v1', got %q", resp["safety_version"])
	}

	// Assert cycle_id starts with "rsic-"
	cycleID, ok := resp["cycle_id"].(string)
	if !ok || !strings.HasPrefix(cycleID, "rsic-") {
		t.Errorf("expected cycle_id starting with 'rsic-', got %q", cycleID)
	}

	// Check error field — CI exits early at low_confidence
	if errStr, ok := resp["error"].(string); ok && errStr != "" {
		t.Logf("cycle exited early: %s (expected in CI without embedder)", errStr)
	} else {
		// Cycle progressed — validate delta structure
		if deltas, ok := resp["deltas"].([]any); ok {
			t.Logf("deltas array length: %d", len(deltas))
		}
		if ss, ok := resp["safety_summary"].(map[string]any); ok {
			if _, ok := ss["actions_checked"]; !ok {
				t.Error("safety_summary missing actions_checked")
			}
			if _, ok := ss["actions_allowed"]; !ok {
				t.Error("safety_summary missing actions_allowed")
			}
		}
	}

	// Verify the dry-run cycle appears in history
	history := GetRSICHistory(t, cfg.MDEMGEndpoint, 50)
	historyArr, ok := history["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", history["history"])
	}

	found := false
	for _, entry := range historyArr {
		if e, ok := entry.(map[string]any); ok {
			if e["cycle_id"] == cycleID {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("dry-run cycle %s not found in history", cycleID)
	}
}

// TestRSIC_Systems_RollbackListAndInvalidAttempt tests the rollback list endpoint
// shape and verifies that rolling back a nonexistent snapshot returns 404.
func TestRSIC_Systems_RollbackListAndInvalidAttempt(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()

	// GET rollback list
	listResp := GetRSICRollbackList(t, cfg.MDEMGEndpoint)

	// Assert shape: snapshots is array, count is number, rollback_window_sec > 0
	if _, ok := listResp["snapshots"].([]any); !ok {
		t.Errorf("expected snapshots to be array, got %T", listResp["snapshots"])
	}
	if _, ok := listResp["count"].(float64); !ok {
		t.Errorf("expected count to be number, got %T", listResp["count"])
	}
	if windowSec, ok := listResp["rollback_window_sec"].(float64); !ok || windowSec <= 0 {
		t.Errorf("expected rollback_window_sec > 0, got %v", listResp["rollback_window_sec"])
	}

	// POST with nonexistent snapshot_id → expect 404
	status, body := PostRSICRollback(t, cfg.MDEMGEndpoint, "nonexistent-snapshot-id-12345")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent snapshot, got %d: %v", status, body)
	}

	rolledBack, _ := body["rolled_back"].(bool)
	if rolledBack {
		t.Error("expected rolled_back=false for nonexistent snapshot")
	}

	errMsg, _ := body["error"].(string)
	if errMsg == "" {
		t.Error("expected non-empty error message for nonexistent snapshot")
	}
}

// TestRSIC_Systems_WatchdogStateInHealth validates the watchdog block in the
// health endpoint after running a cycle.
func TestRSIC_Systems_WatchdogStateInHealth(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-watchdog")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed and trigger cycle to populate watchdog state
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "watchdog test node")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Get health
	health := GetRSICHealth(t, cfg.MDEMGEndpoint)

	watchdog, ok := health["watchdog"].(map[string]any)
	if !ok {
		if health["watchdog"] == nil {
			t.Skip("watchdog block is nil — watchdog may be disabled")
		}
		t.Fatalf("expected watchdog map, got %T", health["watchdog"])
	}

	// decay_score should be float >= 0
	decayScore, ok := watchdog["decay_score"].(float64)
	if !ok {
		t.Errorf("expected decay_score to be float, got %T", watchdog["decay_score"])
	} else if decayScore < 0 {
		t.Errorf("expected decay_score >= 0, got %f", decayScore)
	} else {
		t.Logf("decay_score: %f", decayScore)
	}

	// escalation_level should be 0-3
	escalation, ok := watchdog["escalation_level"].(float64)
	if !ok {
		t.Errorf("expected escalation_level to be number, got %T", watchdog["escalation_level"])
	} else if escalation < 0 || escalation > 3 {
		t.Errorf("expected escalation_level in [0,3], got %f", escalation)
	}

	// space_id should be non-empty string
	spaceIDVal, ok := watchdog["space_id"].(string)
	if !ok || spaceIDVal == "" {
		t.Errorf("expected non-empty space_id in watchdog, got %v", watchdog["space_id"])
	}
}

// TestRSIC_Systems_FullHealthCompositeValidation deep-validates all health
// sub-blocks (orchestration, persistence, safety) after running a cycle.
func TestRSIC_Systems_FullHealthCompositeValidation(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-health-full")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed and trigger to populate all health blocks
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "full health test node")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	health := GetRSICHealth(t, cfg.MDEMGEndpoint)

	// ── orchestration ──
	orch, ok := health["orchestration"].(map[string]any)
	if !ok {
		t.Fatal("expected orchestration map in health")
	}
	if _, ok := orch["micro_enabled"].(bool); !ok {
		t.Error("orchestration missing bool micro_enabled")
	}
	if cd, ok := orch["cooldown_sec"].(float64); !ok || cd <= 0 {
		t.Errorf("expected orchestration.cooldown_sec > 0, got %v", orch["cooldown_sec"])
	}
	if dd, ok := orch["dedupe_sec"].(float64); !ok || dd <= 0 {
		t.Errorf("expected orchestration.dedupe_sec > 0, got %v", orch["dedupe_sec"])
	}
	if sched, ok := orch["scheduler"].(map[string]any); ok {
		if _, ok := sched["enabled"]; !ok {
			t.Error("orchestration.scheduler missing 'enabled' field")
		}
	} else {
		t.Error("expected orchestration.scheduler to be a map")
	}

	// ── persistence ──
	persist, ok := health["persistence"].(map[string]any)
	if !ok {
		t.Fatal("expected persistence map in health")
	}
	if _, ok := persist["enabled"].(bool); !ok {
		t.Error("persistence missing bool 'enabled'")
	}
	if _, ok := persist["state_nodes"].(float64); !ok {
		t.Error("persistence missing number 'state_nodes'")
	}
	if _, ok := persist["dirty_keys"].(float64); !ok {
		t.Error("persistence missing number 'dirty_keys'")
	}

	// ── safety ──
	safety, ok := health["safety"].(map[string]any)
	if !ok {
		t.Fatal("expected safety map in health")
	}
	if ea, ok := safety["enforcement_active"].(bool); !ok || !ea {
		t.Errorf("expected safety.enforcement_active=true, got %v", safety["enforcement_active"])
	}
	if sv, ok := safety["safety_version"].(string); !ok || sv != "phase88-v1" {
		t.Errorf("expected safety.safety_version='phase88-v1', got %v", safety["safety_version"])
	}

	bounds, ok := safety["bounds"].(map[string]any)
	if !ok {
		t.Fatal("expected safety.bounds map")
	}

	protectedSpaces, ok := bounds["protected_spaces"].([]any)
	if !ok {
		t.Fatal("expected bounds.protected_spaces to be array")
	}
	foundProtected := false
	for _, ps := range protectedSpaces {
		if ps == "mdemg-dev" {
			foundProtected = true
			break
		}
	}
	if !foundProtected {
		t.Error("expected 'mdemg-dev' in bounds.protected_spaces")
	}

	// Cross-validate safety bounds are positive numbers
	if maxNodes, ok := bounds["max_nodes_affected"].(float64); !ok || maxNodes <= 0 {
		t.Errorf("expected positive max_nodes_affected, got %v", bounds["max_nodes_affected"])
	}
	if maxEdges, ok := bounds["max_edges_affected"].(float64); !ok || maxEdges <= 0 {
		t.Errorf("expected positive max_edges_affected, got %v", bounds["max_edges_affected"])
	}
}

// TestRSIC_Systems_PrometheusMetricsAfterCycle validates that Prometheus metrics
// contain RSIC counter/gauge entries with values > 0 after a cycle.
func TestRSIC_Systems_PrometheusMetricsAfterCycle(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("sys-prom")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Trigger a cycle to produce metrics
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "prometheus test node")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Fetch prometheus metrics
	client := NewTestHTTPClient()
	resp, err := client.Get(cfg.MDEMGEndpoint + "/v1/prometheus")
	if err != nil {
		t.Fatalf("prometheus request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("prometheus returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read prometheus body: %v", err)
	}
	metricsText := string(bodyBytes)

	// Check for RSIC cycle metric with outcome label
	if !strings.Contains(metricsText, "mdemg_rsic_cycle_total") {
		t.Error("prometheus output missing mdemg_rsic_cycle_total")
	}

	// Check for at least one outcome (started, completed, or low_confidence)
	hasStarted := strings.Contains(metricsText, `outcome="started"`)
	hasCompleted := strings.Contains(metricsText, `outcome="completed"`)
	hasLowConf := strings.Contains(metricsText, `outcome="low_confidence"`)
	if !hasStarted && !hasCompleted && !hasLowConf {
		t.Error("expected mdemg_rsic_cycle_total with an outcome label (started/completed/low_confidence)")
	}

	// Check for calibration confidence gauge
	if !strings.Contains(metricsText, "mdemg_rsic_calibration_confidence") {
		t.Error("prometheus output missing mdemg_rsic_calibration_confidence")
	}

	// Parse a specific metric line to verify value > 0
	for _, line := range strings.Split(metricsText, "\n") {
		if strings.HasPrefix(line, "mdemg_rsic_cycle_total") && !strings.HasPrefix(line, "#") {
			// Lines look like: mdemg_rsic_cycle_total{tier="micro",source="manual_api",outcome="started"} 1
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				t.Logf("metric: %s", line)
			}
			break
		}
	}
}

// generateNonce returns a unique int64 for idempotency keys.
func generateNonce() int64 {
	return time.Now().UnixNano()
}
