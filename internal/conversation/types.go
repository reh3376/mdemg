package conversation

import "time"

// ObservationType represents what kind of observation this is
type ObservationType string

const (
	ObsTypeDecision      ObservationType = "decision"
	ObsTypeCorrection    ObservationType = "correction"
	ObsTypeLearning      ObservationType = "learning"
	ObsTypePreference    ObservationType = "preference"
	ObsTypeError         ObservationType = "error"
	ObsTypeTask          ObservationType = "task"
	ObsTypeTechnicalNote ObservationType = "technical_note" // Technical documentation/notes
	ObsTypeInsight       ObservationType = "insight"        // Insights and discoveries
	ObsTypeContext       ObservationType = "context"        // Context/background information
	ObsTypeProgress      ObservationType = "progress"       // Progress updates
	ObsTypeBlocker         ObservationType = "blocker"          // Blockers/issues encountered
	ObsTypeContextSignal   ObservationType = "context_signal"   // Lightweight hook-generated telemetry (file reads, searches)
	ObsTypeNote            ObservationType = "note"             // Free-form user annotation / memo
	ObsTypeConstraint      ObservationType = "constraint"       // Explicitly recorded constraints ("must", "never", "always")
	ObsTypeSelfImprovement ObservationType = "self_improvement" // RSIC cycle outcomes, calibration results
)

// Visibility levels for multi-tenant memory access
type Visibility string

const (
	VisibilityPrivate Visibility = "private" // Only visible to owner (user_id match)
	VisibilityTeam    Visibility = "team"    // Visible to users in same space_id
	VisibilityGlobal  Visibility = "global"  // Visible to all
)

// ValidVisibility checks if a visibility value is valid
func ValidVisibility(v string) bool {
	switch Visibility(v) {
	case VisibilityPrivate, VisibilityTeam, VisibilityGlobal:
		return true
	case "": // Empty defaults to private
		return true
	}
	return false
}

// DefaultStabilityScore is the initial stability for new volatile observations
const DefaultStabilityScore = 0.1

// GraduationStabilityThreshold is the stability score needed to graduate from volatile
const GraduationStabilityThreshold = 0.8

// Observation represents a significant conversational event
type Observation struct {
	ObsID         string
	SpaceID       string
	SessionID     string
	ObsType       ObservationType
	Content       string
	Summary       string // Auto-generated summary
	Embedding     []float32
	SurpriseScore float64 // 0.0-1.0
	Tags          []string
	Metadata      map[string]any
	CreatedAt     time.Time

	// Identity & Visibility (CMS v2)
	UserID         string     // Owner of the observation
	Visibility     Visibility // private|team|global (default: private)
	Volatile       bool       // True for unreinforced short-term thoughts
	StabilityScore float64    // 0..1, managed by Context Cooler
	Pinned         bool       // Pinned observations are permanent and protected from decay

	// Multi-Agent Identity (CMS v3)
	AgentID string // Persistent agent identity (survives across sessions)

	// Structured Observations (Phase 60)
	TemplateID     string         // Template ID used for this observation
	StructuredData map[string]any // Template-validated structured data

	// Resume Optimization (Phase 60)
	ImportanceScore float64 // 0.0-1.0, for relevance-based resume
	Tier            string  // critical, important, background
	LastAccessedAt  time.Time

	// Org-Level Review (Phase 60)
	OrgReviewStatus string    // none, pending, approved, rejected
	OrgFlaggedAt    time.Time // When flagged for org review
	OrgFlaggedBy    string    // Who flagged it

	// Phase 14.2 — Sparse context fingerprint (Note 05).
	// ContextFingerprintActive is the sorted set of bit positions whose value
	// is 1 in the 256-bit fingerprint vector for this observation. Empty when
	// no active ContextCatalog exists for the space (cold-start fallback).
	// ContextFingerprintVersion identifies the catalog version that produced
	// the fingerprint; 0 means cold-start (no catalog at observe time).
	ContextFingerprintActive  []uint16
	ContextFingerprintVersion int
}

// SurpriseFactors breaks down the surprise score
type SurpriseFactors struct {
	TermNovelty        float64 // Domain-specific terms
	ContradictionScore float64 // Conflicts with known facts
	CorrectionScore    float64 // User explicitly corrected
	EmbeddingNovelty   float64 // Distance from known concepts
}
