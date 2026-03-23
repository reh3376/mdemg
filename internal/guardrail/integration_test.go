//go:build integration

package guardrail

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/embeddings"
)

// ─── Local helpers ───

// setupTestNeo4j creates a Neo4j driver for integration tests and registers cleanup.
func setupTestNeo4j(t *testing.T) neo4j.DriverWithContext {
	t.Helper()

	driver, err := neo4j.NewDriverWithContext(
		"bolt://localhost:7687",
		neo4j.BasicAuth("neo4j", "testpassword", ""),
	)
	if err != nil {
		t.Fatalf("failed to create Neo4j driver: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx) //nolint:errcheck
		t.Fatalf("failed to verify Neo4j connectivity: %v", err)
	}

	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		driver.Close(closeCtx) //nolint:errcheck
	})

	return driver
}

// generateTestSpaceID creates a unique space_id for test isolation.
func generateTestSpaceID(suffix string) string {
	timestamp := time.Now().UnixNano()
	if suffix == "" {
		return fmt.Sprintf("test-%d", timestamp)
	}
	return fmt.Sprintf("test-%d-%s", timestamp, suffix)
}

// cleanupSpace deletes all MemoryNodes for a given space_id.
func cleanupSpace(t *testing.T, driver neo4j.DriverWithContext, spaceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx) //nolint:errcheck

	queries := []string{
		`MATCH (o:Observation {space_id: $spaceId}) DETACH DELETE o`,
		`MATCH (n:MemoryNode {space_id: $spaceId}) DETACH DELETE n`,
		`MATCH (t:TapRoot {space_id: $spaceId}) DETACH DELETE t`,
	}

	for _, q := range queries {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, q, map[string]any{"spaceId": spaceID})
			return nil, err
		})
		if err != nil {
			t.Errorf("cleanup query failed for space %s: %v", spaceID, err)
		}
	}
}

// seedConstraintNode creates a MemoryNode with role_type "constraint" in Neo4j.
// It returns the node_id that was created.
func seedConstraintNode(
	t *testing.T,
	driver neo4j.DriverWithContext,
	spaceID, content, constraintType string,
	keywords []string,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeID := fmt.Sprintf("test-constraint-%d", time.Now().UnixNano())

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx) //nolint:errcheck

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			CREATE (c:MemoryNode {
				node_id: $nodeId,
				space_id: $spaceId,
				role_type: 'constraint',
				constraint_type: $constraintType,
				content: $content,
				keywords: $keywords,
				confidence: 0.9,
				is_archived: false,
				layer: 0,
				created_at: datetime(),
				updated_at: datetime()
			}) RETURN c.node_id`
		_, err := tx.Run(ctx, cypher, map[string]any{
			"nodeId":         nodeID,
			"spaceId":        spaceID,
			"constraintType": constraintType,
			"content":        content,
			"keywords":       keywords,
		})
		return nil, err
	})
	if err != nil {
		t.Fatalf("failed to seed constraint node: %v", err)
	}

	return nodeID
}

// newTestService constructs a GuardrailService wired for integration tests.
// It uses the stub embedder (no external deps) and a disabled circuit breaker registry.
func newTestService(driver neo4j.DriverWithContext) *GuardrailService {
	cfg := GuardrailConfig{
		Enabled:                         true,
		MaxConstraints:                  10,
		VectorIndexName:                 "memNodeEmbedding",
		ConstraintScopeFilteringEnabled: false,
		ConstraintAuthorityEnabled:      false,
	}
	stubEmb := embeddings.NewStub()
	cbReg := circuitbreaker.NewRegistry(circuitbreaker.Config{Enabled: false})
	return NewGuardrailService(cfg, driver, stubEmb, cbReg)
}

// ─── Integration Tests ───

// TestGuardrail_KeywordSearch verifies that a seeded constraint node is found
// when its content contains a keyword passed to keywordSearch.
func TestGuardrail_KeywordSearch(t *testing.T) {
	driver := setupTestNeo4j(t)
	spaceID := generateTestSpaceID("kw")
	t.Cleanup(func() { cleanupSpace(t, driver, spaceID) })

	seedConstraintNode(t, driver, spaceID,
		"never use raw SQL queries in application code",
		"must_not",
		[]string{"sql", "security"},
	)

	svc := newTestService(driver)
	ctx := context.Background()

	matches, err := svc.keywordSearch(ctx, spaceID, []string{"sql"}, DiffContext{}, "standard")
	if err != nil {
		t.Fatalf("keywordSearch returned error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one constraint match, got 0")
	}

	found := false
	for _, m := range matches {
		if contains(m.Content, "sql") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected match with SQL content, got: %+v", matches)
	}
}

// TestGuardrail_KeywordSearch_Empty verifies that an empty keyword list
// returns no error and an empty (nil) result.
func TestGuardrail_KeywordSearch_Empty(t *testing.T) {
	driver := setupTestNeo4j(t)
	spaceID := generateTestSpaceID("kw-empty")
	t.Cleanup(func() { cleanupSpace(t, driver, spaceID) })

	svc := newTestService(driver)
	ctx := context.Background()

	matches, err := svc.keywordSearch(ctx, spaceID, []string{}, DiffContext{}, "standard")
	if err != nil {
		t.Fatalf("keywordSearch with empty keywords returned error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected no matches for empty keywords, got %d", len(matches))
	}
}

// TestGuardrail_SemanticSearch_NoError verifies that semanticSearch completes
// without error when the stub embedder is used. The seeded node has no embedding
// vector so cosine similarity will not exceed 0.3 — an empty result is acceptable.
func TestGuardrail_SemanticSearch_NoError(t *testing.T) {
	driver := setupTestNeo4j(t)
	spaceID := generateTestSpaceID("sem")
	t.Cleanup(func() { cleanupSpace(t, driver, spaceID) })

	seedConstraintNode(t, driver, spaceID,
		"always validate user input before processing",
		"must",
		[]string{"validation", "security"},
	)

	svc := newTestService(driver)
	ctx := context.Background()

	diffCtx := DiffContext{Summary: "validate user input on API endpoints"}

	// Should not error; empty result is acceptable (no stored embeddings to match)
	_, err := svc.semanticSearch(ctx, spaceID, diffCtx.Summary, diffCtx, "standard")
	if err != nil {
		t.Fatalf("semanticSearch returned unexpected error: %v", err)
	}
}

// TestGuardrail_RetrieveConstraints seeds two constraints with distinct keywords
// and confirms that retrieveConstraints returns at least one result when the
// DiffContext carries matching keywords.
func TestGuardrail_RetrieveConstraints(t *testing.T) {
	driver := setupTestNeo4j(t)
	spaceID := generateTestSpaceID("ret")
	t.Cleanup(func() { cleanupSpace(t, driver, spaceID) })

	seedConstraintNode(t, driver, spaceID,
		"never expose database credentials in logs",
		"must_not",
		[]string{"credentials", "logs", "security"},
	)
	seedConstraintNode(t, driver, spaceID,
		"always use parameterized queries to prevent injection",
		"must",
		[]string{"queries", "injection", "security"},
	)

	svc := newTestService(driver)
	ctx := context.Background()

	// DiffContext carries a keyword that should match the first constraint
	diffCtx := DiffContext{
		Keywords: []string{"credentials"},
	}

	results, err := svc.retrieveConstraints(ctx, spaceID, diffCtx, "standard")
	if err != nil {
		t.Fatalf("retrieveConstraints returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one constraint result, got 0")
	}
}

// TestGuardrail_Validate_Enabled_NoConstraints verifies the fail-open / pass-through
// behaviour: when guardrails are enabled but no constraints exist in the space,
// Validate returns "Pass" without invoking the LLM.
func TestGuardrail_Validate_Enabled_NoConstraints(t *testing.T) {
	driver := setupTestNeo4j(t)
	spaceID := generateTestSpaceID("val-empty")
	t.Cleanup(func() { cleanupSpace(t, driver, spaceID) })

	svc := newTestService(driver)
	ctx := context.Background()

	req := ValidateRequest{
		SpaceID:         spaceID,
		FilesChanged:    []string{"main.go"},
		Diff:            "+func doSomething() {}",
		AgentTrustLevel: "standard",
	}

	resp, err := svc.Validate(ctx, req)
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Validate returned nil response")
	}
	if resp.Status != "Pass" {
		t.Errorf("expected status Pass (no constraints in space), got %q", resp.Status)
	}
}

// ─── Utility ───

// contains is a simple case-insensitive substring check used in assertions.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexInsensitive(s, sub) >= 0)
}

// indexInsensitive returns the index of sub in s (case-insensitive), or -1.
func indexInsensitive(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	subLower := toLower(sub)
	sLower := toLower(s)
	for i := 0; i <= len(sLower)-len(subLower); i++ {
		if sLower[i:i+len(subLower)] == subLower {
			return i
		}
	}
	return -1
}

// toLower is a simple ASCII lowercase helper to avoid importing strings in the test.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
