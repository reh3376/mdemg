---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "TD-VERIFY"
---

# Training Data Capture: Structure & Failure Mode Analysis

## Summary

**Feature**: Training Data Capture Verification
**Summary**: Comprehensive verification of the TimescaleDB training data collection pipeline — 3 hypertables, 26+ columns, privacy scrubbing, schema validation, and failure mode documentation.


## Overview

MDEMG collects training data through three TimescaleDB hypertable pipelines. This document describes the exact structure of each record, the privacy scrubbing applied, and every point where data collection can silently fail.

---

## 1. Data Structure

### 1.1 LLM Interactions (`llm_interactions` — 26 columns)

Every `llmclient.Complete()` / `CompleteWithUsage()` call produces one row.

| # | Column | Type | Source | Training Use |
|---|--------|------|--------|-------------|
| 0 | `time` | `TIMESTAMPTZ` | `time.Now()` at record creation | Temporal filtering, curation windows |
| 1 | `trace_id` | `TEXT` | CUIDv2, auto-generated if empty | Row deduplication |
| 2 | `task_name` | `TEXT` | `WithContext(taskName, spaceID)` | Task-specific fine-tuning (16 tasks) |
| 3 | `space_id` | `TEXT` | `WithContext(taskName, spaceID)` | Multi-space partitioning |
| 4 | `session_id` | `TEXT` | Set by caller | Session grouping |
| 5 | `system_prompt` | `TEXT` | Last system message (scrubbed) | SFT system prompt |
| 6 | `user_prompt` | `TEXT` | Last user message (scrubbed) | SFT input |
| 7 | `response` | `TEXT` | Raw LLM response (scrubbed) | SFT target / DPO chosen |
| 8 | `think_content` | `TEXT` | Extracted `<think>...</think>` block (scrubbed) | Chain-of-thought training |
| 9 | `think_mode` | `BOOLEAN` | `true` if think block detected | Think-mode filtering |
| 10 | `latency_ms` | `INTEGER` | Wall-clock LLM call duration | Performance filtering |
| 11 | `tokens_in` | `INTEGER` | Input token count | Cost analysis |
| 12 | `tokens_out` | `INTEGER` | Output token count | Cost analysis |
| 13 | `model_name` | `TEXT` | e.g. `gpt-4o`, `qwen3.6-35b-a3b` | Model-specific curation |
| 14 | `provider` | `TEXT` | `openai` or `ollama` | Provider filtering |
| 15 | `error` | `TEXT` | Error string if call failed | Error exclusion |
| 16 | `quality` | `DOUBLE PRECISION` | 0.0–1.0, `NULL` if not annotated | GRPO/DPO reward signal |
| 17 | `quality_source` | `TEXT` | `feedback_outcome`, `llm_judge`, `deterministic`, `human` | Reward provenance |
| 18 | `used_for_train` | `BOOLEAN` | Always `false` (set by curation job) | Deduplication gate |
| 19 | `dataset_ver` | `TEXT` | Always `""` (set by curation job) | Dataset versioning |
| 20 | `guidance_id` | `TEXT` | Jiminy guidance ID from context | Feedback loop correlation |
| 21 | `source_path` | `TEXT` | Source file for ingest-triggered calls | Document linkage |
| 22 | `retrieval_node_ids` | `TEXT[]` | Retrieved node IDs (RAFT context) | RAFT training — same retrieval at inference |
| 23 | `retrieval_scores` | `FLOAT8[]` | Retrieval relevance scores | RAFT distractor detection |
| 24 | `oracle_node_id` | `TEXT` | Correct answer node if known | RAFT oracle signal |
| 25 | `system_prompt_hash` | `TEXT` | SHA-256 of system prompt | Prompt versioning, stale data filtering |

**Privacy scrubbing**: `llmclient.Scrub(&rec)` is called in `Record()` before buffering. Scrubs columns 5, 6, 7, 8 (SystemPrompt, UserPrompt, Response, ThinkContent) using 5 regex patterns:

| Pattern | Matches | Replacement |
|---------|---------|-------------|
| API keys | `sk-*`, `ghp_*`, `AKIA*`, `Bearer` tokens | `[REDACTED_KEY]` |
| Absolute paths | `/Users/*`, `/home/*`, `C:\Users\*` | `/[PATH]/last/two/components` |
| Env secrets | `PASSWORD=*`, `SECRET=*`, `TOKEN=*`, `API_KEY=*`, `PRIVATE_KEY=*` | `VAR=[REDACTED]` |
| Emails | Standard email addresses | `[EMAIL]` |
| Neo4j creds | `neo4j://user:pass@` | `neo4j://[REDACTED]@` |

**Message extraction**: When multiple system or user messages are provided, the **last message wins** — matching the OpenAI API convention where the final system message is the effective prompt.

### 1.2 Embedding Events (`embedding_events` — 23 columns)

Every `Embed()` / `EmbedBatch()` call produces one row.

| # | Column | Type | Source | Training Use |
|---|--------|------|--------|-------------|
| 0 | `time` | `TIMESTAMPTZ` | Auto-set if zero | Temporal filtering |
| 1 | `event_id` | `TEXT` | CUIDv2, auto-generated | Row deduplication |
| 2 | `event_type` | `TEXT` | `ingest` or `query` | Contrastive pair formation |
| 3 | `space_id` | `TEXT` | Space context | Multi-space partitioning |
| 4 | `text_content` | `TEXT` | Embedded text (scrubbed) | Contrastive anchor text |
| 5 | `text_hash` | `TEXT` | SHA-256 of text | Deduplication |
| 6 | `text_length` | `INTEGER` | Character count | Length filtering |
| 7 | `element_kind` | `TEXT` | Parser AST kind (function, class, etc.) | Type-stratified training |
| 8 | `language` | `TEXT` | Programming language | Language-aware batching |
| 9 | `file_path` | `TEXT` | Source file | Provenance |
| 10 | `chunk_start` | `INTEGER` | Chunk byte offset start | Chunk reconstruction |
| 11 | `chunk_end` | `INTEGER` | Chunk byte offset end | Chunk reconstruction |
| 12 | `package_name` | `TEXT` | Go/Python package | Semantic grouping |
| 13 | `signature` | `TEXT` | Function/class signature | Semantic anchor |
| 14 | `tags` | `TEXT[]` | Metadata tags | Filtering |
| 15 | `call_site` | `TEXT` | Code location that triggered embed | Call-site stratification |
| 16 | `query_text` | `TEXT` | Query string (NOT scrubbed) | Contrastive query side |
| 17 | `model_name` | `TEXT` | e.g. `text-embedding-3-large` | Model tracking |
| 18 | `provider` | `TEXT` | `openai` | Provider tracking |
| 19 | `dimensions` | `INTEGER` | 3072 | Dimension verification |
| 20 | `latency_ms` | `INTEGER` | API call duration | Performance filtering |
| 21 | `cached` | `BOOLEAN` | Cache hit/miss | Cache-hit exclusion |
| 22 | `node_id` | `TEXT` | Neo4j node ID | Graph linkage |

**Privacy scrubbing**: `ScrubString()` applied to `text_content` only (column 4). **`query_text` (column 16) is intentionally NOT scrubbed** — it is needed as-is for contrastive training pair formation. Other fields are not scrubbed.

### 1.3 Retrieval Events (`retrieval_events` — 22 columns)

Every retrieval pipeline execution (vector recall → BM25 → rerank → final) produces one row.

| # | Column | Type | Source | Training Use |
|---|--------|------|--------|-------------|
| 0 | `time` | `TIMESTAMPTZ` | Auto-set if zero | Temporal filtering |
| 1 | `event_id` | `TEXT` | CUIDv2, auto-generated | Row deduplication |
| 2 | `space_id` | `TEXT` | Space context | Multi-space partitioning |
| 3 | `call_site` | `TEXT` | `consult`, `retrieve`, etc. | Call-site analysis |
| 4 | `query_text` | `TEXT` | Search query (NOT scrubbed) | Hard-negative mining query |
| 5 | `query_hash` | `TEXT` | SHA-256 of query | Deduplication |
| 6 | `recall_node_ids` | `TEXT[]` | Vector search results | Pre-rerank population |
| 7 | `recall_scores` | `FLOAT8[]` | Vector similarity scores | Hard-negative signal |
| 8 | `recall_k` | `INTEGER` | Top-k parameter | Recall config |
| 9 | `bm25_node_ids` | `TEXT[]` | BM25 search results | Keyword baseline |
| 10 | `bm25_scores` | `FLOAT8[]` | BM25 scores | Keyword signal |
| 11 | `rerank_node_ids` | `TEXT[]` | Reranked results | Post-rerank population |
| 12 | `rerank_scores` | `FLOAT8[]` | Reranker scores | Reranker training labels |
| 13 | `rerank_model` | `TEXT` | Reranker model name | Model tracking |
| 14 | `result_node_ids` | `TEXT[]` | Final result set | Ground truth |
| 15 | `result_scores` | `FLOAT8[]` | Final scores | Ground truth scores |
| 16 | `result_count` | `INTEGER` | Results returned | Result-count filtering |
| 17 | `guidance_id` | `TEXT` | Jiminy guidance ID | Feedback correlation |
| 18 | `downstream_quality` | `FLOAT8` | Quality of downstream LLM call | End-to-end reward |
| 19 | `recall_latency_ms` | `INTEGER` | Vector recall latency | Performance analysis |
| 20 | `rerank_latency_ms` | `INTEGER` | Rerank latency | Performance analysis |
| 21 | `total_latency_ms` | `INTEGER` | Total pipeline latency | Performance analysis |

**Privacy scrubbing**: None. Retrieval events contain node IDs and query text only — no user-facing PII. Query text is preserved for hard-negative mining.

---

## 2. Silent Failure Modes

### 2.1 Configuration Gates (Data Never Collected)

Each pipeline is independently gated by a config flag. If the flag is `false`, no writer is created and no data is collected — with **no warning or error**.

| Flag | Env Var | Default | Controls |
|------|---------|---------|----------|
| `LLMInteractionLogging` | `LLM_INTERACTION_LOGGING` | `true` | LLM writer creation |
| `EmbeddingEventLogging` | `EMBEDDING_EVENT_LOGGING` | `true` | Embedding writer creation |
| `RetrievalEventLogging` | `RETRIEVAL_EVENT_LOGGING` | `true` | Retrieval writer creation |

**How to detect**: `slog.Info("tsdb: LLM interaction logger attached")` is logged at startup when enabled. Absence of this log means the writer is off.

### 2.2 TSDB Unavailable at Startup

If `SetTSDBClient()` is never called (TimescaleDB unreachable, disabled, or connection failed), **all three writers are nil**. The LLM client's default recorder is never set, so `recordInteraction()` returns immediately at `if c.recorder == nil`. Embedding and retrieval recorders are nil, so their adapter `Record()` calls are no-ops.

**Symptoms**: No log lines mentioning `tsdb:` at startup. No data in any hypertable.

**How to detect**: Check for `slog.Info("tsdb: metric writer attached")` in startup logs. The session-start hook checks TSDB health via `pg_isready` and warns if unavailable.

### 2.3 Empty TaskName (Unroutable Records)

If a consumer creates an `llmclient.Client` without calling `WithContext(taskName, spaceID)`, the record is written with `task_name = ""`. The curation pipeline cannot route these records to task-specific fine-tuning datasets.

**Current state**: All 16 production consumers wire `WithContext()`. Verified in PR #218.

**How to detect**: `SELECT count(*) FROM llm_interactions WHERE task_name = '';` — any non-zero result indicates a regression.

### 2.4 Auto-Flush Errors Swallowed

All three writers use a background goroutine that flushes every 30 seconds (configurable via `TSDB_FLUSH_INTERVAL_SEC`). If `CopyFrom` fails during auto-flush:

- The error is logged via `slog.Warn("...: auto-flush failed")` but **NOT returned to any caller**
- The buffered records are **lost** — the buffer was already cleared before `CopyFrom` was attempted
- The writer continues operating, buffering new records

This is by design (the writer should not block LLM calls on TSDB failures), but means transient TSDB issues cause silent data loss.

**How to detect**: grep logs for `auto-flush failed` or `flush failed`. The `mdemg data audit` command checks TSDB health.

### 2.5 Buffer Overflow (Not Possible — Unbounded)

The writers use a Go slice buffer (`make([]T, 0, 32)`) that grows unboundedly. There is no max buffer size. If TSDB is unreachable for an extended period and flushes fail, the buffer grows in memory. The records are lost on the next successful flush (since the failed batch was already drained from the buffer) or on process exit.

### 2.6 Process Crash Before Flush

Each writer has a `Close()` method that performs a final flush. If the server process crashes (SIGKILL, OOM) before `Close()` is called, all buffered records since the last successful flush are lost.

**Mitigation**: `mdemg service install` sets up launchd/systemd supervision that restarts the process. The 30-second flush interval limits the maximum data loss window to ~30 seconds of records.

### 2.7 Privacy Scrubbing Asymmetry

| Pipeline | Fields Scrubbed | Fields NOT Scrubbed |
|----------|-----------------|---------------------|
| LLM | SystemPrompt, UserPrompt, Response, ThinkContent (4 fields, 5 patterns) | All other fields |
| Embedding | TextContent (1 field, 5 patterns) | QueryText, FilePath, Signature, and all other fields |
| Retrieval | None | All fields (no PII expected in node IDs/scores) |

If a future code change adds user-facing text to an unscrubbed field, it would be stored in plaintext. The scrubbing boundary is documented and tested: `TestScrubBoundary` verifies that the client does NOT scrub (responsibility is in the TSDB writer), and `TestEmbeddingWriter_ScrubAsymmetry` verifies that only `TextContent` is scrubbed in the embedding pipeline.

---

## 3. Verification Test Coverage

17 tests across 5 files verify the above structure and failure modes:

| Test Group | Count | What Is Verified |
|------------|-------|------------------|
| LLM column positions | 3 | All 26 positions match, nil RetrievalCtx safe, nil Quality preserved |
| Multi-message extraction | 1 | "Last wins" rule for system/user prompt selection |
| JSON round-trip | 1 | SanitizeResponse output is valid JSON (5 input variants) |
| Privacy scrubbing | 2 | Scrub boundary (client vs writer), all 5 patterns across 4 fields |
| Embedding column positions | 3 | All 23 positions, scrub asymmetry (TextContent yes, QueryText no), tags preserved |
| Retrieval column positions | 3 | All 22 positions, no scrubbing, slice fields preserved |
| Cross-pipeline | 4 | Scrubbed values in CopyFrom, training columns initialized, batch ordering, empty TaskName regression |

Run: `go test ./internal/tsdb/... ./internal/llmclient/... -count=1 -v`

---

## 4. Data Flow Diagram

```
LLM Call                    Embed() Call               Retrieve() Pipeline
    │                           │                           │
    ▼                           ▼                           ▼
recordInteraction()         CachedEmbedder             Retriever
    │                           │                           │
    │ extract last msgs         │ WithEmbeddingMeta         │ all stage results
    │ extract <think>           │   (parser metadata)       │   (recall/bm25/rerank)
    │ SHA-256 hash              │                           │
    │ context values            │                           │
    ▼                           ▼                           ▼
InteractionRecorder         EmbeddingEventRecorder     RetrievalEventRecorder
(interface)                 (interface)                (interface)
    │                           │                           │
    ▼                           ▼                           ▼
LLMInteractionWriter        EmbeddingEventWriter       RetrievalEventWriter
    │                           │                           │
    │ Scrub(4 fields)           │ ScrubString(TextContent)  │ (no scrub)
    │ auto-gen TraceID          │ auto-gen EventID          │ auto-gen EventID
    │ buffer[]                  │ buffer[]                  │ buffer[]
    │                           │                           │
    └──────────┬────────────────┴───────────────────────────┘
               │ 30s flush interval
               ▼
    pgx.CopyFrom → TimescaleDB Hypertables
```

## Documents Accessed

- `internal/tsdb/llm_writer.go` — LLM writer, 26-column CopyFrom
- `internal/tsdb/embedding_writer.go` — Embedding writer, 23-column CopyFrom
- `internal/tsdb/retrieval_writer.go` — Retrieval writer, 22-column CopyFrom
- `internal/llmclient/recorder.go` — InteractionRecord struct, RetrievalContext
- `internal/llmclient/scrubber.go` — Scrub(), ScrubString(), 5 regex patterns
- `internal/llmclient/sanitize.go` — SanitizeResponse pipeline
- `internal/llmclient/client.go` — recordInteraction(), message extraction, think block
- `internal/api/server.go` — SetTSDBClient(), config flag gating
- `internal/config/config.go` — LLMInteractionLogging, EmbeddingEventLogging, RetrievalEventLogging
- `internal/tsdb/migrations/005_interaction_enrichment.sql` — guidance_id, source_path
- `internal/tsdb/migrations/006_embedding_retrieval_events.sql` — embedding_events, retrieval_events
- `internal/tsdb/migrations/007_raft_context.sql` — RAFT columns
