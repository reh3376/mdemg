package mdemg

// RetrieveRequest is the request body for POST /v1/memory/retrieve.
type RetrieveRequest struct {
	SpaceID        string    `json:"space_id"`
	QueryText      string    `json:"query_text,omitempty"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty"`
	CandidateK     int       `json:"candidate_k,omitempty"`
	TopK           int       `json:"top_k,omitempty"`
	HopDepth       int       `json:"hop_depth,omitempty"`

	JiminyEnabled   bool `json:"jiminy_enabled,omitempty"`
	IncludeEvidence bool `json:"include_evidence,omitempty"`

	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludeExtensions []string `json:"exclude_extensions,omitempty"`
	CodeOnly          bool     `json:"code_only,omitempty"`

	PolicyContext map[string]any `json:"policy_context,omitempty"`

	TemporalAfter  string `json:"temporal_after,omitempty"`
	TemporalBefore string `json:"temporal_before,omitempty"`
	TranslateIntent bool  `json:"translate_intent,omitempty"`

	IncludeGlobalSpace bool   `json:"include_global_space,omitempty"`
	Cursor             string `json:"cursor,omitempty"`
	Limit              int    `json:"limit,omitempty"`
}

// RetrieveResult is a single scored result from the retrieval pipeline.
type RetrieveResult struct {
	NodeID              string             `json:"node_id"`
	Path                string             `json:"path"`
	Name                string             `json:"name"`
	Summary             string             `json:"summary"`
	Layer               int                `json:"layer"`
	Score               float64            `json:"score"`
	NormalizedConfidence float64           `json:"normalized_confidence,omitempty"`
	ConfidenceLevel     string             `json:"confidence_level,omitempty"`
	VectorSim           float64            `json:"vector_sim,omitempty"`
	Activation          float64            `json:"activation,omitempty"`
	Jiminy              *JiminyExplanation `json:"jiminy,omitempty"`
	Evidence            []SymbolEvidence   `json:"evidence,omitempty"`
}

// RetrieveResponse is the response from POST /v1/memory/retrieve.
type RetrieveResponse struct {
	SpaceID          string           `json:"space_id"`
	Results          []RetrieveResult `json:"results"`
	EvidenceMetrics  *EvidenceMetrics `json:"evidence_metrics,omitempty"`
	Debug            map[string]any   `json:"debug,omitempty"`
	NextCursor       string           `json:"next_cursor,omitempty"`
	HasMore          bool             `json:"has_more"`
	TotalCount       int              `json:"total_count,omitempty"`
	TranslatedIntent string           `json:"translated_intent,omitempty"`
}

// SymbolEvidence provides verifiable evidence linking a retrieval result to code.
type SymbolEvidence struct {
	SymbolName string `json:"symbol_name"`
	SymbolType string `json:"symbol_type"`
	FilePath   string `json:"file_path"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end,omitempty"`
	Value      string `json:"value,omitempty"`
	RawValue   string `json:"raw_value,omitempty"`
	Signature  string `json:"signature,omitempty"`
	DocComment string `json:"doc_comment,omitempty"`
}

// JiminyExplanation provides transparency into retrieval ranking decisions.
type JiminyExplanation struct {
	Rationale           string             `json:"rationale"`
	Confidence          float64            `json:"confidence"`
	RetrievalPath       []string           `json:"retrieval_path"`
	ContributingModules []string           `json:"contributing_modules,omitempty"`
	ScoreBreakdown      map[string]float64 `json:"score_breakdown"`
}

// EvidenceMetrics tracks evidence compliance for retrieval quality.
type EvidenceMetrics struct {
	TotalResults        int     `json:"total_results"`
	ResultsWithEvidence int     `json:"results_with_evidence"`
	TotalSymbols        int     `json:"total_symbols"`
	ComplianceRate      float64 `json:"compliance_rate"`
	AvgSymbolsPerResult float64 `json:"avg_symbols_per_result"`
}

// ObserveRequest is the request body for POST /v1/conversation/observe.
type ObserveRequest struct {
	SpaceID   string   `json:"space_id"`
	SessionID string   `json:"session_id"`
	Content   string   `json:"content"`
	ObsType   string   `json:"obs_type,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Pinned    bool     `json:"pinned,omitempty"`
}

// ObserveResponse is the response from POST /v1/conversation/observe.
type ObserveResponse struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	ObsID     string `json:"obs_id"`
	NodeID    string `json:"node_id"`
}

// RecallRequest is the request body for POST /v1/conversation/recall.
type RecallRequest struct {
	SpaceID    string   `json:"space_id"`
	Query      string   `json:"query"`
	TopK       int      `json:"top_k,omitempty"`
	FilterTags []string `json:"filter_tags,omitempty"`
}

// RecallResult is a single result from recall.
type RecallResult struct {
	NodeID    string   `json:"node_id"`
	Content   string   `json:"content"`
	Summary   string   `json:"summary"`
	Score     float64  `json:"score"`
	Tags      []string `json:"tags,omitempty"`
	ObsType   string   `json:"obs_type,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// RecallResponse is the response from POST /v1/conversation/recall.
type RecallResponse struct {
	SpaceID string         `json:"space_id"`
	Results []RecallResult `json:"results"`
}

// IngestRequest is the request body for POST /v1/memory/ingest.
type IngestRequest struct {
	SpaceID     string   `json:"space_id"`
	Timestamp   string   `json:"timestamp"`
	Source      string   `json:"source"`
	Content     any      `json:"content"`
	Tags        []string `json:"tags,omitempty"`
	NodeID      string   `json:"node_id,omitempty"`
	Path        string   `json:"path,omitempty"`
	Name        string   `json:"name,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Sensitivity string   `json:"sensitivity,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

// IngestResponse is the response from POST /v1/memory/ingest.
type IngestResponse struct {
	SpaceID       string `json:"space_id"`
	NodeID        string `json:"node_id"`
	ObsID         string `json:"obs_id"`
	EmbeddingDims int    `json:"embedding_dims,omitempty"`
}

// ConsultRequest is the request body for POST /v1/consulting/consult.
type ConsultRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Query     string `json:"query"`
	Context   string `json:"context,omitempty"`
}

// ConsultResponse is the response from POST /v1/consulting/consult.
type ConsultResponse struct {
	SpaceID    string       `json:"space_id"`
	Guidance   string       `json:"guidance"`
	Confidence float64      `json:"confidence"`
	Triggers   []string     `json:"triggers,omitempty"`
	Conflicts  []string     `json:"conflicts,omitempty"`
	Constraints []Constraint `json:"constraints,omitempty"`
}

// Constraint represents a constraint that applies to a situation.
type Constraint struct {
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Scope       string   `json:"scope,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ResumeRequest is the request body for POST /v1/conversation/resume.
type ResumeRequest struct {
	SpaceID         string `json:"space_id"`
	SessionID       string `json:"session_id"`
	MaxObservations int    `json:"max_observations,omitempty"`
}

// ResumeResponse is the response from POST /v1/conversation/resume.
type ResumeResponse struct {
	SpaceID          string      `json:"space_id"`
	Summary          string      `json:"summary"`
	Observations     []ObsResult `json:"observations"`
	Themes           []Theme     `json:"themes"`
	EmergentConcepts []Concept   `json:"emergent_concepts"`
}

// ObsResult is a recalled observation in a resume response.
type ObsResult struct {
	ObsID     string `json:"obs_id"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	ObsType   string `json:"obs_type"`
	CreatedAt string `json:"created_at"`
}

// Theme is a consolidated topic cluster.
type Theme struct {
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

// Concept is an emergent concept from the hidden layer hierarchy.
type Concept struct {
	Name  string `json:"name"`
	Layer int    `json:"layer"`
	Score float64 `json:"score"`
}

// HealthResponse is the response from GET /healthz.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}
