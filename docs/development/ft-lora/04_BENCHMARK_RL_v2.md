# Phases 10-12: Automated Benchmarks + Reinforcement Learning

**Date:** 2026-04-07 (v4.0 — reward functions implemented, Jiminy benchmarks added)
**Extends:** Implementation Plan Phases 1-9
**Model:** Qwen3-30B-A3B MoE via vllm-mlx

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

GRPO with think mode generates 75% more tokens per completion (think blocks). Split into two runs:

```python
# No-think tasks: group_size=8 (cheap, fast — 7 tasks)
NOTHINK_TASKS = [
    "consulting.classify", "hidden.name_emergence", "hidden.reclassify",
    "jiminy.codegen", "retrieval.intent_translate",
    "retrieval.query_classify", "retrieval.rerank_nli",
]
# ~50 tok per completion × 8 groups = 400 tok per prompt

# Think tasks: group_size=4, think_budget=200 (capped — 9 tasks)
THINK_TASKS = [
    "ape.reflect", "consulting.synthesis", "hidden.summarize",
    "jiminy.evaluate", "jiminy.evaluate_llm", "jiminy.synthesize",
    "metalearn.generalize", "retrieval.rerank_cross", "summarize.generate",
]
# ~(200 think + 200 answer) × 4 groups = 1600 tok per prompt
```

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
