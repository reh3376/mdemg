// Package jiminy provides the Jiminy Guidance Service — an active inner voice
// that reviews coding agent outputs and provides prompt augmentation with
// domain-specific context drawn from MDEMG's curated knowledge graph.
package jiminy

import (
	"context"
	"time"
)

// GuidanceType categorizes the kind of guidance item.
type GuidanceType string

const (
	GuidanceConstraint GuidanceType = "constraint"
	GuidanceCorrection GuidanceType = "correction"
	GuidancePattern    GuidanceType = "pattern"
	GuidanceConflict   GuidanceType = "conflict"
	GuidanceRisk       GuidanceType = "risk"
	GuidanceSuggestion GuidanceType = "suggestion"
	GuidanceFrontier   GuidanceType = "frontier"
	GuidanceDecision   GuidanceType = "decision"   // J7: from retrieval pipeline
	GuidanceLearning   GuidanceType = "learning"   // J7: from retrieval pipeline
	GuidancePreference GuidanceType = "preference" // J7: from retrieval pipeline
	GuidanceConcept    GuidanceType = "concept"    // J7: from retrieval pipeline (L2-L5 nodes)
)

// GuidanceRequest is the input to the Guide() method.
type GuidanceRequest struct {
	SpaceID     string `json:"space_id"`               // required
	Context     string `json:"context"`                // required — what the agent is doing
	FilePath    string `json:"file_path,omitempty"`    // optional — current file
	AgentOutput string `json:"agent_output,omitempty"` // optional — proposed code/action
	Query       string `json:"query,omitempty"`        // optional — user's original query
	SessionID   string `json:"session_id,omitempty"`   // optional — for correction lookup
	MaxItems    int    `json:"max_items,omitempty"`    // default 10
}

// GuidanceResponse is the output from the Guide() method.
type GuidanceResponse struct {
	GuidanceID           string             `json:"guidance_id"` // CUID2 identifier for feedback correlation (Phase AR-2)
	Guidance             []GuidanceItem     `json:"guidance"`
	PromptAugmentation   string             `json:"prompt_augmentation"`
	SynthesizedNarrative string             `json:"synthesized_narrative,omitempty"` // J8: LLM-synthesized guidance
	EncodedAugmentation  string             `json:"encoded_augmentation,omitempty"`  // J17: tier-encoded form (pre-synthesis)
	Confidence           float64            `json:"confidence"`
	Rationale            string             `json:"rationale"`
	Warnings             []string           `json:"warnings,omitempty"`
	SourceCounts         SourceCounts       `json:"source_counts"`
	SessionEscalation    *SessionEscalation `json:"session_escalation,omitempty"` // J12: escalation state
	GuidanceSeq          int64              `json:"guidance_seq,omitempty"`       // J17: monotonic sequence number
	Protocol             *ProtocolInfo      `json:"protocol,omitempty"`           // J17: protocol state info
	Debug                map[string]any     `json:"debug,omitempty"`
}

// GuidanceItem is a single piece of guidance.
type GuidanceItem struct {
	Type            GuidanceType      `json:"type"`
	Priority        string            `json:"priority"` // high, medium, low
	Content         string            `json:"content"`
	Confidence      float64           `json:"confidence"`
	SourceNodes     []string          `json:"source_nodes,omitempty"`
	EscalationLevel EscalationLevel   `json:"escalation_level,omitempty"` // J12: current escalation state
	ConstraintCode  string            `json:"constraint_code,omitempty"`  // J17: mnemonic code for T1 encoding
	Tier            int               `json:"tier,omitempty"`             // J17: encoding tier used (1=coded, 2=telegraphic, 3=full NL)
	Annotations     map[string]string `json:"annotations,omitempty"`      // J17: inline annotations for T1 disambiguation (alt:, scope:, ctx:, neg:)
}

// SourceCounts tracks how many items came from each source.
type SourceCounts struct {
	Constraints int `json:"constraints"`
	Corrections int `json:"corrections"`
	Patterns    int `json:"patterns"`
	Conflicts   int `json:"conflicts"`
	Frontiers   int `json:"frontiers"`
	Retrievals  int `json:"retrievals,omitempty"` // J7: from retrieval pipeline
}

// correctionMatch represents a correction observation found via vector search.
type correctionMatch struct {
	NodeID     string
	Content    string
	Summary    string
	Similarity float64
	CreatedAt  time.Time // for temporal decay weighting
}

// contradictionMatch represents a CONTRADICTS edge found near the context.
type contradictionMatch struct {
	SourceNodeID string
	SourceName   string
	TargetNodeID string
	TargetName   string
	Weight       float64
	Evidence     int
}

// frontierMatch represents a frontier node found via vector search.
type frontierMatch struct {
	NodeID     string
	Name       string
	Summary    string
	Similarity float64
}

// FeedbackDimensions captures multi-dimensional feedback for a guidance item (NS-03).
// Expands the feedback pipeline from adherence-only to three dimensions.
type FeedbackDimensions struct {
	Adherence     GuidanceOutcome `json:"adherence"`
	Comprehension float64         `json:"comprehension"`
	Applicability float64         `json:"applicability"`
	ScoreSource   string          `json:"score_source"` // "heuristic" or "nli"
}

// GuidanceOutcome represents the outcome of a guidance item.
type GuidanceOutcome string

const (
	OutcomeFollowed          GuidanceOutcome = "followed"
	OutcomePartialCompliance GuidanceOutcome = "partial_compliance" // J14: agent addressed some but not all aspects
	OutcomeIgnored           GuidanceOutcome = "ignored"
	OutcomeNotApplicable     GuidanceOutcome = "not_applicable" // guidance topic unrelated to the action taken
	OutcomeContradicted      GuidanceOutcome = "contradicted"
	OutcomeUnknown           GuidanceOutcome = "unknown"
)

// GuidanceFeedbackRequest is the input to the feedback endpoint.
type GuidanceFeedbackRequest struct {
	GuidanceID    string          `json:"guidance_id"`
	ActionSummary string          `json:"action_summary"`
	SpaceID       string          `json:"space_id"`
	SessionID     string          `json:"session_id,omitempty"` // J17: per-session trust routing (falls back to SpaceID)
	Outcome       GuidanceOutcome `json:"outcome,omitempty"`    // B4: explicit outcome bypasses heuristic classification
}

// GuidanceFeedbackResponse is the output from the feedback endpoint.
type GuidanceFeedbackResponse struct {
	GuidanceID string                 `json:"guidance_id"`
	Results    []GuidanceItemFeedback `json:"results"`
	Applied    bool                   `json:"applied"`
}

// GuidanceItemFeedback records the outcome for a single guidance item.
type GuidanceItemFeedback struct {
	Type       GuidanceType        `json:"type"`
	Content    string              `json:"content"`
	Outcome    GuidanceOutcome     `json:"outcome"`
	Similarity float64             `json:"similarity"`
	Reasoning  string              `json:"reasoning,omitempty"`  // J14: LLM classification reasoning
	Dimensions *FeedbackDimensions `json:"dimensions,omitempty"` // NS-03: multi-dimensional feedback
}

// ClassificationResult holds the result of semantic outcome classification (J14).
type ClassificationResult struct {
	Outcome    GuidanceOutcome `json:"outcome"`
	Confidence float64         `json:"confidence"`
	Reasoning  string          `json:"reasoning,omitempty"`
	// Source records WHICH mechanism produced the verdict
	// (JIMINY-OUTCOME-002): "tier1" (embedding similarity band),
	// "llm" (Tier-2 classifier), "heuristic" (fallback when the LLM is
	// unavailable/failed — the class that half-credited everything as
	// partial_compliance while feedback calls were context-cancelled),
	// or "explicit" (caller-provided outcome). Persisted to
	// constraint_outcomes.classifier_source so fallback-derived rows are
	// forever distinguishable from real verdicts.
	Source string `json:"source,omitempty"`
}

// --- RSIC-SK1: SignalLearnerProvider interface ---

// NegativeFeedbackApplier abstracts learning.Service.ApplyNegativeFeedback for
// Jiminy to avoid a circular import (NEGFEED-001 Bridge A). On a contradicted
// guidance outcome, Jiminy weakens the co-activations among the guidance's own
// source nodes — they jointly produced guidance that was contradicted.
type NegativeFeedbackApplier interface {
	ApplyNegativeFeedback(ctx context.Context, spaceID string, queryNodeIDs, rejectedNodeIDs []string) (weakened, contradicted int, err error)
}

// SignalLearnerProvider abstracts ape.SignalLearner for Jiminy to avoid circular imports.
type SignalLearnerProvider interface {
	RecordEmission(code string)
	RecordResponse(code string)
	// GetStrength returns the learned Hebbian strength for a signal code
	// (0.5 default for unknown codes). DORMANT-CENSUS-001 wired this read
	// side into the Guide() sort — it had zero production callers despite
	// a fully-shipped persistence layer (V0024 SignalState).
	GetStrength(code string) float64
}

// --- J7: RetrievalProvider interface ---

// RetrievalProvider defines the interface for accessing the full retrieval pipeline.
type RetrievalProvider interface {
	RetrieveForJiminy(ctx context.Context, spaceID, queryText string, topK, hopDepth int, queryEmbedding []float32) ([]RetrievalResult, error)
}

// RetrievalResult is a simplified view of models.RetrieveResult for Jiminy use.
type RetrievalResult struct {
	NodeID  string  `json:"node_id"`
	Name    string  `json:"name"`
	Summary string  `json:"summary"`
	Layer   int     `json:"layer"`
	Score   float64 `json:"score"`
	ObsType string  `json:"obs_type,omitempty"` // observation type if L0
}

// --- J9: Evaluate types ---

// EvaluateRequest is the input to the Evaluate endpoint.
type EvaluateRequest struct {
	SpaceID     string `json:"space_id"`
	AgentOutput string `json:"agent_output"`
	FilePath    string `json:"file_path,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
}

// EvaluateResponse is the output from the Evaluate endpoint.
type EvaluateResponse struct {
	EvaluationID string           `json:"evaluation_id"`
	Status       string           `json:"status"` // "pass", "warning", "concern"
	Items        []EvaluationItem `json:"items"`
	Summary      string           `json:"summary"`
}

// EvaluationItem is a single evaluation finding.
type EvaluationItem struct {
	Type        GuidanceType `json:"type"`
	Content     string       `json:"content"`
	Severity    string       `json:"severity"` // "low", "medium", "high"
	SourceNode  string       `json:"source_node,omitempty"`
	Reasoning   string       `json:"reasoning,omitempty"`   // J13: LLM reasoning for the finding
	Remediation string       `json:"remediation,omitempty"` // J13: suggested remediation
}

// --- J10: Jiminy stats for RSIC ---

// JiminyStats holds aggregated guidance metrics for RSIC assessment.
type JiminyStats struct {
	TotalGuidanceIssued    int     `json:"total_guidance_issued"`
	TotalFollowed          int     `json:"total_followed"`
	TotalPartialCompliance int     `json:"total_partial_compliance"`
	TotalIgnored           int     `json:"total_ignored"`
	TotalContradicted      int     `json:"total_contradicted"`
	FollowRate             float64 `json:"follow_rate"`
	ConstraintEffRate      float64 `json:"constraint_effectiveness_rate"`
	ConstraintDataAvail    bool    `json:"constraint_data_available"`
	SourceDiversity        float64 `json:"source_diversity"` // 0-1, higher = more diverse
}

// OutcomeWriter records constraint guidance outcomes to TSDB for dynamic
// Grafana queries (user-selected time range effectiveness calculation).
type OutcomeWriter interface {
	RecordOutcome(spaceID, constraintID, constraintCode, guidanceID, sessionID, outcomeType, guidanceType, instanceID, classifierSource string, similarity float64)
}

// GuidanceTrainingWriter persists the per-item training EVIDENCE
// (JIMINY-RELEVANCE-001 Epic 1): the surfaced guidance text, its source-node
// role_type/layer, the agent-action text, and the audited verdict + classifier
// source — the data the diagnostic (Finding 1) found was being discarded.
// Defined here (not in tsdb) to avoid an import cycle; the api layer wires a
// *tsdb.GuidanceTrainingRowsWriter via a thin adapter.
type GuidanceTrainingWriter interface {
	RecordTrainingRow(row GuidanceTrainingRecord)
}

// GuidanceTrainingRecord is the jiminy-side shape of one evidence row; the
// api-layer adapter maps it to tsdb.GuidanceTrainingRow (keeps jiminy free of
// the tsdb import). SourceLayer is a *int so a valid layer 0 is distinct from
// "unresolved" (nil).
type GuidanceTrainingRecord struct {
	SpaceID          string
	SessionID        string
	InstanceID       string
	GuidanceID       string
	GuidanceType     string
	GuidanceContent  string
	SourceNodeID     string
	SourceRoleType   string
	SourceLayer      *int
	ActionSummary    string
	OutcomeType      string
	Similarity       float64
	ClassifierSource string
	ConstraintCode   string
}

// --- J12: Escalation types ---

// EscalationLevel represents the current escalation state for a constraint.
type EscalationLevel string

const (
	EscalationInactive  EscalationLevel = ""          // not yet surfaced
	EscalationSurfaced  EscalationLevel = "surfaced"  // first presentation
	EscalationWarned    EscalationLevel = "warned"    // ignored once, now warned
	EscalationEscalated EscalationLevel = "escalated" // ignored multiple times
	EscalationBlocked   EscalationLevel = "blocked"   // max escalation
	EscalationResolved  EscalationLevel = "resolved"  // agent followed guidance
)

// SessionEscalation summarizes escalation state for a guidance response.
type SessionEscalation struct {
	ActiveEscalations int               `json:"active_escalations"`
	WarnedCount       int               `json:"warned_count"`
	EscalatedCount    int               `json:"escalated_count"`
	BlockedCount      int               `json:"blocked_count"`
	Details           []EscalationEntry `json:"details,omitempty"`
}

// EscalationEntry tracks the escalation state for a specific constraint in a session.
type EscalationEntry struct {
	NodeID       string          `json:"node_id"`
	Level        EscalationLevel `json:"level"`
	IgnoreCount  int             `json:"ignore_count"`
	LastSurfaced time.Time       `json:"last_surfaced"`
}

// ClassifyRequest is the input to the /strict classification endpoint.
type ClassifyRequest struct {
	SpaceID     string `json:"space_id"`
	SessionID   string `json:"session_id"`
	AgentOutput string `json:"agent_output"`
	ToolName    string `json:"tool_name"`
	FilePath    string `json:"file_path,omitempty"`
}

// ClassifyResponse is the output from the /strict classification endpoint.
type ClassifyResponse struct {
	Verdict         string          `json:"verdict"` // "pass" or "deny"
	DenialReason    string          `json:"denial_reason,omitempty"`
	ViolatedCodes   []string        `json:"violated_codes,omitempty"`
	Confidence      float64         `json:"confidence"`
	EscalationLevel EscalationLevel `json:"escalation_level,omitempty"`
}

// StrictReformulationResponse is the output from the /strict reformulation endpoint.
type StrictReformulationResponse struct {
	GuidanceID string  `json:"guidance_id"`
	Directive  string  `json:"directive"`
	ItemCount  int     `json:"item_count"`
	BlockedAny bool    `json:"blocked_any"`
	Confidence float64 `json:"confidence"`
}

// EscalationSnapshot holds serializable escalation state for a session.
type EscalationSnapshot struct {
	SessionID string                     `json:"session_id"`
	States    map[string]EscalationEntry `json:"states"`
	UpdatedAt time.Time                  `json:"updated_at"`
}
