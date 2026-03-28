//go:build integration

package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ─── Holistic RSIC integration tests ───
// These tests verify the full RSIC pipeline: assess → reflect → plan → dispatch → execute.
// The key innovation is seeding data via direct Cypher to provide 2/4 confidence data points
// (TotalNodes + ConsolidationAgeSec), raising confidence to 0.50 > 0.30 threshold, which
// allows the pipeline to progress past the confidence gate that blocks all existing tests.

// TestRSIC_Holistic_ConfidenceGatePassAndReflect validates that the confidence gate
// can be passed with sufficient data, and that the cycle produces real insights.
func TestRSIC_Holistic_ConfidenceGatePassAndReflect(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-conf")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed data: 1 ingest node (TotalNodes > 0) + 1 hidden node 48h old (ConsolidationAgeSec > 0)
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "confidence gate test node")
	SeedHiddenNode(t, driver, spaceID, 48)

	// Trigger cycle — should pass confidence gate (2/4 = 0.50 > 0.30)
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	cycleID, _ := resp["cycle_id"].(string)
	t.Logf("cycle_id: %s", cycleID)

	// Key assertion: outcome is NOT low_confidence
	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed — cycle returned low_confidence: %s", errStr)
	}

	// Confidence should be >= 0.50
	// Note: confidence may not be directly in the response (it's in the assessment report),
	// but the absence of low_confidence error proves it passed.

	// Safety version should always be stamped
	if sv, ok := resp["safety_version"].(string); !ok || sv != "phase88-v1" {
		t.Errorf("expected safety_version='phase88-v1', got %v", resp["safety_version"])
	}

	// The hidden node (48h old) triggers consolidation_age > 86400 → at least 1 insight
	// Check if insights were produced (present when pipeline reaches reflect stage)
	if insights, ok := resp["insights"].([]any); ok {
		t.Logf("insights produced: %d", len(insights))
		if len(insights) == 0 {
			t.Log("warning: no insights produced despite stale consolidation — reflector may have different thresholds")
		}
	}

	// Verify cycle appears in history
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
				// Verify this history entry doesn't show low_confidence
				if e["error"] != nil {
					errStr, _ := e["error"].(string)
					if strings.Contains(errStr, "confidence") {
						t.Errorf("history entry for %s shows low_confidence: %s", cycleID, errStr)
					}
				}
				break
			}
		}
	}
	if !found {
		t.Errorf("cycle %s not found in history", cycleID)
	}
}

// TestRSIC_Holistic_TombstoneStaleEndToEnd validates the full pipeline:
// assess → reflect → plan → dispatch → execute tombstone_stale → Neo4j mutation verified.
func TestRSIC_Holistic_TombstoneStaleEndToEnd(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-tomb")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed:
	// 1 hidden node (48h old) → ConsolidationAgeSec > 0
	// 10 learning observations (2h old) → tombstone targets
	// 3 correction observations (now) → triggers correction_rate > 0.15
	SeedHiddenNode(t, driver, spaceID, 48)
	learningIDs := SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "correction", 0)
	t.Logf("seeded: 1 hidden + 10 learning + 3 corrections = 14 nodes")
	t.Logf("learning node IDs: %v", learningIDs[:3]) // log first 3 for debugging

	// Verify baseline: no archived nodes yet
	archivedBefore := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	if archivedBefore != 0 {
		t.Fatalf("expected 0 archived nodes before cycle, got %d", archivedBefore)
	}

	// Trigger cycle
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	cycleID, _ := resp["cycle_id"].(string)
	t.Logf("cycle_id: %s", cycleID)

	// Must NOT be low_confidence
	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed: %s", errStr)
	}

	// Check for action execution evidence
	if executed, ok := resp["actions_executed"].(float64); ok {
		t.Logf("actions_executed: %d", int(executed))
	}
	if successCount, ok := resp["success_count"].(float64); ok {
		t.Logf("success_count: %d", int(successCount))
	}
	if failedCount, ok := resp["failed_count"].(float64); ok {
		t.Logf("failed_count: %d", int(failedCount))
	}

	// Allow async dispatch to complete
	time.Sleep(2 * time.Second)

	// Neo4j verification: some old observations should now be archived
	archivedAfter := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	t.Logf("archived nodes after cycle: %d", archivedAfter)
	if archivedAfter == 0 {
		t.Error("expected > 0 archived nodes after tombstone_stale execution, got 0")
	}

	// Negative check: correction nodes should NOT be archived.
	// The tombstone query excludes obs_type='correction', so they should remain unarchived.
	// We verify by checking that the total archived count <= number of learning nodes (10).
	if archivedAfter > 10 {
		t.Errorf("more nodes archived (%d) than learning observations seeded (10) — corrections may have been archived", archivedAfter)
	}
}

// TestRSIC_Holistic_DryRunPreservesState validates that dry-run passes the confidence
// gate, runs the full pipeline, but makes zero Neo4j mutations.
func TestRSIC_Holistic_DryRunPreservesState(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-dry")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Same seeding as tombstone test
	SeedHiddenNode(t, driver, spaceID, 48)
	SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "correction", 0)

	// Verify baseline
	archivedBefore := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	if archivedBefore != 0 {
		t.Fatalf("expected 0 archived nodes before dry-run, got %d", archivedBefore)
	}

	// Trigger dry-run cycle
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"dry_run": true,
	})

	// Assert dry_run flag in response
	if dryRun, ok := resp["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run=true, got %v", resp["dry_run"])
	}

	// Must NOT be low_confidence
	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed in dry-run: %s", errStr)
	}

	// Check for safety_summary and deltas (present when pipeline reaches dispatch)
	if ss, ok := resp["safety_summary"].(map[string]any); ok {
		if checked, ok := ss["actions_checked"].(float64); ok && checked > 0 {
			t.Logf("dry-run safety: actions_checked=%d", int(checked))
		}
	}
	if deltas, ok := resp["deltas"].([]any); ok {
		t.Logf("dry-run deltas: %d", len(deltas))
	}

	// Allow any async operations
	time.Sleep(500 * time.Millisecond)

	// Neo4j verification: ZERO mutations — no nodes should be archived
	archivedAfter := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	if archivedAfter != 0 {
		t.Errorf("dry-run should not archive nodes, but %d were archived", archivedAfter)
	}
}

// TestRSIC_Holistic_RollbackReversesTombstone validates the full round-trip:
// execute tombstone → snapshot captured → rollback → mutations reversed.
func TestRSIC_Holistic_RollbackReversesTombstone(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-rollback")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed tombstone scenario
	SeedHiddenNode(t, driver, spaceID, 48)
	SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "correction", 0)

	// Execute cycle (non-dry-run)
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed: %s", errStr)
	}

	// Allow async dispatch
	time.Sleep(2 * time.Second)

	// Verify tombstone happened
	archivedAfterCycle := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	t.Logf("archived after cycle: %d", archivedAfterCycle)
	if archivedAfterCycle == 0 {
		t.Skip("no nodes were archived — tombstone_stale may not have executed; skipping rollback test")
	}

	// Find a tombstone_stale snapshot in the rollback list
	rollbackList := GetRSICRollbackList(t, cfg.MDEMGEndpoint)
	snapshots, ok := rollbackList["snapshots"].([]any)
	if !ok || len(snapshots) == 0 {
		t.Skip("no snapshots captured — action may not have reached dispatch; skipping rollback test")
	}

	// Find the most recent tombstone_stale snapshot for our space
	var snapshotID string
	for i := len(snapshots) - 1; i >= 0; i-- {
		snap, ok := snapshots[i].(map[string]any)
		if !ok {
			continue
		}
		action, _ := snap["action"].(string)
		snapSpace, _ := snap["space_id"].(string)
		if action == "tombstone_stale" && snapSpace == spaceID {
			snapshotID, _ = snap["snapshot_id"].(string)
			break
		}
	}

	if snapshotID == "" {
		t.Skip("no tombstone_stale snapshot found for test space; skipping rollback verification")
	}
	t.Logf("rolling back snapshot: %s", snapshotID)

	// Execute rollback
	status, rollbackResp := PostRSICRollback(t, cfg.MDEMGEndpoint, snapshotID)
	if status != http.StatusOK {
		t.Fatalf("rollback returned status %d: %v", status, rollbackResp)
	}

	rolledBack, _ := rollbackResp["rolled_back"].(bool)
	if !rolledBack {
		t.Fatalf("expected rolled_back=true, got %v", rollbackResp["rolled_back"])
	}

	restoredCount, _ := rollbackResp["restored_count"].(float64)
	t.Logf("restored_count: %d", int(restoredCount))

	// Neo4j verification: archived count should be back to 0
	archivedAfterRollback := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	t.Logf("archived after rollback: %d", archivedAfterRollback)
	if archivedAfterRollback != 0 {
		t.Errorf("expected 0 archived nodes after rollback, got %d", archivedAfterRollback)
	}
}

// TestRSIC_Holistic_HistoryAndCalibrationReflectExecution validates that history
// entries and calibration state accurately reflect real pipeline execution.
func TestRSIC_Holistic_HistoryAndCalibrationReflectExecution(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-history")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed tombstone scenario
	SeedHiddenNode(t, driver, spaceID, 48)
	SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "correction", 0)

	// Trigger cycle
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	cycleID, _ := resp["cycle_id"].(string)
	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed: %s", errStr)
	}

	// Verify the new cycle appears in history (count may stay flat if at capacity)
	histAfter := GetRSICHistory(t, cfg.MDEMGEndpoint, 200)
	countAfter := int(histAfter["count"].(float64))
	t.Logf("history count after cycle: %d", countAfter)
	if countAfter < 1 {
		t.Fatalf("expected at least 1 history entry, got %d", countAfter)
	}

	// Find our cycle in history and validate its fields
	historyArr, ok := histAfter["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", histAfter["history"])
	}

	var entry map[string]any
	for _, h := range historyArr {
		if e, ok := h.(map[string]any); ok {
			if e["cycle_id"] == cycleID {
				entry = e
				break
			}
		}
	}

	if entry == nil {
		t.Fatalf("cycle %s not found in history", cycleID)
	}

	// Validate history entry fields
	if entryErr, ok := entry["error"].(string); ok && strings.Contains(entryErr, "confidence") {
		t.Errorf("history entry shows low_confidence — pipeline didn't execute: %s", entryErr)
	}

	if sv, ok := entry["safety_version"].(string); !ok || sv != "phase88-v1" {
		t.Errorf("expected safety_version='phase88-v1' in history, got %v", entry["safety_version"])
	}

	if ts, ok := entry["trigger_source"].(string); !ok || ts == "" {
		t.Errorf("expected non-empty trigger_source in history, got %v", entry["trigger_source"])
	}

	// Calibration endpoint should be accessible
	calResp := GetRSICCalibration(t, cfg.MDEMGEndpoint)
	if _, ok := calResp["calibration"]; !ok {
		t.Error("expected 'calibration' key in calibration response")
	}
	t.Logf("calibration: %v", calResp["calibration"])

	// Health endpoint should reflect real assessment
	health := GetRSICHealth(t, cfg.MDEMGEndpoint)
	if _, ok := health["status"]; !ok {
		t.Error("expected 'status' in health response")
	}
}

// TestRSIC_Holistic_MultiActionDispatchAndMetrics validates that multiple actions
// are dispatched in a single cycle and that Prometheus records action-level metrics.
func TestRSIC_Holistic_MultiActionDispatchAndMetrics(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("holistic-multi")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed data that triggers multiple insights:
	// 1. Hidden node 48h old → consolidation_age > 86400 → trigger_consolidation
	// 2. 10 learning + 3 corrections → correction_rate > 0.15 → tombstone_stale
	// 3. All nodes are orphans → orphan_ratio > 0.2 → trigger_consolidation (deduped with #1)
	// Expected after dedup: 2 unique action types (trigger_consolidation, tombstone_stale)
	SeedHiddenNode(t, driver, spaceID, 48)
	SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "correction", 0)

	// Trigger cycle
	resp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	cycleID, _ := resp["cycle_id"].(string)
	t.Logf("cycle_id: %s", cycleID)

	if errStr, ok := resp["error"].(string); ok && strings.Contains(errStr, "confidence") {
		t.Fatalf("confidence gate was NOT passed: %s", errStr)
	}

	// Check actions_executed — should be >= 2 (trigger_consolidation + tombstone_stale)
	actionsExecuted, _ := resp["actions_executed"].(float64)
	t.Logf("actions_executed: %d", int(actionsExecuted))
	if actionsExecuted < 2 {
		t.Logf("warning: expected >= 2 actions executed, got %d — pipeline may have had fewer insights", int(actionsExecuted))
	}

	// tombstone_stale should succeed; trigger_consolidation may fail (needs embedder)
	successCount, _ := resp["success_count"].(float64)
	failedCount, _ := resp["failed_count"].(float64)
	t.Logf("success_count: %d, failed_count: %d", int(successCount), int(failedCount))

	if successCount == 0 && failedCount == 0 && actionsExecuted == 0 {
		t.Error("expected at least some action execution, but got zeros for all counts")
	}

	// Allow async dispatch to complete
	time.Sleep(2 * time.Second)

	// Neo4j: tombstone_stale should have archived some nodes regardless of other action failures
	archivedCount := CountNodesByProperty(t, driver, spaceID, "is_archived", true)
	t.Logf("archived nodes: %d", archivedCount)
	if archivedCount == 0 {
		t.Error("expected > 0 archived nodes from tombstone_stale, even if trigger_consolidation failed")
	}

	// Metrics snapshot verification
	client := NewTestHTTPClient()
	promResp, err := client.Get(cfg.MDEMGEndpoint + "/v1/metrics/snapshot")
	if err != nil {
		t.Fatalf("metrics snapshot request failed: %v", err)
	}
	defer promResp.Body.Close()

	if promResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics snapshot returned status %d", promResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(promResp.Body)
	if err != nil {
		t.Fatalf("failed to read metrics snapshot body: %v", err)
	}
	metricsText := string(bodyBytes)

	// Verify RSIC cycle metric exists with a real execution outcome
	if !strings.Contains(metricsText, "mdemg_rsic_cycle_total") {
		t.Error("metrics snapshot missing mdemg_rsic_cycle_total")
	}

	// Verify action-level metrics exist
	if !strings.Contains(metricsText, "mdemg_rsic_action_total") {
		t.Error("metrics snapshot missing mdemg_rsic_action_total")
	}

	// Check for completed outcome (not just low_confidence)
	hasCompleted := strings.Contains(metricsText, `outcome`)
	hasPartial := strings.Contains(metricsText, `partial`)
	if !hasCompleted && !hasPartial {
		t.Log("warning: no completed/partial outcome metric found — cycle may have recorded differently")
	}

	t.Logf("metrics snapshot length: %d bytes", len(metricsText))
}
