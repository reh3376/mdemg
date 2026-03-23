package jiminy

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// TierEffectivenessDataset is a curated aggregation of tier effectiveness data for RSIC.
type TierEffectivenessDataset struct {
	GeneratedAt   time.Time            `json:"generated_at"`
	WindowStart   time.Time            `json:"window_start"`
	WindowEnd     time.Time            `json:"window_end"`
	TotalOutcomes int64                `json:"total_outcomes"`
	TierGrades    [3]TierGrade         `json:"tier_grades"`
	CodeDrift     []CodeDriftEntry     `json:"code_drift"`
	Recommendations []TierRecommendation `json:"recommendations"`
}

// TierGrade is a letter-graded summary of a single tier's comprehension.
type TierGrade struct {
	Tier             int     `json:"tier"`
	AvgComprehension float64 `json:"avg_comprehension"`
	SampleCount      int64   `json:"sample_count"`
	Grade            string  `json:"grade"` // A/B/C/D/F
}

// CodeDriftEntry identifies a constraint code where tier performance diverges.
type CodeDriftEntry struct {
	Code            string     `json:"code"`
	CurrentTier     int        `json:"current_tier"`
	TierComp        [3]float64 `json:"tier_comprehension"`
	DriftDetected   bool       `json:"drift_detected"`
	RecommendedTier int        `json:"recommended_tier"`
}

// TierRecommendation suggests a tier change for a constraint code.
type TierRecommendation struct {
	Code   string  `json:"code"`
	Action string  `json:"action"` // "demote_to_t2", "promote_to_t1", "retire", "keep"
	Reason string  `json:"reason"`
	Delta  float64 `json:"comprehension_delta"`
}

// BuildTierEffectivenessDataset creates a curated dataset from a metrics snapshot.
func BuildTierEffectivenessDataset(snapshot *ProtocolMetrics, minSamples int, threshold float64) *TierEffectivenessDataset {
	if snapshot == nil {
		return &TierEffectivenessDataset{GeneratedAt: time.Now()}
	}

	ds := &TierEffectivenessDataset{
		GeneratedAt: time.Now(),
		WindowStart: snapshot.WindowStart,
		WindowEnd:   snapshot.WindowEnd,
	}

	// Grade each tier
	for i := range 3 {
		ds.TierGrades[i] = TierGrade{
			Tier:             i + 1,
			AvgComprehension: snapshot.TierComprehension[i],
			SampleCount:      snapshot.TierOutcomeCount[i],
			Grade:            letterGrade(snapshot.TierComprehension[i], snapshot.TierOutcomeCount[i]),
		}
		ds.TotalOutcomes += snapshot.TierOutcomeCount[i]
	}

	// Analyze per-code drift
	allCodes := make(map[string]struct{})
	for _, codeMap := range snapshot.TierCodeComprehension {
		for code := range codeMap {
			allCodes[code] = struct{}{}
		}
	}

	for code := range allCodes {
		entry := CodeDriftEntry{
			Code:     code,
			TierComp: [3]float64{-1, -1, -1},
		}

		bestTier := -1
		bestComp := -1.0
		tiersWithData := 0
		for tier := 1; tier <= 3; tier++ {
			if codeMap, ok := snapshot.TierCodeComprehension[tier]; ok {
				if comp, ok := codeMap[code]; ok && snapshot.TierOutcomeCount[tier-1] >= int64(minSamples) {
					entry.TierComp[tier-1] = comp
					tiersWithData++
					if comp > bestComp {
						bestComp = comp
						bestTier = tier
					}
				}
			}
		}

		if tiersWithData < 2 {
			continue
		}

		entry.RecommendedTier = bestTier

		// Detect drift: any tier below threshold AND another tier > 15% better
		for tier := 1; tier <= 3; tier++ {
			if entry.TierComp[tier-1] < 0 {
				continue
			}
			if entry.TierComp[tier-1] < threshold && bestComp > entry.TierComp[tier-1]+0.15 {
				entry.DriftDetected = true
				entry.CurrentTier = tier
				break
			}
		}

		ds.CodeDrift = append(ds.CodeDrift, entry)

		// Generate recommendation
		if entry.DriftDetected {
			var action, reason string
			if entry.CurrentTier == 1 && bestTier > 1 {
				action = "demote_to_t2"
				reason = "T1 comprehension below threshold; T2 performs better"
			} else if entry.CurrentTier == 2 && bestTier == 3 {
				action = "demote_to_t3"
				reason = "T2 comprehension below threshold; T3 performs better"
			} else {
				action = "review"
				reason = "tier drift detected, manual review recommended"
			}
			ds.Recommendations = append(ds.Recommendations, TierRecommendation{
				Code:   code,
				Action: action,
				Reason: reason,
				Delta:  bestComp - entry.TierComp[entry.CurrentTier-1],
			})
		}
	}

	return ds
}

// CollectDataset writes a tier effectiveness dataset to JSON file.
func (dc *ProtocolDataCollector) CollectDataset(dataset *TierEffectivenessDataset) {
	if dc == nil || dataset == nil {
		return
	}
	go dc.writeDataset(dataset)
}

func (dc *ProtocolDataCollector) writeDataset(dataset *TierEffectivenessDataset) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	data, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		log.Printf("j17: tier effectiveness dataset: marshal error: %v", err)
		return
	}

	filename := filepath.Join(dc.dir, "tier-effectiveness-"+time.Now().UTC().Format("20060102-150405")+".json")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		log.Printf("j17: tier effectiveness dataset: write error: %v", err)
		return
	}
	log.Printf("j17: tier effectiveness dataset written: %s", filename)
}

func letterGrade(comp float64, count int64) string {
	if count == 0 {
		return "N/A"
	}
	switch {
	case comp >= 0.9:
		return "A"
	case comp >= 0.8:
		return "B"
	case comp >= 0.7:
		return "C"
	case comp >= 0.5:
		return "D"
	default:
		return "F"
	}
}
