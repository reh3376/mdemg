package jiminy

// JIMINY-CORPUS-001 Epic 4: relevance gate on the outcome classifier.
// Band structure (gate enabled, naThreshold ≤ lowThreshold):
//
//	sim ≥ HIGH                → followed        (unchanged)
//	LOW ≤ sim < HIGH          → tier-2 LLM / partial (unchanged)
//	NA_SIM ≤ sim < LOW        → ignored         (relevant domain, not followed — a real ignore)
//	sim < NA_SIM              → not_applicable  (unrelated domain — the guidance did not apply)
//
// Gate disabled (naThreshold ≤ 0): the whole sub-LOW tail is not_applicable
// (byte-identical to the JIMINY-OUTCOME-002 behavior at HEAD).

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mdemg/internal/llmclient"
)

func newGatedClassifier(embedder *sequenceEmbedder, naThreshold float64) *OutcomeClassifier {
	return NewOutcomeClassifier(embedder, OutcomeClassifierConfig{
		LLMEnabled:             false,
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: naThreshold,
	})
}

func TestRelevanceGate_UnrelatedTail_NotApplicable(t *testing.T) {
	// sim 0.05 < NA_SIM 0.10 — clearly-unrelated pair → not_applicable
	emb := &sequenceEmbedder{targetSim: 0.05}
	oc := newGatedClassifier(emb, 0.10)
	item := GuidanceItem{Content: "use CUIDv2 for all identifiers", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "updated README documentation")

	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("expected not_applicable for sim=0.05 < gate=0.10, got %s", cr.Outcome)
	}
	if cr.Source != "tier1" {
		t.Errorf("expected source tier1, got %s", cr.Source)
	}
}

func TestRelevanceGate_RelevantButUnfollowed_Ignored(t *testing.T) {
	// NA_SIM 0.10 ≤ sim 0.15 < LOW 0.20 — relevant-domain but not followed → still ignored
	emb := &sequenceEmbedder{targetSim: 0.15}
	oc := newGatedClassifier(emb, 0.10)
	item := GuidanceItem{Content: "always run golangci-lint before committing", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "committed the change without running lint")

	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected ignored for gate=0.10 ≤ sim=0.15 < low=0.20, got %s", cr.Outcome)
	}
	if math.Abs(cr.Confidence-0.15) > 0.05 {
		t.Errorf("expected confidence ~0.15, got %f", cr.Confidence)
	}
	if cr.Source != "tier1" {
		t.Errorf("expected source tier1, got %s", cr.Source)
	}
}

func TestRelevanceGate_BoundaryAtGate_Ignored(t *testing.T) {
	// Exactly at the gate (sim == NA_SIM) the band is inclusive → ignored
	emb := &sequenceEmbedder{targetSim: 0.10}
	oc := newGatedClassifier(emb, 0.10)
	item := GuidanceItem{Content: "use error wrapping with %w", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "wrote unrelated-ish code")

	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected ignored at boundary sim=gate=0.10, got %s", cr.Outcome)
	}
}

func TestRelevanceGate_FollowedUnchanged(t *testing.T) {
	// sim ≥ HIGH stays followed regardless of the gate
	emb := &sequenceEmbedder{targetSim: 0.60}
	oc := newGatedClassifier(emb, 0.10)
	item := GuidanceItem{Content: "add structured logging to handlers", Type: GuidancePattern}

	cr := oc.Classify(context.Background(), item, "added structured slog logging to handler return paths")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for sim=0.60 with gate enabled, got %s", cr.Outcome)
	}
}

func TestRelevanceGate_Tier2BandUnchanged(t *testing.T) {
	// LOW ≤ sim < HIGH still routes to tier-2 / heuristic partial_compliance
	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := newGatedClassifier(emb, 0.10)
	item := GuidanceItem{Content: "ensure error handling uses structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added error handling to handler.go")

	if cr.Outcome != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance for sim=0.35 (uncertain band, LLM off), got %s", cr.Outcome)
	}
}

func TestRelevanceGate_Disabled_TailStaysNotApplicable(t *testing.T) {
	// Gate disabled (threshold ≤ 0): the pre-gate behavior — the ENTIRE sub-LOW
	// tail is not_applicable (JIMINY-OUTCOME-002) — must be preserved exactly.
	for _, na := range []float64{0, -1} {
		for _, sim := range []float64{0.05, 0.15} {
			emb := &sequenceEmbedder{targetSim: sim}
			oc := newGatedClassifier(emb, na)
			item := GuidanceItem{Content: "use CUIDv2 for all identifiers", Type: GuidanceConstraint}

			cr := oc.Classify(context.Background(), item, "updated README documentation")

			want := ClassificationResult{Outcome: OutcomeNotApplicable, Confidence: cr.Confidence, Source: "tier1"}
			if cr != want {
				t.Errorf("gate=%v sim=%v: expected pre-gate result %+v, got %+v", na, sim, want, cr)
			}
			if math.Abs(cr.Confidence-sim) > 0.05 {
				t.Errorf("gate=%v: expected confidence ~%v, got %f", na, sim, cr.Confidence)
			}
		}
	}
}

func TestRelevanceGate_ClampedAboveLow(t *testing.T) {
	// Ordering invariant: NA_SIM must be ≤ LOW. A misconfigured gate above LOW
	// is clamped to LOW (with a warning) — never eats the tier-2 band.
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: 0.50,
	})
	if oc.naThreshold != 0.20 {
		t.Fatalf("expected naThreshold clamped to lowThreshold=0.20, got %f", oc.naThreshold)
	}

	// Effect: with the gate clamped to LOW, the whole sub-LOW tail is below the
	// gate → not_applicable; the tier-2 band is untouched.
	emb := &sequenceEmbedder{targetSim: 0.15}
	occ := NewOutcomeClassifier(emb, OutcomeClassifierConfig{
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: 0.50,
	})
	cr := occ.Classify(context.Background(), GuidanceItem{Content: "x y z", Type: GuidanceConstraint}, "unrelated action")
	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("expected not_applicable for sim=0.15 under clamped gate, got %s", cr.Outcome)
	}
}

func TestRelevanceGate_DefaultZeroValueDisabled(t *testing.T) {
	// Zero-value config (no NotApplicableThreshold) = gate disabled, unlike
	// high/low which default. The production default (0.10) flows from config.
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{})
	if oc.naThreshold != 0 {
		t.Errorf("expected zero-value config to leave gate disabled (0), got %f", oc.naThreshold)
	}
}

func TestRelevanceGate_LLMVerdictNotOverridden(t *testing.T) {
	// An available tier-2 LLM verdict takes precedence over similarity: when the
	// LLM says the guidance was relevant-and-ignored, the gate must not rewrite
	// it to not_applicable.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := llmclient.OpenAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: `{"outcome": "ignored", "confidence": 0.9, "reasoning": "guidance was relevant; agent did not apply it"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test server
	}))
	defer server.Close()

	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := NewOutcomeClassifier(emb, OutcomeClassifierConfig{
		LLMEnabled:             true,
		LLMProvider:            "openai",
		LLMBaseURL:             server.URL,
		LLMAPIKey:              "test-key",
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: 0.10,
	})

	item := GuidanceItem{Content: "use structured logging", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "wrote a handler with fmt.Println logging")

	if cr.Outcome != OutcomeIgnored {
		t.Errorf("expected LLM's ignored verdict preserved, got %s", cr.Outcome)
	}
	if cr.Source != "llm" {
		t.Errorf("expected source llm, got %s", cr.Source)
	}
}

func TestRelevanceGate_SubGateTail_NeverCallsLLM(t *testing.T) {
	// The gate is a tier-1 short-circuit: a clearly-unrelated pair must be
	// classified not_applicable WITHOUT an LLM call (there is no LLM verdict
	// for the gate to conflict with in the sub-LOW tail).
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		resp := llmclient.OpenAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: `{"outcome": "followed", "confidence": 0.9, "reasoning": "n/a"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test server
	}))
	defer server.Close()

	emb := &sequenceEmbedder{targetSim: 0.05}
	oc := NewOutcomeClassifier(emb, OutcomeClassifierConfig{
		LLMEnabled:             true,
		LLMProvider:            "openai",
		LLMBaseURL:             server.URL,
		LLMAPIKey:              "test-key",
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: 0.10,
	})

	cr := oc.Classify(context.Background(), GuidanceItem{Content: "pin TSDB schema version", Type: GuidanceConstraint}, "renamed a UI label")

	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("expected not_applicable for sim=0.05, got %s", cr.Outcome)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("expected 0 LLM calls for sub-gate tail, got %d", got)
	}
}
