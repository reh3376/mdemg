package jiminy

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// --- J7: Retrieval Source Tests ---

func TestMapRetrievalToGuidance_Basic(t *testing.T) {
	results := []RetrievalResult{
		{NodeID: "n1", Name: "Auth Pattern", Summary: "Use OAuth2", Layer: 0, Score: 0.85, ObsType: "decision"},
		{NodeID: "n2", Name: "API Design", Summary: "REST conventions", Layer: 3, Score: 0.90},
		{NodeID: "n3", Name: "Bug Fix", Summary: "", Layer: 0, Score: 0.75, ObsType: "correction"},
	}

	items := mapRetrievalToGuidance(results, 0.45, 8.0)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Check type classification
	if items[0].Type != GuidanceDecision {
		t.Errorf("expected decision type, got %s", items[0].Type)
	}
	if items[1].Type != GuidanceConcept {
		t.Errorf("expected concept type for L3 node, got %s", items[1].Type)
	}
	if items[2].Type != GuidanceCorrection {
		t.Errorf("expected correction type, got %s", items[2].Type)
	}
}

func TestMapRetrievalToGuidance_EmptyContent(t *testing.T) {
	results := []RetrievalResult{
		{NodeID: "n1", Name: "", Summary: "", Layer: 0, Score: 0.5},
	}
	items := mapRetrievalToGuidance(results, 0.45, 8.0)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty content, got %d", len(items))
	}
}

func TestClassifyRetrievalItem(t *testing.T) {
	tests := []struct {
		name     string
		result   RetrievalResult
		expected GuidanceType
	}{
		// Existing obs_type + layer table (unchanged).
		{"L3 concept", RetrievalResult{Layer: 3}, GuidanceConcept},
		{"L5 concept", RetrievalResult{Layer: 5}, GuidanceConcept},
		{"correction obs", RetrievalResult{Layer: 0, ObsType: "correction"}, GuidanceCorrection},
		{"constraint obs", RetrievalResult{Layer: 0, ObsType: "constraint"}, GuidanceConstraint},
		{"error", RetrievalResult{Layer: 0, ObsType: "error"}, GuidanceRisk},
		{"preference", RetrievalResult{Layer: 0, ObsType: "preference"}, GuidancePreference},
		{"unknown obs_type", RetrievalResult{Layer: 0, ObsType: "custom"}, GuidanceLearning},

		// JIMINY-ROLETYPE-ADAPTER-001: role_type precedence. Constraint /
		// correction MemoryNodes are Layer 1 in mdemg-dev, so without this
		// prefix they defaulted to GuidanceLearning.
		{"L1 role=constraint", RetrievalResult{Layer: 1, RoleType: "constraint"}, GuidanceConstraint},
		{"L1 role=correction", RetrievalResult{Layer: 1, RoleType: "correction"}, GuidanceCorrection},
		{"role beats obs (constraint vs learning)", RetrievalResult{Layer: 1, RoleType: "constraint", ObsType: "learning"}, GuidanceConstraint},
		{"role beats L>=2 concept short-circuit", RetrievalResult{Layer: 2, RoleType: "constraint"}, GuidanceConstraint},
		{"L0 default preserved", RetrievalResult{Layer: 0}, GuidanceLearning},
		{"L1 obs=learning still learning when no role", RetrievalResult{Layer: 1, ObsType: "learning"}, GuidanceLearning},
		{"unknown role_type ignored (falls through)", RetrievalResult{Layer: 0, RoleType: "banana"}, GuidanceLearning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRetrievalItem(tt.result)
			if got != tt.expected {
				t.Errorf("classifyRetrievalItem(%s) = %s, want %s", tt.name, got, tt.expected)
			}
		})
	}
}

func TestLayerToPriority(t *testing.T) {
	if layerToPriority(0) != "low" {
		t.Error("L0 should be low priority")
	}
	if layerToPriority(1) != "medium" {
		t.Error("L1 should be medium priority")
	}
	if layerToPriority(3) != "high" {
		t.Error("L3 should be high priority")
	}
	if layerToPriority(5) != "high" {
		t.Error("L5 should be high priority")
	}
}

// --- J7: Guide with retriever ---

type mockRetriever struct {
	results []RetrievalResult
	err     error
}

func (m *mockRetriever) RetrieveForJiminy(_ context.Context, _, _ string, _, _ int, _ []float32) ([]RetrievalResult, error) {
	return m.results, m.err
}

func TestGuide_WithRetriever(t *testing.T) {
	retriever := &mockRetriever{
		results: []RetrievalResult{
			{NodeID: "r1", Name: "Past Decision", Summary: "Use PostgreSQL", Layer: 0, Score: 3.0, ObsType: "decision"},
		},
	}
	cfg := config.Config{
		JiminyEnabled:           true,
		JiminyTimeoutMs:         2000,
		JiminyMaxItems:          10,
		JiminyMinConfidence:     0.3,
		JiminyRetrievalEnabled:  true,
		JiminyRetrievalTopK:     10,
		JiminyRetrievalHopDepth: 2,
	}
	s := NewService(cfg, nil, nil, nil)
	s.SetRetriever(retriever)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Which database should I use?",
	})
	if err != nil {
		t.Fatalf("Guide failed: %v", err)
	}

	if len(resp.Guidance) == 0 {
		t.Error("expected guidance items from retriever")
	}
	if resp.SourceCounts.Retrievals != 1 {
		t.Errorf("expected 1 retrieval, got %d", resp.SourceCounts.Retrievals)
	}
}

// --- J8: Synthesis Tests ---

func TestGuidancePromptBuild(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "Must use OAuth2", Confidence: 0.9, SourceNodes: []string{"c1"}},
		{Type: GuidanceCorrection, Content: "Fixed auth bug", Confidence: 0.8, SourceNodes: []string{"c2"}},
	}
	prompt := buildGuidancePrompt(items, "Working on auth", "", 200000, 200000)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if len(prompt) < 50 {
		t.Error("prompt seems too short")
	}
}

func TestGuidancePromptBuild_WithAgentOutput(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "No hardcoded values", Confidence: 0.9},
	}
	prompt := buildGuidancePrompt(items, "Coding", "hardcoded path = '/var/log'", 200000, 200000)
	if prompt == "" {
		t.Error("expected non-empty prompt with agent output")
	}
}

// --- J9: Evaluator Tests ---

func TestEvaluator_MissingSpaceID(t *testing.T) {
	e := NewEvaluator(config.Config{}, nil, nil)
	_, err := e.Evaluate(context.Background(), EvaluateRequest{AgentOutput: "test"})
	if err == nil {
		t.Error("expected error for missing space_id")
	}
}

func TestEvaluator_MissingAgentOutput(t *testing.T) {
	e := NewEvaluator(config.Config{}, nil, nil)
	_, err := e.Evaluate(context.Background(), EvaluateRequest{SpaceID: "test"})
	if err == nil {
		t.Error("expected error for missing agent_output")
	}
}

func TestEvaluator_NoEmbedder(t *testing.T) {
	e := NewEvaluator(config.Config{}, nil, nil)
	resp, err := e.Evaluate(context.Background(), EvaluateRequest{
		SpaceID:     "test",
		AgentOutput: "some code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "pass" {
		t.Errorf("expected pass with no embedder, got %s", resp.Status)
	}
	if resp.EvaluationID == "" {
		t.Error("expected evaluation_id to be set")
	}
}

// --- J11: Outcome Classifier Tests ---

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	sim := cosineSimilarity(a, b)
	if sim < 0.99 {
		t.Errorf("expected ~1.0, got %.4f", sim)
	}

	c := []float32{0, 1, 0}
	sim = cosineSimilarity(a, c)
	if sim > 0.01 {
		t.Errorf("expected ~0.0, got %.4f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := cosineSimilarity(nil, nil)
	if sim != 0.0 {
		t.Errorf("expected 0.0 for nil, got %.4f", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	sim := cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0})
	if sim != 0.0 {
		t.Errorf("expected 0.0 for different lengths, got %.4f", sim)
	}
}

func TestOutcomeClassifier_FallbackToTextOverlap(t *testing.T) {
	// No embedder → falls back to classifyOutcome
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{})
	cr := oc.Classify(context.Background(),
		GuidanceItem{Content: "always validate input data carefully"},
		"validated input data carefully before processing")
	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed, got %s", cr.Outcome)
	}
}

// --- J12: Escalation Tests ---

func TestEscalationTracker_BasicFlow(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     2,
		JiminyEscalationEscalateAfter: 4,
		JiminyEscalationBlockAfter:    6,
		JiminyEscalationBlockEnabled:  false,
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)

	// First surface → SURFACED
	level := et.RecordSurface("session1", "constraint1")
	if level != EscalationSurfaced {
		t.Errorf("expected surfaced, got %s", level)
	}

	// Follow → RESOLVED
	level = et.RecordOutcome("session1", "constraint1", OutcomeFollowed)
	if level != EscalationResolved {
		t.Errorf("expected resolved, got %s", level)
	}
}

func TestEscalationTracker_EscalationPath(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     2,
		JiminyEscalationEscalateAfter: 4,
		JiminyEscalationBlockAfter:    6,
		JiminyEscalationBlockEnabled:  true,
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)

	et.RecordSurface("s1", "c1")

	// Ignore twice → WARNED
	et.RecordOutcome("s1", "c1", OutcomeIgnored)
	level := et.RecordOutcome("s1", "c1", OutcomeIgnored)
	if level != EscalationWarned {
		t.Errorf("expected warned after 2 ignores, got %s", level)
	}

	// Ignore 2 more → ESCALATED
	et.RecordOutcome("s1", "c1", OutcomeIgnored)
	level = et.RecordOutcome("s1", "c1", OutcomeIgnored)
	if level != EscalationEscalated {
		t.Errorf("expected escalated after 4 ignores, got %s", level)
	}

	// Ignore 2 more → BLOCKED
	et.RecordOutcome("s1", "c1", OutcomeIgnored)
	level = et.RecordOutcome("s1", "c1", OutcomeIgnored)
	if level != EscalationBlocked {
		t.Errorf("expected blocked after 6 ignores, got %s", level)
	}
}

func TestEscalationTracker_BlockDisabled(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     1,
		JiminyEscalationEscalateAfter: 2,
		JiminyEscalationBlockAfter:    3,
		JiminyEscalationBlockEnabled:  false, // blocking disabled
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)

	et.RecordSurface("s1", "c1")

	// Should max out at ESCALATED, never reach BLOCKED
	for i := 0; i < 10; i++ {
		et.RecordOutcome("s1", "c1", OutcomeIgnored)
	}
	level := et.GetState("s1", "c1")
	if level == EscalationBlocked {
		t.Error("should not reach blocked when blockEnabled=false")
	}
	if level != EscalationEscalated {
		t.Errorf("expected escalated, got %s", level)
	}
}

func TestEscalationTracker_DecayReset(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     2,
		JiminyEscalationEscalateAfter: 4,
		JiminyEscalationBlockAfter:    6,
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)
	// Override decay for testing
	et.decayDuration = 1 * time.Millisecond

	et.RecordSurface("s1", "c1")
	et.RecordOutcome("s1", "c1", OutcomeIgnored)
	et.RecordOutcome("s1", "c1", OutcomeIgnored)

	// Wait for decay
	time.Sleep(5 * time.Millisecond)

	// After decay, should reset to SURFACED
	level := et.RecordSurface("s1", "c1")
	if level != EscalationSurfaced {
		t.Errorf("expected reset to surfaced after decay, got %s", level)
	}
}

func TestApplyEscalation_Warned(t *testing.T) {
	item := GuidanceItem{
		Type:     GuidanceConstraint,
		Priority: "medium",
		Content:  "Use config files",
	}
	ApplyEscalation(&item, EscalationWarned)
	if item.Priority != "high" {
		t.Errorf("expected high priority, got %s", item.Priority)
	}
	if item.Content[:9] != "REPEATED:" {
		t.Errorf("expected REPEATED: prefix, got %s", item.Content[:20])
	}
}

func TestApplyEscalation_Escalated(t *testing.T) {
	item := GuidanceItem{
		Type:       GuidanceConstraint,
		Priority:   "low",
		Content:    "Validate inputs",
		Confidence: 0.5,
	}
	ApplyEscalation(&item, EscalationEscalated)
	if item.Priority != "high" {
		t.Errorf("expected high priority, got %s", item.Priority)
	}
	if item.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %.2f", item.Confidence)
	}
}

func TestEscalationTracker_SessionEscalation(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     1,
		JiminyEscalationEscalateAfter: 3,
		JiminyEscalationBlockAfter:    5,
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)

	et.RecordSurface("s1", "c1")
	et.RecordOutcome("s1", "c1", OutcomeIgnored) // → warned

	summary := et.GetSessionEscalation("s1")
	if summary == nil {
		t.Fatal("expected non-nil session escalation")
	}
	if summary.WarnedCount != 1 {
		t.Errorf("expected 1 warned, got %d", summary.WarnedCount)
	}
}

func TestEscalationTracker_CleanupExpired(t *testing.T) {
	cfg := config.Config{
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     2,
		JiminyEscalationEscalateAfter: 4,
		JiminyEscalationBlockAfter:    6,
		JiminyEscalationDecayMinutes:  60,
	}
	et := NewEscalationTracker(cfg)
	et.decayDuration = 1 * time.Millisecond

	et.RecordSurface("s1", "c1")
	et.RecordSurface("s1", "c2")

	time.Sleep(5 * time.Millisecond)

	removed := et.CleanupExpired()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if et.Size() != 0 {
		t.Errorf("expected 0 remaining, got %d", et.Size())
	}
}

// --- Guide with escalation integration ---

func TestGuide_WithEscalation(t *testing.T) {
	consultant := &mockConsultant{
		resp: mockSuggestResponseWithConstraint(),
	}
	cfg := config.Config{
		JiminyEnabled:                 true,
		JiminyTimeoutMs:               2000,
		JiminyMaxItems:                10,
		JiminyMinConfidence:           0.3,
		JiminyEscalationEnabled:       true,
		JiminyEscalationWarnAfter:     1,
		JiminyEscalationEscalateAfter: 3,
		JiminyEscalationBlockAfter:    5,
		JiminyEscalationDecayMinutes:  60,
	}
	s := NewService(cfg, nil, consultant, nil)

	// First guide — should just surface
	resp1, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "working on auth",
		SessionID: "session1",
	})
	if err != nil {
		t.Fatalf("Guide failed: %v", err)
	}
	if len(resp1.Guidance) == 0 {
		t.Error("expected guidance items")
	}
}

// --- Rationale with retrievals ---

func TestBuildRationale_WithRetrievals(t *testing.T) {
	counts := SourceCounts{
		Constraints: 2,
		Corrections: 1,
		Retrievals:  3,
	}
	rationale := buildRationale(counts)
	if rationale == "" {
		t.Error("expected non-empty rationale")
	}
}

// --- Helpers ---

func mockSuggestResponseWithConstraint() models.SuggestResponse {
	return models.SuggestResponse{
		SpaceID:    "test",
		Confidence: 0.85,
		Constraints: []models.Constraint{
			{
				Name:           "Auth Constraint",
				Description:    "Must use OAuth2 for authentication",
				ConstraintType: "must",
				SourceNodes:    []string{"constraint-1"},
				Confidence:     0.9,
			},
		},
	}
}
