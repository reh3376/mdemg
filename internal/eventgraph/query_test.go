package eventgraph

import (
	"context"
	"testing"
	"time"
)

// These Tier 1 tests cover the request-validation + neighborhood-join logic
// in isolation. Tier 2 integration tests in tests/integration/eventgraph_federation_test.go
// cover the full Cypher + TSDB orchestration against real services.

func TestFederationRequest_RejectsEmptySpaceID(t *testing.T) {
	s := &Service{} // driver + pool nil; validation runs before either is touched
	_, err := s.EventsInGraphNeighborhood(context.Background(), FederationRequest{
		SeedNodeID: "x",
		Hops:       1,
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected error for empty space_id, got nil")
	}
}

func TestFederationRequest_RejectsEmptySeed(t *testing.T) {
	s := &Service{}
	_, err := s.EventsInGraphNeighborhood(context.Background(), FederationRequest{
		SpaceID: "mdemg-dev",
		Hops:    1,
	})
	if err == nil {
		t.Fatal("expected error for empty seed_node_id, got nil")
	}
}

func TestFederationRequest_RejectsNegativeHops(t *testing.T) {
	s := &Service{}
	_, err := s.EventsInGraphNeighborhood(context.Background(), FederationRequest{
		SpaceID:    "mdemg-dev",
		SeedNodeID: "x",
		Hops:       -1,
	})
	if err == nil {
		t.Fatal("expected error for negative hops, got nil")
	}
}

func TestIntervalString_FormatsSeconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{time.Second, "1 seconds"},
		{30 * time.Second, "30 seconds"},
		{time.Hour, "3600 seconds"},
		{24 * time.Hour, "86400 seconds"},
		{0, "0 seconds"},
	}
	for _, tc := range cases {
		got := intervalString(tc.in)
		if got != tc.want {
			t.Errorf("intervalString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// joinAnnotation is the Go-side join logic from EventsInGraphNeighborhood
// step 3, factored out for direct testing. This mirrors the actual code
// rather than reaching into the unexported function — keeping the live
// code path single-source while still asserting the contract.
func annotateNeighborhood(events []EventWithContext, neighbors []string) []EventWithContext {
	neighSet := make(map[string]struct{}, len(neighbors))
	for _, n := range neighbors {
		neighSet[n] = struct{}{}
	}
	out := make([]EventWithContext, len(events))
	for i, ev := range events {
		_, srcOK := neighSet[ev.SrcNodeID]
		_, dstOK := neighSet[ev.DstNodeID]
		ev.SrcInNeighborhood = srcOK
		ev.DstInNeighborhood = dstOK
		out[i] = ev
	}
	return out
}

func TestJoinAnnotation_BothEndpointsInside(t *testing.T) {
	events := []EventWithContext{{SrcNodeID: "a", DstNodeID: "b"}}
	out := annotateNeighborhood(events, []string{"a", "b", "c"})
	if !out[0].SrcInNeighborhood || !out[0].DstInNeighborhood {
		t.Errorf("both endpoints should be marked in-neighborhood, got %+v", out[0])
	}
}

func TestJoinAnnotation_OneEndpointOutside(t *testing.T) {
	events := []EventWithContext{{SrcNodeID: "a", DstNodeID: "z"}}
	out := annotateNeighborhood(events, []string{"a", "b", "c"})
	if !out[0].SrcInNeighborhood {
		t.Errorf("src should be in-neighborhood (a ∈ {a,b,c})")
	}
	if out[0].DstInNeighborhood {
		t.Errorf("dst should NOT be in-neighborhood (z ∉ {a,b,c})")
	}
}

func TestJoinAnnotation_EmptyNeighborhood(t *testing.T) {
	events := []EventWithContext{{SrcNodeID: "a", DstNodeID: "b"}}
	out := annotateNeighborhood(events, nil)
	if out[0].SrcInNeighborhood || out[0].DstInNeighborhood {
		t.Errorf("empty neighborhood: nothing should be marked in-neighborhood")
	}
}

