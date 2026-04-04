---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: FT-INFRA
---

# RAFT Retrieval Context Enrichment

## Summary

**Feature**: RAFT Retrieval Context
**Summary**: Captures which documents were retrieved and their relevance scores alongside every LLM interaction, enabling training data that matches MDEMG's open-book inference mode. Based on UC Berkeley's RAFT research (COLM 2024).

## Vision & Goals

MDEMG operates in open-book mode — retrieval context is injected into LLM prompts. But training data that only captures LLM I/O (closed-book) degrades fine-tuned model quality by 5-15% (RAFT, COLM 2024). By capturing retrieval context alongside interactions, training data matches the actual inference-time conditions, producing better fine-tuned models.

## Current State

### Architecture

**RetrievalContext Struct** (`internal/llmclient/recorder.go`):

```go
type RetrievalContext struct {
    NodeIDs  []string  `json:"node_ids"`
    Scores   []float64 `json:"scores"`
    OracleID string    `json:"oracle_id,omitempty"`
}
```

**Context Propagation:**
- `WithRetrievalContext(ctx, rc)` — attaches RetrievalContext to Go context
- `recordInteraction()` reads it back and stores on InteractionRecord
- TSDB writer extracts into 4 new columns

### Workflow

**Wired Services:**

- **consulting/service.go — Suggest()**: After filteredResults scored, before proactive suggestion generation. Captures node IDs and scores from retrieval results.
- **jiminy/service.go — Guide()**: After constraints retrieved and filtered, before J8 LLM synthesis. Captures constraint source node IDs and confidence scores.

**TSDB Schema** (Migration 007): 4 new columns on `llm_interactions`:
- `retrieval_node_ids TEXT[]`, `retrieval_scores DOUBLE PRECISION[]`
- `oracle_node_id TEXT`, `system_prompt_hash TEXT`
- Index on `(system_prompt_hash, time DESC)`

**Training Data Strategy:**
- 80/20 split: 80% with retrieval context (open-book), 20% without (parametric recall)
- `guidance_id` correlation for joining retrieval events with quality signals
- Oracle ID marks the "correct" answer when ground truth available

### Configuration

No feature flags — RAFT context is always captured when available (nil-safe). Schema version bumped from 6 to 7.

## Notes

### Known Limitations

- Oracle ID requires manual annotation — no automatic ground truth detection
- Only 2 services currently wired (consulting, jiminy) — other LLM consumers not yet instrumented

### Risks & Gaps

None identified.

### Future Improvements

- Wire RAFT context to all 16 LLM consumers
- Automatic oracle detection from user feedback signals

## API Endpoints

None — RAFT context is captured internally, not exposed via API.

## CLI Commands

None — automatic capture, no CLI interaction needed.

## Configuration Reference

None — always active when TSDB is available.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| TimescaleDB | Requires — RAFT columns stored in llm_interactions table |
| LLM Client Recorder | Requires — context propagated via Go context |
| Consulting Service | Enhances — captures retrieval context for suggest calls |
| Jiminy Service | Enhances — captures constraint retrieval context for guidance calls |
| Training Data Pipeline | Feeds into — RAFT context used in training data export |

## Related Files

- `internal/llmclient/recorder.go` - RetrievalContext struct, WithRetrievalContext()
- `internal/llmclient/client.go` - recordInteraction() context extraction
- `internal/consulting/service.go` - Suggest() RAFT wiring
- `internal/jiminy/service.go` - Guide() RAFT wiring
- `internal/tsdb/llm_writer.go` - TSDB column extraction
- `internal/tsdb/migrations/007_raft_context.sql` - Schema migration
