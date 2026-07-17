package jiminy

import (
	"mdemg/internal/config"
	"mdemg/internal/mathutil"
)

// Retrieval score normalization parameters.
// Retrieval FinalScore is a composite sum (vector + BM25 + activation + boosts)
// that is unbounded above 1.0. These parameters control the sigmoid normalization
// that maps it to [0, 1] for use as a confidence value comparable with
// cosine-similarity-based confidence from other Jiminy sources.
//
// midpoint=1.5: a retrieval score of 1.5 maps to confidence 0.50
// steepness=1.5: controls transition sharpness (higher = sharper)
//
// Resulting mapping:
//
//	score 0.5 → ~0.18,  1.0 → ~0.32,  1.5 → ~0.50,  2.0 → ~0.68,  3.0 → ~0.90
//
// RRF-SCALE-001: sigmoid midpoint/steepness are now config-driven (passed in
// from the caller via cfg.RetrievalConfidenceSigmoid{Midpoint,Steepness}) and
// recalibrated for RRF scale (legacy 1.5/1.5 crushed RRF scores to ~0.1-0.2).
// These constants are the zero-value fallback only.
const (
	// Aliases to the single source of truth in config (no duplicated literals).
	defaultRetrievalScoreMidpoint  = config.DefaultRetrievalConfidenceSigmoidMidpoint
	defaultRetrievalScoreSteepness = config.DefaultRetrievalConfidenceSigmoidSteepness
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
func mapRetrievalToGuidance(results []RetrievalResult, sigmoidMidpoint, sigmoidSteepness float64) []GuidanceItem {
	if sigmoidMidpoint <= 0 {
		sigmoidMidpoint = defaultRetrievalScoreMidpoint
	}
	if sigmoidSteepness <= 0 {
		sigmoidSteepness = defaultRetrievalScoreSteepness
	}
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

		conf := mathutil.NormalizeScore(r.Score, sigmoidMidpoint, sigmoidSteepness)
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

// classifyRetrievalItem maps a RetrievalResult to a GuidanceType based on
// role_type first, then obs_type + layer. role_type precedence closes the
// JIMINY-CORPUS-001 disclosed follow-up: before JIMINY-ROLETYPE-ADAPTER-001
// the adapter dropped role_type and every retrieval-sourced constraint node
// defaulted to GuidanceLearning (see docs/features/jiminy-actionability.md
// §Follow-up).
func classifyRetrievalItem(r RetrievalResult) GuidanceType {
	// Precedence: an explicit role_type on the node beats layer + obs_type
	// heuristic. Constraint / correction MemoryNodes are Layer 1 in mdemg-dev,
	// so without this prefix they hit the ObsType switch (empty for role-based
	// nodes) → default GuidanceLearning.
	switch r.RoleType {
	case "constraint":
		return GuidanceConstraint
	case "correction":
		return GuidanceCorrection
	}

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
