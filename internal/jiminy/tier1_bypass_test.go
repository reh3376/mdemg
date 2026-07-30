package jiminy

// JIMINY-TIER1-BYPASS-001: bypass tier1 (embedding-similarity) for the
// follow/ignore decision. Keep tier1 as the fast pre-gate for the sub-
// naThreshold → NotApplicable case only. All other cases route to LLM tier2.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mdemg/internal/llmclient"
)

func newBypassClassifier(embedder *sequenceEmbedder, naThreshold float64, bypass bool, llmURL string) *OutcomeClassifier {
	oc := NewOutcomeClassifier(embedder, OutcomeClassifierConfig{
		LLMEnabled:             llmURL != "",
		LLMProvider:            "openai",
		LLMModel:               "test-model",
		LLMAPIKey:              "test-key",
		LLMBaseURL:             llmURL,
		HighThreshold:          0.55,
		LowThreshold:           0.20,
		NotApplicableThreshold: naThreshold,
		Tier1BypassEnabled:     bypass,
	})
	return oc
}

// stubLLM answers every classify call with a canned verdict for testing that
// the classifier ROUTED to tier2 (not that the LLM verdict itself is right).
func stubLLM(t *testing.T, verdict string, hitCounter *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCounter.Add(1)
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"outcome":"` + verdict + `","confidence":0.8,"reasoning":"test"}`,
				}},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestTier1Bypass_SubNAThreshold_StaysNotApplicable(t *testing.T) {
	// sim 0.05 < naThreshold 0.10 → NotApplicable, tier1 (unchanged by bypass)
	var hits atomic.Int32
	srv := stubLLM(t, "followed", &hits)
	defer srv.Close()
	emb := &sequenceEmbedder{targetSim: 0.05}
	oc := newBypassClassifier(emb, 0.10, true, srv.URL)
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "unrelated action")
	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("bypass ON + sub-naThreshold should still be NotApplicable; got %s", cr.Outcome)
	}
	if cr.Source != "tier1" {
		t.Errorf("expected tier1 source (pre-gate stays); got %s", cr.Source)
	}
	if hits.Load() != 0 {
		t.Errorf("LLM should NOT be called for sub-naThreshold; hits=%d", hits.Load())
	}
}

func TestTier1Bypass_BandOff_TerminalIgnored(t *testing.T) {
	// bypass OFF: [naThreshold, lowThreshold) → tier1 Ignored (legacy behavior)
	var hits atomic.Int32
	srv := stubLLM(t, "followed", &hits)
	defer srv.Close()
	emb := &sequenceEmbedder{targetSim: 0.15}
	oc := newBypassClassifier(emb, 0.10, false, srv.URL)
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "action text")
	if cr.Outcome != OutcomeIgnored {
		t.Errorf("bypass OFF: [naThreshold, low) should return tier1 Ignored; got %s", cr.Outcome)
	}
	if cr.Source != "tier1" {
		t.Errorf("expected tier1 source; got %s", cr.Source)
	}
	if hits.Load() != 0 {
		t.Errorf("LLM should NOT be called with bypass OFF; hits=%d", hits.Load())
	}
}

func TestTier1Bypass_BandOn_RoutesToLLM(t *testing.T) {
	// bypass ON: [naThreshold, lowThreshold) falls through to LLM tier2
	var hits atomic.Int32
	srv := stubLLM(t, "followed", &hits)
	defer srv.Close()
	emb := &sequenceEmbedder{targetSim: 0.15}
	oc := newBypassClassifier(emb, 0.10, true, srv.URL)
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "action text")
	if cr.Source != "llm" {
		t.Errorf("bypass ON: [naThreshold, low) should route to LLM; got source %s outcome %s", cr.Source, cr.Outcome)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 LLM call; got %d", hits.Load())
	}
	if cr.Outcome != OutcomeFollowed {
		t.Errorf("LLM returned 'followed'; got outcome %s", cr.Outcome)
	}
}

func TestTier1Bypass_HighNoNegOff_TerminalFollowed(t *testing.T) {
	// bypass OFF: sim ≥ highThreshold && !negation → tier1 Followed (legacy)
	var hits atomic.Int32
	srv := stubLLM(t, "ignored", &hits)
	defer srv.Close()
	emb := &sequenceEmbedder{targetSim: 0.80}
	oc := newBypassClassifier(emb, 0.10, false, srv.URL)
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "action text")
	if cr.Outcome != OutcomeFollowed {
		t.Errorf("bypass OFF + HIGH sim + no negation should return tier1 Followed; got %s", cr.Outcome)
	}
	if cr.Source != "tier1" {
		t.Errorf("expected tier1 source; got %s", cr.Source)
	}
	if hits.Load() != 0 {
		t.Errorf("LLM should NOT be called with bypass OFF; hits=%d", hits.Load())
	}
}

func TestTier1Bypass_HighNoNegOn_RoutesToLLM(t *testing.T) {
	// bypass ON: sim ≥ highThreshold falls through to LLM tier2
	var hits atomic.Int32
	srv := stubLLM(t, "ignored", &hits)
	defer srv.Close()
	emb := &sequenceEmbedder{targetSim: 0.80}
	oc := newBypassClassifier(emb, 0.10, true, srv.URL)
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}
	cr := oc.Classify(context.Background(), item, "action text")
	if cr.Source != "llm" {
		t.Errorf("bypass ON: HIGH sim should route to LLM; got source %s outcome %s", cr.Source, cr.Outcome)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 LLM call; got %d", hits.Load())
	}
	if cr.Outcome != OutcomeIgnored {
		t.Errorf("LLM returned 'ignored'; got %s", cr.Outcome)
	}
}

func TestTier1Bypass_MiddleBand_LLMUnchanged(t *testing.T) {
	// Middle band [lowThreshold, highThreshold): both flag values route to LLM (unchanged)
	var hitsOff, hitsOn atomic.Int32
	srvOff := stubLLM(t, "partial_compliance", &hitsOff)
	defer srvOff.Close()
	srvOn := stubLLM(t, "partial_compliance", &hitsOn)
	defer srvOn.Close()
	emb := &sequenceEmbedder{targetSim: 0.40}
	item := GuidanceItem{Content: "test rule", Type: GuidanceConstraint}

	off := newBypassClassifier(emb, 0.10, false, srvOff.URL)
	crOff := off.Classify(context.Background(), item, "action text")
	if crOff.Source != "llm" {
		t.Errorf("middle band should route to LLM (bypass OFF); got source %s", crOff.Source)
	}
	if hitsOff.Load() != 1 {
		t.Errorf("expected 1 LLM call (OFF); got %d", hitsOff.Load())
	}

	on := newBypassClassifier(emb, 0.10, true, srvOn.URL)
	crOn := on.Classify(context.Background(), item, "action text")
	if crOn.Source != "llm" {
		t.Errorf("middle band should route to LLM (bypass ON); got source %s", crOn.Source)
	}
	if hitsOn.Load() != 1 {
		t.Errorf("expected 1 LLM call (ON); got %d", hitsOn.Load())
	}
}

// ensure test file compiles when llmclient package is unused (defensive)
var _ = llmclient.Client{}
