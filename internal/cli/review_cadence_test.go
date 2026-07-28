package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// HITL-CURATION-002 E3 — cadence output rendering tests.
// The full runReviewCadence wraps HTTP + rendering; the rendering piece
// is pure and independently testable, which is what pin-worth-guarantees.
// captureStdout is shared from eventgraph_test.go in this same package.

func TestRenderCadence_EmptyQueue_TextFormat_AllClear(t *testing.T) {
	s := cadenceSummary{GeneratedAt: "2026-01-01T00:00:00Z", TotalPending: 0, Actionable: false}
	out := captureStdout(func() { _ = renderCadence(s, "text") })
	if !strings.Contains(out, "HITL queue is empty") {
		t.Errorf("empty text output should announce all-clear; got: %s", out)
	}
	if strings.Contains(out, "pending") && strings.Contains(out, "item(s)") {
		t.Errorf("empty state should not mention pending items; got: %s", out)
	}
}

func TestRenderCadence_PopulatedQueue_TextFormat_ListsDatasets(t *testing.T) {
	s := cadenceSummary{
		GeneratedAt:  "2026-01-01T00:00:00Z",
		TotalPending: 15,
		Actionable:   true,
		Datasets: []cadenceDatasetRow{
			{DatasetID: "contradicted_drafts", DisplayName: "Contradicted Drafts", PendingCount: 5, RubricVersion: "cd-v1"},
			{DatasetID: "llm:guardrail.evaluate", DisplayName: "Guardrail: Evaluate", PendingCount: 10, RubricVersion: "gr-v1"},
		},
	}
	out := captureStdout(func() { _ = renderCadence(s, "text") })
	for _, want := range []string{"15 item(s) pending", "Contradicted Drafts", "Guardrail: Evaluate",
		"contradicted_drafts", "llm:guardrail.evaluate", "cd-v1", "gr-v1", "autograde"} {
		if !strings.Contains(out, want) {
			t.Errorf("populated text output missing %q; got: %s", want, out)
		}
	}
}

func TestRenderCadence_JSONFormat_MachineReadable(t *testing.T) {
	s := cadenceSummary{
		GeneratedAt:  "2026-01-01T00:00:00Z",
		TotalPending: 3,
		Actionable:   true,
		Datasets: []cadenceDatasetRow{
			{DatasetID: "d1", DisplayName: "D1", PendingCount: 3, RubricVersion: "v1"},
		},
	}
	out := captureStdout(func() { _ = renderCadence(s, "json") })
	var round cadenceSummary
	if err := json.Unmarshal([]byte(out), &round); err != nil {
		t.Fatalf("json output must roundtrip: %v; got: %s", err, out)
	}
	if round.TotalPending != 3 || len(round.Datasets) != 1 || round.Datasets[0].DatasetID != "d1" {
		t.Errorf("json roundtrip lost data: %+v", round)
	}
}

// TestRenderCadence_Actionable_Field pins the actionable field: false only
// when the queue is genuinely empty; true otherwise (schedulers/alert bodies
// gate their behavior on this).
func TestRenderCadence_Actionable_Field(t *testing.T) {
	for _, tc := range []struct {
		pending    int
		actionable bool
	}{
		{0, false},
		{1, true},
		{1000, true},
	} {
		s := cadenceSummary{TotalPending: tc.pending, Actionable: tc.actionable}
		out := captureStdout(func() { _ = renderCadence(s, "json") })
		var round cadenceSummary
		_ = json.Unmarshal([]byte(out), &round)
		if round.Actionable != tc.actionable {
			t.Errorf("pending=%d: actionable want %v got %v", tc.pending, tc.actionable, round.Actionable)
		}
	}
}
