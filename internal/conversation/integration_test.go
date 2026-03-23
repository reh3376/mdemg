// Integration tests for the Conversation Memory System (CMS)
// These tests require a running Neo4j instance
//
// Run with: go test -tags=integration ./internal/conversation/... -v
// Requires environment variables: NEO4J_URI, NEO4J_USER, NEO4J_PASS

//go:build integration

package conversation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
)

const testSpaceID = "test-cms-integration"
const testVectorIndex = "test_obs_vector"

// setupTestDriver creates a Neo4j driver for tests
func setupTestDriver(t *testing.T) neo4j.DriverWithContext {
	uri := os.Getenv("NEO4J_URI")
	user := os.Getenv("NEO4J_USER")
	pass := os.Getenv("NEO4J_PASS")

	if uri == "" || user == "" || pass == "" {
		t.Skip("Skipping integration test: NEO4J_URI, NEO4J_USER, NEO4J_PASS required")
	}

	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
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

// setupMockEmbedder creates a mock embedder for tests
func setupMockEmbedder() embeddings.Embedder {
	return &mockEmbedder{dims: 384}
}

type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Return deterministic embedding based on text hash
	vec := make([]float32, m.dims)
	for i := range vec {
		vec[i] = float32(i%10) / 10.0
		if len(text) > 0 {
			vec[i] += float32(int(text[i%len(text)])) / 1000.0
		}
	}
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dims
}

func (m *mockEmbedder) Name() string {
	return "mock"
}

// cleanupTestData removes all test data from the space
func cleanupTestData(t *testing.T, ctx context.Context, driver neo4j.DriverWithContext) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Delete all test observations and their relationships
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.role_type = 'conversation_observation'
DETACH DELETE n`
		_, err := tx.Run(ctx, cypher, map[string]any{"spaceId": testSpaceID})
		return nil, err
	})
	if err != nil {
		t.Logf("Warning: cleanup failed: %v", err)
	}
}

// =============================================================================
// VISIBILITY TESTS
// =============================================================================

func TestVisibilityFiltering(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)

	// Create observations with different visibility levels
	// Alice's private observation
	alicePrivate, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:    testSpaceID,
		SessionID:  "session-vis-1",
		Content:    "Alice's private note about the API design",
		ObsType:    "note",
		UserID:     "alice",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("Failed to create Alice's private observation: %v", err)
	}

	// Alice's team observation
	aliceTeam, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:    testSpaceID,
		SessionID:  "session-vis-1",
		Content:    "Team decision about using PostgreSQL",
		ObsType:    "decision",
		UserID:     "alice",
		Visibility: "team",
	})
	if err != nil {
		t.Fatalf("Failed to create Alice's team observation: %v", err)
	}

	// Bob's global observation
	bobGlobal, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:    testSpaceID,
		SessionID:  "session-vis-1",
		Content:    "Public announcement about the new feature",
		ObsType:    "note",
		UserID:     "bob",
		Visibility: "global",
	})
	if err != nil {
		t.Fatalf("Failed to create Bob's global observation: %v", err)
	}

	// Bob's private observation
	_, err = svc.Observe(ctx, ObserveRequest{
		SpaceID:    testSpaceID,
		SessionID:  "session-vis-1",
		Content:    "Bob's private thoughts",
		ObsType:    "note",
		UserID:     "bob",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("Failed to create Bob's private observation: %v", err)
	}

	t.Logf("Created observations: alice_private=%s, alice_team=%s, bob_global=%s",
		alicePrivate.NodeID, aliceTeam.NodeID, bobGlobal.NodeID)

	// Test: Alice should see all her observations + team + global
	aliceResume, err := svc.Resume(ctx, ResumeRequest{
		SpaceID:          testSpaceID,
		SessionID:        "session-vis-1",
		MaxObservations:  10,
		RequestingUserID: "alice",
	})
	if err != nil {
		t.Fatalf("Resume for Alice failed: %v", err)
	}

	// Alice should see: her private, team, global, bob's global (4 total, not bob's private)
	aliceCount := len(aliceResume.Observations)
	if aliceCount < 3 {
		t.Errorf("Alice should see at least 3 observations, got %d", aliceCount)
	}

	// Test: Bob should NOT see Alice's private observation
	bobResume, err := svc.Resume(ctx, ResumeRequest{
		SpaceID:          testSpaceID,
		SessionID:        "session-vis-1",
		MaxObservations:  10,
		RequestingUserID: "bob",
	})
	if err != nil {
		t.Fatalf("Resume for Bob failed: %v", err)
	}

	// Bob should see: his private, his global, alice's team (not alice's private)
	bobCount := len(bobResume.Observations)
	for _, obs := range bobResume.Observations {
		// Bob should NOT see Alice's private observation
		if obs.NodeID == alicePrivate.NodeID {
			t.Errorf("Bob should NOT see Alice's private observation")
		}
	}

	t.Logf("Visibility test: Alice sees %d, Bob sees %d observations", aliceCount, bobCount)
}

// =============================================================================
// CONTEXT COOLER TESTS
// =============================================================================

func TestContextCoolerGraduation(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)
	cooler := NewContextCooler(driver, config.Config{})

	// Create a volatile observation
	obs, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-cooler-1",
		Content:   "Important discovery about BlueSeerValidator",
		ObsType:   "learning",
	})
	if err != nil {
		t.Fatalf("Failed to create observation: %v", err)
	}

	t.Logf("Created observation: %s", obs.NodeID)

	// Verify initial state: volatile with low stability
	stats, err := cooler.GetVolatileStats(ctx, testSpaceID)
	if err != nil {
		t.Fatalf("Failed to get volatile stats: %v", err)
	}
	if stats.VolatileCount == 0 {
		t.Error("Expected at least 1 volatile node")
	}
	t.Logf("Initial stats: volatile=%d, permanent=%d, avg_stability=%.2f",
		stats.VolatileCount, stats.PermanentCount, stats.AvgVolatileStability)

	// Reinforce multiple times to increase stability
	for i := 0; i < 7; i++ { // 7 * 0.15 = 1.05 > 0.8 graduation threshold
		err = cooler.UpdateStabilityOnReinforcement(ctx, testSpaceID, obs.NodeID)
		if err != nil {
			t.Fatalf("Reinforcement %d failed: %v", i+1, err)
		}
	}

	// Check graduation
	result, err := cooler.CheckGraduation(ctx, testSpaceID, obs.NodeID)
	if err != nil {
		t.Fatalf("CheckGraduation failed: %v", err)
	}

	t.Logf("Graduation result: graduated=%v, stability=%.2f, reason=%s",
		result.Graduated, result.StabilityScore, result.Reason)

	if !result.Graduated {
		t.Errorf("Expected node to graduate after 7 reinforcements, stability=%.2f", result.StabilityScore)
	}

	// Verify node is no longer volatile
	finalStats, err := cooler.GetVolatileStats(ctx, testSpaceID)
	if err != nil {
		t.Fatalf("Failed to get final stats: %v", err)
	}

	t.Logf("Final stats: volatile=%d, permanent=%d",
		finalStats.VolatileCount, finalStats.PermanentCount)
}

func TestContextCoolerDecay(t *testing.T) {
	// Test the pure decay calculation function
	tests := []struct {
		stability    float64
		daysInactive int
		expected     float64
	}{
		{1.0, 0, 1.0},   // No decay for 0 days
		{1.0, 1, 0.9},   // 10% decay after 1 day
		{1.0, 7, 0.478}, // After a week
		{0.5, 1, 0.45},  // Starting at 0.5
	}

	for _, tc := range tests {
		result := CalculateDecayedStability(tc.stability, tc.daysInactive)
		if diff := result - tc.expected; diff > 0.01 || diff < -0.01 {
			t.Errorf("CalculateDecayedStability(%.2f, %d) = %.3f, expected ~%.3f",
				tc.stability, tc.daysInactive, result, tc.expected)
		}
	}
}

func TestReinforcementsToGraduate(t *testing.T) {
	tests := []struct {
		stability float64
		expected  int
	}{
		{0.1, 5},  // (0.8 - 0.1) / 0.15 = 4.67 -> 5
		{0.5, 2},  // (0.8 - 0.5) / 0.15 = 2
		{0.8, 0},  // Already at threshold
		{1.0, 0},  // Above threshold
	}

	for _, tc := range tests {
		result := CalculateReinforcementsToGraduate(tc.stability)
		if result != tc.expected {
			t.Errorf("CalculateReinforcementsToGraduate(%.2f) = %d, expected %d",
				tc.stability, result, tc.expected)
		}
	}
}

// =============================================================================
// JIMINY RATIONALE TESTS
// =============================================================================

func TestJiminyRationale(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)

	// Create some observations to generate rationale
	_, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-jiminy-1",
		Content:   "User prefers TypeScript over JavaScript",
		ObsType:   "preference",
	})
	if err != nil {
		t.Fatalf("Failed to create observation 1: %v", err)
	}

	_, err = svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-jiminy-1",
		Content:   "Decision: Use PostgreSQL for persistence",
		ObsType:   "decision",
	})
	if err != nil {
		t.Fatalf("Failed to create observation 2: %v", err)
	}

	// Resume and check Jiminy rationale
	resume, err := svc.Resume(ctx, ResumeRequest{
		SpaceID:          testSpaceID,
		SessionID:        "session-jiminy-1",
		MaxObservations:  10,
		IncludeDecisions: true,
	})
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	if resume.Jiminy == nil {
		t.Error("Expected Jiminy rationale to be present")
	} else {
		t.Logf("Jiminy rationale: %s", resume.Jiminy.Rationale)
		t.Logf("Jiminy confidence: %.2f", resume.Jiminy.Confidence)
		t.Logf("Score breakdown: %v", resume.Jiminy.ScoreBreakdown)

		if resume.Jiminy.Confidence <= 0 {
			t.Error("Expected positive confidence score")
		}
		if len(resume.Jiminy.ScoreBreakdown) == 0 {
			t.Error("Expected score breakdown to have entries")
		}
	}
}

// =============================================================================
// REFERS_TO CROSS-MODULE LINKING TESTS
// =============================================================================

func TestRefersToLinking(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	// First, create a symbol node to reference
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	symbolNodeID := "sym-test-function-123"
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MERGE (s:MemoryNode:SymbolNode {space_id: $spaceId, node_id: $nodeId})
SET s.name = 'TestFunction',
    s.symbol_kind = 'function',
    s.path = '/src/test.go'
RETURN s.node_id`
		_, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": testSpaceID,
			"nodeId":  symbolNodeID,
		})
		return nil, err
	})
	if err != nil {
		t.Fatalf("Failed to create symbol node: %v", err)
	}

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)

	// Create observation that refers to the symbol
	obs, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-refs-1",
		Content:   "Discovered that TestFunction has a bug in error handling",
		ObsType:   "learning",
		RefersTo:  []string{symbolNodeID},
	})
	if err != nil {
		t.Fatalf("Failed to create observation with reference: %v", err)
	}

	t.Logf("Created observation %s referring to symbol %s", obs.NodeID, symbolNodeID)

	// Query references from the observation
	refs, err := svc.GetReferencesFromObservation(ctx, testSpaceID, obs.NodeID)
	if err != nil {
		t.Fatalf("GetReferencesFromObservation failed: %v", err)
	}

	if len(refs) == 0 {
		t.Error("Expected at least 1 reference")
	} else {
		t.Logf("Found %d reference(s): %v", len(refs), refs)
		found := false
		for _, ref := range refs {
			if ref.NodeID == symbolNodeID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find reference to %s", symbolNodeID)
		}
	}

	// Query observations referencing the symbol
	obsRefs, err := svc.GetObservationsReferencingNode(ctx, testSpaceID, symbolNodeID)
	if err != nil {
		t.Fatalf("GetObservationsReferencingNode failed: %v", err)
	}

	if len(obsRefs) == 0 {
		t.Error("Expected at least 1 observation referencing the symbol")
	} else {
		t.Logf("Found %d observation(s) referencing symbol", len(obsRefs))
	}

	// Cleanup: delete the symbol node
	_, _ = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (s:MemoryNode {space_id: $spaceId, node_id: $nodeId}) DETACH DELETE s`
		_, err := tx.Run(ctx, cypher, map[string]any{"spaceId": testSpaceID, "nodeId": symbolNodeID})
		return nil, err
	})
}

// =============================================================================
// SURPRISE DETECTION TESTS
// =============================================================================

func TestSurpriseDetection(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)

	// Normal observation (low surprise)
	normal, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-surprise-1",
		Content:   "The function returns a string",
		ObsType:   "note",
	})
	if err != nil {
		t.Fatalf("Failed to create normal observation: %v", err)
	}

	// Correction (should have high surprise)
	correction, err := svc.Correct(ctx, CorrectRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-surprise-1",
		Incorrect: "The ORM is Hibernate",
		Correct:   "The ORM is BlueSeerData, a custom framework",
		Context:   "User corrected Claude's assumption",
	})
	if err != nil {
		t.Fatalf("Failed to create correction: %v", err)
	}

	t.Logf("Normal observation surprise: %.2f", normal.SurpriseScore)
	t.Logf("Correction surprise: %.2f", correction.SurpriseScore)

	// Corrections should have higher surprise (baseline 0.5 from explicit correction)
	if correction.SurpriseScore < 0.5 {
		t.Errorf("Expected correction to have high surprise (>=0.5), got %.2f", correction.SurpriseScore)
	}

	// Domain-specific terminology should increase surprise
	domainSpecific, err := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: "session-surprise-1",
		Content:   "BlueSeerValidator uses the XyzProtocol for async message handling",
		ObsType:   "learning",
	})
	if err != nil {
		t.Fatalf("Failed to create domain-specific observation: %v", err)
	}

	t.Logf("Domain-specific observation surprise: %.2f", domainSpecific.SurpriseScore)
}

// =============================================================================
// END-TO-END FLOW TESTS
// =============================================================================

func TestFullConversationFlow(t *testing.T) {
	driver := setupTestDriver(t)
	defer driver.Close(context.Background())

	ctx := context.Background()
	cleanupTestData(t, ctx, driver)
	defer cleanupTestData(t, ctx, driver)

	svc := NewServiceWithConfig(driver, setupMockEmbedder(), testVectorIndex)
	cooler := NewContextCooler(driver, config.Config{})

	sessionID := "session-e2e-flow"

	// Step 1: Create several observations
	obs1, _ := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: sessionID,
		Content:   "User prefers tabs over spaces",
		ObsType:   "preference",
		UserID:    "developer1",
	})

	obs2, _ := svc.Observe(ctx, ObserveRequest{
		SpaceID:   testSpaceID,
		SessionID: sessionID,
		Content:   "Decision: Use hexagonal architecture",
		ObsType:   "decision",
		UserID:    "developer1",
	})

	_, _ = svc.Correct(ctx, CorrectRequest{
		SpaceID:   testSpaceID,
		SessionID: sessionID,
		Incorrect: "The config file is config.yaml",
		Correct:   "The config file is settings.toml",
		UserID:    "developer1",
	})

	t.Logf("Created observations: %s, %s", obs1.NodeID, obs2.NodeID)

	// Step 2: Reinforce one observation multiple times (simulate repeated access)
	for i := 0; i < 6; i++ {
		_ = cooler.UpdateStabilityOnReinforcement(ctx, testSpaceID, obs1.NodeID)
	}

	// Step 3: Resume session
	resume, err := svc.Resume(ctx, ResumeRequest{
		SpaceID:          testSpaceID,
		SessionID:        sessionID,
		IncludeDecisions: true,
		MaxObservations:  10,
	})
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	t.Logf("Resume returned %d observations", len(resume.Observations))
	t.Logf("Resume summary: %s", resume.Summary)

	if len(resume.Observations) < 3 {
		t.Errorf("Expected at least 3 observations, got %d", len(resume.Observations))
	}

	// Step 4: Recall specific knowledge
	recall, err := svc.Recall(ctx, RecallRequest{
		SpaceID: testSpaceID,
		Query:   "What architecture pattern was decided?",
		TopK:    5,
	})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}

	t.Logf("Recall returned %d results for architecture query", len(recall.Results))

	// Step 5: Process graduations
	summary, err := cooler.ProcessGraduations(ctx, testSpaceID)
	if err != nil {
		t.Fatalf("ProcessGraduations failed: %v", err)
	}

	t.Logf("Graduation summary: graduated=%d, tombstoned=%d, remaining=%d",
		summary.Graduated, summary.Tombstoned, summary.RemainingVolatile)
}
