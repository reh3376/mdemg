//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// floatNearlyEqual compares two float64 values with a small tolerance
// to account for floating-point precision differences.
// Returns true if the values are within epsilon of each other.
func floatNearlyEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// RetrieveRequest mirrors the API request structure for tests.
type RetrieveRequest struct {
	SpaceID        string    `json:"space_id"`
	QueryText      string    `json:"query_text,omitempty"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty"`
	CandidateK     int       `json:"candidate_k,omitempty"`
	TopK           int       `json:"top_k,omitempty"`
	HopDepth       int       `json:"hop_depth,omitempty"`
}

// RetrieveResult mirrors the API result structure for tests.
type RetrieveResult struct {
	NodeID     string  `json:"node_id"`
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary"`
	Score      float64 `json:"score"`
	VectorSim  float64 `json:"vector_sim,omitempty"`
	Activation float64 `json:"activation,omitempty"`
}

// RetrieveResponse mirrors the API response structure for tests.
type RetrieveResponse struct {
	SpaceID string           `json:"space_id"`
	Results []RetrieveResult `json:"results"`
	Debug   map[string]any   `json:"debug,omitempty"`
}

// TestIngestAndRetrieve validates the end-to-end flow: ingest a node, then retrieve it.
// This test verifies that:
// 1. A node ingested with an embedding can be found via vector search
// 2. The retrieve API returns the ingested node with correct properties
// 3. Vector similarity scoring works correctly (same embedding should return high similarity)
func TestIngestAndRetrieve(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("retrieve")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Create a test embedding that we'll use for both ingest and retrieve
	testEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)

	// --- Step 1: Ingest a node with a known embedding ---
	confidence := 0.9
	ingestReq := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "test-source-retrieve",
		Content:     "This is test content for retrieval testing",
		Tags:        []string{"retrieve", "integration", "test"},
		Name:        "retrieve-test-node",
		Path:        "/test/retrieve/node1",
		Sensitivity: "internal",
		Confidence:  &confidence,
		Embedding:   testEmbedding,
	}

	ingestBody, err := json.Marshal(ingestReq)
	if err != nil {
		t.Fatalf("failed to marshal ingest request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpIngestReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
	if err != nil {
		t.Fatalf("failed to create ingest HTTP request: %v", err)
	}
	httpIngestReq.Header.Set("Content-Type", "application/json")

	ingestResp, err := client.Do(httpIngestReq)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer ingestResp.Body.Close()

	if ingestResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(ingestResp.Body).Decode(&errResp)
		t.Fatalf("ingest returned status %d: %v", ingestResp.StatusCode, errResp)
	}

	var ingestResponse IngestResponse
	if err := json.NewDecoder(ingestResp.Body).Decode(&ingestResponse); err != nil {
		t.Fatalf("failed to decode ingest response: %v", err)
	}

	if ingestResponse.NodeID == "" {
		t.Fatal("ingest response node_id is empty")
	}

	ingestedNodeID := ingestResponse.NodeID
	t.Logf("Ingested node with ID: %s", ingestedNodeID)

	// Small delay to ensure data is committed and indexed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Retrieve using the same embedding ---
	retrieveReq := RetrieveRequest{
		SpaceID:        spaceID,
		QueryEmbedding: testEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2,
	}

	retrieveBody, err := json.Marshal(retrieveReq)
	if err != nil {
		t.Fatalf("failed to marshal retrieve request: %v", err)
	}

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"
	httpRetrieveReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
	if err != nil {
		t.Fatalf("failed to create retrieve HTTP request: %v", err)
	}
	httpRetrieveReq.Header.Set("Content-Type", "application/json")

	retrieveResp, err := client.Do(httpRetrieveReq)
	if err != nil {
		t.Fatalf("retrieve request failed: %v", err)
	}
	defer retrieveResp.Body.Close()

	if retrieveResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(retrieveResp.Body).Decode(&errResp)
		t.Fatalf("retrieve returned status %d: %v", retrieveResp.StatusCode, errResp)
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(retrieveResp.Body).Decode(&retrieveResponse); err != nil {
		t.Fatalf("failed to decode retrieve response: %v", err)
	}

	// --- Step 3: Verify the response ---

	// Check space_id matches
	if retrieveResponse.SpaceID != spaceID {
		t.Errorf("retrieve response space_id mismatch: got %q, want %q", retrieveResponse.SpaceID, spaceID)
	}

	// Check that results are not empty
	if len(retrieveResponse.Results) == 0 {
		t.Fatal("retrieve returned no results - expected at least the ingested node")
	}

	// Check that the ingested node is in the results
	var foundNode *RetrieveResult
	for i, result := range retrieveResponse.Results {
		if result.NodeID == ingestedNodeID {
			foundNode = &retrieveResponse.Results[i]
			break
		}
	}

	if foundNode == nil {
		t.Fatalf("ingested node %q not found in retrieve results. Got results: %+v", ingestedNodeID, retrieveResponse.Results)
	}

	// Verify node properties match
	if foundNode.Path != ingestReq.Path {
		t.Errorf("result path mismatch: got %q, want %q", foundNode.Path, ingestReq.Path)
	}
	if foundNode.Name != ingestReq.Name {
		t.Errorf("result name mismatch: got %q, want %q", foundNode.Name, ingestReq.Name)
	}

	// Score should be > 0 for a matching node
	if foundNode.Score <= 0 {
		t.Errorf("result score should be positive, got %f", foundNode.Score)
	}

	// Since we used the exact same embedding for ingest and retrieve,
	// the vector similarity should be high (close to 1.0 for cosine similarity)
	if foundNode.VectorSim <= 0.5 {
		t.Errorf("vector_sim should be high for identical embedding, got %f", foundNode.VectorSim)
	}

	// Verify debug information is present
	if retrieveResponse.Debug == nil {
		t.Error("retrieve response debug is nil - expected debug information")
	} else {
		// Check for expected debug fields
		if _, ok := retrieveResponse.Debug["candidate_k"]; !ok {
			t.Error("debug missing candidate_k field")
		}
		if _, ok := retrieveResponse.Debug["hop_depth"]; !ok {
			t.Error("debug missing hop_depth field")
		}
	}

	t.Logf("Successfully retrieved ingested node with score=%f, vector_sim=%f", foundNode.Score, foundNode.VectorSim)
}

// TestGraphExpansion verifies that graph traversal retrieves related nodes via ASSOCIATED_WITH edges.
// This test validates the bounded expansion phase of the retrieval pipeline:
// 1. Vector recall finds the seed node
// 2. Graph expansion traverses ASSOCIATED_WITH edges to find related nodes
// 3. Related nodes appear in results even if they have low vector similarity to the query
func TestGraphExpansion(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("graph-expansion")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- Step 1: Ingest a "seed" node with a known embedding ---
	// This node will be found via vector recall and serve as the expansion starting point
	seedEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)
	seedConfidence := 0.9

	seedIngestReq := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "test-source-graph-expansion",
		Content:     "This is the seed node for graph expansion testing",
		Tags:        []string{"graph", "expansion", "seed"},
		Name:        "seed-node",
		Path:        "/test/graph-expansion/seed",
		Sensitivity: "internal",
		Confidence:  &seedConfidence,
		Embedding:   seedEmbedding,
	}

	seedIngestBody, err := json.Marshal(seedIngestReq)
	if err != nil {
		t.Fatalf("failed to marshal seed ingest request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpSeedIngestReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(seedIngestBody))
	if err != nil {
		t.Fatalf("failed to create seed ingest HTTP request: %v", err)
	}
	httpSeedIngestReq.Header.Set("Content-Type", "application/json")

	seedIngestResp, err := client.Do(httpSeedIngestReq)
	if err != nil {
		t.Fatalf("seed ingest request failed: %v", err)
	}
	defer seedIngestResp.Body.Close()

	if seedIngestResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(seedIngestResp.Body).Decode(&errResp)
		t.Fatalf("seed ingest returned status %d: %v", seedIngestResp.StatusCode, errResp)
	}

	var seedResponse IngestResponse
	if err := json.NewDecoder(seedIngestResp.Body).Decode(&seedResponse); err != nil {
		t.Fatalf("failed to decode seed ingest response: %v", err)
	}

	seedNodeID := seedResponse.NodeID
	t.Logf("Ingested seed node with ID: %s", seedNodeID)

	// --- Step 2: Ingest "related" nodes with DIFFERENT embeddings ---
	// These nodes have embeddings that are dissimilar to the query,
	// so they should only be found via graph expansion, not vector recall
	type relatedNode struct {
		name   string
		path   string
		seed   float32 // Different seed values produce dissimilar embeddings
		nodeID string
	}

	relatedNodes := []relatedNode{
		{name: "related-node-1", path: "/test/graph-expansion/related1", seed: 100.0},
		{name: "related-node-2", path: "/test/graph-expansion/related2", seed: 200.0},
	}

	for i := range relatedNodes {
		rn := &relatedNodes[i]
		relatedEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, rn.seed)
		confidence := 0.7

		relatedIngestReq := IngestRequest{
			SpaceID:     spaceID,
			Timestamp:   time.Now().Format(time.RFC3339),
			Source:      "test-source-graph-expansion",
			Content:     "Related content for " + rn.name,
			Tags:        []string{"graph", "expansion", "related"},
			Name:        rn.name,
			Path:        rn.path,
			Sensitivity: "internal",
			Confidence:  &confidence,
			Embedding:   relatedEmbedding,
		}

		relatedIngestBody, err := json.Marshal(relatedIngestReq)
		if err != nil {
			t.Fatalf("failed to marshal %s ingest request: %v", rn.name, err)
		}

		httpRelatedReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(relatedIngestBody))
		if err != nil {
			t.Fatalf("failed to create %s ingest HTTP request: %v", rn.name, err)
		}
		httpRelatedReq.Header.Set("Content-Type", "application/json")

		relatedResp, err := client.Do(httpRelatedReq)
		if err != nil {
			t.Fatalf("%s ingest request failed: %v", rn.name, err)
		}
		defer relatedResp.Body.Close()

		if relatedResp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(relatedResp.Body).Decode(&errResp)
			t.Fatalf("%s ingest returned status %d: %v", rn.name, relatedResp.StatusCode, errResp)
		}

		var relatedResponse IngestResponse
		if err := json.NewDecoder(relatedResp.Body).Decode(&relatedResponse); err != nil {
			t.Fatalf("failed to decode %s ingest response: %v", rn.name, err)
		}

		rn.nodeID = relatedResponse.NodeID
		t.Logf("Ingested %s with ID: %s", rn.name, rn.nodeID)
	}

	// --- Step 3: Create ASSOCIATED_WITH edges from seed to related nodes ---
	// This must be done directly in Neo4j since the ingest API doesn't create edges
	createAssociatedWithEdges(t, driver, spaceID, seedNodeID, []string{relatedNodes[0].nodeID, relatedNodes[1].nodeID})

	// Small delay to ensure data is committed and indexed
	time.Sleep(500 * time.Millisecond)

	// --- Step 4: Query using the seed node's embedding ---
	// The query embedding matches the seed node, which should trigger graph expansion
	retrieveReq := RetrieveRequest{
		SpaceID:        spaceID,
		QueryEmbedding: seedEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2, // Enable 2-hop expansion to find related nodes
	}

	retrieveBody, err := json.Marshal(retrieveReq)
	if err != nil {
		t.Fatalf("failed to marshal retrieve request: %v", err)
	}

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"
	httpRetrieveReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
	if err != nil {
		t.Fatalf("failed to create retrieve HTTP request: %v", err)
	}
	httpRetrieveReq.Header.Set("Content-Type", "application/json")

	retrieveResp, err := client.Do(httpRetrieveReq)
	if err != nil {
		t.Fatalf("retrieve request failed: %v", err)
	}
	defer retrieveResp.Body.Close()

	if retrieveResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(retrieveResp.Body).Decode(&errResp)
		t.Fatalf("retrieve returned status %d: %v", retrieveResp.StatusCode, errResp)
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(retrieveResp.Body).Decode(&retrieveResponse); err != nil {
		t.Fatalf("failed to decode retrieve response: %v", err)
	}

	// --- Step 5: Verify results include both seed and related nodes ---

	// Check that results are not empty
	if len(retrieveResponse.Results) == 0 {
		t.Fatal("retrieve returned no results - expected seed and related nodes")
	}

	t.Logf("Retrieve returned %d results", len(retrieveResponse.Results))

	// Find the seed node in results (should have high vector_sim)
	var foundSeed *RetrieveResult
	for i, result := range retrieveResponse.Results {
		if result.NodeID == seedNodeID {
			foundSeed = &retrieveResponse.Results[i]
			break
		}
	}

	if foundSeed == nil {
		t.Fatalf("seed node %q not found in retrieve results. Got results: %+v", seedNodeID, retrieveResponse.Results)
	}

	t.Logf("Found seed node: score=%f, vector_sim=%f, activation=%f", foundSeed.Score, foundSeed.VectorSim, foundSeed.Activation)

	// Verify seed node has high vector similarity (since we used its embedding for the query)
	if foundSeed.VectorSim < 0.9 {
		t.Errorf("seed node should have high vector_sim, got %f", foundSeed.VectorSim)
	}

	// Check that related nodes are in the results (they should be found via graph expansion)
	foundRelatedCount := 0
	for _, rn := range relatedNodes {
		for i, result := range retrieveResponse.Results {
			if result.NodeID == rn.nodeID {
				foundRelatedCount++
				t.Logf("Found %s: score=%f, vector_sim=%f, activation=%f", rn.name, result.Score, result.VectorSim, result.Activation)

				// Related nodes should have lower vector_sim (different embeddings)
				// but positive activation (received activation from seed via graph expansion)
				if retrieveResponse.Results[i].Activation <= 0 {
					t.Errorf("%s should have positive activation from graph expansion, got %f", rn.name, result.Activation)
				}
				break
			}
		}
	}

	if foundRelatedCount < len(relatedNodes) {
		t.Errorf("expected to find %d related nodes via graph expansion, found %d", len(relatedNodes), foundRelatedCount)
		t.Logf("Results: %+v", retrieveResponse.Results)
	}

	// Verify debug info shows edges were fetched (evidence of graph expansion)
	if retrieveResponse.Debug != nil {
		if edgesFetched, ok := retrieveResponse.Debug["edges_fetched"]; ok {
			t.Logf("Debug: edges_fetched=%v", edgesFetched)
			// We created 2 edges, so at least 2 should have been fetched
			if edgeCount, ok := edgesFetched.(float64); ok && edgeCount < 2 {
				t.Errorf("expected at least 2 edges to be fetched for graph expansion, got %v", edgesFetched)
			}
		}
	}

	t.Logf("Graph expansion test passed: found seed node and %d related nodes via ASSOCIATED_WITH edges", foundRelatedCount)
}

// createAssociatedWithEdges creates ASSOCIATED_WITH edges from srcNodeID to each of the dstNodeIDs.
// This helper is used to set up graph relationships for testing graph expansion.
func createAssociatedWithEdges(t *testing.T, driver neo4j.DriverWithContext, spaceID, srcNodeID string, dstNodeIDs []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	for _, dstNodeID := range dstNodeIDs {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `
				MATCH (src:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
				MATCH (dst:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
				MERGE (src)-[r:ASSOCIATED_WITH {space_id: $spaceId}]->(dst)
				ON CREATE SET r.weight = 0.8,
				              r.dim_semantic = 0.7,
				              r.dim_temporal = 0.1,
				              r.dim_coactivation = 0.1,
				              r.status = 'active',
				              r.created_at = datetime(),
				              r.updated_at = datetime()
				RETURN r
			`
			res, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId":   spaceID,
				"srcNodeId": srcNodeID,
				"dstNodeId": dstNodeID,
			})
			if err != nil {
				return nil, err
			}

			// Consume result to ensure the query executed
			if !res.Next(ctx) {
				return nil, fmt.Errorf("failed to create edge from %s to %s - nodes may not exist", srcNodeID, dstNodeID)
			}

			return nil, res.Err()
		})
		if err != nil {
			t.Fatalf("failed to create ASSOCIATED_WITH edge from %s to %s: %v", srcNodeID, dstNodeID, err)
		}
		t.Logf("Created ASSOCIATED_WITH edge: %s -> %s", srcNodeID, dstNodeID)
	}
}

// TestScoringDeterminism verifies that identical queries produce identical scores across multiple runs.
// This test ensures the retrieval pipeline (vector recall, spreading activation, scoring) is deterministic.
// According to doc 12_Retrieval_Scoring_Worked_Examples.md, scoring is computed as:
//
//	S_i = α*v_i + β*a_i + γ*r_i + δ*c_i - φ*h_i - κ*d_i
//
// All these factors should produce identical results for identical inputs.
func TestScoringDeterminism(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("scoring-determinism")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- Step 1: Ingest multiple nodes with known embeddings ---
	// Create several nodes to ensure scoring involves non-trivial computation
	// (vector similarity, activation spread, hub penalties)
	testNodes := []struct {
		name       string
		path       string
		seed       float32
		confidence float64
	}{
		{"node-alpha", "/test/scoring/alpha", 1.0, 0.9},
		{"node-beta", "/test/scoring/beta", 1.5, 0.8},
		{"node-gamma", "/test/scoring/gamma", 2.0, 0.7},
		{"node-delta", "/test/scoring/delta", 2.5, 0.6},
		{"node-epsilon", "/test/scoring/epsilon", 3.0, 0.5},
	}

	for _, node := range testNodes {
		embedding := CreateTestEmbedding(DefaultEmbeddingDims, node.seed)
		confidence := node.confidence

		ingestReq := IngestRequest{
			SpaceID:     spaceID,
			Timestamp:   time.Now().Format(time.RFC3339),
			Source:      "test-source-determinism",
			Content:     "Content for " + node.name + " - scoring determinism test",
			Tags:        []string{"determinism", "test"},
			Name:        node.name,
			Path:        node.path,
			Sensitivity: "internal",
			Confidence:  &confidence,
			Embedding:   embedding,
		}

		ingestBody, err := json.Marshal(ingestReq)
		if err != nil {
			t.Fatalf("failed to marshal ingest request for %s: %v", node.name, err)
		}

		ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
		httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(ingestBody))
		if err != nil {
			t.Fatalf("failed to create ingest HTTP request for %s: %v", node.name, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("ingest request failed for %s: %v", node.name, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("ingest returned status %d for %s: %v", resp.StatusCode, node.name, errResp)
		}
	}

	// Allow time for data to be committed and indexed
	time.Sleep(500 * time.Millisecond)

	// --- Step 2: Build the query that will be repeated ---
	// Use a known embedding that will match our test nodes at varying similarities
	queryEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.25) // Between alpha and beta

	retrieveReq := RetrieveRequest{
		SpaceID:        spaceID,
		QueryEmbedding: queryEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2,
	}

	// --- Step 3: Run the same query multiple times and collect results ---
	numRuns := 5
	allResponses := make([]RetrieveResponse, numRuns)

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"

	for i := 0; i < numRuns; i++ {
		retrieveBody, err := json.Marshal(retrieveReq)
		if err != nil {
			t.Fatalf("run %d: failed to marshal retrieve request: %v", i+1, err)
		}

		httpReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
		if err != nil {
			t.Fatalf("run %d: failed to create retrieve HTTP request: %v", i+1, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("run %d: retrieve request failed: %v", i+1, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]any
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("run %d: retrieve returned status %d: %v", i+1, resp.StatusCode, errResp)
		}

		if err := json.NewDecoder(resp.Body).Decode(&allResponses[i]); err != nil {
			t.Fatalf("run %d: failed to decode retrieve response: %v", i+1, err)
		}
	}

	// --- Step 4: Verify all runs produced identical results ---
	baseline := allResponses[0]

	// First, check that we got results
	if len(baseline.Results) == 0 {
		t.Fatal("baseline query returned no results - cannot verify determinism")
	}

	t.Logf("Baseline query returned %d results", len(baseline.Results))

	for runIdx := 1; runIdx < numRuns; runIdx++ {
		current := allResponses[runIdx]

		// Check result count matches
		if len(current.Results) != len(baseline.Results) {
			t.Errorf("run %d: result count differs from baseline: got %d, want %d",
				runIdx+1, len(current.Results), len(baseline.Results))
			continue
		}

		// Check each result matches exactly
		for i, baseResult := range baseline.Results {
			currResult := current.Results[i]

			// Verify node ordering is identical
			if currResult.NodeID != baseResult.NodeID {
				t.Errorf("run %d, position %d: node_id differs: got %q, want %q",
					runIdx+1, i, currResult.NodeID, baseResult.NodeID)
			}

			// Verify vector similarity is nearly identical (not affected by Hebbian learning)
			const vectorEpsilon = 1e-6
			if !floatNearlyEqual(currResult.VectorSim, baseResult.VectorSim, vectorEpsilon) {
				t.Errorf("run %d, position %d (node %s): vector_sim differs beyond tolerance: got %.15f, want %.15f",
					runIdx+1, i, baseResult.NodeID, currResult.VectorSim, baseResult.VectorSim)
			}

			// Scores and activations may drift slightly due to Hebbian learning
			// being triggered on each retrieve call. Use wider tolerance.
			const scoreTolerance = 0.05 // Allow 5% drift due to learning effects
			if !floatNearlyEqual(currResult.Score, baseResult.Score, scoreTolerance) {
				t.Errorf("run %d, position %d (node %s): score differs beyond tolerance: got %.6f, want %.6f",
					runIdx+1, i, baseResult.NodeID, currResult.Score, baseResult.Score)
			}

			const activationTolerance = 0.10 // Activations can drift more due to edge weight changes
			if !floatNearlyEqual(currResult.Activation, baseResult.Activation, activationTolerance) {
				t.Errorf("run %d, position %d (node %s): activation differs beyond tolerance: got %.6f, want %.6f",
					runIdx+1, i, baseResult.NodeID, currResult.Activation, baseResult.Activation)
			}
		}
	}

	// Log summary of baseline results for debugging
	t.Logf("Scoring determinism verified across %d runs with %d results each", numRuns, len(baseline.Results))
	for i, result := range baseline.Results {
		t.Logf("  Position %d: node=%s, score=%.6f, vector_sim=%.6f, activation=%.6f",
			i, result.NodeID, result.Score, result.VectorSim, result.Activation)
	}
}

// TestEmptyGraphHandling verifies that querying a non-existent space returns graceful empty results.
// This test ensures the retrieval pipeline handles the edge case of:
// 1. A space_id that has never been used (no TapRoot, no MemoryNodes)
// 2. Returns HTTP 200 with an empty results array (not an error)
// 3. Includes proper response structure with space_id echoed back
func TestEmptyGraphHandling(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Generate a unique space_id that definitely doesn't exist in the database
	// Using a very specific format to ensure it's never been used
	nonExistentSpaceID := GenerateTestSpaceID("empty-graph-nonexistent")

	// Note: No cleanup needed since we're not creating any data
	// The space doesn't exist and we're only reading

	// Create a test embedding for the query
	queryEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 42.0)

	// --- Step 1: Query the non-existent space ---
	retrieveReq := RetrieveRequest{
		SpaceID:        nonExistentSpaceID,
		QueryEmbedding: queryEmbedding,
		CandidateK:     50,
		TopK:           10,
		HopDepth:       2,
	}

	retrieveBody, err := json.Marshal(retrieveReq)
	if err != nil {
		t.Fatalf("failed to marshal retrieve request: %v", err)
	}

	retrieveURL := cfg.MDEMGEndpoint + "/v1/memory/retrieve"
	httpRetrieveReq, err := http.NewRequest(http.MethodPost, retrieveURL, bytes.NewReader(retrieveBody))
	if err != nil {
		t.Fatalf("failed to create retrieve HTTP request: %v", err)
	}
	httpRetrieveReq.Header.Set("Content-Type", "application/json")

	retrieveResp, err := client.Do(httpRetrieveReq)
	if err != nil {
		t.Fatalf("retrieve request failed: %v", err)
	}
	defer retrieveResp.Body.Close()

	// --- Step 2: Verify HTTP 200 OK response (not an error status) ---
	if retrieveResp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(retrieveResp.Body).Decode(&errResp)
		t.Fatalf("expected HTTP 200 for empty graph query, got status %d: %v", retrieveResp.StatusCode, errResp)
	}

	var retrieveResponse RetrieveResponse
	if err := json.NewDecoder(retrieveResp.Body).Decode(&retrieveResponse); err != nil {
		t.Fatalf("failed to decode retrieve response: %v", err)
	}

	// --- Step 3: Verify the response structure ---

	// Space ID should be echoed back correctly
	if retrieveResponse.SpaceID != nonExistentSpaceID {
		t.Errorf("response space_id mismatch: got %q, want %q", retrieveResponse.SpaceID, nonExistentSpaceID)
	}

	// Results should be an empty array (not nil)
	if retrieveResponse.Results == nil {
		t.Error("response results should be an empty array, not nil")
	}

	// Results should have zero elements
	if len(retrieveResponse.Results) != 0 {
		t.Errorf("expected empty results for non-existent space, got %d results: %+v",
			len(retrieveResponse.Results), retrieveResponse.Results)
	}

	// Debug info should still be present (pipeline ran, just found nothing)
	if retrieveResponse.Debug == nil {
		t.Log("debug info is nil - this is acceptable but may indicate the endpoint skipped debug for empty results")
	} else {
		// If debug info is present, it should have expected fields
		t.Logf("debug info present: %+v", retrieveResponse.Debug)

		// Verify candidate_k is reflected
		if candidateK, ok := retrieveResponse.Debug["candidate_k"]; ok {
			t.Logf("debug candidate_k: %v", candidateK)
		}

		// Verify hop_depth is reflected
		if hopDepth, ok := retrieveResponse.Debug["hop_depth"]; ok {
			t.Logf("debug hop_depth: %v", hopDepth)
		}
	}

	t.Logf("Empty graph handling test passed: non-existent space %q returned HTTP 200 with empty results array", nonExistentSpaceID)
}
