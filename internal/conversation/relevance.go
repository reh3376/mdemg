package conversation

import (
	"math"
	"time"
)

// RelevanceWeights defines the weights for each scoring dimension
type RelevanceWeights struct {
	Recency       float64 `json:"recency"`
	Importance    float64 `json:"importance"`
	TaskRelevance float64 `json:"task_relevance"`
}

// DefaultRelevanceWeights returns the default weights for relevance scoring
func DefaultRelevanceWeights() *RelevanceWeights {
	return &RelevanceWeights{
		Recency:       0.3,
		Importance:    0.4,
		TaskRelevance: 0.3,
	}
}

// RelevanceScorer scores observations for resume prioritization
type RelevanceScorer struct {
	weights           *RelevanceWeights
	recencyHalfLife   time.Duration // Time after which recency score halves
	importanceDecay   float64       // Decay rate for importance
	now               time.Time     // Reference time for scoring
	queryEmbedding    []float32     // Optional: embedding for task-relevance
}

// NewRelevanceScorer creates a scorer with default settings
func NewRelevanceScorer() *RelevanceScorer {
	return &RelevanceScorer{
		weights:         DefaultRelevanceWeights(),
		recencyHalfLife: 24 * time.Hour, // Score halves after 1 day
		importanceDecay: 0.1,            // 10% decay per reinforcement cycle
		now:             time.Now(),
	}
}

// WithWeights sets custom weights
func (s *RelevanceScorer) WithWeights(weights *RelevanceWeights) *RelevanceScorer {
	if weights != nil {
		s.weights = weights
	}
	return s
}

// WithRecencyHalfLife sets the half-life for recency decay
func (s *RelevanceScorer) WithRecencyHalfLife(halfLife time.Duration) *RelevanceScorer {
	if halfLife > 0 {
		s.recencyHalfLife = halfLife
	}
	return s
}

// WithReferenceTime sets the reference time for scoring
func (s *RelevanceScorer) WithReferenceTime(t time.Time) *RelevanceScorer {
	s.now = t
	return s
}

// WithQueryEmbedding sets the query embedding for task-relevance scoring
func (s *RelevanceScorer) WithQueryEmbedding(embedding []float32) *RelevanceScorer {
	s.queryEmbedding = embedding
	return s
}

// Score calculates the overall relevance score for an observation
func (s *RelevanceScorer) Score(obs *Observation) float64 {
	recencyScore := s.scoreRecency(obs)
	importanceScore := s.scoreImportance(obs)
	taskRelevanceScore := s.scoreTaskRelevance(obs)

	// Weighted combination
	total := (s.weights.Recency * recencyScore) +
		(s.weights.Importance * importanceScore) +
		(s.weights.TaskRelevance * taskRelevanceScore)

	// Normalize to ensure weights sum to 1
	weightSum := s.weights.Recency + s.weights.Importance + s.weights.TaskRelevance
	if weightSum > 0 {
		total = total / weightSum
	}

	return clamp(total, 0.0, 1.0)
}

// scoreRecency calculates recency score using exponential decay
// Returns 1.0 for very recent, decaying toward 0 for older observations
func (s *RelevanceScorer) scoreRecency(obs *Observation) float64 {
	if obs.CreatedAt.IsZero() {
		return 0.5 // Default for missing timestamp
	}

	age := s.now.Sub(obs.CreatedAt)
	if age < 0 {
		age = 0 // Future timestamps treated as now
	}

	// Exponential decay: score = e^(-age/halfLife * ln(2))
	// This gives score of 0.5 at half-life
	halfLifeSeconds := s.recencyHalfLife.Seconds()
	if halfLifeSeconds <= 0 {
		halfLifeSeconds = 86400 // Default 1 day
	}

	ageSeconds := age.Seconds()
	decayConstant := math.Log(2) / halfLifeSeconds
	score := math.Exp(-decayConstant * ageSeconds)

	return clamp(score, 0.0, 1.0)
}

// scoreImportance calculates importance score based on obs_type and metadata
func (s *RelevanceScorer) scoreImportance(obs *Observation) float64 {
	// Start with base importance from observation type
	baseScore := getTypeImportance(obs.ObsType)

	// Use stored importance score if available (already calculated)
	if obs.ImportanceScore > 0 {
		baseScore = obs.ImportanceScore
	}

	// Boost for high surprise score (novel information)
	surpriseBoost := obs.SurpriseScore * 0.2 // Up to 20% boost

	// Boost for stability (well-reinforced observations)
	stabilityBoost := obs.StabilityScore * 0.1 // Up to 10% boost

	// Penalty for volatile (unconfirmed) observations
	volatilePenalty := 0.0
	if obs.Volatile {
		volatilePenalty = 0.2 // 20% penalty
	}

	// Access recency boost
	accessBoost := 0.0
	if !obs.LastAccessedAt.IsZero() {
		accessAge := s.now.Sub(obs.LastAccessedAt)
		if accessAge < 24*time.Hour {
			accessBoost = 0.1 // 10% boost if accessed recently
		}
	}

	score := baseScore + surpriseBoost + stabilityBoost + accessBoost - volatilePenalty

	return clamp(score, 0.0, 1.0)
}

// scoreTaskRelevance calculates relevance to current task context
func (s *RelevanceScorer) scoreTaskRelevance(obs *Observation) float64 {
	// If no query embedding, use template and tier-based scoring
	if s.queryEmbedding == nil {
		return s.scoreTaskRelevanceHeuristic(obs)
	}

	// If observation has no embedding, fall back to heuristic
	if len(obs.Embedding) == 0 {
		return s.scoreTaskRelevanceHeuristic(obs)
	}

	// Cosine similarity between query and observation embeddings
	similarity := cosineSimilarity(s.queryEmbedding, obs.Embedding)

	// Convert similarity (-1 to 1) to score (0 to 1)
	score := (similarity + 1.0) / 2.0

	return clamp(score, 0.0, 1.0)
}

// scoreTaskRelevanceHeuristic uses template and tier for relevance when embeddings unavailable
func (s *RelevanceScorer) scoreTaskRelevanceHeuristic(obs *Observation) float64 {
	score := 0.5 // Default medium relevance

	// Boost for task-related templates
	switch obs.TemplateID {
	case "task_handoff":
		score = 0.95 // Highly relevant for task continuity
	case "decision":
		score = 0.85 // Important for understanding context
	case "error":
		score = 0.80 // Need to know what went wrong
	case "correction":
		score = 0.90 // Critical to avoid repeating mistakes
	case "learning":
		score = 0.70 // Useful background
	}

	// Boost for critical tier
	switch obs.Tier {
	case "critical":
		score = math.Max(score, 0.9)
	case "important":
		score = math.Max(score, 0.7)
	case "background":
		score = math.Min(score, 0.5)
	}

	return clamp(score, 0.0, 1.0)
}

// getTypeImportance returns base importance for observation types
func getTypeImportance(obsType ObservationType) float64 {
	switch obsType {
	case ObsTypeCorrection:
		return 0.9 // Critical - don't repeat mistakes
	case ObsTypeDecision:
		return 0.8 // Important context
	case ObsTypeError:
		return 0.75 // Good to know
	case ObsTypeBlocker:
		return 0.85 // May still be relevant
	case ObsTypeProgress:
		return 0.6 // Useful but not critical
	case ObsTypeLearning:
		return 0.7 // Build understanding
	case ObsTypePreference:
		return 0.65 // Should respect
	case ObsTypeContext:
		return 0.7 // Background info
	case ObsTypeTechnicalNote:
		return 0.6 // Reference material
	case ObsTypeInsight:
		return 0.75 // Valuable discovery
	case ObsTypeTask:
		return 0.8 // Task tracking
	case ObsTypeConstraint:
		return 0.9 // Highest priority rules
	case ObsTypeSelfImprovement:
		return 0.55 // RSIC cycle data
	case ObsTypeNote:
		return 0.5 // Free-form memos
	case ObsTypeContextSignal:
		return 0.2 // Background telemetry
	default:
		return 0.5 // Unknown type
	}
}

// Note: cosineSimilarity is defined in surprise.go

// clamp restricts a value to a range
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// =============================================================================
// Tier Classification
// =============================================================================

// Tier represents observation importance tier
type Tier string

const (
	TierCritical   Tier = "critical"
	TierImportant  Tier = "important"
	TierBackground Tier = "background"
)

// ClassifyTier determines the tier based on relevance score
func ClassifyTier(score float64) Tier {
	switch {
	case score >= 0.8:
		return TierCritical
	case score >= 0.5:
		return TierImportant
	default:
		return TierBackground
	}
}

// TierFromType determines initial tier based on observation type
func TierFromType(obsType ObservationType) Tier {
	switch obsType {
	case ObsTypeCorrection, ObsTypeError, ObsTypeBlocker, ObsTypeConstraint:
		return TierCritical
	case ObsTypeDecision, ObsTypeTask, ObsTypeInsight:
		return TierImportant
	default:
		return TierBackground
	}
}

// =============================================================================
// Batch Scoring
// =============================================================================

// ScoreObservations scores a batch of observations and returns them sorted by score
func (s *RelevanceScorer) ScoreObservations(observations []*Observation) []ScoredObservation {
	scored := make([]ScoredObservation, len(observations))
	for i, obs := range observations {
		score := s.Score(obs)
		scored[i] = ScoredObservation{
			Observation:    obs,
			RelevanceScore: score,
			Tier:           ClassifyTier(score),
		}
	}

	// Sort by score descending
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].RelevanceScore > scored[i].RelevanceScore {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored
}

// ScoredObservation pairs an observation with its relevance score
type ScoredObservation struct {
	Observation    *Observation `json:"observation"`
	RelevanceScore float64      `json:"relevance_score"`
	Tier           Tier         `json:"tier"`
}

// =============================================================================
// Resume Options (Extended)
// =============================================================================

// ResumeStrategy defines how resume should select observations
type ResumeStrategy string

const (
	ResumeTaskFocused   ResumeStrategy = "task_focused"   // Prioritize task context
	ResumeComprehensive ResumeStrategy = "comprehensive"  // Include more observations
	ResumeMinimal       ResumeStrategy = "minimal"        // Only critical items
)

// ResumeOptions configures enhanced resume behavior
type ResumeOptions struct {
	MaxTokens        int               `json:"max_tokens,omitempty"`
	IncludeTemplates []string          `json:"include_templates,omitempty"`
	RelevanceWeights *RelevanceWeights `json:"relevance_weights,omitempty"`
	QueryContext     string            `json:"query_context,omitempty"`
	Tiered           bool              `json:"tiered,omitempty"`
}

// EnhancedResumeRequest extends the resume request
type EnhancedResumeRequest struct {
	SpaceID   string         `json:"space_id"`
	SessionID string         `json:"session_id"`
	Strategy  ResumeStrategy `json:"strategy,omitempty"`
	Options   *ResumeOptions `json:"options,omitempty"`
}

// EnhancedResumeResponse extends the resume response
type EnhancedResumeResponse struct {
	SessionID           string                `json:"session_id"`
	Observations        []EnhancedObservation `json:"observations"`
	SummaryObservations []EnhancedObservation `json:"summary_observations,omitempty"`
	TokenCount          int                   `json:"token_count"`
	TokenBudget         int                   `json:"token_budget"`
	OmittedCount        int                   `json:"omitted_count"`
	ContinuityContext   *ContinuityContext    `json:"continuity_context,omitempty"`
}

// EnhancedObservation adds resume-specific fields to observation output
type EnhancedObservation struct {
	ObsID          string                 `json:"obs_id"`
	Tier           string                 `json:"tier"`
	ObsType        string                 `json:"obs_type"`
	TemplateID     string                 `json:"template_id,omitempty"`
	Content        string                 `json:"content"`
	StructuredData map[string]interface{} `json:"structured_data,omitempty"`
	RelevanceScore float64                `json:"relevance_score"`
	CreatedAt      time.Time              `json:"created_at"`
	Truncated      bool                   `json:"truncated"`
	Summarizes     []string               `json:"summarizes,omitempty"`
}

// ContinuityContext provides session continuity information
type ContinuityContext struct {
	LastSessionID string    `json:"last_session_id"`
	LastActivity  time.Time `json:"last_activity"`
	ActiveTask    string    `json:"active_task,omitempty"`
}
