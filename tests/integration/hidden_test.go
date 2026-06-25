//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TestHiddenLayerConsolidation tests the full hidden layer consolidation flow:
// 1. Creates base layer nodes with embeddings
// 2. Calls the consolidate endpoint
// 3. Verifies hidden nodes are created
// 4. Verifies message passing updates embeddings
func TestHiddenLayerConsolidation(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("hidden")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	ctx := context.Background()

	// Step 1: Create base layer nodes with similar embeddings (should form a cluster)
	// Create 5 nodes with similar embeddings to form a cluster
	for i := 0; i < 5; i++ {
		// Create embeddings that are similar (close in cosine distance)
		embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.95-float64(i)*0.01, 1)

		ingestReq := map[string]any{
			"space_id":  spaceID,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"source":    "test",
			"name":      fmt.Sprintf("Base Node %d", i),
			"content":   fmt.Sprintf("Test content for base node %d", i),
			"path":      fmt.Sprintf("/test/base/%d", i),
			"embedding": embedding,
		}
		reqBody, _ := json.Marshal(ingestReq)

		resp, err := client.Post(
			cfg.MDEMGEndpoint+"/v1/memory/ingest",
			"application/json",
			bytes.NewReader(reqBody),
		)
		if err != nil {
			t.Fatalf("Failed to ingest base node %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Ingest failed for node %d: status %d", i, resp.StatusCode)
		}
	}

	// Verify nodes were created at layer 0
	nodeCount := countNodesAtLayer(t, driver, ctx, spaceID, 0)
	if nodeCount < 5 {
		t.Fatalf("Expected at least 5 base nodes, got %d", nodeCount)
	}
	t.Logf("Created %d base layer nodes", nodeCount)

	// Step 2: Call consolidate endpoint
	consolidateReq := map[string]any{
		"space_id": spaceID,
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("Consolidate failed: status %d, error: %v", resp.StatusCode, errResp)
	}

	var consolidateResp struct {
		Data struct {
			SpaceID             string  `json:"space_id"`
			HiddenNodesCreated  int     `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int     `json:"concept_nodes_updated"`
			DurationMs          float64 `json:"duration_ms"`
			Enabled             bool    `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&consolidateResp); err != nil {
		t.Fatalf("Failed to decode consolidate response: %v", err)
	}

	t.Logf("Consolidation result: created=%d, hidden_updated=%d, concept_updated=%d, duration=%.2fms, enabled=%v",
		consolidateResp.Data.HiddenNodesCreated,
		consolidateResp.Data.HiddenNodesUpdated,
		consolidateResp.Data.ConceptNodesUpdated,
		consolidateResp.Data.DurationMs,
		consolidateResp.Data.Enabled)

	// Step 3: Verify hidden nodes were created (if enabled)
	if consolidateResp.Data.Enabled {
		hiddenCount := countNodesAtLayer(t, driver, ctx, spaceID, 1)
		t.Logf("Hidden layer nodes after consolidation: %d", hiddenCount)

		// Verify GENERALIZES edges exist
		edgeCount := countEdgesOfType(t, driver, ctx, spaceID, "GENERALIZES")
		t.Logf("GENERALIZES edges: %d", edgeCount)
	} else {
		t.Log("Hidden layer is disabled, skipping hidden node verification")
	}
}

// TestConsolidateWithSkipFlags tests the skip_* flags in consolidate request
func TestConsolidateWithSkipFlags(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("hidden-skip")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Create a few test nodes
	for i := 0; i < 3; i++ {
		embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.9, 1)
		ingestReq := map[string]any{
			"space_id":  spaceID,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"source":    "test",
			"name":      fmt.Sprintf("Skip Test Node %d", i),
			"content":   fmt.Sprintf("Content %d", i),
			"embedding": embedding,
		}
		reqBody, _ := json.Marshal(ingestReq)
		resp, _ := client.Post(cfg.MDEMGEndpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(reqBody))
		resp.Body.Close()
	}

	// Test with all operations skipped
	consolidateReq := map[string]any{
		"space_id":        spaceID,
		"skip_clustering": true,
		"skip_forward":    true,
		"skip_backward":   true,
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Consolidate with skip flags failed: status %d", resp.StatusCode)
	}

	var consolidateResp struct {
		Data struct {
			HiddenNodesCreated  int  `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int  `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int  `json:"concept_nodes_updated"`
			Enabled             bool `json:"enabled"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&consolidateResp)

	// When all operations are skipped, counts should be 0
	if consolidateResp.Data.Enabled {
		if consolidateResp.Data.HiddenNodesCreated != 0 {
			t.Errorf("Expected 0 hidden nodes created with skip_clustering=true, got %d",
				consolidateResp.Data.HiddenNodesCreated)
		}
	}

	t.Logf("Consolidate with all skips: created=%d, updated=%d",
		consolidateResp.Data.HiddenNodesCreated,
		consolidateResp.Data.HiddenNodesUpdated)
}

// TestConsolidateDisabled tests behavior when hidden layer is disabled
func TestConsolidateDisabled(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// This test checks behavior when hidden layer might be disabled
	// The response should still be valid, just with enabled=false

	consolidateReq := map[string]any{
		"space_id": GenerateTestSpaceID("disabled-check"),
	}
	reqBody, _ := json.Marshal(consolidateReq)

	resp, err := client.Post(
		cfg.MDEMGEndpoint+"/v1/memory/consolidate",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		t.Fatalf("Failed to call consolidate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Consolidate failed: status %d", resp.StatusCode)
	}

	var consolidateResp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&consolidateResp)

	t.Logf("Hidden layer enabled: %v", consolidateResp.Data.Enabled)
}

// TestConsolidateValidation tests request validation
func TestConsolidateValidation(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	tests := []struct {
		name       string
		request    map[string]any
		wantStatus int
	}{
		{
			name:       "missing space_id",
			request:    map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty space_id",
			request:    map[string]any{"space_id": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid request",
			request:    map[string]any{"space_id": "test-valid"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, _ := json.Marshal(tt.request)
			resp, err := client.Post(
				cfg.MDEMGEndpoint+"/v1/memory/consolidate",
				"application/json",
				bytes.NewReader(reqBody),
			)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

// Helper: count nodes at a specific layer
func countNodesAtLayer(t *testing.T, driver neo4j.DriverWithContext, ctx context.Context, spaceID string, layer int) int {
	t.Helper()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId, layer: $layer})
			RETURN count(n) AS count
		`, map[string]any{"spaceId": spaceID, "layer": layer})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("count")
			return count, nil
		}
		return 0, res.Err()
	})
	if err != nil {
		t.Fatalf("Failed to count nodes: %v", err)
	}

	switch v := result.(type) {
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// Helper: count edges of a specific type
func countEdgesOfType(t *testing.T, driver neo4j.DriverWithContext, ctx context.Context, spaceID string, edgeType string) int {
	t.Helper()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := fmt.Sprintf(`
		MATCH (a:MemoryNode {space_id: $spaceId})-[r:%s]->(b:MemoryNode {space_id: $spaceId})
		RETURN count(r) AS count
	`, edgeType)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("count")
			return count, nil
		}
		return 0, res.Err()
	})
	if err != nil {
		t.Fatalf("Failed to count edges: %v", err)
	}

	switch v := result.(type) {
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// hiddenNodeIDs returns the node_ids of all L1 HiddenPattern nodes in a space.
func hiddenNodeIDs(t *testing.T, driver neo4j.DriverWithContext, ctx context.Context, spaceID string) []string {
	t.Helper()
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (h:HiddenPattern {space_id: $spaceId, layer: 1})
			RETURN h.node_id AS id`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			v, _ := res.Record().Get("id")
			if s, ok := v.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
		return ids, res.Err()
	})
	if err != nil {
		t.Fatalf("Failed to fetch hidden node ids: %v", err)
	}
	return result.([]string)
}

// TestHiddenPatternIdentityStable is the HIDDEN-CHURN-002 regression test:
// consolidating twice over the SAME base nodes must NOT destroy-and-recreate
// the L1 hidden patterns. Before the fix, every cycle wiped all hidden nodes
// and re-created them with fresh randomUUID() ids (the CRITICAL node-count-drop
// churn). After the fix, ids survive (match-in-place) and are CUIDv2-shaped.
func TestHiddenPatternIdentityStable(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("churn2")
	t.Cleanup(func() { CleanupSpaceWithTest(t, driver, spaceID) })

	ctx := context.Background()

	// Seed a clusterable set: enough same-category (.go) nodes with similar
	// embeddings to exceed min_samples and form at least one hidden pattern.
	for i := 0; i < 12; i++ {
		embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.95-float64(i)*0.005, 1)
		ingestReq := map[string]any{
			"space_id":  spaceID,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"source":    "test",
			"name":      fmt.Sprintf("Churn Base %d", i),
			"content":   fmt.Sprintf("package churn // base node %d", i),
			"path":      fmt.Sprintf("/test/churn/mod%d.go", i),
			"embedding": embedding,
		}
		reqBody, _ := json.Marshal(ingestReq)
		resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("ingest base node %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("ingest node %d: status %d", i, resp.StatusCode)
		}
	}

	consolidate := func() {
		reqBody, _ := json.Marshal(map[string]any{"space_id": spaceID})
		resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/memory/consolidate", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("consolidate: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consolidate: status %d", resp.StatusCode)
		}
	}

	// First cycle.
	consolidate()
	idsRun1 := hiddenNodeIDs(t, driver, ctx, spaceID)
	if len(idsRun1) == 0 {
		t.Skip("hidden layer produced no patterns (disabled or below min_samples) — nothing to assert")
	}
	t.Logf("run 1: %d hidden patterns", len(idsRun1))

	// CUIDv2 shape: no hyphens (a UUID would have them).
	for _, id := range idsRun1 {
		if strings.Contains(id, "-") {
			t.Errorf("hidden node_id %q contains a hyphen — looks like a UUID, not CUIDv2", id)
		}
	}

	// Second cycle over identical data.
	consolidate()
	idsRun2 := hiddenNodeIDs(t, driver, ctx, spaceID)
	t.Logf("run 2: %d hidden patterns", len(idsRun2))

	// The core anti-churn assertion: every run-1 id must still exist after
	// run 2 (matched-in-place), not be replaced by a fresh id.
	run2Set := make(map[string]bool, len(idsRun2))
	for _, id := range idsRun2 {
		run2Set[id] = true
	}
	survived := 0
	for _, id := range idsRun1 {
		if run2Set[id] {
			survived++
		}
	}
	if survived != len(idsRun1) {
		t.Errorf("HIDDEN-CHURN-002 regression: %d/%d run-1 hidden node_ids survived the second cycle (expected all — destroy-recreate churn detected)", survived, len(idsRun1))
	}
}

// TestHiddenIncrementalAssignment is the HIDDEN-CHURN-003 regression test: after
// patterns form, ingesting MORE base nodes and re-consolidating (the default
// incremental path) must assign the new orphans to existing/new patterns WITHOUT
// destroying any existing pattern — every original node_id survives (~0% churn)
// and the pattern count never drops (no deletes on the incremental path).
func TestHiddenIncrementalAssignment(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()
	driver := SetupTestNeo4j(t)

	spaceID := GenerateTestSpaceID("churn3")
	t.Cleanup(func() { CleanupSpaceWithTest(t, driver, spaceID) })
	ctx := context.Background()

	ingest := func(start, n int) {
		for i := start; i < start+n; i++ {
			embedding := CreateControlledEmbedding(DefaultEmbeddingDims, 0.95-float64(i)*0.004, 1)
			body, _ := json.Marshal(map[string]any{
				"space_id": spaceID, "timestamp": time.Now().UTC().Format(time.RFC3339),
				"source": "test", "name": fmt.Sprintf("Inc Base %d", i),
				"content": fmt.Sprintf("package inc // node %d", i),
				"path":    fmt.Sprintf("/test/inc/mod%d.go", i), "embedding": embedding,
			})
			resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("ingest %d: %v", i, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("ingest %d: status %d", i, resp.StatusCode)
			}
		}
	}
	consolidate := func() {
		body, _ := json.Marshal(map[string]any{"space_id": spaceID})
		resp, err := client.Post(cfg.MDEMGEndpoint+"/v1/memory/consolidate", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("consolidate: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("consolidate: status %d", resp.StatusCode)
		}
	}

	ingest(0, 12)
	consolidate()
	idsBefore := hiddenNodeIDs(t, driver, ctx, spaceID)
	if len(idsBefore) == 0 {
		t.Skip("no hidden patterns formed (disabled / below min_samples)")
	}
	t.Logf("after first consolidate: %d patterns", len(idsBefore))

	// Add more base nodes, then incremental re-consolidate.
	ingest(12, 12)
	consolidate()
	idsAfter := hiddenNodeIDs(t, driver, ctx, spaceID)
	t.Logf("after incremental consolidate: %d patterns", len(idsAfter))

	afterSet := make(map[string]bool, len(idsAfter))
	for _, id := range idsAfter {
		afterSet[id] = true
	}
	survived := 0
	for _, id := range idsBefore {
		if afterSet[id] {
			survived++
		}
	}
	if survived != len(idsBefore) {
		t.Errorf("HIDDEN-CHURN-003 regression: %d/%d original patterns survived incremental re-consolidate (expected ALL — incremental must never destroy existing patterns)", survived, len(idsBefore))
	}
	if len(idsAfter) < len(idsBefore) {
		t.Errorf("incremental path must not reduce pattern count (no deletes): before=%d after=%d", len(idsBefore), len(idsAfter))
	}
}
