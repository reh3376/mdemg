# Sprint Plan: Embedding Model Training Data Collection

**Date:** 2026-03-30
**Goal:** Build governed data collection infrastructure for future embedding model fine-tuning, without fine-tuning the embedding model now.
**For:** AI planning agent — build the sprint from this document

---

## Context

MDEMG uses a dedicated embedding model (`text-embedding-3-large` via OpenAI or `qwen3-embedding:8b` via Ollama) for vector search. This is architecturally separate from the generative LLM calls (OpenAI `gpt-5-nano` / `gpt-4o-mini`) targeted by the LoRA fine-tuning plan. Embedding models are trained via contrastive learning (not SFT/GRPO) — a fundamentally different technique.

The generative LoRA plan (Qwen3-30B-A3B, 16 tasks) is the primary fine-tuning workstream. Embedding fine-tuning is a future optimization. But the training data for embedding fine-tuning must be collected NOW — the same "every day without collection is lost data" principle applies.

### Why Fine-Tune the Embedding Model Later?

MDEMG standardizes on **3072-dimension vector embeddings** across all providers:
- **OpenAI `text-embedding-3-large`:** 3072 native dimensions
- **Ollama `qwen3-embedding:8b`:** 4096 native dimensions → MRL-truncated to 3072
- **Neo4j vector index:** hardcoded to 3072 (`vectorIndexDimensions` constant in `embeddings.go`)

Any future fine-tuned embedding model must produce **3072-dimension vectors** to remain compatible with the existing Neo4j vector index and all stored embeddings. This is a hard architectural constraint — changing dimensions would require re-embedding every node in the graph (34K+ nodes).

MDEMG has a unique chunking strategy: 27 language parsers produce AST-aware code elements (functions, classes, structs, interfaces, enums, modules) with rich metadata (element kind, package, file path, line ranges, signatures, concerns). Generic embedding models receive this as flat text — they don't know:

- A function boundary was identified by a Go regex parser, not arbitrary text splitting
- Two functions from the same package have structural similarity beyond textual similarity
- Import groups, decorator chains, and method signatures have domain-specific relevance in MDEMG's retrieval context
- Constraint nodes should cluster differently from regular observation nodes

A domain-fine-tuned embedding model trained on MDEMG's actual chunk format + retrieval quality signals would produce embeddings better aligned to MDEMG's retrieval needs.

### What Contrastive Training Needs

Embedding fine-tuning uses contrastive learning: pull positive pairs (query, relevant_document) closer in embedding space, push negative pairs (query, irrelevant_document) apart. The most valuable training signal is **hard negatives** — documents close in current embedding space but not actually relevant.

MDEMG already produces this data through its retrieval pipeline:
1. Vector recall returns candidates with cosine similarity scores (what the embedding thinks is similar)
2. Cross-encoder reranking re-scores them (ground truth relevance)
3. The gap between these rankings IS the hard-negative mining signal

---

## What Data to Collect

### Source 1: Embedding Events (every Embed/EmbedBatch call)

Every time text is embedded, capture what was embedded and why:

| Field | Source | Training Value |
|---|---|---|
| Text content | The string passed to Embed() | The document/query being embedded |
| Text hash (SHA-256) | Computed | Dedup in training data |
| Event type | Call site context | Distinguish ingest vs query vs internal |
| Element kind | Parser metadata | "function", "class", "struct", "file", "section" |
| Language | Parser tag | Go, Python, TypeScript, etc. |
| File path | IngestRequest.Path or parser | Source document path |
| Chunk boundaries | Parser start_line/end_line | Where this chunk sits in the file |
| Package/signature | Parser metadata | Structural context for code chunks |
| Call site | Which MDEMG component triggered | "ingest", "retrieve", "consult", "jiminy.evaluate" |
| Model used | Embedder.Name() | text-embedding-3-large, qwen3-embedding:8b |
| Dimensions | Embedder.Dimensions() | 3072, 1536, etc. |
| Cache hit | CachedEmbedder hit/miss | Volume estimation, cache effectiveness |
| Node ID | Created/matched Neo4j node | Links embedding to graph node |

### Source 2: Retrieval Events (every Retrieve pipeline)

Every time a retrieval pipeline runs, capture the full journey from query to result:

| Field | Source | Training Value |
|---|---|---|
| Query text | RetrieveRequest.QueryText | The search query |
| Recall candidates + scores | vectorRecall() output | Pre-rerank cosine similarities |
| BM25 candidates + scores | bm25Recall() output | Lexical match scores |
| Rerank candidates + scores | Cross-encoder output | Ground truth relevance |
| Final results | Retrieve() response | What was actually returned |
| Call site | Which component triggered | "retrieve", "consult", "jiminy.evaluate" |
| guidance_id | Context (PR #219) | Links to Jiminy feedback outcomes |
| Downstream quality | Post-hoc annotation | Was the retrieval useful? |

### Source 3: Existing Collectors (already built)

| Source | What It Captures | Embedding Training Value |
|---|---|---|
| Rerank JSONL collector | query + candidates + rerank_scores | Positive/negative pair labels |
| Protocol JSONL collector | constraint + tier + outcome + comprehension | Downstream quality for constraint retrieval |
| LLM interaction logger | All 16 task I/O including jiminy.evaluate | Quality of embedding-dependent tasks |

---

## Architecture

### New Tables

**`embedding_events`** — every Embed() call with parser metadata and context

```sql
CREATE TABLE IF NOT EXISTS embedding_events (
    time           TIMESTAMPTZ      NOT NULL,
    event_id       TEXT             NOT NULL,
    event_type     TEXT             NOT NULL,  -- 'ingest', 'query', 'internal'
    space_id       TEXT             NOT NULL,
    text_content   TEXT             NOT NULL,
    text_hash      TEXT             NOT NULL,
    text_length    INTEGER          NOT NULL,
    element_kind   TEXT,            -- 'function', 'class', 'struct', 'file', etc.
    language       TEXT,            -- 'go', 'python', 'typescript', etc.
    file_path      TEXT,
    chunk_start    INTEGER,
    chunk_end      INTEGER,
    package_name   TEXT,
    signature      TEXT,
    tags           TEXT[],
    call_site      TEXT,            -- 'ingest', 'retrieve', 'consult', 'jiminy.evaluate'
    query_text     TEXT,            -- for query events: the search query
    model_name     TEXT,
    provider       TEXT,
    dimensions     INTEGER,
    latency_ms     INTEGER,
    cached         BOOLEAN          DEFAULT FALSE,
    node_id        TEXT
);

SELECT create_hypertable('embedding_events', 'time',
    chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_embedding_events_type
    ON embedding_events (event_type, time DESC);
CREATE INDEX IF NOT EXISTS idx_embedding_events_element_kind
    ON embedding_events (element_kind, time DESC)
    WHERE element_kind IS NOT NULL;
```

**`retrieval_events`** — every full retrieval pipeline with pre/post rerank scores

```sql
CREATE TABLE IF NOT EXISTS retrieval_events (
    time               TIMESTAMPTZ      NOT NULL,
    event_id           TEXT             NOT NULL,
    space_id           TEXT             NOT NULL,
    call_site          TEXT             NOT NULL,
    query_text         TEXT             NOT NULL,
    query_hash         TEXT             NOT NULL,
    recall_node_ids    TEXT[]           NOT NULL,
    recall_scores      DOUBLE PRECISION[] NOT NULL,
    recall_k           INTEGER,
    bm25_node_ids      TEXT[],
    bm25_scores        DOUBLE PRECISION[],
    rerank_node_ids    TEXT[],
    rerank_scores      DOUBLE PRECISION[],
    rerank_model       TEXT,
    result_node_ids    TEXT[],
    result_scores      DOUBLE PRECISION[],
    result_count       INTEGER,
    guidance_id        TEXT,
    downstream_quality DOUBLE PRECISION,
    recall_latency_ms  INTEGER,
    rerank_latency_ms  INTEGER,
    total_latency_ms   INTEGER
);

SELECT create_hypertable('retrieval_events', 'time',
    chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);
```

---

## Implementation Phases

### Phase 1: Embedding Event Recorder [3-4 hrs]

**1A. New file: `internal/embeddings/recorder.go`**

EmbeddingEvent struct + EmbeddingEventRecorder interface. Same pattern as llmclient.InteractionRecorder.

**1B. New file: `internal/tsdb/embedding_writer.go`**

Buffered TSDB writer using pgx.CopyFrom. Same pattern as llm_writer.go.

**1C. Context propagation for parser metadata**

Use context.WithValue to pass parser metadata from call sites to the embedder. New context key helpers in embeddings package:

```go
func WithEmbeddingMeta(ctx context.Context, kind, lang, path, callSite string) context.Context
```

Wire at call sites:
- `handleIngest` (handlers.go:554) — set from req.Tags (language), req.Path, req.Name
- `handleRetrieve` (handlers.go:405) — set callSite="retrieve"
- `consulting/service.go:165` — set callSite="consult"
- `jiminy/evaluator.go:74` — set callSite="jiminy.evaluate"
- `jiminy/outcome_classifier.go:143` — set callSite="jiminy.outcome"
- `walkCodebase` (ingest.go) — set from codeElement.Kind, .FilePath, .Package

**1D. Wire into CachedEmbedder**

Record events at the CachedEmbedder level to capture cache hits/misses. Call `recorder.RecordEmbed(ctx, event)` after each Embed/EmbedBatch call.

### Phase 2: Retrieval Event Recorder [3-4 hrs]

**2A. New file: `internal/retrieval/retrieval_recorder.go`**

RetrievalEvent struct + RetrievalEventRecorder interface.

**2B. New file: `internal/tsdb/retrieval_writer.go`**

Buffered TSDB writer.

**2C. Wire into retrieval/service.go Retrieve()**

After the full pipeline completes (vector recall → BM25 → fusion → rerank → result), record the event with all pipeline stages. The recall candidates + scores, rerank candidates + scores, and final results are all available at the end of Retrieve().

### Phase 3: TSDB Migration [30 min]

**New file: `internal/tsdb/migrations/007_embedding_events.sql`**

Creates both tables with hypertable conversion and indexes. Update schema version 5 → 6.

### Phase 4: Data Monitoring [1-2 hrs]

Extend `mdemg data status` and `mdemg data stats` to include embedding + retrieval event counts, per-type breakdowns, element kind distribution, cache hit rates.

### Phase 5: Configuration [30 min]

```go
EmbeddingEventLogging  bool  // EMBEDDING_EVENT_LOGGING (default: true)
RetrievalEventLogging  bool  // RETRIEVAL_EVENT_LOGGING (default: true)
```

Both default ON — collect from day one.

### Phase 6: Privacy Scrubbing [30 min]

Reuse existing `llmclient.Scrub` patterns on text_content in embedding events. Same 5 regex patterns.

---

## Execution Order

```
Phase 3: Migration (create tables first)       [30 min]
Phase 5: Configuration                         [30 min]
Phase 1: Embedding event recorder              [3-4 hrs]
Phase 2: Retrieval event recorder              [3-4 hrs]
Phase 6: Privacy scrubbing                     [30 min]
Phase 4: Data monitoring                       [1-2 hrs]
```

**Estimated total:** ~10-12 hours

---

## Files Summary

### New Files (6)

| File | Language | Phase | Purpose |
|---|---|---|---|
| `internal/embeddings/recorder.go` | Go | 1A | EmbeddingEvent struct + recorder interface + context helpers |
| `internal/tsdb/embedding_writer.go` | Go | 1B | Buffered TSDB writer for embedding events |
| `internal/retrieval/retrieval_recorder.go` | Go | 2A | RetrievalEvent struct + recorder interface |
| `internal/tsdb/retrieval_writer.go` | Go | 2B | Buffered TSDB writer for retrieval events |
| `internal/tsdb/migrations/007_embedding_events.sql` | SQL | 3 | Both tables + hypertable + indexes |
| `docs/tests/uobs/specs/embedding_event_logging.uobs.json` | JSON | — | Observability spec |

### Modified Files (6)

| File | Phase | Change |
|---|---|---|
| `internal/embeddings/embeddings.go` | 1D | Wire recorder into CachedEmbedder |
| `internal/api/handlers.go` | 1C | Set embedding context on ingest + retrieve |
| `internal/api/server.go` | 1B, 2B | Initialize writers, wire recorders |
| `internal/retrieval/service.go` | 2C | Record retrieval events after Retrieve() |
| `internal/cli/data.go` | 4 | Extend status/stats for embedding + retrieval data |
| `internal/config/config.go` | 3, 5 | Schema version + config fields |

---

## Contrastive Training Pair Extraction (Future — NOT This Sprint)

When embedding fine-tuning begins, the collected data produces contrastive pairs:

```python
def extract_contrastive_pairs(retrieval_events, embedding_events):
    for event in retrieval_events:
        query = event.query_text
        
        # Positive: high rerank score → relevant to query
        for nid, score in zip(event.rerank_node_ids, event.rerank_scores):
            if score > 0.7:
                text = lookup_text_from_embedding_events(nid)
                yield ("positive", query, text)
        
        # Hard negative: high cosine sim BUT low rerank → close but irrelevant
        for nid, recall_score in zip(event.recall_node_ids, event.recall_scores):
            rerank_score = lookup_rerank_score(nid, event)
            if recall_score > 0.5 and (rerank_score is None or rerank_score < 0.3):
                text = lookup_text_from_embedding_events(nid)
                yield ("hard_negative", query, text)
```

The gap between vector recall and cross-encoder reranking is the hard-negative mining signal — exactly what Cisco/NVIDIA and UC Berkeley use in their embedding fine-tuning recipes (NDCG@10 improvements of 5-15%).

---

## FT Plan Document Updates Required

| Document | Section | Change |
|---|---|---|
| 01_RESEARCH.md | §1.2 | Add note: embedding model is separate workstream, not part of generative LoRA scope |
| 01_RESEARCH.md | NEW §1.4 | Embedding fine-tuning overview: contrastive learning, parser-aware chunks, data collection |
| 05_DATA_COLLECTION.md | §1.1 | Add embedding_events + retrieval_events to active collection table |
| 05_DATA_COLLECTION.md | §4.1 | Add both tables to storage architecture |
| 06_CORRECTIONS_APPLIED.md | v3.0 | Add ISSUE 20: embedding scope separated from generative LoRA, data collection added |
| Deep-dive analysis | Part 1 | Fix: MDEMG uses OpenAI models (gpt-5-nano/gpt-4o-mini), not Claude |
