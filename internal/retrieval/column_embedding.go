package retrieval

import (
	"context"
	"errors"
	"time"
)

// EmbeddingColumn is the vector-similarity column. It wraps the existing
// Service.vectorRecall path so the legacy linear scorer and the new RRF
// aggregator share the same Cypher and the same Neo4j vector index.
//
// Phase 13 — Note 04 Column-Voting Retrieval. This is the "Embedding" column
// of the 6-column ensemble (the other 5 ride alongside but consult separate
// signals). The column produces a ranked list keyed by cosine similarity to
// the query vector.
type EmbeddingColumn struct {
	svc *Service
}

// NewEmbeddingColumn constructs an EmbeddingColumn over the given Service.
// The Service supplies the Neo4j driver, vector index name, and config.
func NewEmbeddingColumn(svc *Service) *EmbeddingColumn {
	return &EmbeddingColumn{svc: svc}
}

// Name implements [Column].
func (c *EmbeddingColumn) Name() string { return "embedding" }

// Run implements [Column]. Returns ColumnResult with Latency populated
// regardless of success — the aggregator records per-column histogram
// timing in all cases.
func (c *EmbeddingColumn) Run(ctx context.Context, q ColumnQuery) ColumnResult {
	start := time.Now()
	res := ColumnResult{Column: c.Name()}

	if c.svc == nil {
		res.Err = errors.New("embedding column: nil Service")
		res.Latency = time.Since(start)
		return res
	}
	if len(q.QueryEmbedding) == 0 {
		// Without an embedding the column has nothing to rank by. Return
		// success-with-empty rather than error so the aggregator simply
		// excludes us — this happens e.g. for queries where intent
		// translation skipped the embed step.
		res.Latency = time.Since(start)
		return res
	}

	cands, err := c.svc.vectorRecall(ctx, q.SpaceIDs, q.QueryEmbedding, q.TopN, q.Filter)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err
		return res
	}
	res.Candidates = cands
	return res
}
