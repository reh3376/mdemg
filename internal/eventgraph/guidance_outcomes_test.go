package eventgraph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Tier 1 — validation + join + JSON-contract logic in isolation. The full
// Cypher + TSDB orchestration is covered by the integration test
// (guidance_outcomes_integration_test.go, -tags=integration).

func TestGuidanceOutcome_RejectsEmptySpaceID(t *testing.T) {
	s := &Service{} // driver + pool nil; validation runs before either is touched
	_, err := s.GuidanceOutcomesInNeighborhood(context.Background(), GuidanceOutcomeRequest{
		SeedNodeID: "x", Hops: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "space_id") {
		t.Fatalf("expected space_id error, got %v", err)
	}
}

func TestGuidanceOutcome_RejectsEmptySeed(t *testing.T) {
	s := &Service{}
	_, err := s.GuidanceOutcomesInNeighborhood(context.Background(), GuidanceOutcomeRequest{
		SpaceID: "mdemg-dev", Hops: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "seed_node_id") {
		t.Fatalf("expected seed_node_id error, got %v", err)
	}
}

func TestGuidanceOutcome_RejectsNegativeHops(t *testing.T) {
	s := &Service{}
	_, err := s.GuidanceOutcomesInNeighborhood(context.Background(), GuidanceOutcomeRequest{
		SpaceID: "mdemg-dev", SeedNodeID: "x", Hops: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "hops") {
		t.Fatalf("expected hops error, got %v", err)
	}
}

// TestGuidanceOutcomeResult_EmptyArraysNotNull pins the JSON contract: all three
// array fields serialize as [] (never null) when empty. Same lesson as
// EVENTGRAPH-CLI-001 — an unknown seed / codeless neighborhood must not emit null.
func TestGuidanceOutcomeResult_EmptyArraysNotNull(t *testing.T) {
	r := GuidanceOutcomeResult{
		Outcomes:                []GuidanceOutcomeWithContext{},
		NeighborNodeIDs:         []string{},
		NeighborConstraintCodes: []string{},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, field := range []string{"outcomes", "neighbor_node_ids", "neighbor_constraint_codes"} {
		if strings.Contains(s, `"`+field+`":null`) || !strings.Contains(s, `"`+field+`":[]`) {
			t.Errorf("%s must serialize as [] not null: %s", field, s)
		}
	}
}

func TestSortedKeys_DeterministicOrder(t *testing.T) {
	m := map[string]string{"zebra": "n1", "alpha": "n2", "mike": "n3"}
	got := sortedKeys(m)
	want := []string{"alpha", "mike", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(sortedKeys(map[string]string{})) != 0 {
		t.Error("empty map should yield empty slice")
	}
}

// annotateGuidanceOutcomes mirrors step 3 of GuidanceOutcomesInNeighborhood —
// resolving each outcome's code → neighborhood node — for direct testing
// without a live driver.
func annotateGuidanceOutcomes(outcomes []GuidanceOutcomeWithContext, codeToNode map[string]string) []GuidanceOutcomeWithContext {
	out := make([]GuidanceOutcomeWithContext, len(outcomes))
	for i, o := range outcomes {
		o.ConstraintNodeID = codeToNode[o.ConstraintCode]
		o.InNeighborhood = true
		out[i] = o
	}
	return out
}

func TestGuidanceOutcome_JoinResolvesNode(t *testing.T) {
	codeToNode := map[string]string{
		"no-direct-main-commits":          "n_aaa",
		"mandatory-use-cms-every-session": "n_bbb",
	}
	outcomes := []GuidanceOutcomeWithContext{
		{ConstraintCode: "no-direct-main-commits", OutcomeType: "followed"},
		{ConstraintCode: "mandatory-use-cms-every-session", OutcomeType: "ignored"},
		{ConstraintCode: "unmapped-code", OutcomeType: "followed"}, // not in neighborhood map
	}
	out := annotateGuidanceOutcomes(outcomes, codeToNode)
	if out[0].ConstraintNodeID != "n_aaa" || !out[0].InNeighborhood {
		t.Errorf("outcome 0 should resolve to n_aaa in-neighborhood, got %+v", out[0])
	}
	if out[1].ConstraintNodeID != "n_bbb" {
		t.Errorf("outcome 1 should resolve to n_bbb, got %q", out[1].ConstraintNodeID)
	}
	if out[2].ConstraintNodeID != "" {
		t.Errorf("unmapped code should resolve to empty node id, got %q", out[2].ConstraintNodeID)
	}
}
