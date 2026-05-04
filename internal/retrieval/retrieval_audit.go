package retrieval

import (
	"context"
	"time"
)

// RetrievalAuditRecord is one row of the V0017 `retrieval_audit` hypertable.
// Phase 13 — Note 04 Column-Voting Retrieval. Captures per-call scorer
// metadata + consensus_strength + per-column latency so Phase 14 has a
// historical baseline of column-voting behavior available the day Notes
// 05+06 ship.
type RetrievalAuditRecord struct {
	// SpaceID identifies the retrieval scope.
	SpaceID string

	// QueryTextHash is the first 16 hex chars of SHA256(QueryText). Avoids
	// storing PII while preserving the ability to bucket by query template.
	QueryTextHash string

	// ScorerVersion is the active scorer ("v0-linear" or "v1-rrf4"). Comes
	// from Service.scorerVersion() and bumps when column set or weight
	// defaults change so historical rows stay segmented.
	ScorerVersion string

	// ConsensusStrength is the aggregator's per-call mean across the
	// returned top-N. Zero for v0-linear rows (legacy scorer doesn't
	// compute it).
	ConsensusStrength float64

	// PerColumnLatency[name] = column wall-clock for this call. Recorded
	// for every attempted column (success or fail) — a slow column shows
	// up here without needing per-call spans.
	PerColumnLatency map[string]time.Duration

	// ColumnsQueried is the count of columns the aggregator attempted
	// (denominator for consensus). Includes columns that errored or
	// timed out.
	ColumnsQueried int

	// ColumnsReturned is the count of columns that produced ≥1 candidate.
	ColumnsReturned int

	// TopKNodeIDs is the final ranked output, capped at the response's
	// top-K. Useful for forensic "did Phase 13 return the same nodes as
	// legacy on this query?" queries.
	TopKNodeIDs []string

	// TotalLatencyMs is the wall-clock for the entire retrieve call.
	TotalLatencyMs int64
}

// RetrievalAuditWriter is the contract for persisting retrieval_audit rows.
// Implementations write to TimescaleDB via pgxpool. Service holds an
// optional implementation; nil means audit-row writes are skipped (the
// default state when V0017 hasn't been applied or RETRIEVAL_AUDIT_ENABLED
// is false).
type RetrievalAuditWriter interface {
	// Write persists one row. Errors are non-fatal at the call site —
	// the implementation is expected to log internally so observability
	// catches them. Phase 13 v1 callers fail-open on write errors to
	// avoid degrading the user-facing retrieve call.
	Write(ctx context.Context, rec RetrievalAuditRecord) error
}

// SetRetrievalAuditWriter wires a writer. Pass nil to disable. Mirrors the
// SetRetrievalRecorder pattern. api.NewServer is the expected wiring site
// once V0017 is applied + RETRIEVAL_AUDIT_ENABLED=true.
func (s *Service) SetRetrievalAuditWriter(w RetrievalAuditWriter) {
	s.retrievalAuditWriter = w
}
