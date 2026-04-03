package models

type RetrieveRequest struct {
	SpaceID        string    `json:"space_id" validate:"required,min=1,max=256"`
	QueryText      string    `json:"query_text,omitempty" validate:"required_without=QueryEmbedding,omitempty,min=1"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty" validate:"required_without=QueryText,omitempty,embedding_dims"`

	CandidateK int `json:"candidate_k,omitempty" validate:"omitempty,min=1,max=1000"`
	TopK       int `json:"top_k,omitempty" validate:"omitempty,min=1,max=100"`
	HopDepth   int `json:"hop_depth,omitempty" validate:"omitempty,min=0,max=5"`

	JiminyEnabled   bool `json:"jiminy_enabled,omitempty"`   // Enable explainable retrieval (adds rationale + score breakdown)
	IncludeEvidence bool `json:"include_evidence,omitempty"` // Include symbol evidence for each result

	// File type filtering (applied at query time, not ingest time)
	// These filter results based on file path extensions.
	IncludeExtensions []string `json:"include_extensions,omitempty"` // Only include files with these extensions (e.g., ["java", "go", "py"])
	ExcludeExtensions []string `json:"exclude_extensions,omitempty"` // Exclude files with these extensions (e.g., ["md", "txt"])
	CodeOnly          bool     `json:"code_only,omitempty"`          // Shorthand: exclude common non-code files (md, txt, json, yaml, etc.)

	PolicyContext map[string]any `json:"policy_context,omitempty"`

	// Temporal override fields (Phase 1: Time-Aware Retrieval)
	// When set, these override the parsed temporal intent with a hard-mode constraint.
	TemporalAfter  string `json:"temporal_after,omitempty"`  // ISO8601: force hard filter after this time
	TemporalBefore string `json:"temporal_before,omitempty"` // ISO8601: force hard filter before this time

	// Intent Translation (Phase 102)
	TranslateIntent bool `json:"translate_intent,omitempty"` // Enable LLM query rewriting before embedding

	// Session tracking for TSDB training data
	SessionID string `json:"session_id,omitempty"` // Propagated to TSDB for session-level analysis

	// Global Meta-Learning (Phase 105)
	IncludeGlobalSpace bool `json:"include_global_space,omitempty"` // Include mdemg-global space in retrieval results

	// Pagination fields (Phase 48.3)
	Cursor string `json:"cursor,omitempty"` // Node ID to start after for cursor pagination
	Limit  int    `json:"limit,omitempty"`  // Max results per page (default: 50, max: 500)
}

type RetrieveResult struct {
	NodeID  string  `json:"node_id"`
	Path    string  `json:"path"`
	Name    string  `json:"name"`
	Summary string  `json:"summary"`
	Layer   int     `json:"layer"`
	Score   float64 `json:"score"`

	// Normalized confidence metrics (immune to learning edge density)
	NormalizedConfidence float64 `json:"normalized_confidence,omitempty"` // Percentile rank 0-100
	ConfidenceLevel      string  `json:"confidence_level,omitempty"`      // HIGH/MEDIUM/LOW based on percentile

	VectorSim  float64             `json:"vector_sim,omitempty"`
	Activation float64             `json:"activation,omitempty"`
	Jiminy     *JiminyExplanation  `json:"jiminy,omitempty"`  // Explainable retrieval (when enabled)
	Evidence   []SymbolEvidence    `json:"evidence,omitempty"` // Symbol evidence grounding the result
}

// SymbolEvidence provides verifiable evidence linking a retrieval result to specific code.
// This enables Evidence Compliance tracking for grounded retrieval.
// Field names follow UPTS (Universal Parser Test Specification) v1.0.0
type SymbolEvidence struct {
	SymbolName string `json:"symbol_name"`           // Name of the symbol (e.g., "MAX_TIMEOUT")
	SymbolType string `json:"symbol_type"`           // Type: const, function, class, etc.
	FilePath   string `json:"file_path"`             // Path to the source file
	Line       int    `json:"line"`                  // 1-indexed start line (UPTS standard)
	LineEnd    int    `json:"line_end,omitempty"`    // End line for multi-line symbols
	Value      string `json:"value,omitempty"`       // Resolved value (for constants)
	RawValue   string `json:"raw_value,omitempty"`   // Original value as written
	Signature  string `json:"signature,omitempty"`   // Function/method signature
	DocComment string `json:"doc_comment,omitempty"` // Documentation comment
}

// JiminyExplanation provides transparency into why a result was retrieved and ranked.
// Named after the character who guides and explains - MDEMG's "conscience" layer.
type JiminyExplanation struct {
	Rationale           string             `json:"rationale"`
	Confidence          float64            `json:"confidence"`
	RetrievalPath       []string           `json:"retrieval_path"`
	ContributingModules []string           `json:"contributing_modules,omitempty"`
	ScoreBreakdown      map[string]float64 `json:"score_breakdown"`
}

type RetrieveResponse struct {
	SpaceID          string            `json:"space_id"`
	Results          []RetrieveResult  `json:"results"`
	EvidenceMetrics  *EvidenceMetrics  `json:"evidence_metrics,omitempty"` // Evidence compliance tracking
	Debug            map[string]any    `json:"debug,omitempty"`

	// Pagination fields (Phase 48.3)
	NextCursor string `json:"next_cursor,omitempty"` // Cursor for next page (empty if no more results)
	HasMore    bool   `json:"has_more"`              // True if more results are available
	TotalCount int    `json:"total_count,omitempty"` // Total matching results (optional, may be expensive)

	// Intent Translation (Phase 102)
	TranslatedIntent string `json:"translated_intent,omitempty"` // Rewritten query from intent translation
}

// EvidenceMetrics tracks evidence compliance for retrieval quality measurement.
type EvidenceMetrics struct {
	TotalResults       int     `json:"total_results"`        // Total number of results returned
	ResultsWithEvidence int    `json:"results_with_evidence"` // Results that have at least one symbol evidence
	TotalSymbols       int     `json:"total_symbols"`        // Total symbols across all results
	ComplianceRate     float64 `json:"compliance_rate"`      // % of results with evidence (ECR)
	AvgSymbolsPerResult float64 `json:"avg_symbols_per_result"` // Average symbols per result
}

type IngestRequest struct {
	SpaceID     string    `json:"space_id" validate:"required,min=1,max=256"`
	Timestamp   string    `json:"timestamp" validate:"required,min=1"`
	Source      string    `json:"source" validate:"required,min=1,max=64"`
	Content     any       `json:"content" validate:"required"`
	Tags        []string  `json:"tags,omitempty" validate:"omitempty,dive,min=1"`
	NodeID      string    `json:"node_id,omitempty"`
	Path        string    `json:"path,omitempty" validate:"omitempty,max=512"`
	Name        string    `json:"name,omitempty"`
	Summary     string    `json:"summary,omitempty" validate:"omitempty,max=1000"` // Brief summary for reranking (max 1000 chars)
	Sensitivity string    `json:"sensitivity,omitempty" validate:"omitempty,oneof=public internal confidential"`
	Confidence  *float64  `json:"confidence,omitempty" validate:"omitempty,min=0,max=1"`
	Embedding       []float32 `json:"embedding,omitempty" validate:"omitempty,embedding_dims"` // Optional: pre-computed embedding
	CanonicalTime   string    `json:"canonical_time,omitempty"`                                // ISO8601: content-relevant time (Phase 2 Temporal)
	TimestampFormat string    `json:"timestamp_format,omitempty" validate:"omitempty,oneof=rfc3339 unix unix_ms date_only"` // Timestamp format enum (default: rfc3339)
	ContentHash     string    `json:"content_hash,omitempty" validate:"omitempty,max=128"`     // SHA256 hash for change detection (skip re-ingest if unchanged)
	FileSize        int64     `json:"file_size,omitempty"`                                     // File size in bytes
	LineCount       int       `json:"line_count,omitempty"`                                    // Number of lines in the file
}

type IngestResponse struct {
	SpaceID       string    `json:"space_id"`
	NodeID        string    `json:"node_id"`
	ObsID         string    `json:"obs_id"`
	EmbeddingDims int       `json:"embedding_dims,omitempty"` // Dimensions of generated embedding
	Anomalies     []Anomaly `json:"anomalies,omitempty"`      // Detected anomalies (non-blocking)
	Skipped       bool      `json:"skipped,omitempty"`        // True if content_hash matched (no new observation created)
}

// AnomalyType represents the type of anomaly detected during ingest
type AnomalyType string

const (
	AnomalyContradiction AnomalyType = "contradiction" // Conflicts with existing node
	AnomalyDuplicate     AnomalyType = "duplicate"     // Very similar node exists
	AnomalyOutlier       AnomalyType = "outlier"       // Unusual for this space
	AnomalyStaleUpdate   AnomalyType = "stale_update"  // Updating old node
)

// Anomaly represents a detected anomaly during ingest
type Anomaly struct {
	Type        AnomalyType `json:"type"`
	Severity    string      `json:"severity"`               // "info", "warning", "critical"
	Message     string      `json:"message"`
	RelatedNode string      `json:"related_node,omitempty"` // Node ID of related node
	Confidence  float64     `json:"confidence"`             // 0.0 - 1.0
}

// AnomalySignal represents a meta-cognitive anomaly detected during CMS operations.
// Used by resume/recall handlers to flag degraded memory states.
type AnomalySignal struct {
	Code     string `json:"code"`     // "empty-resume", "no-themes", "empty-recall"
	Severity string `json:"severity"` // "critical", "high", "medium", "low"
	Message  string `json:"message"`
	Action   string `json:"action"`   // Recommended action (e.g., curl command)
}

// MetricsResponse - main response structure for GET /v1/metrics
type MetricsResponse struct {
	TotalNodes     int64            `json:"total_nodes"`
	TotalEdges     int64            `json:"total_edges"`
	NodesByLayer   map[int]int64    `json:"nodes_by_layer"`
	EdgesByType    map[string]int64 `json:"edges_by_type"`
	AvgEdgeWeight  float64          `json:"avg_edge_weight"`
	HubNodes       []HubNode        `json:"hub_nodes"`       // top 10 by degree
	OrphanNodes    int64            `json:"orphan_nodes"`    // nodes with no edges
	RecentActivity *ActivityStats   `json:"recent_activity"` // last 24h
}

// HubNode - represents a high-connectivity node
type HubNode struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	Degree int    `json:"degree"`
}

// ActivityStats - recent activity within 24h window
type ActivityStats struct {
	NodesCreated int64 `json:"nodes_created"`
	EdgesCreated int64 `json:"edges_created"`
	Retrievals   int64 `json:"retrievals"`
}

// ReflectRequest - request for deep context exploration via /v1/memory/reflect
type ReflectRequest struct {
	SpaceID        string    `json:"space_id" validate:"required,min=1"`
	Topic          string    `json:"topic" validate:"required_without=TopicEmbedding,omitempty,min=1,max=500"`                     // natural language topic (required)
	TopicEmbedding []float32 `json:"topic_embedding,omitempty" validate:"required_without=Topic,omitempty,embedding_dims"` // pre-computed embedding for topic
	MaxDepth       int       `json:"max_depth,omitempty" validate:"omitempty,min=1,max=10"`       // hop depth (default: 3)
	MaxNodes       int       `json:"max_nodes,omitempty" validate:"omitempty,min=1,max=500"`       // cap results (default: 50)
}

// ReflectResponse - response from deep context exploration
type ReflectResponse struct {
	Topic           string        `json:"topic"`
	CoreMemories    []ScoredNode  `json:"core_memories"`
	RelatedConcepts []ScoredNode  `json:"related_concepts"`
	Abstractions    []ScoredNode  `json:"abstractions"`
	Insights        []Insight     `json:"insights"`
	GraphContext    *GraphContext `json:"graph_context"`
}

// ScoredNode - a node with relevance scoring for reflection results
type ScoredNode struct {
	NodeID   string  `json:"node_id"`
	Name     string  `json:"name"`
	Path     string  `json:"path,omitempty"`
	Summary  string  `json:"summary,omitempty"`
	Layer    int     `json:"layer"`
	Score    float64 `json:"score"`    // relevance score
	Distance int     `json:"distance"` // hops from seed
}

// Insight - a detected pattern or observation from reflection
type Insight struct {
	Type        string   `json:"type"`        // "cluster", "pattern", "gap"
	Description string   `json:"description"`
	NodeIDs     []string `json:"node_ids"`
}

// GraphContext - traversal statistics from reflection
type GraphContext struct {
	NodesExplored   int `json:"nodes_explored"`
	EdgesTraversed  int `json:"edges_traversed"`
	MaxLayerReached int `json:"max_layer_reached"`
}

// BatchIngestRequest - request for batch ingest endpoint
type BatchIngestRequest struct {
	SpaceID      string            `json:"space_id" validate:"required,min=1"`
	Observations []BatchIngestItem `json:"observations" validate:"required,min=1,max=2000,dive"`
}

// BatchIngestItem - single observation in a batch ingest request
type BatchIngestItem struct {
	Timestamp   string         `json:"timestamp" validate:"required,min=1"`
	Source      string         `json:"source" validate:"required,min=1,max=64"`
	Content     any            `json:"content" validate:"required"`
	Tags        []string       `json:"tags,omitempty" validate:"omitempty,dive,min=1"`
	NodeID      string         `json:"node_id,omitempty"`
	Path        string         `json:"path,omitempty" validate:"omitempty,max=512"`
	Name        string         `json:"name,omitempty"`
	Summary     string         `json:"summary,omitempty" validate:"omitempty,max=1000"` // Brief summary for reranking
	Symbols     []IngestSymbol `json:"symbols,omitempty"`                               // Extracted code symbols (Phase 8)
	Sensitivity string         `json:"sensitivity,omitempty" validate:"omitempty,oneof=public internal confidential"`
	Confidence  *float64       `json:"confidence,omitempty" validate:"omitempty,min=0,max=1"`
	Embedding       []float32      `json:"embedding,omitempty" validate:"omitempty,embedding_dims"`
	CanonicalTime   string         `json:"canonical_time,omitempty"`                                // ISO8601: content-relevant time (Phase 2 Temporal)
	TimestampFormat string         `json:"timestamp_format,omitempty" validate:"omitempty,oneof=rfc3339 unix unix_ms date_only"` // Timestamp format enum (default: rfc3339)
	ContentHash     string         `json:"content_hash,omitempty" validate:"omitempty,max=128"`     // SHA256 hash for change detection
	FileSize        int64          `json:"file_size,omitempty"`                                     // File size in bytes
	LineCount       int            `json:"line_count,omitempty"`                                    // Number of lines
}

// IngestSymbol represents an extracted code symbol (constant, function, class, etc.)
// Used for evidence-locked retrieval in Phase 8 symbol-level indexing.
type IngestSymbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"`                      // const, function, class, enum, enum_value, etc.
	Value          string `json:"value,omitempty"`           // Resolved value (e.g., "60000" from "60 * 1000")
	RawValue       string `json:"raw_value,omitempty"`       // Original value as written in code
	Line           int    `json:"line"`                      // 1-indexed start line (UPTS standard)
	LineEnd        int    `json:"line_end,omitempty"`        // End line for multi-line symbols
	Exported       bool   `json:"exported"`
	DocComment     string `json:"doc_comment,omitempty"`
	Signature      string `json:"signature,omitempty"`       // Function signature
	Parent         string `json:"parent,omitempty"`          // Parent class/module name
	TypeAnnotation string `json:"type_annotation,omitempty"` // Type annotation if present
	Language       string `json:"language,omitempty"`        // go, typescript, python, etc.
}

// BatchIngestResult - result for a single item in batch ingest
type BatchIngestResult struct {
	Index         int    `json:"index"`
	Status        string `json:"status"` // "success" or "error"
	NodeID        string `json:"node_id,omitempty"`
	ObsID         string `json:"obs_id,omitempty"`
	EmbeddingDims int    `json:"embedding_dims,omitempty"`
	Error         string `json:"error,omitempty"`
}

// BatchIngestResponse - response for batch ingest endpoint
type BatchIngestResponse struct {
	SpaceID      string              `json:"space_id"`
	TotalItems   int                 `json:"total_items"`
	SuccessCount int                 `json:"success_count"`
	ErrorCount   int                 `json:"error_count"`
	Results      []BatchIngestResult `json:"results"`
}

// StatsResponse - response for GET /v1/memory/stats endpoint
// Provides comprehensive per-space memory statistics including counts,
// embedding coverage, learning metrics, and health indicators.
type StatsResponse struct {
	SpaceID               string               `json:"space_id"`
	MemoryCount           int64                `json:"memory_count"`
	ObservationCount      int64                `json:"observation_count"`
	MemoriesByLayer       map[int]int64        `json:"memories_by_layer"`
	EmbeddingCoverage     float64              `json:"embedding_coverage"`      // 0.0 - 1.0
	AvgEmbeddingDimensions int                 `json:"avg_embedding_dimensions"`
	LearningActivity      *LearningActivity    `json:"learning_activity"`
	TemporalDistribution  *TemporalDistribution `json:"temporal_distribution"`
	Connectivity          *Connectivity        `json:"connectivity"`
	HealthScore           float64              `json:"health_score"` // 0.0 - 1.0
	ComputedAt            string               `json:"computed_at"`  // ISO8601 timestamp
}

// LearningActivity - Hebbian learning metrics from CO_ACTIVATED_WITH edges
type LearningActivity struct {
	CoActivatedEdges int64   `json:"co_activated_edges"`
	AvgWeight        float64 `json:"avg_weight"`
	MaxWeight        float64 `json:"max_weight"`
}

// TemporalDistribution - memory creation counts over time periods
type TemporalDistribution struct {
	Last24h int64 `json:"last_24h"`
	Last7d  int64 `json:"last_7d"`
	Last30d int64 `json:"last_30d"`
}

// Connectivity - graph connectivity statistics for a space
type Connectivity struct {
	AvgDegree   float64 `json:"avg_degree"`
	MaxDegree   int     `json:"max_degree"`
	OrphanCount int64   `json:"orphan_count"`
}

// ArchiveRequest - request for archiving a memory node
type ArchiveRequest struct {
	Reason string `json:"reason,omitempty"` // optional reason for archiving
}

// ArchiveResponse - response from archive endpoint
type ArchiveResponse struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	ArchivedAt string `json:"archived_at"`
	Reason     string `json:"reason,omitempty"`
}

// UnarchiveResponse - response from unarchive endpoint
type UnarchiveResponse struct {
	NodeID       string `json:"node_id"`
	Name         string `json:"name"`
	UnarchivedAt string `json:"unarchived_at"`
}

// DeleteResponse - response from delete endpoint
type DeleteResponse struct {
	NodeID       string `json:"node_id"`
	DeletedNodes int    `json:"deleted_nodes"`
	DeletedEdges int    `json:"deleted_edges"`
}

// BulkArchiveRequest - request for bulk archiving memory nodes
type BulkArchiveRequest struct {
	SpaceID string   `json:"space_id"`
	NodeIDs []string `json:"node_ids"`
	Reason  string   `json:"reason,omitempty"` // optional reason for archiving
}

// BulkArchiveResult - result for a single item in bulk archive
type BulkArchiveResult struct {
	NodeID     string `json:"node_id"`
	Status     string `json:"status"` // "success" or "error"
	ArchivedAt string `json:"archived_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BulkArchiveResponse - response for bulk archive endpoint
type BulkArchiveResponse struct {
	SpaceID      string              `json:"space_id"`
	TotalItems   int                 `json:"total_items"`
	SuccessCount int                 `json:"success_count"`
	ErrorCount   int                 `json:"error_count"`
	Results      []BulkArchiveResult `json:"results"`
}

// ConsultRequest - request for POST /v1/memory/consult
// The Agent Consulting Service acts as an SME for coding agents.
type ConsultRequest struct {
	SpaceID  string         `json:"space_id" validate:"required,min=1,max=256"`
	Context  string         `json:"context" validate:"required,min=1,max=10000"`   // Current context (code, error, task description)
	Question string         `json:"question" validate:"required,min=1,max=2000"`   // What the agent is asking about
	Tags     []string       `json:"tags,omitempty" validate:"omitempty,dive,min=1"` // Optional filtering tags
	MaxSuggestions int      `json:"max_suggestions,omitempty" validate:"omitempty,min=1,max=20"` // Max suggestions to return (default 5)
	IncludeEvidence bool    `json:"include_evidence,omitempty"` // Include symbol evidence for suggestions
	LlmSynthesis    bool    `json:"llm_synthesis,omitempty"`    // Enable LLM synthesis for narrative response (Phase 101)
	TranslateIntent bool    `json:"translate_intent,omitempty"` // Enable LLM query rewriting before embedding (Phase 102)
	SessionID       string  `json:"session_id,omitempty"`       // Propagated to TSDB for session-level analysis
}

// ConsultResponse - response from the Agent Consulting Service
type ConsultResponse struct {
	SpaceID         string            `json:"space_id"`
	Suggestions     []Suggestion      `json:"suggestions"`
	RelatedConcepts []RelatedConcept  `json:"related_concepts,omitempty"` // Higher-level concepts
	Confidence      float64           `json:"confidence"`                 // Overall confidence 0.0-1.0
	Rationale       string            `json:"rationale,omitempty"`        // Why these suggestions
	Synthesis       string            `json:"synthesis,omitempty"`        // LLM-generated narrative (Phase 101)
	TranslatedIntent string           `json:"translated_intent,omitempty"` // Rewritten query from intent translation (Phase 102)
	Debug           map[string]any    `json:"debug,omitempty"`
}

// Suggestion represents a single piece of SME advice
type Suggestion struct {
	Type        SuggestionType     `json:"type"`         // context, process, concept, risk
	Content     string             `json:"content"`      // The suggestion text
	Confidence  float64            `json:"confidence"`   // Confidence 0.0-1.0
	SourceNodes []string           `json:"source_nodes"` // Node IDs backing this suggestion
	Evidence    []SymbolEvidence   `json:"evidence,omitempty"` // Symbol evidence (when requested)
}

// SuggestionType categorizes what kind of advice this is
type SuggestionType string

const (
	SuggestionContext SuggestionType = "context"  // "Based on this codebase's patterns..."
	SuggestionProcess SuggestionType = "process"  // "The typical workflow for this type of change is..."
	SuggestionConcept SuggestionType = "concept"  // "This relates to the higher-level principle of..."
	SuggestionRisk    SuggestionType = "risk"     // "Previous attempts at this approach encountered..."
)

// RelatedConcept represents a higher-level concept from the hidden layer
type RelatedConcept struct {
	NodeID     string  `json:"node_id"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Layer      int     `json:"layer"`
	Relevance  float64 `json:"relevance"` // How relevant to the query 0.0-1.0
}

// SuggestRequest - request for POST /v1/memory/suggest
// Context-triggered suggestions: proactive surfacing of relevant info without explicit questions.
// This moves MDEMG from passive retrieval to active participation.
type SuggestRequest struct {
	SpaceID            string   `json:"space_id" validate:"required,min=1,max=256"`
	Context            string   `json:"context" validate:"required,min=1,max=20000"` // Current context (code being edited, file content, error message)
	FilePath           string   `json:"file_path,omitempty"`                         // Optional: path of file being edited
	Tags               []string `json:"tags,omitempty" validate:"omitempty,dive,min=1"`
	MaxSuggestions     int      `json:"max_suggestions,omitempty" validate:"omitempty,min=1,max=20"` // Max suggestions (default 5)
	MinConfidence      float64  `json:"min_confidence,omitempty" validate:"omitempty,min=0,max=1"`   // Minimum confidence threshold (default 0.5)
	IncludeEvidence    bool     `json:"include_evidence,omitempty"`                                  // Include symbol evidence
	IncludeConflicts   bool     `json:"include_conflicts,omitempty"`                                 // Detect potential conflicts
	IncludeConstraints bool     `json:"include_constraints,omitempty"`                               // Include architectural constraints
	TranslateIntent    bool     `json:"translate_intent,omitempty"`                                  // Enable LLM query rewriting before embedding (Phase 102)
}

// SuggestResponse - response from context-triggered suggestion endpoint
type SuggestResponse struct {
	SpaceID         string             `json:"space_id"`
	Triggers        []ContextTrigger   `json:"triggers,omitempty"`         // What triggered these suggestions
	Suggestions     []Suggestion       `json:"suggestions"`                // Proactive suggestions
	Conflicts       []ConflictWarning  `json:"conflicts,omitempty"`        // Potential conflicts with existing knowledge
	Constraints     []Constraint       `json:"constraints,omitempty"`      // Relevant architectural constraints
	RelatedConcepts []RelatedConcept   `json:"related_concepts,omitempty"` // Higher-level concepts
	Confidence       float64            `json:"confidence"`                  // Overall confidence
	TranslatedIntent string             `json:"translated_intent,omitempty"` // Rewritten query from intent translation (Phase 102)
	Debug            map[string]any     `json:"debug,omitempty"`
}

// ContextTrigger explains what in the context triggered suggestions
type ContextTrigger struct {
	TriggerType string   `json:"trigger_type"` // pattern_match, keyword, file_type, semantic
	Matched     string   `json:"matched"`      // What was matched in the context
	Keywords    []string `json:"keywords,omitempty"`
}

// ConflictWarning indicates a potential conflict with existing knowledge
type ConflictWarning struct {
	Severity    string   `json:"severity"`     // low, medium, high
	Description string   `json:"description"`  // What the conflict is
	ConflictsWith string `json:"conflicts_with"` // Node ID or pattern that conflicts
	Evidence    string   `json:"evidence,omitempty"` // Supporting evidence
	SourceNodes []string `json:"source_nodes"`
}

// Constraint represents an architectural constraint that applies to the context
type Constraint struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ConstraintType string `json:"constraint_type"` // must, must_not, should, should_not
	Scope       string   `json:"scope,omitempty"` // Where this constraint applies
	SourceNodes []string `json:"source_nodes"`
	Confidence  float64  `json:"confidence"`
}

// Additional suggestion types for context-triggered suggestions
const (
	SuggestionPattern    SuggestionType = "pattern"    // "Related pattern in this codebase..."
	SuggestionSolution   SuggestionType = "solution"   // "Previous solution to similar problem..."
	SuggestionConstraint SuggestionType = "constraint" // "Architectural constraint that applies..."
	SuggestionConflict   SuggestionType = "conflict"   // "Potential conflict with existing..."
)

// ConsolidateRequest - request for POST /v1/memory/consolidate
// Triggers hidden layer creation and message passing operations.
type ConsolidateRequest struct {
	SpaceID       string `json:"space_id" validate:"required,min=1,max=256"`
	SkipClustering bool   `json:"skip_clustering,omitempty"` // Skip DBSCAN clustering, only run message passing
	SkipForward   bool   `json:"skip_forward,omitempty"`    // Skip forward pass
	SkipBackward  bool   `json:"skip_backward,omitempty"`   // Skip backward pass

	// Dynamic Emergence (Phase 103)
	EnableDynamicEmergence bool    `json:"enable_dynamic_emergence,omitempty"` // Enable LLM-driven concept naming
	MinClusterDensity      float64 `json:"min_cluster_density,omitempty"`      // Override min CO_ACTIVATED_WITH weight
}

// StepResultAPI is the API-layer mirror of hidden.StepResult.
// Used in ConsolidateResponse.Steps for dynamic pipeline results.
type StepResultAPI struct {
	NodesCreated int            `json:"nodes_created"`
	NodesUpdated int            `json:"nodes_updated"`
	EdgesCreated int            `json:"edges_created"`
	Details      map[string]any `json:"details,omitempty"`
}

// ConsolidateResponse - response from consolidate endpoint
type ConsolidateResponse struct {
	SpaceID             string                     `json:"space_id"`
	Enabled             bool                       `json:"enabled"` // Whether hidden layer is enabled
	Steps               map[string]*StepResultAPI  `json:"steps,omitempty"` // Dynamic pipeline step results (Phase 46)

	// Core consolidation fields (forward/backward pass, concepts)
	HiddenNodesCreated  int     `json:"hidden_nodes_created"`
	HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
	ConceptNodesCreated int     `json:"concept_nodes_created"`
	ConceptNodesMerged  int     `json:"concept_nodes_merged"`
	ConceptNodesUpdated int     `json:"concept_nodes_updated"`
	EdgesStrengthened   int     `json:"edges_strengthened"`
	SummariesGenerated  int     `json:"summaries_generated"`
	EdgesRefreshed      int     `json:"edges_refreshed"`
	DurationMs          float64 `json:"duration_ms"`

	// Flat fields preserved for backward compatibility (populated from Steps map)
	ConcernNodesCreated    int  `json:"concern_nodes_created"`
	ConcernEdgesCreated    int  `json:"concern_edges_created"`
	ConfigNodeCreated      bool `json:"config_node_created"`
	ConfigEdgesCreated     int  `json:"config_edges_created"`
	ComparisonNodesCreated int  `json:"comparison_nodes_created"`
	ComparisonEdgesCreated int  `json:"comparison_edges_created"`
	TemporalNodeCreated    bool `json:"temporal_node_created"`
	TemporalEdgesCreated   int  `json:"temporal_edges_created"`
	UINodesCreated         int  `json:"ui_nodes_created"`
	UIEdgesCreated         int  `json:"ui_edges_created"`
	ConstraintNodesCreated int  `json:"constraint_nodes_created"`
	ConstraintNodesUpdated int  `json:"constraint_nodes_updated"`
	ConstraintEdgesLinked  int  `json:"constraint_edges_linked"`
	DynamicEdgesCreated    int  `json:"dynamic_edges_created"`
	L5NodesCreated         int  `json:"l5_nodes_created"`

	// Dynamic Emergence (Phase 103)
	DynamicEmergentNodesCreated int `json:"dynamic_emergent_nodes_created,omitempty"`
}

// ObserveRequest - request for POST /v1/conversation/observe
// Captures a significant conversation observation with surprise detection.
type ObserveRequest struct {
	SpaceID   string         `json:"space_id" validate:"required,min=1,max=256"`
	SessionID string         `json:"session_id" validate:"required,min=1"`
	Content   string         `json:"content" validate:"required,min=1"`
	ObsType   string         `json:"obs_type,omitempty"` // decision, learning, preference, correction, error, task, technical_note, insight, context, progress, blocker, context_signal, note, constraint, self_improvement
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`

	// Identity & Visibility (CMS v2)
	UserID     string `json:"user_id,omitempty"`                                                   // Owner of the observation
	Visibility string `json:"visibility,omitempty" validate:"omitempty,oneof=private team global"` // Default: private

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Persistent agent identity (survives across sessions)

	// Cross-module linking (CMS v2)
	RefersTo []string `json:"refers_to,omitempty"` // Node/symbol IDs this observation references

	// Pinned Observations (Phase 47)
	Pinned bool `json:"pinned,omitempty"` // When true, observation is permanent and non-decaying
}

// ObserveResponse - response from POST /v1/conversation/observe
// DetectedConstraintInfo represents a constraint found during observation (Phase 45.5)
type DetectedConstraintInfo struct {
	ConstraintType string  `json:"constraint_type"`
	Name           string  `json:"name"`
	Confidence     float64 `json:"confidence"`
}

type ObserveResponse struct {
	ObsID               string                   `json:"obs_id"`
	NodeID              string                   `json:"node_id"`
	SurpriseScore       float64                  `json:"surprise_score"`
	SurpriseFactors     map[string]float64       `json:"surprise_factors"`
	Summary             string                   `json:"summary,omitempty"`
	DetectedConstraints []DetectedConstraintInfo  `json:"detected_constraints,omitempty"`
}

// CorrectRequest - request for POST /v1/conversation/correct
// Captures an explicit user correction with high surprise score.
type CorrectRequest struct {
	SpaceID   string `json:"space_id" validate:"required,min=1,max=256"`
	SessionID string `json:"session_id" validate:"required,min=1"`
	Incorrect string `json:"incorrect" validate:"required,min=1"`
	Correct   string `json:"correct" validate:"required,min=1"`
	Context   string `json:"context,omitempty"`

	// Identity & Visibility (CMS v2)
	UserID     string `json:"user_id,omitempty"`                                                   // Owner of the correction
	Visibility string `json:"visibility,omitempty" validate:"omitempty,oneof=private team global"` // Default: private

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Persistent agent identity

	// Cross-module linking (CMS v2)
	RefersTo []string `json:"refers_to,omitempty"` // Node/symbol IDs this correction references
}

// IngestTriggerRequest - request for POST /v1/memory/ingest/trigger
// Triggers a background codebase re-ingestion job.
type IngestTriggerRequest struct {
	SpaceID         string   `json:"space_id" validate:"required,min=1,max=256"`
	Path            string   `json:"path" validate:"required,min=1"`
	BatchSize       int      `json:"batch_size,omitempty"`       // Items per batch (default: 100)
	Workers         int      `json:"workers,omitempty"`          // Parallel workers (default: 4)
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty"`  // HTTP timeout (default: 300)
	ExtractSymbols  *bool    `json:"extract_symbols,omitempty"`  // Extract code symbols (default: true)
	Consolidate     *bool    `json:"consolidate,omitempty"`      // Run consolidation after (default: true)
	IncludeTests    bool     `json:"include_tests,omitempty"`    // Include test files
	IncludeMarkdown *bool    `json:"include_md,omitempty"`       // Include markdown (default: true)
	IncludeTS       *bool    `json:"include_ts,omitempty"`       // Include TypeScript/JS (default: true)
	IncludePython   *bool    `json:"include_py,omitempty"`       // Include Python (default: true)
	Incremental     bool     `json:"incremental,omitempty"`      // Only changed files
	SinceCommit     string   `json:"since_commit,omitempty"`     // Git commit for incremental (default: HEAD~1)
	ArchiveDeleted  *bool    `json:"archive_deleted,omitempty"`  // Archive deleted file nodes (default: true)
	ExcludeDirs     []string `json:"exclude_dirs,omitempty"`     // Directories to skip
	Limit           int      `json:"limit,omitempty"`            // Max elements to ingest (0 = no limit)
	DryRun          bool     `json:"dry_run,omitempty"`          // Preview without ingesting
}

// IngestTriggerResponse - response from POST /v1/memory/ingest/trigger
type IngestTriggerResponse struct {
	JobID     string `json:"job_id"`
	SpaceID   string `json:"space_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"created_at"`
}

// IngestJobStatusResponse - response from GET /v1/memory/ingest/status/{job_id}
type IngestJobStatusResponse struct {
	JobID       string         `json:"job_id"`
	SpaceID     string         `json:"space_id,omitempty"`
	Status      string         `json:"status"`
	Progress    IngestProgress `json:"progress"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	StartedAt   string         `json:"started_at,omitempty"`
	CompletedAt string         `json:"completed_at,omitempty"`
	CreatedAt   string         `json:"created_at"`
}

// IngestProgress tracks progress of an ingest job.
type IngestProgress struct {
	Total      int     `json:"total"`
	Current    int     `json:"current"`
	Percentage float64 `json:"percentage"`
	Phase      string  `json:"phase,omitempty"` // discovery, ingestion, consolidation
	Rate       string  `json:"rate,omitempty"`  // e.g., "15.2 elements/sec"
}

// IngestFilesRequest - request for POST /v1/memory/ingest/files
// Triggers re-ingestion of specific files.
type IngestFilesRequest struct {
	SpaceID        string   `json:"space_id" validate:"required,min=1,max=256"`
	Files          []string `json:"files" validate:"required,min=1"`
	ExtractSymbols *bool    `json:"extract_symbols,omitempty"` // default: true
	Consolidate    *bool    `json:"consolidate,omitempty"`     // default: false
}

// IngestFilesResponse - response from POST /v1/memory/ingest/files
type IngestFilesResponse struct {
	SpaceID      string             `json:"space_id"`
	TotalFiles   int                `json:"total_files"`
	SuccessCount int                `json:"success_count"`
	ErrorCount   int                `json:"error_count"`
	Results      []IngestFileResult `json:"results"`
	JobID        string             `json:"job_id,omitempty"` // Set when >50 files triggers background job
}

// IngestFileResult - result for a single file in file-level ingest
type IngestFileResult struct {
	File   string `json:"file"`
	Status string `json:"status"` // "success" or "error"
	NodeID string `json:"node_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// =============================================================================
// PHASE 5: CONTEXT-AWARE RETRIEVAL AND SKILL INTEGRATION
// =============================================================================
// These types support conversation memory resume/recall operations and
// context-aware retrieval that blends conversation knowledge into results.

// ResumeRequest - request for POST /v1/conversation/resume
// Restores context after context compaction by retrieving recent observations,
// themes, and emergent concepts.
type ResumeRequest struct {
	SpaceID          string `json:"space_id" validate:"required,min=1,max=256"`
	SessionID        string `json:"session_id,omitempty"`        // Optional: filter to specific session
	IncludeTasks     bool   `json:"include_tasks,omitempty"`     // Include task-type observations
	IncludeDecisions bool   `json:"include_decisions,omitempty"` // Include decision-type observations
	IncludeLearnings bool   `json:"include_learnings,omitempty"` // Include learning-type observations
	MaxObservations  int    `json:"max_observations,omitempty"`  // Max observations to return (default: 20)

	// Visibility filtering (CMS v2)
	RequestingUserID string `json:"requesting_user_id,omitempty"` // Filter private observations to this user

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Resume across sessions for this agent
}

// JiminyRationale explains WHY specific state was rehydrated during resume.
// Named after Jiminy Cricket - the "conscience" that provides guidance.
type JiminyRationale struct {
	Rationale      string             `json:"rationale"`       // Human-readable explanation
	Confidence     float64            `json:"confidence"`      // 0.0-1.0 confidence in rationale
	ScoreBreakdown map[string]float64 `json:"score_breakdown"` // Factors: surprise, recency, reinforcement
	Highlights     []string           `json:"highlights"`      // Key items to pay attention to
}

// ResumeResponse - response from POST /v1/conversation/resume
// Provides context restoration after context compaction.
type ResumeResponse struct {
	SpaceID          string                    `json:"space_id"`
	SessionID        string                    `json:"session_id,omitempty"`
	Observations     []ConversationObsResult   `json:"observations"`
	Themes           []ConversationThemeResult `json:"themes"`
	EmergentConcepts []EmergentConceptResult   `json:"emergent_concepts"`
	Summary          string                    `json:"summary"`          // Generated context summary
	Jiminy           *JiminyRationale          `json:"jiminy,omitempty"` // Explains why state was rehydrated
	Debug            map[string]any            `json:"debug,omitempty"`
	Anomalies        []AnomalySignal           `json:"anomalies,omitempty"`
	MemoryState      string                    `json:"memory_state"` // "degraded", "nominal", "healthy"
}

// ConversationObsResult represents a conversation observation in resume/recall responses
type ConversationObsResult struct {
	NodeID        string    `json:"node_id"`
	ObsType       string    `json:"obs_type"`       // decision, learning, preference, error, task, correction, technical_note, insight, context, progress, blocker, context_signal, note, constraint, self_improvement
	Content       string    `json:"content"`
	Summary       string    `json:"summary"`
	SessionID     string    `json:"session_id"`
	SurpriseScore float64   `json:"surprise_score"` // How novel/surprising (0.0-1.0)
	Score         float64   `json:"score,omitempty"` // Relevance score (for recall)
	Tags          []string  `json:"tags,omitempty"`
	CreatedAt     string    `json:"created_at"`
}

// ConversationThemeResult represents a conversation theme in resume/recall responses
type ConversationThemeResult struct {
	NodeID           string  `json:"node_id"`
	Name             string  `json:"name"`
	Summary          string  `json:"summary"`
	MemberCount      int     `json:"member_count"`
	DominantObsType  string  `json:"dominant_obs_type,omitempty"`
	AvgSurpriseScore float64 `json:"avg_surprise_score"`
	Score            float64 `json:"score,omitempty"` // Relevance score (for recall)
}

// EmergentConceptResult represents an emergent concept in resume/recall responses
type EmergentConceptResult struct {
	NodeID       string   `json:"node_id"`
	Name         string   `json:"name"`
	Summary      string   `json:"summary"`
	Layer        int      `json:"layer"` // 2 for first level, 3+ for higher
	Keywords     []string `json:"keywords,omitempty"`
	SessionCount int      `json:"session_count,omitempty"` // Number of sessions represented
	Score        float64  `json:"score,omitempty"`         // Relevance score (for recall)
}

// RecallRequest - request for POST /v1/conversation/recall
// Retrieves relevant conversation knowledge via semantic query.
type RecallRequest struct {
	SpaceID         string    `json:"space_id" validate:"required,min=1,max=256"`
	Query           string    `json:"query" validate:"required,min=1,max=2000"` // Natural language query
	QueryEmbedding  []float32 `json:"query_embedding,omitempty"`                // Optional: pre-computed embedding
	TopK            int       `json:"top_k,omitempty"`                          // Max results (default: 10)
	IncludeThemes   bool      `json:"include_themes,omitempty"`                 // Include conversation themes
	IncludeConcepts bool      `json:"include_concepts,omitempty"`               // Include emergent concepts

	// Visibility filtering (CMS v2)
	RequestingUserID string `json:"requesting_user_id,omitempty"` // Filter private observations to this user

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Filter to this agent's observations

	// Temporal filtering (Phase 1: Time-Aware Retrieval)
	TemporalAfter  string `json:"temporal_after,omitempty"`  // ISO8601: filter results after this time
	TemporalBefore string `json:"temporal_before,omitempty"` // ISO8601: filter results before this time

	// Tag filtering (Phase 47: Pinned Observations)
	FilterTags []string `json:"filter_tags,omitempty"` // Return only observations with ALL these tags
}

// RecallResponse - response from POST /v1/conversation/recall
// Returns relevant conversation knowledge based on semantic query.
type RecallResponse struct {
	SpaceID     string          `json:"space_id"`
	Query       string          `json:"query"`
	Results     []RecallResult  `json:"results"`
	Debug       map[string]any  `json:"debug,omitempty"`
	Anomalies   []AnomalySignal `json:"anomalies,omitempty"`
	MemoryState string          `json:"memory_state,omitempty"`
}

// RecallResult represents a single result from conversation recall
type RecallResult struct {
	Type     string  `json:"type"`     // conversation_observation, conversation_theme, emergent_concept
	NodeID   string  `json:"node_id"`
	Content  string  `json:"content"`  // For observations: content, for themes/concepts: summary
	Score    float64 `json:"score"`    // Relevance score (0.0-1.0)
	Layer    int     `json:"layer"`    // 0 for observations, 1 for themes, 2+ for concepts
	Metadata map[string]any `json:"metadata,omitempty"` // Additional metadata
}

// FreshnessResponse provides freshness/staleness info for a space's TapRoot.
type FreshnessResponse struct {
	SpaceID        string `json:"space_id"`
	LastIngestAt   string `json:"last_ingest_at,omitempty"`
	LastIngestType string `json:"last_ingest_type,omitempty"`
	IngestCount    int    `json:"ingest_count"`
	IsStale        bool   `json:"is_stale"`
	StaleHours     int    `json:"stale_hours,omitempty"`
	ThresholdHours int    `json:"threshold_hours"`
}

// ContextRetrieveRequest extends RetrieveRequest with conversation context options
// Used internally when blending conversation knowledge into retrieval.
type ContextRetrieveRequest struct {
	RetrieveRequest                       // Embedded base request
	BlendConversationContext bool         `json:"blend_conversation_context,omitempty"` // Enable conversation blending
	ConversationBoostFactor  float64      `json:"conversation_boost_factor,omitempty"`  // Boost factor for conversation-supported results (default: 1.2)
}

// =============================================================================
// PHASE 9.5: ORPHAN CLEANUP
// =============================================================================

// OrphanCleanupRequest - request for POST /v1/memory/cleanup/orphans
// Detects and optionally acts on orphaned L0 nodes that were not included
// in the most recent re-ingestion (last_ingested_at < TapRoot.last_ingest_at).
type OrphanCleanupRequest struct {
	SpaceID       string `json:"space_id" validate:"required,min=1,max=256"`
	Action        string `json:"action" validate:"required,oneof=archive delete list count"`
	Limit         int    `json:"limit,omitempty"`           // default: 100, max: 1000
	DryRun        bool   `json:"dry_run,omitempty"`         // preview without acting
	OlderThanDays int    `json:"older_than_days,omitempty"` // Additional staleness filter (days since last_ingested_at)
	PathPrefix    string `json:"path_prefix,omitempty"`     // Filter by path prefix
}

// OrphanCleanupResponse - response from POST /v1/memory/cleanup/orphans
type OrphanCleanupResponse struct {
	SpaceID      string       `json:"space_id"`
	OrphansFound int          `json:"orphans_found"`
	OrphansActed int          `json:"orphans_acted"`
	Action       string       `json:"action"`
	DryRun       bool         `json:"dry_run"`
	Orphans      []OrphanNode `json:"orphans"`
}

// OrphanNode represents a single orphan candidate detected during cleanup.
type OrphanNode struct {
	NodeID         string `json:"node_id"`
	Path           string `json:"path"`
	Name           string `json:"name"`
	LastIngestedAt string `json:"last_ingested_at"`
	Status         string `json:"status"` // "archived", "deleted", "listed"
}

// ScheduleCleanupRequest - request for POST /v1/memory/cleanup/schedule
// Sets up scheduled orphan cleanup for a space.
type ScheduleCleanupRequest struct {
	SpaceID       string `json:"space_id" validate:"required,min=1,max=256"`
	IntervalHours int    `json:"interval_hours" validate:"required,min=1,max=720"` // 1 hour to 30 days
	Action        string `json:"action" validate:"required,oneof=archive delete"`
	Limit         int    `json:"limit,omitempty"`           // default: 100, max: 1000
	OlderThanDays int    `json:"older_than_days,omitempty"` // Only clean orphans older than N days
}

// ScheduleCleanupResponse - response from POST /v1/memory/cleanup/schedule
type ScheduleCleanupResponse struct {
	SpaceID       string `json:"space_id"`
	ScheduleID    string `json:"schedule_id"`
	IntervalHours int    `json:"interval_hours"`
	Action        string `json:"action"`
	Status        string `json:"status"` // "enabled", "disabled"
	NextRunAt     string `json:"next_run_at,omitempty"`
}

// GraphOrphanRequest — POST /v1/memory/cleanup/graph-orphans
type GraphOrphanRequest struct {
	Action     string   `json:"action" validate:"required,oneof=scan consolidate archive delete"`
	SpaceIDs   []string `json:"space_ids,omitempty"`
	MinAgeDays int      `json:"min_age_days,omitempty"`
	Layers     []int    `json:"layers,omitempty"`
	DryRun     bool     `json:"dry_run,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// GraphOrphanResponse — response from graph orphan cleanup
type GraphOrphanResponse struct {
	Action        string              `json:"action"`
	DryRun        bool                `json:"dry_run"`
	TotalSpaces   int                 `json:"total_spaces"`
	TotalOrphans  int                 `json:"total_orphans"`
	TotalAffected int                 `json:"total_affected"`
	SpaceResults  []SpaceOrphanResult `json:"space_results"`
	Warnings      []string            `json:"warnings,omitempty"`
}

// SpaceOrphanResult — per-space orphan scan/fix result
type SpaceOrphanResult struct {
	SpaceID        string            `json:"space_id"`
	OrphanCount    int               `json:"orphan_count"`
	AffectedCount  int               `json:"affected_count"`
	LayerBreakdown map[string]int    `json:"layer_breakdown"`
	Skipped        bool              `json:"skipped,omitempty"`
	SkipReason     string            `json:"skip_reason,omitempty"`
	Nodes          []GraphOrphanNode `json:"nodes,omitempty"`
}

// GraphOrphanNode — a single zero-edge node
type GraphOrphanNode struct {
	NodeID    string `json:"node_id"`
	Layer     int    `json:"layer"`
	RoleType  string `json:"role_type"`
	CreatedAt string `json:"created_at"`
	Status    string `json:"status,omitempty"`
}

// =============================================================================
// ADMIN: SPACE LIFECYCLE MANAGEMENT
// =============================================================================

// SpaceInfo represents a single space in the admin spaces listing.
type SpaceInfo struct {
	SpaceID          string `json:"space_id"`
	Prunable         bool   `json:"prunable"`
	Protected        bool   `json:"protected"`
	CreatedAt        string `json:"created_at,omitempty"`
	LastIngestAt     string `json:"last_ingest_at,omitempty"`
	IngestCount      int    `json:"ingest_count"`
	NodeCount        int    `json:"node_count"`
	ObservationCount int    `json:"observation_count"`
}

// AdminSpacesResponse - response from GET /v1/admin/spaces
type AdminSpacesResponse struct {
	Spaces        []SpaceInfo `json:"spaces"`
	Total         int         `json:"total"`
	PrunableCount int         `json:"prunable_count"`
}

// AdminSpaceUpdateRequest - request for PATCH /v1/admin/spaces/{space_id}
type AdminSpaceUpdateRequest struct {
	Prunable *bool `json:"prunable"`
}

// AdminSpacePruneRequest - request for POST /v1/admin/spaces/prune
type AdminSpacePruneRequest struct {
	DryRun    bool `json:"dry_run"`
	BatchSize int  `json:"batch_size"`
	MaxSpaces int  `json:"max_spaces"`
}

// SpacePruneResult - result for a single space in prune response
type SpacePruneResult struct {
	SpaceID      string `json:"space_id"`
	NodesDeleted int    `json:"nodes_deleted"`
	Status       string `json:"status"` // "pruned", "dry_run", "skipped", "error"
	DurationMs   int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"`
}

// AdminSpacePruneResponse - response from POST /v1/admin/spaces/prune
type AdminSpacePruneResponse struct {
	DryRun            bool              `json:"dry_run"`
	SpacesPruned      int               `json:"spaces_pruned"`
	SpacesSkipped     int               `json:"spaces_skipped"`
	TotalNodesDeleted int               `json:"total_nodes_deleted"`
	Results           []SpacePruneResult `json:"results"`
}

// Neo4jOverviewResponse - response for GET /v1/neo4j/overview
type Neo4jOverviewResponse struct {
	Database   DatabaseOverview `json:"database"`
	Spaces     []SpaceOverview  `json:"spaces"`
	Backups    BackupOverview   `json:"backups"`
	ComputedAt string           `json:"computed_at"`
}

// DatabaseOverview - aggregate database health and counts
type DatabaseOverview struct {
	Status        string `json:"status"`         // "healthy", "degraded", "unavailable"
	Version       string `json:"version"`        // MDEMG version from config
	SchemaVersion int    `json:"schema_version"` // migration schema version
	TotalNodes    int64  `json:"total_nodes"`
	TotalEdges    int64  `json:"total_edges"`
	TotalSpaces   int    `json:"total_spaces"`
}

// SpaceOverview - per-space node/edge/health summary
type SpaceOverview struct {
	SpaceID           string        `json:"space_id"`
	NodeCount         int64         `json:"node_count"`
	EdgeCount         int64         `json:"edge_count"`
	NodesByLayer      map[int]int64 `json:"nodes_by_layer"`
	ObservationCount  int64         `json:"observation_count"`
	HealthScore       float64       `json:"health_score"`
	LastConsolidation string        `json:"last_consolidation,omitempty"`
	LastIngest        string        `json:"last_ingest,omitempty"`
	LastIngestType    string        `json:"last_ingest_type,omitempty"`
	IngestCount       int           `json:"ingest_count"`
	IsStale           bool          `json:"is_stale"`
	LearningEdges     int64         `json:"learning_edges"`
	OrphanCount       int64         `json:"orphan_count"`
}

// GuardrailValidateRequest is the request body for POST /v1/memory/guardrail/validate
type GuardrailValidateRequest struct {
	SpaceID      string   `json:"space_id"      validate:"required"`
	FilesChanged []string `json:"files_changed" validate:"required"`
	Diff         string   `json:"diff"          validate:"required"`

	// F20: Agent trust level for authority-level filtering.
	// Values: "restricted" (all constraints), "standard" (org_policy + team_standard),
	// "elevated" (org_policy only). Optional; omit for no filtering.
	AgentTrustLevel string `json:"agent_trust_level,omitempty"`
}

// GuardrailViolation represents a single constraint violation or warning.
type GuardrailViolation struct {
	ConstraintNodeID string `json:"constraint_node_id"`
	Description      string `json:"description"`
	Rationale        string `json:"rationale"`
}

// GuardrailValidateResponse is the response body for POST /v1/memory/guardrail/validate
type GuardrailValidateResponse struct {
	Status     string               `json:"status"` // "Pass", "Warning", "Block"
	Violations []GuardrailViolation `json:"violations"`
	Warnings   []GuardrailViolation `json:"warnings"`
}

// BackupOverview - backup status summary
type BackupOverview struct {
	LastFull    *BackupSummary `json:"last_full,omitempty"`
	LastPartial *BackupSummary `json:"last_partial,omitempty"`
	TotalCount  int            `json:"total_count"`
}

// BackupSummary - minimal backup info for overview
type BackupSummary struct {
	BackupID  string   `json:"backup_id"`
	CreatedAt string   `json:"created_at"`
	SizeBytes int64    `json:"size_bytes"`
	Spaces    []string `json:"spaces,omitempty"`
}

// =============================================================================
// PHASE 105: GLOBAL META-LEARNING (Cross-Space Collective Learning)
// =============================================================================

// MetaLearnRequest — POST /v1/memory/meta-learn
// Triggers promotion of high-value concepts from a source space to the global space.
type MetaLearnRequest struct {
	SourceSpaceID  string `json:"source_space_id" validate:"required,min=1,max=256"`
	MinLayer       int    `json:"min_layer,omitempty"`
	MinUpdateCount int    `json:"min_update_count,omitempty"`
}

// PromotedConcept — a single concept promoted to the global space
type PromotedConcept struct {
	ID            string `json:"id"`
	OriginalID    string `json:"original_id"`
	Name          string `json:"name"`
	GlobalSpaceID string `json:"global_space_id"`
}

// MetaLearnResponse — response from meta-learn endpoint
type MetaLearnResponse struct {
	Status            string            `json:"status"`
	ConceptsEvaluated int               `json:"concepts_evaluated"`
	ConceptsPromoted  int               `json:"concepts_promoted"`
	PromotedNodes     []PromotedConcept `json:"promoted_nodes"`
}

// GraphVizNode — a node for Grafana Node Graph visualization (GAP-20)
type GraphVizNode struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	SubTitle      string  `json:"subTitle"`
	MainStat      string `json:"mainStat"`
	SecondaryStat string `json:"secondaryStat"`
	Color         string  `json:"color"`
	NodeRadius    float64 `json:"nodeRadius,omitempty"`
	// detail__* fields shown in click context menu
	DetailRoleType    string `json:"detail__role_type"`
	DetailObsType     string `json:"detail__obs_type"`
	DetailTags        string `json:"detail__tags"`
	DetailStability   string `json:"detail__stability"`
	DetailSurprise    string `json:"detail__surprise"`
	DetailCreatedAt   string `json:"detail__created_at"`
	DetailLastActive  string `json:"detail__last_activated"`
	DetailEdgeCount   string `json:"detail__edge_count"`
	DetailStatus      string `json:"detail__status"`
}

// GraphVizEdge — an edge for Grafana Node Graph visualization (GAP-20)
type GraphVizEdge struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	MainStat string `json:"mainStat"`
	Color    string `json:"color,omitempty"`
}

// GraphVizResponse — response format for Grafana Node Graph API plugin (GAP-20)
type GraphVizResponse struct {
	Nodes      []GraphVizNode `json:"nodes"`
	Edges      []GraphVizEdge `json:"edges"`
	TotalNodes int            `json:"total_nodes"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}
