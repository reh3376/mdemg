package retrieval

import (
	"context"
	"errors"
	"sort"
	"time"
)

// GraphColumn is the topological-relatedness column. It seeds from a small
// vector-recall set and propagates activation across CO_ACTIVATED_WITH (and
// other learning) edges, returning the spread set as its ranked output.
//
// Phase 13 — Note 04 Column-Voting Retrieval. Distinct from EmbeddingColumn:
// graph activation surfaces nodes the user *connects to in practice* (Hebbian
// co-activation), even when their embedding similarity is moderate. The
// aggregator's `consensus_strength` rises when both columns rank the same
// node highly.
//
// Implementation note: this column re-runs vector recall internally to seed
// itself. The cost is paid once per column (parallel with the other columns
// that also do vector recall). Avoiding this duplication would require
// orchestrator-side seed sharing — a Phase 13.1 follow-up.
type GraphColumn struct {
	svc *Service
	// SeedK caps the vector-recall seed set used to bootstrap activation.
	// Smaller = faster + tighter spread; larger = broader recall but more
	// edge-fetch latency. Default 50 (mirrors RetrievalConfig.SeedN).
	SeedK int
}

// NewGraphColumn constructs a GraphColumn with sensible defaults.
func NewGraphColumn(svc *Service) *GraphColumn {
	return &GraphColumn{svc: svc, SeedK: 50}
}

// Name implements [Column].
func (c *GraphColumn) Name() string { return "graph" }

// Run implements [Column]. The column is a self-contained mini-pipeline:
// vector-recall → edge-fetch → SpreadingActivation → rank by activation.
func (c *GraphColumn) Run(ctx context.Context, q ColumnQuery) ColumnResult {
	start := time.Now()
	res := ColumnResult{Column: c.Name()}

	if c.svc == nil {
		res.Err = errors.New("graph column: nil Service")
		res.Latency = time.Since(start)
		return res
	}
	if len(q.QueryEmbedding) == 0 {
		// Without an embedding we can't seed activation. Surface as
		// success-empty so the aggregator just excludes us.
		res.Latency = time.Since(start)
		return res
	}

	seedK := c.SeedK
	if seedK <= 0 {
		seedK = 50
	}
	if seedK > q.TopN {
		seedK = q.TopN
	}

	// Seed via vector recall.
	seeds, err := c.svc.vectorRecall(ctx, q.SpaceIDs, q.QueryEmbedding, seedK, q.Filter)
	if err != nil {
		res.Err = err
		res.Latency = time.Since(start)
		return res
	}
	if len(seeds) == 0 {
		res.Latency = time.Since(start)
		return res
	}

	// Edge fetch on the seed set (1-hop). Activation will spread from these
	// seeds across the fetched edges.
	seedIDs := make([]string, 0, len(seeds))
	for _, s := range seeds {
		seedIDs = append(seedIDs, s.NodeID)
	}
	edges, _, err := c.svc.fetchOutgoingEdges(ctx, q.SpaceIDs, seedIDs)
	if err != nil {
		// Edge-fetch failure isn't fatal — we can still return the seed set
		// (with no activation expansion). Treat as a warning and degrade.
		res.Err = err
		res.Candidates = seeds
		res.Latency = time.Since(start)
		return res
	}

	// Run spreading activation. Use modest defaults — the column doesn't
	// need to fight the legacy scorer's tuning; consensus aggregation will
	// fold this output in alongside the other 5 columns.
	const (
		steps  = 2
		lambda = 0.5
	)
	hopMin := []float64{0.0, 0.05}
	// RETRIEVAL-TYPED-EDGES-001: by default the column spreads only through
	// CO_ACTIVATED_WITH (basic SpreadingActivation) — the typed semantic edges
	// (ANALOGOUS_TO/BRIDGES/etc.) are fetched but ignored. With the flag on, use
	// attention spreading so those edges (config-weighted) influence retrieval.
	// Default-off until the UVTS A/B confirms it lifts quality without saturation.
	var act map[string]float64
	if c.svc.cfg.RetrievalGraphTypedEdgesEnabled {
		attention := ComputeEdgeAttention(QueryContext{QueryText: q.QueryText}, c.svc.cfg)
		act = SpreadingActivationWithAttention(seeds, edges, steps, lambda, attention, hopMin)
	} else {
		act = SpreadingActivation(seeds, edges, steps, lambda, hopMin)
	}

	// Build output Candidates ranked by activation. Reuse seed metadata
	// where available; for activated-but-not-seeded nodes we have only the
	// node_id from the activation map and need to surface them with empty
	// metadata (the aggregator/consumer fills in via downstream lookups).
	seedMap := make(map[string]Candidate, len(seeds))
	for _, s := range seeds {
		seedMap[s.NodeID] = s
	}

	type scored struct {
		id    string
		score float64
	}
	all := make([]scored, 0, len(act))
	for id, score := range act {
		all = append(all, scored{id, score})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	limit := q.TopN
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}

	cands := make([]Candidate, 0, limit)
	for _, s := range all[:limit] {
		c, ok := seedMap[s.id]
		if !ok {
			// Activated-but-not-seeded — minimal Candidate with the score
			// recorded as VectorSim so RRF treats the rank position as the
			// signal (the aggregator only consults rank, not score, for
			// fusion).
			c = Candidate{NodeID: s.id, VectorSim: s.score}
		} else {
			// Overwrite VectorSim with activation score so the candidate's
			// per-column score reflects the graph signal rather than the
			// seed embedding similarity.
			c.VectorSim = s.score
		}
		cands = append(cands, c)
	}
	res.Candidates = cands
	res.Latency = time.Since(start)
	return res
}
