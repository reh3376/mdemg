package jiminy

import (
	"context"
	"strings"
	"testing"

	"mdemg/internal/config"
)

// ─── J13: Evaluator LLM Reasoning Tests ───

func TestEvaluator_LLMDisabled_VectorOnly(t *testing.T) {
	// With LLM disabled, evaluator should still work via Tier 1 only
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
}

func TestEvalPromptBuild(t *testing.T) {
	constraints := []EvaluationItem{
		{Type: GuidanceConstraint, Content: "[must] Use OAuth2", Severity: "high", SourceNode: "c1"},
		{Type: GuidanceConstraint, Content: "[should] Prefer REST", Severity: "medium", SourceNode: "c2"},
	}
	corrections := []EvaluationItem{
		{Type: GuidanceCorrection, Content: "Prior correction: Fixed auth bug", Severity: "medium", SourceNode: "c3"},
	}

	prompt := buildEvalPrompt("const token = 'hardcoded'", constraints, corrections, 200000, 0)

	if !strings.Contains(prompt, "Agent Output") {
		t.Error("prompt should contain Agent Output section")
	}
	if !strings.Contains(prompt, "Active Constraints") {
		t.Error("prompt should contain Active Constraints section")
	}
	if !strings.Contains(prompt, "Past Corrections") {
		t.Error("prompt should contain Past Corrections section")
	}
	if !strings.Contains(prompt, "c1") {
		t.Error("prompt should contain constraint node IDs")
	}
	if !strings.Contains(prompt, "must") {
		t.Error("prompt should contain constraint type")
	}
}

func TestParseEvalResponse_ValidJSON(t *testing.T) {
	raw := `{"violations": [{"constraint_node_id": "c1", "description": "uses hardcoded token", "reasoning": "violates must constraint", "remediation": "use config file"}], "warnings": []}`
	result, err := parseEvalResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(result.Violations))
	}
	if result.Violations[0].ConstraintNodeID != "c1" {
		t.Errorf("expected constraint_node_id c1, got %s", result.Violations[0].ConstraintNodeID)
	}
	if result.Violations[0].Reasoning != "violates must constraint" {
		t.Errorf("unexpected reasoning: %s", result.Violations[0].Reasoning)
	}
}

func TestParseEvalResponse_EmptyResult(t *testing.T) {
	raw := `{"violations": [], "warnings": []}`
	result, err := parseEvalResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(result.Violations))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(result.Warnings))
	}
}

func TestParseEvalResponse_MarkdownFenced(t *testing.T) {
	raw := "```json\n{\"violations\": [], \"warnings\": [{\"constraint_node_id\": \"w1\", \"description\": \"test\", \"reasoning\": \"r\", \"remediation\": \"fix\"}]}\n```"
	result, err := parseEvalResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestRevalidateResults_DemoteShouldToWarning(t *testing.T) {
	result := &llmEvalResult{
		Violations: []llmEvalViolation{
			{ConstraintNodeID: "c1", Description: "deviates from recommendation", Reasoning: "test", Remediation: "fix"},
		},
	}
	constraintMap := map[string]EvaluationItem{
		"c1": {Type: GuidanceConstraint, Content: "[should] Prefer REST", Severity: "medium", SourceNode: "c1"},
	}
	items := revalidateResults(result, constraintMap)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Severity != "medium" {
		t.Errorf("should constraint violation should be demoted to medium, got %s", items[0].Severity)
	}
	if !strings.Contains(items[0].Reasoning, "demoted") {
		t.Error("reasoning should indicate demotion")
	}
}

func TestRevalidateResults_KeepMustAsViolation(t *testing.T) {
	result := &llmEvalResult{
		Violations: []llmEvalViolation{
			{ConstraintNodeID: "c1", Description: "violates auth", Reasoning: "direct violation", Remediation: "use OAuth2"},
		},
	}
	constraintMap := map[string]EvaluationItem{
		"c1": {Type: GuidanceConstraint, Content: "[must] Use OAuth2", Severity: "high", SourceNode: "c1"},
	}
	items := revalidateResults(result, constraintMap)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Severity != "high" {
		t.Errorf("must constraint violation should stay high, got %s", items[0].Severity)
	}
}

func TestRevalidateResults_UnknownNodeDemoted(t *testing.T) {
	result := &llmEvalResult{
		Violations: []llmEvalViolation{
			{ConstraintNodeID: "unknown-node", Description: "test", Reasoning: "test", Remediation: "test"},
		},
	}
	constraintMap := map[string]EvaluationItem{}
	items := revalidateResults(result, constraintMap)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Severity != "medium" {
		t.Errorf("unknown node should be demoted to medium, got %s", items[0].Severity)
	}
	if !strings.Contains(items[0].Reasoning, "not found") {
		t.Error("reasoning should mention constraint not found")
	}
}

func TestEvalCache_HitAndMiss(t *testing.T) {
	e := NewEvaluator(config.Config{}, nil, nil)

	key := evalCacheKey("test output", "constraint-1")
	result := &llmEvalResult{
		Violations: []llmEvalViolation{{ConstraintNodeID: "c1", Description: "test"}},
		Warnings:   []llmEvalViolation{},
	}

	// Miss
	if got := e.evalCacheGet(key); got != nil {
		t.Error("expected cache miss")
	}

	// Put
	e.evalCachePut(key, result)

	// Hit
	got := e.evalCacheGet(key)
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if len(got.Violations) != 1 {
		t.Errorf("expected 1 violation in cached result, got %d", len(got.Violations))
	}
}

func TestEvalCache_Eviction(t *testing.T) {
	e := NewEvaluator(config.Config{}, nil, nil)
	e.cacheCap = 2 // Small capacity for testing

	result := &llmEvalResult{Violations: []llmEvalViolation{}, Warnings: []llmEvalViolation{}}

	e.evalCachePut("key1", result)
	e.evalCachePut("key2", result)
	e.evalCachePut("key3", result) // Should evict key1

	if e.evalCacheGet("key1") != nil {
		t.Error("key1 should have been evicted")
	}
	if e.evalCacheGet("key2") == nil {
		t.Error("key2 should still be cached")
	}
	if e.evalCacheGet("key3") == nil {
		t.Error("key3 should still be cached")
	}
}

func TestEvaluator_LLMFallback(t *testing.T) {
	// If LLM is enabled but evaluator has no LLM client, should still work (Tier 1 only)
	e := NewEvaluator(config.Config{JiminyEvaluateLLMEnabled: true}, nil, nil)
	resp, err := e.Evaluate(context.Background(), EvaluateRequest{
		SpaceID:     "test",
		AgentOutput: "some code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "pass" {
		t.Errorf("expected pass, got %s", resp.Status)
	}
}

func TestTruncateForPrompt(t *testing.T) {
	if truncateForPrompt("short", 100) != "short" {
		t.Error("should not truncate short text")
	}
	result := truncateForPrompt("this is a longer text", 10)
	if len(result) != 13 { // 10 + "..."
		t.Errorf("expected truncated to 13 chars, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated text should end with ...")
	}
}

func TestConstraintTypeFromContent(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"[must] Use OAuth2", "must"},
		{"[should] Prefer REST", "should"},
		{"[must_not] No hardcoded", "must_not"},
		{"no brackets here", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := constraintTypeFromContent(tt.content)
		if got != tt.expected {
			t.Errorf("constraintTypeFromContent(%q) = %q, want %q", tt.content, got, tt.expected)
		}
	}
}

// ─── J14: Outcome Classifier Upgrade Tests ───

func TestOutcomeClassifier_PartialCompliance(t *testing.T) {
	// partial_compliance is a valid outcome
	result := mapOutcomeString("partial_compliance")
	if result != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance, got %s", result)
	}
}

func TestOutcomeClassifier_CacheHit(t *testing.T) {
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{CacheSize: 10})
	key := classifyCacheKey("test guidance", "test action")

	expected := ClassificationResult{
		Outcome:    OutcomeFollowed,
		Confidence: 0.85,
		Reasoning:  "agent followed guidance",
	}

	// Miss
	if oc.classifyCacheGet(key) != nil {
		t.Error("expected cache miss")
	}

	// Put + Hit
	oc.classifyCachePut(key, expected)
	got := oc.classifyCacheGet(key)
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if got.Outcome != OutcomeFollowed {
		t.Errorf("expected followed, got %s", got.Outcome)
	}
	if got.Reasoning != "agent followed guidance" {
		t.Errorf("expected reasoning, got %s", got.Reasoning)
	}
}

func TestOutcomeClassifier_CacheEviction(t *testing.T) {
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{CacheSize: 2})

	result := ClassificationResult{Outcome: OutcomeFollowed, Confidence: 0.8}
	oc.classifyCachePut("k1", result)
	oc.classifyCachePut("k2", result)
	oc.classifyCachePut("k3", result) // Evicts k1

	if oc.classifyCacheGet("k1") != nil {
		t.Error("k1 should have been evicted")
	}
	if oc.classifyCacheGet("k2") == nil {
		t.Error("k2 should still be cached")
	}
}

func TestOutcomeClassifier_InvalidOutcomeEnum(t *testing.T) {
	result := mapOutcomeString("invalid_value")
	if result != OutcomeUnknown {
		t.Errorf("expected unknown for invalid enum, got %s", result)
	}
}

func TestClassifyPromptBuild(t *testing.T) {
	item := GuidanceItem{
		Type:        GuidanceConstraint,
		Priority:    "high",
		Content:     "Must use OAuth2",
		SourceNodes: []string{"c1"},
	}
	prompt := buildClassifyPrompt(item, "Used OAuth2 for auth", 0.65)

	if !strings.Contains(prompt, "constraint") {
		t.Error("prompt should contain guidance type")
	}
	if !strings.Contains(prompt, "high") {
		t.Error("prompt should contain priority")
	}
	if !strings.Contains(prompt, "Must use OAuth2") {
		t.Error("prompt should contain guidance content")
	}
	if !strings.Contains(prompt, "0.650") {
		t.Error("prompt should contain similarity score")
	}
	if !strings.Contains(prompt, "c1") {
		t.Error("prompt should contain source node")
	}
}

func TestParseClassifyResponse_Valid(t *testing.T) {
	raw := `{"outcome": "followed", "confidence": 0.92, "reasoning": "agent implemented OAuth2 as required"}`
	cr := parseClassifyResponse(raw, 0.5)
	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed, got %s", cr.Outcome)
	}
	if cr.Confidence != 0.92 {
		t.Errorf("expected confidence 0.92, got %.2f", cr.Confidence)
	}
	if cr.Reasoning != "agent implemented OAuth2 as required" {
		t.Errorf("unexpected reasoning: %s", cr.Reasoning)
	}
}

func TestParseClassifyResponse_PartialCompliance(t *testing.T) {
	raw := `{"outcome": "partial_compliance", "confidence": 0.7, "reasoning": "addressed auth but not token rotation"}`
	cr := parseClassifyResponse(raw, 0.5)
	if cr.Outcome != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance, got %s", cr.Outcome)
	}
}

func TestParseClassifyResponse_InvalidJSON(t *testing.T) {
	raw := "not json"
	cr := parseClassifyResponse(raw, 0.5)
	if cr.Outcome != OutcomeUnknown {
		t.Errorf("expected unknown for invalid JSON, got %s", cr.Outcome)
	}
	if cr.Confidence != 0.5 {
		t.Errorf("expected fallback confidence 0.5, got %.2f", cr.Confidence)
	}
}

func TestRecordOutcome_WithReasoning(t *testing.T) {
	// Use a service with no classifier (text overlap fallback) to verify ClassificationResult flow
	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     2000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.1,
	}
	s := NewService(cfg, nil, nil, nil)

	// Track a guidance item
	s.tracker.Track("test-guid-1", []GuidanceItem{
		{Type: GuidanceConstraint, Content: "always validate input data", Confidence: 0.9},
	})

	resp, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    "test-guid-1",
		ActionSummary: "validated input data before processing",
		SpaceID:       "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Applied {
		t.Error("expected applied=true")
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Outcome != OutcomeFollowed {
		t.Errorf("expected followed, got %s", resp.Results[0].Outcome)
	}
}

// ─── J15: Synthesis Prompt Engineering Tests ───

func TestGuidanceSystemPrompt_ContainsConstraintTypes(t *testing.T) {
	if !strings.Contains(guidanceSystemPrompt, "must") {
		t.Error("system prompt should mention must constraints")
	}
	if !strings.Contains(guidanceSystemPrompt, "must_not") {
		t.Error("system prompt should mention must_not constraints")
	}
	if !strings.Contains(guidanceSystemPrompt, "should") {
		t.Error("system prompt should mention should constraints")
	}
	if !strings.Contains(guidanceSystemPrompt, "RECOMMENDATIONS") {
		t.Error("system prompt should distinguish should as recommendations")
	}
	if !strings.Contains(guidanceSystemPrompt, "NON-NEGOTIABLE") {
		t.Error("system prompt should mark must as non-negotiable")
	}
	if !strings.Contains(guidanceSystemPrompt, "CONCEPTS") {
		t.Error("system prompt should mention concepts")
	}
	if !strings.Contains(guidanceSystemPrompt, "DECISIONS") {
		t.Error("system prompt should mention decisions")
	}
}

func TestBuildGuidancePrompt_ConceptsSeparated(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "[must] Use OAuth2", Confidence: 0.9, SourceNodes: []string{"c1"}},
		{Type: GuidanceConcept, Content: "Auth patterns cluster", Confidence: 0.8, SourceNodes: []string{"n1"}},
	}
	prompt := buildGuidancePrompt(items, "Working on auth", "", 200000, 200000)

	if !strings.Contains(prompt, "ORGANIZATIONAL CONCEPTS") {
		t.Error("concepts should be in separate section")
	}
	if !strings.Contains(prompt, "hidden layers") {
		t.Error("concepts section should mention hidden layers")
	}
}

func TestBuildGuidancePrompt_ConstraintSubTypes(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "[must] No hardcoded values", Confidence: 0.95, SourceNodes: []string{"c1"}},
		{Type: GuidanceConstraint, Content: "[should] Prefer config files", Confidence: 0.7, SourceNodes: []string{"c2"}},
	}
	prompt := buildGuidancePrompt(items, "Working on config", "", 200000, 200000)

	if !strings.Contains(prompt, "[must]") {
		t.Error("prompt should show [must] sub-type label")
	}
	if !strings.Contains(prompt, "[should]") {
		t.Error("prompt should show [should] sub-type label")
	}
	if !strings.Contains(prompt, "[conf:") {
		t.Error("prompt should show confidence scores")
	}
}

func TestSynthesizer_TemperaturePassthrough(t *testing.T) {
	temp := 0.3
	cfg := SynthesisConfig{
		Enabled:     true,
		Provider:    "openai",
		Model:       "test",
		MaxTokens:   100,
		TimeoutMs:   1000,
		Temperature: &temp,
	}
	syn := NewGuidanceSynthesizer(cfg, nil)
	if syn.cfg.Temperature == nil {
		t.Error("temperature should be set")
	}
	if *syn.cfg.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", *syn.cfg.Temperature)
	}
}

func TestSynthesizer_DefaultsChanged(t *testing.T) {
	// J15: Verify new defaults — these are the config parsing defaults, tested here for documentation
	cfg := config.Config{}
	// The defaults are set in FromEnv, but we verify the struct zero-values don't interfere
	// by testing the synthesizer handles zero maxTokens correctly
	synCfg := SynthesisConfig{
		Enabled:   true,
		Provider:  "openai",
		MaxTokens: 0, // should default to 2000 in Synthesize()
		TimeoutMs: 0, // should default to 5000 in Synthesize()
	}
	syn := NewGuidanceSynthesizer(synCfg, nil)
	// Can't call Synthesize without LLM, but verify the struct is created
	if syn == nil {
		t.Error("synthesizer should be created")
	}
	_ = cfg
}

func TestExtractConstraintSubType(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"[must] Use OAuth2", "must"},
		{"[should] Prefer REST", "should"},
		{"[must_not] No hardcoded", "must_not"},
		{"no brackets", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractConstraintSubType(tt.content)
		if got != tt.expected {
			t.Errorf("extractConstraintSubType(%q) = %q, want %q", tt.content, got, tt.expected)
		}
	}
}

func TestCleanEvalJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"plain JSON", `{"violations":[],"warnings":[]}`, `"violations"`},
		{"markdown fenced", "```json\n{\"violations\":[],\"warnings\":[]}\n```", `"violations"`},
		{"text before JSON", "Here is the result:\n{\"violations\":[],\"warnings\":[]}", `"violations"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := cleanEvalJSONResponse(tt.input)
			if !strings.Contains(cleaned, tt.contains) {
				t.Errorf("cleaned response should contain %q, got: %s", tt.contains, cleaned)
			}
		})
	}
}

// --- J16: Cache Key Collision Tests ---

func TestCacheKey_FullContextHash(t *testing.T) {
	base := strings.Repeat("a", 200)
	k1 := cacheKey("space", base+"SUFFIX_A")
	k2 := cacheKey("space", base+"SUFFIX_B")
	if k1 == k2 {
		t.Error("cache keys should differ for different contexts even with shared 200-char prefix")
	}
}

func TestEvalCacheKey_FullOutputHash(t *testing.T) {
	base := strings.Repeat("x", 200)
	k1 := evalCacheKey(base+"OUTPUT_A", "c1")
	k2 := evalCacheKey(base+"OUTPUT_B", "c1")
	if k1 == k2 {
		t.Error("eval cache keys should differ for different outputs even with shared prefix")
	}
}
