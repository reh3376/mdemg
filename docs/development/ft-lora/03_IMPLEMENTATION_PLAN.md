# MDEMG Fine-Tuned LLM: Implementation Plan

**Date:** 2026-03-30 (v3.0 — Phase 1 COMPLETE, new phases 4A/4B added, task names aligned to codebase)
**Model:** Qwen3-30B-A3B MoE via vllm-mlx
**Scope:** 11 phases (was 9), 16 LLM consumers (was 15), ~55 files

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

---

## Phase 2: Think Mode + Response Sanitization (Go) ⬜ NOT STARTED

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

## Phase 3: vllm-mlx Integration ⬜ CONFIG ONLY

Install vllm-mlx, create launchd/systemd service file, point `LLM_BASE_URL` to `http://localhost:8100/v1`. No Go or Python code changes needed — existing OpenAI provider works directly.

**Effort:** S — config + service file only.

---

## Phase 4: Teacher Distillation Pipeline (Python) ⬜ NOT STARTED

### 4A. `neural/training/input_extractor.py` — Extract real task inputs from Neo4j
### 4B. `neural/training/teacher_distill.py` — Generate anchor dataset via external LLM (~9,400 examples across 16 tasks)
### 4C. `neural/training/synthetic_failures.py` — Generate failure detection training examples
### 4D. `neural/training/quality_filter.py` — Filter raw data (errors, empties, duplicates, timeouts)
### 4E. `neural/training/format_converter.py` — Convert to MLX chat format with task-prefix system prompts

**Effort:** M — 5 Python scripts, mostly data processing.

---

## Phase 4A: RAFT Retrieval Context Enrichment (v3.0 Addition) ⬜ NOT STARTED

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

## Phase 4B: ULTS Spec Framework (v3.0 Addition) ⬜ NOT STARTED

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

## Phase 5: Training Pipeline (Python + MLX) ⬜ NOT STARTED

### 5A. `neural/training/train_ft.py` — LoRA fine-tuning via `mlx-lm-lora`
### 5B. `neural/training/evaluate_ft.py` — Per-task evaluation against held-out test set
### 5C. `neural/training/regression_gate.py` — Version comparison (no task regresses >5%, at least 2 improve ≥2%)
### 5D. `neural/training/quantize_deploy.py` — Fuse LoRA adapter and quantize for production

**Effort:** M-L — 4 Python scripts with evaluation logic.

---

## Phase 6: Recursive Cycle Automation (Python) ⬜ NOT STARTED

### 6A. `neural/training/cycle_runner.py` — Orchestrates complete cycle with anti-collapse protocol
### 6B. `neural/training/anchor_manager.py` — Manages anchor dataset (never deleted, included in every run)
### 6C. `neural/training/entropy_monitor.py` — Tracks output entropy across versions, alerts on >10% decay
### 6D. `neural/training/dataset_versioner.py` — Assembles datasets with temporal splits, dedup, α enforcement, ULTS-based filtering, provenance manifests
### 6E. Dead-Man's Switch — After 3 consecutive rejections, fall back to external LLM and retrain from base

**Effort:** M — orchestration + anti-collapse monitoring.

---

## Phase 7: RSIC Integration (Go) ⬜ NOT STARTED

New reflection patterns 22-28 (training cycle trigger, regression/stagnation alerts, data balance checks, entropy decay, exogenous ratio enforcement). Training cycle handler in task_dispatch.go.

**Effort:** S-M

---

## Phase 8: CLI Commands (Go) 🔄 PARTIAL

`mdemg data` subcommands built in PR #219: status, inspect, stats, annotate, quality.

Remaining: `mdemg finetune` subcommands — status, train, eval, deploy, rollback. Plus `mdemg data curate`, `mdemg data anchor generate`, `mdemg data manifest`.

**Effort:** S-M

---

## Phase 9: Monitoring (Go + Grafana) ⬜ NOT STARTED

FT model metrics (version, latency, cycles), data governance metrics (exogenous ratio, entropy), benchmark metrics. Two new Grafana dashboards.

**Effort:** S

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

| Phase | Dependencies | Effort | Duration | Status |
|---|---|---|---|---|
| **1** Interaction Logger | None | M | 1 week | ✅ COMPLETE |
| **2** Think Mode + Sanitize | None | M | 3-4 days | ⬜ NEXT |
| **3** vllm-mlx Integration | None (parallel) | S | 1-2 days | ⬜ |
| **4A** RAFT Context (v3.0) | Phase 1 | M | 3-4 days | ⬜ NEXT |
| **4B** ULTS Specs (v3.0) | None | M | 3-4 days | ⬜ NEXT |
| **4** Teacher Distillation | Phase 1 data (2-3 months) | M | 1-2 weeks | ⬜ |
| **5** Training Pipeline | Phase 4 | M-L | 1-2 weeks | ⬜ |
| **6** Recursive Cycle | Phases 4+5 | M | 1 week | ⬜ |
| **7** RSIC Integration | Phases 1+6 | S-M | 3-4 days | ⬜ |
| **8** CLI Commands | Phases 5+6 | S | 2-3 days | 🔄 Partial |
| **9** Monitoring | Phases 3+7 | S | 2-3 days | ⬜ |

**Critical path:** Phase 2 → Phase 4A → (data accumulation 2-3 months) → Phase 4 → Phase 5 → Phase 6

**Next sprint priorities:**
1. Phase 2 (SanitizeResponse) — blocks local model switch
2. Phase 4A (RAFT context) — enriches training data quality
3. Phase 4B (ULTS specs) — formalizes contracts, enables automation
