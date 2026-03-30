# RAFT Retrieval Context Enrichment

## Overview
RAFT (Retrieval Augmented Fine-Tuning) context enrichment captures which documents were retrieved and their relevance scores alongside every LLM interaction. This enables training data that matches MDEMG's open-book inference mode.

## Problem
MDEMG operates in open-book mode — retrieval context is injected into LLM prompts — but training data only captured LLM I/O (closed-book). UC Berkeley's RAFT research (COLM 2024) shows this gap degrades fine-tuned model quality by 5-15%.

## Architecture

### RetrievalContext Struct
Located in `internal/llmclient/recorder.go`:
```go
type RetrievalContext struct {
    NodeIDs  []string  `json:"node_ids"`
    Scores   []float64 `json:"scores"`
    OracleID string    `json:"oracle_id,omitempty"` // the "correct" node if known
}
```

### Context Propagation
- `WithRetrievalContext(ctx, rc)` — attaches RetrievalContext to Go context
- `recordInteraction()` reads it back and stores on InteractionRecord
- TSDB writer extracts into 4 new columns

### Wired Services

**consulting/service.go — Suggest()**: After filteredResults are scored (Step 4: filter by minimum confidence threshold), before proactive suggestion generation (Step 5). Captures node IDs and scores from retrieval results.

**jiminy/service.go — Guide()**: After constraints are retrieved and filtered, before J8 LLM synthesis call. Captures constraint source node IDs and confidence scores, iterating over `item.SourceNodes` for each filtered guidance item.

### TSDB Schema (Migration 007)
New columns on llm_interactions:
- `retrieval_node_ids TEXT[]` — IDs of retrieved nodes
- `retrieval_scores DOUBLE PRECISION[]` — corresponding relevance scores
- `oracle_node_id TEXT` — the "correct" node if known (for supervised signals)
- `system_prompt_hash TEXT` — SHA-256 of system prompt used
- Index on `(system_prompt_hash, time DESC)` for prompt-version queries

Column count expanded from 22 to 26.

## Training Data Strategy
- **80/20 split**: 80% of training examples include retrieval context (open-book), 20% without (forces parametric recall)
- **guidance_id correlation**: Built in PR #219, enables joining retrieval events with downstream quality signals
- **Oracle ID**: When ground truth is available, marks which retrieved node was the "correct" answer for RAFT's oracle-document training signal

## Configuration
- Schema version bumped from 6 to 7 (`TSDB_REQUIRED_SCHEMA_VERSION`)
- No feature flags — RAFT context is always captured when available (nil-safe)

## Documents Accessed
- internal/llmclient/recorder.go
- internal/llmclient/client.go
- internal/consulting/service.go
- internal/jiminy/service.go
- internal/tsdb/llm_writer.go
- internal/tsdb/migrations/007_raft_context.sql
