# Phases 10-12: Automated Benchmarks + Reinforcement Learning

**Date:** 2026-04-21 (v5.0 — three-group sampling recipes + router-entropy monitoring + val-loss early-stop per memo 07 v3.1)
**Extends:** Implementation Plan Phases 1-9
**Model:** Qwen3.6-35B-A3B MoE via vllm-mlx (fallback Qwen3.5-35B-A3B)

---

## Changes in v5.0 (per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 — 2026-04-21)

1. **Base model**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B**.
2. **NEW §10.0 — Three-Group Sampling Parameter Recipes** (memo §3.3) — all 16 tasks map to one of three groups. `presence_penalty=1.5` is mandatory on the 3 J-group tasks.
3. **§11.2 Think-Mode GRPO Split updated** — aligned to §1.1 Group column (7 think / 9 no-think), moving `retrieval.rerank_nli` to the think-task list to match the family partition.
4. **NEW §11.2.1 — Router-Entropy Monitoring** (memo §3.6) — MoE-specific GRPO safeguard; layer-level routing entropy must stay ≥ 1.5 nats; `router_aux_loss_coef=0.002` regularization during all RL runs.
5. **⚠️ NEW §11.6 — Val-Loss-Divergence Early-Stop for RL** (Sprint A planner-introduced policy — same rationale as SFT in `03_IMPLEMENTATION_PLAN_v2.md §Phase 5F`): `val_reward < best_val_reward × 0.95` for 2 consecutive evals triggers early-stop on GRPO/DPO runs. FT-OAI-001 overfitting at step 1200 is the forcing function for SFT; the mirror policy for RL prevents analogous reward-hacking regressions.

---

## Training Stage Architecture

```
Stage 1: SFT (Supervised Fine-Tuning)          ← Phase 5
    │ Teacher-distilled data → LoRA adapters
    ▼
Stage 2: Automated Benchmarks                   ← Phase 10
    │ Per-task scoring via ULTS specs, regression detection
    ▼
Stage 3: Automated RL (GRPO/DPO)               ← Phase 11
    │ MDEMG reward functions, split by think/no-think
    ▼
Stage 4: Human-in-the-Loop RL (DPO)            ← Phase 12
    │ Expert preference pairs for subjective quality
    ▼
Stage 5: Deploy or Reject                       ← Regression gate
```

---

## Phase 10: Automated Benchmark Framework

> **✅ EXECUTED — Sprint FT-LORA-PHASE10 (2026-04-23 → 2026-04-24).**
>
> MVP framework shipped. First authoritative baseline for `.local-models/qwen3-14b-mdemg-v1/` captured: **aggregate weighted score 0.8338** across **16 of 17 ULTS specs × 5 runs = 80 rows** (all `finish_reason=stop`, zero truncations). Per-group: T=0.8404 / C=0.8222 / J=0.8389.
>
> - **Post-run report**: [`phase_10_benchmark_post.md`](phase_10_benchmark_post.md)
> - **Sprint plan**: [`sprint_plan_ft_lora_phase10.md`](sprint_plan_ft_lora_phase10.md)
> - **Baseline artifact**: `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` (run_id `q283a23bz59mrg6faxo32ydx2`, config SHA `3716f9a4…`, golden SHA `8e44cdf9…`, file SHA `789459f1…`)
> - **SHA pins**: base model config `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5`
> - **New modules**: `neural/benchmarks/{run_benchmark,llm_judge,sampling_policy,variance,preflight}.py` + `judge_prompts/{coherence,depth,relevance,naturalness}.txt`
> - **Config**: `configs/benchmark_phase10.yaml` (zero hardcoded constants — N_runs, stagnation thresholds, group weights, judge kwargs, performance floors all declarative + CLI-overridable)
> - **TSDB V0012**: `internal/tsdb/migrations/012_benchmark_results.sql` (additive, hypertable on `recorded_at`; live migration deferred, baseline JSON persisted as durable sidecar)
> - **Scorer fixes** (`neural/training/reward_functions.py`): `classification_accuracy` + `evaluation_accuracy` each had a silent kwarg/shape mismatch that inflated/deflated scores. First baseline under buggy scorers: 0.7990; post-fix: 0.8338 (+0.0348, +4.4% relative). Epic-4 shadow-run confirmed registry path bit-compatible with legacy heuristic path within `|delta|<1%` on the Phase 5 dev set.
> - **Deferrals (Phase 10.5)**: (1) `guardrail.evaluate` 17th task — no golden rows + 2 unimplemented reward functions + no SFT training data (#216); (2) UBENCH promotion to formal UxTS framework (#215); (3) live TSDB migration when Docker restored; (4) Grafana panels for `benchmark_results` / `benchmark_runs`; (5) `neural/benchmarks/benchmark_scheduler.py` + launchd automation (Phase 11 operational scaffolding).
> - **Phase 11 GRPO unblocked.** Consumes `benchmark_results.reward_vector` per (task, run_idx) + `stddev` per task as advantage-normalization denominator.

### 10.0 Sampling Parameter Recipes (Three Groups — memo §3.3)

Every inference path (evaluation harness, benchmark runner, GRPO rollout, production vllm-mlx) **must** use the group recipe for the task's Group label from [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md). No per-task deviations in v5.0 — a task's group membership determines its sampling recipe.

| Group | Think | Sampling recipe | Task count |
|---|---|---|---|
| **T** — reasoning-think | `/think` | `temperature=0.6, top_p=0.95, top_k=20, min_p=0, max_tokens=4096` | 7 |
| **C** — classify-notink | `/no_think` | `temperature=0.3, top_p=0.95, top_k=20, max_tokens=64` | 6 |
| **J** — structured-notink | `/no_think` | `temperature=0.7, top_p=0.95, top_k=20, **presence_penalty=1.5**, max_tokens=2048` | 3 |

**`presence_penalty=1.5` is mandatory on all J-group tasks** (`hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`). Without it, no-think JSON generation on Qwen3.6 exhibits key-value repetition collapse on long outputs (empirical finding in memo §3.3). It is not applied to C-group (output too short for collapse to matter) or T-group (think-mode preamble prevents the degenerate state).

**Per-task mapping — all 16 tasks:**

| Task | Group | temp | top_p | top_k | min_p | presence_penalty | max_tokens |
|---|---|---|---|---|---|---|---|
| `ape.reflect` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `consulting.classify` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `consulting.synthesis` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `hidden.summarize` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `hidden.name_emergence` | **J** | 0.7 | 0.95 | 20 | — | **1.5** | 2048 |
| `hidden.reclassify` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `jiminy.evaluate_llm` | **J** | 0.7 | 0.95 | 20 | — | **1.5** | 2048 |
| `jiminy.evaluate` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `jiminy.synthesize` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `jiminy.codegen` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `metalearn.generalize` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `retrieval.intent_translate` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `retrieval.query_classify` | C | 0.3 | 0.95 | 20 | — | — | 64 |
| `retrieval.rerank_cross` | **J** | 0.7 | 0.95 | 20 | — | **1.5** | 2048 |
| `retrieval.rerank_nli` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |
| `summarize.generate` | T | 0.6 | 0.95 | 20 | 0 | — | 4096 |

**Gate criterion:** "all 16 tasks correctly assigned to one of the three groups" — NOT "all 16 tasks with unique per-task tuning". Per-task deviations require an explicit Sprint C+ memo update, not a silent config override.

**Enforcement (Sprint FT-LORA-B):** each task's ULTS spec (`docs/tests/ults/specs/<task>.ults.json`) must carry a `sampling_group` field and matching sampling recipe under `inference.sampling`. The 16 ULTS specs are audited/patched as part of Sprint B.

### 10.1 Task Registry (All 16 Tasks)

When ULTS specs exist (Phase 4B), this registry is generated from the ULTS spec files. ULTS is the single source of truth for task contracts, quality metrics, and reward function mappings. Until then, this Python dict serves as the reference:

```python
TASK_REGISTRY = {
    # === JSON-output tasks (9 tasks, all use SanitizeResponse) ===
    "ape.reflect": {
        "type": "structured", "think": True,
        "metrics": {
            "json_valid_rate": {"weight": 0.2, "threshold": 0.95},
            "insight_relevance": {"weight": 0.3, "threshold": 0.7},
            "actionability_score": {"weight": 0.3, "threshold": 0.7},
            "severity_accuracy": {"weight": 0.2, "threshold": 0.8},
        },
    },
    "consulting.classify": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.3, "threshold": 0.85},
            "precision": {"weight": 0.25, "threshold": 0.80},
            "recall": {"weight": 0.25, "threshold": 0.80},
            "f1": {"weight": 0.2, "threshold": 0.80},
        },
    },
    "hidden.name_emergence": {
        "type": "structured", "think": False,
        "metrics": {
            "name_quality": {"weight": 0.4, "threshold": 0.7},
            "type_accuracy": {"weight": 0.3, "threshold": 0.8},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
        },
    },
    "hidden.reclassify": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.5, "threshold": 0.85},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "confidence_calibration": {"weight": 0.2, "threshold": 0.7},
        },
    },
    "jiminy.evaluate": {
        "type": "detection", "think": True,
        "metrics": {
            "detection_rate": {"weight": 0.3, "threshold": 0.80},
            "false_positive_rate": {"weight": 0.3, "threshold": 0.15, "direction": "lower"},
            "severity_accuracy": {"weight": 0.2, "threshold": 0.75},
            "format_valid": {"weight": 0.2, "threshold": 0.95},
        },
    },
    "jiminy.evaluate_llm": {
        "type": "detection", "think": True,
        "metrics": {
            "revalidation_accuracy": {"weight": 0.4, "threshold": 0.80},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "false_positive_rate": {"weight": 0.3, "threshold": 0.15, "direction": "lower"},
        },
    },
    "metalearn.generalize": {
        "type": "structured", "think": True,
        "metrics": {
            "applicability_accuracy": {"weight": 0.4, "threshold": 0.75},
            "abstraction_quality": {"weight": 0.3, "threshold": 0.7},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
        },
    },
    "retrieval.query_classify": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.5, "threshold": 0.85},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "type_coverage": {"weight": 0.2, "threshold": 0.8},
        },
    },
    "retrieval.rerank_cross": {
        "type": "structured", "think": True,
        "metrics": {
            "ndcg_at_5": {"weight": 0.4, "threshold": 0.75},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "rank_correlation": {"weight": 0.3, "threshold": 0.7},
        },
    },

    # === Free-form / mixed output tasks (7 tasks) ===
    "consulting.synthesis": {
        "type": "generation", "think": True,
        "metrics": {
            "coherence": {"weight": 0.3, "threshold": 0.7},
            "coverage": {"weight": 0.3, "threshold": 0.7},
            "conciseness": {"weight": 0.2, "threshold": 0.6},
            "format_valid": {"weight": 0.2, "threshold": 0.9},
        },
    },
    "hidden.summarize": {
        "type": "generation", "think": True,
        "metrics": {
            "name_quality": {"weight": 0.4, "threshold": 0.7},
            "summary_coverage": {"weight": 0.3, "threshold": 0.7},
            "abstraction_level": {"weight": 0.3, "threshold": 0.6},
        },
    },
    "jiminy.synthesize": {
        "type": "generation", "think": True,
        "metrics": {
            "coherence": {"weight": 0.3, "threshold": 0.7},
            "coverage": {"weight": 0.3, "threshold": 0.7},
            "token_efficiency": {"weight": 0.2, "threshold": 0.6},
            "format_valid": {"weight": 0.2, "threshold": 0.9},
        },
    },
    "jiminy.codegen": {
        "type": "structured", "think": False,
        "metrics": {
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "uniqueness": {"weight": 0.3, "threshold": 0.90},
            "mnemonic_quality": {"weight": 0.4, "threshold": 0.7},
        },
    },
    "retrieval.intent_translate": {
        "type": "generation", "think": False,
        "metrics": {
            "relevance": {"weight": 0.4, "threshold": 0.7},
            "conciseness": {"weight": 0.3, "threshold": 0.7},
            "query_quality": {"weight": 0.3, "threshold": 0.7},
        },
    },
    "retrieval.rerank_nli": {
        "type": "structured", "think": False,
        "metrics": {
            "ndcg_at_5": {"weight": 0.4, "threshold": 0.70},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "rank_correlation": {"weight": 0.3, "threshold": 0.65},
        },
    },
    "summarize.generate": {
        "type": "generation", "think": True,
        "metrics": {
            "accuracy": {"weight": 0.4, "threshold": 0.75},
            "conciseness": {"weight": 0.3, "threshold": 0.7},
            "format_valid": {"weight": 0.3, "threshold": 0.9},
        },
    },

    # === Meta-task (not one of the 16, for recursive improvement) ===
    "system_integrity_check": {
        "type": "detection", "think": True,
        "metrics": {
            "detection_rate": {"weight": 0.4, "threshold": 0.70},
            "false_positive_rate": {"weight": 0.3, "threshold": 0.20, "direction": "lower"},
            "severity_accuracy": {"weight": 0.3, "threshold": 0.70},
        },
    },
}
```

### 10.2 New Files

| File | Purpose |
|---|---|
| `neural/benchmarks/task_benchmark.py` | Per-task benchmark runner |
| `neural/benchmarks/llm_judge.py` | LLM-as-judge for subjective metrics |
| `neural/benchmarks/deterministic_evaluators.py` | Objective metric evaluators |
| `neural/benchmarks/progress_tracker.py` | Timeline tracking + stagnation detection |
| `neural/benchmarks/benchmark_scheduler.py` | Cron/post-training/on-demand scheduling |

### 10.3 Scheduling

```bash
# Weekly (cron/launchd):
0 6 * * 0 cd /Users/reh3376/mdemg && python -m benchmarks.benchmark_scheduler
# Post-training (called by cycle_runner.py automatically)
# On-demand:
mdemg finetune eval
```

### 10.4 Jiminy-Specific Benchmarks

Post-v0.7.1, Jiminy effectiveness data provides additional benchmark dimensions:

- **Outcome accuracy:** Does `jiminy.evaluate_llm` classification match human review? (baseline: 70-80% agreement)
- **Tier effectiveness:** Does T1-encoded guidance achieve comparable follow rates to T3?
- **Content normalization impact:** Does the fine-tuned model handle both structured metadata and natural language equally well?

Data source: `scripts/jiminy_effectiveness_report.py --space-id <space> --days 7`

---

## Phase 11: Automated Reinforcement Learning

### 11.1 Method Selection

| Stage | Method | Data Format |
|---|---|---|
| Primary RL | **GRPO** | `{"prompt": "...", "answer": "..."}` |
| Preference alignment | **DPO** | `{"prompt": "...", "chosen": "...", "rejected": "..."}` |

Both supported natively by `mlx-lm-lora` on Apple Silicon.

MDEMG is well-suited for GRPO with verifiable rewards (RLVR) because many tasks have deterministic reward signals: JSON validity, classification accuracy, comprehension scores, guidance follow rates. This eliminates the need for a separate reward model — the ToolRL (ICLR 2026) approach of fine-grained reward design for tool use applies directly.

### 11.2 Think-Mode GRPO Split

GRPO with think mode generates 75% more tokens per completion (think blocks). Split into two runs, **aligned to the §1.1 Group column** (7 think / 9 no-think = 6 C + 3 J):

```python
# No-think tasks (9): Group C (classify, 6) + Group J (structured, 3)
# group_size=8 (cheap, fast); J-group applies presence_penalty=1.5 per §10.0
NOTHINK_TASKS = [
    # Group C — classify
    "consulting.classify", "hidden.reclassify",
    "jiminy.evaluate", "jiminy.codegen",
    "retrieval.intent_translate", "retrieval.query_classify",
    # Group J — structured JSON (presence_penalty=1.5 required)
    "hidden.name_emergence", "jiminy.evaluate_llm", "retrieval.rerank_cross",
]
# C tasks: ~50 tok/completion × 8 groups = 400 tok/prompt
# J tasks: ~500 tok/completion × 8 groups = 4000 tok/prompt

# Think tasks (7): Group T — reasoning-think
# group_size=4, think_budget=200 (capped)
THINK_TASKS = [
    "ape.reflect", "consulting.synthesis", "hidden.summarize",
    "jiminy.synthesize", "metalearn.generalize",
    "retrieval.rerank_nli",      # Group T per v5.0 re-audit (moved from no-think list)
    "summarize.generate",
]
# ~(200 think + 200 answer) × 4 groups = 1600 tok per prompt
```

**v5.0 delta from v4.0 list:** (1) `retrieval.rerank_nli` moved to `THINK_TASKS` to match Group T assignment in [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md). (2) `jiminy.evaluate` (outcome classifier) moved to `NOTHINK_TASKS` as Group C; `jiminy.evaluate_llm` stays in `NOTHINK_TASKS` but is now J-group (was in think tasks for v4.0). Sprint C validation verifies these reassignments hold under the Qwen3.6-adapted model.

### 11.2.1 Router-Entropy Monitoring (memo §3.6, MoE-specific GRPO safeguard)

Qwen3.6-35B-A3B's router selects 8 of 255 routed experts per token per layer. GRPO reward gradients applied to Tier 2 expert adapters can collapse routing (a few experts absorb all traffic), destroying the MoE's efficiency. Two safeguards:

1. **Continuous `router_aux_loss_coef = 0.002`** during all RL runs (matches SFT; memo §3.5). Sprint FT-LORA-E exposes the coefficient in `mlx-lm-lora`'s GRPO trainer.
2. **Layer-level entropy monitoring** logged per eval:
   - For each transformer layer, compute entropy of the routed-expert activation distribution across a 512-example eval batch: `H_layer = -Σ p_i log p_i` where `p_i` is the fraction of tokens routed to expert i.
   - **Gate:** `H_layer ≥ 1.5 nats` for every layer, every eval. Below 1.5 → **stop the run** (not a soft warning) and escalate to a revised `router_aux_loss_coef` (likely raise to 0.005).
   - Early-release risk: Qwen3.6 is 5 days old at sprint start; Sprint C may find the 1.5-nat gate is too tight or too loose on real routing. Revise in a Sprint C addendum, not silently.

**New metric:** `neural/training/reward_functions.py` gains no new reward function — this is a **health gate**, not a reward signal. Implementation lands in `neural/training/run_grpo.py` alongside existing gate checks.

### 11.3 MDEMG-Specific Reward Functions

**Implemented:** `neural/training/reward_functions.py` — 21 GRPO reward functions across 5 categories, registered in `REWARD_REGISTRY` with lookup via `get_reward_function(name)` and batch computation via `compute_reward(response, reward_names)`.

```python
# ── Structural (3) ──
json_valid          # 1.0 if valid JSON object
schema_match        # Partial credit: required keys + type checks
format_valid        # Non-empty, well-formed (JSON or text)

# ── Classification (2) ──
classification_accuracy   # Exact match against expected label
evaluation_accuracy       # Correct verdict for jiminy.evaluate tasks

# ── Quality (11) ──
coherence_score              # Sentence structure + length + repetition penalty
coverage_score               # Detail breadth via word count tiers
summary_quality              # (coherence + coverage) / 2
explanation_quality          # Reasoning depth for evaluation tasks
specificity_score            # Concrete > generic language
actionability_score          # Actionable recommendations
follow_rate                  # (specificity + actionability) / 2
insight_count                # Distinct points/bullets
uniqueness_check             # Lexical diversity ratio
naming_quality_score         # 2-5 word descriptive concept names
generalization_quality_score # (coherence + specificity + coverage) / 3

# ── Ranking (4) ──
ndcg_delta          # Ranking quality improvement vs reference
score_calibration   # Scores in [0, 1] range
score_correlation   # Proxy via calibration
recall_improvement  # Query expansion breadth

# ── Performance (1) ──
latency_reward      # Sigmoid decay vs latency budget
```

ULTS specs reference these by name in their `reward_functions` arrays. The `REWARD_REGISTRY` dict maps string names to callables, enabling data-driven reward composition per task without hardcoded mappings.

### 11.4 Reward Function Validation

New file: `neural/tests/test_reward_functions.py`

Unit tests for every reward function + calibration checks on 100 known-good/known-bad pairs to verify bimodal score distribution.

### 11.5 New Files

| File | Purpose |
|---|---|
| `neural/training/reward_functions.py` | GRPO reward functions |
| `neural/training/grpo_data_gen.py` | GRPO training data from interaction logs |
| `neural/training/run_grpo.py` | GRPO training orchestrator |
| `neural/training/dpo_data_gen.py` | DPO preference pair generation |
| `neural/training/run_dpo.py` | DPO training orchestrator |
| `neural/tests/test_reward_functions.py` | Reward function validation |

### 11.6 ⚠️ Val-Reward-Divergence Early-Stop for RL (Sprint A NEW — planner-introduced)

> **This policy is a Sprint FT-LORA-A addition, not in memo 07 v3.1. Flagged for user sign-off via the Sprint A commit body and PR summary. Mirror of the SFT early-stop policy in [`03_IMPLEMENTATION_PLAN_v2.md §Phase 5F`](03_IMPLEMENTATION_PLAN_v2.md) — the RL phase has its own overfitting/reward-hacking failure mode that warrants the equivalent safeguard.**

**Policy:**
- **Early-stop trigger:** `val_reward < best_val_reward × 0.95` for **2 consecutive evals** on any GRPO/DPO run.
- **Max epochs:** 3 per RL run (same cap as SFT).
- **Enforcement:** Sprint FT-LORA-E adds the check to `neural/training/run_grpo.py` and `neural/training/run_dpo.py` alongside the router-entropy gate (§11.2.1).

**Why 0.95× / 2-evals (same shape as SFT's 1.05× / 2-evals, inverted for reward-maximization):**
- Rewards are maximized (higher = better), so we stop when reward **drops** below best × 0.95. The 5% band tolerates expected reward noise; 2-eval patience avoids single-eval transients.
- No RL-specific historical analogue in MDEMG yet (Phase 11 hasn't been run). The policy is a prophylactic derived from the SFT overfitting incident (FT-OAI-001, step 1200). If Sprint C RL validation shows the 0.95× band is wrong (false positives on healthy training or late stops on reward-hacking regressions), revise explicitly in a Sprint C addendum.

**Interaction with router-entropy gate (§11.2.1):** both gates run every eval. If **either** trips, the run stops. Router-entropy trip → escalate aux coef; val-reward trip → stop and roll back to best checkpoint.

---

## Phase 12: Human-in-the-Loop RL

### 12.1 Review Dimensions

Human review for quality dimensions automated rewards can't judge: RSIC reflection insight quality, synthesis narrative coherence, constraint detection edge cases, silent failure detection novelty, code mnemonic quality.

### 12.2 New Files

| File | Purpose |
|---|---|
| `neural/training/hitl_server.py` | FastAPI web UI for preference review |
| `neural/training/hitl_queue_gen.py` | Populate review queue from model disagreements |
| `neural/training/hitl_to_dpo.py` | Convert review decisions → DPO format |
| `neural/training/run_online_dpo.py` | Online DPO via mlx-lm-lora `--judge human` |

### 12.3 CLI

```bash
mdemg finetune hitl serve       # Start review web UI (:8200)
mdemg finetune hitl queue       # Generate review queue
mdemg finetune hitl stats       # Review progress
mdemg finetune hitl train       # Run online DPO with human preferences
```

---

## Estimated Timeline

| Phase | Duration |
|---|---|
| Phases 1-6 (foundation + first cycle) | 6-8 weeks (Phase 1 DONE) |
| Phase 10 (benchmarks) | 2 weeks (parallel with 7-9) |
| Phase 11 (automated RL) | 2 weeks |
| Phase 12 (HITL) | 2-3 weeks |
| **Total to full pipeline** | **12-16 weeks** |
