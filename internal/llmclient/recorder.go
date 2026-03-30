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
}
