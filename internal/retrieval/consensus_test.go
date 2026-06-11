package retrieval

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// fakeColumn is a Column implementation that returns a precomputed result.
// Used to drive the aggregator under controlled inputs (no Neo4j needed).
type fakeColumn struct {
	name string
	res  ColumnResult
}

func (f *fakeColumn) Name() string                                      { return f.name }
func (f *fakeColumn) Run(_ context.Context, _ ColumnQuery) ColumnResult { return f.res }

// candList is a tiny helper to build a ranked Candidate slice.
func candList(ids ...string) []Candidate {
	out := make([]Candidate, len(ids))
	for i, id := range ids {
		out[i] = Candidate{NodeID: id, Path: "p/" + id, Name: id}
	}
	return out
}

// TestAggregate_NoColumnsErrorsCleanly verifies the early-exit guard.
func TestAggregate_NoColumnsErrorsCleanly(t *testing.T) {
	res, err := Aggregate(context.Background(), nil, ColumnQuery{}, ConsensusOpts{})
	if err == nil {
		t.Error("expected error on zero columns")
	}
	if len(res.Ranked) != 0 {
		t.Errorf("Ranked must be empty when zero columns, got %d", len(res.Ranked))
	}
}

// TestAggregate_AllColumnsAgreeYieldsHighConsensus verifies the unanimous
// case: every column ranks the same node #1 → consensus_strength near 1.0.
func TestAggregate_AllColumnsAgreeYieldsHighConsensus(t *testing.T) {
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("a", "b", "c")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("a", "b", "c")}},
		&fakeColumn{name: "graph", res: ColumnResult{Column: "graph", Candidates: candList("a", "b", "c")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 3}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if len(res.Ranked) != 3 {
		t.Errorf("Ranked len=%d, want 3", len(res.Ranked))
	}
	if res.Ranked[0].NodeID != "a" {
		t.Errorf("top=%q, want a", res.Ranked[0].NodeID)
	}
	if res.AggregateConsensus < 0.65 { // 1.0 (coverage) × ~0.78 (avg normalized rank for 3-of-3) ≈ 0.78
		t.Errorf("AggregateConsensus=%.3f, want ≥ 0.65 for unanimous agreement", res.AggregateConsensus)
	}
	if res.ConsensusByNode["a"] < 0.95 { // Top-1 in all 3 cols = coverage 1.0 × avg-rank 1.0
		t.Errorf("ConsensusByNode[a]=%.3f, want ≥ 0.95 (top in all 3)", res.ConsensusByNode["a"])
	}
}

// TestAggregate_DisjointColumnsYieldLowConsensus verifies the worst case:
// each column returns disjoint sets, so consensus per node = 1/N coverage.
func TestAggregate_DisjointColumnsYieldLowConsensus(t *testing.T) {
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("x1", "x2")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("y1", "y2")}},
		&fakeColumn{name: "graph", res: ColumnResult{Column: "graph", Candidates: candList("z1", "z2")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 6}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	// Each node appears in exactly 1 column → coverage = 1/3.
	for nodeID, cs := range res.ConsensusByNode {
		coverage := 1.0 / 3.0
		// Avg normalized rank ≤ 1.0; consensus = coverage × avgNormRank ≤ 1/3
		if cs > coverage+0.01 {
			t.Errorf("ConsensusByNode[%q]=%.3f, want ≤ %.3f for disjoint columns", nodeID, cs, coverage)
		}
	}
}

// TestAggregate_FailedColumnLowersConsensus verifies that a failed column
// is counted in the denominator (so consensus DROPS) rather than silently
// skipped.
func TestAggregate_FailedColumnLowersConsensus(t *testing.T) {
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("a", "b")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("a", "b")}},
		&fakeColumn{name: "graph", res: ColumnResult{Column: "graph", Err: errors.New("graph backend down")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 5}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if res.ColumnsQueried != 3 {
		t.Errorf("ColumnsQueried=%d, want 3 (failed col counts in denominator)", res.ColumnsQueried)
	}
	if res.ColumnsReturned != 2 {
		t.Errorf("ColumnsReturned=%d, want 2 (only successful cols return)", res.ColumnsReturned)
	}
	if res.PerColumnError["graph"] == nil {
		t.Error("PerColumnError[graph] should be non-nil")
	}
	// Node "a" is in 2 of 3 columns → coverage = 2/3, max consensus ≤ 2/3.
	csA := res.ConsensusByNode["a"]
	if csA > 2.0/3.0+0.01 {
		t.Errorf("ConsensusByNode[a]=%.3f, want ≤ 0.67 (2 of 3 cols agreed; failed col lowers consensus)", csA)
	}
	if csA < 0.5 {
		t.Errorf("ConsensusByNode[a]=%.3f, want ≥ 0.5 (top-1 in 2 surviving cols)", csA)
	}
}

// TestAggregate_RankInfluencesRRF verifies that being ranked higher in any
// column produces a higher RRF score — basic correctness check on the
// fusion formula.
func TestAggregate_RankInfluencesRRF(t *testing.T) {
	// Both columns have nodes a, b. In col1, a is #1, b is #2. In col2, b
	// is #1, a is #2. With equal weights and equal column counts, RRF
	// totals: a = 1/(60+1) + 1/(60+2) = ~0.0327, same for b. They tie and
	// stable-sort breaks to alphabetical (a then b).
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("a", "b")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("b", "a")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 2}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if len(res.Ranked) != 2 {
		t.Fatalf("Ranked len=%d, want 2", len(res.Ranked))
	}
	// Tie + stable-sort breaks alphabetical → a first, b second.
	if res.Ranked[0].NodeID != "a" || res.Ranked[1].NodeID != "b" {
		t.Errorf("ranking=[%q, %q], want [a, b] (RRF tie + alphabetical stable-sort)", res.Ranked[0].NodeID, res.Ranked[1].NodeID)
	}
	// Now skew: a is #1 in both. Should clearly beat b.
	cols[1] = &fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("a", "b")}}
	res, err = Aggregate(context.Background(), cols, ColumnQuery{TopN: 2}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if res.Ranked[0].NodeID != "a" {
		t.Errorf("top=%q, want a (#1 in both columns)", res.Ranked[0].NodeID)
	}
	if res.Ranked[0].RRFScore <= res.Ranked[1].RRFScore {
		t.Errorf("a's RRF (%.4f) should beat b's (%.4f)", res.Ranked[0].RRFScore, res.Ranked[1].RRFScore)
	}
}

// TestAggregate_ColumnWeightsRespected verifies that explicit per-column
// weights influence the final ranking.
func TestAggregate_ColumnWeightsRespected(t *testing.T) {
	// Embedding ranks "a" #1, "b" #2; BM25 ranks "b" #1, "a" #2. With
	// embedding weight 10× larger, "a" should dominate.
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("a", "b")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("b", "a")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 2}, ConsensusOpts{
		ColumnWeights: map[string]float64{"embedding": 10.0, "bm25": 1.0},
	})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if res.Ranked[0].NodeID != "a" {
		t.Errorf("top=%q, want a (embedding weight=10× bm25)", res.Ranked[0].NodeID)
	}
}

// TestAggregate_ZeroWeightExcludesColumn verifies that weight=0 effectively
// excludes a column from the RRF sum.
func TestAggregate_ZeroWeightExcludesColumn(t *testing.T) {
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Candidates: candList("a")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Candidates: candList("z")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 2}, ConsensusOpts{
		ColumnWeights: map[string]float64{"embedding": 1.0, "bm25": 0.0},
	})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	// "a" has positive weighted score; "z" has score=0.
	var aScore, zScore float64
	for _, c := range res.Ranked {
		if c.NodeID == "a" {
			aScore = c.RRFScore
		}
		if c.NodeID == "z" {
			zScore = c.RRFScore
		}
	}
	if aScore <= 0 {
		t.Errorf("a RRFScore=%.4f, want > 0 (embedding weight=1)", aScore)
	}
	if math.Abs(zScore) > 1e-9 {
		t.Errorf("z RRFScore=%.4f, want 0 (bm25 weight=0)", zScore)
	}
}

// TestAggregate_PerColumnLatencyAlwaysReturned verifies the latency map
// has an entry for every attempted column (success or fail).
func TestAggregate_PerColumnLatencyAlwaysReturned(t *testing.T) {
	cols := []Column{
		&fakeColumn{name: "embedding", res: ColumnResult{Column: "embedding", Latency: 5 * time.Millisecond, Candidates: candList("a")}},
		&fakeColumn{name: "bm25", res: ColumnResult{Column: "bm25", Latency: 7 * time.Millisecond, Err: errors.New("oops")}},
	}
	res, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 2}, ConsensusOpts{})
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	if _, ok := res.PerColumnLatency["embedding"]; !ok {
		t.Error("PerColumnLatency missing 'embedding'")
	}
	if _, ok := res.PerColumnLatency["bm25"]; !ok {
		t.Error("PerColumnLatency missing 'bm25' (failed col still records latency)")
	}
}

// TestColumnWeight_DefaultsToOne verifies the default weight contract.
func TestColumnWeight_DefaultsToOne(t *testing.T) {
	o := ConsensusOpts{}
	if w := o.ColumnWeight("anything"); w != 1.0 {
		t.Errorf("default ColumnWeight=%.2f, want 1.0", w)
	}
	o2 := ConsensusOpts{ColumnWeights: map[string]float64{"explicit": 0.7}}
	if w := o2.ColumnWeight("missing"); w != 1.0 {
		t.Errorf("missing key ColumnWeight=%.2f, want 1.0 default", w)
	}
	if w := o2.ColumnWeight("explicit"); w != 0.7 {
		t.Errorf("explicit ColumnWeight=%.2f, want 0.7", w)
	}
	o3 := ConsensusOpts{ColumnWeights: map[string]float64{"neg": -1.5}}
	if w := o3.ColumnWeight("neg"); w != 0 {
		t.Errorf("negative weight=%.2f, want clamp to 0", w)
	}
}

// TestAggregate_ParallelExecutionSpeedup verifies that running 4 columns
// each taking 50ms completes in ~50ms (parallel) rather than ~200ms
// (serial). Sanity-check on the errgroup wiring.
func TestAggregate_ParallelExecutionSpeedup(t *testing.T) {
	makeSlow := func(name string) Column {
		return &slowColumn{name: name, sleep: 50 * time.Millisecond, cands: candList("x")}
	}
	cols := []Column{makeSlow("c1"), makeSlow("c2"), makeSlow("c3"), makeSlow("c4")}
	start := time.Now()
	_, err := Aggregate(context.Background(), cols, ColumnQuery{TopN: 5}, ConsensusOpts{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Aggregate err=%v", err)
	}
	// Parallel: should be ~50ms; serial would be ~200ms.
	if elapsed > 150*time.Millisecond {
		t.Errorf("Aggregate took %v with 4 parallel 50ms cols — looks serial (want <150ms)", elapsed)
	}
}

// slowColumn deliberately sleeps inside Run() to exercise the parallel path.
type slowColumn struct {
	name  string
	sleep time.Duration
	cands []Candidate
}

func (s *slowColumn) Name() string { return s.name }
func (s *slowColumn) Run(ctx context.Context, _ ColumnQuery) ColumnResult {
	start := time.Now()
	select {
	case <-time.After(s.sleep):
	case <-ctx.Done():
	}
	return ColumnResult{Column: s.name, Candidates: s.cands, Latency: time.Since(start)}
}
