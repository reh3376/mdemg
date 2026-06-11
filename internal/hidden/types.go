// Package hidden implements hierarchical graph convolution with hidden layers
// for generalized concept representations in MDEMG.
package hidden

import "time"

// HiddenNode represents a node in the hidden layer (layer 1)
type HiddenNode struct {
	NodeID               string
	SpaceID              string
	Name                 string
	Embedding            []float64
	MessagePassEmbedding []float64
	AggregationCount     int
	StabilityScore       float64
	LastForwardPass      *time.Time
	LastBackwardPass     *time.Time
}

// BaseNode represents a node in the base data layer (layer 0)
// Also used for intermediate layer nodes during clustering
type BaseNode struct {
	NodeID               string
	SpaceID              string
	Path                 string // File path for grouping
	Summary              string // LLM-generated summary from ingestion
	Embedding            []float64
	MessagePassEmbedding []float64 // Used when clustering higher layers
}

// ConceptNode represents a node in the concept layer (layer 2+)
type ConceptNode struct {
	NodeID               string
	SpaceID              string
	Layer                int
	Embedding            []float64
	MessagePassEmbedding []float64
}

// Cluster represents a group of base nodes that form a natural cluster
type Cluster struct {
	Members  []BaseNode
	Centroid []float64
}

// ClusteringResult holds the output of DBSCAN clustering
type ClusteringResult struct {
	Clusters     []Cluster
	NoisePoints  []BaseNode
	TotalPoints  int
	ClusterCount int
	NoiseCount   int
}

// ForwardPassResult holds statistics from a forward pass operation
type ForwardPassResult struct {
	HiddenNodesUpdated  int
	ConceptNodesUpdated int
	Duration            time.Duration
}

// BackwardPassResult holds statistics from a backward pass operation
type BackwardPassResult struct {
	HiddenNodesUpdated int
	EdgesStrengthened  int
	Duration           time.Duration
}

// ConsolidationResult holds the combined results of a full consolidation run
type ConsolidationResult struct {
	Skipped                 bool // true if consolidation was skipped (concurrent lock)
	EmptyGraph              bool // true if no nodes to consolidate (empty space)
	HiddenNodesCreated      int
	ConceptNodesCreated     map[int]int            // layer -> count of concepts created
	ConceptNodesMerged      int                    // count of clusters merged into existing concepts
	ConstraintNodesResult   *ConstraintNodeResult  `json:"constraint_nodes,omitempty"` // Phase 45.5
	PipelineSteps           map[string]*StepResult // Phase 46: all pipeline step results
	ForwardPass             *ForwardPassResult
	BackwardPass            *BackwardPassResult
	DynamicEdgesCreated     int // Phase 75: dynamic edges created
	L5NodesCreated          int // Phase 75: L5 emergent nodes created
	L5GroundingEdgesCreated int // ANN Opt Phase C: GROUNDED_BY edges created
	TotalDuration           time.Duration
}

// Edge represents a relationship between nodes with weight
type Edge struct {
	SourceID string
	TargetID string
	Type     string
	Weight   float64
}

// ConcernNode represents a cross-cutting concern node (role_type='concern')
// These are created based on tags like "concern:authentication", "concern:authorization"
type ConcernNode struct {
	NodeID      string
	SpaceID     string
	Name        string    // e.g., "concern:authentication"
	ConcernType string    // e.g., "authentication"
	Embedding   []float64 // Centroid of all implementing nodes
	MemberCount int
}

// ConcernNodeResult holds the results of concern node creation
type ConcernNodeResult struct {
	ConcernNodesCreated int
	EdgesCreated        int
	Concerns            []string // List of concerns found
}

// TemporalNodeResult holds the results of temporal pattern node creation
// Track 5: Temporal Pattern Detection
type TemporalNodeResult struct {
	TemporalNodeCreated bool
	EdgesCreated        int
	PatternsDetected    []string // List of temporal patterns found (e.g., "validFrom/validTo", "soft-delete")
}

// UINodeResult holds the results of UI pattern node creation
// Track 6: UI/UX Pattern Detection
type UINodeResult struct {
	UINodesCreated   int
	EdgesCreated     int
	PatternsDetected []string // List of UI patterns found (e.g., "store", "component", "data-fetching")
}

// =============================================================================
// DYNAMIC EDGE AND NODE TYPES FOR UPPER LAYERS (L4-H4-L5)
// =============================================================================
// Upper layers need richer semantics than lower layers. Rather than fixed types,
// we infer relationship and concept types from structural and embedding analysis.

// DynamicEdgeType represents inferred edge types for upper layer relationships
type DynamicEdgeType string

const (
	// Structural relationships (inferred from embedding geometry)
	EdgeAnalogous   DynamicEdgeType = "ANALOGOUS_TO"   // Parallel vectors across domains
	EdgeContrasts   DynamicEdgeType = "CONTRASTS_WITH" // Orthogonal/opposing approaches
	EdgeComposes    DynamicEdgeType = "COMPOSES_WITH"  // Combines to form larger pattern
	EdgeTensions    DynamicEdgeType = "TENSIONS_WITH"  // Design tradeoff relationship
	EdgeInfluences  DynamicEdgeType = "INFLUENCES"     // Soft architectural dependency
	EdgeSpecializes DynamicEdgeType = "SPECIALIZES"    // More specific variant
	EdgeGeneralizes DynamicEdgeType = "GENERALIZES_TO" // More abstract variant

	// Emergent relationships (discovered during consolidation)
	EdgeEmergesFrom DynamicEdgeType = "EMERGES_FROM" // Indicates emergent formation
	EdgeBridges     DynamicEdgeType = "BRIDGES"      // Connects disparate domains
	EdgeUnifies     DynamicEdgeType = "UNIFIES"      // Represents common abstraction

	// L0 Skip Connection edge type (ANN Optimization Phase C)
	EdgeGroundedBy DynamicEdgeType = "GROUNDED_BY" // L5 → L0 direct grounding
)

// DynamicNodeType represents inferred node types for upper layer concepts
type DynamicNodeType string

const (
	// Structural node types (inferred from graph position)
	NodePrinciple  DynamicNodeType = "principle"  // High-level guiding concept
	NodePattern    DynamicNodeType = "pattern"    // Recurring architectural pattern
	NodeConstraint DynamicNodeType = "constraint" // Limiting/guiding constraint
	NodeBridge     DynamicNodeType = "bridge"     // Connects disparate domains
	NodeHub        DynamicNodeType = "hub"        // Central connecting concept

	// Emergent node types (discovered during analysis)
	NodeEmergent    DynamicNodeType = "emergent"    // Newly formed concept
	NodeEstablished DynamicNodeType = "established" // Stable, mature concept
	NodeTension     DynamicNodeType = "tension"     // Represents a tradeoff
	NodeSynthesis   DynamicNodeType = "synthesis"   // Combines multiple patterns
)

// EdgeInference contains the result of inferring an edge type between two nodes
type EdgeInference struct {
	SourceID     string
	TargetID     string
	InferredType DynamicEdgeType
	Confidence   float64     // 0.0 - 1.0
	Evidence     string      // Human-readable explanation
	Metrics      EdgeMetrics // Raw metrics used for inference
}

// EdgeMetrics holds the raw measurements used to infer edge types
type EdgeMetrics struct {
	CosineSimilarity float64 // Embedding similarity
	PathOverlap      float64 // Domain path similarity
	CoActivation     float64 // Hebbian co-activation strength
	LayerDistance    int     // Layers apart
	DomainDistance   float64 // Cross-domain metric
}

// NodeInference contains the result of inferring a node type for upper layer concepts
type NodeInference struct {
	NodeID       string
	InferredType DynamicNodeType
	Confidence   float64     // 0.0 - 1.0
	Evidence     string      // Human-readable explanation
	Metrics      NodeMetrics // Raw metrics used for inference
}

// NodeMetrics holds the raw measurements used to infer node types
type NodeMetrics struct {
	InDegree         int     // Incoming edges
	OutDegree        int     // Outgoing edges
	CrossDomainLinks int     // Links to different domains
	StabilityScore   float64 // How stable the embedding is
	AggregationDepth int     // How many layers of children
	ChildDiversity   float64 // Diversity of child node types
}

// InferenceThresholds configures the thresholds for type inference
type InferenceThresholds struct {
	// Edge type thresholds
	AnalogousMinSim   float64 // Min cosine sim for ANALOGOUS_TO (default: 0.7)
	ContrastsMaxSim   float64 // Max cosine sim for CONTRASTS_WITH (default: 0.3)
	ComposesMinCoact  float64 // Min co-activation for COMPOSES_WITH (default: 0.5)
	BridgesMinDomains int     // Min distinct domains for BRIDGES (default: 3)

	// Node type thresholds
	HubMinDegree       int     // Min degree to be a hub (default: 10)
	BridgeMinDomains   int     // Min domains to be a bridge (default: 3)
	EstablishedMinStab float64 // Min stability for established (default: 0.8)
	EmergentMaxAge     int     // Max consolidation cycles for emergent (default: 3)
}

// DefaultInferenceThresholds returns sensible defaults for type inference
func DefaultInferenceThresholds() InferenceThresholds {
	return InferenceThresholds{
		AnalogousMinSim:    0.7,
		ContrastsMaxSim:    0.3,
		ComposesMinCoact:   0.5,
		BridgesMinDomains:  3,
		HubMinDegree:       10,
		BridgeMinDomains:   3,
		EstablishedMinStab: 0.8,
		EmergentMaxAge:     3,
	}
}

// UpperLayerEdge represents an edge in the upper layers with dynamic typing
type UpperLayerEdge struct {
	EdgeID     string
	SourceID   string
	TargetID   string
	SpaceID    string
	EdgeType   DynamicEdgeType // Inferred type
	Weight     float64
	Confidence float64 // Confidence in the type inference
	Evidence   string  // Why this type was inferred
	CreatedAt  time.Time
	InferredAt time.Time // When the type was last inferred
}

// UpperLayerNode represents a node in L4-H4-L5 with dynamic typing
type UpperLayerNode struct {
	NodeID     string
	SpaceID    string
	Layer      int
	Name       string
	NodeType   DynamicNodeType // Inferred type
	Embedding  []float64
	Confidence float64 // Confidence in the type inference
	Evidence   string  // Why this type was inferred
	Stability  float64 // Embedding stability over time
	CreatedAt  time.Time
	InferredAt time.Time // When the type was last inferred
}

// =============================================================================
// CONVERSATION HIDDEN LAYER TYPES (Phase 3)
// =============================================================================
// Conversation observations cluster into themes, similar to how code files
// cluster into concepts. These are kept separate from code clustering.

// ConversationObservation represents a conversation_observation node for clustering
type ConversationObservation struct {
	NodeID        string
	SpaceID       string
	ObsType       string    // decision, correction, learning, preference, error, task, technical_note, insight, context, progress, blocker, context_signal, note, constraint, self_improvement
	Content       string    // The observation content
	Summary       string    // Brief summary of the observation
	Embedding     []float64 // Semantic embedding
	SurpriseScore float64   // How novel/surprising the information is (0.0-1.0)
	SessionID     string    // Which session this came from
	Tags          []string  // Additional tags
}

// ConversationTheme represents a conversation_theme node in layer 1
// Themes emerge from clustering related conversation observations
type ConversationTheme struct {
	NodeID           string
	SpaceID          string
	Name             string    // Generated theme name
	Summary          string    // Generated summary describing the theme
	Embedding        []float64 // Centroid of clustered observations
	MemberCount      int       // Number of observations in this theme
	DominantObsType  string    // Most common observation type in the theme
	AvgSurpriseScore float64   // Average surprise score of members
}

// ConversationThemeResult holds the results of conversation theme clustering
type ConversationThemeResult struct {
	ThemesCreated     int      // Number of new conversation_theme nodes created
	ThemesUpdated     int      // HIDDEN-CHURN-001: themes matched by centroid and refreshed in place
	EdgesCreated      int      // Number of GENERALIZES edges created
	ThemeSummaries    []string // Summaries of created themes
	ObservationsUsed  int      // Total observations that were clustered
	NoiseObservations int      // Observations that didn't fit any cluster
	NoiseAssigned     int      // PR-B: noise observations density-assigned to nearest theme
}

// ConversationConsolidationResult holds results from conversation-specific consolidation
type ConversationConsolidationResult struct {
	ThemeResult   *ConversationThemeResult
	ConceptResult *EmergentConceptResult
	ForwardPass   *ForwardPassResult
	TotalDuration time.Duration
}

// =============================================================================
// PHASE 4: EMERGENT CONCEPT FORMATION TYPES
// =============================================================================
// Emergent concepts (Layer 2+) form from clustering conversation_theme nodes.
// These represent higher-level abstractions spanning multiple themes across sessions.

// EmergentConcept represents an emergent_concept node in layer 2+
// Emergent concepts form from clustering related conversation themes
type EmergentConcept struct {
	NodeID           string
	SpaceID          string
	Layer            int       // 2 for first level, 3 for higher, etc.
	Name             string    // Generated concept name
	Summary          string    // High-level summary of what this concept represents
	Embedding        []float64 // Centroid of clustered themes/concepts
	MemberCount      int       // Number of themes/concepts in this cluster
	Keywords         []string  // Aggregated keywords from member themes
	AvgSurpriseScore float64   // Average surprise score from member themes
	SessionCount     int       // Number of distinct sessions represented
}

// EmergentConceptResult holds the results of emergent concept clustering
type EmergentConceptResult struct {
	ConceptsCreated  map[int]int // layer -> count of concepts created
	EdgesCreated     int         // Number of ABSTRACTS_TO edges created
	ThemesUsed       int         // Total themes that were clustered
	NoiseThemes      int         // Themes that didn't fit any cluster
	ConceptSummaries []string    // Summaries of created concepts
	MaxLayerReached  int         // Highest layer with concepts
}

// ConversationThemeForClustering represents a conversation_theme node for clustering into emergent concepts
type ConversationThemeForClustering struct {
	NodeID               string
	SpaceID              string
	Name                 string
	Summary              string
	Embedding            []float64
	MessagePassEmbedding []float64
	MemberCount          int
	AvgSurpriseScore     float64
	SessionIDs           []string // Sessions represented by this theme
	Keywords             []string
}

// EmergentConceptNode represents an emergent concept node for higher-layer clustering
// Used when clustering L2 concepts into L3, L3 into L4, etc.
type EmergentConceptNode struct {
	NodeID               string
	SpaceID              string
	Layer                int
	Name                 string
	Summary              string
	Embedding            []float64
	MessagePassEmbedding []float64
	Keywords             []string
	SessionCount         int
}

// ConceptHierarchyNode represents a node at any layer for hierarchy traversal
type ConceptHierarchyNode struct {
	NodeID    string
	SpaceID   string
	Layer     int
	RoleType  string // conversation_observation, conversation_theme, emergent_concept
	Name      string
	Summary   string
	Embedding []float64
}
