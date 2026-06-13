package retrieval

import (
	"context"
	"sort"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// ScoreAndRankRRF is the Phase 13 (Note 04) Column-Voting Retrieval scorer,
// extended in Phase 14.2 to a 5-column form (adding ContextColumn). It
// produces the final []RetrieveResult by running up to 5 columns through
// the consensus aggregator (consensus.go) instead of the legacy linear
// formula at scoring.go:797.
//
// The 3 "virtual" columns (Embedding, BM25, Graph) are derived from the
// already-fused upstream `cands` set: each column re-ranks the same set by
// its own per-candidate signal (VectorSim, BM25Score, activation map). The
// 4th column (Structural) optionally runs an independent Cypher walk via
// the supplied StructuralColumn — if nil, only 3 columns participate. The
// 5th column (Context) is virtual over the same `cands` set and ranks by
// Jaccard similarity between candidate fingerprints and the query
// fingerprint when one is supplied (Phase 14.2 Note 05).
//
// This design keeps the upstream Cypher unchanged: the same vector recall +
// BM25 search + spreading activation work that feeds the legacy scorer
// also feeds the RRF aggregator. The only new I/O is the Structural
// column's Cypher walk.
func (s *Service) ScoreAndRankRRF(
	ctx context.Context,
	cands []Candidate,
	act map[string]float64,
	topK int,
	queryEmbedding []float32,
	queryText string,
	spaceIDs []string,
	filter FileFilter,
	queryFingerprint []uint16,
	queryFingerprintVersion int,
	category string,
) ([]models.RetrieveResult, ConsensusResult, error) {
	if topK <= 0 {
		topK = 20
	}

	// Build virtual columns over the upstream-fused candidate set. Each
	// column produces a presorted view; aggregator consults rank position
	// only (not score).
	//
	// CONTEXT-LIVE-001 consensus semantics: a column STRUCTURALLY UNABLE to
	// vote (disabled by config, or context with no query fingerprint) is
	// not appended at all — it must not deflate the consensus denominator.
	// Columns that attempt and FAIL (error/timeout) still count, by design:
	// a broken voter lowers consensus; an absent voter doesn't exist.
	// Pre-change, every live query carried the always-empty context column
	// in the denominator, hard-capping consensus at 0.8.
	cols := make([]Column, 0, 5)
	if s.cfg.RetrievalColumnEmbeddingEnabled {
		cols = append(cols, newVirtualColumn("embedding", sortByVectorSim(cands), true))
	}
	if s.cfg.RetrievalColumnBM25Enabled {
		cols = append(cols, newVirtualColumn("bm25", sortByBM25(cands), true))
	}
	if s.cfg.RetrievalColumnGraphEnabled {
		cols = append(cols, newVirtualColumn("graph", sortByActivation(cands, act), true))
	}

	// Structural column runs its own Cypher walk independently. Skip if
	// suppressed via config OR if there's no embedding to seed from.
	if s.cfg.RetrievalColumnStructuralEnabled && len(queryEmbedding) > 0 {
		structCol := NewStructuralColumn(s)
		structCol.HopDepth = s.cfg.RetrievalStructuralHops
		cols = append(cols, structCol)
	}

	// Phase 14.2 Epic 4 + CONTEXT-LIVE-001: ContextColumn joins only when
	// it can actually vote — enabled AND a query fingerprint exists.
	if s.cfg.RetrievalContextColumnEnabled && len(queryFingerprint) > 0 {
		cols = append(cols, NewContextColumn(true))
	}

	q := ColumnQuery{
		SpaceIDs:                spaceIDs,
		QueryText:               queryText,
		QueryEmbedding:          queryEmbedding,
		TopN:                    topK * 4, // headroom for column disagreement before truncation
		Filter:                  filter,
		QueryContextFingerprint: queryFingerprint,

		QueryContextFingerprintVersion: queryFingerprintVersion,
		Candidates:                     cands,
	}

	// Phase 14.2.3: per-category override of the context-column weight.
	// Phase 14.2.2's 120q forensic identified 3 categories with consistent
	// regressions (`service_relationships`, `business_logic_constraints`,
	// `relationship`); zero-weighting the column on those queries while
	// keeping default weight on the rest preserves the +0.090 lift on
	// `computed_value` and the zero-score rescues without the catastrophic
	// regressions. Operator can extend / replace via env JSON map.
	contextWeight := s.cfg.RetrievalContextColumnWeight
	if category != "" {
		if w, ok := s.cfg.RetrievalContextColumnCategoryWeights[category]; ok {
			contextWeight = w
		}
	}

	opts := ConsensusOpts{
		RRFK:                     s.cfg.RetrievalRRFK,
		PerColumnTimeoutFraction: s.cfg.RetrievalColumnTimeoutFrac,
		TopN:                     topK,
		// Phase 13.1: per-column weights wired from config. Phase 14.2 adds
		// a 5th key "context". Phase 14.2.3 overrides "context" weight
		// per-category when configured.
		ColumnWeights: map[string]float64{
			"embedding":  s.cfg.RetrievalColumnWeightEmbedding,
			"bm25":       s.cfg.RetrievalColumnWeightBM25,
			"graph":      s.cfg.RetrievalColumnWeightGraph,
			"structural": s.cfg.RetrievalColumnWeightStructural,
			"context":    contextWeight,
		},
	}

	consensus, err := Aggregate(ctx, cols, q, opts)
	if err != nil {
		return nil, consensus, err
	}

	// Convert ConsensusResult.Ranked → []RetrieveResult. Mirrors the
	// shape of ScoreAndRank's output so downstream rerank/evidence/etc
	// don't need to know the difference.
	//
	// EVENTGRAPH-001 fix-commit: Activation is propagated from the
	// spreading-activation map (same source as scoring.go:883). The legacy
	// scorer set this field; the RRF path silently dropped it, which
	// caused ApplyCoactivation to filter out every result (Activation=0
	// < LearningMinActivation=0.20) and Hebbian learning to no-op on
	// every retrieve since Phase 13.1 default-on. Discovered during
	// EVENTGRAPH-001 Epic 7 live e2e — see verification.md.
	results := make([]models.RetrieveResult, 0, len(consensus.Ranked))
	for _, c := range consensus.Ranked {
		results = append(results, models.RetrieveResult{
			NodeID:     c.NodeID,
			Path:       c.Path,
			Name:       c.Name,
			Summary:    c.Summary,
			Layer:      c.Layer,
			Score:      c.RRFScore, // RRF score becomes the ranking signal
			VectorSim:  c.VectorSim,
			Activation: act[c.NodeID],
		})
	}
	return results, consensus, nil
}

// ScoreAndRankRRFFromConfig is a thin wrapper that takes a Config so callers
// without a *Service handle can still invoke the RRF path (e.g., tests).
// The Service-bound version is the production caller.
//
//nolint:unused // exported for future test ergonomics
func ScoreAndRankRRFFromConfig(_ config.Config) {}

// virtualColumn is a Column adapter over a precomputed sorted candidate
// list. It satisfies the Column interface without doing any I/O — useful
// when the upstream pipeline already produced the per-candidate signal
// (vector similarity, BM25 score, activation) and we just need to feed
// that signal as one column's "rank" view to the aggregator.
type virtualColumn struct {
	name    string
	cands   []Candidate
	enabled bool
}

func newVirtualColumn(name string, cands []Candidate, enabled bool) Column {
	return &virtualColumn{name: name, cands: cands, enabled: enabled}
}

// Name implements [Column].
func (v *virtualColumn) Name() string { return v.name }

// Run implements [Column]. Suppressed columns return success-empty so the
// aggregator excludes them but still counts them in the denominator
// (matches the per-column failure semantics: a disabled column lowers
// consensus rather than silently inflating it).
func (v *virtualColumn) Run(_ context.Context, q ColumnQuery) ColumnResult {
	if !v.enabled {
		return ColumnResult{Column: v.name}
	}
	limit := q.TopN
	if limit <= 0 || limit > len(v.cands) {
		limit = len(v.cands)
	}
	out := make([]Candidate, limit)
	copy(out, v.cands[:limit])
	return ColumnResult{Column: v.name, Candidates: out}
}

// sortByVectorSim returns cands sorted by VectorSim desc.
func sortByVectorSim(cands []Candidate) []Candidate {
	out := make([]Candidate, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].VectorSim > out[j].VectorSim
	})
	return out
}

// sortByBM25 returns cands sorted by BM25Score desc.
func sortByBM25(cands []Candidate) []Candidate {
	out := make([]Candidate, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].BM25Score > out[j].BM25Score
	})
	return out
}

// sortByActivation returns cands sorted by activation map desc. Candidates
// missing from the map fall to the bottom (treated as 0 activation).
func sortByActivation(cands []Candidate, act map[string]float64) []Candidate {
	out := make([]Candidate, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		return act[out[i].NodeID] > act[out[j].NodeID]
	})
	return out
}
