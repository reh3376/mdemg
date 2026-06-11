package jiminy

import (
	"strings"
	"testing"
)

// JIMINY-OUTCOME-002: the Tier-2 surface must OFFER not_applicable. The type
// system, parser (mapOutcomeString), and persistence path (service.go skips
// not_applicable at all four sinks) already handled it — only the LLM was
// never given the option, so guidance that couldn't apply to an action was
// scored "ignored" (the post-SUPERVISOR-002 effectiveness-floor artifact).
func TestClassifyPrompts_OfferNotApplicable(t *testing.T) {
	for name, prompt := range map[string]string{
		"full":    classifySystemPrompt,
		"compact": classifySystemPromptCompact,
	} {
		if !strings.Contains(prompt, "not_applicable") {
			t.Errorf("%s prompt does not offer not_applicable", name)
		}
		if !strings.Contains(prompt, "ignored") {
			t.Errorf("%s prompt lost the ignored option", name)
		}
	}
	if !strings.Contains(string(ollamaClassifySchema), `"not_applicable"`) {
		t.Error("ollamaClassifySchema enum missing not_applicable")
	}
}

// Verdict provenance (JIMINY-OUTCOME-002): every Classify decision path must
// stamp Source so fallback-derived rows (the artifact class) are forever
// distinguishable from real verdicts in constraint_outcomes.classifier_source.
func TestClassify_SourceStamped(t *testing.T) {
	// nil embedder → text-overlap heuristic
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{})
	cr := oc.Classify(t.Context(), GuidanceItem{Content: "use CUIDv2 for identifiers"}, "edited file: added a helper")
	if cr.Source != "heuristic" {
		t.Errorf("nil-embedder path Source = %q, want heuristic", cr.Source)
	}

	// tier1 low-similarity band → not_applicable with Source tier1
	ocEmb := NewOutcomeClassifier(&sequenceEmbedder{targetSim: 0.05}, OutcomeClassifierConfig{})
	cr = ocEmb.Classify(t.Context(), GuidanceItem{Content: "guidance"}, "action")
	if cr.Outcome != OutcomeNotApplicable || cr.Source != "tier1" {
		t.Errorf("low-band = (%v, %q), want (not_applicable, tier1)", cr.Outcome, cr.Source)
	}

	// tier1 high-similarity band → followed with Source tier1
	ocSame := NewOutcomeClassifier(&sequenceEmbedder{targetSim: 0.9}, OutcomeClassifierConfig{})
	cr = ocSame.Classify(t.Context(), GuidanceItem{Content: "guidance"}, "action")
	if cr.Outcome != OutcomeFollowed || cr.Source != "tier1" {
		t.Errorf("high-band = (%v, %q), want (followed, tier1)", cr.Outcome, cr.Source)
	}
}
