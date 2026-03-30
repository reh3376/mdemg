# Phases 10-12: Automated Benchmarks + Reinforcement Learning

**Date:** 2026-03-27 (v2.0)
**Extends:** Implementation Plan Phases 1-9
**Model:** Qwen3-30B-A3B MoE via vllm-mlx

---

## Training Stage Architecture

```
Stage 1: SFT (Supervised Fine-Tuning)          ← Phase 5
    │ Teacher-distilled data → LoRA adapters
    ▼
Stage 2: Automated Benchmarks                   ← Phase 10
    │ Per-task scoring, regression detection
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

### 10.1 Task Registry (All 15 Tasks)

```python
TASK_REGISTRY = {
    # === JSON-output tasks (9 tasks, all use SanitizeResponse) ===
    "rsic_reflection": {
        "type": "structured", "think": True,
        "metrics": {
            "json_valid_rate": {"weight": 0.2, "threshold": 0.95},
            "insight_relevance": {"weight": 0.3, "threshold": 0.7},
            "actionability_score": {"weight": 0.3, "threshold": 0.7},
            "severity_accuracy": {"weight": 0.2, "threshold": 0.8},
        },
    },
    "constraint_classification": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.3, "threshold": 0.85},
            "precision": {"weight": 0.25, "threshold": 0.80},
            "recall": {"weight": 0.25, "threshold": 0.80},
            "f1": {"weight": 0.2, "threshold": 0.80},
        },
    },
    "emergence_naming": {
        "type": "structured", "think": False,
        "metrics": {
            "name_quality": {"weight": 0.4, "threshold": 0.7},
            "type_accuracy": {"weight": 0.3, "threshold": 0.8},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
        },
    },
    "node_reclassification": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.5, "threshold": 0.85},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "confidence_calibration": {"weight": 0.2, "threshold": 0.7},
        },
    },
    "j9_evaluation": {
        "type": "detection", "think": True,
        "metrics": {
            "detection_rate": {"weight": 0.3, "threshold": 0.80},
            "false_positive_rate": {"weight": 0.3, "threshold": 0.15, "direction": "lower"},
            "severity_accuracy": {"weight": 0.2, "threshold": 0.75},
            "format_valid": {"weight": 0.2, "threshold": 0.95},
        },
    },
    "outcome_classification": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.4, "threshold": 0.80},
            "followed_precision": {"weight": 0.3, "threshold": 0.85},
            "contradicted_recall": {"weight": 0.3, "threshold": 0.75},
        },
    },
    "cross_space_generalization": {
        "type": "structured", "think": True,
        "metrics": {
            "applicability_accuracy": {"weight": 0.4, "threshold": 0.75},
            "abstraction_quality": {"weight": 0.3, "threshold": 0.7},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
        },
    },
    "query_classification": {
        "type": "classification", "think": False,
        "metrics": {
            "accuracy": {"weight": 0.5, "threshold": 0.85},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "type_coverage": {"weight": 0.2, "threshold": 0.8},
        },
    },
    "llm_reranking": {
        "type": "structured", "think": True,
        "metrics": {
            "ndcg_at_5": {"weight": 0.4, "threshold": 0.75},
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "rank_correlation": {"weight": 0.3, "threshold": 0.7},
        },
    },

    # === Free-form text output tasks (5 tasks) ===
    "memory_synthesis": {
        "type": "generation", "think": True,
        "metrics": {
            "coherence": {"weight": 0.3, "threshold": 0.7},
            "coverage": {"weight": 0.3, "threshold": 0.7},
            "conciseness": {"weight": 0.2, "threshold": 0.6},
            "format_valid": {"weight": 0.2, "threshold": 0.9},
        },
    },
    "cluster_summarization": {
        "type": "generation", "think": True,
        "metrics": {
            "name_quality": {"weight": 0.4, "threshold": 0.7},
            "summary_coverage": {"weight": 0.3, "threshold": 0.7},
            "abstraction_level": {"weight": 0.3, "threshold": 0.6},
        },
    },
    "guidance_synthesis": {
        "type": "generation", "think": True,
        "metrics": {
            "coherence": {"weight": 0.3, "threshold": 0.7},
            "coverage": {"weight": 0.3, "threshold": 0.7},
            "token_efficiency": {"weight": 0.2, "threshold": 0.6},
            "format_valid": {"weight": 0.2, "threshold": 0.9},
        },
    },
    "j17_codegen": {
        "type": "structured", "think": False,
        "metrics": {
            "format_valid": {"weight": 0.3, "threshold": 0.95},
            "uniqueness": {"weight": 0.3, "threshold": 0.90},
            "mnemonic_quality": {"weight": 0.4, "threshold": 0.7},
        },
    },
    "intent_translation": {
        "type": "generation", "think": False,
        "metrics": {
            "relevance": {"weight": 0.4, "threshold": 0.7},
            "conciseness": {"weight": 0.3, "threshold": 0.7},
            "query_quality": {"weight": 0.3, "threshold": 0.7},
        },
    },
    "code_summarization": {
        "type": "generation", "think": True,
        "metrics": {
            "accuracy": {"weight": 0.4, "threshold": 0.75},
            "conciseness": {"weight": 0.3, "threshold": 0.7},
            "format_valid": {"weight": 0.3, "threshold": 0.9},
        },
    },

    # === Meta-task (not one of the 15, for recursive improvement) ===
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

---

## Phase 11: Automated Reinforcement Learning

### 11.1 Method Selection

| Stage | Method | Data Format |
|---|---|---|
| Primary RL | **GRPO** | `{"prompt": "...", "answer": "..."}` |
| Preference alignment | **DPO** | `{"prompt": "...", "chosen": "...", "rejected": "..."}` |

Both supported natively by `mlx-lm-lora` on Apple Silicon.

### 11.2 Think-Mode GRPO Split (v2.0 Addition)

GRPO with think mode generates 75% more tokens per completion (think blocks). Split into two runs:

```python
# No-think tasks: group_size=8 (cheap, fast — 7 tasks)
NOTHINK_TASKS = [
    "constraint_classification", "emergence_naming", "node_reclassification",
    "outcome_classification", "j17_codegen", "intent_translation", "query_classification",
]
# ~50 tok per completion × 8 groups = 400 tok per prompt

# Think tasks: group_size=4, think_budget=200 (capped — 8 tasks)
THINK_TASKS = [
    "rsic_reflection", "memory_synthesis", "cluster_summarization",
    "j9_evaluation", "guidance_synthesis", "cross_space_generalization",
    "llm_reranking", "code_summarization",
]
# ~(200 think + 200 answer) × 4 groups = 1600 tok per prompt
```

### 11.3 MDEMG-Specific Reward Functions

New file: `neural/training/reward_functions.py`

```python
# Format rewards (deterministic)
def json_valid_reward(prompt, completion, meta) -> float
def format_compliance_reward(prompt, completion, meta) -> float

# Accuracy rewards (ground truth comparison)
def classification_accuracy_reward(prompt, completion, meta) -> float
def detection_precision_reward(prompt, completion, meta) -> float

# Quality rewards (MDEMG infrastructure)
def rsic_actionability_reward(prompt, completion, meta) -> float
def think_quality_reward(prompt, completion, meta) -> float

# Composite per-task registry
TASK_REWARDS = {
    "rsic_reflection": [(json_valid_reward, 0.2), (rsic_actionability_reward, 0.5), (think_quality_reward, 0.3)],
    "constraint_classification": [(json_valid_reward, 0.3), (classification_accuracy_reward, 0.5), (format_compliance_reward, 0.2)],
    # ... all 15 tasks ...
}
```

### 11.4 Reward Function Validation (v2.0 Addition)

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

Human review for quality dimensions automated rewards can't judge:
- RSIC reflection insight quality
- Synthesis narrative coherence
- Constraint detection edge cases
- Silent failure detection novelty
- Code mnemonic quality

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

## Complete Phase 10-12 File Inventory

### New Files (15)

| File | Phase | Purpose |
|---|---|---|
| `neural/benchmarks/task_benchmark.py` | 10 | Per-task benchmark runner |
| `neural/benchmarks/llm_judge.py` | 10 | LLM-as-judge |
| `neural/benchmarks/deterministic_evaluators.py` | 10 | Objective metrics |
| `neural/benchmarks/progress_tracker.py` | 10 | Timeline + stagnation |
| `neural/benchmarks/benchmark_scheduler.py` | 10 | Scheduling |
| `neural/training/reward_functions.py` | 11 | GRPO rewards |
| `neural/training/grpo_data_gen.py` | 11 | GRPO data |
| `neural/training/run_grpo.py` | 11 | GRPO training |
| `neural/training/dpo_data_gen.py` | 11 | DPO pairs |
| `neural/training/run_dpo.py` | 11 | DPO training |
| `neural/tests/test_reward_functions.py` | 11 | Reward validation |
| `neural/training/hitl_server.py` | 12 | Review web UI |
| `neural/training/hitl_queue_gen.py` | 12 | Queue population |
| `neural/training/hitl_to_dpo.py` | 12 | Decisions → DPO |
| `neural/training/run_online_dpo.py` | 12 | Online DPO |

### Modified Files (4)

| File | Phase | Change |
|---|---|---|
| `neural/training/cycle_runner.py` | 11 | Add GRPO + DPO stages |
| `internal/ape/self_reflect.go` | 10 | Patterns 23-24 (regression, stagnation) |
| `internal/metrics/collectors.go` | 10 | Benchmark metrics |
| `internal/cli/finetune.go` | 12 | HITL subcommands |

---

## Estimated Timeline

| Phase | Duration |
|---|---|
| Phases 1-6 (foundation + first cycle) | 6-8 weeks |
| Phase 10 (benchmarks) | 2 weeks (parallel with 7-9) |
| Phase 11 (automated RL) | 2 weeks |
| Phase 12 (HITL) | 2-3 weeks |
| **Total to full pipeline** | **12-16 weeks** |
