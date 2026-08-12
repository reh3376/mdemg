package jiminy

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mdemg/internal/llmclient"
)

// sequenceEmbedder returns different embeddings on successive calls to control cosine similarity.
// First call returns [1, 0, 0], second call returns [targetSim, sqrt(1-targetSim^2), 0].
// The cosine similarity between these two vectors equals targetSim.
type sequenceEmbedder struct {
	targetSim float64
	callCount int
}

func (s *sequenceEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	s.callCount++
	if s.callCount%2 == 1 {
		// Guidance embedding (first call)
		return []float32{1, 0, 0}, nil
	}
	// Action embedding (second call) — cosine similarity with [1,0,0] = targetSim
	return []float32{float32(s.targetSim), float32(math.Sqrt(1 - s.targetSim*s.targetSim)), 0}, nil
}

func (s *sequenceEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (s *sequenceEmbedder) Name() string    { return "test-sequence" }
func (s *sequenceEmbedder) Dimensions() int { return 3 }

func newTestClassifier(embedder *sequenceEmbedder, llmEnabled bool) *OutcomeClassifier {
	return NewOutcomeClassifier(embedder, OutcomeClassifierConfig{
		LLMEnabled:    llmEnabled,
		HighThreshold: 0.55,
		LowThreshold:  0.20,
	})
}

func TestHeuristicFallback_UncertainDefaultsToIgnored(t *testing.T) {
	// JIMINY-HEURISTIC-DEFAULT-001 (2026-08-10): uncertain-range verdicts
	// (in [lowThreshold, highThreshold)) with no negation and LLM disabled
	// now default to OutcomeIgnored (was OutcomePartialCompliance). Rationale:
	// the gauge weights partial=0.5 credit; defaulting UNKNOWN to half-credit
	// inflated mdemg_jiminy_follow_rate whenever the heuristic fired. See
	// docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md.
	emb := &sequenceEmbedder{targetSim: 0.4}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "ensure error handling uses structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added error handling to handler.go")

	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected ignored (post-JIMINY-HEURISTIC-DEFAULT-001) for similarity=0.4, got %s", cr.Outcome)
	}
	if cr.Source != "heuristic" {
		t.Errorf("expected source=heuristic, got %s", cr.Source)
	}
	if math.Abs(cr.Confidence-0.4) > 0.05 {
		t.Errorf("expected confidence ~0.4, got %f", cr.Confidence)
	}
}

func TestHeuristicFallback_NotApplicableBelowLow(t *testing.T) {
	// Similarity 0.15 is below low threshold (0.20) — should be not_applicable (topics unrelated)
	emb := &sequenceEmbedder{targetSim: 0.15}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use CUIDv2 for all identifiers", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "updated README documentation")

	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("expected not_applicable for similarity=0.15, got %s", cr.Outcome)
	}
	if math.Abs(cr.Confidence-0.15) > 0.05 {
		t.Errorf("expected confidence ~0.15, got %f", cr.Confidence)
	}
}

func TestHeuristicFallback_FollowedAboveHigh(t *testing.T) {
	// Similarity 0.6 is above high threshold (0.55) — should be followed
	emb := &sequenceEmbedder{targetSim: 0.6}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "add structured logging to handlers", Type: GuidancePattern}

	cr := oc.Classify(context.Background(), item, "added structured slog.Error logging to all handler return paths")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for similarity=0.6, got %s", cr.Outcome)
	}
}

func TestNewThresholdDefaults(t *testing.T) {
	// Zero-value config should use the new defaults (0.55/0.20)
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{})

	if oc.highThreshold != 0.55 {
		t.Errorf("expected default highThreshold=0.55, got %f", oc.highThreshold)
	}
	if oc.lowThreshold != 0.20 {
		t.Errorf("expected default lowThreshold=0.20, got %f", oc.lowThreshold)
	}
}

func TestHeuristicFallback_LLMDisabled(t *testing.T) {
	// With LLM disabled, similarity 0.35 should hit heuristic fallback → partial_compliance
	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "never use UUID v4, always use CUIDv2", Type: GuidanceCorrection}

	cr := oc.Classify(context.Background(), item, "implemented identifier generation using cuid2 package")

	// JIMINY-HEURISTIC-DEFAULT-001 (2026-08-10): default is now ignored, not partial_compliance.
	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected ignored (post-JIMINY-HEURISTIC-DEFAULT-001) for similarity=0.35 with LLM disabled, got %s", cr.Outcome)
	}
}

func TestMapOutcomeString_NotApplicable(t *testing.T) {
	got := mapOutcomeString("not_applicable")
	if got != OutcomeNotApplicable {
		t.Errorf("expected OutcomeNotApplicable, got %s", got)
	}
	// Also test with whitespace
	got = mapOutcomeString("  Not_Applicable  ")
	if got != OutcomeNotApplicable {
		t.Errorf("expected OutcomeNotApplicable for padded input, got %s", got)
	}
}

func TestNotApplicable_VerySimilarIsStillFollowed(t *testing.T) {
	// High similarity should still be followed even with the not_applicable change
	emb := &sequenceEmbedder{targetSim: 0.7}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "always validate config weights sum to 1.0", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added weight validation to config.go Validate()")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for similarity=0.7, got %s", cr.Outcome)
	}
}

func TestNotApplicable_BoundaryAtLowThreshold(t *testing.T) {
	// Exactly at lowThreshold (0.20) should be in the uncertain range, NOT not_applicable
	emb := &sequenceEmbedder{targetSim: 0.20}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use error wrapping with %w", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "wrote error handling code")

	// 0.20 is >= lowThreshold, so it enters the uncertain range → partial_compliance (LLM disabled)
	// JIMINY-HEURISTIC-DEFAULT-001: default changed to Ignored.
	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected ignored at boundary sim=0.20 (post-JIMINY-HEURISTIC-DEFAULT-001), got %s", cr.Outcome)
	}
}

// --- Negation detection tests ---

func TestDetectNegation_Patterns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		pattern string
	}{
		{"instead_of", "used fmt instead of slog", true, "instead of"},
		{"did_not", "did not add error handling", true, "did not"},
		{"didnt", "didn't follow the pattern", true, "didn't"},
		{"ignored", "ignored the constraint", true, "ignored"},
		{"skipped", "skipped validation step", true, "skipped"},
		{"contrary_to", "contrary to guidance, used raw sql", true, "contrary to"},
		{"no_negation", "added structured logging to handlers", false, ""},
		{"code_content", "replaced 'skipped_test' with 'enabled_test'", true, "skipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, pat := detectNegation(tt.input)
			if got != tt.want {
				t.Errorf("detectNegation(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if pat != tt.pattern {
				t.Errorf("detectNegation(%q) pattern = %q, want %q", tt.input, pat, tt.pattern)
			}
		})
	}
}

func TestNegation_NoLLM_HeuristicContradicted(t *testing.T) {
	// Similarity 0.35 is in uncertain range, negation present, no LLM → contradicted
	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "did not use structured logging")

	if cr.Outcome != OutcomeContradicted {
		t.Errorf("expected contradicted for negation+no-LLM, got %s", cr.Outcome)
	}
}

func TestNegation_HighSim_NoLLM_HeuristicContradicted(t *testing.T) {
	// Similarity 0.7 is above high threshold, but negation present, no LLM → contradicted
	emb := &sequenceEmbedder{targetSim: 0.7}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "ignored structured logging requirement")

	if cr.Outcome != OutcomeContradicted {
		t.Errorf("expected contradicted for high-sim negation+no-LLM, got %s", cr.Outcome)
	}
}

func TestNegation_HighSim_NoNegation_Followed(t *testing.T) {
	// Similarity 0.7, no negation → followed (unchanged behavior)
	emb := &sequenceEmbedder{targetSim: 0.7}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added structured slog logging")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for high-sim no-negation, got %s", cr.Outcome)
	}
}

func TestNegation_WithLLM_DelegatesToLLM(t *testing.T) {
	// When LLM is available, negation does NOT short-circuit — LLM decides
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := llmclient.OpenAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: `{"outcome": "followed", "confidence": 0.85, "reasoning": "negation words are from quoted code content"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := NewOutcomeClassifier(emb, OutcomeClassifierConfig{
		LLMEnabled:    true,
		LLMProvider:   "openai",
		LLMBaseURL:    server.URL,
		LLMAPIKey:     "test-key",
		HighThreshold: 0.55,
		LowThreshold:  0.20,
	})

	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "replaced 'skipped_validation' with 'enabled_validation' instead of removing it")

	// The LLM should decide, not the heuristic
	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected LLM to override heuristic negation, got %s", cr.Outcome)
	}
	if cr.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85 from LLM, got %f", cr.Confidence)
	}
}

func TestNegation_WithLLM_HighSim_DelegatesToLLM(t *testing.T) {
	// High similarity + negation + LLM available → LLM confirms contradicted
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := llmclient.OpenAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: `{"outcome": "contradicted", "confidence": 0.90, "reasoning": "agent explicitly ignored the guidance"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	emb := &sequenceEmbedder{targetSim: 0.7}
	oc := NewOutcomeClassifier(emb, OutcomeClassifierConfig{
		LLMEnabled:    true,
		LLMProvider:   "openai",
		LLMBaseURL:    server.URL,
		LLMAPIKey:     "test-key",
		HighThreshold: 0.55,
		LowThreshold:  0.20,
	})

	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "ignored the structured logging requirement entirely")

	if cr.Outcome != OutcomeContradicted {
		t.Errorf("expected LLM to confirm contradicted, got %s", cr.Outcome)
	}
}

func TestBuildClassifyPrompt_WithNegation(t *testing.T) {
	item := GuidanceItem{
		Type:     GuidanceConstraint,
		Priority: "high",
		Content:  "Must use OAuth2",
	}
	prompt := buildClassifyPrompt(item, "did not use OAuth2", 0.45, false, true, "did not")

	if !strings.Contains(prompt, "Negation Detected: true") {
		t.Error("prompt should contain negation detection flag")
	}
	if !strings.Contains(prompt, `"did not"`) {
		t.Error("prompt should contain matched pattern")
	}
}

func TestBuildClassifyPrompt_WithoutNegation(t *testing.T) {
	item := GuidanceItem{
		Type:     GuidanceConstraint,
		Priority: "high",
		Content:  "Must use OAuth2",
	}
	prompt := buildClassifyPrompt(item, "Used OAuth2 for auth", 0.65, false, false, "")

	if strings.Contains(prompt, "Negation Detected") {
		t.Error("prompt should NOT contain negation section when no negation found")
	}
}

// JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001: E3 pin tests

// TestResolveClassifySystemPrompt_DefaultOff_ByteIdentical proves the
// default-off render is byte-identical to the historical classifySystemPrompt
// constant — so the ULTS system_prompt_hash pin STAYS unchanged and the
// ULTS-CI-001 drift check passes without a hash update.
func TestResolveClassifySystemPrompt_DefaultOff_ByteIdentical(t *testing.T) {
	oc := &OutcomeClassifier{compressPrompts: false, nonViolationCredit: false}
	if got := oc.resolveClassifySystemPrompt(); got != classifySystemPrompt {
		t.Errorf("default-off render is NOT byte-identical to classifySystemPrompt const — ULTS hash pin would break.\ngot len=%d, want len=%d", len(got), len(classifySystemPrompt))
	}
	oc2 := &OutcomeClassifier{compressPrompts: true, nonViolationCredit: false}
	if got := oc2.resolveClassifySystemPrompt(); got != classifySystemPromptCompact {
		t.Errorf("default-off + compressed render is NOT byte-identical to classifySystemPromptCompact")
	}
}

// TestResolveClassifySystemPrompt_Enabled_ExtendedWithClause proves the
// flag-on render appends the nonViolationCreditClause to whichever base
// prompt is selected by compressPrompts.
func TestResolveClassifySystemPrompt_Enabled_ExtendedWithClause(t *testing.T) {
	for _, tc := range []struct {
		name     string
		compress bool
		base     string
	}{
		{"full", false, classifySystemPrompt},
		{"compact", true, classifySystemPromptCompact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oc := &OutcomeClassifier{compressPrompts: tc.compress, nonViolationCredit: true}
			got := oc.resolveClassifySystemPrompt()
			want := tc.base + nonViolationCreditClause
			if got != want {
				t.Errorf("flag-on render doesn't match base + clause; got len=%d want len=%d", len(got), len(want))
			}
			// Content-level assertions on the extension clause.
			if !strings.Contains(got, "NON-VIOLATION CREDIT for must_not") {
				t.Error("extended prompt missing the must_not credit header")
			}
			if !strings.Contains(got, "\"not_applicable\", NOT \"ignored\"") {
				t.Error("extended prompt missing the not_applicable-not-ignored routing directive")
			}
			// The extension must include the concrete examples so the LLM has
			// anchors — an abstract rule alone under-transfers.
			if !strings.Contains(got, "never commit to main") {
				t.Error("extended prompt missing the concrete never-commit-to-main example")
			}
		})
	}
}

// TestNewOutcomeClassifier_NonViolationCredit_Propagates verifies the
// OutcomeClassifierConfig.NonViolationCredit field flows through
// NewOutcomeClassifier to the internal field the render checks.
func TestNewOutcomeClassifier_NonViolationCredit_Propagates(t *testing.T) {
	off := NewOutcomeClassifier(nil, OutcomeClassifierConfig{NonViolationCredit: false})
	if off.nonViolationCredit {
		t.Error("NonViolationCredit=false should not propagate to true")
	}
	on := NewOutcomeClassifier(nil, OutcomeClassifierConfig{NonViolationCredit: true})
	if !on.nonViolationCredit {
		t.Error("NonViolationCredit=true should propagate")
	}
	// And the render differs accordingly.
	if off.resolveClassifySystemPrompt() == on.resolveClassifySystemPrompt() {
		t.Error("flag on/off renders should differ")
	}
}

// JIMINY-CLASSIFIER-CONTEXT-001: pin tests

// TestResolveClassifySystemPrompt_ContextMismatch_DefaultOff_ByteIdentical
// proves that with contextMismatchCredit=false (and nonViolationCredit=false),
// the resolved prompt is byte-identical to the historical const — ULTS
// system_prompt_hash pin stays unchanged. Preserving this is why the resolver
// method is an extension point rather than a mutation of the const.
func TestResolveClassifySystemPrompt_ContextMismatch_DefaultOff_ByteIdentical(t *testing.T) {
	oc := &OutcomeClassifier{compressPrompts: false, nonViolationCredit: false, contextMismatchCredit: false}
	if got := oc.resolveClassifySystemPrompt(); got != classifySystemPrompt {
		t.Errorf("default-off render (all flags false) is NOT byte-identical to classifySystemPrompt — ULTS hash pin would break.\ngot len=%d, want len=%d", len(got), len(classifySystemPrompt))
	}
	oc2 := &OutcomeClassifier{compressPrompts: true, nonViolationCredit: false, contextMismatchCredit: false}
	if got := oc2.resolveClassifySystemPrompt(); got != classifySystemPromptCompact {
		t.Errorf("default-off + compressed render is NOT byte-identical to classifySystemPromptCompact")
	}
}

// TestResolveClassifySystemPrompt_ContextMismatch_On_Extended proves the
// flag-on render appends the contextMismatchCreditClause to whichever base
// prompt is selected.
func TestResolveClassifySystemPrompt_ContextMismatch_On_Extended(t *testing.T) {
	for _, tc := range []struct {
		name     string
		compress bool
		base     string
	}{
		{"full", false, classifySystemPrompt},
		{"compact", true, classifySystemPromptCompact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oc := &OutcomeClassifier{compressPrompts: tc.compress, contextMismatchCredit: true}
			got := oc.resolveClassifySystemPrompt()
			want := tc.base + contextMismatchCreditClause
			if got != want {
				t.Errorf("flag-on render doesn't match base + clause; got len=%d want len=%d", len(got), len(want))
			}
			// Content-level assertions on the extension clause.
			if !strings.Contains(got, "CONTEXT-MISMATCH CREDIT for any constraint type") {
				t.Error("extended prompt missing the context-mismatch credit header")
			}
			if !strings.Contains(got, "\"not_applicable\", NOT \"ignored\"") {
				t.Error("extended prompt missing the not_applicable-not-ignored routing directive")
			}
			// Concrete examples must be present — an abstract rule alone
			// under-transfers to real cases.
			if !strings.Contains(got, "read-only Cypher query") {
				t.Error("extended prompt missing the code-vs-query concrete example")
			}
			if !strings.Contains(got, "completion log / session artifact / phase description") {
				t.Error("extended prompt missing the surface-mismatch fallback example")
			}
		})
	}
}

// TestResolveClassifySystemPrompt_BothCredits_Both_Appended pins that when
// both flags are ON, both clauses splice — non-violation FIRST (must_not-
// specific — narrower), context-mismatch SECOND (general — broader). This
// ordering documents intent: the specific case is a subset of the general
// case; combining them is additive, and the LLM sees the specific example
// before the general rule.
func TestResolveClassifySystemPrompt_BothCredits_Both_Appended(t *testing.T) {
	oc := &OutcomeClassifier{
		compressPrompts:       false,
		nonViolationCredit:    true,
		contextMismatchCredit: true,
	}
	got := oc.resolveClassifySystemPrompt()
	want := classifySystemPrompt + nonViolationCreditClause + contextMismatchCreditClause
	if got != want {
		t.Errorf("both-flags render doesn't match base + both clauses in expected order.\ngot len=%d want len=%d", len(got), len(want))
	}
	// Ordering assertion: non-violation appears BEFORE context-mismatch.
	nvIdx := strings.Index(got, "NON-VIOLATION CREDIT")
	cmIdx := strings.Index(got, "CONTEXT-MISMATCH CREDIT")
	if nvIdx < 0 || cmIdx < 0 {
		t.Fatalf("both clauses must be present; nvIdx=%d cmIdx=%d", nvIdx, cmIdx)
	}
	if nvIdx > cmIdx {
		t.Errorf("non-violation clause must appear BEFORE context-mismatch clause; nvIdx=%d cmIdx=%d", nvIdx, cmIdx)
	}
}

// TestNewOutcomeClassifier_ContextMismatchCredit_Propagates verifies the
// OutcomeClassifierConfig.ContextMismatchCredit field flows through
// NewOutcomeClassifier to the internal field.
func TestNewOutcomeClassifier_ContextMismatchCredit_Propagates(t *testing.T) {
	off := NewOutcomeClassifier(nil, OutcomeClassifierConfig{ContextMismatchCredit: false})
	if off.contextMismatchCredit {
		t.Error("ContextMismatchCredit=false should not propagate to true")
	}
	on := NewOutcomeClassifier(nil, OutcomeClassifierConfig{ContextMismatchCredit: true})
	if !on.contextMismatchCredit {
		t.Error("ContextMismatchCredit=true should propagate")
	}
	if off.resolveClassifySystemPrompt() == on.resolveClassifySystemPrompt() {
		t.Error("ContextMismatchCredit flag on/off renders should differ")
	}
}


// JIMINY-CLASSIFIER-CONTEXT-002 (Phase 3 of JIMINY-CEILING-BREAK-2,
// 2026-08-12) pin tests.

// TestResolveClassifySystemPrompt_MechanismScope_DefaultOff_ByteIdentical
// proves the mechanism-scope credit flag default OFF preserves the historical
// system-prompt render exactly, so the ULTS-CI-001 hash pin is unaffected
// by this sprint (matches CONTEXT-001 shape).
func TestResolveClassifySystemPrompt_MechanismScope_DefaultOff_ByteIdentical(t *testing.T) {
	oc := &OutcomeClassifier{compressPrompts: false, mechanismScopeCredit: false}
	got := oc.resolveClassifySystemPrompt()
	want := classifySystemPrompt
	if got != want {
		t.Errorf("default-off render differs from base classifySystemPrompt — ULTS pin would break. got len=%d want len=%d", len(got), len(want))
	}
}

// TestResolveClassifySystemPrompt_MechanismScope_On_Extended proves flipping
// the flag appends the clause exactly once.
func TestResolveClassifySystemPrompt_MechanismScope_On_Extended(t *testing.T) {
	oc := &OutcomeClassifier{compressPrompts: false, mechanismScopeCredit: true}
	got := oc.resolveClassifySystemPrompt()
	want := classifySystemPrompt + mechanismScopeCreditClause
	if got != want {
		t.Errorf("mechanism-scope-on render doesn't match base + clause. got len=%d want len=%d", len(got), len(want))
	}
	if !strings.Contains(got, "MECHANISM-SCOPE HARD-PRECEDENCE GATE") {
		t.Errorf("expected 'MECHANISM-SCOPE HARD-PRECEDENCE GATE' header in output; got prefix: %s...", got[:min(400, len(got))])
	}
}

// TestNewOutcomeClassifier_MechanismScopeCredit_Propagates verifies the
// OutcomeClassifierConfig.MechanismScopeCredit field flows through
// NewOutcomeClassifier to the internal field.
func TestNewOutcomeClassifier_MechanismScopeCredit_Propagates(t *testing.T) {
	off := NewOutcomeClassifier(nil, OutcomeClassifierConfig{MechanismScopeCredit: false})
	if off.mechanismScopeCredit {
		t.Error("MechanismScopeCredit=false should not propagate to true")
	}
	on := NewOutcomeClassifier(nil, OutcomeClassifierConfig{MechanismScopeCredit: true})
	if !on.mechanismScopeCredit {
		t.Error("MechanismScopeCredit=true should propagate")
	}
	if off.resolveClassifySystemPrompt() == on.resolveClassifySystemPrompt() {
		t.Error("MechanismScopeCredit flag on/off renders should differ")
	}
}

// TestResolveClassifySystemPrompt_AllThreeCredits_Ordering pins that when
// all three flags are ON, the clauses splice in narrower→broader order
// (non-violation FIRST, context-mismatch SECOND, mechanism-scope LAST).
// The recency-weighted LLM prompt gives the STRONGEST gate the last word.
func TestResolveClassifySystemPrompt_AllThreeCredits_Ordering(t *testing.T) {
	oc := &OutcomeClassifier{
		compressPrompts:       false,
		nonViolationCredit:    true,
		contextMismatchCredit: true,
		mechanismScopeCredit:  true,
	}
	got := oc.resolveClassifySystemPrompt()
	want := classifySystemPrompt + nonViolationCreditClause + contextMismatchCreditClause + mechanismScopeCreditClause
	if got != want {
		t.Errorf("all-three-flags render doesn't match base + all three clauses in expected order. got len=%d want len=%d", len(got), len(want))
	}
	nvIdx := strings.Index(got, "NON-VIOLATION CREDIT")
	cmIdx := strings.Index(got, "CONTEXT-MISMATCH CREDIT")
	msIdx := strings.Index(got, "MECHANISM-SCOPE HARD-PRECEDENCE GATE")
	if nvIdx < 0 || cmIdx < 0 || msIdx < 0 {
		t.Fatalf("all three clauses must be present; nvIdx=%d cmIdx=%d msIdx=%d", nvIdx, cmIdx, msIdx)
	}
	if !(nvIdx < cmIdx && cmIdx < msIdx) {
		t.Errorf("clause ordering must be non-violation < context-mismatch < mechanism-scope (recency-weighted strongest gate last); got nv=%d cm=%d ms=%d", nvIdx, cmIdx, msIdx)
	}
}
