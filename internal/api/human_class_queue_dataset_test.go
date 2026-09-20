package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mdemg/internal/review"
)

// JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (Q5 §3 #2) — sink pins.
//
// Tier 1 unit-tests for the human_class_queue sink logic. Verify:
//   - `followed` dim → outcome enum mapping
//   - Auto-grader is rejected at Preview + Apply with a named error
//   - dim==2 defer produces the no-op detail without touching the writer
//
// The dataset's FetchCandidates / FetchItem SQL is integration-tested
// against real TSDB in E4 live-smoke, not here (raw SQL over the shared
// pool — pattern mirrors HITL-CURATION-003 guidance dataset).

func TestHumanClassOutcomeFor(t *testing.T) {
	cases := []struct {
		name string
		val  int
		want string
	}{
		{"clearly-violated 0", 0, "ignored"},
		{"partially-violated 1", 1, "ignored"},
		{"unclear 2 -> defer", 2, ""},
		{"partially-followed 3", 3, "followed"},
		{"clearly-followed 4", 4, "followed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := humanClassOutcomeFor(review.Grade{
				GoldDimensions: map[string]any{"followed": tc.val},
			})
			if got != tc.want {
				t.Errorf("dim=%d: got %q want %q", tc.val, got, tc.want)
			}
		})
	}
}

func TestHumanClassOutcomeFor_MissingDim(t *testing.T) {
	// No `followed` key → return "" (defer). Prevents accidental writes on
	// a grade payload missing the primary dimension.
	got := humanClassOutcomeFor(review.Grade{GoldDimensions: map[string]any{}})
	if got != "" {
		t.Errorf("missing dim must defer, got %q", got)
	}
}

// TestHumanClassSink_AutograderRejected — the load-bearing invariant.
// Both Preview and Apply MUST refuse any grade whose grader_id begins
// with review.AutoGraderPrefix. Enforced at the sink so an operator
// misinvoking `mdemg review autograde --dataset human_class_queue` gets a
// named error instead of silent substrate pollution.
func TestHumanClassSink_AutograderRejected(t *testing.T) {
	sink := humanClassQueueSink{writer: nil} // writer irrelevant — reject fires first
	autoGrade := review.Grade{
		GradeID:        "g-1",
		GraderID:       review.AutoGraderPrefix + "gpt-5.4-mini@sha",
		GoldDimensions: map[string]any{"followed": 4}, // even a "followed" verdict must reject
	}

	if _, err := sink.Preview(context.Background(), autoGrade); !errors.Is(err, errAutograderRejected) {
		t.Errorf("Preview: want errAutograderRejected, got %v", err)
	}
	if _, err := sink.Apply(context.Background(), autoGrade); !errors.Is(err, errAutograderRejected) {
		t.Errorf("Apply: want errAutograderRejected, got %v", err)
	}

	// The error message MUST name the taxonomy source so an operator
	// looking at logs learns WHY the sink refuses.
	if err := errAutograderRejected; !strings.Contains(err.Error(), "JIMINY-METRIC-DENOMINATOR-DESIGN-001") {
		t.Errorf("rejection error must cite taxonomy source, got %q", err.Error())
	}
}

// TestHumanClassSink_HumanGraderAllowedOnDeferPath — an operator-authored
// dim==2 defer path returns detail with no substrate write.
func TestHumanClassSink_HumanGraderAllowedOnDeferPath(t *testing.T) {
	sink := humanClassQueueSink{writer: nil}
	deferGrade := review.Grade{
		GradeID:        "g-defer",
		GraderID:       "operator:reh3376",
		GoldDimensions: map[string]any{"followed": 2},
	}
	// Preview
	prev, err := sink.Preview(context.Background(), deferGrade)
	if err != nil {
		t.Fatalf("Preview defer: unexpected err %v", err)
	}
	if prev.Detail.Verb != "human_outcome:defer" {
		t.Errorf("defer verb: got %q want human_outcome:defer", prev.Detail.Verb)
	}
	// Apply (writer=nil; if the sink tried to write it would panic — the
	// defer path MUST short-circuit before touching the writer)
	det, err := sink.Apply(context.Background(), deferGrade)
	if err != nil {
		t.Fatalf("Apply defer: unexpected err %v", err)
	}
	if det.Verb != "human_outcome:defer" {
		t.Errorf("defer detail verb: got %q want human_outcome:defer", det.Verb)
	}
}

// TestHumanClassSink_SinkIDStable — SinkID is the audit key persisted in
// review_grades.reinforcement_detail; a rename here would break audit-log
// filters. Regression-pin.
func TestHumanClassSink_SinkIDStable(t *testing.T) {
	if got := (humanClassQueueSink{}).SinkID(); got != "human_class_queue" {
		t.Errorf("SinkID = %q want human_class_queue", got)
	}
}

// TestHumanClassSink_ReverseIsNoOp — constraint_outcomes rows are append-
// only telemetry; Reverse pins to a no-op so a downstream refactor that
// tries to DELETE substrate rows on reverse gets caught here.
func TestHumanClassSink_ReverseIsNoOp(t *testing.T) {
	err := humanClassQueueSink{}.Reverse(context.Background(), review.ReinforcementDetail{})
	if err != nil {
		t.Errorf("Reverse must be no-op, got %v", err)
	}
}
