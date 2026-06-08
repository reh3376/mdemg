package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// EVENTGRAPH-CLI-001 Tier 1: flag→request mapping, --query seed resolution,
// render, and error handling — without a live server (httptest).

func TestEventgraph_SeedRequired(t *testing.T) {
	err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{spaceID: "sp"})
	if err == nil || !strings.Contains(err.Error(), "seed is required") {
		t.Fatalf("expected seed-required error, got %v", err)
	}
}

func TestEventgraph_RequestOmitsUnsetDefaults(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/eventgraph/reinforcement-neighborhood" {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			_, _ = w.Write([]byte(`{"events":[],"neighbor_node_ids":[],"graph_hops":0,"tsdb_rows_scanned":0,"truncated":false}`))
		}
	}))
	defer srv.Close()
	t.Setenv("MDEMG_URL", srv.URL)

	// Unset hops/since/limit → must be omitted (server applies its defaults).
	if err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{
		spaceID: "sp", seed: "n_abc", hops: -1, jsonOutput: true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := gotBody["hops"]; ok {
		t.Errorf("unset hops should be omitted, got %v", gotBody["hops"])
	}
	if _, ok := gotBody["since_seconds"]; ok {
		t.Errorf("unset since should be omitted")
	}
	if _, ok := gotBody["limit"]; ok {
		t.Errorf("unset limit should be omitted")
	}
	if gotBody["seed_node_id"] != "n_abc" || gotBody["space_id"] != "sp" {
		t.Errorf("required fields wrong: %v", gotBody)
	}

	// Set them → must be present + correctly converted.
	gotBody = nil
	if err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{
		spaceID: "sp", seed: "n_abc", hops: 2, since: "24h", limit: 50, jsonOutput: true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["hops"].(float64) != 2 {
		t.Errorf("hops = %v, want 2", gotBody["hops"])
	}
	if gotBody["since_seconds"].(float64) != 86400 {
		t.Errorf("since_seconds = %v, want 86400 (24h)", gotBody["since_seconds"])
	}
	if gotBody["limit"].(float64) != 50 {
		t.Errorf("limit = %v, want 50", gotBody["limit"])
	}
}

func TestEventgraph_InvalidSince(t *testing.T) {
	t.Setenv("MDEMG_URL", "http://127.0.0.1:0") // unused; error happens before HTTP
	err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{spaceID: "sp", seed: "n_abc", since: "notaduration"})
	if err == nil || !strings.Contains(err.Error(), "invalid --since") {
		t.Fatalf("expected invalid --since error, got %v", err)
	}
}

func TestEventgraph_QueryResolvesSeed(t *testing.T) {
	var fedSeed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/memory/retrieve":
			_, _ = w.Write([]byte(`{"results":[{"node_id":"n_from_query"}]}`))
		case "/v1/eventgraph/reinforcement-neighborhood":
			b, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(b, &body)
			fedSeed, _ = body["seed_node_id"].(string)
			_, _ = w.Write([]byte(`{"events":[],"neighbor_node_ids":[],"graph_hops":1,"tsdb_rows_scanned":0,"truncated":false}`))
		}
	}))
	defer srv.Close()
	t.Setenv("MDEMG_URL", srv.URL)

	if err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{
		spaceID: "sp", query: "circuit breaker", jsonOutput: true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fedSeed != "n_from_query" {
		t.Errorf("--query should resolve seed to n_from_query, federation got %q", fedSeed)
	}
}

func TestEventgraph_QueryNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	t.Setenv("MDEMG_URL", srv.URL)
	err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{spaceID: "sp", query: "nothing matches"})
	if err == nil || !strings.Contains(err.Error(), "no retrieval results") {
		t.Fatalf("expected no-results error, got %v", err)
	}
}

func TestEventgraph_ErrorStatusSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"eventgraph disabled","reason":"EVENTGRAPH_ENABLED=false"}`))
	}))
	defer srv.Close()
	t.Setenv("MDEMG_URL", srv.URL)
	err := runReinforcementNeighborhood(reinforcementNeighborhoodOpts{spaceID: "sp", seed: "n_abc"})
	if err == nil || !strings.Contains(err.Error(), "eventgraph disabled") {
		t.Fatalf("expected surfaced 503 error, got %v", err)
	}
}

func TestEventgraph_Helpers(t *testing.T) {
	if shortNodeID("n_0123456789abcdefghij") != "n_0123456789ab" {
		t.Errorf("shortNodeID truncation wrong: %q", shortNodeID("n_0123456789abcdefghij"))
	}
	if shortNodeID("short") != "short" {
		t.Errorf("shortNodeID should pass short IDs through")
	}
	if boolMark(true) != "✓" || boolMark(false) != "·" {
		t.Errorf("boolMark wrong")
	}
}

func TestEventgraph_RenderEmptyAndTable(t *testing.T) {
	// Empty → friendly message.
	if out := captureStdout(func() { printFedResult(fedResult{NeighborNodeIDs: []string{"a", "b"}, GraphHops: 1}, "n_seed") }); !strings.Contains(out, "No reinforcement events") {
		t.Errorf("empty render missing message: %q", out)
	}
	// With events → table row.
	res := fedResult{
		NeighborNodeIDs: []string{"a", "b", "c"}, GraphHops: 2, TSDBRowsScanned: 5,
		Events: []fedEvent{{SrcNodeID: "n_src", DstNodeID: "n_dst", DeltaWeight: 0.0083, NewWeight: 0.18, Direction: "bidirectional", CreatedNewEdge: true, SrcInNeighborhood: true, DstInNeighborhood: true}},
	}
	out := captureStdout(func() { printFedResult(res, "n_seed") })
	if !strings.Contains(out, "n_src") || !strings.Contains(out, "n_dst") || !strings.Contains(out, "neighborhood: 3 nodes") {
		t.Errorf("table render missing expected content: %q", out)
	}
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}
