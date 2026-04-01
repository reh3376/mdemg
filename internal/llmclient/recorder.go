package llmclient

import (
	"context"
	"time"
)

// InteractionRecorder is an optional interface for recording LLM interactions.
// When set on a Client via SetRecorder, Complete/CompleteWithUsage will
// automatically record each interaction after the call completes.
type InteractionRecorder interface {
	Record(ctx context.Context, rec InteractionRecord)
}

// RetrievalContext captures which graph nodes were retrieved to inform an LLM call.
// This enables RAFT-style training: the fine-tuned model sees the same retrieval
// context it will encounter at inference time.
type RetrievalContext struct {
	NodeIDs  []string  `json:"node_ids"`
	Scores   []float64 `json:"scores"`
	OracleID string    `json:"oracle_id,omitempty"` // the "correct" node if known
}

// InteractionRecord captures a single LLM request/response pair.
// This struct is portable — no TSDB dependency. The TSDB writer
// implements InteractionRecorder and maps these fields to the
// llm_interactions table.
type InteractionRecord struct {
	Time         time.Time
	TraceID      string // CUIDv2
	TaskName     string // e.g. "ape.reflect", "jiminy.evaluate"
	SpaceID      string
	SessionID    string
	SystemPrompt string
	UserPrompt   string
	Response     string
	LatencyMs    int
	TokensIn     int
	TokensOut    int
	ModelName    string
	Provider     string
	Error        string

	// Correlation
	GuidanceID string // Jiminy guidance_id for feedback loop correlation
	SourcePath string // Source file path for ingest-triggered calls

	// Think content
	ThinkContent string // Extracted <think>...</think> block
	ThinkMode    bool   // Whether think mode was detected

	// Prompt versioning
	SystemPromptHash string // SHA-256 of system prompt for training data curation

	// RAFT context — retrieved nodes that informed this LLM call
	RetrievalCtx *RetrievalContext

	// Quality (populated post-hoc by annotation job)
	Quality       *float64 // 0.0-1.0, nil = not yet annotated
	QualitySource string   // "feedback_outcome", "llm_judge", "deterministic", "human"

	// Multi-instance isolation
	InstanceID string // Identifies which MDEMG instance produced this record
}
