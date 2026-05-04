// Phase 14 Epic 1 — Note 06 sparse activation gate.
//
// After RRF aggregation (or legacy linear scoring) produces a ranked candidate
// list, ApplySparseGate cuts the list down to candidates whose score crosses
// the per-call activation percentile. Defaults derived from
// docs/development/post-ft-lora/phase_14_score_distribution_analysis.md (Epic
// 0 forensic): per-call score distributions are heavy-tailed (p98/p50 ≈ 4-5x)
// and within-call clamps dominate percentile choice, so the spec's p98 default
// was relaxed to p95 with MIN_ACTIVE=3, MAX_ACTIVE=20 floors/ceilings.
//
// Gate ordering: pre-rerank. The expensive LLM-bound rerank prompt receives a
// 4-6x smaller candidate set, which is the dominant quantitative win (Epic 0
// §3 — p95 rerank input 4301 tok → ~700-1000 tok at top-3).
package retrieval

import (
	"sort"

	"mdemg/internal/models"
)

// SparseGateOpts carries the per-call gate configuration. Pulled from
// cfg.SparseRetrievalEnabled / SparseActivationPercentile / SparseMinActive /
// SparseMaxActive at the call site, with optional per-request overrides.
type SparseGateOpts struct {
	// Enabled — when false the gate is a passthrough (returns input as active,
	// nothing dropped). Honored at the call site; the function itself is safe
	// to call with Enabled=false (it short-circuits).
	Enabled bool

	// Percentile — within-call score percentile cutoff in [0.5, 0.999].
	// Candidates with score ≥ percentile-threshold fire; others are dropped.
	Percentile float64

	// MinActive — floor on active set size. The gate cannot drop the active
	// set below this many candidates. Set ≤0 to disable the floor.
	MinActive int

	// MaxActive — ceiling on active set size. The gate cannot grow the active
	// set above this many candidates. Set ≤0 to disable the ceiling.
	MaxActive int
}

// SparseGateMetadata captures observability for one gate application. Emitted
// to Prometheus + V0020 sparse_gate_metrics in callers.
type SparseGateMetadata struct {
	// InputCount is the size of the input candidate list.
	InputCount int

	// ActiveCount is the size of the returned active set, after clamps.
	ActiveCount int

	// DroppedCount is the number of candidates that did not fire.
	DroppedCount int

	// Threshold is the score value at the percentile cutoff (informational —
	// the gate uses it as ">= Threshold" admission rule before clamps).
	Threshold float64

	// PercentileApplied is the percentile actually used (after any per-call
	// override). Useful for distinguishing config-default vs override calls.
	PercentileApplied float64

	// FloorApplied is true if the MIN_ACTIVE clamp grew the active set above
	// what the percentile alone would have admitted.
	FloorApplied bool

	// CeilingApplied is true if the MAX_ACTIVE clamp shrunk the active set
	// below what the percentile alone would have admitted.
	CeilingApplied bool
}

// ApplySparseGate filters a ranked candidate list down to the active set per
// the percentile + clamps in opts. The input slice is assumed to be sorted
// descending by Score (the standard ScoreAndRank / ScoreAndRankRRF output
// shape); if it is not the function still produces correct results but the
// "dropped" slice ordering reflects the input order.
//
// Returns:
//   - active: the candidates whose score met the percentile bar (or were kept
//     by the MIN_ACTIVE floor). When Enabled=false, equals the input verbatim.
//   - dropped: the remainder. Useful for the JiminyEnabled debug response
//     field (`debug.below_threshold`). Empty when Enabled=false or all
//     candidates fired.
//   - meta: observability metadata for Prometheus + V0020.
//
// Safe to call with empty input (returns empty + zeroed metadata).
func ApplySparseGate(items []models.RetrieveResult, opts SparseGateOpts) (active, dropped []models.RetrieveResult, meta SparseGateMetadata) {
	meta.InputCount = len(items)
	meta.PercentileApplied = opts.Percentile

	if !opts.Enabled || len(items) == 0 {
		// Passthrough: gate is off or empty input. Active = input verbatim.
		meta.ActiveCount = len(items)
		return items, nil, meta
	}

	// Compute the percentile threshold from the score distribution. We treat
	// items as already sorted desc by score (standard upstream contract); if
	// they're not, sort a copy of the score values to avoid mutating the
	// caller's slice ordering.
	scores := make([]float64, len(items))
	for i, it := range items {
		scores[i] = it.Score
	}
	sort.Float64s(scores) // ascending — quantile is straightforward

	// Linear-interpolated percentile (R-7 / Excel default). For a typical
	// per-call list of 20-50 candidates this is fine without going to a more
	// elaborate estimator; the alternative would be the next-rank rule which
	// has discrete steps that confuse operators.
	threshold := percentileLinear(scores, opts.Percentile)
	meta.Threshold = threshold

	// Admit candidates with score >= threshold. Use >= so the cutoff is
	// inclusive — at p95 the top 5% fire, including the 95th-percentile
	// candidate itself.
	preClamp := make([]models.RetrieveResult, 0, len(items))
	belowClamp := make([]models.RetrieveResult, 0)
	for _, it := range items {
		if it.Score >= threshold {
			preClamp = append(preClamp, it)
		} else {
			belowClamp = append(belowClamp, it)
		}
	}

	// Apply the MIN_ACTIVE floor: if the percentile admitted fewer than
	// MinActive candidates, pull more from the top of the dropped set
	// (highest-scored first) until the floor is met.
	if opts.MinActive > 0 && len(preClamp) < opts.MinActive {
		// Sort dropped desc by score to pick the highest-scored fallbacks.
		// Stable sort so ties preserve input order.
		sort.SliceStable(belowClamp, func(i, j int) bool {
			return belowClamp[i].Score > belowClamp[j].Score
		})
		need := opts.MinActive - len(preClamp)
		if need > len(belowClamp) {
			need = len(belowClamp)
		}
		preClamp = append(preClamp, belowClamp[:need]...)
		belowClamp = belowClamp[need:]
		meta.FloorApplied = true
	}

	// Apply the MAX_ACTIVE ceiling: if the percentile admitted more than
	// MaxActive candidates, demote the lowest-scored excess into dropped.
	if opts.MaxActive > 0 && len(preClamp) > opts.MaxActive {
		// preClamp inherits input order (caller's score-desc sort). Slice off
		// the tail.
		demoted := preClamp[opts.MaxActive:]
		preClamp = preClamp[:opts.MaxActive]
		// Prepend demoted to belowClamp so the dropped set still reflects
		// "items not in the active set" without reordering caller-visible
		// data.
		belowClamp = append(demoted, belowClamp...)
		meta.CeilingApplied = true
	}

	meta.ActiveCount = len(preClamp)
	meta.DroppedCount = len(belowClamp)
	return preClamp, belowClamp, meta
}

// percentileLinear computes the q-th quantile (q ∈ [0, 1]) of an ascending-
// sorted slice using R-7 / Excel-default linear interpolation. Returns the
// minimum value for empty input (defensive — caller should not pass empty).
func percentileLinear(sortedAsc []float64, q float64) float64 {
	n := len(sortedAsc)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sortedAsc[0]
	}
	if q <= 0 {
		return sortedAsc[0]
	}
	if q >= 1 {
		return sortedAsc[n-1]
	}

	// R-7 / Excel default: index = q * (n - 1).
	idx := q * float64(n-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= n {
		return sortedAsc[n-1]
	}
	frac := idx - float64(lo)
	return sortedAsc[lo] + frac*(sortedAsc[hi]-sortedAsc[lo])
}
