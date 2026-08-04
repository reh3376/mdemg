package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// HITL-AUTO-DISMISS-001 — pin tests for the NonReinforcingApplier interface
// contract on the contradicted_drafts sink.

// mockContradictedWriter captures which sink methods fire, in what order.
type mockContradictedWriter struct {
	markDismissedCalls []string
	markApprovedCalls  []string
	resetToPendingIDs  []string
	markDismissedErr   error
}

func (m *mockContradictedWriter) MarkApproved(_ context.Context, id, _, _ string) error {
	m.markApprovedCalls = append(m.markApprovedCalls, id)
	return nil
}
func (m *mockContradictedWriter) MarkDismissed(_ context.Context, id string) error {
	m.markDismissedCalls = append(m.markDismissedCalls, id)
	return m.markDismissedErr
}
func (m *mockContradictedWriter) ResetToPending(_ context.Context, id string) error {
	m.resetToPendingIDs = append(m.resetToPendingIDs, id)
	return nil
}
func (m *mockContradictedWriter) FetchPendingBySpace(_ context.Context, _ string, _ int) ([]tsdb.ContradictedDraftRow, error) {
	return nil, nil
}
func (m *mockContradictedWriter) FetchByID(_ context.Context, _ string) (*tsdb.ContradictedDraftRow, error) {
	return nil, nil
}

func autoGrade(itemID string, durableRule int) review.Grade {
	return review.Grade{
		GradeID:   "g-auto-" + itemID,
		DatasetID: "contradicted_drafts",
		ItemID:    itemID,
		SpaceID:   "test-space",
		GraderID:  "auto:mdemg-llm-v1@test",
		Item: review.ReviewItem{
			ItemID: itemID,
			Meta:   map[string]string{"status": "pending"},
		},
		GoldDimensions: map[string]any{
			"durable_rule":     durableRule,
			"phrasing_quality": 2,
		},
	}
}

// TestContradictedSinkImplementsNonReinforcingApplier — interface contract pin.
// If a refactor removes this capability, the auto-dismiss path silently reverts
// to "no-op on reinforce=false" and the alert regressess.
func TestContradictedSinkImplementsNonReinforcingApplier(t *testing.T) {
	var s any = contradictedDraftsSink{}
	if _, ok := s.(review.NonReinforcingApplier); !ok {
		t.Error("contradictedDraftsSink MUST implement review.NonReinforcingApplier — the auto-dismiss capability depends on this")
	}
}

// Dim=0 → dismiss verdict → MarkDismissed called, handled=true.
func TestApplyNonReinforcing_DismissVerdictHandlesAndUpdatesStatus(t *testing.T) {
	w := &mockContradictedWriter{}
	s := contradictedDraftsSink{writer: w}
	detail, handled, err := s.ApplyNonReinforcing(context.Background(), autoGrade("draft-noise", 0))
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Error("dim=0 (dismiss) MUST return handled=true — this is the whole point of the sprint")
	}
	if len(w.markDismissedCalls) != 1 || w.markDismissedCalls[0] != "draft-noise" {
		t.Errorf("MarkDismissed must fire exactly once for draft-noise, got %v", w.markDismissedCalls)
	}
	if !strings.HasSuffix(detail.Verb, ":auto") {
		t.Errorf(":auto suffix on verb is the auditability marker for non-reinforcing writes, got %q", detail.Verb)
	}
	if len(w.markApprovedCalls) != 0 {
		t.Errorf("MarkApproved must NEVER fire from ApplyNonReinforcing (would mutate substrate via conversation.Correct)")
	}
}

// Dim=4 → approve verdict → REFUSED (returns handled=false, no mutation).
// This is the invariant guardrail: approve = substrate mutation, requires
// operator grade + reinforce=true.
func TestApplyNonReinforcing_ApproveVerdictRefusedForInvariant(t *testing.T) {
	w := &mockContradictedWriter{}
	s := contradictedDraftsSink{writer: w}
	_, handled, err := s.ApplyNonReinforcing(context.Background(), autoGrade("draft-real-rule", 4))
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Error("dim=4 (approve) MUST return handled=false under ApplyNonReinforcing — approve mutates the substrate via conversation.Correct")
	}
	if len(w.markDismissedCalls) != 0 || len(w.markApprovedCalls) != 0 {
		t.Errorf("NO writer method should fire for dim=4 under ApplyNonReinforcing; got Dismissed=%v Approved=%v",
			w.markDismissedCalls, w.markApprovedCalls)
	}
}

// Dim=2 → defer verdict → returns handled=false, no mutation.
func TestApplyNonReinforcing_DeferVerdictReturnsFalse(t *testing.T) {
	w := &mockContradictedWriter{}
	s := contradictedDraftsSink{writer: w}
	_, handled, err := s.ApplyNonReinforcing(context.Background(), autoGrade("draft-borderline", 2))
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Error("dim=2 (defer) MUST return handled=false — leaves draft pending for operator")
	}
	if len(w.markDismissedCalls) != 0 || len(w.markApprovedCalls) != 0 {
		t.Errorf("no writer method should fire for dim=2, got Dismissed=%v Approved=%v",
			w.markDismissedCalls, w.markApprovedCalls)
	}
}

// Nil writer → clean error (not panic).
func TestApplyNonReinforcing_NilWriterErrors(t *testing.T) {
	s := contradictedDraftsSink{writer: nil}
	_, _, err := s.ApplyNonReinforcing(context.Background(), autoGrade("x", 0))
	if err == nil {
		t.Error("nil writer must return an error, not panic or silently succeed")
	}
}

// Writer error surfaces cleanly with handled=false.
func TestApplyNonReinforcing_WriterErrorPropagates(t *testing.T) {
	w := &mockContradictedWriter{markDismissedErr: errors.New("db: connection refused")}
	s := contradictedDraftsSink{writer: w}
	_, handled, err := s.ApplyNonReinforcing(context.Background(), autoGrade("x", 0))
	if err == nil {
		t.Error("writer error must propagate")
	}
	if handled {
		t.Error("handled must be false on writer error — nothing durable happened")
	}
}
