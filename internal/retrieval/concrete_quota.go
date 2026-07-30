package retrieval

import (
	"log/slog"

	"mdemg/internal/models"
)

func slogInfoConcreteQuota(poolSize, concreteInPool, minSlots, topK int) {
	slog.Debug("retrieval: concrete-quota check",
		"pool_size", poolSize,
		"concrete_in_pool", concreteInPool,
		"min_slots", minSlots,
		"top_k", topK)
}

// rerankReturnK computes how many candidates to ask the reranker to return.
// When the concrete-quota is off, the historical behavior of returning exactly
// topK is preserved. When the quota is on, we ask for the FULL rerankTopN
// scored set (the reranker aggressively demotes concrete L0/L1 candidates
// below any smaller ReturnK — live-caught during Tier-3 smoke: at ReturnK=15
// none of the concrete candidates survived the rerank cut, so a smaller
// return-K left the quota with an empty concrete pool). rerankTopN is a
// bounded scoring budget already; returning the whole scored set is free.
func rerankReturnK(topK int, quotaEnabled bool, rerankTopN int) int {
	if !quotaEnabled {
		return topK
	}
	if rerankTopN > topK {
		return rerankTopN
	}
	return topK
}

// ConcreteQuotaCfg configures the post-rerank layer quota: guarantee at least
// N slots in the top-K output are held by "concrete" (L0/L1 role-typed)
// results, promoting the highest-scoring concrete candidates ahead of the
// truncation seam when they aren't already in the natural top-K.
//
// RETRIEVAL-LAYER-BALANCE-001 Epic 2 (added after live smoke revealed the
// concrete-recall pool inclusion alone doesn't surface concrete candidates —
// they DO reach the fused pool, but RRF fusion + rerank rank them below the
// abstract L3+ emergent-concept clusters that share query terms).
type ConcreteQuotaCfg struct {
	Enabled   bool
	MinSlots  int  // guarantee this many concrete slots in top-K (default 1)
	LayerMax  int  // max layer treated as "concrete" (default 1)
	RoleTypes []string // when set, only these role_types count as "concrete"
}

// ApplyConcreteQuota reorders `results` so that the FIRST `MinSlots` positions
// (within topK) contain concrete candidates (layer <= LayerMax, role_type ∈
// RoleTypes) when they exist in the pool. Preserves relative ordering within
// each group (concrete-first, then non-concrete in original rank order).
//
// If the natural top-K already has ≥ MinSlots concrete candidates, this is a
// no-op. If fewer than MinSlots concrete candidates exist in the ENTIRE pool,
// the function promotes as many as it can (never fabricates slots).
//
// Runs BEFORE the topK truncation — same architectural placement as the
// RETRIEVAL-DIVERSITY-001 filter. Composes with diversity: quota promotes
// concrete into top-K; diversity then dedups near-duplicates within the
// promoted set.
//
// The `extraConcrete` param carries concrete candidates fetched via a separate
// role-filtered path (concrete-recall) — they bypass the LLM cross-encoder
// rerank entirely, which is critical because live-caught during Tier-3 smoke:
// the reranker aggressively demotes concrete L0/L1 candidates below the top-K
// cut for keyword-shaped queries (their scored representation is much lower
// than same-topic emergent-concept clusters that share query terms in their
// names). If the primary `results` pool already contains enough concrete
// candidates naturally, extraConcrete is ignored (dedup by NodeID).
func ApplyConcreteQuotaWithExtra(
	results []models.RetrieveResult,
	extraConcrete []models.RetrieveResult,
	topK int,
	cfg ConcreteQuotaCfg,
) []models.RetrieveResult {
	if !cfg.Enabled || topK <= 0 || cfg.MinSlots <= 0 {
		return results
	}
	if len(results) == 0 && len(extraConcrete) == 0 {
		return results
	}
	// If the natural results already have enough concrete, no injection needed.
	if hasEnoughConcrete(results, topK, cfg) {
		return ApplyConcreteQuota(results, topK, cfg)
	}
	// Inject extraConcrete at the head-injectable position, dedup by NodeID.
	// The extras are score-sorted by their VectorSim already (from
	// fetchConcreteRecall's ORDER BY sim DESC), so appending them to the pool
	// gives the promoter a valid ranking to draw from.
	seen := map[string]struct{}{}
	for _, r := range results {
		seen[r.NodeID] = struct{}{}
	}
	appended := append([]models.RetrieveResult{}, results...)
	for _, r := range extraConcrete {
		if r.NodeID == "" {
			continue
		}
		if _, dup := seen[r.NodeID]; dup {
			continue
		}
		appended = append(appended, r)
		seen[r.NodeID] = struct{}{}
	}
	return ApplyConcreteQuota(appended, topK, cfg)
}

func hasEnoughConcrete(results []models.RetrieveResult, topK int, cfg ConcreteQuotaCfg) bool {
	roleSet := make(map[string]struct{}, len(cfg.RoleTypes))
	for _, rt := range cfg.RoleTypes {
		if rt != "" {
			roleSet[rt] = struct{}{}
		}
	}
	limit := topK
	if limit > len(results) {
		limit = len(results)
	}
	count := 0
	for i := 0; i < limit; i++ {
		if results[i].Layer > cfg.LayerMax {
			continue
		}
		if len(roleSet) > 0 {
			if _, ok := roleSet[results[i].RoleType]; !ok {
				continue
			}
		}
		count++
	}
	return count >= cfg.MinSlots
}

func ApplyConcreteQuota(results []models.RetrieveResult, topK int, cfg ConcreteQuotaCfg) []models.RetrieveResult {
	if !cfg.Enabled || topK <= 0 || cfg.MinSlots <= 0 || len(results) == 0 {
		return results
	}
	// Diagnostic: total concrete count in the pool.
	total := 0
	for _, r := range results {
		if r.Layer <= cfg.LayerMax {
			total++
		}
	}
	slogInfoConcreteQuota(len(results), total, cfg.MinSlots, topK)
	minSlots := cfg.MinSlots
	if minSlots > topK {
		minSlots = topK
	}
	layerMax := cfg.LayerMax
	if layerMax < 0 {
		layerMax = 0
	}
	roleSet := make(map[string]struct{}, len(cfg.RoleTypes))
	for _, rt := range cfg.RoleTypes {
		if rt != "" {
			roleSet[rt] = struct{}{}
		}
	}
	isConcrete := func(r models.RetrieveResult) bool {
		if r.Layer > layerMax {
			return false
		}
		if len(roleSet) == 0 {
			return true
		}
		_, ok := roleSet[r.RoleType]
		return ok
	}

	// Count concrete already in the natural top-K.
	inTopK := 0
	scanLim := topK
	if scanLim > len(results) {
		scanLim = len(results)
	}
	for i := 0; i < scanLim; i++ {
		if isConcrete(results[i]) {
			inTopK++
		}
	}
	if inTopK >= minSlots {
		return results // quota already satisfied naturally
	}

	// Collect the concretes in natural rank order — top-N by score will be
	// promoted; the rest of the pool retains its natural rank (which happens
	// automatically since we filter promoted IDs out of the tail below).
	var concrete []models.RetrieveResult
	for _, r := range results {
		if isConcrete(r) {
			concrete = append(concrete, r)
		}
	}
	if len(concrete) == 0 {
		return results // nothing to promote
	}
	// Reorder: first MinSlots concrete, then interleave the rest of concrete
	// with other preserving each group's original rank order. Simpler shape:
	// take up to MinSlots concrete, then append the remaining pool in its
	// original relative order (concrete-then-other from the natural rank).
	//
	// The "remaining pool" fills the tail from `other` first (they had higher
	// rank than the not-yet-promoted concrete in the pool). But actually the
	// clean invariant is: promote the top-N concrete to the front, keep the
	// natural rank order elsewhere. Since post-rerank `results` is sorted by
	// score desc, this gives: [top-MinSlots concrete] + [everything else in
	// natural rank, minus the promoted concrete].
	promoted := minSlots
	if promoted > len(concrete) {
		promoted = len(concrete)
	}
	promotedSet := make(map[string]struct{}, promoted)
	head := make([]models.RetrieveResult, 0, promoted)
	for i := 0; i < promoted; i++ {
		head = append(head, concrete[i])
		promotedSet[concrete[i].NodeID] = struct{}{}
	}
	tail := make([]models.RetrieveResult, 0, len(results)-promoted)
	for _, r := range results {
		if _, ok := promotedSet[r.NodeID]; ok {
			continue
		}
		tail = append(tail, r)
	}
	return append(head, tail...)
}
