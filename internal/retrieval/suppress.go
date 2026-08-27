// Sprint RETRIEVAL-META-DOC-SUPPRESSION-001 (task #143) — targeted per-path
// score suppression for retrieval fusion.
//
// Purpose: MDEMG-DOCS-INGEST-001 (task #142) surfaced that ~3 specific hub
// nodes systematically over-score on MDEMG-usage queries via BM25 + short
// whole-file content containing "MDEMG" repeatedly. Archiving them is unsafe
// (they're heavy graph hubs anchoring L1 Hidden clusters). Reranker changes
// are nondeterministic. This helper is a NARROW opt-in intervention that
// multiplies the fused RRF score of nodes at operator-specified paths by
// a configurable factor, applied post-fusion pre-seed-extraction.
//
// Config knobs (see internal/config/config.go):
//   - RETRIEVAL_SUPPRESS_PATHS — comma-separated exact-match paths (default empty)
//   - RETRIEVAL_SUPPRESS_FACTOR — score multiplier (default 0.3; 0 = fully suppress score but keep discoverable)
//
// Default OFF (empty path list). Operator opts in via env.
package retrieval

import (
	"sort"

	"mdemg/internal/models"
)

// SuppressCandidatesByPath multiplies the RRFScore of candidates whose Path
// matches one of suppressPaths by factor, then re-sorts by RRFScore desc.
//
// - Empty suppressPaths → no-op (returns input unchanged).
// - factor=1.0 → no-op (scores unchanged; still re-sorted for consistency).
// - factor=0.0 → matched candidates keep score 0 but remain in the pool for
//   downstream traversal/rerank; not the same as archive.
// - Exact path match only (no regex); most auditable + surprise-free.
// - Stable sort so ties preserve original order.
//
// In-place mutation of the input slice (Path unchanged; RRFScore modified).
// Returns the modified + re-sorted slice.
func SuppressCandidatesByPath(cands []Candidate, suppressPaths []string, factor float64) []Candidate {
	if len(suppressPaths) == 0 || len(cands) == 0 {
		return cands
	}
	suppressSet := buildPathSet(suppressPaths)
	if len(suppressSet) == 0 {
		return cands
	}

	// Apply factor in place
	for i := range cands {
		if _, hit := suppressSet[cands[i].Path]; hit {
			cands[i].RRFScore *= factor
		}
	}
	// Stable re-sort by RRFScore desc
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].RRFScore > cands[j].RRFScore
	})
	return cands
}

// SuppressResultsByPath is the post-scoring twin of SuppressCandidatesByPath.
// The column-voting path (ScoreAndRankRRF) OVERWRITES cand.RRFScore from its
// own per-column votes, so pre-scoring suppression alone is a no-op for the
// default RRF path. This variant applies the same factor to the final
// models.RetrieveResult.Score AFTER scoring, before the sparse gate + rerank
// see the ordering. Same contract:
//   - Empty suppressPaths → no-op
//   - factor=1.0 → no-op (still re-sorted)
//   - factor=0.0 → matched keep score 0 but stay in the pool for rerank
//   - Exact path match only
//   - Stable sort so ties preserve original order
//
// In-place mutation. Returns the modified + re-sorted slice.
func SuppressResultsByPath(results []models.RetrieveResult, suppressPaths []string, factor float64) []models.RetrieveResult {
	if len(suppressPaths) == 0 || len(results) == 0 {
		return results
	}
	suppressSet := buildPathSet(suppressPaths)
	if len(suppressSet) == 0 {
		return results
	}
	for i := range results {
		if _, hit := suppressSet[results[i].Path]; hit {
			results[i].Score *= factor
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func buildPathSet(paths []string) map[string]struct{} {
	m := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p != "" {
			m[p] = struct{}{}
		}
	}
	return m
}
