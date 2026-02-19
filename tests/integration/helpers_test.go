//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// TestConfig holds configuration for integration tests.
type TestConfig struct {
	Neo4jURI      string
	Neo4jUser     string
	Neo4jPass     string
	MDEMGEndpoint string
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultVal string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	return v
}

// GetTestConfig loads test configuration from environment variables with sensible defaults.
func GetTestConfig() TestConfig {
	// For MDEMG endpoint: explicit test env var takes priority, then dynamic resolution
	endpoint := getEnv("TEST_MDEMG_ENDPOINT", "")
	if endpoint == "" {
		endpoint = config.ResolveEndpoint("http://localhost:9999")
	}

	return TestConfig{
		Neo4jURI:      getEnv("TEST_NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:     getEnv("TEST_NEO4J_USER", "neo4j"),
		Neo4jPass:     getEnv("TEST_NEO4J_PASS", "testpassword"),
		MDEMGEndpoint: endpoint,
	}
}

// SetupTestNeo4j creates a Neo4j driver for integration tests.
// It returns the driver and registers cleanup with t.Cleanup().
// The function will fail the test if connection cannot be established.
func SetupTestNeo4j(t *testing.T) neo4j.DriverWithContext {
	t.Helper()

	cfg := GetTestConfig()

	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""),
	)
	if err != nil {
		t.Fatalf("failed to create Neo4j driver: %v", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		t.Fatalf("failed to verify Neo4j connectivity: %v", err)
	}

	// Register cleanup to close driver
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		driver.Close(ctx)
	})

	return driver
}

// CleanupSpace deletes all MemoryNodes, Observations, and TapRoot nodes for a given space_id.
// This ensures test isolation by removing all test data.
func CleanupSpace(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Delete all nodes and relationships for this space
	// Order matters: delete observations first, then memory nodes, then taproot
	queries := []string{
		// Delete all Observations for this space
		`MATCH (o:Observation {space_id: $spaceId}) DETACH DELETE o`,
		// Delete all MemoryNodes for this space
		`MATCH (n:MemoryNode {space_id: $spaceId}) DETACH DELETE n`,
		// Delete TapRoot for this space
		`MATCH (t:TapRoot {space_id: $spaceId}) DETACH DELETE t`,
	}

	for _, query := range queries {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, query, map[string]any{"spaceId": spaceID})
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("cleanup query failed: %w", err)
		}
	}

	return nil
}

// CleanupSpaceWithTest is a helper that calls CleanupSpace and fails the test on error.
// Use this in t.Cleanup() registrations.
func CleanupSpaceWithTest(t *testing.T, driver neo4j.DriverWithContext, spaceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := CleanupSpace(ctx, driver, spaceID); err != nil {
		t.Errorf("failed to cleanup space %s: %v", spaceID, err)
	}
}

// GenerateTestSpaceID creates a unique space_id for test isolation.
// Format: test-<timestamp>-<suffix>
// All test space IDs start with "test-" to enable easy cleanup queries.
func GenerateTestSpaceID(suffix string) string {
	timestamp := time.Now().UnixNano()
	if suffix == "" {
		return fmt.Sprintf("test-%d", timestamp)
	}
	return fmt.Sprintf("test-%d-%s", timestamp, suffix)
}

// NewTestHTTPClient creates an HTTP client configured for integration tests.
// The client has reasonable timeouts for test scenarios.
func NewTestHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true,
			DisableKeepAlives:   false,
			MaxConnsPerHost:     10,
			MaxIdleConnsPerHost: 10,
		},
	}
}

// RequireServiceReady checks if the MDEMG service is ready before running tests.
// It calls the /healthz endpoint and fails the test if the service is not available.
func RequireServiceReady(t *testing.T) {
	t.Helper()

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Try /healthz endpoint
	healthURL := cfg.MDEMGEndpoint + "/healthz"
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("MDEMG service not available at %s: %v", cfg.MDEMGEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MDEMG service health check failed: status %d", resp.StatusCode)
	}
}

// RequireEmbeddingProvider checks if an embedding provider is configured.
// It calls the /readyz endpoint and checks for the embedding_provider field.
// Returns true if embedding provider is available, false otherwise.
// Does not fail the test - caller should use t.Skip() if needed.
func RequireEmbeddingProvider(t *testing.T) bool {
	t.Helper()

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	// Try /readyz endpoint which includes embedding provider status
	readyURL := cfg.MDEMGEndpoint + "/readyz"
	resp, err := client.Get(readyURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Decode response and check for embedding_provider field
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	// Check if embedding_provider field exists and is non-empty
	provider, ok := result["embedding_provider"].(string)
	return ok && provider != ""
}

// CreateTestEmbedding generates a test embedding of the specified dimension.
// The embedding is a simple normalized vector useful for testing.
// For deterministic tests, use the same seed values.
func CreateTestEmbedding(dims int, seed float32) []float32 {
	embedding := make([]float32, dims)
	for i := range embedding {
		// Create a pattern that varies by position and seed
		embedding[i] = (seed + float32(i%10)*0.1) / float32(dims)
	}
	// Normalize to unit vector (approximate)
	var sum float32
	for _, v := range embedding {
		sum += v * v
	}
	if sum > 0 {
		norm := float32(1.0) / float32(sum)
		for i := range embedding {
			embedding[i] *= norm
		}
	}
	return embedding
}

// DefaultEmbeddingDims is the default embedding dimension for tests.
// Using 1536 to match the Neo4j vector index (OpenAI ada-002 dimension).
const DefaultEmbeddingDims = 1536

// CreateQueryEmbedding creates a standard query embedding [1, 0, 0, ...].
// This is used with CreateControlledEmbedding to achieve specific cosine similarities.
func CreateQueryEmbedding(dims int) []float32 {
	embedding := make([]float32, dims)
	embedding[0] = 1.0
	return embedding
}

// CreateControlledEmbedding creates an embedding that will have a specific cosine similarity
// with the query embedding created by CreateQueryEmbedding.
//
// The math: For unit vectors Q and N, cosine_similarity(Q, N) = dot(Q, N).
// With Q = [1, 0, 0, ...], we create N = [similarity, sqrt(1-sim^2), 0, ...].
//
// Parameters:
//   - dims: embedding dimensions (768 for default)
//   - similarity: target cosine similarity (0.0 to 1.0)
//   - perpIndex: which dimension to use for the perpendicular component (1 to dims-1)
//
// Different perpIndex values create embeddings that are orthogonal to each other
// in the perpendicular component, while maintaining the same similarity to the query.
func CreateControlledEmbedding(dims int, similarity float64, perpIndex int) []float32 {
	if similarity < 0 {
		similarity = 0
	}
	if similarity > 1 {
		similarity = 1
	}
	if perpIndex < 1 || perpIndex >= dims {
		perpIndex = 1
	}

	embedding := make([]float32, dims)

	// Parallel component (determines cosine similarity with query)
	embedding[0] = float32(similarity)

	// Perpendicular component (makes it a unit vector)
	perpComponent := 1.0 - similarity*similarity
	if perpComponent > 0 {
		embedding[perpIndex] = float32(math.Sqrt(perpComponent))
	}

	return embedding
}

// ─── RSIC Helpers ───

// ObserveTestNode creates a minimal node in the test space via /v1/memory/ingest.
// This uses the ingest endpoint (not conversation/observe) so it works without
// an embedding provider — critical for CI where no embedder is available.
func ObserveTestNode(t *testing.T, endpoint, spaceID, content string) {
	t.Helper()

	body := map[string]any{
		"space_id":  spaceID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"source":    "integration-test",
		"name":      content,
		"content":   content,
		"path":      fmt.Sprintf("/test/rsic/%d", time.Now().UnixNano()),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal ingest request: %v", err)
	}

	client := NewTestHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/memory/ingest", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create ingest request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("ingest returned status %d: %v", resp.StatusCode, errBody)
	}
}

// TriggerRSICCycle triggers an RSIC cycle and returns the parsed response body.
func TriggerRSICCycle(t *testing.T, endpoint, spaceID string, opts map[string]any) map[string]any {
	t.Helper()

	body := map[string]any{"space_id": spaceID, "tier": "micro"}
	for k, v := range opts {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal cycle request: %v", err)
	}

	client := NewTestHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/self-improve/cycle", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create cycle request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("cycle request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("cycle returned status %d: %v", resp.StatusCode, errBody)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode cycle response: %v", err)
	}
	return result
}

// GetRSICHealth fetches the RSIC health endpoint and returns parsed response.
func GetRSICHealth(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	client := NewTestHTTPClient()
	resp, err := client.Get(endpoint + "/v1/self-improve/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	return result
}

// TriggerRSICCycleRaw triggers an RSIC cycle and returns (statusCode, body) without
// fataling on non-200. Used by rejection/dedupe tests that expect 409 or alternate 200.
func TriggerRSICCycleRaw(t *testing.T, endpoint, spaceID string, opts map[string]any) (int, map[string]any) {
	t.Helper()

	body := map[string]any{"space_id": spaceID, "tier": "micro"}
	for k, v := range opts {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal cycle request: %v", err)
	}

	client := NewTestHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/self-improve/cycle", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create cycle request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("cycle request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode cycle response (status %d): %v", resp.StatusCode, err)
	}
	return resp.StatusCode, result
}

// GetRSICCalibration fetches the RSIC calibration endpoint and returns parsed response.
func GetRSICCalibration(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	client := NewTestHTTPClient()
	resp, err := client.Get(endpoint + "/v1/self-improve/calibration")
	if err != nil {
		t.Fatalf("calibration request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("calibration returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode calibration response: %v", err)
	}
	return result
}

// GetRSICHistoryFiltered fetches RSIC history with filter query parameters.
func GetRSICHistoryFiltered(t *testing.T, endpoint string, limit int, filters map[string]string) map[string]any {
	t.Helper()

	url := fmt.Sprintf("%s/v1/self-improve/history?limit=%d", endpoint, limit)
	for k, v := range filters {
		url += fmt.Sprintf("&%s=%s", k, v)
	}

	client := NewTestHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("filtered history request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("filtered history returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode filtered history response: %v", err)
	}
	return result
}

// GetRSICRollbackList fetches the rollback snapshot list (GET /v1/self-improve/rollback).
func GetRSICRollbackList(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	client := NewTestHTTPClient()
	resp, err := client.Get(endpoint + "/v1/self-improve/rollback")
	if err != nil {
		t.Fatalf("rollback list request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rollback list returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode rollback list response: %v", err)
	}
	return result
}

// PostRSICRollback attempts a rollback by snapshot_id and returns (statusCode, body).
func PostRSICRollback(t *testing.T, endpoint, snapshotID string) (int, map[string]any) {
	t.Helper()

	body := map[string]any{"snapshot_id": snapshotID}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal rollback request: %v", err)
	}

	client := NewTestHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/self-improve/rollback", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create rollback request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("rollback request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode rollback response (status %d): %v", resp.StatusCode, err)
	}
	return resp.StatusCode, result
}

// GetRSICSignals fetches the signals endpoint (GET /v1/self-improve/signals).
func GetRSICSignals(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	client := NewTestHTTPClient()
	resp, err := client.Get(endpoint + "/v1/self-improve/signals")
	if err != nil {
		t.Fatalf("signals request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signals returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode signals response: %v", err)
	}
	return result
}

// GetRSICHistory fetches RSIC cycle history and returns parsed response.
func GetRSICHistory(t *testing.T, endpoint string, limit int) map[string]any {
	t.Helper()

	url := fmt.Sprintf("%s/v1/self-improve/history?limit=%d", endpoint, limit)
	client := NewTestHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("history request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode history response: %v", err)
	}
	return result
}
