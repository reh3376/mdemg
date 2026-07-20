package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mdemg/internal/conversation"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// JIMINY-CONTRADICTED-BRIDGE-001 Epic 4 — sink Apply/Preview/Reverse pins.

// mockCorrectService captures Correct invocations.
type mockCorrectService struct {
	called  bool
	lastReq conversation.CorrectRequest
	resp    *conversation.ObserveResponse
	err     error
}

func (m *mockCorrectService) Correct(_ context.Context, req conversation.CorrectRequest) (*conversation.ObserveResponse, error) {
	m.called = true
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

// The sink's writer-facing calls are covered by writer-level tests; the sink
// tests below use a nil writer for Preview (which never touches it) and
// exercise only the code paths whose absence would change behavior.
func TestDraftVerdict_Table(t *testing.T) {
	cases := []struct {
		name string
		dim  map[string]any
		want string
	}{
		{"missing", map[string]any{}, ""},
		{"score_4_approve", map[string]any{"durable_rule": 4}, "approve"},
		{"score_3_approve", map[string]any{"durable_rule": 3}, "approve"},
		{"score_2_defer", map[string]any{"durable_rule": 2}, ""},
		{"score_1_dismiss", map[string]any{"durable_rule": 1}, "dismiss"},
		{"score_0_dismiss", map[string]any{"durable_rule": 0}, "dismiss"},
		{"float_coerced", map[string]any{"durable_rule": float64(3)}, "approve"},
		{"int64_coerced", map[string]any{"durable_rule": int64(0)}, "dismiss"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := review.Grade{GoldDimensions: tc.dim}
			if got := draftVerdict(g); got != tc.want {
				t.Errorf("draftVerdict(%v) = %q, want %q", tc.dim, got, tc.want)
			}
		})
	}
}

func TestContradictedDraftsSink_Preview_Approve(t *testing.T) {
	s := contradictedDraftsSink{svc: &mockCorrectService{}}
	g := review.Grade{
		GradeID:        "grade-1",
		ItemID:         "draft-1",
		GoldDimensions: map[string]any{"durable_rule": 4},
		Item: review.ReviewItem{
			Meta: map[string]string{
				"draft_incorrect": "the action that violated",
				"draft_correct":   "the rule to follow",
			},
		},
	}
	p, err := s.Preview(context.Background(), g)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if p.Summary == "" {
		t.Error("Preview approve returned empty Summary")
	}
	if p.Detail.Verb != "correction_draft:approve" {
		t.Errorf("Verb = %q, want correction_draft:approve", p.Detail.Verb)
	}
	// Preview must NOT touch the mock (no substrate mutation).
	if s.svc.(*mockCorrectService).called {
		t.Error("Preview must not call CorrectService")
	}
}

func TestContradictedDraftsSink_Preview_Dismiss(t *testing.T) {
	s := contradictedDraftsSink{svc: &mockCorrectService{}}
	g := review.Grade{GradeID: "g1", ItemID: "d1", GoldDimensions: map[string]any{"durable_rule": 0}}
	p, _ := s.Preview(context.Background(), g)
	if p.Detail.Verb != "correction_draft:dismiss" {
		t.Errorf("Verb = %q, want correction_draft:dismiss", p.Detail.Verb)
	}
}

func TestContradictedDraftsSink_Preview_Unclear(t *testing.T) {
	s := contradictedDraftsSink{}
	g := review.Grade{GoldDimensions: map[string]any{"durable_rule": 2}}
	p, _ := s.Preview(context.Background(), g)
	if p.Summary == "" {
		t.Error("Preview unclear returned empty Summary")
	}
	// No Verb set on unclear branch (defer).
	if p.Detail.Verb != "" {
		t.Errorf("unclear Preview set Verb = %q, want empty", p.Detail.Verb)
	}
}

func TestContradictedDraftsSink_Apply_NoWriterErrors(t *testing.T) {
	s := contradictedDraftsSink{}
	g := review.Grade{GradeID: "g1", ItemID: "d1", GoldDimensions: map[string]any{"durable_rule": 4}}
	_, err := s.Apply(context.Background(), g)
	if err == nil || !strings.Contains(err.Error(), "writer not wired") {
		t.Errorf("expected writer-not-wired error, got %v", err)
	}
}

func TestContradictedDraftsSink_Apply_ApproveWithoutSvcErrors(t *testing.T) {
	// Writer non-nil (would need a real DB or a mock), but we test the
	// early-return: no CorrectService → error before touching the writer.
	s := contradictedDraftsSink{svc: nil, writer: nonNilWriter()}
	g := review.Grade{GradeID: "g1", ItemID: "d1", GoldDimensions: map[string]any{"durable_rule": 4}}
	_, err := s.Apply(context.Background(), g)
	if err == nil || !strings.Contains(err.Error(), "CorrectService not wired") {
		t.Errorf("expected CorrectService-not-wired error, got %v", err)
	}
}

func TestContradictedDraftsSink_Apply_ApproveSurfacesCorrectError(t *testing.T) {
	svc := &mockCorrectService{err: errors.New("neo4j unavailable")}
	s := contradictedDraftsSink{svc: svc, writer: nonNilWriter()}
	g := review.Grade{
		GradeID: "g1", ItemID: "d1",
		GoldDimensions: map[string]any{"durable_rule": 4},
		Item: review.ReviewItem{Meta: map[string]string{
			"draft_incorrect": "wrong", "draft_correct": "right",
		}},
	}
	_, err := s.Apply(context.Background(), g)
	if err == nil || !strings.Contains(err.Error(), "neo4j unavailable") {
		t.Errorf("Apply must propagate CorrectService error; got %v", err)
	}
	if !svc.called {
		t.Error("Apply must call CorrectService on approve")
	}
	if svc.lastReq.Incorrect != "wrong" || svc.lastReq.Correct != "right" {
		t.Errorf("Apply passed wrong Incorrect/Correct: got Incorrect=%q Correct=%q",
			svc.lastReq.Incorrect, svc.lastReq.Correct)
	}
}

func TestContradictedDraftsSink_Apply_Unclear_NoSvcCall(t *testing.T) {
	svc := &mockCorrectService{}
	s := contradictedDraftsSink{svc: svc, writer: nonNilWriter()}
	g := review.Grade{GradeID: "g1", ItemID: "d1", GoldDimensions: map[string]any{"durable_rule": 2}}
	d, err := s.Apply(context.Background(), g)
	if err != nil {
		t.Fatalf("Apply on unclear grade should not error: %v", err)
	}
	if svc.called {
		t.Error("Apply on unclear grade must not call CorrectService")
	}
	if d.Verb != "noop:unclear" {
		t.Errorf("Verb = %q, want noop:unclear", d.Verb)
	}
}

// nonNilWriter returns a zero-value *tsdb.ContradictedDraftsWriter — the
// sink's nil-check passes, but any actual method invocation would panic on
// the nil internal pool. Tests using this MUST hit code paths that never
// reach a writer method (CorrectService error, dismiss verb no-op, unclear
// verdict). Live-writer paths are tested in internal/tsdb (writer tests).
func nonNilWriter() *tsdb.ContradictedDraftsWriter {
	return &tsdb.ContradictedDraftsWriter{}
}
