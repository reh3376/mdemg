# MDEMG Training Data Collection, Governance, Storage & Curation Plan

**Date:** 2026-03-30 (v3.0 — reflects built infrastructure PRs #217-#219, RAFT pattern, ULTS integration)
**Purpose:** Define the complete data infrastructure needed to support the fine-tuning pipeline (Phases 1-12) and ensure high-quality training data collection.

---

## 1. Current State Assessment

### 1.1 Active Data Collection (3 sources — collecting now)

| Source | Storage | Default | Data Type |
|---|---|---|---|
| **LLM Interaction Logger** (PR #217/#218) | TimescaleDB `llm_interactions` | **ON** | All 16 generative LLM call I/O with task labels |
| **RSIC Persistence** | Neo4j (write-behind) | **ON** | Health reports, reflection insights, action outcomes |
| **Jiminy Persistence** | Neo4j (write-through) | **ON** | Guidance outcomes, trust scores, feedback counts |

The LLM interaction logger captures every generative LLM call across all 16 consumers with `task_name` labels, `guidance_id` correlation, `source_path` linkage, privacy scrubbing, and think content extraction. Data flows into the `llm_interactions` hypertable with 7-day chunking, 180-day retention, and 14-day compression.

### 1.2 Inactive Data Collection (2 sources — config flip needed)

| Source | Storage | Default | Data Type |
|---|---|---|---|
| **Rerank JSONL Collector** | `.mdemg/neural/training-data/*.jsonl` | **OFF** | Query + candidates + rerank scores + latency |
| **Protocol JSONL Collector** | `.mdemg/neural/training-data/*.jsonl` | **OFF** | Constraint code, tier, outcome, comprehension, trust, sidecar arbitration |

Both are fully built and tested. They write timestamped JSONL with automatic 50MB rotation and 90-day pruning. Enabling them is a config change — zero code.

### 1.3 Built Infrastructure (PRs #217-#219)

| Component | PR | Status |
|---|---|---|
| InteractionRecorder + SetDefaultRecorder | #217 | ✅ Built |
| LLMInteractionWriter (pgx CopyFrom) | #217 | ✅ Built |
| 16 WithContext task labels | #218 | ✅ Built |
| UOBS spec (llm_interaction_logging) | #218 | ✅ Built |
| InteractionRecord enrichment (guidance_id, source_path, think, quality) | #219 | ✅ Built |
| Migration 005 (guidance_id + source_path columns + indexes) | #219 | ✅ Built |
| Privacy scrubber (scrubber.go, 5 regex patterns) | #219 | ✅ Built |
| guidance_id correlation (context.WithValue, jiminy/service.go) | #219 | ✅ Built |
| source_path linkage (consulting/service.go) | #219 | ✅ Built |
| Think content extraction (recordInteraction) | #219 | ✅ Built |
| Quality annotation (quality_annotator.py, 468 lines) | #219 | ✅ Built |
| Quality report (quality_report.py, 244 lines) | #219 | ✅ Built |
| Data CLI (mdemg data status/inspect/stats/annotate/quality) | #219 | ✅ Built |
| JSONL backup integration in TSDB backup tar | #219 | ✅ Built |
| Protocol JSONL guidance_id field | #219 | ✅ Built |

### 1.4 Not Yet Built

| Component | Plan Phase | Notes |
|---|---|---|
| SanitizeResponse / StripThinkBlock | Phase 2D-2F | Required before switching to local model |
| RAFT retrieval context capture | Phase 4A (NEW) | Enriches training data with retrieval context |
| ULTS spec framework | Phase 4B (NEW) | Formalizes LLM call contracts |
| System prompt hash in records | Phase 2H (NEW) | Enables training data versioning |
| Retrieval event logger | Embedding sprint | Captures (query, results, scores) for embedding fine-tuning |
| Chunk provenance tags | Embedding sprint | Parser name, language, chunk type on embedded nodes |
| Dataset versioner (dataset_versioner.py) | Phase 6D | Not built |
| Teacher distillation (teacher_distill.py) | Phase 4B | Not built |
| Entropy monitor (entropy_monitor.py) | Phase 6C | Not built |
| Input extractor (input_extractor.py) | Phase 4A | Not built |

### 1.5 Embedding Training Data (Separate Workstream)

Embedding fine-tuning uses contrastive learning on encoder models (not LoRA on the generative decoder). Data collection for this workstream runs in parallel with generative training data collection. The fine-tuned embedding model must produce **3072-dimension vectors** to remain compatible with the Neo4j vector index and all stored embeddings.

Current embedding dimensions across providers:
- **OpenAI `text-embedding-3-large`:** 3072 native
- **Ollama `qwen3-embedding:8b`:** 4096 native → MRL-truncated to 3072
- **Neo4j vector index:** hardcoded to 3072 (`vectorIndexDimensions` constant)

| Collector | Storage | Status | Training Signal |
|---|---|---|---|
| Embedding Event Logger | TimescaleDB `embedding_events` | ⬜ PLANNED | Every Embed() call with parser metadata (element kind, language, chunk boundaries) |
| Retrieval Event Logger | TimescaleDB `retrieval_events` | ⬜ PLANNED | (query, recall scores, rerank scores) → contrastive pairs |
| Rerank JSONL Collector | `.mdemg/neural/training-data/` | Built (OFF) | Cross-encoder relevance scores → contrastive labels |
| Chunk Provenance | Neo4j node properties | ⬜ PLANNED | Parser name, language, chunk type per node |
| Retrieval-to-Guidance Linkage | Via guidance_id join | ✅ BUILT (PR #219) | Was the retrieved node useful downstream? |

The most valuable signal for embedding improvement is **hard negatives**: nodes with high vector similarity (the current embedding thinks they're similar) but low rerank scores (the cross-encoder says they're actually not relevant). Training the embedding model on these pairs teaches it to distinguish "looks similar" from "actually relevant" in MDEMG's domain.

Both loggers default ON (`EMBEDDING_EVENT_LOGGING=true`, `RETRIEVAL_EVENT_LOGGING=true`) to start accumulating data immediately.

### 1.6 Additional Data Sources (Not LLM Calls)

| Source | Location | Content | Fine-Tuning Value |
|---|---|---|---|
| Neo4j graph (34K+ nodes) | `bolt://localhost:7687` | MemoryNodes, Observations, constraint nodes, edges | Ground truth for classification, naming, synthesis |
| UATS specs (202 specs) | `docs/api/api-spec/uats/specs/` | Expected API responses | Format validation for structured output tasks |
| UETS specs (8 specs) | `docs/tests/uets/specs/` | Emergence naming quality criteria | Evaluation rubrics |
| Git history | `.git/` | Commit messages, diffs, decisions | Context for constraint detection |
| CMS observations | Neo4j `:Observation` nodes | Append-only development events | Real-world task context |
| System prompts | 15 `const` declarations in Go source | Task definitions | Training example system messages |
| RSIC cycle logs | In-memory + RSIC persistence | Health reports, reflection insights | Training data for ape.reflect task |

---

## 2. Immediate Action (Config Flip — User Must Do This)

```bash
export NEURAL_DATA_COLLECTION=true
export J17_PROTOCOL_DATA_COLLECTION=true
export TSDB_BACKUP_ENABLED=true
```

Then restart the server. This enables the 2 remaining inactive collectors and starts backing up the llm_interactions table. **Every day without this is lost protocol and rerank training data.**

Verification:
```bash
ls .mdemg/neural/training-data/          # rerank + protocol JSONL files appearing
mdemg tsdb backup list                    # backup schedule active
mdemg data status                         # collection rates across all sources
```

---

## 3. The Claude Code ↔ MDEMG Data Flow

Understanding what data flows during a Claude Code session is critical for training data completeness.

### 3.1 Session Lifecycle

```
SESSION START (session-start.sh)
  ├→ ingest-claude-md (background) — ingests CLAUDE.md, AGENT_HANDOFF.md,
  │     VISION.md, auto-memory, plans, rules, session memory
  │     └→ POST /v1/memory/ingest → embedding + edge creation
  │     └→ fires "ingest_complete" → RSIC ape.reflect LLM call [CAPTURED ✅]
  ├→ /v1/jiminy/warm → jiminy.synthesize LLM call [CAPTURED ✅]
  └→ bootstrap codification check → may fire RSIC meso cycle [CAPTURED ✅]

EVERY PROMPT (prompt-context.sh)
  ├→ GET /v1/jiminy/latest → reads cached guidance
  │     └→ prompt_augmentation text injected into Claude's context window
  │     └→ Claude reads constraints, acts based on guidance
  └→ POST /v1/jiminy/warm (fire-and-forget for NEXT prompt)
        └→ jiminy.synthesize LLM call [CAPTURED ✅]

EVERY TOOL CALL (post-tool-observe.py)
  ├→ builds action_summary from Write/Edit/Bash
  ├→ POST /v1/jiminy/evaluate → jiminy.evaluate LLM call [CAPTURED ✅]
  ├→ POST /v1/jiminy/feedback → outcome classifier LLM call [CAPTURED ✅]
  │     └→ NLI comprehension scoring
  │     └→ trust score update → TrustStore
  │     └→ protocol JSONL record [CAPTURED if enabled]
  └→ if .md file edited → ingest-claude-md --file [CAPTURED ✅]

PRE-COMPACT (context window compaction)
  └→ ingest-claude-md --force → re-ingests ALL tracked .md files [CAPTURED ✅]
```

### 3.2 What's Captured vs What's Missing

All 16 internal LLM calls are captured in `llm_interactions`. The guidance_id correlation (PR #219) enables joining "what guidance we synthesized" with "whether Claude followed it."

**Still missing (planned for Phase 4A):**
- Retrieval context (which nodes were retrieved, their scores, which was the oracle) — needed for RAFT training
- System prompt hash — needed for training data versioning when prompts change

---

## 4. Storage Architecture

### 4.1 Data Locations

| Data Type | Storage | Location |
|---|---|---|
| LLM interactions (all 16 tasks) | TimescaleDB | `llm_interactions` hypertable |
| Rerank training data | JSONL files | `.mdemg/neural/training-data/rerank-*.jsonl` |
| Protocol training data | JSONL files | `.mdemg/neural/training-data/protocol-*.jsonl` |
| RSIC persistence | Neo4j | `:RSICCycle`, `:RSICAction` nodes |
| Jiminy persistence | Neo4j | `:GuidanceOutcome` nodes |
| Curated datasets | JSONL files | `.mdemg/neural/datasets/v{N}/` |
| Anchor dataset | JSONL files | `.mdemg/neural/datasets/anchor/` |
| Model artifacts | LoRA adapters | `.mdemg/neural/models/ft/v{N}/` |
| Benchmark results | JSON files | `.mdemg/neural/benchmarks/` |

### 4.2 Storage Budget

| Data Type | Growth Rate | 6-Month Estimate | Location |
|---|---|---|---|
| LLM interactions (TSDB) | ~5-10 MB/day (compressed) | ~1-2 GB | Internal SSD (Docker volume) |
| Rerank + protocol JSONL | ~2-4 MB/day | ~500 MB - 1 GB | Internal SSD |
| Curated datasets | ~20-50 MB each | ~200-500 MB (10 versions) | Internal SSD |
| Model artifacts (per version) | ~200-400 MB LoRA, ~17 GB quantized | ~90 GB (5 deployed) | Internal + External SSD |
| **Total (internal SSD)** | | **~3-5 GB** | Within 2TB budget |

### 4.3 Backup Strategy

| Data | Backup Method | Status |
|---|---|---|
| TimescaleDB (llm_interactions + metrics) | `mdemg tsdb backup` (pg_dump + JSONL tar) | ✅ Built (PR #215) |
| JSONL training data | Included in TSDB backup tar | ✅ Built (PR #219) |
| Neo4j (RSIC + Jiminy + graph) | `mdemg backup` (Neo4j dump) | ✅ Built |
| Curated datasets | Immutable + versioned (no rotation) | Manual |
| Model artifacts | External SSD archive | Manual |

---

## 5. Data Governance

### 5.1 Data Classification

| Classification | Examples | Handling |
|---|---|---|
| **Training-safe** | System prompts, model responses, UATS specs, emergence names | Collect freely, include in training |
| **Scrub-required** | User prompts containing file paths, repo names, org-specific code | Scrubbed at write time by `scrubber.go` |
| **Exclude** | API keys, auth tokens, passwords, PII | Scrubbed to `[REDACTED_KEY]`, `[EMAIL]`, etc. |
| **Provenance-tagged** | All data | Must carry source tag (teacher/production/synthetic/hitl) |

### 5.2 Data Quality Gates

| Gate | Check | Action on Failure |
|---|---|---|
| **Format validity** | Response parses as expected format (JSON for structured tasks) | Exclude from training |
| **Non-empty** | Response length > 10 characters | Exclude |
| **Non-error** | `error` field is empty | Exclude |
| **Non-fallback** | `score_source != "nli_fallback"` for protocol records | Exclude from comprehension training |
| **Non-duplicate** | Prompt hash not already in dataset | Keep first occurrence only |
| **Latency reasonable** | `latency_ms < 60000` (not a timeout-retry artifact) | Exclude |
| **Model version known** | `model_name` is non-empty | Flag for review |
| **System prompt current** | `system_prompt_hash` matches current ULTS spec version | Exclude stale data |
| **ULTS output valid** | Response matches ULTS output_schema | Exclude malformed |

### 5.3 Exogenous Data Ratio (Anti-Collapse Protocol)

Every curated dataset must maintain α ≥ 0.4 — at minimum 40% of training data originates from outside the fine-tuned model. Tracked in the dataset manifest.

### 5.4 Temporal Split Protocol

Test data MUST come from a later time period than training data to prevent temporal leakage. Implementation in `dataset_versioner.py` uses timestamp-based splits, never random splits.

### 5.5 Prompt Deduplication

Hash all prompts (SHA-256 of system_prompt + user_prompt) to prevent train/test contamination. Keep first occurrence only.

### 5.6 RAFT Training Data Preparation (v3.0 Addition)

For tasks that receive retrieval context (consulting.classify, jiminy.synthesize, jiminy.evaluate, retrieval.rerank_cross), training examples should include the retrieval context:

- **80% of examples:** system_prompt + retrieved_context + user_prompt → response
- **20% of examples:** system_prompt + user_prompt → response (no retrieval context)

This teaches the model to work in MDEMG's actual operating mode (open-book with retrieved graph context) rather than learning to answer prompts in isolation (closed-book). UC Berkeley's RAFT research (COLM 2024) demonstrates this consistently outperforms both pure RAG and pure fine-tuning.

### 5.7 System Prompt Versioning (v3.0 Addition)

Each `InteractionRecord` includes a `system_prompt_hash` (SHA-256). During dataset curation:

- Group records by `system_prompt_hash`
- Use only the current (or N most recent) prompt versions
- Discard data from prompt versions that changed output format (breaking changes)
- Migrate data forward if the change is additive (new optional fields)

This prevents prompt evolution from poisoning training data with stale format expectations.

---

## 6. What Makes Training Data "High Value"

Not all collected data is equally useful. The highest-value examples are:

1. **Edge cases where the external LLM got it right despite difficulty** — teaches hard-input handling
2. **Examples where guidance was followed** (quality=1.0) — positive training signals for Jiminy tasks
3. **Examples where guidance was contradicted** (quality=0.0) with confirmation — negative training signals
4. **Examples with rich retrieval context** — the RAFT pattern, teaching retrieval-aware behavior
5. **Examples from diverse time periods** — prevents overfitting to a specific development phase

The lowest-value examples are:
- Duplicate prompts (same question asked multiple times)
- Error responses (LLM timed out, circuit breaker tripped)
- Degraded-model outputs (from a rejected model version)
- Examples where the system prompt changed and the output format no longer matches

---

## 7. Data Volume Estimates

Based on typical development velocity (~4-5 PR merges/day):

| Source | Records/Day | Records/Month | Storage/Month |
|---|---|---|---|
| LLM interactions (16 tasks) | ~50-100 | ~1,500-3,000 | ~5-10 MB (compressed) |
| Rerank JSONL | ~100-200 | ~3,000-6,000 | ~2-4 MB |
| Protocol JSONL | ~50-100 | ~1,500-3,000 | ~1-2 MB |
| **Total** | **~200-400** | **~6,000-12,000** | **~8-16 MB** |

At this rate, reaching the fine-tuning plan's target of 500 samples per task (8,000-16,000 total) will take approximately **2-3 months** of active development. Low-frequency tasks (metalearn.generalize, hidden.reclassify) will be the bottleneck — teacher distillation (Phase 4 of the impl plan) generates synthetic examples for these.

---

## 8. CLI Commands

### 8.1 Built (PR #219)

```bash
mdemg data status                     # Collection rates, storage, per-task counts
mdemg data inspect --task jiminy.evaluate --last 10   # View recent records
mdemg data stats                      # Per-task statistics
mdemg data annotate [--dry-run]       # Run quality annotation pipeline
mdemg data quality                    # Quality report
```

### 8.2 Planned (Not Yet Built)

```bash
mdemg data curate                     # Run full curation pipeline (filter → format → version)
mdemg data curate --dry-run           # Show what would be included/excluded
mdemg data anchor generate            # Generate anchor dataset via teacher distillation
mdemg data anchor verify              # Verify anchor dataset integrity
mdemg data quality entropy            # Entropy health check against baseline
mdemg data manifest --version v3      # Show dataset manifest with provenance
mdemg data export --version v3 --format huggingface   # Export for external tools
```

---

## 9. Implementation Priority

| Priority | Action | When | Effort | Status |
|---|---|---|---|---|
| **P0 (TODAY)** | Config flip (3 env vars) | Immediately | None | ⬜ USER ACTION |
| **P0 (DONE)** | LLM Interaction Logger | PR #217 | — | ✅ COMPLETE |
| **P0 (DONE)** | 16 consumer task labels | PR #218 | — | ✅ COMPLETE |
| **P0 (DONE)** | Privacy scrubber | PR #219 | — | ✅ COMPLETE |
| **P0 (DONE)** | guidance_id correlation | PR #219 | — | ✅ COMPLETE |
| **P0 (DONE)** | Quality annotation pipeline | PR #219 | — | ✅ COMPLETE |
| **P0 (DONE)** | Data monitoring CLI | PR #219 | — | ✅ COMPLETE |
| **P0 (DONE)** | JSONL backup integration | PR #219 | — | ✅ COMPLETE |
| **P1 (NEXT)** | SanitizeResponse / StripThinkBlock | Next sprint | S | ⬜ |
| **P1 (NEXT)** | RAFT retrieval context capture | Next sprint | M | ⬜ |
| **P1 (NEXT)** | ULTS spec framework (16 specs) | Next sprint | M | ⬜ |
| **P1 (NEXT)** | System prompt hash in InteractionRecord | Next sprint | S | ⬜ |
| **P2** | Teacher distillation | After 2-3 months data | M | ⬜ |
| **P2** | Dataset versioner | Before first training cycle | M | ⬜ |
| **P3** | Entropy monitor | Before second cycle | S | ⬜ |
