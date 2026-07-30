package retrieval

import (
	"log/slog"

	"mdemg/internal/models"
)

// ReverseRefQuotaCfg configures the post-rerank injection of reverse-ref
// (grep-matched) candidates into the top-K output. Mirrors the concrete-quota
// shape but keys on "was this node fetched by reverse-ref?" (identity, not
// layer/role) — the caller passes the set of reverse-ref-sourced NodeIDs.
type ReverseRefQuotaCfg struct {
	Enabled  bool
	MinSlots int
}

// ApplyReverseRefQuota promotes reverse-ref candidates (identified by
// membership in reverseRefResults) to the front of `results` up to MinSlots,
// preserving natural order elsewhere. If the natural top-K already has ≥
// MinSlots reverse-ref candidates, this is a no-op.
//
// The reverse-ref pool is APPENDED to results first (dedup by NodeID), then
// the quota promoter runs. Same architectural shape as
// ApplyConcreteQuotaWithExtra: fetch-then-inject. Runs BEFORE topK truncation.
func ApplyReverseRefQuota(
	results []models.RetrieveResult,
	reverseRefResults []models.RetrieveResult,
	topK int,
	cfg ReverseRefQuotaCfg,
) []models.RetrieveResult {
	if !cfg.Enabled || topK <= 0 || cfg.MinSlots <= 0 {
		return results
	}
	if len(results) == 0 && len(reverseRefResults) == 0 {
		return results
	}
	minSlots := cfg.MinSlots
	if minSlots > topK {
		minSlots = topK
	}
	// Identity: a node is "reverse-ref" if its NodeID appears in
	// reverseRefResults. Some may already be in `results` naturally.
	rrSet := make(map[string]struct{}, len(reverseRefResults))
	for _, r := range reverseRefResults {
		if r.NodeID != "" {
			rrSet[r.NodeID] = struct{}{}
		}
	}
	if len(rrSet) == 0 {
		return results
	}
	// Count reverse-ref already in natural top-K.
	scanLim := topK
	if scanLim > len(results) {
		scanLim = len(results)
	}
	inTopK := 0
	for i := 0; i < scanLim; i++ {
		if _, ok := rrSet[results[i].NodeID]; ok {
			inTopK++
		}
	}
	if inTopK >= minSlots {
		slog.Debug("retrieval: reverse-ref quota already satisfied", "in_top_k", inTopK, "min_slots", minSlots)
		return results
	}
	// Append reverse-ref candidates not already in results (dedup by NodeID).
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		seen[r.NodeID] = struct{}{}
	}
	appended := append([]models.RetrieveResult{}, results...)
	for _, r := range reverseRefResults {
		if r.NodeID == "" {
			continue
		}
		if _, dup := seen[r.NodeID]; dup {
			continue
		}
		appended = append(appended, r)
		seen[r.NodeID] = struct{}{}
	}
	// Promote the top-MinSlots reverse-ref candidates (by natural order in
	// the reverseRefResults input — grep-hit-count DESC) to the front.
	promoted := minSlots
	if promoted > len(reverseRefResults) {
		promoted = len(reverseRefResults)
	}
	promotedSet := make(map[string]struct{}, promoted)
	head := make([]models.RetrieveResult, 0, promoted)
	// Prefer reverseRefResults entries that we ACTUALLY appended (i.e. that
	// exist in the pool now). Walk in reverseRefResults order.
	for _, r := range reverseRefResults {
		if len(head) >= promoted {
			break
		}
		if r.NodeID == "" {
			continue
		}
		if _, dup := promotedSet[r.NodeID]; dup {
			continue
		}
		// Use the entry from `appended` (canonical) if it exists — may carry
		// richer scoring than the reverse-ref-authored one when the node was
		// already in the primary pool.
		found := false
		for _, a := range appended {
			if a.NodeID == r.NodeID {
				head = append(head, a)
				promotedSet[a.NodeID] = struct{}{}
				found = true
				break
			}
		}
		if !found {
			head = append(head, r)
			promotedSet[r.NodeID] = struct{}{}
		}
	}
	tail := make([]models.RetrieveResult, 0, len(appended)-len(head))
	for _, r := range appended {
		if _, ok := promotedSet[r.NodeID]; ok {
			continue
		}
		tail = append(tail, r)
	}
	slog.Debug("retrieval: reverse-ref quota promoted",
		"promoted", len(head),
		"pool_size_after", len(head)+len(tail))
	return append(head, tail...)
}
