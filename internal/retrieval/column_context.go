// Phase 14.2 Epic 4 — ContextColumn: 5th RRF column scoring candidates by
// Jaccard similarity between their observation context fingerprint and the
// query's fingerprint. Implements the Column interface.
//
// Scoring: |A ∩ B| / |A ∪ B|, bounded [0, 1]. Either-fingerprint-empty →
// score 0 (graceful degradation; column contributes 0 to RRF sum but
// doesn't block other columns from voting). When the query fingerprint
// is empty, this column emits zero candidates so the aggregator simply
// excludes it from the RRF denominator.
//
// I/O: ContextColumn does NOT do its own Cypher walk — it consumes the
// upstream-fused candidate set passed in via ColumnQuery.Candidates and
// reads each candidate's ContextFingerprintActive (populated by the
// orchestrator at fetch time).
package retrieval

import (
	"context"
	"sort"
)

// ContextColumn ranks candidates by fingerprint-Jaccard similarity to the
// query's fingerprint. Construct via NewContextColumn(weight); the weight
// itself flows separately into the aggregator's ColumnWeights map.
type ContextColumn struct {
	enabled bool
}

// NewContextColumn returns a ContextColumn. enabled mirrors the
// RETRIEVAL_CONTEXT_COLUMN_ENABLED config flag. Disabled columns return
// success-empty so the aggregator counts them in the denominator without
// inflating consensus (matches virtualColumn semantics).
func NewContextColumn(enabled bool) *ContextColumn {
	return &ContextColumn{enabled: enabled}
}

// Name implements Column.
func (c *ContextColumn) Name() string { return "context" }

// Run implements Column. Returns candidates sorted by Jaccard score desc.
// Candidates whose ContextFingerprintActive is empty AND query fingerprint
// is non-empty get score 0 (no information). Both-empty returns
// success-empty (graceful no-op).
func (c *ContextColumn) Run(_ context.Context, q ColumnQuery) ColumnResult {
	if !c.enabled {
		return ColumnResult{Column: c.Name()}
	}
	if len(q.QueryContextFingerprint) == 0 || len(q.Candidates) == 0 {
		return ColumnResult{Column: c.Name()}
	}

	type scored struct {
		idx   int
		score float64
	}
	scores := make([]scored, 0, len(q.Candidates))
	for i, cand := range q.Candidates {
		score := JaccardFingerprint(q.QueryContextFingerprint, cand.ContextFingerprintActive)
		scores = append(scores, scored{idx: i, score: score})
	}
	// Sort by score desc; stable so candidates with equal score keep upstream order.
	sort.SliceStable(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	limit := q.TopN
	if limit <= 0 || limit > len(scores) {
		limit = len(scores)
	}
	out := make([]Candidate, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, q.Candidates[scores[i].idx])
	}
	return ColumnResult{Column: c.Name(), Candidates: out}
}

// JaccardFingerprint computes the Jaccard similarity of two sorted sparse
// fingerprints. Both inputs must be sorted ascending and contain no
// duplicates (matches the contract from
// conversation.ComputeContextFingerprintLocal). Time complexity O(|A|+|B|).
//
// Returns 0 when either input is empty (caller treats this as "no
// information," not "definitely dissimilar"). Returns 1 when A == B and
// both non-empty.
func JaccardFingerprint(a, b []uint16) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			intersection++
			i++
			j++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
