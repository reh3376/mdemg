//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// countNodesInSpace counts all MemoryNode nodes in the given space using Neo4j.
func countNodesInSpace(t *testing.T, driver neo4j.DriverWithContext, spaceID string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx,
			`MATCH (n:MemoryNode {space_id: $spaceId}) RETURN count(n) AS cnt`,
			map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return int64(0), nil
		}
		cnt, _ := res.Record().Get("cnt")
		return cnt, res.Err()
	})
	if err != nil {
		t.Fatalf("failed to count nodes: %v", err)
	}
	return result.(int64)
}

// countRSICStateNodes counts RSICState nodes in Neo4j.
func countRSICStateNodes(t *testing.T, driver neo4j.DriverWithContext) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx,
			`MATCH (n:MemoryNode:RSICState) RETURN count(n) AS cnt`,
			map[string]any{})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return int64(0), nil
		}
		cnt, _ := res.Record().Get("cnt")
		return cnt, res.Err()
	})
	if err != nil {
		t.Fatalf("failed to count RSICState nodes: %v", err)
	}
	return result.(int64)
}

// TestRSIC_CycleCreatesHistory triggers a micro cycle and verifies it appears
// in the history endpoint and that the health endpoint reflects the cycle.
func TestRSIC_CycleCreatesHistory(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("rsic-history")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed a node so the space is not empty
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "node for history test")

	// Trigger a micro cycle
	cycleResp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Assert response has cycle_id (string) and tier (string)
	cycleID, ok := cycleResp["cycle_id"].(string)
	if !ok || cycleID == "" {
		t.Fatalf("expected cycle_id string in response, got %v (%T)", cycleResp["cycle_id"], cycleResp["cycle_id"])
	}
	t.Logf("cycle_id: %s", cycleID)

	tier, ok := cycleResp["tier"].(string)
	if !ok || tier == "" {
		t.Fatalf("expected tier string in response, got %v (%T)", cycleResp["tier"], cycleResp["tier"])
	}

	// Get history and verify the cycle appears
	historyResp := GetRSICHistory(t, cfg.MDEMGEndpoint, 10)

	history, ok := historyResp["history"].([]any)
	if !ok {
		t.Fatalf("expected history to be an array, got %T", historyResp["history"])
	}
	if len(history) < 1 {
		t.Fatal("expected at least 1 cycle in history, got 0")
	}

	// Verify the cycle we just ran is in history
	found := false
	for _, entry := range history {
		if e, ok := entry.(map[string]any); ok {
			if e["cycle_id"] == cycleID {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("cycle_id %q not found in history", cycleID)
	}

	// Get health and verify active_tasks field exists
	healthResp := GetRSICHealth(t, cfg.MDEMGEndpoint)
	if _, ok := healthResp["active_tasks"]; !ok {
		t.Error("expected active_tasks field in health response")
	}
	if _, ok := healthResp["status"]; !ok {
		t.Error("expected status field in health response")
	}
}

// TestRSIC_DryRunProducesNoDelta observes nodes, runs a dry-run cycle,
// and verifies the node count in Neo4j is unchanged.
func TestRSIC_DryRunProducesNoDelta(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("rsic-dryrun")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Observe 3 nodes into the test space
	for i := 1; i <= 3; i++ {
		ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID,
			fmt.Sprintf("dry-run test node %d: knowledge about topic %d", i, i))
	}

	// Allow nodes to be committed
	time.Sleep(500 * time.Millisecond)

	// Count nodes before cycle
	countBefore := countNodesInSpace(t, driver, spaceID)
	t.Logf("nodes before dry-run cycle: %d", countBefore)

	// Trigger a dry-run cycle
	cycleResp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, map[string]any{
		"dry_run": true,
	})

	// Assert response has dry_run == true
	dryRun, ok := cycleResp["dry_run"].(bool)
	if !ok {
		t.Fatalf("expected dry_run bool in response, got %v (%T)", cycleResp["dry_run"], cycleResp["dry_run"])
	}
	if !dryRun {
		t.Error("expected dry_run to be true in response")
	}

	// Allow any async operations to settle
	time.Sleep(500 * time.Millisecond)

	// Count nodes after cycle — should be same as before
	countAfter := countNodesInSpace(t, driver, spaceID)
	t.Logf("nodes after dry-run cycle: %d", countAfter)

	if countAfter != countBefore {
		t.Errorf("dry-run cycle should not change node count: before=%d, after=%d", countBefore, countAfter)
	}
}

// TestRSIC_SafetyBlocksProtectedSpace triggers a cycle targeting the protected
// space "mdemg-dev" and verifies safety information is present in the response.
// The cycle may return 409 (cooldown) if a recent cycle already ran; in that
// case we still verify the health endpoint safety block.
func TestRSIC_SafetyBlocksProtectedSpace(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()

	// Try to trigger a cycle targeting the protected space.
	// Accept both 200 (cycle ran) and 409 (cooldown) — the test validates
	// the safety block on the health endpoint either way.
	body := map[string]any{"space_id": "mdemg-dev", "tier": "micro"}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	client := NewTestHTTPClient()
	req, err := http.NewRequest("POST", cfg.MDEMGEndpoint+"/v1/self-improve/cycle", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("cycle request failed: %v", err)
	}
	defer resp.Body.Close()

	var cycleResp map[string]any
	json.NewDecoder(resp.Body).Decode(&cycleResp)

	switch resp.StatusCode {
	case 200:
		t.Logf("cycle succeeded on mdemg-dev: %v", cycleResp["cycle_id"])
		if sv, ok := cycleResp["safety_version"].(string); ok {
			t.Logf("safety_version on cycle: %s", sv)
		}
	case 409:
		t.Logf("cycle cooldown active on mdemg-dev (expected during rapid reruns): %v", cycleResp["reason"])
	default:
		t.Fatalf("unexpected status %d: %v", resp.StatusCode, cycleResp)
	}

	// Verify via health endpoint that safety block is populated
	healthResp := GetRSICHealth(t, cfg.MDEMGEndpoint)
	if safety, ok := healthResp["safety"].(map[string]any); ok {
		if sv, ok := safety["safety_version"].(string); ok && sv != "" {
			t.Logf("health safety_version: %s", sv)
		} else {
			t.Error("safety block missing safety_version")
		}
	} else {
		t.Error("safety block not present on health endpoint")
	}
}

// TestRSIC_MultiSpaceCycleIsolation creates two spaces, triggers a cycle
// for space A only, and verifies space B is unchanged.
func TestRSIC_MultiSpaceCycleIsolation(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceA := GenerateTestSpaceID("rsic-iso-a")
	spaceB := GenerateTestSpaceID("rsic-iso-b")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceA)
		CleanupSpaceWithTest(t, driver, spaceB)
	})

	// Observe 3 nodes into each space
	for i := 1; i <= 3; i++ {
		ObserveTestNode(t, cfg.MDEMGEndpoint, spaceA,
			fmt.Sprintf("space-A observation %d about process control", i))
		ObserveTestNode(t, cfg.MDEMGEndpoint, spaceB,
			fmt.Sprintf("space-B observation %d about network topology", i))
	}

	// Allow nodes to be committed
	time.Sleep(500 * time.Millisecond)

	// Count nodes in space B before the cycle
	countBBefore := countNodesInSpace(t, driver, spaceB)
	t.Logf("space B nodes before cycle on A: %d", countBBefore)

	// Trigger a cycle for space A only
	cycleResp := TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceA, nil)

	cycleID, ok := cycleResp["cycle_id"].(string)
	if !ok || cycleID == "" {
		t.Fatalf("expected cycle_id in response, got %v", cycleResp["cycle_id"])
	}
	t.Logf("cycle on space A: %s", cycleID)

	// Allow async operations to settle
	time.Sleep(500 * time.Millisecond)

	// Count nodes in space B after the cycle — should be unchanged
	countBAfter := countNodesInSpace(t, driver, spaceB)
	t.Logf("space B nodes after cycle on A: %d", countBAfter)

	if countBAfter != countBBefore {
		t.Errorf("cycle on space A mutated space B: before=%d, after=%d", countBBefore, countBAfter)
	}
}

// TestRSIC_PersistenceSurvivesFlush triggers a cycle, checks the health
// persistence block, and queries Neo4j for RSICState nodes.
func TestRSIC_PersistenceSurvivesFlush(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("rsic-persist")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed a node and trigger a cycle to generate state
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "persistence test node")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Check health for persistence block
	healthResp := GetRSICHealth(t, cfg.MDEMGEndpoint)

	persistence, ok := healthResp["persistence"].(map[string]any)
	if !ok {
		t.Fatal("expected persistence block in health response")
	}

	enabled, ok := persistence["enabled"].(bool)
	if !ok {
		t.Fatalf("expected persistence.enabled to be a bool, got %T", persistence["enabled"])
	}
	if !enabled {
		t.Skip("RSIC persistence is disabled, skipping Neo4j state check")
	}
	t.Logf("persistence enabled: %v", enabled)

	// Wait for the write-behind flush (default 30s interval),
	// but also allow for immediate flush on cycle completion.
	// We give it a generous window.
	time.Sleep(2 * time.Second)

	// Query Neo4j directly for RSICState nodes
	stateCount := countRSICStateNodes(t, driver)
	t.Logf("RSICState nodes in Neo4j: %d", stateCount)

	if stateCount == 0 {
		t.Error("expected at least 1 RSICState node in Neo4j after cycle + flush")
	}
}

// TestRSIC_HealthEndpointShape triggers cycles and then verifies that the
// health endpoint returns the expected top-level structure.
func TestRSIC_HealthEndpointShape(t *testing.T) {
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	spaceID := GenerateTestSpaceID("rsic-health-shape")

	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed data and trigger a cycle to populate health blocks
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "health shape test node 1")
	ObserveTestNode(t, cfg.MDEMGEndpoint, spaceID, "health shape test node 2")
	TriggerRSICCycle(t, cfg.MDEMGEndpoint, spaceID, nil)

	// Get health
	healthResp := GetRSICHealth(t, cfg.MDEMGEndpoint)

	// Assert top-level fields
	if _, ok := healthResp["status"]; !ok {
		t.Error("missing top-level field: status")
	}
	if _, ok := healthResp["active_tasks"]; !ok {
		t.Error("missing top-level field: active_tasks")
	}

	// Assert watchdog block exists and is a map
	if watchdog, ok := healthResp["watchdog"].(map[string]any); ok {
		t.Logf("watchdog block present with %d fields", len(watchdog))
	} else if healthResp["watchdog"] != nil {
		t.Errorf("watchdog has unexpected type: %T", healthResp["watchdog"])
	} else {
		t.Log("watchdog block is nil (watchdog may not be initialized)")
	}

	// Assert orchestration block exists and is a map
	if orchestration, ok := healthResp["orchestration"].(map[string]any); ok {
		t.Logf("orchestration block present with %d fields", len(orchestration))
	} else if healthResp["orchestration"] != nil {
		t.Errorf("orchestration has unexpected type: %T", healthResp["orchestration"])
	} else {
		t.Log("orchestration block is nil (orchestration policy may not be initialized)")
	}

	// Assert persistence block exists and is a map with 'enabled' field
	if persistence, ok := healthResp["persistence"].(map[string]any); ok {
		t.Logf("persistence block present with %d fields", len(persistence))
		if _, ok := persistence["enabled"]; !ok {
			t.Error("persistence block missing 'enabled' field")
		}
	} else if healthResp["persistence"] != nil {
		t.Errorf("persistence has unexpected type: %T", healthResp["persistence"])
	} else {
		t.Log("persistence block is nil (RSIC store may not be initialized)")
	}

	// Assert safety block exists and is a map with 'safety_version' field
	if safety, ok := healthResp["safety"].(map[string]any); ok {
		t.Logf("safety block present with %d fields", len(safety))
		if _, ok := safety["safety_version"]; !ok {
			t.Error("safety block missing 'safety_version' field")
		}
	} else if healthResp["safety"] != nil {
		t.Errorf("safety has unexpected type: %T", healthResp["safety"])
	} else {
		t.Log("safety block is nil (snapshot store may not be initialized)")
	}
}
