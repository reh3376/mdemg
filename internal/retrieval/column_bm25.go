package retrieval

import (
	"context"
	"errors"
	"time"
)

// BM25Column is the lexical/keyword column. It wraps the existing
// Service.BM25Search path. Phase 13 — Note 04 Column-Voting Retrieval.
//
// BM25 is an independent signal from Embedding: the same node ranking very
// high in the embedding column but absent from BM25 means the lexical match
// disagrees, which the aggregator surfaces as lower `consensus_strength`.
type BM25Column struct {
	svc *Service
}

// NewBM25Column constructs a BM25Column over the given Service.
func NewBM25Column(svc *Service) *BM25Column {
	return &BM25Column{svc: svc}
}

// Name implements [Column].
func (c *BM25Column) Name() string { return "bm25" }

// Run implements [Column].
func (c *BM25Column) Run(ctx context.Context, q ColumnQuery) ColumnResult {
	start := time.Now()
	res := ColumnResult{Column: c.Name()}

	if c.svc == nil {
		res.Err = errors.New("bm25 column: nil Service")
		res.Latency = time.Since(start)
		return res
	}
	if q.QueryText == "" {
		// No textual query — BM25 has nothing to score. Return success-empty.
		res.Latency = time.Since(start)
		return res
	}

	bm25Results, err := c.svc.BM25Search(ctx, q.SpaceIDs, q.QueryText, q.TopN, q.Filter)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err
		return res
	}

	// Convert []BM25Result -> []Candidate so the aggregator sees a uniform
	// shape across columns. Preserve original ordering (BM25Search returns
	// best-first); aggregator re-ranks via RRF on the integer position.
	cands := make([]Candidate, 0, len(bm25Results))
	for _, r := range bm25Results {
		cands = append(cands, Candidate{
			NodeID:     r.NodeID,
			Path:       r.Path,
			Name:       r.Name,
			Summary:    r.Summary,
			UpdatedAt:  r.UpdatedAt,
			Confidence: r.Confidence,
			BM25Score:  r.Score,
			Layer:      r.Layer,
			Tags:       r.Tags,
		})
	}
	res.Candidates = cands
	return res
}
