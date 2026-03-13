// Package jiminy provides the Jiminy Guidance Service — an active inner voice
// that reviews coding agent outputs and provides prompt augmentation with
// domain-specific context drawn from MDEMG's curated knowledge graph.
package jiminy

// GuidanceType categorizes the kind of guidance item.
type GuidanceType string

const (
	GuidanceConstraint    GuidanceType = "constraint"
	GuidanceCorrection    GuidanceType = "correction"
	GuidancePattern       GuidanceType = "pattern"
	GuidanceConflict      GuidanceType = "conflict"
	GuidanceRisk          GuidanceType = "risk"
	GuidanceSuggestion    GuidanceType = "suggestion"
	GuidanceFrontier      GuidanceType = "frontier"
)

// GuidanceRequest is the input to the Guide() method.
type GuidanceRequest struct {
	SpaceID     string `json:"space_id"`               // required
	Context     string `json:"context"`                 // required — what the agent is doing
	FilePath    string `json:"file_path,omitempty"`     // optional — current file
	AgentOutput string `json:"agent_output,omitempty"`  // optional — proposed code/action
	Query       string `json:"query,omitempty"`         // optional — user's original query
	SessionID   string `json:"session_id,omitempty"`    // optional — for correction lookup
	MaxItems    int    `json:"max_items,omitempty"`     // default 10
}

// GuidanceResponse is the output from the Guide() method.
type GuidanceResponse struct {
	Guidance           []GuidanceItem `json:"guidance"`
	PromptAugmentation string         `json:"prompt_augmentation"`
	Confidence         float64        `json:"confidence"`
	Rationale          string         `json:"rationale"`
	Warnings           []string       `json:"warnings,omitempty"`
	SourceCounts       SourceCounts   `json:"source_counts"`
	Debug              map[string]any `json:"debug,omitempty"`
}

// GuidanceItem is a single piece of guidance.
type GuidanceItem struct {
	Type        GuidanceType `json:"type"`
	Priority    string       `json:"priority"`    // high, medium, low
	Content     string       `json:"content"`
	Confidence  float64      `json:"confidence"`
	SourceNodes []string     `json:"source_nodes,omitempty"`
}

// SourceCounts tracks how many items came from each source.
type SourceCounts struct {
	Constraints   int `json:"constraints"`
	Corrections   int `json:"corrections"`
	Patterns      int `json:"patterns"`
	Conflicts     int `json:"conflicts"`
	Frontiers     int `json:"frontiers"`
}

// correctionMatch represents a correction observation found via vector search.
type correctionMatch struct {
	NodeID     string
	Content    string
	Summary    string
	Similarity float64
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
