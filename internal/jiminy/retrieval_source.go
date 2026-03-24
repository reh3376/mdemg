package jiminy

import "mdemg/internal/mathutil"

// Retrieval score normalization parameters.
// Retrieval FinalScore is a composite sum (vector + BM25 + activation + boosts)
// that is unbounded above 1.0. These parameters control the sigmoid normalization
// that maps it to [0, 1] for use as a confidence value comparable with
// cosine-similarity-based confidence from other Jiminy sources.
//
// midpoint=2.0: a retrieval score of 2.0 maps to confidence 0.50
// steepness=1.5: controls transition sharpness (higher = sharper)
//
// Resulting mapping:
//
//	score 0.5 → ~0.09,  1.0 → ~0.18,  2.0 → ~0.50,  3.0 → ~0.82,  4.0 → ~0.95
//
// These constants MUST match those in internal/consulting/service.go.
const (
	retrievalScoreMidpoint  = 2.0
	retrievalScoreSteepness = 1.5
	// maxConfidence caps normalized scores. From a Bayesian perspective,
	// absolute certainty is epistemologically invalid.
	maxConfidence = 0.95
)

// mapRetrievalToGuidance converts retrieval pipeline results into GuidanceItems.
// Items from L3-L5 layers get high priority (concepts), L1-L2 get medium, L0 gets low.
// Observation types are mapped to appropriate GuidanceTypes.
//
// Retrieval scores are normalized to [0, 1] via sigmoid so they are comparable
// with cosine-similarity-based confidence from constraint/correction/frontier sources.
func mapRetrievalToGuidance(results []RetrievalResult) []GuidanceItem {
	var items []GuidanceItem
	for _, r := range results {
		gType := classifyRetrievalItem(r)
		priority := layerToPriority(r.Layer)
		content := r.Name
		if r.Summary != "" {
			if content != "" {
				content += ": " + r.Summary
			} else {
				content = r.Summary
			}
		}
		if content == "" {
			continue
		}

		conf := mathutil.NormalizeScore(r.Score, retrievalScoreMidpoint, retrievalScoreSteepness)
		if conf > maxConfidence {
			conf = maxConfidence
		}

		items = append(items, GuidanceItem{
			Type:        gType,
			Priority:    priority,
			Content:     content,
			Confidence:  conf,
			SourceNodes: []string{r.NodeID},
		})
	}
	return items
}

// classifyRetrievalItem maps a RetrievalResult to a GuidanceType based on obs_type and layer.
func classifyRetrievalItem(r RetrievalResult) GuidanceType {
	// Higher-layer nodes are concepts
	if r.Layer >= 2 {
		return GuidanceConcept
	}

	// L0 nodes classified by obs_type
	switch r.ObsType {
	case "correction":
		return GuidanceCorrection
	case "constraint":
		return GuidanceConstraint
	case "decision":
		return GuidanceDecision
	case "learning", "technical_note", "insight", "context":
		return GuidanceLearning
	case "preference":
		return GuidancePreference
	case "error", "blocker":
		return GuidanceRisk
	case "progress", "context_signal", "self_improvement":
		return GuidancePattern
	case "note":
		return GuidanceLearning
	case "task":
		return GuidanceDecision
	default:
		return GuidanceLearning
	}
}

// layerToPriority maps concept hierarchy layers to priority levels.
func layerToPriority(layer int) string {
	switch {
	case layer >= 3:
		return "high"
	case layer >= 1:
		return "medium"
	default:
		return "low"
	}
}
