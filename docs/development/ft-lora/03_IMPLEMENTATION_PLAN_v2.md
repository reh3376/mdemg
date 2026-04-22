# MDEMG Fine-Tuned LLM: Implementation Plan

**Date:** 2026-04-21 (v5.0 — Qwen3.6-35B-A3B + two-tier MoE LoRA per memo 07 v3.1) | **Last verified:** 2026-04-21
**Model:** Qwen3.6-35B-A3B MoE via vllm-mlx (fallback Qwen3.5-35B-A3B)
**Scope:** 13 phases + Phase 5.X expert activation profiling, 16 LLM consumers, ~70 files

---

## Changes in v5.0 (per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 — 2026-04-21)

1. **Base model**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (see [`01_RESEARCH_v2.md §3`](01_RESEARCH_v2.md)).
2. **Phase 5 rewritten** (§Phase 5) — two-tier SFT (Tier 1 universal, then per-family Tier 2). Replaces the single-LoRA ✅ COMPLETE marker; the pipeline code still works, but the training loop needs the two-tier orchestration and must be re-run against Qwen3.6 during Sprint FT-LORA-C.
3. **New Phase 5.X — Expert Activation Profiling** — Sprint FT-LORA-D deliverable that validates or revises the family partition before Tier 2 training.
4. **⚠️ Two planner-introduced engineering policies** (Sprint FT-LORA-A addition, not in memo 07 — flagged for user sign-off via commit body + PR summary):
   - **Epoch cap + early-stop**: `val_loss > best_val_loss × 1.05` for 2 consecutive evals triggers early-stop. Max 3 epochs. Closes memo §6.1 open question.
   - **`n_epochs=auto` disallowed** on all LoRA runs. Explicit cap required.
   - **Forcing function:** FT-OAI-001 crossed the 1.05× threshold between step 1250 and step 1300 (val 0.684 → 0.792 = +16%), 2 evals past best. See `training_data/openai_ft/20260420/run_notes.md`.

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

**16 Consumer Task Labels (all WithContext wired; re-audited 2026-04-21 — canonical source is [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md)):**

| # | File | Task Label | Think Mode | Group (§5 family) |
|---|---|---|---|---|
| 1 | `ape/llm_reflector.go` | `ape.reflect` | ✅ | **T** (reasoning-think) |
| 2 | `consulting/llm_classifier.go` | `consulting.classify` | ❌ | **C** (classify-notink) |
| 3 | `consulting/synthesis.go` | `consulting.synthesis` | ✅ | **T** |
| 4 | `hidden/cluster_summarizer.go` | `hidden.summarize` | ✅ | **T** |
| 5 | `hidden/emergence_namer.go` | `hidden.name_emergence` | ❌ | **J** (structured-notink) |
| 6 | `hidden/reclassifier.go` | `hidden.reclassify` | ❌ | **C** |
| 7 | `api/server.go` (wraps `jiminy/evaluator.go`) | `jiminy.evaluate_llm` | ✅ | **J** |
| 8 | `jiminy/outcome_classifier.go` | `jiminy.evaluate` | ❌ | **C** |
| 9 | `jiminy/synthesizer.go` | `jiminy.synthesize` | ✅ | **T** |
| 10 | `api/server.go` (wraps `jiminy/codegen.go`) | `jiminy.codegen` | ❌ | **C** |
| 11 | `metalearn/generalizer.go` | `metalearn.generalize` | ✅ | **T** |
| 12 | `retrieval/intent_translator.go` | `retrieval.intent_translate` | ❌ | **C** |
| 13 | `retrieval/query_classifier.go` | `retrieval.query_classify` | ❌ | **C** |
| 14 | `retrieval/rerank.go` | `retrieval.rerank_cross` | ✅ | **J** |
| 15 | `retrieval/rerank.go` | `retrieval.rerank_nli` | ✅ | **T** |
| 16 | `summarize/service.go` | `summarize.generate` | ✅ | **T** |

Group totals: **T**=7, **C**=6, **J**=3. These drive the Sprint D family-partition validation — see [`01_RESEARCH_v2.md §5`](01_RESEARCH_v2.md).

**v5.0 drift correction vs v4.0:** v4.0 listed 17 rows because `jiminy/evaluator.go` appeared twice (once as `jiminy.evaluate` reasoning, once as `jiminy.evaluate_llm`). Re-audit confirms `evaluate.go` retains a `CallSite: "jiminy.evaluate"` string for the **embedding recorder** (no llmclient generative call); the llmclient generative call for `jiminy.evaluate` lives in `outcome_classifier.go`, and the `jiminy.evaluate_llm` + `jiminy.codegen` llmclient calls live in `api/server.go`. Table is now **16 rows = 16 distinct task labels** (no double-counting).

**Guardrail consumer (17th call site, not shown above):** `internal/guardrail/llm_evaluator.go` bypasses `llmclient` and is therefore absent from both this table and every training dataset. Migration to `llmclient` is queued for Sprint FT-LORA-B — see [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md) guardrail note.

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

Per the v5.0 re-audit (see [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md) and Phase 1 Group column above): **7 think tasks (Group T)** and **9 no-think tasks (6 Group C + 3 Group J)**. See consumer table above.

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

## Phase 5: Training Pipeline — Two-Tier SFT (Python + MLX) 🔄 REWORKED for v5.0 — **UNBLOCKED 2026-04-22**

**v4.0 status:** single-LoRA pipeline shipped ✅. **v5.0 status:** pipeline code largely reusable; the **training orchestration** is reworked into two sequential tiers (memo 07 v3.1 §3; see [`01_RESEARCH_v2.md §5`](01_RESEARCH_v2.md)). Sprint FT-LORA-E ✅ shipped the config/flag additions 2026-04-22 (see [`sprint_plan_ft_lora_e.md`](sprint_plan_ft_lora_e.md)); Sprint FT-LORA-C ✅ validated convergence on Qwen3.6 (all 3 gates green); **Sprint FT-LORA-DATA ✅ shipped the curated datasets 2026-04-22** (see [`sprint_plan_ft_lora_data.md`](sprint_plan_ft_lora_data.md)); pre-flight verdict **CLEAR** (see [`phase_5_dataset_preflight_post.md`](phase_5_dataset_preflight_post.md) with baseline [`phase_5_dataset_preflight.md`](phase_5_dataset_preflight.md)).

**Phase 5 pre-reqs complete.** Ready-to-invoke cheat-sheet (directory-based; mlx_lm 0.31.2 consumes a directory containing `train.jsonl` + `valid.jsonl` via `--data`, **not** a file via `--dataset`):

```bash
# Tier 1 — universal adapter (one run, all 16 tasks balanced; 3,500 rows = 3,150 train + 350 valid)
python -m neural.training.train_ft \
  --tier 1 --mode sft \
  --base-model <sprint-c-mxfp4-path> \
  --expected-sha256 cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734 \
  --data training_data/sft/tier1/ \
  --adapter-path adapters/tier1_attn_shared/ \
  --rank 32 --alpha 64 \
  --n-epochs 3 \
  --router-aux-loss-coef 0.002 \
  --early-stop-ratio 1.05 --early-stop-patience 2

# Tier 2 — per family (3 runs; reasoning-think shown; substitute family + profile + data dir + output)
python -m neural.training.train_ft \
  --tier 2 --family reasoning-think \
  --expert-selection-path training_data/routing_profiles/profile_routing_reasoning_think.json \
  --expected-sha256 cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734 \
  --base-adapter adapters/tier1_attn_shared/ \
  --base-model <sprint-c-mxfp4-path> \
  --data training_data/sft/family_reasoning_think/ \
  --adapter-path adapters/tier2_reasoning_think/ \
  --rank 8 --alpha 16 --n-epochs 3 \
  --router-aux-loss-coef 0.002 \
  --early-stop-ratio 1.05 --early-stop-patience 2

# Asymmetric quant classification (after all adapters merged)
python -m neural.training.quantize_asymmetric \
  --input-model adapters/merged_final/ \
  --output-model adapters/merged_final_mxfp4/ \
  --shared-bits bf16 --routed-spec mxfp4 --attn-bits bf16
```

**Directory-vs-file guidance (authoritative, supersedes Sprint E cheat-sheet):**
- mlx_lm `--data <dir>` points at a directory; loader expects `train.jsonl` + `valid.jsonl` siblings.
- Sprint FT-LORA-DATA produces the 4 directories consumed by Phase 5:
  - `training_data/sft/tier1/` — 3,150 train + 350 valid (all 16 tasks balanced)
  - `training_data/sft/family_reasoning_think/` — 1,530 train + 170 valid (7 T tasks; ape.reflect target=500)
  - `training_data/sft/family_classify_notink/` — 1,080 train + 120 valid (6 C tasks)
  - `training_data/sft/family_structured_notink/` — 540 train + 60 valid (3 J tasks)
- Each directory also contains a `manifest.json` pinning: `generator_sha`, `trained_against_model_sha` (Sprint C `cdc167566e…`), `raw_dataset_sha_pin` (`7caebf75fd59da37…`), per-task counts, source composition, duplication factors, seed, `file_sha256`, and `synthesis_version` (`v1-aaa646e`).

**Gating reminder:** Tier 2 cannot start until Tier 1 adapter exists (composition via `--base-adapter`). Phase 5 runbook owns sequencing + checkpoint-behavior empirical verification (Sprint E deferred this because it never launches training).

### 5.X-Data. Sprint FT-LORA-DATA — Dataset Curation ✅ EXECUTED 2026-04-22

**Scripts (all shipped this sprint):**
- `neural/training/recurate.py` — provenance-preserving re-curation with raw-SHA pin assertion.
- `neural/training/distill_driver.py` — mixed-teacher orchestrator (`gpt-5.4-mini` for 3 OpenAI-teacher tasks + Qwen3.6 MLX local for 2 MLX-teacher tasks); per-row structured logging with flush, endpoint pre-flight, MLX single-instance guard, debug log, HTTP retry policy (Epic 6.0 stabilization).
- `neural/training/balanced_sampler.py` — per-tier pre-processing sampler (upsample/downsample/passthrough, duplication ceiling = 5×, seed=42).
- `neural/training/stratified_split.py` — 90/10 stratified splitter + SHA256-stamped `manifest.json` writer.

**Tests (Epic 5, 3 tiers):**
- Unit: `neural/training/tests/{test_recurate.py, test_distill_driver.py, test_balanced_sampler.py, test_stratified_split.py}`
- Integration: `neural/training/tests/test_data_pipeline_integration.py`
- E2E: `scripts/sprint_ft_lora_data_e2e.sh`
- Live (gated): `neural/training/tests/test_distill_driver_live.py` (`MDEMG_LIVE_MLX=1`)

**Absent-task synthesis** (4 tasks had 0 rows in 21-day window):
- `consulting.synthesis`, `metalearn.generalize`, `hidden.summarize` → `gpt-5.4-mini` teacher (3 × 200 rows, ~$0.35–0.50 OpenAI spend; `metalearn.generalize` rows carry `weak_signal: True`).
- `retrieval.rerank_nli`, `summarize.generate` → Qwen3.6 MLX local teacher ($0, 2 × 200 rows).

**Pre-flight results:** baseline (`aaa646e`) = BLOCKED with 5 reasons; post-run verdict = **CLEAR** (see [`phase_5_dataset_preflight_post.md`](phase_5_dataset_preflight_post.md)).

**Highest duplication factor:** `jiminy.evaluate_llm` at 4.348× (45 real rows → 200-row floor), inside the 5× ceiling. Tier 1 `manifest.json` records this explicitly.

**Pinning:** raw dataset SHA `7caebf75fd59da37221acef887dc822ac9b80d04e19c19b750dd9a4e5eceb988`; model config SHA `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`; all 4 tools assert both on every invocation.

### 5A. Tier 1 — Universal LoRA (attention + shared expert, r=32, all 16 tasks balanced)

**Script:** `neural/training/train_ft.py` (existing; needs new flags per Sprint E)

**New flags (Sprint E adds):**
- `--tier=1` (default `monolithic` for v4.0 back-compat)
- `--target-modules=attention,shared_expert` (Tier 1 scope)
- `--rank=32 --alpha=64`
- `--router-aux-loss-coef=0.002` (memo §3.5; Sprint E exposes in `mlx-lm-lora`)
- `--n-epochs=<int>` (REQUIRED — no `auto`; see §5F below)
- `--early-stop-ratio=1.05 --early-stop-patience=2` (see §5F)

**Data:** balanced 16-task mix — each task weighted **equally** regardless of row count (see `05_DATA_COLLECTION_v2.md` balanced-sampling appendix).

**Output:** `tier1_universal.safetensors` (Tier 1 adapter).

### 5B. Phase 5.X — Expert Activation Profiling ⏩ Sprint FT-LORA-D ✅ EXECUTED 2026-04-22

**Script:** `neural/training/profile_expert_routing.py` ✅ (Sprint D shipped)

**Input:** Sprint C-validated Qwen3.6-35B-A3B-mxfp4 base model (SHA `cdc167566e54…`) + 320-prompt anchor set (20/task × 16 tasks) at `training_data/routing_profiles/anchor_prompts.jsonl`.

**Output per family (reasoning-think / classify-notink / structured-notink):**
- `training_data/routing_profiles/profile_routing_{family}.json` — top-25% routed experts per layer (64 of 256), activation counts, KL divergence of routing distribution vs uniform, per-layer breakdown
- `training_data/routing_profiles/raw_activation_counts.json` — per-(task, layer, expert) counts for post-hoc re-analysis
- `docs/development/ft-lora/sprint_c_d_profile_results.md` — decision doc (verdict + tables + Sprint E recommendation)

**Decision criteria (sprint_plan_ft_lora_d.md §5 Epic 3):**
- **Cross-family Jaccard overlap > 0.80** applied per-pair:
  - 0 pairs exceed → `3-family-confirmed` (proceed as planned)
  - 1 pair exceeds → `2-family-merged-<pair>` (merge, 2 Tier 2 adapters)
  - ≥ 2 pairs exceed → `1-family-collapsed` (single Tier 2 over union)
- **Per-family task-cohesion (within-family pairwise Jaccard of per-task top-25% sets):**
  - ≥ 0.70 → cohesive (no split)
  - 0.40-0.70 → ambiguous (report only)
  - < 0.40 → split-candidate (emit hierarchical cluster boundary; Sprint E decides)

Bimodality coefficient (BC = (skew²+1)/kurt) was considered and rejected as methodologically unsound for discrete 256-bin distributions with n=60-140 prompts; replaced with direct task-cohesion analysis using real per-task top-64 expert sets.

**Sprint E consumer:** `neural/training/train_ft.py --expert-selection-path=training_data/routing_profiles/profile_routing_{family}.json` — reads `per_layer[*].top_experts` to gate Tier 2 LoRA adapter instantiation.

### 5C. Tier 2 — Per-Family LoRA (top-25% routed experts, r=8, per family)

**Script:** `neural/training/train_ft.py` with new Tier 2 flags

**New flags (Sprint E adds):**
- `--tier=2`
- `--family={reasoning-think,classify-notink,structured-notink}`
- `--target-modules=routed_experts --expert-selection-path=profile_routing_{family}.json`
- `--rank=8 --alpha=16`
- `--base-adapter=tier1_universal.safetensors` (Tier 1 merged or stacked; Tier 2 trains on top)
- `--router-aux-loss-coef=0.002` (continue regularization)
- `--n-epochs=<int>` (REQUIRED) and `--early-stop-*` flags (same as Tier 1)

**Data:** per-family subset of the curated dataset (tasks filtered to the family's Group-column membership from §Phase 1 table).

**Output per family:** `tier2_{family}.safetensors` × 3 families.

### 5D. Asymmetric quantization and deployment

**Script:** `neural/training/quantize_deploy.py` (existing; needs Sprint E patch per memo §3.8)

**Sprint E patch:** `mlx_lm.convert` must accept per-module-class quantization selectors:
- Attention (q/k/v/o_proj): **BF16**
- Shared expert MLP: **BF16**
- Routed expert MLPs: **MXFP4_MOE** (4-bit MoE-aware)
- Router/gate weights: **BF16**

Existing pipeline (Tier 1 merge → quantize → deploy) extended to stack Tier 2 adapters post-quantization for inference.

### 5E. Evaluation + regression gate (existing pipeline, Qwen3.6-ready)

- `neural/training/evaluate_ft.py` ✅ COMPLETE (PR #247) — per-task eval runs after **each** tier (Tier 1 alone, Tier 1+Tier 2 per family, Tier 1+all Tier 2s)
- `neural/training/regression_gate.py` ✅ COMPLETE (PR #248) — no task regresses >5%, at least 2 improve ≥2% — applies at each tier boundary
- `neural/training/teacher_distill.py` ✅ COMPLETE (PR #249)
- `neural/training/reward_functions.py` ✅ COMPLETE (PR #249) — used in Phase 6 GRPO, not SFT

### 5F. ⚠️ Overfitting-prevention policy (Sprint A NEW — planner-introduced)

> **Policies 1 and 2 below are Sprint FT-LORA-A additions, not in memo 07 v3.1. Flagged for user sign-off via the Sprint A commit message body and PR summary. Forcing function: FT-OAI-001 (OpenAI FT) crossed the `val_loss > best × 1.05` threshold between step 1250 and 1300 (val 0.684 → 0.792 = +16%, 2 evals past best) — see `training_data/openai_ft/20260420/run_notes.md`.**

**Policy 1 — Epoch cap + early-stop threshold:**
- **Max 3 epochs** on every LoRA run (Tier 1 and each Tier 2).
- **Early-stop trigger:** `val_loss > best_val_loss × 1.05` for **2 consecutive evals**.
- **Rationale for 1.05× / 2-evals:** the 5% band tolerates expected noise; 2-eval patience avoids single-eval transient trips. FT-OAI-001 showed this threshold would have stopped training exactly at the onset of overfitting without false positives on the stable steps 500-1200. If Sprint C validation shows this is too tight (false positives on healthy training) or too loose (late stops), revise in a Sprint C addendum to this doc — not silently.

**Policy 2 — `n_epochs=auto` is disallowed.**
- Every LoRA run **must** specify an explicit `--n-epochs` integer value.
- **Rationale:** FT-OAI-001 was configured with `n_epochs=auto`, which OpenAI inflated to 3 epochs and the model overfit past step 1200. MLX `mlx-lm-lora` does not have an `auto` mode today, but any future equivalent (schedule-based or heuristic) must be rejected at the CLI layer.

**Enforcement:** Sprint E adds a CLI validator in `train_ft.py` that rejects `--n-epochs=auto` or a missing `--n-epochs` argument. Sprint C verifies both policies are active on the first Qwen3.6 training run.

**Open questions (memo §6.1):**
- Shared-expert epochs: default 3 (matches Tier 2). If Tier 1 Sprint C eval shows underconvergence, raise to 5 — but always subject to the early-stop trigger above.
- Per-family epoch differentiation: Tier 2 runs independently per family; same cap (3) each but early-stop evaluates per-family.

**Effort:** M — orchestration rework + CLI flag additions; evaluation/regression/quantize scripts reused.

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
