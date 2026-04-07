# MDEMG Fine-Tuned LLM: Implementation Plan

**Date:** 2026-04-07 (v4.0 — reconciled through PR #243, 13 phases) | **Last verified:** 2026-04-07
**Model:** Qwen3-30B-A3B MoE via vllm-mlx
**Scope:** 13 phases, 16 LLM consumers, ~70 files

---

## Phase 1: LLM Interaction Logger ✅ COMPLETE (PRs #217-#219)

Implementation diverged from the v2.0 plan in a better direction:

| v2.0 Plan | v3.0 Reality (Built) |
|---|---|
| `LLMCompleter` interface | `InteractionRecorder` interface |
| `InteractionLogger` wrapper per consumer | `SetDefaultRecorder` pattern (zero consumer changes) |
| JSONL collector on disk | TimescaleDB `llm_interactions` hypertable (pgx CopyFrom) |
| Per-consumer `*Client` → `LLMCompleter` | `WithContext(taskName, spaceID)` per consumer |
| `scrubber.go` (Phase 1D) | Built in PR #219 (5 regex patterns, wired into writer) |

**Additional capabilities built beyond v2.0 plan:**
- `guidance_id` correlation via `context.WithValue` (PR #219)
- `source_path` linkage via `consulting/service.go` (PR #219)
- Think content extraction in `recordInteraction()` (PR #219)
- Quality annotation pipeline (`quality_annotator.py`, 468 lines) (PR #219)
- Quality report (`quality_report.py`, 244 lines) (PR #219)
- Data monitoring CLI: `mdemg data status/inspect/stats/annotate/quality` (PR #219)
- JSONL backup integration in TSDB backup tar (PR #219)
- Migration 005: `guidance_id` + `source_path` columns with partial indexes
- TSDB schema version 4 → 5

**16 Consumer Task Labels (all WithContext wired):**

| # | File | Task Label | Think Mode |
|---|---|---|---|
| 1 | `ape/llm_reflector.go` | `ape.reflect` | ✅ |
| 2 | `consulting/llm_classifier.go` | `consulting.classify` | ❌ |
| 3 | `consulting/synthesis.go` | `consulting.synthesis` | ✅ |
| 4 | `hidden/cluster_summarizer.go` | `hidden.summarize` | ✅ |
| 5 | `hidden/emergence_namer.go` | `hidden.name_emergence` | ❌ |
| 6 | `hidden/reclassifier.go` | `hidden.reclassify` | ❌ |
| 7 | `jiminy/evaluator.go` | `jiminy.evaluate` | ✅ |
| 8 | `jiminy/evaluator.go` (LLM tier) | `jiminy.evaluate_llm` | ✅ |
| 9 | `jiminy/outcome_classifier.go` | `jiminy.evaluate` (outcome) | ❌ |
| 10 | `jiminy/synthesizer.go` | `jiminy.synthesize` | ✅ |
| 11 | `jiminy/codegen.go` | `jiminy.codegen` | ❌ |
| 12 | `metalearn/generalizer.go` | `metalearn.generalize` | ✅ |
| 13 | `retrieval/intent_translator.go` | `retrieval.intent_translate` | ❌ |
| 14 | `retrieval/query_classifier.go` | `retrieval.query_classify` | ❌ |
| 15 | `retrieval/rerank.go` | `retrieval.rerank_cross` | ✅ |
| 16 | `retrieval/rerank.go` | `retrieval.rerank_nli` | ❌ |
| 17 | `summarize/service.go` | `summarize.generate` | ✅ |

**Note:** The Jiminy outcome classifier (`outcome_classifier.go:llmClassify`) uses the `jiminy.evaluate` task label for its LLM calls. This shares the label with the existing evaluator. Training data for both paths is currently mixed under the same task_name. Consider splitting to `jiminy.outcome_classify` for cleaner per-task training data if the two tasks show divergent quality requirements.

---

## Phase 2: Think Mode + Response Sanitization (Go) ✅ PARTIALLY COMPLETE (2D-2F done)

> **2D-2F (SanitizeResponse) COMPLETE:** `internal/llmclient/sanitize.go` with `StripThinkBlock`, `StripCodeFence`, `SanitizeResponse`. Wired into all 11 JSON-parsing call sites (10 files). System prompt hash added to InteractionRecord. See `docs/features/llm-response-sanitization.md`.
> **2A-2C (Think mode opt-in per consumer) NOT STARTED.**

### 2A. Modify: `internal/llmclient/client.go` — Add Think to CompleteOpts

```go
type CompleteOpts struct {
    MaxTokens   int
    Temperature *float64
    Format      json.RawMessage
    Options     map[string]any
    Think       bool            // NEW: enable /think mode for reasoning tasks
}
```

### 2B. Modify: `internal/llmclient/client.go` — Think Mode in OpenAI Provider

Since vllm-mlx is OpenAI-compatible, think mode is controlled via the system prompt or the vllm-mlx reasoning parser. The OpenAI provider prepends `/think` or `/no_think` to the first user message.

### 2C. Modify: All 16 Call Sites — Set Think Mode

7 no-think tasks (classification, codegen, query rewriting) and 9 think tasks (reflection, synthesis, evaluation). See consumer table above.

### 2D. New File: `internal/llmclient/sanitize.go` *(CRITICAL)*

**9 of 16 consumers call `json.Unmarshal` on the raw LLM response.** Qwen3's think mode returns `<think>reasoning</think>\n{"json": "response"}`. Without stripping the think block, all JSON-parsing consumers break.

```go
func SanitizeResponse(s string) string {
    s = StripThinkBlock(s)
    s = StripCodeFence(s)
    return strings.TrimSpace(s)
}

func StripThinkBlock(s string) string {
    startIdx := strings.Index(s, "<think>")
    if startIdx == -1 { return s }
    endIdx := strings.Index(s, "</think>")
    if endIdx == -1 { return strings.TrimSpace(s[:startIdx]) }
    return strings.TrimSpace(s[:startIdx] + s[endIdx+len("</think>"):])
}
```

### 2E. Modify: All 9 JSON-Parsing Consumers

Replace `StripCodeFence` with `SanitizeResponse` in every consumer that parses JSON.

### 2F. New: Format Retry Logic — `CompleteJSON()` helper

Wraps Complete with JSON validation and single retry on parse failure.

### 2G. New: System Prompt Compression Strategy

Progressive compression: full → compact → minimal, controlled by `LLM_PROMPT_MODE` config.

### 2H. New: System Prompt Hash (v3.0 Addition)

Add `SystemPromptHash` to `InteractionRecord` for training data versioning:

```go
// In recordInteraction():
rec.SystemPromptHash = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(system)))
```

Add column to `llm_interactions` via migration 006.

**Effort:** M — SanitizeResponse is critical path, rest is small additions.

---

## Phase 3: vllm-mlx Integration ✅ COMPLETE

Install vllm-mlx, create launchd/systemd service file, point `LLM_BASE_URL` to `http://localhost:8100/v1`. No Go or Python code changes needed — existing OpenAI provider works directly.

> **Delivered (PR #246):** Setup guide (`docs/development/ft-lora/vllm-mlx-setup.md`) + smoke test (`neural/training/test_vllm_mlx.py`) written and verified.

**Effort:** S — config + service file only.

---

## Phase 4: Teacher Distillation Pipeline (Python) 🔄 PARTIALLY COMPLETE [Verified: 2026-04-02]

### 4A. `neural/training/input_extractor.py` — Extract real task inputs from Neo4j ⬜ NOT STARTED
### 4B. `neural/training/teacher_distill.py` — Generate anchor dataset via external LLM ✅ COMPLETE (PR #249)
### 4C. `neural/training/synthetic_failures.py` — Generate failure detection examples ⬜ NOT STARTED
### 4D. `neural/training/quality_filter.py` ✅ COMPLETE (PR #240) — 8 quality gates, 25 tests, ULTS validation
### 4E. `neural/training/format_converter.py` ✅ COMPLETE (PR #240) — HuggingFace MLX chat format, RAFT 80/20 handling

**Effort:** S-M remaining (4A-4C only).

---

## Phase 4A: RAFT Retrieval Context Enrichment (v3.0 Addition) ✅ COMPLETE (2026-03-30)

> **All sub-phases complete:** RetrievalContext struct, WithRetrievalContext() context propagation, consulting + jiminy wiring, TSDB columns 22→26, migration 007, system prompt hash. See `docs/features/raft-retrieval-context.md`.

MDEMG operates in an open-book setting where every LLM call receives retrieved context from Neo4j. Training data must capture this context so the model learns to work with retrieval, not just answer prompts in isolation.

### 4A.1 Add RetrievalContext to InteractionRecord

**File:** `internal/llmclient/recorder.go`

```go
type RetrievalContext struct {
    RetrievedNodeIDs []string  `json:"retrieved_node_ids"`
    RetrievedScores  []float64 `json:"retrieved_scores"`
    OracleNodeID     string    `json:"oracle_node_id,omitempty"`
}

// Add to InteractionRecord:
RetrievalCtx *RetrievalContext
```

### 4A.2 Context key for retrieval metadata

**File:** `internal/llmclient/client.go`

```go
func WithRetrievalContext(ctx context.Context, rc *RetrievalContext) context.Context
```

Read in `recordInteraction()` the same way guidance_id and source_path are read.

### 4A.3 Wire into consulting/service.go

In `findApplicableConstraints()`, before calling classifier, set retrieval context on the Go context with the candidate nodes and scores.

### 4A.4 Wire into jiminy/service.go

In `Guide()`, before synthesis, set retrieval context with the constraint items that were retrieved.

### 4A.5 Migration 006: retrieval columns

```sql
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS retrieval_node_ids TEXT[];
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS retrieval_scores DOUBLE PRECISION[];
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS oracle_node_id TEXT;
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS system_prompt_hash TEXT;
```

### 4A.6 Training data preparation

When curating RAFT-style training data: 80% of examples include retrieved context in the user prompt, 20% omit it (force parametric recall). The model learns when to trust retrieved context vs. internalized knowledge.

**Effort:** M — touches recorder, writer, 2 service files, 1 migration.

---

## Phase 4B: ULTS Spec Framework (v3.0 Addition) ✅ COMPLETE (2026-03-30)

> **All sub-phases complete:** JSON Schema, 16 spec files, ults_runner.py, all 16 pass validation (100%). See `docs/features/ults-framework.md`.

Formalize all 16 LLM call contracts as machine-readable specs. Each spec defines: input_schema, output_schema, system_prompt_hash, latency_budget_ms, think_mode, quality_metrics, reward_functions, and training_config.

### 4B.1 Schema

**New file:** `docs/tests/ults/schema/ults.schema.json`

### 4B.2 Specs (16 files)

**New files:** `docs/tests/ults/specs/{task_name}.ults.json` — one per LLM consumer task.

Example (`consulting.classify`):
```json
{
  "task_name": "consulting.classify",
  "version": "1.0.0",
  "system_prompt_hash": "sha256:abc123...",
  "think_mode": false,
  "latency_budget_ms": 2000,
  "output_schema": {
    "type": "object",
    "required": ["is_constraint", "type", "confidence"],
    "properties": {
      "is_constraint": {"type": "boolean"},
      "type": {"type": "string", "enum": ["architectural", "process", "security", "none"]},
      "confidence": {"type": "number", "minimum": 0, "maximum": 1}
    }
  },
  "quality_metrics": {
    "accuracy": {"threshold": 0.85, "weight": 0.3},
    "format_valid_rate": {"threshold": 0.95, "weight": 0.25}
  },
  "training_config": { "rank": 16, "min_examples": 500 }
}
```

### 4B.3 Runner

**New file:** `docs/tests/ults/runners/ults_runner.py` — validates runtime LLM behavior against specs.

### 4B.4 Integration

- Dataset versioner reads ULTS specs to filter by system_prompt_hash and validate output format
- Phase 10 task registry becomes derivable from ULTS specs (single source of truth)
- Per-task LoRA rank configurable via `training_config.rank`

**Effort:** M — schema + 16 spec files + runner.

---

## Phase 5: Training Pipeline (Python + MLX) ✅ COMPLETE

### 5A. `neural/training/train_ft.py` — LoRA fine-tuning via `mlx-lm-lora` ✅ COMPLETE (PR #246)
### 5B. `neural/training/evaluate_ft.py` — Per-task evaluation against held-out test set ✅ COMPLETE (PR #247)
### 5C. `neural/training/regression_gate.py` — Version comparison (no task regresses >5%, at least 2 improve ≥2%) ✅ COMPLETE (PR #248)
### 5D. `neural/training/quantize_deploy.py` — Fuse LoRA adapter and quantize for production ✅ COMPLETE (PR #250)
### `neural/training/teacher_distill.py` — Teacher distillation for anchor dataset generation ✅ COMPLETE (PR #249)
### `neural/training/reward_functions.py` — Per-task reward functions for RLHF/DPO ✅ COMPLETE (PR #249)

**Effort:** M-L — 4 Python scripts with evaluation logic.

---

## Phase 6: Recursive Cycle Automation (Python) 🔄 PARTIALLY COMPLETE [Verified: 2026-04-02]

### 6A. `neural/training/cycle_runner.py` — Orchestrates complete cycle with anti-collapse protocol ⬜ NOT STARTED
### 6B. `neural/training/anchor_manager.py` — Manages anchor dataset ⬜ NOT STARTED
### 6C. `neural/training/entropy_monitor.py` — Tracks output entropy across versions ⬜ NOT STARTED
### 6D. `neural/training/dataset_versioner.py` ✅ COMPLETE (PR #240) — Temporal train/test/val split, cross-source dedup, SHA-256, manifest generation, 20 tests
### 6E. Dead-Man's Switch ⬜ NOT STARTED

**Effort:** M remaining (6A-6C, 6E).

---

## Phase 7: RSIC Integration (Go) 🔄 PARTIALLY COMPLETE [Verified: 2026-04-02]

> **Patterns 25-30 COMPLETE (PR #237):** 6 TSDB-aware reflection patterns (latency regression, error rate spike, retrieval quality degradation, embedding pipeline regression, training data readiness, trust trajectory decline). DatasetBuilder with 5 curated dataset queries. DatasetProvider interface for testing. 14 DatasetBuilder tests + 11 TSDB reflection tests. RSIC dashboard with Data-Driven Insights row.

Remaining: Training cycle trigger patterns (22-24), data balance checks, entropy decay, exogenous ratio enforcement, training cycle handler in task_dispatch.go.

**Effort:** S remaining.

---

## Phase 8: CLI Commands (Go) 🔄 MOSTLY COMPLETE [Verified: 2026-04-02]

> **Built:** `mdemg data status/inspect/stats/annotate/quality/audit/export/check` (PRs #219, #240, #243). Multi-table export with streaming privacy scan. Pre-campaign validation (`data check --pre-campaign`).

Remaining: `mdemg finetune` subcommands — status, train, eval, deploy, rollback (Phase 5 now COMPLETE). `mdemg data curate`, `mdemg data anchor generate`, `mdemg data manifest`.

**Effort:** S remaining (unblocked — Phase 5 complete).

---

## Phase 9: Monitoring (Go + Grafana) 🔄 PARTIALLY COMPLETE [Verified: 2026-04-02]

> **Built:** RSIC data-driven Grafana panels (PR #237), TSDB data quality diagnostic script (`scripts/tsdb_data_review.py`, 7 sections, PR #238), `mdemg data status` with `--warn` exit code (PR #219). J17 trust trend panels (PR #236).

Remaining: FT model-specific metrics (version, latency, cycles), data governance metrics (exogenous ratio, entropy), FT training dashboard.

**Effort:** S remaining.

---

## Phase 10: Data Collection Infrastructure ✅ COMPLETE [Verified: 2026-04-02]

> **All built in PRs #225-#242:**
> - Embedding event logging (`embedding_events` hypertable, PR #225)
> - Retrieval event logging (`retrieval_events` hypertable, PR #225)
> - Multi-table export pipeline with streaming privacy scan (PR #240)
> - UTDS spec framework (3 specs, schema, runner, PR #240)
> - TSDB data review diagnostic (7 sections, privacy verification, PR #238)

---

## Phase 11: Instance Isolation ✅ COMPLETE [Verified: 2026-04-02]

> **All built in PR #242:**
> - Migration 008: `instance_id TEXT NOT NULL DEFAULT ''` on all 3 training tables
> - Migration 009: Backfill empty `space_id` to `'mdemg-dev'`
> - Runtime `BackfillInstanceID()` at server startup (idempotent)
> - `defaultSpaceID` package-level fallback fixing 16 background LLM call sites
> - Neo4j memory tiering (4-tier auto-detection, platform-specific RAM detection)

---

## Phase 12: Campaign Hardening (PROD-READINESS Sprint) ✅ COMPLETE [Verified: 2026-04-02]

> **Built in PR #243:**
> - `session_id` propagation: context key, WithSessionID/SessionIDFromContext helpers, per-handler injection, defaultSessionID fallback
> - Query classifier wiring: 5 config vars, service field + setter, call site change to `ComputeRetrievalHintsWithLLM`
> - `mdemg data check --pre-campaign`: 8 automated checks with pass/fail + JSON output
> - Campaign task activation guide (`docs/operations/campaign-task-activation.md`)
> - FT implementation plan reconciliation (this update)

---

## TSDB Schema: 10 migrations (001-010)

- 001: Base metrics schema
- 002: FT schema (llm_interactions, retrieval_events)
- 003: Metric types
- 004: Aggregate policies
- 005: Interaction enrichment (guidance_id, source_path)
- 006: Embedding retrieval events
- 007: RAFT context (retrieval_node_ids, retrieval_scores, oracle_node_id, system_prompt_hash)
- 008: Instance ID
- 009: Backfill space_id
- 010: Fix schema version

---

## Complete File Inventory

### New Files (Phase 2+)

| File | Phase | Language | Purpose |
|---|---|---|---|
| `internal/llmclient/sanitize.go` | 2D | Go | StripThinkBlock + SanitizeResponse |
| `internal/llmclient/sanitize_test.go` | 2D | Go | Sanitize tests |
| `internal/cli/finetune.go` | 8 | Go | Fine-tuning CLI |
| `docs/tests/ults/schema/ults.schema.json` | 4B | JSON | ULTS schema |
| `docs/tests/ults/specs/*.ults.json` (×16) | 4B | JSON | Per-task LLM contracts |
| `docs/tests/ults/runners/ults_runner.py` | 4B | Python | ULTS validation runner |
| `neural/training/input_extractor.py` | 4A | Python | Extract inputs from Neo4j |
| `neural/training/teacher_distill.py` | 4B-old | Python | Anchor dataset generation |
| `neural/training/synthetic_failures.py` | 4C | Python | Failure detection examples |
| `neural/training/quality_filter.py` | 4D | Python | Quality gate filtering |
| `neural/training/format_converter.py` | 4E | Python | Interaction → MLX format |
| `neural/training/train_ft.py` | 5A | Python | LoRA fine-tuning |
| `neural/training/evaluate_ft.py` | 5B | Python | Per-task evaluation |
| `neural/training/regression_gate.py` | 5C | Python | Version comparison |
| `neural/training/quantize_deploy.py` | 5D | Python | Fuse + quantize |
| `neural/training/cycle_runner.py` | 6A | Python | Recursive cycle orchestrator |
| `neural/training/anchor_manager.py` | 6B | Python | Anchor dataset management |
| `neural/training/entropy_monitor.py` | 6C | Python | Anti-collapse monitoring |
| `neural/training/dataset_versioner.py` | 6D | Python | Dataset assembly + provenance |

### Already Built (Phase 1 — PRs #217-#219)

| File | PR | Purpose |
|---|---|---|
| `internal/llmclient/recorder.go` | #217 | InteractionRecorder interface + InteractionRecord struct |
| `internal/tsdb/llm_writer.go` | #217 | LLMInteractionWriter (pgx CopyFrom batch insert) |
| `internal/llmclient/scrubber.go` | #219 | Privacy scrubbing (5 regex patterns) |
| `internal/llmclient/scrubber_test.go` | #219 | 12 scrubber test functions |
| `internal/cli/data.go` | #219 | `mdemg data` CLI (status/inspect/stats/annotate/quality) |
| `neural/training/quality_annotator.py` | #219 | Post-hoc quality annotation from feedback outcomes |
| `neural/training/quality_report.py` | #219 | Data quality analysis + reporting |
| `internal/tsdb/migrations/005_interaction_enrichment.sql` | #219 | guidance_id + source_path columns |
| `internal/tsdb/migrations/006_embedding_retrieval_events.sql` | #225 | embedding_events + retrieval_events hypertables |
| `internal/tsdb/migrations/007_raft_context.sql` | #222 | RAFT context columns on llm_interactions |
| `internal/tsdb/migrations/008_instance_id.sql` | #242 | instance_id on all 3 training tables |
| `internal/tsdb/migrations/009_backfill_space_id.sql` | #242 | space_id backfill data fix |
| `internal/tsdb/migrations/010_fix_schema_version.sql` | #258 | Fix schema version |
| `internal/tsdb/embedding_writer.go` | #225 | Embedding event logger |
| `internal/tsdb/retrieval_writer.go` | #225 | Retrieval event logger |
| `internal/tsdb/backfill.go` | #242 | Runtime instance_id backfill |
| `internal/tsdb/dataset_builder.go` | #237 | RSIC-DATA curated dataset queries |
| `internal/cli/data_export.go` | #240 | Multi-table training data export |
| `internal/cli/data_check.go` | #243 | Pre-campaign validation checks |
| `docs/tests/utds/schema/utds.schema.json` | #240 | UTDS validation schema |
| `docs/tests/utds/runners/utds_runner.py` | #240 | UTDS export validation runner |
| `neural/training/quality_filter.py` | #240 | 8 quality gates for training data |
| `neural/training/format_converter.py` | #240 | HuggingFace MLX chat format converter |
| `neural/training/dataset_versioner.py` | #240 | Temporal split + dedup + manifest |
| `scripts/tsdb_data_review.py` | #238 | TSDB data quality diagnostic (7 sections) |
| `docs/operations/campaign-task-activation.md` | #243 | Campaign task activation guide |

### Additional Scripts (Built Post-v3.0)

| File | Purpose | Status |
|---|---|---|
| `neural/training/reward_functions.py` | 21 GRPO reward functions referenced by ULTS specs (json_valid, classification_accuracy, format_compliance, etc.) | COMPLETE |
| `neural/training/quality_report.py` | Training data readiness report — per-task row counts, quality coverage, quality_source breakdown | COMPLETE |

### Files Eliminated (vs v1.0)

| v1.0 File | Reason Eliminated |
|---|---|
| `neural/neural_sidecar/generator.py` | Replaced by vllm-mlx |
| `neural/neural_sidecar/schemas_generate.py` | Replaced by vllm-mlx |
| `internal/llmclient/mlx.go` | Use existing OpenAI provider to vllm-mlx |
| `internal/llmclient/interface.go` | Not needed — SetDefaultRecorder pattern avoids interface changes |
| `internal/llmclient/interaction_logger.go` | Not needed — recorder approach is simpler |
| `internal/llmclient/interaction_collector.go` | Replaced by TSDB writer |

---

## Implementation Schedule

| Phase | Dependencies | Effort | Status |
|---|---|---|---|
| **1** Interaction Logger | None | M | ✅ COMPLETE (PRs #217-#219) |
| **2** Think Mode + Sanitize | None | M | 🔄 PARTIAL (2D-2F done) |
| **3** vllm-mlx Integration | None | S | ✅ COMPLETE (PR #246) |
| **4A** RAFT Context | Phase 1 | M | ✅ COMPLETE (PR #222) |
| **4B** ULTS Specs | None | M | ✅ COMPLETE (PR #225) |
| **4** Teacher Distillation | 30-day campaign data | S-M | 🔄 PARTIAL (4D, 4E done) |
| **5** Training Pipeline | Phase 4 | M-L | ✅ COMPLETE (PRs #246-#250) |
| **6** Recursive Cycle | Phases 4+5 | M | 🔄 PARTIAL (6D done) |
| **7** RSIC Integration | Phases 1+6 | S-M | 🔄 PARTIAL (patterns 25-30 done) |
| **8** CLI Commands | Phases 5+6 | S | 🔄 MOSTLY COMPLETE |
| **9** Monitoring | Phases 3+7 | S | 🔄 PARTIAL |
| **10** Data Collection Infra | Phase 1 | M | ✅ COMPLETE (PRs #225-#240) |
| **11** Instance Isolation | Phase 10 | M | ✅ COMPLETE (PR #242) |
| **12** Campaign Hardening | Phase 11 | M | ✅ COMPLETE (PR #243) |

**Critical path:** 30-day campaign (task activation + data accumulation) → Phase 4A/4C (teacher distillation for rare tasks — 4B complete PR #249) → Phase 6A-C (recursive automation). Phase 5 (training pipeline) is COMPLETE.

**Next priorities:**
1. **30-day multi-instance collection campaign** — infrastructure ready, data activation needed
2. Phase 2A-2C (Think mode opt-in per consumer) — blocks local model switch
3. Phase 3 (vllm-mlx integration) — config-only, can be done anytime
3. Phase 4B (ULTS specs) — formalizes contracts, enables automation
