package retrieval

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// ConsensusOpts tunes the aggregator. Zero values fall back to plan defaults.
type ConsensusOpts struct {
	// RRFK is the Reciprocal Rank Fusion constant (`score = w / (k + rank)`).
	// Default 60 (matches RetrievalConfig.RRFConstant + Note 04 §4 standard).
	// Higher k = flatter contribution from low ranks; lower k = sharper.
	RRFK int

	// PerColumnTimeoutFraction caps each column's wall-clock as a fraction
	// of the parent ctx deadline. 0.8 leaves 20% headroom for the aggregator
	// itself + downstream rerank/scoring. Plan §13 default.
	PerColumnTimeoutFraction float64

	// ColumnWeights map column-name → weight in the RRF sum. Missing columns
	// default to 1.0 (equal weights). Phase 13 v1 ships equal weights;
	// ablation deferred to Phase 13.1.
	ColumnWeights map[string]float64

	// TopN caps the final ranked output. Aggregator passes the same value
	// to each column's [ColumnQuery.TopN] so all columns return enough to
	// disagree before truncation.
	TopN int
}

// ConsensusResult bundles the aggregator's output. The ranked Candidates +
// per-node ConsensusByNode are the canonical outputs; AggregateConsensus is
// the headline single-number signal exposed to downstream consumers
// (rerank, DH-005 confidence, the retrieval response).
type ConsensusResult struct {
	// Ranked is the final ranked Candidate slice (best first), capped at
	// the requested TopN. Each entry's RRFScore is the sum of per-column
	// weighted RRF contributions.
	Ranked []Candidate

	// ConsensusByNode[node_id] = (cols_with_node / cols_queried) ×
	// avg(normalized_rank), clipped to [0, 1]. Higher = more agreement
	// across columns. Surfaced on retrieval response per-node so callers
	// can gate expensive downstream work on uncertainty.
	ConsensusByNode map[string]float64

	// AggregateConsensus is the mean ConsensusByNode across the Ranked
	// set's top-N. The single-number signal Phase 14 (Notes 05+06) and
	// the DH-005 retrieval-confidence dimension consume.
	AggregateConsensus float64

	// ColumnsQueried is the count of columns the aggregator attempted —
	// the consensus denominator. Includes columns that errored or timed
	// out; excludes nil columns.
	ColumnsQueried int

	// ColumnsReturned is the count of columns that produced ≥1 candidate.
	// Useful diagnostic for Grafana ("are we falling back to fewer
	// columns?").
	ColumnsReturned int

	// PerColumnLatency[name] is the wall-clock spent in each column's
	// Run(). Populated for every column attempted (success or fail).
	// Aggregator records each value into the per-column histogram.
	PerColumnLatency map[string]time.Duration

	// PerColumnError[name] is the column's failure (nil on success).
	// Aggregator increments mdemg_retrieval_column_failed_total{column,
	// reason} for each non-nil entry.
	PerColumnError map[string]error
}

// Aggregate runs the given columns in parallel against the query, fuses
// their rankings via Reciprocal Rank Fusion, and returns the merged
// candidates + a `consensus_strength` signal per node + an aggregate.
//
// The aggregator NEVER blocks indefinitely: each column gets a per-column
// timeout (PerColumnTimeoutFraction × parent ctx remaining). A timed-out
// column is excluded from RRF but counted in the consensus denominator —
// missing columns *lower* consensus rather than silently inflating it.
//
// Phase 13 — Note 04 Column-Voting Retrieval, the heart of the new ranker.
func Aggregate(ctx context.Context, cols []Column, q ColumnQuery, opts ConsensusOpts) (ConsensusResult, error) {
	if len(cols) == 0 {
		return ConsensusResult{
			ConsensusByNode:  map[string]float64{},
			PerColumnLatency: map[string]time.Duration{},
			PerColumnError:   map[string]error{},
		}, errors.New("consensus aggregate: no columns provided")
	}
	rrfK := opts.RRFK
	if rrfK <= 0 {
		rrfK = 60
	}
	perColTimeoutFrac := opts.PerColumnTimeoutFraction
	if perColTimeoutFrac <= 0 || perColTimeoutFrac > 1 {
		perColTimeoutFrac = 0.8
	}
	topN := opts.TopN
	if topN <= 0 {
		topN = q.TopN
	}
	if topN <= 0 {
		topN = 50
	}

	// Per-column ctx: child of parent with a fraction of remaining deadline.
	colCtx := ctx
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining > 0 {
			colTimeout := time.Duration(float64(remaining) * perColTimeoutFrac)
			var cancel context.CancelFunc
			colCtx, cancel = context.WithTimeout(ctx, colTimeout)
			defer cancel()
		}
	}

	// Run all columns in parallel.
	results := make([]ColumnResult, len(cols))
	g, gCtx := errgroup.WithContext(colCtx)
	var resultsMu sync.Mutex
	for i, col := range cols {
		i, col := i, col
		if col == nil {
			continue
		}
		g.Go(func() error {
			r := col.Run(gCtx, q)
			resultsMu.Lock()
			results[i] = r
			resultsMu.Unlock()
			// Column errors are non-fatal; aggregator surfaces them via
			// PerColumnError but never aborts the whole fan-out.
			return nil
		})
	}
	_ = g.Wait()

	// Fuse via RRF, build consensus_strength per node, and rank.
	out := ConsensusResult{
		ConsensusByNode:  map[string]float64{},
		PerColumnLatency: make(map[string]time.Duration, len(results)),
		PerColumnError:   make(map[string]error, len(results)),
	}

	// First pass: gather metadata, count columns, build per-column rank maps.
	type rankInfo struct {
		rank    int // 1-indexed position in the column's output (1 = best)
		maxRank int // total nodes the column returned (used for normalized rank)
	}
	// rankByNode[node_id][column_name] -> rank info
	rankByNode := map[string]map[string]rankInfo{}
	// canonical metadata captured from whichever column saw the node first
	metaByNode := map[string]Candidate{}

	colsAttempted := 0
	colsReturned := 0
	for i, r := range results {
		if cols[i] == nil {
			continue
		}
		colsAttempted++
		out.PerColumnLatency[r.Column] = r.Latency
		if r.Err != nil {
			out.PerColumnError[r.Column] = r.Err
			// Failed column counted in denominator (lowers consensus) but
			// produces no per-node rank entries.
			continue
		}
		if len(r.Candidates) > 0 {
			colsReturned++
		}
		for rank, cand := range r.Candidates {
			byCol, ok := rankByNode[cand.NodeID]
			if !ok {
				byCol = map[string]rankInfo{}
				rankByNode[cand.NodeID] = byCol
			}
			byCol[r.Column] = rankInfo{
				rank:    rank + 1,
				maxRank: len(r.Candidates),
			}
			if _, seen := metaByNode[cand.NodeID]; !seen {
				// First time we've seen metadata for this node — capture
				// it for the final output. Later columns may have richer
				// metadata; an Phase 13.1 follow-up could merge fields.
				metaByNode[cand.NodeID] = cand
			}
		}
	}
	out.ColumnsQueried = colsAttempted
	out.ColumnsReturned = colsReturned

	if colsAttempted == 0 {
		return out, errors.New("consensus aggregate: zero columns attempted")
	}

	// Second pass: compute RRF score per node + per-node consensus.
	// RRF: score(node) = Σ (weight_c / (rrfK + rank_c(node)))
	// Consensus: (cols_with_node / cols_queried) × avg(1 - (rank-1)/maxRank)
	type scored struct {
		nodeID    string
		rrfScore  float64
		consensus float64
	}
	all := make([]scored, 0, len(rankByNode))
	for nodeID, byCol := range rankByNode {
		var rrfSum float64
		var normRankSum float64
		for colName, ri := range byCol {
			weight := opts.ColumnWeight(colName)
			rrfSum += weight / float64(rrfK+ri.rank)
			// Normalized rank: 1.0 for top, → 0 as we approach maxRank.
			// Captures *how confidently* this column ranked the node.
			if ri.maxRank > 0 {
				normRankSum += 1.0 - float64(ri.rank-1)/float64(ri.maxRank)
			}
		}
		// Consensus per node:
		nCols := len(byCol)
		coverage := float64(nCols) / float64(colsAttempted)
		var avgNormRank float64
		if nCols > 0 {
			avgNormRank = normRankSum / float64(nCols)
		}
		consensus := coverage * avgNormRank
		if consensus < 0 {
			consensus = 0
		}
		if consensus > 1 {
			consensus = 1
		}
		all = append(all, scored{nodeID: nodeID, rrfScore: rrfSum, consensus: consensus})
		out.ConsensusByNode[nodeID] = consensus
	}

	// Sort by RRF score descending; stable on nodeID for determinism.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].rrfScore != all[j].rrfScore {
			return all[i].rrfScore > all[j].rrfScore
		}
		return all[i].nodeID < all[j].nodeID
	})

	// Truncate to topN, build final []Candidate carrying RRF score in the
	// existing RRFScore field so downstream consumers can read uniformly.
	limit := topN
	if limit > len(all) {
		limit = len(all)
	}
	ranked := make([]Candidate, 0, limit)
	var consensusSum float64
	for _, s := range all[:limit] {
		c := metaByNode[s.nodeID]
		c.NodeID = s.nodeID
		c.RRFScore = s.rrfScore
		ranked = append(ranked, c)
		consensusSum += s.consensus
	}
	out.Ranked = ranked
	if limit > 0 {
		out.AggregateConsensus = consensusSum / float64(limit)
	}

	return out, nil
}

// ColumnWeight returns the weight for the given column name. Defaults to 1.0
// when no weight is set (equal-weights regime, Phase 13 v1).
func (o ConsensusOpts) ColumnWeight(name string) float64 {
	if o.ColumnWeights == nil {
		return 1.0
	}
	w, ok := o.ColumnWeights[name]
	if !ok {
		return 1.0
	}
	if w < 0 {
		return 0
	}
	return w
}
