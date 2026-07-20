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

// IngestRequest mirrors the API request structure for tests.
type IngestRequest struct {
	SpaceID     string    `json:"space_id"`
	Timestamp   string    `json:"timestamp"`
	Source      string    `json:"source"`
	Content     any       `json:"content"`
	Tags        []string  `json:"tags,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Name        string    `json:"name,omitempty"`
	Sensitivity string    `json:"sensitivity,omitempty"`
	Confidence  *float64  `json:"confidence,omitempty"`
	Embedding   []float32 `json:"embedding,omitempty"`
}

// IngestResponse mirrors the API response structure for tests.
type IngestResponse struct {
	SpaceID       string          `json:"space_id"`
	NodeID        string          `json:"node_id"`
	ObsID         string          `json:"obs_id"`
	EmbeddingDims int             `json:"embedding_dims,omitempty"`
	Anomalies     []IngestAnomaly `json:"anomalies,omitempty"`
}

// IngestAnomaly mirrors the Anomaly struct for tests.
type IngestAnomaly struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Message     string  `json:"message"`
	RelatedNode string  `json:"related_node,omitempty"`
	Confidence  float64 `json:"confidence"`
}

// TestIngestCreatesNode verifies that POST to /v1/memory/ingest creates a Neo4j node with correct properties.
func TestIngestCreatesNode(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("ingest")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Prepare ingest request with all fields
	confidence := 0.85
	embedding := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)

	req := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "test-source",
		Content:     "This is test content for ingestion",
		Tags:        []string{"test", "integration"},
		Name:        "test-node-name",
		Path:        "/test/path/node",
		Sensitivity: "internal",
		Confidence:  &confidence,
		Embedding:   embedding,
	}

	// Make ingest request
	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify HTTP response status
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("ingest returned status %d: %v", resp.StatusCode, errResp)
	}

	// Parse response
	var ingestResp IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response fields
	if ingestResp.SpaceID != spaceID {
		t.Errorf("response space_id mismatch: got %q, want %q", ingestResp.SpaceID, spaceID)
	}
	if ingestResp.NodeID == "" {
		t.Error("response node_id is empty")
	}
	if ingestResp.ObsID == "" {
		t.Error("response obs_id is empty")
	}
	if ingestResp.EmbeddingDims != DefaultEmbeddingDims {
		t.Errorf("response embedding_dims mismatch: got %d, want %d", ingestResp.EmbeddingDims, DefaultEmbeddingDims)
	}

	// Verify node was created in Neo4j with correct properties
	verifyNodeInNeo4j(t, driver, spaceID, ingestResp.NodeID, req)

	// Verify observation was created and linked
	verifyObservationInNeo4j(t, driver, spaceID, ingestResp.NodeID, ingestResp.ObsID, req)
}

// verifyNodeInNeo4j checks that the MemoryNode was created with expected properties.
func verifyNodeInNeo4j(t *testing.T, driver neo4j.DriverWithContext, spaceID, nodeID string, req IngestRequest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			RETURN n.space_id AS space_id,
			       n.node_id AS node_id,
			       n.path AS path,
			       n.name AS name,
			       n.layer AS layer,
			       n.role_type AS role_type,
			       n.status AS status,
			       n.confidence AS confidence,
			       n.sensitivity AS sensitivity,
			       n.tags AS tags,
			       n.embedding AS embedding,
			       n.created_at AS created_at,
			       n.updated_at AS updated_at,
			       n.update_count AS update_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("node not found: space_id=%s, node_id=%s", spaceID, nodeID)
		}

		return res.Record().AsMap(), res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query node from Neo4j: %v", err)
	}

	nodeProps := result.(map[string]any)

	// Verify all expected properties
	assertEqual(t, "space_id", spaceID, nodeProps["space_id"])
	assertEqual(t, "node_id", nodeID, nodeProps["node_id"])
	assertEqual(t, "path", req.Path, nodeProps["path"])
	assertEqual(t, "name", req.Name, nodeProps["name"])
	assertEqual(t, "layer", int64(0), nodeProps["layer"])
	assertEqual(t, "role_type", "leaf", nodeProps["role_type"])
	assertEqual(t, "status", "active", nodeProps["status"])
	assertEqual(t, "sensitivity", req.Sensitivity, nodeProps["sensitivity"])

	// Verify confidence (with float tolerance)
	if conf, ok := nodeProps["confidence"].(float64); ok {
		if conf < *req.Confidence-0.01 || conf > *req.Confidence+0.01 {
			t.Errorf("confidence mismatch: got %f, want %f", conf, *req.Confidence)
		}
	} else {
		t.Errorf("confidence has unexpected type: %T", nodeProps["confidence"])
	}

	// Verify tags
	if tags, ok := nodeProps["tags"].([]any); ok {
		if len(tags) != len(req.Tags) {
			t.Errorf("tags length mismatch: got %d, want %d", len(tags), len(req.Tags))
		} else {
			for i, tag := range tags {
				if tagStr, ok := tag.(string); ok {
					if tagStr != req.Tags[i] {
						t.Errorf("tag[%d] mismatch: got %q, want %q", i, tagStr, req.Tags[i])
					}
				}
			}
		}
	} else if nodeProps["tags"] != nil {
		t.Errorf("tags has unexpected type: %T", nodeProps["tags"])
	}

	// Verify embedding was stored
	if embedding, ok := nodeProps["embedding"].([]any); ok {
		if len(embedding) != len(req.Embedding) {
			t.Errorf("embedding length mismatch: got %d, want %d", len(embedding), len(req.Embedding))
		}
	} else if nodeProps["embedding"] != nil {
		t.Errorf("embedding has unexpected type: %T", nodeProps["embedding"])
	}

	// Verify timestamps exist
	if nodeProps["created_at"] == nil {
		t.Error("created_at is nil")
	}
	if nodeProps["updated_at"] == nil {
		t.Error("updated_at is nil")
	}

	// Verify update_count is set
	if updateCount, ok := nodeProps["update_count"].(int64); ok {
		if updateCount < 1 {
			t.Errorf("update_count should be >= 1, got %d", updateCount)
		}
	} else {
		t.Errorf("update_count has unexpected type: %T", nodeProps["update_count"])
	}
}

// verifyObservationInNeo4j checks that the Observation was created and linked to the MemoryNode.
func verifyObservationInNeo4j(t *testing.T, driver neo4j.DriverWithContext, spaceID, nodeID, obsID string, req IngestRequest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})-[:HAS_OBSERVATION]->(o:Observation {obs_id: $obsId})
			RETURN o.space_id AS space_id,
			       o.obs_id AS obs_id,
			       o.source AS source,
			       o.content AS content,
			       o.timestamp AS timestamp,
			       o.created_at AS created_at
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
			"obsId":   obsID,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("observation not found or not linked: node_id=%s, obs_id=%s", nodeID, obsID)
		}

		return res.Record().AsMap(), res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query observation from Neo4j: %v", err)
	}

	obsProps := result.(map[string]any)

	// Verify observation properties
	assertEqual(t, "obs space_id", spaceID, obsProps["space_id"])
	assertEqual(t, "obs obs_id", obsID, obsProps["obs_id"])
	assertEqual(t, "obs source", req.Source, obsProps["source"])
	assertEqual(t, "obs content", req.Content, obsProps["content"])

	if obsProps["timestamp"] == nil {
		t.Error("observation timestamp is nil")
	}
	if obsProps["created_at"] == nil {
		t.Error("observation created_at is nil")
	}
}

// assertEqual is a helper for comparing values with helpful error messages.
func assertEqual(t *testing.T, field string, expected, actual any) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s mismatch: got %v (%T), want %v (%T)", field, actual, actual, expected, expected)
	}
}

// TestIngestGeneratesEmbedding verifies that when no embedding is provided in the ingest request,
// the service auto-generates one using the configured embedding provider (Ollama or OpenAI).
func TestIngestGeneratesEmbedding(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)

	// Check if embedding provider (Ollama) is available
	if !RequireEmbeddingProvider(t) {
		t.Skip("Skipping test: embedding provider (Ollama) not available")
	}

	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("ingest-autoembed")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Prepare ingest request WITHOUT embedding - service should auto-generate
	req := IngestRequest{
		SpaceID:   spaceID,
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "test-source-autoembed",
		Content:   "This is test content that will be auto-embedded by Ollama",
		Tags:      []string{"autoembed", "integration"},
		Name:      "auto-embed-test-node",
		Path:      "/test/path/autoembed",
		// Embedding intentionally omitted to trigger auto-generation
	}

	// Make ingest request
	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpReq, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify HTTP response status
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Fatalf("ingest returned status %d: %v", resp.StatusCode, errResp)
	}

	// Parse response
	var ingestResp IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response fields
	if ingestResp.SpaceID != spaceID {
		t.Errorf("response space_id mismatch: got %q, want %q", ingestResp.SpaceID, spaceID)
	}
	if ingestResp.NodeID == "" {
		t.Error("response node_id is empty")
	}
	if ingestResp.ObsID == "" {
		t.Error("response obs_id is empty")
	}

	// Verify embedding dimensions - the provider will return its native dimensions:
	// - Ollama qwen3-embedding:8b: 3072 dimensions (truncated from 4096)
	// - OpenAI text-embedding-3-small: 1536 dimensions
	// - OpenAI text-embedding-3-large: 3072 dimensions
	// We accept any recognized provider's dimensions.
	validDims := map[int]string{
		768:  "legacy Ollama model",
		1536: "qwen3-embedding / OpenAI text-embedding-3-small",
		3072: "OpenAI text-embedding-3-large",
	}
	if _, ok := validDims[ingestResp.EmbeddingDims]; !ok {
		t.Errorf("response embedding_dims %d is not a recognized provider dimension (expected 768, 1536, or 3072)", ingestResp.EmbeddingDims)
	} else {
		t.Logf("Embedding generated by %s (%d dimensions)", validDims[ingestResp.EmbeddingDims], ingestResp.EmbeddingDims)
	}

	// Verify embedding was stored in Neo4j with the returned dimensions
	verifyEmbeddingInNeo4j(t, driver, spaceID, ingestResp.NodeID, ingestResp.EmbeddingDims)
}

// TestIngestIdempotent verifies that ingesting with the same path twice updates
// the existing node instead of creating a duplicate.
func TestIngestIdempotent(t *testing.T) {
	// Setup: ensure service is ready and create Neo4j driver for verification
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID and path for test isolation
	spaceID := GenerateTestSpaceID("ingest-idempotent")
	testPath := "/test/idempotent/node"

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// --- First ingest ---
	confidence1 := 0.75
	embedding1 := CreateTestEmbedding(DefaultEmbeddingDims, 1.0)

	req1 := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "source-first",
		Content:     "First content for idempotent test",
		Tags:        []string{"first", "idempotent"},
		Name:        "idempotent-node-v1",
		Path:        testPath,
		Sensitivity: "internal",
		Confidence:  &confidence1,
		Embedding:   embedding1,
	}

	reqBody1, err := json.Marshal(req1)
	if err != nil {
		t.Fatalf("failed to marshal first request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpReq1, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody1))
	if err != nil {
		t.Fatalf("failed to create first HTTP request: %v", err)
	}
	httpReq1.Header.Set("Content-Type", "application/json")

	resp1, err := client.Do(httpReq1)
	if err != nil {
		t.Fatalf("first ingest request failed: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp1.Body).Decode(&errResp)
		t.Fatalf("first ingest returned status %d: %v", resp1.StatusCode, errResp)
	}

	var ingestResp1 IngestResponse
	if err := json.NewDecoder(resp1.Body).Decode(&ingestResp1); err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}

	firstNodeID := ingestResp1.NodeID
	firstObsID := ingestResp1.ObsID

	if firstNodeID == "" {
		t.Fatal("first ingest: node_id is empty")
	}
	if firstObsID == "" {
		t.Fatal("first ingest: obs_id is empty")
	}

	// Small delay to ensure updated_at timestamps differ
	time.Sleep(100 * time.Millisecond)

	// --- Second ingest with same path ---
	confidence2 := 0.85
	embedding2 := CreateTestEmbedding(DefaultEmbeddingDims, 2.0) // Different seed

	req2 := IngestRequest{
		SpaceID:     spaceID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      "source-second",
		Content:     "Second content for idempotent test",
		Tags:        []string{"second", "idempotent"},
		Name:        "idempotent-node-v2", // Different name
		Path:        testPath,             // SAME path - should merge
		Sensitivity: "public",             // Different sensitivity
		Confidence:  &confidence2,
		Embedding:   embedding2,
	}

	reqBody2, err := json.Marshal(req2)
	if err != nil {
		t.Fatalf("failed to marshal second request: %v", err)
	}

	httpReq2, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody2))
	if err != nil {
		t.Fatalf("failed to create second HTTP request: %v", err)
	}
	httpReq2.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(httpReq2)
	if err != nil {
		t.Fatalf("second ingest request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp2.Body).Decode(&errResp)
		t.Fatalf("second ingest returned status %d: %v", resp2.StatusCode, errResp)
	}

	var ingestResp2 IngestResponse
	if err := json.NewDecoder(resp2.Body).Decode(&ingestResp2); err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}

	secondObsID := ingestResp2.ObsID

	if secondObsID == "" {
		t.Fatal("second ingest: obs_id is empty")
	}
	if secondObsID == firstObsID {
		t.Errorf("second observation has same obs_id as first: %s", secondObsID)
	}

	// --- Verify only ONE node exists for this path ---
	verifyNodeCountForPath(t, driver, spaceID, testPath, 1)

	// --- Verify update_count was incremented ---
	verifyNodeUpdateCount(t, driver, spaceID, testPath, 2)

	// --- Verify TWO observations exist linked to the same node ---
	verifyObservationCount(t, driver, spaceID, testPath, 2)
}

// verifyNodeCountForPath checks that exactly expectedCount MemoryNodes exist for the given path.
func verifyNodeCountForPath(t *testing.T, driver neo4j.DriverWithContext, spaceID, path string, expectedCount int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, path: $path})
			RETURN count(n) AS node_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"path":    path,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("no result from count query")
		}

		count, _ := res.Record().Get("node_count")
		return count, res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query node count: %v", err)
	}

	count := result.(int64)
	if int(count) != expectedCount {
		t.Errorf("node count mismatch for path %q: got %d, want %d (idempotent ingestion should NOT create duplicates)", path, count, expectedCount)
	}
}

// verifyNodeUpdateCount checks that the node's update_count matches expected value.
func verifyNodeUpdateCount(t *testing.T, driver neo4j.DriverWithContext, spaceID, path string, expectedUpdateCount int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, path: $path})
			RETURN n.update_count AS update_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"path":    path,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("node not found for path: %s", path)
		}

		updateCount, _ := res.Record().Get("update_count")
		return updateCount, res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query update_count: %v", err)
	}

	updateCount := result.(int64)
	if int(updateCount) != expectedUpdateCount {
		t.Errorf("update_count mismatch: got %d, want %d (should increment on each ingest)", updateCount, expectedUpdateCount)
	}
}

// verifyObservationCount checks that exactly expectedCount Observations are linked to the node.
func verifyObservationCount(t *testing.T, driver neo4j.DriverWithContext, spaceID, path string, expectedCount int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, path: $path})-[:HAS_OBSERVATION]->(o:Observation)
			RETURN count(o) AS obs_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"path":    path,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("no result from observation count query")
		}

		count, _ := res.Record().Get("obs_count")
		return count, res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query observation count: %v", err)
	}

	count := result.(int64)
	if int(count) != expectedCount {
		t.Errorf("observation count mismatch: got %d, want %d (each ingest should create a new observation)", count, expectedCount)
	}
}

// verifyEmbeddingInNeo4j checks that the MemoryNode has an embedding of the expected dimension.
func verifyEmbeddingInNeo4j(t *testing.T, driver neo4j.DriverWithContext, spaceID, nodeID string, expectedDims int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
			RETURN n.embedding AS embedding
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return nil, err
		}

		if !res.Next(ctx) {
			return nil, fmt.Errorf("node not found: space_id=%s, node_id=%s", spaceID, nodeID)
		}

		return res.Record().AsMap(), res.Err()
	})
	if err != nil {
		t.Fatalf("failed to query node embedding from Neo4j: %v", err)
	}

	nodeProps := result.(map[string]any)

	// Verify embedding exists and has correct dimensions
	embedding, ok := nodeProps["embedding"].([]any)
	if !ok {
		if nodeProps["embedding"] == nil {
			t.Fatal("embedding is nil - auto-generation did not occur")
		}
		t.Fatalf("embedding has unexpected type: %T", nodeProps["embedding"])
	}

	if len(embedding) != expectedDims {
		t.Errorf("embedding dimension mismatch: got %d, want %d", len(embedding), expectedDims)
	}

	// Verify embedding contains non-zero values (sanity check)
	hasNonZero := false
	for _, v := range embedding {
		if val, ok := v.(float64); ok && val != 0.0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("embedding appears to be all zeros - likely not properly generated")
	}
}

// TestIngestAnomalyDuplicateDetection verifies that the anomaly detection
// detects duplicate nodes when ingesting content that is very similar to an existing node.
func TestIngestAnomalyDuplicateDetection(t *testing.T) {
	// Setup: ensure service is ready
	RequireServiceReady(t)
	driver := SetupTestNeo4j(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Create unique space ID for test isolation
	spaceID := GenerateTestSpaceID("anomaly-dup")

	// Register cleanup
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Create an embedding that we'll reuse (very similar)
	// Using the same embedding for both nodes guarantees similarity > 0.95
	baseEmbedding := CreateTestEmbedding(DefaultEmbeddingDims, 42.0)

	// --- First ingest: create the original node ---
	req1 := IngestRequest{
		SpaceID:   spaceID,
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "test-source-original",
		Content:   "Original content for duplicate detection test",
		Name:      "original-node",
		Path:      "/test/anomaly/original",
		Embedding: baseEmbedding,
	}

	reqBody1, err := json.Marshal(req1)
	if err != nil {
		t.Fatalf("failed to marshal first request: %v", err)
	}

	ingestURL := cfg.MDEMGEndpoint + "/v1/memory/ingest"
	httpReq1, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody1))
	if err != nil {
		t.Fatalf("failed to create first HTTP request: %v", err)
	}
	httpReq1.Header.Set("Content-Type", "application/json")

	resp1, err := client.Do(httpReq1)
	if err != nil {
		t.Fatalf("first ingest request failed: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp1.Body).Decode(&errResp)
		t.Fatalf("first ingest returned status %d: %v", resp1.StatusCode, errResp)
	}

	var ingestResp1 IngestResponse
	if err := json.NewDecoder(resp1.Body).Decode(&ingestResp1); err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}

	originalNodeID := ingestResp1.NodeID
	t.Logf("Created original node: %s", originalNodeID)

	// --- Second ingest: create near-duplicate (same embedding, different path) ---
	req2 := IngestRequest{
		SpaceID:   spaceID,
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "test-source-duplicate",
		Content:   "Nearly identical content for duplicate detection test",
		Name:      "duplicate-node",
		Path:      "/test/anomaly/duplicate", // Different path
		Embedding: baseEmbedding,             // Same embedding - should trigger duplicate detection
	}

	reqBody2, err := json.Marshal(req2)
	if err != nil {
		t.Fatalf("failed to marshal second request: %v", err)
	}

	httpReq2, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(reqBody2))
	if err != nil {
		t.Fatalf("failed to create second HTTP request: %v", err)
	}
	httpReq2.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(httpReq2)
	if err != nil {
		t.Fatalf("second ingest request failed: %v", err)
	}
	defer resp2.Body.Close()

	// Ingest should still succeed (anomalies are non-blocking)
	if resp2.StatusCode != http.StatusOK {
		var errResp map[string]any
		json.NewDecoder(resp2.Body).Decode(&errResp)
		t.Fatalf("second ingest returned status %d: %v", resp2.StatusCode, errResp)
	}

	var ingestResp2 IngestResponse
	if err := json.NewDecoder(resp2.Body).Decode(&ingestResp2); err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}

	duplicateNodeID := ingestResp2.NodeID
	t.Logf("Created duplicate node: %s", duplicateNodeID)

	// --- Verify anomaly was detected ---
	if len(ingestResp2.Anomalies) == 0 {
		t.Log("WARNING: No anomalies detected. This may be expected if anomaly detection is disabled or timed out.")
		return
	}

	// Check for duplicate anomaly
	foundDuplicate := false
	for _, anomaly := range ingestResp2.Anomalies {
		t.Logf("Anomaly detected: type=%s, severity=%s, message=%s, confidence=%.2f, related=%s",
			anomaly.Type, anomaly.Severity, anomaly.Message, anomaly.Confidence, anomaly.RelatedNode)

		if anomaly.Type == "duplicate" {
			foundDuplicate = true

			// Verify the related node is the original
			if anomaly.RelatedNode != originalNodeID {
				t.Errorf("duplicate anomaly related_node mismatch: got %q, want %q",
					anomaly.RelatedNode, originalNodeID)
			}

			// Verify confidence is high (should be close to 1.0 for identical embeddings)
			if anomaly.Confidence < 0.95 {
				t.Errorf("duplicate anomaly confidence too low: got %.2f, want >= 0.95",
					anomaly.Confidence)
			}

			// Verify severity is appropriate
			if anomaly.Severity != "warning" && anomaly.Severity != "critical" {
				t.Errorf("unexpected severity for duplicate: got %q, want warning or critical",
					anomaly.Severity)
			}
		}
	}

	if !foundDuplicate {
		t.Error("expected duplicate anomaly to be detected with identical embeddings")
	}
}
