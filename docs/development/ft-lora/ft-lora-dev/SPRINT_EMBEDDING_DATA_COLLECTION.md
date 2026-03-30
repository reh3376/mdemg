# Sprint Plan: Embedding Model Training Data Collection

**Date:** 2026-03-30
**Goal:** Start collecting governed data for future contrastive fine-tuning of a domain-specific embedding model. Collection starts now; training happens later.
**Scope:** Data collection infrastructure only — no embedding model training in this sprint
**For:** AI planning agent — build the sprint from this document

---

## Why This Matters

MDEMG uses `text-embedding-3-large` (OpenAI) or `qwen3-embedding:8b` (Ollama) to embed code chunks, observations, constraints, and queries into 3072-dim vectors stored in Neo4j. Retrieval quality is the foundation of everything downstream — Jiminy guidance, constraint detection, RSIC reflection, and the consulting service all depend on accurate vector retrieval.

General-purpose embedding models don't understand MDEMG's domain-specific semantics:

- A Go function implementing a constraint and a CLAUDE.md rule describing that same constraint should embed close together — but generic models don't know they're related
- Parser-produced chunks (function bodies, import blocks, class definitions) have structural patterns that generic sentence embeddings miss
- MDEMG queries are developer-intent queries ("what constraints apply to this handler?"), not web-search queries

Domain-specific contrastive fine-tuning on MDEMG's actual retrieval data can close this gap. But training requires curated datasets of (query, relevant_passage, hard_negative) triples that take months to accumulate. **Every retrieval event that goes unlogged is a lost training signal.**

---

## Current State

### What's Captured Today

| Signal | Storage | Status | Value for Embedding Training |
|---|---|---|---|
| Rerank JSONL (query, candidates, scores) | `.mdemg/neural/training-data/` | **OFF** (config flip) | ✅ **Primary** — cross-encoder relevance scores are implicit contrastive labels |
| Vector similarity scores | Not persisted | Lost after retrieval | Medium — shows what the current embedding thinks is similar |
| BM25 lexical scores | Not persisted | Lost after retrieval | Low — lexical signal, not semantic |
| Guidance feedback (followed/ignored) | TimescaleDB `llm_interactions` + protocol JSONL | ✅ ON / OFF | ✅ **High** — downstream signal: was the retrieved node actually useful? |

### What's Missing (This Sprint)

| Signal | Why It Matters | Effort |
|---|---|---|
| **Retrieval event log** — full record of each retrieval call with query text, results, scores | The core training data: what query retrieved what nodes at what scores | M |
| **Chunk provenance** — which parser, language, chunk type produced each embedded node | Enables per-domain embedding quality analysis | S |
| **Retrieval-to-guidance linkage** — which retrieved nodes became Jiminy guidance | Downstream success signal: was the retrieval actually useful? | M |
| **Hard negative identification** — nodes with high vector sim but low rerank score | The most valuable contrastive training signal for improving retrieval | S |

---

## Architecture: What Gets Collected

### The Retrieval Event Record

Every call to `retrieval.Service.Retrieve()` generates a record:

```go
// RetrievalEventRecord captures a single retrieval call for embedding training.
// Stored in TimescaleDB retrieval_events hypertable.
type RetrievalEventRecord struct {
    Time             time.Time   // When the retrieval happened
    TraceID          string      // CUIDv2, links to LLM interaction if downstream
    SpaceID          string      // Which memory space
    QueryText        string      // The raw query text
    QuerySource      string      // "jiminy_warm", "consult", "retrieve_api", "search"
    
    // Results (top-K, ordered by final score)
    ResultNodeIDs    []string    // Node IDs returned
    ResultScores     []float64   // Final blended scores
    ResultVectorSims []float64   // Raw vector cosine similarities (before rerank/fusion)
    
    // Rerank signal (if reranking was applied)
    RerankApplied    bool        // Was cross-encoder/LLM rerank used?
    RerankScores     []float64   // Cross-encoder scores (most valuable training signal)
    
    // Downstream linkage
    GuidanceID       string      // If this retrieval fed Jiminy guidance
    UsedNodeIDs      []string    // Which results were actually used downstream
    
    // Query metadata
    QueryClassType   string      // From query_classifier: "factual", "procedural", "constraint", etc.
    IntentRewritten  string      // If intent_translator rewrote the query
    
    // Retrieval config at time of call
    TopK             int         // How many results were requested
    VectorWeight     float64     // Hybrid retrieval vector weight
    BM25Weight       float64     // Hybrid retrieval BM25 weight
}
```

### The Chunk Provenance Record

When a node is created/updated via ingest, record how it was chunked:

```go
// ChunkProvenanceRecord captures how embedded content was produced.
// Stored on the Neo4j node as properties (not a separate table).
type ChunkProvenanceRecord struct {
    ParserName    string // "go", "python", "markdown", "yaml", etc. (from UPTS)
    ChunkType     string // "function", "class", "import_block", "comment", "file_summary", "observation"
    Language      string // Programming language or "natural" for prose
    FileExtension string // ".go", ".py", ".md"
    LineStart     int    // Start line in source file
    LineEnd       int    // End line in source file
    SymbolCount   int    // Number of symbols in this chunk
    EmbedText     string // The EXACT text that was passed to embedder.Embed()
    EmbedTextHash string // SHA-256 of embed text (for dedup without storing full text)
    ContentHash   string // SHA-256 of raw content (before "name: content" prefixing)
}
```

### The Contrastive Training Pair (Derived, Not Stored)

During dataset curation (future), these records produce contrastive pairs:

```
Positive pair:  (query_text, node_content) where rerank_score > 0.7
Hard negative:  (query_text, node_content) where vector_sim > 0.5 BUT rerank_score < 0.3
Easy negative:  (query_text, random_node_content) from different space/domain

Gold positive:  (query_text, node_content) where the node was used in guidance that was FOLLOWED
Gold negative:  (query_text, node_content) where the node was retrieved but guidance was IGNORED
```

The rerank scores provide **implicit relevance labels** — the cross-encoder already judges whether a candidate is actually relevant to the query. Nodes with high vector similarity but low rerank scores are **hard negatives** — exactly what contrastive learning needs to improve the embedding space.

---

## Implementation Phases

### Phase 1: Retrieval Event Logger

**New file: `internal/retrieval/event_logger.go`**

```go
// RetrievalEventLogger records retrieval calls to TimescaleDB.
// Same pattern as LLMInteractionWriter: buffer + periodic flush via pgx CopyFrom.
type RetrievalEventLogger struct {
    pool      poolIface
    buffer    []RetrievalEventRecord
    mu        sync.Mutex
    flushTick *time.Ticker
    done      chan struct{}
}

// Record buffers a retrieval event for async flush.
func (l *RetrievalEventLogger) Record(rec RetrievalEventRecord) {
    if rec.TraceID == "" { rec.TraceID = cuid2.Generate() }
    l.mu.Lock()
    l.buffer = append(l.buffer, rec)
    l.mu.Unlock()
}
```

**Wire into `retrieval/service.go` — `Retrieve()` method:**

After retrieval completes (results scored and sorted), before returning:

```go
if s.eventLogger != nil {
    s.eventLogger.Record(RetrievalEventRecord{
        Time:             time.Now(),
        SpaceID:          req.SpaceID,
        QueryText:        req.QueryText,
        QuerySource:      querySource, // passed via context or parameter
        ResultNodeIDs:    extractNodeIDs(results),
        ResultScores:     extractScores(results),
        ResultVectorSims: extractVectorSims(results),
        TopK:             req.TopK,
        VectorWeight:     hints.VectorWeight,
        BM25Weight:       hints.BM25Weight,
    })
}
```

**Wire rerank scores in `retrieval/rerank.go`:**

After reranking completes, enrich the event record with rerank scores. Use `context.WithValue` to pass the rerank scores back to the Retrieve caller (same pattern as guidance_id).

**Config:**

```go
RetrievalEventLogging  bool   // RETRIEVAL_EVENT_LOGGING — log retrieval calls to TSDB (default: true)
```

Default ON — same rationale as LLM interaction logging.

### Phase 2: TimescaleDB Schema

**New migration: `internal/tsdb/migrations/007_retrieval_events.sql`**

```sql
CREATE TABLE IF NOT EXISTS retrieval_events (
    time              TIMESTAMPTZ      NOT NULL,
    trace_id          TEXT             NOT NULL,
    space_id          TEXT             NOT NULL,
    query_text        TEXT             NOT NULL,
    query_source      TEXT,
    
    -- Results
    result_node_ids   TEXT[],
    result_scores     DOUBLE PRECISION[],
    result_vector_sims DOUBLE PRECISION[],
    
    -- Rerank
    rerank_applied    BOOLEAN          NOT NULL DEFAULT FALSE,
    rerank_scores     DOUBLE PRECISION[],
    
    -- Downstream linkage
    guidance_id       TEXT,
    used_node_ids     TEXT[],
    
    -- Query metadata
    query_class_type  TEXT,
    intent_rewritten  TEXT,
    
    -- Config
    top_k             INTEGER,
    vector_weight     DOUBLE PRECISION,
    bm25_weight       DOUBLE PRECISION
);

SELECT create_hypertable('retrieval_events', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- Index for query analysis
CREATE INDEX IF NOT EXISTS idx_retrieval_events_space
    ON retrieval_events (space_id, time DESC);

-- Index for hard negative mining
CREATE INDEX IF NOT EXISTS idx_retrieval_events_rerank
    ON retrieval_events (rerank_applied, time DESC)
    WHERE rerank_applied = TRUE;

UPDATE tsdb_schema_meta SET value = '7' WHERE key = 'schema_version';
```

### Phase 3: Chunk Provenance on Neo4j Nodes

**Modify: `internal/api/handlers.go` — `handleIngest()`**

When embedding text is constructed (the `"name: content"` string), store the provenance on the node:

```go
// Store what was embedded for training provenance
embedProps := map[string]any{
    "embed_text_hash": fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(textForEmbedding))),
    "embed_source":    req.Source,    // "ingest-claude-md", "ingest-codebase", "observe"
}
```

**Modify: `internal/cli/ingest.go` — `walkCodebase()`**

When building codeElements from parsed files, include parser metadata in tags:

```go
elem := codeElement{
    Name:     symbol.Name,
    Kind:     symbol.Type,
    Path:     filePath,
    Content:  content,
    Tags:     append(tags, "parser:"+parserName, "lang:"+language, "chunk:"+chunkType),
}
```

These tags flow through ingest to the Neo4j node, making parser provenance queryable.

**Modify: `internal/cli/ingest_claude_md.go`**

Add tags for claude-md provenance:

```go
"tags": append(tags, "parser:markdown", "lang:natural", "chunk:document"),
```

### Phase 4: Retrieval-to-Guidance Linkage

**Modify: `internal/jiminy/service.go` — `Guide()`**

When Jiminy retrieves nodes for guidance (line ~655-674), capture which node IDs were retrieved and pass them through context so the retrieval event logger can link them:

```go
results, err := s.retriever.RetrieveForJiminy(ctx, req.SpaceID, queryText, ...)
retrievalItems := mapRetrievalToGuidance(results)

// NEW: Link retrieval results to guidance_id for embedding training
ctx = retrieval.WithRetrievalNodeIDs(ctx, extractNodeIDs(results))
```

When feedback arrives via `RecordOutcome()`, the guidance_id already links to the interaction. Now we can join:
```
retrieval_events.guidance_id → llm_interactions.guidance_id → protocol JSONL outcome
```

This tells us: "These nodes were retrieved → they became guidance → the guidance was followed/ignored." That's the gold-standard signal for embedding quality: **did retrieval lead to useful guidance?**

### Phase 5: Hard Negative Mining Query

Not a code change — a Python script for dataset curation that identifies hard negatives:

**New file: `neural/training/embedding_data_curator.py`**

```python
"""Curate embedding training data from retrieval events.

Produces contrastive training pairs:
- Positive: (query, passage) where rerank_score > threshold
- Hard negative: (query, passage) where vector_sim > 0.5 AND rerank_score < 0.3
- Gold positive: (query, passage) where downstream guidance was FOLLOWED
- Gold negative: (query, passage) where guidance was IGNORED

Usage:
    python -m training.embedding_data_curator \
        --tsdb-dsn "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics" \
        --neo4j-uri bolt://localhost:7687 \
        --output .mdemg/neural/datasets/embedding/v1/ \
        --min-rerank-score 0.7 \
        --hard-neg-threshold 0.3
"""

def mine_hard_negatives(dsn: str, neo4j_uri: str) -> list[dict]:
    """Find nodes with high vector similarity but low rerank scores.
    
    These are the most valuable contrastive training examples because
    they teach the embedding model to distinguish "looks similar" from
    "actually relevant" in MDEMG's domain.
    """
    query = """
    SELECT 
        re.query_text,
        unnest(re.result_node_ids) as node_id,
        unnest(re.result_vector_sims) as vector_sim,
        unnest(re.rerank_scores) as rerank_score
    FROM retrieval_events re
    WHERE re.rerank_applied = TRUE
      AND re.time > NOW() - INTERVAL '90 days'
    """
    # Filter: vector_sim > 0.5 AND rerank_score < 0.3 → hard negative
    # Filter: rerank_score > 0.7 → positive pair
    ...

def mine_gold_pairs(dsn: str) -> list[dict]:
    """Find retrieval → guidance → outcome chains.
    
    Join retrieval_events → llm_interactions → protocol JSONL
    via guidance_id to find retrievals that led to followed/ignored guidance.
    """
    query = """
    SELECT 
        re.query_text,
        re.result_node_ids,
        li.quality,
        li.quality_source
    FROM retrieval_events re
    JOIN llm_interactions li ON re.guidance_id = li.guidance_id
    WHERE li.quality IS NOT NULL
      AND li.task_name = 'jiminy.synthesize'
    """
    ...
```

### Phase 6: Data Monitoring

**Modify: `internal/cli/data.go` — add embedding data status**

```bash
mdemg data status
# Output now includes:
#
# Embedding Training Data:
#   Retrieval events:     4,521 (last 30 days)
#   With rerank scores:   2,103 (46.5%)
#   Hard negatives:       312 (14.8% of reranked)
#   Gold pairs:           89 (via guidance feedback)
#   Unique queries:       1,847
#   Unique nodes hit:     3,201
```

### Phase 7: Backup Integration

**Modify: `internal/tsdb/backup.go`**

The `retrieval_events` table is in TimescaleDB, so it's automatically included in the existing `pg_dump` backup. No additional work needed — the TSDB backup already covers all hypertables.

---

## Execution Order

```
Phase 1: Retrieval event logger          [3-4 hrs]
    ├→ event_logger.go (buffer + flush pattern)
    ├→ Wire into retrieval/service.go Retrieve()
    ├→ Wire rerank scores via context
    └→ Config: RETRIEVAL_EVENT_LOGGING=true

Phase 2: TimescaleDB migration           [30 min]
    └→ Migration 007 (retrieval_events hypertable)

Phase 3: Chunk provenance                [1-2 hrs]
    ├→ embed_text_hash on ingest
    ├→ Parser tags in walkCodebase
    └→ Parser tags in ingest-claude-md

Phase 4: Retrieval-to-guidance linkage   [1-2 hrs]
    ├→ Capture retrieved node IDs in jiminy/service.go
    └→ Pass through context to retrieval event record

Phase 5: Hard negative mining script     [2-3 hrs]
    └→ neural/training/embedding_data_curator.py

Phase 6: CLI monitoring                  [1 hr]
    └→ Add embedding section to mdemg data status

Phase 7: Backup verification             [15 min]
    └→ Verify retrieval_events in pg_dump
```

**Total estimated effort: ~10-12 hours**

Dependencies: Phase 1 → Phase 2 (schema must exist before logger writes). Phases 3-4 can run in parallel. Phase 5 depends on data accumulation (run after weeks of collection). Phases 6-7 are independent.

---

## Files Summary

### New Files (3)

| File | Language | Phase | Purpose |
|---|---|---|---|
| `internal/retrieval/event_logger.go` | Go | 1 | Retrieval event buffer + TSDB writer |
| `internal/tsdb/migrations/007_retrieval_events.sql` | SQL | 2 | retrieval_events hypertable |
| `neural/training/embedding_data_curator.py` | Python | 5 | Contrastive pair mining from retrieval events |

### Modified Files (7)

| File | Phase | Change |
|---|---|---|
| `internal/retrieval/service.go` | 1 | Record retrieval events after Retrieve() completes |
| `internal/retrieval/rerank.go` | 1 | Pass rerank scores via context for event enrichment |
| `internal/api/server.go` | 1 | Wire RetrievalEventLogger, shutdown ordering |
| `internal/config/config.go` | 1 | RETRIEVAL_EVENT_LOGGING config, schema version 5→7 |
| `internal/api/handlers.go` | 3 | embed_text_hash on ingest |
| `internal/cli/ingest.go` | 3 | Parser provenance tags |
| `internal/jiminy/service.go` | 4 | Capture retrieved node IDs for guidance linkage |
| `internal/cli/data.go` | 6 | Embedding data section in status output |

### Config Changes

| Variable | Default | Purpose |
|---|---|---|
| `RETRIEVAL_EVENT_LOGGING` | `true` | Log retrieval calls to retrieval_events table |

---

## Data Volume Estimates

| Signal | Records/Day | Records/Month | Storage/Month |
|---|---|---|---|
| Retrieval events | ~100-300 | ~3,000-9,000 | ~5-15 MB |
| With rerank scores | ~50-150 (50% of events) | ~1,500-4,500 | Included above |
| Hard negatives (derived) | ~15-45 (30% of reranked) | ~450-1,350 | Derived, not stored |
| Gold pairs (derived) | ~5-15 (10% of reranked with feedback) | ~150-450 | Derived, not stored |

At this rate, after 3 months: ~9,000-27,000 retrieval events, ~1,350-4,050 hard negatives, ~450-1,350 gold pairs. This is sufficient for a meaningful contrastive fine-tuning experiment on a small embedding model.

---

## How This Feeds Future Embedding Fine-Tuning

When ready to fine-tune:

1. **Run `embedding_data_curator.py`** — mines hard negatives and gold pairs from accumulated retrieval events
2. **Choose base model** — `bge-m3`, `gte-Qwen2-7B-instruct`, or `text-embedding-3-large` (if OpenAI releases weights)
3. **Contrastive training** with `sentence-transformers` — MultipleNegativesRankingLoss or InfoNCE loss
4. **Evaluate on held-out retrieval events** — compare nDCG@5, MRR, recall@10 before/after
5. **A/B test in production** — run both embeddings in parallel, compare downstream guidance quality

The parser provenance metadata enables per-domain analysis: "Is embedding quality worse for Go functions than for Python classes? Are natural-language observations embedded well but code chunks poorly?" This guides which training data to emphasize.

---

## Relationship to Generative LoRA Plan

This is a **separate workstream** from the Qwen3-30B-A3B fine-tuning:

| Aspect | Generative LoRA | Embedding Fine-Tuning |
|---|---|---|
| Model | Qwen3-30B-A3B MoE (decoder) | TBD: bge-m3, gte-Qwen2, or similar (encoder) |
| Technique | SFT → GRPO/DPO (next-token prediction) | Contrastive learning (InfoNCE/MNRL loss) |
| Training data | LLM I/O from `llm_interactions` | Retrieval events from `retrieval_events` |
| Quality signal | Guidance follow rate, JSON validity | Rerank scores, downstream guidance usage |
| Hardware | MLX bf16 LoRA (~74GB) | Much smaller (~2-4GB for encoder models) |
| Timeline | After 2-3 months of LLM data collection | After 3+ months of retrieval data collection |

Both benefit from early data collection. Both are "collect now, train later" patterns. But they use completely different models, techniques, and data.

---

## Updates Required to FT Plan Docs

The v3.0 fine-tuning plan documents should add:

**01_RESEARCH.md:** New §1.4 "Embedding Model (Separate Workstream)" explaining that embeddings use a dedicated encoder model, not the generative Qwen3, and that data collection is underway.

**05_DATA_COLLECTION.md:** New §1.6 "Embedding Training Data" listing the retrieval event logger and chunk provenance as active collectors. Update the priority table with `RETRIEVAL_EVENT_LOGGING` config.

**06_CORRECTIONS_APPLIED.md:** Add ISSUE 20 (LLM provider correction: OpenAI not Claude) and ISSUE 21 (Embedding scope clarification: separate workstream, not LoRA target).
