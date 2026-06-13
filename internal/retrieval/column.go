package retrieval

import (
	"context"
	"time"
)

// Column is one of the parallel retrieval signals consumed by the consensus
// aggregator (consensus.go). Phase 13 — Column-Voting Retrieval (Note 04).
//
// Each column produces an independently-ranked set of [Candidate]s for a given
// query. The aggregator runs all enabled columns in parallel via errgroup
// (subject to a per-column timeout), then combines them via Reciprocal Rank
// Fusion to produce the final ranking + a `consensus_strength` signal.
//
// A column's [Run] error is non-fatal: the aggregator excludes the column
// from the fusion (treating its missing nodes as rank=∞) but still returns a
// valid result. This degrades gracefully when (e.g.) the Neo4j vector index
// is unavailable.
type Column interface {
	// Name returns a stable identifier ("embedding", "bm25", "graph",
	// "structural", "temporal", "role"). Used as the per-column metric label
	// + the per-column-failed counter dimension.
	Name() string

	// Run executes the column against the query and returns its top-N
	// [Candidate]s in ranked order (best first). The returned [ColumnResult]
	// always has Latency set even on error, so the aggregator can surface
	// the timing in the per-column histogram.
	Run(ctx context.Context, q ColumnQuery) ColumnResult
}

// ColumnQuery is the input to all columns. Built once by the orchestrator
// from the inbound RetrieveRequest + computed query embedding.
type ColumnQuery struct {
	// SpaceIDs scopes the search. Always populated.
	SpaceIDs []string

	// QueryText is the user's textual query (BM25 + reranker input).
	QueryText string

	// QueryEmbedding is the precomputed query vector (Embedding column +
	// optional Structural / Temporal / Role columns that need vector seeds).
	QueryEmbedding []float32

	// TopN caps the per-column returned set. Aggregator typically passes a
	// large value (e.g. 200) so RRF has room to disagree before truncation.
	TopN int

	// Filter optionally restricts the candidate pool (extension allow/deny,
	// path globs, etc.). Same shape as the legacy retrieval path.
	Filter FileFilter

	// Phase 14.2 Epic 4 — Sparse context fingerprint for the query (Note 05).
	// When non-empty, ContextColumn scores candidates by Jaccard similarity
	// between their observation fingerprint and this vector. Empty → the
	// column contributes 0 / column suppressed (no context-aware ranking).
	QueryContextFingerprint []uint16

	// CONTEXT-LIVE-001 — catalog version QueryContextFingerprint was
	// derived against. >0 enables the cross-version guard in
	// ContextColumn (mismatched candidate fingerprints score 0).
	QueryContextFingerprintVersion int

	// Candidates carries the upstream-fused candidate set (already loaded
	// once by the orchestrator). ContextColumn re-ranks this set; it does
	// NOT do a separate Cypher walk. Other future columns may reuse this
	// to avoid duplicate I/O too.
	Candidates []Candidate
}

// ColumnResult bundles a column's output. Latency is reported in all cases
// (success, partial, error) so the aggregator can record per-column timing
// uniformly.
type ColumnResult struct {
	// Column echoes the column name (matches Run-receiver's Name()) so the
	// aggregator can demux without holding the Column reference.
	Column string

	// Candidates are the column's top-N ranked [Candidate]s, best first.
	// Empty slice on error or genuine zero-result.
	Candidates []Candidate

	// Latency is the wall-clock time spent in Run(), including any
	// per-column timeout that fired. Populated even on error.
	Latency time.Duration

	// Err is the column's failure reason (nil on success). Non-fatal —
	// the aggregator will exclude this column from RRF but still produce
	// a result. The aggregator increments
	// `mdemg_retrieval_column_failed_total{column,reason}` on non-nil Err.
	Err error
}
