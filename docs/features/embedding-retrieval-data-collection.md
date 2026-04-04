---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "FT-INFRA"
---

# Embedding and Retrieval Data Collection

## Summary

**Feature**: Embedding & Retrieval Data Collection
**Summary**: Infrastructure for collecting every embedding call and retrieval pipeline execution into TimescaleDB, enabling future domain-specific embedding model training via contrastive learning.


## Overview

Infrastructure for collecting every embedding call and retrieval pipeline execution, enabling future domain-specific embedding model training via contrastive learning.

## Problem

A domain-tuned embedding model trained on MDEMG's 27-parser AST-aware chunks would outperform generic models. The most valuable training signal — hard-negative mining (high vector similarity + low rerank score) — was being lost because retrieval pipeline internals weren't recorded.

## Architecture

### Two New TimescaleDB Hypertables

**embedding_events** — captures every `Embed()`/`EmbedBatch()` call:

- Event metadata: `event_id`, `time`, `event_type`, `space_id`
- Text: `text_content` (privacy-scrubbed), `text_hash`, `text_length`
- Parser metadata: `element_kind`, `language`, `file_path`, `chunk_start`, `chunk_end`, `package_name`, `signature`
- Provenance: `call_site`, `query_text`, `tags`
- Model: `model_name`, `provider`, `dimensions`
- Performance: `latency_ms`, `cached`, `node_id`

**retrieval_events** — captures full retrieval pipeline:

- Query: `query_text`, `query_hash`, `space_id`, `call_site`
- Vector recall: `recall_node_ids[]`, `recall_scores[]`, `recall_k`
- BM25: `bm25_node_ids[]`, `bm25_scores[]`
- Rerank: `rerank_node_ids[]`, `rerank_scores[]`, `rerank_model`
- Final: `result_node_ids[]`, `result_scores[]`, `result_count`
- Correlation: `guidance_id`, `downstream_quality`
- Latency: `recall_latency_ms`, `rerank_latency_ms`, `total_latency_ms`

Both hypertables use a 7-day chunk interval. The `embedding_events` table carries indexes on `event_type`, `element_kind`, and `call_site`; `retrieval_events` carries indexes on `call_site` and `guidance_id`.

### Context Propagation (WithEmbeddingMeta)

`embeddings.WithEmbeddingMeta(ctx, meta)` attaches parser metadata to the Go context. The `CachedEmbedder` reads this metadata when recording events. Wired at 9 call sites:

| # | File | CallSite | Key Metadata |
|---|------|----------|--------------|
| 1 | `internal/api/handlers.go` (handleIngest) | `"ingest"` | SpaceID, FilePath, Tags |
| 2 | `internal/api/handlers.go` (handleRetrieve) | `"retrieve"` | SpaceID, QueryText |
| 3 | `internal/consulting/service.go` (Consult) | `"consult"` | SpaceID, Tags, QueryText |
| 4 | `internal/consulting/service.go` (Suggest) | `"consult.suggest"` | SpaceID, FilePath, Tags |
| 5 | `internal/jiminy/evaluator.go` (Evaluate) | `"jiminy.evaluate"` | SpaceID, FilePath, QueryText |
| 6 | `internal/jiminy/outcome_classifier.go` (Classify) | `"jiminy.outcome"` | ElementKind, QueryText |
| 7 | `internal/conversation/service.go` (Observe) | `"conversation"` | SpaceID, Tags, QueryText |
| 8 | `internal/guardrail/constraint_retrieval.go` (semanticSearch) | `"guardrail"` | SpaceID, FilePath, QueryText |
| 9 | `internal/jiminy/service.go` (Guide) | `"jiminy.guide"` | SpaceID, FilePath, QueryText |

### Buffered Writer Pattern

Both `EmbeddingEventWriter` and `RetrievalEventWriter` follow the same pattern as `LLMInteractionWriter`:

- `poolIface` abstraction (pgx connection pool) — avoids import cycles
- Buffer with capacity 32
- Flush-swap under mutex lock (swap buffer under lock, write outside lock)
- `pgx.CopyFrom` for efficient batch inserts
- Auto-flush ticker (configurable interval, defaults to 30 seconds)
- `Close()` with final flush after stopping the ticker

The `EmbeddingEventRow` type in `internal/tsdb/embedding_writer.go` mirrors `embeddings.EmbeddingEvent` without importing the `embeddings` package, which would create an import cycle through `metrics → tsdb`.

### Privacy Scrubbing

`llmclient.ScrubString()` is applied to `text_content` inside `EmbeddingEventWriter.Record()` before the row enters the buffer. This applies the same 5 regex categories used for LLM interaction scrubbing: API keys, bearer tokens, passwords, SSNs, and credit card numbers. Retrieval events do not scrub query text (query text is already stored in the LLM interaction log and is not privacy-gated at this layer).

## Hard-Negative Mining

The most valuable signal for contrastive training:

- **High vector similarity + low rerank score** = hard negative (semantically similar but not task-relevant)
- Captured by recording both `recall_scores` (vector cosine similarity) and `rerank_scores` (cross-encoder score) in `retrieval_events`
- These pairs teach the embedding model to distinguish semantic similarity from task-relevant similarity — the core limitation of generic embedding models on domain-specific corpora

## Contrastive Learning vs SFT/GRPO

| Axis | Phases A–C (Generative LoRA) | Phase D (Embedding Fine-tuning) |
|------|------------------------------|---------------------------------|
| Model | Qwen3-30B-A3B | Domain-specific embedding model |
| Technique | SFT + GRPO | Contrastive learning |
| Training signal | LLM I/O from 16 tasks | Hard-negative text pairs |
| Goal | Better reasoning/generation | Better semantic retrieval |

These are fundamentally different training techniques operating on different model types. Phase D data collection is infrastructure-only; the training run is a future phase.

## Configuration

| Environment Variable | Default | Purpose |
|----------------------|---------|---------|
| `EMBEDDING_EVENT_LOGGING` | `true` | Enable/disable embedding event collection |
| `RETRIEVAL_EVENT_LOGGING` | `true` | Enable/disable retrieval event collection |

Both default to `true` so collection begins from day one without requiring explicit opt-in.

The 3072-dimension constraint is enforced by three components that must stay in sync: the Neo4j vector index, OpenAI `text-embedding-3-large`, and Ollama `qwen3-embedding:8b` MRL truncation.

## Migration

Migration 006 (`internal/tsdb/migrations/006_embedding_retrieval_events.sql`) creates both hypertables and their indexes in a single transaction. Sets schema version to `6` in `tsdb_schema_meta`.

## Documents Accessed

- `internal/embeddings/recorder.go`
- `internal/retrieval/retrieval_recorder.go`
- `internal/tsdb/embedding_writer.go`
- `internal/tsdb/retrieval_writer.go`
- `internal/tsdb/migrations/006_embedding_retrieval_events.sql`
- `docs/development/ft-lora/ft-lora-dev/SPRINT_EMBEDDING_DATA_COLLECTION_v2.md`
- `docs/features/training-data-capture-verification.md` — column-position verification tests, privacy scrub analysis, silent failure modes
