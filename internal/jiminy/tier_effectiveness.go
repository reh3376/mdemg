package jiminy

// TierEffectivenessReport grades comprehension across protocol tiers.
type TierEffectivenessReport struct {
	OverallTierComprehension [3]float64            `json:"overall_tier_comprehension"`
	TierOutcomeCount         [3]int64              `json:"tier_outcome_count"`
	CodeTierDelta            []CodeTierComparison  `json:"code_tier_delta,omitempty"`
	IneffectiveTiers         []TierIneffectiveness `json:"ineffective_tiers,omitempty"`
}

// CodeTierComparison shows comprehension differences across tiers for a single code.
type CodeTierComparison struct {
	Code              string     `json:"code"`
	TierComprehension [3]float64 `json:"tier_comprehension"` // -1 = no data
	BestTier          int        `json:"best_tier"`
	WorstTier         int        `json:"worst_tier"`
	DeltaPercent      float64    `json:"delta_percent"`
}

// TierIneffectiveness flags a specific tier/code combination that underperforms.
type TierIneffectiveness struct {
	Code          string  `json:"code"`
	Tier          int     `json:"tier"`
	Comprehension float64 `json:"comprehension"`
	BetterTier    int     `json:"better_tier"`
	BetterComp    float64 `json:"better_comprehension"`
}

// GradeTierEffectiveness analyzes a metrics snapshot to produce a tier effectiveness report.
// Returns nil if there is insufficient cross-tier data to make meaningful comparisons.
// minSamples: minimum outcome count per tier/code before including in analysis.
// ineffectiveThreshold: comprehension below this is flagged as ineffective if a better tier exists.
func GradeTierEffectiveness(snapshot *ProtocolMetrics, minSamples int, ineffectiveThreshold float64) *TierEffectivenessReport {
	if snapshot == nil {
		return nil
	}

	report := &TierEffectivenessReport{
		OverallTierComprehension: snapshot.TierComprehension,
		TierOutcomeCount:         snapshot.TierOutcomeCount,
	}

	if len(snapshot.TierCodeComprehension) < 2 {
		return nil // need at least 2 tiers with data for cross-tier analysis
	}

	// Build set of all codes that appear in any tier
	allCodes := make(map[string]struct{})
	for _, codeMap := range snapshot.TierCodeComprehension {
		for code := range codeMap {
			allCodes[code] = struct{}{}
		}
	}

	// For each code, compare comprehension across tiers
	hasCrossTierData := false
	for code := range allCodes {
		comp := CodeTierComparison{
			Code:              code,
			TierComprehension: [3]float64{-1, -1, -1},
			BestTier:          -1,
			WorstTier:         -1,
		}

		tiersWithData := 0
		bestComp := -1.0
		worstComp := 2.0 // impossibly high
		for tier := 1; tier <= 3; tier++ {
			codeMap, ok := snapshot.TierCodeComprehension[tier]
			if !ok {
				continue
			}
			c, ok := codeMap[code]
			if !ok {
				continue
			}
			// Check minimum sample count for this tier
			if snapshot.TierOutcomeCount[tier-1] < int64(minSamples) {
				continue
			}
			comp.TierComprehension[tier-1] = c
			tiersWithData++
			if c > bestComp {
				bestComp = c
				comp.BestTier = tier
			}
			if c < worstComp {
				worstComp = c
				comp.WorstTier = tier
			}
		}

		if tiersWithData < 2 {
			continue // need cross-tier data for this code
		}
		hasCrossTierData = true

		if bestComp > 0 && worstComp < 2.0 {
			comp.DeltaPercent = (bestComp - worstComp) * 100
		}

		report.CodeTierDelta = append(report.CodeTierDelta, comp)

		// Check for ineffective tiers: comprehension < threshold AND a better tier exists with >15% advantage
		for tier := 1; tier <= 3; tier++ {
			if comp.TierComprehension[tier-1] < 0 {
				continue
			}
			if comp.TierComprehension[tier-1] >= ineffectiveThreshold {
				continue
			}
			// Find a better tier
			for otherTier := 1; otherTier <= 3; otherTier++ {
				if otherTier == tier || comp.TierComprehension[otherTier-1] < 0 {
					continue
				}
				if comp.TierComprehension[otherTier-1] > comp.TierComprehension[tier-1]+0.15 {
					report.IneffectiveTiers = append(report.IneffectiveTiers, TierIneffectiveness{
						Code:          code,
						Tier:          tier,
						Comprehension: comp.TierComprehension[tier-1],
						BetterTier:    otherTier,
						BetterComp:    comp.TierComprehension[otherTier-1],
					})
					break
				}
			}
		}
	}

	if !hasCrossTierData {
		return nil
	}

	return report
}
