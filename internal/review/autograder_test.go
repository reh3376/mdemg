package review

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// HITL-CURATION-002 E1 unit tests.

// stubLLM implements LLMGrader for tests — returns a fixed response or an error.
type stubLLM struct {
	response string
	err      error
	lastSys  string
	lastUsr  string
	lastMax  int
	calls    int
}

func (s *stubLLM) CompleteJSON(_ context.Context, sys, usr string, maxTokens int) (string, error) {
	s.calls++
	s.lastSys = sys
	s.lastUsr = usr
	s.lastMax = maxTokens
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}

func newTestRubric() Rubric {
	return Rubric{
		Version: "test-v1",
		Kind:    RubricRated,
		Dimensions: []RubricDimension{
			{Key: "durable_rule", Anchors: [5]string{"a", "b", "c", "d", "e"}},
			{Key: "phrasing_quality", Anchors: [5]string{"a", "b", "c", "d", "e"}},
		},
	}
}

func TestNewAutograder_NilLLM_ReturnsNil(t *testing.T) {
	if got := NewAutograder(AutograderConfig{LLM: nil}); got != nil {
		t.Fatalf("expected nil for nil LLM, got %+v", got)
	}
}

func TestAutograder_GraderID_HasAutoPrefixAndModelBinary(t *testing.T) {
	ag := NewAutograder(AutograderConfig{
		LLM: &stubLLM{}, ModelID: "mdemg-llm-v1", BinarySHA: "abc1234",
	})
	got := ag.GraderID()
	if !strings.HasPrefix(got, AutoGraderPrefix) {
		t.Errorf("grader_id must start with %q; got %q", AutoGraderPrefix, got)
	}
	if !strings.Contains(got, "mdemg-llm-v1") || !strings.Contains(got, "abc1234") {
		t.Errorf("grader_id must embed model + binary sha; got %q", got)
	}
}

func TestAutograder_Grade_HighConfidence_Ok(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":4,"phrasing_quality":3},"confidence":0.92,"rationale":"clear rule"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm, MinConfidence: 0.8})
	res, ok, err := ag.Grade(context.Background(), "contradicted_drafts",
		ReviewItem{ItemID: "x1", Content: "never do X", Context: "did X"}, newTestRubric())
	if err != nil {
		t.Fatalf("grade err: %v", err)
	}
	if !ok {
		t.Fatalf("expected confidence-pass, got not-ok (conf=%v)", res.Confidence)
	}
	if res.Submission.DatasetID != "contradicted_drafts" || res.Submission.ItemID != "x1" {
		t.Fatalf("submission mislabeled: %+v", res.Submission)
	}
	if res.Submission.Dimensions["durable_rule"] != 4 || res.Submission.Dimensions["phrasing_quality"] != 3 {
		t.Fatalf("dims mis-parsed: %+v", res.Submission.Dimensions)
	}
	// Prompt hygiene checks (system prompt names the rubric and its anchors).
	if !strings.Contains(llm.lastSys, "test-v1") {
		t.Errorf("system prompt should carry rubric version; got: %s", llm.lastSys)
	}
	if !strings.Contains(llm.lastSys, "durable_rule") {
		t.Errorf("system prompt should carry dimension keys; got: %s", llm.lastSys)
	}
}

func TestAutograder_Grade_LowConfidence_LeavesPending(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":2,"phrasing_quality":2},"confidence":0.55,"rationale":"unclear"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm, MinConfidence: 0.8})
	res, ok, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err != nil {
		t.Fatalf("grade err: %v", err)
	}
	if ok {
		t.Errorf("expected confidence-fail below 0.8; got ok with conf=%v", res.Confidence)
	}
	// Result is still well-formed — the caller can inspect + leave-pending.
	if res.Submission.Dimensions["durable_rule"] != 2 {
		t.Errorf("dim parse should still work: %+v", res.Submission.Dimensions)
	}
}

func TestAutograder_Grade_MarkdownFencedJSON_Ok(t *testing.T) {
	// Some models wrap JSON in ```json … ``` fences. Autograder must strip.
	llm := &stubLLM{response: "```json\n" + `{"dimensions":{"durable_rule":3,"phrasing_quality":3},"confidence":0.85,"rationale":"ok"}` + "\n```"}
	ag := NewAutograder(AutograderConfig{LLM: llm, MinConfidence: 0.8})
	if _, ok, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric()); err != nil || !ok {
		t.Fatalf("fenced JSON should parse ok; err=%v ok=%v", err, ok)
	}
}

func TestAutograder_Grade_MissingDimension_Errors(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":3},"confidence":0.9,"rationale":"missing dim"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err == nil {
		t.Fatalf("expected error on missing rubric dimension")
	}
	if !strings.Contains(err.Error(), "phrasing_quality") {
		t.Errorf("error should name the missing dimension; got: %v", err)
	}
}

func TestAutograder_Grade_OutOfRangeScore_Errors(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":7,"phrasing_quality":3},"confidence":0.9,"rationale":"bad"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error; got: %v", err)
	}
}

func TestAutograder_Grade_OutOfRangeConfidence_Errors(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":3,"phrasing_quality":3},"confidence":1.5,"rationale":"bad"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err == nil || !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("expected confidence-range error; got: %v", err)
	}
}

func TestAutograder_Grade_InvalidJSON_Errors(t *testing.T) {
	llm := &stubLLM{response: "not json at all"}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err == nil {
		t.Fatalf("expected JSON parse error")
	}
	if !strings.Contains(err.Error(), "raw=") {
		t.Errorf("error should include a truncated raw payload for debugging; got: %v", err)
	}
}

func TestAutograder_Grade_LLMError_Propagates(t *testing.T) {
	llm := &stubLLM{err: errors.New("connection refused")}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, newTestRubric())
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected llm error to propagate; got: %v", err)
	}
}

func TestAutograder_Grade_RankedRubric_Rejected(t *testing.T) {
	// DPO/ranked rubrics need a different prompt shape (chosen/rejected) —
	// HITL-REVIEW-002 scope. This sprint rejects explicitly.
	ranked := Rubric{Version: "dpo-v1", Kind: RubricRanked}
	ag := NewAutograder(AutograderConfig{LLM: &stubLLM{}})
	_, _, err := ag.Grade(context.Background(), "d", ReviewItem{ItemID: "x"}, ranked)
	if err == nil || !strings.Contains(err.Error(), "rated") {
		t.Fatalf("expected ranked-rubric rejection; got: %v", err)
	}
}

// TestAutograder_Grade_Item_ContentAndContextInPrompt pins that both fields
// reach the model (else the grader is blind to the offending-action half of
// a contradicted-draft).
func TestAutograder_Grade_Item_ContentAndContextInPrompt(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":3,"phrasing_quality":3},"confidence":0.9,"rationale":"ok"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.Grade(context.Background(), "d",
		ReviewItem{ItemID: "x", Content: "CORRECT_TEXT", Context: "INCORRECT_TEXT"}, newTestRubric())
	if err != nil {
		t.Fatalf("grade err: %v", err)
	}
	if !strings.Contains(llm.lastUsr, "CORRECT_TEXT") || !strings.Contains(llm.lastUsr, "INCORRECT_TEXT") {
		t.Errorf("user prompt must include both content and context; got: %s", llm.lastUsr)
	}
}

// TestAutograder_GraderID_InvariantPrefix pins the "auto:" prefix — the alert
// rules and dashboards read it to separate auto- from operator-grades. Changing
// this prefix is a breaking change.
func TestAutograder_GraderID_InvariantPrefix(t *testing.T) {
	if AutoGraderPrefix != "auto:" {
		t.Fatalf("AutoGraderPrefix invariant broken; must remain %q, got %q", "auto:", AutoGraderPrefix)
	}
	ag := NewAutograder(AutograderConfig{LLM: &stubLLM{}, ModelID: "any", BinarySHA: "x"})
	if !strings.HasPrefix(ag.GraderID(), "auto:") {
		t.Fatalf("grader_id lost the invariant prefix: %q", ag.GraderID())
	}
}

// TestAutograder_GradeWithHint_SplicesHint pins that a non-empty dataset
// hint reaches the system prompt in the "Dataset-specific guidance:" block
// (E1 prompt tuning after the 2026-07-27 dry-run showed the model needed
// typology guidance the rubric anchors alone couldn't provide).
func TestAutograder_GradeWithHint_SplicesHint(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":3,"phrasing_quality":3},"confidence":0.9,"rationale":"ok"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	hint := "UNIQUE_HINT_TOKEN — do not conflate logs with rules."
	_, _, err := ag.GradeWithHint(context.Background(), "d",
		ReviewItem{ItemID: "x", Content: "c"}, newTestRubric(), hint)
	if err != nil {
		t.Fatalf("grade err: %v", err)
	}
	if !strings.Contains(llm.lastSys, "Dataset-specific guidance:") {
		t.Errorf("system prompt must carry the guidance block header; got: %s", llm.lastSys)
	}
	if !strings.Contains(llm.lastSys, "UNIQUE_HINT_TOKEN") {
		t.Errorf("system prompt must splice the hint verbatim; got: %s", llm.lastSys)
	}
}

// TestAutograder_GradeWithHint_EmptyHint_NoBlockSection pins that an empty
// hint is a no-op (no leading "Dataset-specific guidance:" block).
func TestAutograder_GradeWithHint_EmptyHint_NoBlockSection(t *testing.T) {
	llm := &stubLLM{response: `{"dimensions":{"durable_rule":3,"phrasing_quality":3},"confidence":0.9,"rationale":"ok"}`}
	ag := NewAutograder(AutograderConfig{LLM: llm})
	_, _, err := ag.GradeWithHint(context.Background(), "d",
		ReviewItem{ItemID: "x", Content: "c"}, newTestRubric(), "")
	if err != nil {
		t.Fatalf("grade err: %v", err)
	}
	if strings.Contains(llm.lastSys, "Dataset-specific guidance:") {
		t.Errorf("empty hint must not add the guidance block; got: %s", llm.lastSys)
	}
}
