package conversation

import (
	"testing"
)

// JIMINY-STRUCTURED-CORRECTION-001 Epic 1 — buildCorrectionObserveRequest
// contract: the built ObserveRequest carries the correction fields both in
// the joined content string (backward compat) AND in structured_data
// (the graph-persisted first-class shape).

func TestBuildCorrectionObserveRequest_StructuredDataPopulated(t *testing.T) {
	req := CorrectRequest{
		SpaceID:   "mdemg-dev",
		SessionID: "sess-1",
		Incorrect: "committed directly to main",
		Correct:   "always use a dev branch + PR",
		Context:   "git workflow rule",
	}
	got := buildCorrectionObserveRequest(req)
	sd, ok := got.StructuredData["correction"].(map[string]any)
	if !ok {
		t.Fatalf("StructuredData[correction] missing or wrong type: %#v", got.StructuredData)
	}
	if sd["incorrect"] != "committed directly to main" {
		t.Errorf("incorrect: got %v, want the request value", sd["incorrect"])
	}
	if sd["correct"] != "always use a dev branch + PR" {
		t.Errorf("correct: got %v, want the request value", sd["correct"])
	}
	if sd["context"] != "git workflow rule" {
		t.Errorf("context: got %v, want the request value", sd["context"])
	}
}

func TestBuildCorrectionObserveRequest_ContentBackwardCompatible(t *testing.T) {
	req := CorrectRequest{
		SpaceID:   "s",
		Incorrect: "A",
		Correct:   "B",
		Context:   "C",
	}
	got := buildCorrectionObserveRequest(req)
	want := "CORRECTION: Incorrect: A | Correct: B | Context: C"
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

func TestBuildCorrectionObserveRequest_ContentOmitsEmptyContext(t *testing.T) {
	req := CorrectRequest{Incorrect: "A", Correct: "B"} // no Context
	got := buildCorrectionObserveRequest(req)
	want := "CORRECTION: Incorrect: A | Correct: B"
	if got.Content != want {
		t.Errorf("Content = %q, want %q (must not have trailing | Context: )", got.Content, want)
	}
	// Even with empty context, structured_data still carries an empty string
	// under the key — downstream can distinguish "no context provided" from
	// "context missing entirely" by presence of the correction sub-object.
	sd := got.StructuredData["correction"].(map[string]any)
	if sd["context"] != "" {
		t.Errorf("context: got %v, want empty string when not supplied", sd["context"])
	}
}

func TestBuildCorrectionObserveRequest_MetadataPreservedForAudit(t *testing.T) {
	// JIMINY-STRUCTURED-CORRECTION-001 keeps the Metadata map on the request
	// even though it's NOT graph-persisted (the metadata_* flatten in
	// createObservationNode was retired dead code). The Metadata is still
	// useful for downstream audit/logging paths that consume ObserveRequest
	// directly.
	req := CorrectRequest{Incorrect: "X", Correct: "Y"}
	got := buildCorrectionObserveRequest(req)
	if got.Metadata["incorrect"] != "X" || got.Metadata["correct"] != "Y" {
		t.Errorf("Metadata not preserved: %#v", got.Metadata)
	}
}

func TestBuildCorrectionObserveRequest_ObsTypeAndTagsPin(t *testing.T) {
	got := buildCorrectionObserveRequest(CorrectRequest{Incorrect: "x", Correct: "y"})
	if got.ObsType != string(ObsTypeCorrection) {
		t.Errorf("ObsType = %q, want %q", got.ObsType, ObsTypeCorrection)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "correction" {
		t.Errorf("Tags = %v, want [correction]", got.Tags)
	}
}

func TestBuildCorrectionObserveRequest_AgentIDPropagates(t *testing.T) {
	// Regression pin: the multi-agent AgentID must flow through the helper.
	got := buildCorrectionObserveRequest(CorrectRequest{
		Incorrect: "x", Correct: "y", AgentID: "agent-beta",
	})
	if got.AgentID != "agent-beta" {
		t.Errorf("AgentID = %q, want agent-beta", got.AgentID)
	}
}
