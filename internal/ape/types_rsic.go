package ape

import (
	"context"
	"time"
)

// ───────────── Tier ─────────────

// CycleTier represents the granularity of an RSIC cycle.
type CycleTier string

const (
	TierMicro CycleTier = "micro" // per-request opportunistic
	TierMeso  CycleTier = "meso"  // periodic (hours/sessions)
	TierMacro CycleTier = "macro" // cron-scheduled deep maintenance
)

// ───────────── Trigger Source (Phase 87) ─────────────

// TriggerSource identifies what initiated an RSIC cycle.
type TriggerSource string

const (
	TriggerManualAPI        TriggerSource = "manual_api"
	TriggerMicroAuto        TriggerSource = "micro_auto"
	TriggerSessionPeriodic  TriggerSource = "session_periodic"
	TriggerMacroCron        TriggerSource = "macro_cron"
	TriggerWatchdogForce    TriggerSource = "watchdog_force"
)

// PolicyVersion is the current orchestration policy version.
const PolicyVersion = "phase87-v1"

// SafetyVersion is the current safety enforcement version.
const SafetyVersion = "phase88-v1"

// DestructiveActions lists action types that delete or archive data.
var DestructiveActions = map[string]bool{
	"prune_decayed_edges": true,
	"prune_excess_edges":  true,
	"tombstone_stale":     true,
}

// IsDestructiveAction returns true if the action can delete or archive data.
func IsDestructiveAction(actionType string) bool {
	return DestructiveActions[actionType]
}

// ValidTriggerSources maps valid source strings for input validation.
var ValidTriggerSources = map[string]TriggerSource{
	"manual_api":        TriggerManualAPI,
	"micro_auto":        TriggerMicroAuto,
	"session_periodic":  TriggerSessionPeriodic,
	"macro_cron":        TriggerMacroCron,
	"watchdog_force":    TriggerWatchdogForce,
}

// Priority returns the trigger priority (lower = higher priority).
// watchdog_force=0, manual_api=1, macro_cron=2, session_periodic=3, micro_auto=4.
func (ts TriggerSource) Priority() int {
	switch ts {
	case TriggerWatchdogForce:
		return 0
	case TriggerManualAPI:
		return 1
	case TriggerMacroCron:
		return 2
	case TriggerSessionPeriodic:
		return 3
	case TriggerMicroAuto:
		return 4
	default:
		return 99
	}
}

// AllowedTiers returns the tiers this trigger source is allowed to initiate.
func (ts TriggerSource) AllowedTiers() []CycleTier {
	switch ts {
	case TriggerManualAPI:
		return []CycleTier{TierMicro, TierMeso, TierMacro}
	case TriggerMicroAuto:
		return []CycleTier{TierMicro}
	case TriggerSessionPeriodic:
		return []CycleTier{TierMeso}
	case TriggerMacroCron:
		return []CycleTier{TierMacro}
	case TriggerWatchdogForce:
		return []CycleTier{TierMeso}
	default:
		return nil
	}
}

// TriggerMetadata carries provenance information for a cycle execution.
type TriggerMetadata struct {
	TriggerSource  TriggerSource `json:"trigger_source"`
	TriggerID      string        `json:"trigger_id"`
	TriggeredAt    time.Time     `json:"triggered_at"`
	PolicyVersion  string        `json:"policy_version"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"`
}

// ───────────── Assessment ─────────────

// SelfAssessmentReport contains the output of the Assess stage.
type SelfAssessmentReport struct {
	SpaceID   string    `json:"space_id"`
	Tier      CycleTier `json:"tier"`
	Timestamp time.Time `json:"timestamp"`

	// Sub-scores (0–1, higher = healthier)
	RetrievalQuality float64 `json:"retrieval_quality"`
	TaskPerformance  float64 `json:"task_performance"`
	MemoryHealth     float64 `json:"memory_health"`
	EdgeHealth       float64 `json:"edge_health"`
	GuidanceHealth   float64 `json:"guidance_health"` // J10: Jiminy guidance effectiveness
	ProtocolHealth   float64 `json:"protocol_health"` // J17: protocol encoding effectiveness

	// Derived
	OverallHealth float64 `json:"overall_health"`
	Confidence    float64 `json:"confidence"`

	// Raw details exposed for reflection
	LearningPhase        string  `json:"learning_phase"`
	EdgeCount            int64   `json:"edge_count"`
	OrphanCount          int64   `json:"orphan_count"`
	TotalNodes           int64   `json:"total_nodes"`
	OrphanRatio          float64 `json:"orphan_ratio"`
	CorrectionRate       float64 `json:"correction_rate"`
	ConsolidationAgeSec  int64   `json:"consolidation_age_sec"`
	VolatileCount        int     `json:"volatile_count"`
	PermanentCount       int     `json:"permanent_count"`
	AvgEdgeWeight        float64 `json:"avg_edge_weight"`
	EdgesBelowThreshold  int64   `json:"edges_below_threshold"`
	EdgeWeightEntropy    float64 `json:"edge_weight_entropy"`
}

// ───────────── Reflection ─────────────

// InsightSeverity ranks the urgency of a reflection insight.
type InsightSeverity string

const (
	SeverityLow      InsightSeverity = "low"
	SeverityMedium   InsightSeverity = "medium"
	SeverityHigh     InsightSeverity = "high"
	SeverityCritical InsightSeverity = "critical"
)

// ReflectionInsight is a single observation produced by the Reflect stage.
type ReflectionInsight struct {
	PatternID         string          `json:"pattern_id"`
	Severity          InsightSeverity `json:"severity"`
	Description       string          `json:"description"`
	RecommendedAction string          `json:"recommended_action"` // action type key
	Metric            string          `json:"metric"`
	Value             float64         `json:"value"`
	Threshold         float64         `json:"threshold"`
}

// ───────────── Improvement Actions ─────────────

// ImprovementAction maps an insight to a concrete action.
type ImprovementAction struct {
	ActionType string    `json:"action_type"`
	TargetSpace string   `json:"target_space"`
	Scope       string   `json:"scope"` // "space" | "global"
	Priority    int      `json:"priority"`
	Rationale   string   `json:"rationale"`
}

// ───────────── Task Spec ─────────────

// EndpointSpec declares an API endpoint a task is allowed to call.
type EndpointSpec struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// SafetyBounds constrains the blast radius of a task.
type SafetyBounds struct {
	MaxNodesAffected int      `json:"max_nodes_affected"`
	MaxEdgesAffected int      `json:"max_edges_affected"`
	ProtectedSpaces  []string `json:"protected_spaces"`
	DryRun           bool     `json:"dry_run"`
}

// Deliverable describes an expected output from a task.
type Deliverable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Format      string `json:"format"` // "json" | "text"
	Required    bool   `json:"required"`
}

// Criterion defines a success metric check.
type Criterion struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"` // "gte" | "lte" | "eq" | "gt" | "lt"
	Threshold float64 `json:"threshold"`
}

// ReportSchedule configures when progress reports are emitted.
type ReportSchedule struct {
	IntervalType string   `json:"interval_type"` // "time" | "progress" | "milestone"
	Values       []string `json:"values"`        // durations or milestone names
}

// RSICTaskSpec is the standardised specification for a single improvement task.
type RSICTaskSpec struct {
	// Identity
	TaskID  string `json:"task_id"`
	CycleID string `json:"cycle_id"`

	// Purpose
	ActionType  string `json:"action_type"`
	Description string `json:"description"`
	Rationale   string `json:"rationale"`

	// Scope
	TargetSpace string `json:"target_space"`

	// Permissions
	AllowedEndpoints []EndpointSpec `json:"allowed_endpoints"`
	Safety           SafetyBounds   `json:"safety"`

	// Deliverables
	Deliverables    []Deliverable  `json:"deliverables"`
	SuccessCriteria []Criterion    `json:"success_criteria"`

	// Reporting
	ReportSchedule ReportSchedule `json:"report_schedule"`

	// Execution limits
	Timeout         time.Duration  `json:"timeout"`
	BaselineMetrics map[string]float64 `json:"baseline_metrics"`
	Priority        int            `json:"priority"`
}

// ───────────── Progress & Outcome ─────────────

// RSICProgressReport is emitted by a running task at milestones.
type RSICProgressReport struct {
	TaskID       string             `json:"task_id"`
	CycleID      string             `json:"cycle_id"`
	Status       string             `json:"status"` // "running" | "completed" | "failed"
	ProgressPct  float64            `json:"progress_pct"`
	Milestone    string             `json:"milestone"`
	Summary      string             `json:"summary"`
	MetricsDelta map[string]float64 `json:"metrics_delta,omitempty"`
	Deliverables map[string]any     `json:"deliverables,omitempty"`
	Timestamp    time.Time          `json:"timestamp"`
	Error        string             `json:"error,omitempty"`
}

// CycleOutcome summarises a completed RSIC cycle.
type CycleOutcome struct {
	CycleID          string             `json:"cycle_id"`
	Tier             CycleTier          `json:"tier"`
	SpaceID          string             `json:"space_id"`
	StartedAt        time.Time          `json:"started_at"`
	CompletedAt      time.Time          `json:"completed_at"`
	ActionsExecuted  int                `json:"actions_executed"`
	SuccessCount     int                `json:"success_count"`
	FailedCount      int                `json:"failed_count"`
	MetricsBefore    map[string]float64 `json:"metrics_before"`
	MetricsAfter     map[string]float64 `json:"metrics_after"`
	CalibrationDelta map[string]float64 `json:"calibration_delta,omitempty"`
	Insights         []ReflectionInsight `json:"insights,omitempty"`
	Error            string             `json:"error,omitempty"`

	// Phase 87: Trigger provenance
	TriggerSource  TriggerSource `json:"trigger_source,omitempty"`
	TriggerID      string        `json:"trigger_id,omitempty"`
	TriggeredAt    time.Time     `json:"triggered_at,omitempty"`
	PolicyVersion  string        `json:"policy_version,omitempty"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"`

	// Phase 88: Safety enforcement
	DryRun         bool            `json:"dry_run,omitempty"`
	SafetyVersion  string          `json:"safety_version,omitempty"`
	SafetySummary  *SafetySummary  `json:"safety_summary,omitempty"`
	Deltas         []ActionDelta   `json:"deltas,omitempty"`

	// Phase AR-1: Feedback loop — criteria evaluation
	CriteriaMet    bool              `json:"criteria_met"`
	CriteriaDetail map[string]string `json:"criteria_detail,omitempty"`
}

// SafetySummary records safety enforcement results for a cycle.
type SafetySummary struct {
	ActionsChecked  int               `json:"actions_checked"`
	ActionsAllowed  int               `json:"actions_allowed"`
	ActionsRejected int               `json:"actions_rejected"`
	Rejections      []SafetyRejection `json:"rejections,omitempty"`
	SnapshotsCreated int              `json:"snapshots_created"`
}

// SafetyRejection records why an action was rejected by the safety validator.
type SafetyRejection struct {
	Action            string `json:"action"`
	Reason            string `json:"reason"`
	EstimatedAffected int    `json:"estimated_affected"`
	Limit             int    `json:"limit"`
}

// ActionDelta describes what a dry-run action would do without executing.
type ActionDelta struct {
	Action               string `json:"action"`
	WouldExecute         bool   `json:"would_execute"`
	EstimatedAffected    int    `json:"estimated_affected"`
	SafetyLimit          int    `json:"safety_limit"`
	WithinBounds         bool   `json:"within_bounds"`
	ProtectedSpaceBlocked bool  `json:"protected_space_blocked"`
	RejectionReason      string `json:"rejection_reason,omitempty"`
	Note                 string `json:"note,omitempty"`
}

// ───────────── Watchdog ─────────────

// EscalationLevel represents the watchdog's urgency state.
type EscalationLevel int

const (
	EscalationNominal EscalationLevel = 0 // all good
	EscalationNudge   EscalationLevel = 1 // gentle log
	EscalationWarn    EscalationLevel = 2 // warning log
	EscalationForce   EscalationLevel = 3 // auto-trigger
)

// WatchdogState tracks the decay watchdog's current state.
type WatchdogState struct {
	DecayScore         float64         `json:"decay_score"`
	EscalationLevel    EscalationLevel `json:"escalation_level"`
	LastCycleTime      time.Time       `json:"last_cycle_time"`
	NextDue            time.Time       `json:"next_due"`
	SpaceID            string          `json:"space_id"`
	SessionHealthScore float64         `json:"session_health_score"`
	ObsRatePerHour     float64         `json:"obs_rate_per_hour"`
	ActiveAnomalies    []string        `json:"active_anomalies,omitempty"`
	ConsolidationAge   int64           `json:"consolidation_age_sec"`
	LastTriggerSource  TriggerSource   `json:"last_trigger_source,omitempty"`
}

// ───────────── Provider Interfaces ─────────────
// These interfaces decouple RSIC from concrete service implementations.

// LearningStatsProvider exposes learning-edge operations needed by RSIC.
type LearningStatsProvider interface {
	GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error)
	PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error)
	PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error)
}

// ConversationStatsProvider exposes CMS stats and observation recording.
type ConversationStatsProvider interface {
	GetVolatileStats(ctx context.Context, spaceID string) (VolatileStatsResult, error)
}

// VolatileStatsResult mirrors the subset of conversation.VolatileStats we need.
type VolatileStatsResult struct {
	VolatileCount        int     `json:"volatile_count"`
	PermanentCount       int     `json:"permanent_count"`
	AvgVolatileStability float64 `json:"avg_volatile_stability"`
}

// HiddenLayerProvider exposes consolidation triggers.
type HiddenLayerProvider interface {
	RunFullConversationConsolidation(ctx context.Context, spaceID string) (any, error)
}

// JiminyStatsProvider exposes guidance metrics for RSIC assessment (J10).
type JiminyStatsProvider interface {
	GetGuidanceStats(ctx context.Context, spaceID string) (JiminyStatsResult, error)
}

// JiminyStatsResult mirrors jiminy.JiminyStats for RSIC use.
type JiminyStatsResult struct {
	TotalGuidanceIssued int     `json:"total_guidance_issued"`
	TotalFollowed       int     `json:"total_followed"`
	TotalIgnored        int     `json:"total_ignored"`
	TotalContradicted   int     `json:"total_contradicted"`
	FollowRate          float64 `json:"follow_rate"`
	ConstraintEffRate   float64 `json:"constraint_effectiveness_rate"`
	SourceDiversity     float64 `json:"source_diversity"`
}

// ProtocolStatsProvider exposes J17 protocol metrics for RSIC assessment.
type ProtocolStatsProvider interface {
	GetProtocolStats(ctx context.Context, spaceID string) (ProtocolStatsResult, error)
}

// ProtocolStatsResult carries J17 protocol metrics for RSIC assessment.
type ProtocolStatsResult struct {
	TierDistribution         [3]float64                 `json:"tier_distribution"`
	CompressionRatio         float64                    `json:"compression_ratio"`
	AvgComprehension         float64                    `json:"avg_comprehension"`
	ReplayFrequencyPerHour   float64                    `json:"replay_frequency_per_hour"`
	TicketRestoreSuccessRate float64                    `json:"ticket_restore_success_rate"`
	CodeCoverage             float64                    `json:"code_coverage"`
	TotalEvents              int64                      `json:"total_events"`
	T2FrequencyByConstraint  map[string]int             `json:"t2_frequency_by_constraint,omitempty"`
	CodeComprehension        map[string]float64         `json:"code_comprehension,omitempty"`

	// Per-tier comprehension (NLI feedback loop)
	TierComprehension     [3]float64                 `json:"tier_comprehension"`
	TierCodeComprehension map[int]map[string]float64 `json:"tier_code_comprehension,omitempty"`
	TierOutcomeCount      [3]int64                   `json:"tier_outcome_count"`

	// NLI calibration (Epic 3)
	NLIMeanBias  float64 `json:"nli_mean_bias"`
	NLIBiasAlert bool    `json:"nli_bias_alert"`
}

// TierEffectivenessProvider builds curated tier effectiveness datasets for RSIC.
type TierEffectivenessProvider interface {
	BuildDataset() map[string]any
}

// ProtocolEvolverProvider executes protocol mutations proposed by RSIC.
type ProtocolEvolverProvider interface {
	CodifyConstraint(ctx context.Context, spaceID string, constraintNodeID string) (map[string]any, error)
	RetireCode(ctx context.Context, spaceID string, constraintCode string) (map[string]any, error)
	AdjustTierThresholds(ctx context.Context, spaceID string) (map[string]any, error)
	AdjustReplayBuffer(ctx context.Context, spaceID string) (map[string]any, error)
}

// GuidanceCalibrationProvider exposes guidance effectiveness and confidence
// calibration operations for RSIC-SK1 self-calibrating guidance.
type GuidanceCalibrationProvider interface {
	GetConstraintEffectiveness(ctx context.Context, spaceID string) ([]GuidanceEffectivenessItem, error)
	UpdateNodeConfidence(ctx context.Context, nodeID string, outcome string) error
	ArchiveStaleConstraints(ctx context.Context, spaceID string) (int, error)
}

// GuidanceEffectivenessItem carries per-constraint effectiveness metrics for RSIC calibration.
type GuidanceEffectivenessItem struct {
	NodeID            string  `json:"node_id"`
	Name              string  `json:"name"`
	Confidence        float64 `json:"confidence"`
	TotalSurfaced     int     `json:"total_surfaced"`
	TotalFollowed     int     `json:"total_followed"`
	TotalIgnored      int     `json:"total_ignored"`
	TotalContradicted int     `json:"total_contradicted"`
	EffectivenessRate float64 `json:"effectiveness_rate"`
}

// WatchdogSignalProvider supplies additional monitoring signals for multi-dimensional watchdog.
type WatchdogSignalProvider interface {
	GetSessionHealthScore(sessionID string) float64
	GetObservationRate(spaceID string) float64
	GetConsolidationAgeSec(ctx context.Context, spaceID string) (int64, error)
}
