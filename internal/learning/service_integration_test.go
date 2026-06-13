// Integration tests for the learning service
// These tests require a running Neo4j instance
//
// Run with: go test -tags=integration ./internal/learning/... -v
// Requires environment variables: NEO4J_URI, NEO4J_USER, NEO4J_PASS

//go:build integration

package learning

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/models"
)

// testConfig returns a config suitable for integration tests
func testConfig() config.Config {
	return config.Config{
		Neo4jURI:                  os.Getenv("NEO4J_URI"),
		Neo4jUser:                 os.Getenv("NEO4J_USER"),
		Neo4jPass:                 os.Getenv("NEO4J_PASS"),
		LearningEdgeCapPerRequest: 200,
		LearningMinActivation:     0.20,
		LearningEta:               0.02,
		LearningMu:                0.01,
		LearningWMin:              0.0,
		LearningWMax:              1.0,
	}
}

// setupTestDriver creates a Neo4j driver for tests
func setupTestDriver(t *testing.T) neo4j.DriverWithContext {
	cfg := testConfig()
	if cfg.Neo4jURI == "" || cfg.Neo4jUser == "" || cfg.Neo4jPass == "" {
		t.Skip("Skipping integration test: NEO4J_URI, NEO4J_USER, NEO4J_PASS required")
	}

	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		t.Fatalf("Failed to create Neo4j driver: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		t.Skipf("Skipping integration test: Neo4j not reachable: %v", err)
	}

	return driver
}

// setupTestNodes creates test MemoryNodes in a test space
// Returns a cleanup function that should be deferred
func setupTestNodes(t *testing.T, ctx context.Context, driver neo4j.DriverWithContext, spaceID string, nodeIDs []string) func() {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Create test nodes
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		for _, nodeID := range nodeIDs {
			cypher := `
				MERGE (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
				ON CREATE SET n.created_at = datetime(), n.name = $nodeId, n.path = '/test/' + $nodeId
				RETURN n.node_id
			`
			_, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId": spaceID,
				"nodeId":  nodeID,
			})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Failed to create test nodes: %v", err)
	}

	// Return cleanup function
	return func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cleanupSess := driver.NewSession(cleanupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer cleanupSess.Close(cleanupCtx)

		// Delete test nodes and their relationships
		_, _ = cleanupSess.ExecuteWrite(cleanupCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `
				MATCH (n:MemoryNode {space_id: $spaceId})
				WHERE n.node_id IN $nodeIds
				DETACH DELETE n
			`
			_, err := tx.Run(cleanupCtx, cypher, map[string]any{
				"spaceId": spaceID,
				"nodeIds": nodeIDs,
			})
			return nil, err
		})
	}
}

// getEdge retrieves a CO_ACTIVATED_WITH edge between two nodes
func getEdge(ctx context.Context, driver neo4j.DriverWithContext, spaceID, srcID, dstID string) (map[string]any, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcId})
			      -[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->
			      (b:MemoryNode {space_id: $spaceId, node_id: $dstId})
			RETURN r.edge_id AS edge_id,
			       r.weight AS weight,
			       r.evidence_count AS evidence_count,
			       r.version AS version,
			       r.status AS status,
			       r.created_at AS created_at,
			       r.updated_at AS updated_at,
			       r.last_activated_at AS last_activated_at,
			       r.dim_coactivation AS dim_coactivation,
			       r.decay_rate AS decay_rate
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"srcId":   srcID,
			"dstId":   dstID,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, nil // No edge found
		}
		rec := res.Record()
		props := make(map[string]any)
		for _, key := range rec.Keys {
			v, _ := rec.Get(key)
			props[key] = v
		}
		return props, nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(map[string]any), nil
}

// TestApplyCoactivationCreatesEdges tests that ApplyCoactivation creates new edges
func TestApplyCoactivationCreatesEdges(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	spaceID := "test-learning-create-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"node-a", "node-b", "node-c"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	// Create the learning service
	svc := NewService(cfg, driver)

	// Simulate retrieval results with sufficient activation
	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "node-a", Activation: 0.8},
			{NodeID: "node-b", Activation: 0.6},
			{NodeID: "node-c", Activation: 0.4},
		},
	}

	// Apply coactivation
	err := svc.ApplyCoactivation(ctx, spaceID, resp)
	if err != nil {
		t.Fatalf("ApplyCoactivation failed: %v", err)
	}

	// Verify edges were created
	// With 3 nodes above threshold (0.20), we expect 3 pairs: (a,b), (a,c), (b,c)

	// Check edge a->b
	edgeAB, err := getEdge(ctx, driver, spaceID, "node-a", "node-b")
	if err != nil {
		t.Fatalf("Failed to get edge a->b: %v", err)
	}
	if edgeAB == nil {
		t.Error("Expected edge a->b to exist, but it was not found")
	} else {
		verifyNewEdge(t, edgeAB, "a->b")
	}

	// Check reverse edge b->a (symmetry)
	edgeBA, err := getEdge(ctx, driver, spaceID, "node-b", "node-a")
	if err != nil {
		t.Fatalf("Failed to get edge b->a: %v", err)
	}
	if edgeBA == nil {
		t.Error("Expected reverse edge b->a to exist, but it was not found")
	} else {
		verifyNewEdge(t, edgeBA, "b->a")
	}

	// Check edge a->c
	edgeAC, err := getEdge(ctx, driver, spaceID, "node-a", "node-c")
	if err != nil {
		t.Fatalf("Failed to get edge a->c: %v", err)
	}
	if edgeAC == nil {
		t.Error("Expected edge a->c to exist, but it was not found")
	}

	// Check edge b->c
	edgeBC, err := getEdge(ctx, driver, spaceID, "node-b", "node-c")
	if err != nil {
		t.Fatalf("Failed to get edge b->c: %v", err)
	}
	if edgeBC == nil {
		t.Error("Expected edge b->c to exist, but it was not found")
	}
}

// verifyNewEdge checks that a newly created edge has the expected properties
func verifyNewEdge(t *testing.T, edge map[string]any, label string) {
	t.Helper()

	// Check edge_id exists
	if edge["edge_id"] == nil || edge["edge_id"] == "" {
		t.Errorf("[%s] edge_id should be set", label)
	}

	// Check initial weight (0.10 base + Hebbian update)
	if weight, ok := edge["weight"].(float64); ok {
		if weight < 0.0 || weight > 1.0 {
			t.Errorf("[%s] weight should be in [0,1], got %f", label, weight)
		}
	} else {
		t.Errorf("[%s] weight should be float64, got %T", label, edge["weight"])
	}

	// Check evidence_count is 1 for new edges
	if count, ok := edge["evidence_count"].(int64); ok {
		if count != 1 {
			t.Errorf("[%s] evidence_count should be 1 for new edge, got %d", label, count)
		}
	} else {
		t.Errorf("[%s] evidence_count should be int64, got %T", label, edge["evidence_count"])
	}

	// Check version is 1 for new edges
	if version, ok := edge["version"].(int64); ok {
		if version != 1 {
			t.Errorf("[%s] version should be 1 for new edge, got %d", label, version)
		}
	} else {
		t.Errorf("[%s] version should be int64, got %T", label, edge["version"])
	}

	// Check status is 'active'
	if status, ok := edge["status"].(string); ok {
		if status != "active" {
			t.Errorf("[%s] status should be 'active', got %s", label, status)
		}
	} else {
		t.Errorf("[%s] status should be string, got %T", label, edge["status"])
	}

	// Check dim_coactivation is 1.0
	if dimCoact, ok := edge["dim_coactivation"].(float64); ok {
		if dimCoact != 1.0 {
			t.Errorf("[%s] dim_coactivation should be 1.0, got %f", label, dimCoact)
		}
	} else {
		t.Errorf("[%s] dim_coactivation should be float64, got %T", label, edge["dim_coactivation"])
	}

	// Check decay_rate is 0.001
	if decayRate, ok := edge["decay_rate"].(float64); ok {
		if decayRate != 0.001 {
			t.Errorf("[%s] decay_rate should be 0.001, got %f", label, decayRate)
		}
	} else {
		t.Errorf("[%s] decay_rate should be float64, got %T", label, edge["decay_rate"])
	}

	// Check timestamps exist
	if edge["created_at"] == nil {
		t.Errorf("[%s] created_at should be set", label)
	}
	if edge["updated_at"] == nil {
		t.Errorf("[%s] updated_at should be set", label)
	}
	if edge["last_activated_at"] == nil {
		t.Errorf("[%s] last_activated_at should be set", label)
	}
}

// TestApplyCoactivationUpdatesEdges tests that ApplyCoactivation updates existing edges
func TestApplyCoactivationUpdatesEdges(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	spaceID := "test-learning-update-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"node-x", "node-y"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	// First coactivation - creates edges
	resp1 := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "node-x", Activation: 0.7},
			{NodeID: "node-y", Activation: 0.8},
		},
	}

	err := svc.ApplyCoactivation(ctx, spaceID, resp1)
	if err != nil {
		t.Fatalf("First ApplyCoactivation failed: %v", err)
	}

	// Get initial edge state
	initialEdge, err := getEdge(ctx, driver, spaceID, "node-x", "node-y")
	if err != nil {
		t.Fatalf("Failed to get initial edge: %v", err)
	}
	if initialEdge == nil {
		t.Fatal("Expected edge to exist after first coactivation")
	}

	initialWeight := initialEdge["weight"].(float64)
	initialEvidenceCount := initialEdge["evidence_count"].(int64)
	initialVersion := initialEdge["version"].(int64)

	// Small delay to ensure timestamps differ
	time.Sleep(10 * time.Millisecond)

	// Second coactivation - updates edges
	resp2 := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "node-x", Activation: 0.9},
			{NodeID: "node-y", Activation: 0.9},
		},
	}

	err = svc.ApplyCoactivation(ctx, spaceID, resp2)
	if err != nil {
		t.Fatalf("Second ApplyCoactivation failed: %v", err)
	}

	// Get updated edge state
	updatedEdge, err := getEdge(ctx, driver, spaceID, "node-x", "node-y")
	if err != nil {
		t.Fatalf("Failed to get updated edge: %v", err)
	}
	if updatedEdge == nil {
		t.Fatal("Expected edge to still exist after second coactivation")
	}

	updatedWeight := updatedEdge["weight"].(float64)
	updatedEvidenceCount := updatedEdge["evidence_count"].(int64)
	updatedVersion := updatedEdge["version"].(int64)

	// Verify weight increased (high activations should strengthen the edge)
	// With activation product 0.9*0.9=0.81 and eta=0.02, the weight should increase
	if updatedWeight <= initialWeight {
		t.Errorf("Weight should increase with high co-activation: initial=%f, updated=%f",
			initialWeight, updatedWeight)
	}

	// Verify evidence_count incremented
	expectedEvidenceCount := initialEvidenceCount + 1
	if updatedEvidenceCount != expectedEvidenceCount {
		t.Errorf("evidence_count should increment: expected %d, got %d",
			expectedEvidenceCount, updatedEvidenceCount)
	}

	// Verify version incremented
	expectedVersion := initialVersion + 1
	if updatedVersion != expectedVersion {
		t.Errorf("version should increment: expected %d, got %d",
			expectedVersion, updatedVersion)
	}

	// Verify timestamps updated
	if updatedEdge["updated_at"] == initialEdge["updated_at"] {
		t.Error("updated_at should change on update")
	}
	if updatedEdge["last_activated_at"] == initialEdge["last_activated_at"] {
		t.Error("last_activated_at should change on update")
	}

	// Verify edge_id stays the same (MERGE preserves identity)
	if updatedEdge["edge_id"] != initialEdge["edge_id"] {
		t.Errorf("edge_id should be preserved: initial=%v, updated=%v",
			initialEdge["edge_id"], updatedEdge["edge_id"])
	}
}

// TestApplyCoactivationEdgeSymmetry tests that both directions have the same weight
func TestApplyCoactivationEdgeSymmetry(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	spaceID := "test-learning-symmetry-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"sym-a", "sym-b"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "sym-a", Activation: 0.6},
			{NodeID: "sym-b", Activation: 0.8},
		},
	}

	err := svc.ApplyCoactivation(ctx, spaceID, resp)
	if err != nil {
		t.Fatalf("ApplyCoactivation failed: %v", err)
	}

	// Get both directions
	forwardEdge, err := getEdge(ctx, driver, spaceID, "sym-a", "sym-b")
	if err != nil {
		t.Fatalf("Failed to get forward edge: %v", err)
	}
	reverseEdge, err := getEdge(ctx, driver, spaceID, "sym-b", "sym-a")
	if err != nil {
		t.Fatalf("Failed to get reverse edge: %v", err)
	}

	if forwardEdge == nil || reverseEdge == nil {
		t.Fatal("Both forward and reverse edges should exist")
	}

	// Weights should be identical
	forwardWeight := forwardEdge["weight"].(float64)
	reverseWeight := reverseEdge["weight"].(float64)

	if forwardWeight != reverseWeight {
		t.Errorf("Edge weights should be symmetric: forward=%f, reverse=%f",
			forwardWeight, reverseWeight)
	}

	// Both should have the same metadata (except edge_id)
	if forwardEdge["evidence_count"] != reverseEdge["evidence_count"] {
		t.Errorf("evidence_count should be symmetric: forward=%v, reverse=%v",
			forwardEdge["evidence_count"], reverseEdge["evidence_count"])
	}
	if forwardEdge["version"] != reverseEdge["version"] {
		t.Errorf("version should be symmetric: forward=%v, reverse=%v",
			forwardEdge["version"], reverseEdge["version"])
	}
}

// TestApplyCoactivationBelowThreshold tests that nodes below threshold don't create edges
func TestApplyCoactivationBelowThreshold(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	cfg.LearningMinActivation = 0.5 // Set high threshold
	spaceID := "test-learning-threshold-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"low-a", "low-b", "high-c"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	// Only high-c is above threshold
	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "low-a", Activation: 0.3},  // Below threshold
			{NodeID: "low-b", Activation: 0.4},  // Below threshold
			{NodeID: "high-c", Activation: 0.8}, // Above threshold
		},
	}

	err := svc.ApplyCoactivation(ctx, spaceID, resp)
	if err != nil {
		t.Fatalf("ApplyCoactivation failed: %v", err)
	}

	// No edges should be created because only one node is above threshold
	edgeAB, _ := getEdge(ctx, driver, spaceID, "low-a", "low-b")
	edgeAC, _ := getEdge(ctx, driver, spaceID, "low-a", "high-c")
	edgeBC, _ := getEdge(ctx, driver, spaceID, "low-b", "high-c")

	if edgeAB != nil {
		t.Error("Edge between low activation nodes should not be created")
	}
	if edgeAC != nil {
		t.Error("Edge involving low activation node should not be created")
	}
	if edgeBC != nil {
		t.Error("Edge involving low activation node should not be created")
	}
}

// TestApplyCoactivationEmptyResults tests handling of empty or insufficient results
func TestApplyCoactivationEmptyResults(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	spaceID := "test-learning-empty-" + time.Now().Format("20060102150405")

	svc := NewService(cfg, driver)

	tests := []struct {
		name    string
		spaceID string
		resp    models.RetrieveResponse
	}{
		{
			name:    "empty space ID",
			spaceID: "",
			resp: models.RetrieveResponse{
				Results: []models.RetrieveResult{
					{NodeID: "n1", Activation: 0.5},
					{NodeID: "n2", Activation: 0.5},
				},
			},
		},
		{
			name:    "empty results",
			spaceID: spaceID,
			resp:    models.RetrieveResponse{Results: []models.RetrieveResult{}},
		},
		{
			name:    "single result",
			spaceID: spaceID,
			resp: models.RetrieveResponse{
				Results: []models.RetrieveResult{
					{NodeID: "n1", Activation: 0.5},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ApplyCoactivation(ctx, tt.spaceID, tt.resp)
			if err != nil {
				t.Errorf("ApplyCoactivation should handle %s gracefully, got error: %v", tt.name, err)
			}
		})
	}
}

// TestApplyCoactivationMultipleIterations tests cumulative learning over multiple calls
func TestApplyCoactivationMultipleIterations(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	spaceID := "test-learning-iterations-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"iter-a", "iter-b"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "iter-a", Activation: 0.7},
			{NodeID: "iter-b", Activation: 0.8},
		},
	}

	// Apply coactivation multiple times and track weight progression
	var weights []float64
	iterations := 5

	for i := 0; i < iterations; i++ {
		err := svc.ApplyCoactivation(ctx, spaceID, resp)
		if err != nil {
			t.Fatalf("ApplyCoactivation iteration %d failed: %v", i+1, err)
		}

		edge, err := getEdge(ctx, driver, spaceID, "iter-a", "iter-b")
		if err != nil {
			t.Fatalf("Failed to get edge at iteration %d: %v", i+1, err)
		}
		if edge == nil {
			t.Fatalf("Edge should exist at iteration %d", i+1)
		}
		weights = append(weights, edge["weight"].(float64))
	}

	// Verify monotonic weight increase (learning > decay for high activations)
	for i := 1; i < len(weights); i++ {
		if weights[i] <= weights[i-1] {
			t.Errorf("Weight should increase monotonically: weights[%d]=%f <= weights[%d]=%f",
				i, weights[i], i-1, weights[i-1])
		}
	}

	// Verify final evidence count
	finalEdge, _ := getEdge(ctx, driver, spaceID, "iter-a", "iter-b")
	expectedCount := int64(iterations)
	if finalEdge["evidence_count"].(int64) != expectedCount {
		t.Errorf("evidence_count should be %d after %d iterations, got %d",
			expectedCount, iterations, finalEdge["evidence_count"])
	}

	t.Logf("Weight progression over %d iterations: %v", iterations, weights)
}

// TestApplyCoactivationEdgeCapEnforcement tests that the edge write cap is enforced
// This is an important safeguard: with N nodes, there are C(N,2) = N*(N-1)/2 potential pairs
// Without the cap, this could lead to excessive database writes
func TestApplyCoactivationEdgeCapEnforcement(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()

	// Set a small cap for testing (10 edges max)
	cap := 10
	cfg.LearningEdgeCapPerRequest = cap
	cfg.LearningMinActivation = 0.0 // Include all nodes

	spaceID := "test-learning-cap-" + time.Now().Format("20060102150405")

	// Create 10 nodes - this generates C(10,2) = 45 potential pairs
	// With cap=10, only top 10 should be written
	numNodes := 10
	nodeIDs := make([]string, numNodes)
	for i := 0; i < numNodes; i++ {
		nodeIDs[i] = "cap-node-" + strconv.Itoa(i)
	}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	// Create results with varying activation levels
	// Higher-indexed nodes get higher activations so we can predict which pairs win
	results := make([]models.RetrieveResult, numNodes)
	for i := 0; i < numNodes; i++ {
		// Activation ranges from 0.1 to 1.0
		results[i] = models.RetrieveResult{
			NodeID:     nodeIDs[i],
			Activation: 0.1 + 0.9*float64(i)/float64(numNodes-1),
		}
	}

	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: results,
	}

	err := svc.ApplyCoactivation(ctx, spaceID, resp)
	if err != nil {
		t.Fatalf("ApplyCoactivation failed: %v", err)
	}

	// Count actual edges created in the database
	edgeCount, err := countEdges(ctx, driver, spaceID)
	if err != nil {
		t.Fatalf("Failed to count edges: %v", err)
	}

	// Each pair creates 2 directed edges (bidirectional)
	// So cap=10 pairs means 20 directed edges
	expectedEdges := cap * 2
	if edgeCount != expectedEdges {
		t.Errorf("Expected %d directed edges (%d pairs x 2), got %d",
			expectedEdges, cap, edgeCount)
	}

	// Verify that the highest activation product pairs were selected
	// The highest product is between the two highest-activated nodes (indices 8 and 9)
	// node 8: activation = 0.9, node 9: activation = 1.0
	highestPairEdge, err := getEdge(ctx, driver, spaceID, nodeIDs[8], nodeIDs[9])
	if err != nil {
		t.Fatalf("Failed to get highest pair edge: %v", err)
	}
	if highestPairEdge == nil {
		t.Error("Expected edge between highest activated nodes to exist (should be in top-K)")
	}

	// The lowest product pair (indices 0 and 1) should NOT be selected
	// node 0: activation = 0.1, node 1: activation = 0.2
	lowestPairEdge, err := getEdge(ctx, driver, spaceID, nodeIDs[0], nodeIDs[1])
	if err != nil {
		t.Fatalf("Failed to get lowest pair edge: %v", err)
	}
	if lowestPairEdge != nil {
		t.Error("Edge between lowest activated nodes should NOT exist (not in top-K)")
	}

	t.Logf("Edge cap enforcement verified: %d edges created (cap=%d pairs)", edgeCount, cap)
}

// TestApplyCoactivationEdgeCapWithLargeInput tests cap enforcement with more nodes
func TestApplyCoactivationEdgeCapWithLargeInput(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()

	// Set cap to 50 edges
	cap := 50
	cfg.LearningEdgeCapPerRequest = cap
	cfg.LearningMinActivation = 0.0

	spaceID := "test-learning-cap-large-" + time.Now().Format("20060102150405")

	// Create 20 nodes - this generates C(20,2) = 190 potential pairs
	// With cap=50, only top 50 should be written
	numNodes := 20
	nodeIDs := make([]string, numNodes)
	for i := 0; i < numNodes; i++ {
		nodeIDs[i] = "lcap-" + strconv.Itoa(i)
	}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	// Create results with varying activation levels
	results := make([]models.RetrieveResult, numNodes)
	for i := 0; i < numNodes; i++ {
		results[i] = models.RetrieveResult{
			NodeID:     nodeIDs[i],
			Activation: 0.1 + 0.9*float64(i)/float64(numNodes-1),
		}
	}

	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: results,
	}

	err := svc.ApplyCoactivation(ctx, spaceID, resp)
	if err != nil {
		t.Fatalf("ApplyCoactivation failed: %v", err)
	}

	// Count actual edges
	edgeCount, err := countEdges(ctx, driver, spaceID)
	if err != nil {
		t.Fatalf("Failed to count edges: %v", err)
	}

	// cap=50 pairs means 100 directed edges
	expectedEdges := cap * 2
	if edgeCount != expectedEdges {
		t.Errorf("Expected %d directed edges (%d pairs x 2), got %d",
			expectedEdges, cap, edgeCount)
	}

	// Without cap, we would have C(20,2) = 190 pairs = 380 edges
	potentialEdges := numNodes * (numNodes - 1) // directed edges
	t.Logf("Cap enforcement: %d edges created from %d potential (cap=%d pairs)",
		edgeCount, potentialEdges, cap)
}

// countEdges counts the number of CO_ACTIVATED_WITH edges in a space
func countEdges(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH ()-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->()
			RETURN count(r) AS edge_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
		})
		if err != nil {
			return 0, err
		}
		if !res.Next(ctx) {
			return 0, nil
		}
		count, _ := res.Record().Get("edge_count")
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

// TestApplyCoactivationWeightBounds tests that weights stay within bounds
func TestApplyCoactivationWeightBounds(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cfg := testConfig()
	cfg.LearningEta = 0.5 // High learning rate to approach bounds quickly
	cfg.LearningWMin = 0.1
	cfg.LearningWMax = 0.9
	spaceID := "test-learning-bounds-" + time.Now().Format("20060102150405")
	nodeIDs := []string{"bound-a", "bound-b"}

	cleanup := setupTestNodes(t, ctx, driver, spaceID, nodeIDs)
	defer cleanup()

	svc := NewService(cfg, driver)

	// High activations to push weight toward max
	resp := models.RetrieveResponse{
		SpaceID: spaceID,
		Results: []models.RetrieveResult{
			{NodeID: "bound-a", Activation: 1.0},
			{NodeID: "bound-b", Activation: 1.0},
		},
	}

	// Apply many times to hit the upper bound
	for i := 0; i < 20; i++ {
		err := svc.ApplyCoactivation(ctx, spaceID, resp)
		if err != nil {
			t.Fatalf("ApplyCoactivation failed: %v", err)
		}
	}

	edge, err := getEdge(ctx, driver, spaceID, "bound-a", "bound-b")
	if err != nil {
		t.Fatalf("Failed to get edge: %v", err)
	}

	weight := edge["weight"].(float64)
	if weight > cfg.LearningWMax {
		t.Errorf("Weight %f exceeds maximum bound %f", weight, cfg.LearningWMax)
	}
	if weight < cfg.LearningWMin {
		t.Errorf("Weight %f is below minimum bound %f", weight, cfg.LearningWMin)
	}

	t.Logf("Final weight after 20 iterations with high learning rate: %f (max: %f)", weight, cfg.LearningWMax)
}

// mockStabilityReinforcer captures the node IDs it was called with.
type mockStabilityReinforcer struct {
	called []string
}

func (m *mockStabilityReinforcer) UpdateStabilityOnReinforcement(_ context.Context, _, nodeID string) error {
	m.called = append(m.called, nodeID)
	return nil
}

// TestCoactivateSession_ReinforcesStability verifies that session coactivation
// reinforces stability for every conversation observation in the session.
//
// DH-004 E5: prior to this fix, CoactivateSession only created CO_ACTIVATED_WITH
// edges — it never called UpdateStabilityOnReinforcement, so volatile
// observations never graduated. Investigation on mdemg-dev found 4991 volatile
// observations with p99 stability = 0.048 (threshold = 0.8), only 4 ever
// reinforced.
func TestCoactivateSession_ReinforcesStability(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spaceID := "test-coactivate-session-reinforce"
	sessionID := "test-session-reinforce"
	nodeIDs := []string{"obs-reinforce-1", "obs-reinforce-2", "obs-reinforce-3"}

	// Create observation nodes with the exact shape CoactivateSession expects.
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		for i, id := range nodeIDs {
			cypher := `
				MERGE (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
				ON CREATE SET n.created_at = datetime(),
				              n.role_type = 'conversation_observation',
				              n.session_id = $sessionId,
				              n.volatile = true,
				              n.stability_score = 0.1,
				              n.surprise_score = 0.5
				RETURN n.node_id
			`
			_, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId":   spaceID,
				"nodeId":    id,
				"sessionId": sessionID,
				"idx":       i,
			})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	sess.Close(ctx)
	if err != nil {
		t.Fatalf("failed to seed observations: %v", err)
	}

	// Cleanup
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup := driver.NewSession(cleanupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer cleanup.Close(cleanupCtx)
		_, _ = cleanup.ExecuteWrite(cleanupCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(cleanupCtx,
				`MATCH (n:MemoryNode {space_id: $spaceId}) DETACH DELETE n`,
				map[string]any{"spaceId": spaceID})
			return nil, err
		})
	}()

	// Run CoactivateSession with a mock reinforcer.
	cfg := testConfig()
	s := NewService(cfg, driver)
	reinforcer := &mockStabilityReinforcer{}
	s.SetStabilityReinforcer(reinforcer)

	newObsNodeID := nodeIDs[len(nodeIDs)-1]
	if err := s.CoactivateSession(ctx, spaceID, sessionID, newObsNodeID); err != nil {
		t.Fatalf("CoactivateSession: %v", err)
	}

	// Each session observation must have been reinforced exactly once.
	if len(reinforcer.called) != len(nodeIDs) {
		t.Errorf("reinforcer called %d times, want %d; called=%v",
			len(reinforcer.called), len(nodeIDs), reinforcer.called)
	}
	seen := map[string]bool{}
	for _, id := range reinforcer.called {
		seen[id] = true
	}
	for _, want := range nodeIDs {
		if !seen[want] {
			t.Errorf("expected reinforcer called for %q, got %v", want, reinforcer.called)
		}
	}
}
