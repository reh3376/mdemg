package review

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// Sampler orders/selects review items so developer time hits the most
// informative ones: ACTIVE (prefer items whose AutoScore is uncertain — near 0.5
// — or that disagree with a secondary signal) + STRATIFIED (round-robin across
// the dataset's natural strata so none is starved). Deterministic for a fixed
// non-zero Seed.
type Sampler struct {
	SampleSize      int     // cap; ≤0 → no cap
	UncertaintyBand float64 // items with |AutoScore-0.5| ≤ band/2 get a strong bonus
	Seed            int64   // 0 → time-seeded; non-zero → deterministic
}

// informativeness scores how much grading this item would teach us: high when
// the auto-classifier is uncertain (AutoScore near 0.5) OR a secondary signal
// disagrees with it, with a strong bonus inside the active uncertainty band.
func informativeness(it ReviewItem, band float64) float64 {
	unc := 1.0 - 2.0*math.Abs(it.AutoScore-0.5) // 1.0 at 0.5, →0 at the extremes
	if unc < 0 {
		unc = 0
	}
	dis := 0.0 // a secondary signal far from the auto score = disagreement
	for _, sig := range it.Signals {
		if d := math.Abs(sig - it.AutoScore); d > dis {
			dis = d
		}
	}
	inf := math.Max(unc, dis)
	if math.Abs(it.AutoScore-0.5) <= band/2 {
		inf += 1.0 // within the active band — prioritize strongly
	}
	return inf
}

// Select returns the chosen items, most-informative-first within each stratum,
// round-robin across strata, capped at SampleSize.
func (s Sampler) Select(items []ReviewItem) []ReviewItem {
	if len(items) == 0 {
		return nil
	}
	seed := s.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // sampling order, not security
	band := s.UncertaintyBand

	limit := s.SampleSize
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}

	// Group by stratum, preserving first-seen order.
	byStratum := map[string][]ReviewItem{}
	var order []string
	for _, it := range items {
		if _, ok := byStratum[it.Stratum]; !ok {
			order = append(order, it.Stratum)
		}
		byStratum[it.Stratum] = append(byStratum[it.Stratum], it)
	}

	// Sort each stratum by informativeness desc; tie-break by ItemID for full
	// determinism within a stratum.
	for st := range byStratum {
		lst := byStratum[st]
		sort.SliceStable(lst, func(i, j int) bool {
			ii, ij := informativeness(lst[i], band), informativeness(lst[j], band)
			if ii != ij {
				return ii > ij
			}
			return lst[i].ItemID < lst[j].ItemID
		})
		byStratum[st] = lst
	}

	// Shuffle the stratum-visiting order deterministically by seed so different
	// seeds vary which strata are favored when the cap bites.
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	out := make([]ReviewItem, 0, limit)
	idx := make(map[string]int, len(order))
	for len(out) < limit {
		progressed := false
		for _, st := range order {
			if len(out) >= limit {
				break
			}
			if i := idx[st]; i < len(byStratum[st]) {
				out = append(out, byStratum[st][i])
				idx[st] = i + 1
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return out
}
