package retrieval

// RETRIEVAL-DIVERSITY-001: post-rerank near-duplicate suppression.
//
// RETRIEVAL-QUALITY-AUDIT-001 catalogued the near-duplicate pattern in the
// top-5 (q04 returned "pre-bash-check" twice + "sql" twice; q14 returned 3
// duplicate/near-duplicate L3/L4 emergent-concepts for a CUIDv2 query).
// ~11% of top-5 slots were wasted on redundant content instead of diverse
// coverage.
//
// This filter drops results whose `Name` already appears MaxPerName times
// in the kept set, with a fill-from-skipped fallback so the caller never
// gets fewer results than requested. Pure function; no I/O; unit-testable.

import (
	"mdemg/internal/models"
)

// DiversityCfg carries the filter's tunables. Zero-value cfg with
// Enabled=false is a no-op passthrough (safe default).
type DiversityCfg struct {
	Enabled    bool
	MaxPerName int // Max results with the same Name to keep in the output; <=0 → 1 (strict dedup)
	MinOutput  int // Safety-net: back-fill from skipped duplicates ONLY when dedup would drop output below this. <=0 → 1 (bare minimum). Set to topK to enforce "never short" back-fill (defeats the diversity intent).
}

// ApplyDiversityFilter trims near-duplicates from a ranked result list.
//
// Semantics:
//   - When Enabled=false or topK <= 0, returns input unchanged (safe
//     passthrough).
//   - Otherwise iterates in rank order:
//     * Keeps a result if its Name has appeared < MaxPerName times among
//       already-kept results.
//     * Drops otherwise (fully — the RQA-001 design intent is "prefer
//       diverse coverage over completeness"; a caller asking for topK=5
//       who gets 4 diverse results is BETTER OFF than one who gets 5
//       with a near-duplicate).
//     * An EMPTY Name is treated as always-diverse (never dedup-dropped) —
//       protects results that don't carry a name from spurious dedup.
//   - Fill-from-skipped fallback ONLY kicks in when dedup would collapse
//     the output below MinOutput (safety net against pathological cases
//     like all-same-name where the caller would otherwise get 1 result
//     for topK=10). MinOutput defaults to 1 (bare minimum). Operators
//     who want strict "never short" back-fill can set MinOutput = topK.
//
// The filter is post-rerank: it runs AFTER the RRF/rerank pipeline has
// ranked candidates. Right architectural placement is BEFORE the topK
// truncation — the filter picks from a larger candidate pool than the
// caller will see. When input already fits topK, dedup STILL runs
// (that's the whole point — free duplicate slots for the demoted
// diverse content).
func ApplyDiversityFilter(results []models.RetrieveResult, topK int, cfg DiversityCfg) []models.RetrieveResult {
	if !cfg.Enabled || topK <= 0 {
		return results
	}
	maxPerName := cfg.MaxPerName
	if maxPerName <= 0 {
		maxPerName = 1
	}
	minOutput := cfg.MinOutput
	if minOutput <= 0 {
		minOutput = 1
	}
	// Zero-hint make() — the caller-supplied topK MUST NOT reach a capacity
	// hint (CodeQL go/uncontrolled-allocation-size). Slice + map grow
	// dynamically; retrieval typically runs on <100 candidates so the perf
	// cost is negligible. The loop is bounded by len(results) upstream AND
	// by `len(kept) >= topK` inside.
	var kept []models.RetrieveResult
	var skipped []models.RetrieveResult
	seen := map[string]int{}
	for _, r := range results {
		if len(kept) >= topK {
			break
		}
		name := r.Name
		// Empty-name results are always-diverse (bypass dedup).
		if name != "" && seen[name] >= maxPerName {
			skipped = append(skipped, r)
			continue
		}
		kept = append(kept, r)
		if name != "" {
			seen[name]++
		}
	}
	// Safety-net back-fill: only if kept < MinOutput. Prefer diverse-shorter
	// output over duplicate-longer output when possible.
	for _, r := range skipped {
		if len(kept) >= minOutput || len(kept) >= topK {
			break
		}
		kept = append(kept, r)
	}
	return kept
}
