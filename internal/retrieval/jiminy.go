package retrieval

import (
	"fmt"
	"strings"
)

// JiminyExplanation provides transparency into why a result was retrieved and ranked.
// Named after the character who guides and explains - MDEMG's "conscience" layer.
type JiminyExplanation struct {
	Rationale           string             `json:"rationale"`
	Confidence          float64            `json:"confidence"`
	RetrievalPath       []string           `json:"retrieval_path"`
	ContributingModules []string           `json:"contributing_modules,omitempty"`
	ScoreBreakdown      map[string]float64 `json:"score_breakdown"`
}

// RetrievalPathStage represents a stage in the retrieval pipeline
type RetrievalPathStage string

const (
	StageVectorRecall        RetrievalPathStage = "vector_recall"
	StageSpreadingActivation RetrievalPathStage = "spreading_activation"
	StageLLMRerank           RetrievalPathStage = "llm_rerank"
	StageLearningEdge        RetrievalPathStage = "learning_edge"
	StagePathBoost           RetrievalPathStage = "path_boost"
	StageComparisonBoost     RetrievalPathStage = "comparison_boost"
	StageConfigBoost         RetrievalPathStage = "config_boost"
	StageTemporalBoost       RetrievalPathStage = "temporal_boost"
	StageStaleRefPenalty     RetrievalPathStage = "stale_ref_penalty"
)

// GenerateJiminyExplanation creates a human-readable explanation for why a result was ranked.
func GenerateJiminyExplanation(breakdown ScoreBreakdown, path []string) JiminyExplanation {
	// Build the rationale from significant contributors
	parts := []string{}

	// Always mention vector similarity as the primary signal
	if breakdown.VectorSimilarity > 0.01 {
		strength := describeStrength(breakdown.VectorSimilarity, 0.3, 0.5)
		parts = append(parts, fmt.Sprintf("%s semantic match (%.2f)", strength, breakdown.VectorSimilarity))
	}

	// Activation from spreading
	if breakdown.Activation > 0.01 {
		strength := describeStrength(breakdown.Activation, 0.1, 0.2)
		parts = append(parts, fmt.Sprintf("%s graph activation (%.2f)", strength, breakdown.Activation))
	}

	// Learning edge boost
	if breakdown.LearningEdgeBoost > 0.01 {
		parts = append(parts, fmt.Sprintf("learning edge boost (+%.2f)", breakdown.LearningEdgeBoost))
	}

	// Path boost
	if breakdown.PathBoost > 0.01 {
		parts = append(parts, fmt.Sprintf("path pattern match (+%.2f)", breakdown.PathBoost))
	}

	// Comparison boost
	if breakdown.ComparisonBoost > 0.01 {
		parts = append(parts, fmt.Sprintf("comparison query match (+%.2f)", breakdown.ComparisonBoost))
	}

	// Config boost
	if breakdown.ConfigBoost > 0.01 {
		parts = append(parts, fmt.Sprintf("config node boost (+%.2f)", breakdown.ConfigBoost))
	}

	// Temporal boost
	if breakdown.TemporalBoost > 0.01 {
		parts = append(parts, fmt.Sprintf("temporal recency boost (+%.2f)", breakdown.TemporalBoost))
	}

	// Value residual bypass
	if breakdown.BypassBonus > 0.01 {
		parts = append(parts, fmt.Sprintf("high-confidence vector bypass (+%.2f)", breakdown.BypassBonus))
	}

	// Stale reference penalty (Phase 2 Temporal)
	if breakdown.StaleRefPenalty < -0.01 {
		parts = append(parts, fmt.Sprintf("stale reference penalty (%.2f)", breakdown.StaleRefPenalty))
	}

	// Rerank delta (if LLM reranking affected position)
	if breakdown.RerankDelta > 0.01 {
		parts = append(parts, fmt.Sprintf("LLM rerank boost (+%.2f)", breakdown.RerankDelta))
	} else if breakdown.RerankDelta < -0.01 {
		parts = append(parts, fmt.Sprintf("LLM rerank adjustment (%.2f)", breakdown.RerankDelta))
	}

	// Penalties (only mention if significant)
	if breakdown.HubPenalty < -0.02 {
		parts = append(parts, fmt.Sprintf("hub penalty (%.2f)", breakdown.HubPenalty))
	}
	if breakdown.RedundancyPenalty < -0.02 {
		parts = append(parts, fmt.Sprintf("redundancy penalty (%.2f)", breakdown.RedundancyPenalty))
	}

	// Compose rationale
	rationale := "No significant signals"
	if len(parts) > 0 {
		rationale = strings.Join(parts, " + ")
	}

	// Calculate confidence based on signal clarity
	confidence := calculateConfidence(breakdown)

	// Convert breakdown to map for JSON
	breakdownMap := map[string]float64{
		"vector_similarity":   breakdown.VectorSimilarity,
		"activation":          breakdown.Activation,
		"recency":             breakdown.Recency,
		"confidence":          breakdown.Confidence,
		"path_boost":          breakdown.PathBoost,
		"comparison_boost":    breakdown.ComparisonBoost,
		"config_boost":        breakdown.ConfigBoost,
		"hub_penalty":         breakdown.HubPenalty,
		"redundancy_penalty":  breakdown.RedundancyPenalty,
		"rerank_delta":        breakdown.RerankDelta,
		"learning_edge_boost": breakdown.LearningEdgeBoost,
		"temporal_boost":      breakdown.TemporalBoost,
		"stale_ref_penalty":   breakdown.StaleRefPenalty,
		"bypass_bonus":        breakdown.BypassBonus,
		"final_score":         breakdown.FinalScore,
	}

	return JiminyExplanation{
		Rationale:      rationale,
		Confidence:     confidence,
		RetrievalPath:  path,
		ScoreBreakdown: breakdownMap,
	}
}

// describeStrength returns a descriptor based on value relative to thresholds
func describeStrength(value, medium, strong float64) string {
	if value >= strong {
		return "Strong"
	}
	if value >= medium {
		return "Moderate"
	}
	return "Weak"
}

// calculateConfidence computes an overall confidence score (0-1) based on signal clarity.
// High confidence = strong primary signal (vector sim) and few penalties.
// Low confidence = weak signals or heavy penalties.
func calculateConfidence(breakdown ScoreBreakdown) float64 {
	// Start with vector similarity as the base confidence
	conf := breakdown.VectorSimilarity

	// Boost for activation (shows graph connectivity)
	if breakdown.Activation > 0.1 {
		conf += 0.1
	}

	// Boost for learning edges (shows historical relevance)
	if breakdown.LearningEdgeBoost > 0.01 {
		conf += 0.05
	}

	// Boost for path/comparison matches (explicit query alignment)
	if breakdown.PathBoost > 0.01 || breakdown.ComparisonBoost > 0.01 {
		conf += 0.05
	}

	// Reduce for heavy penalties
	if breakdown.HubPenalty < -0.1 {
		conf -= 0.1
	}
	if breakdown.RedundancyPenalty < -0.1 {
		conf -= 0.05
	}

	// Clamp to [0, 1]
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}

	return conf
}

// DetermineRetrievalPath builds the list of stages that contributed to this result.
func DetermineRetrievalPath(breakdown ScoreBreakdown, wasReranked bool) []string {
	path := []string{string(StageVectorRecall)} // Always present

	if breakdown.Activation > 0.01 {
		path = append(path, string(StageSpreadingActivation))
	}

	if breakdown.LearningEdgeBoost > 0.01 {
		path = append(path, string(StageLearningEdge))
	}

	if breakdown.PathBoost > 0.01 {
		path = append(path, string(StagePathBoost))
	}

	if breakdown.ComparisonBoost > 0.01 {
		path = append(path, string(StageComparisonBoost))
	}

	if breakdown.ConfigBoost > 0.01 {
		path = append(path, string(StageConfigBoost))
	}

	if breakdown.TemporalBoost > 0.01 {
		path = append(path, string(StageTemporalBoost))
	}

	if breakdown.StaleRefPenalty < -0.01 {
		path = append(path, string(StageStaleRefPenalty))
	}

	if wasReranked {
		path = append(path, string(StageLLMRerank))
	}

	return path
}
