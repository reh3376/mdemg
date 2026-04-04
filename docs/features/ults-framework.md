---
created: 2026-03-30
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "FT-INFRA-C"
---

# ULTS Framework — Universal LLM Task Specification

## Summary

**Feature**: ULTS Framework
**Summary**: Universal LLM Task Specifications — machine-readable JSON contracts for all 16 LLM tasks, enabling automated quality benchmarking and regression gating.


## Overview

ULTS is the 13th UxTS framework type in MDEMG. It provides machine-readable contracts for all 16 LLM tasks, defining system prompt hashes, output schemas, quality metrics, think mode requirements, and training configuration. Each task gets a `.ults.json` spec file; the runner validates the full set for completeness, structural correctness, and (optionally) prompt hash integrity against Go source code.

ULTS was introduced in Phase C of the FT Infrastructure Sprint.

## Problem

16 LLM tasks have implicit contracts encoded in Go source code — system prompts, expected output formats, and quality thresholds. Without machine-readable specs:

- Training data cannot be automatically validated against expected output schemas
- Prompt version changes cannot be detected (stale training data goes unnoticed)
- Quality metrics are not standardized across tasks
- Benchmark automation requires manual task registry maintenance
- GRPO reward function mapping is ad hoc and undiscoverable

## Spec Structure

Each `.ults.json` file lives in `docs/tests/ults/specs/` and is validated against `docs/tests/ults/schema/ults.schema.json` (JSON Schema draft/2020-12).

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `ults_version` | string | Semver (currently `1.0.0`) |
| `task.name` | string | `domain.action` format matching `llmclient.WithContext` taskName |
| `task.description` | string | Human-readable description |
| `metadata.author` | string | Spec author |
| `metadata.created` | string | ISO date (`YYYY-MM-DD`) |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `task.version` | string | Semver of this task contract |
| `metadata.priority` | string | `critical`, `high`, `medium`, or `low` |
| `metadata.notes` | string | Free-form notes |
| `prompt.system_prompt_hash` | string | SHA-256 of the system prompt constant, or `"dynamic"` |
| `prompt.system_prompt_source` | string | `file:line` reference to Go source (e.g., `internal/ape/llm_reflector.go:62`) |
| `prompt.think_mode` | boolean | Whether this task benefits from chain-of-thought reasoning |
| `prompt.dynamic_prompt` | boolean | Whether the prompt is built at runtime with injected parameters |
| `performance.latency_budget_ms` | integer | Maximum acceptable LLM call latency in milliseconds |
| `performance.max_tokens` | integer | Token budget for this task |
| `output_schema` | object | JSON Schema for the expected LLM output format |
| `quality_metrics` | array | Array of `{name, weight, threshold, description}` — weights must sum to ~1.0 |
| `reward_functions` | array | Named reward function strings for GRPO training |
| `training_config.rank` | integer | LoRA rank for this task |
| `training_config.min_examples` | integer | Minimum training examples before fine-tuning is viable |
| `training_config.quality_gate` | number | Minimum average quality score required for training data (0.0–1.0) |

### Example: `jiminy.synthesize`

```json
{
  "ults_version": "1.0.0",
  "task": {
    "name": "jiminy.synthesize",
    "description": "Guidance synthesizer — transforms filtered guidance items into cohesive narrative for agent prompt augmentation",
    "version": "1.0.0"
  },
  "metadata": {
    "author": "reh3376",
    "created": "2026-03-30",
    "priority": "critical"
  },
  "prompt": {
    "system_prompt_hash": "8cf0ae2dbba97fcc627342e27653139b78bd1a04ce1a07556bb85c94a1937f39",
    "system_prompt_source": "internal/jiminy/guidance_prompt.go:11",
    "think_mode": true,
    "dynamic_prompt": false
  },
  "performance": { "latency_budget_ms": 10000, "max_tokens": 2000 },
  "output_schema": { "type": "string" },
  "quality_metrics": [
    { "name": "follow_rate",  "weight": 0.4, "threshold": 0.6 },
    { "name": "coherence",    "weight": 0.3, "threshold": 0.6 },
    { "name": "specificity",  "weight": 0.3, "threshold": 0.5 }
  ],
  "training_config": { "rank": 16, "min_examples": 500, "quality_gate": 0.7 }
}
```

## All 16 Task Specs

| Task | Think Mode | Hash Type | Key Quality Metrics | LoRA Rank |
|------|-----------|-----------|---------------------|-----------|
| `ape.reflect` | yes | static | json_valid, actionability, insight_quality | 16 |
| `consulting.classify` | no | static | json_valid, accuracy, precision | 8 |
| `consulting.synthesis` | yes | dynamic | coherence, coverage, actionability | 16 |
| `hidden.name_emergence` | no | static | json_valid, naming_quality | 8 |
| `hidden.reclassify` | no | dynamic | json_valid, accuracy | 8 |
| `hidden.summarize` | yes | static | coherence, coverage | 16 |
| `jiminy.codegen` | no | dynamic | json_valid, syntax_valid | 16 |
| `jiminy.evaluate` | yes | static | json_valid, comprehension_accuracy | 16 |
| `jiminy.evaluate_llm` | yes | static | json_valid, comprehension_accuracy | 16 |
| `jiminy.synthesize` | yes | static | follow_rate, coherence, specificity | 16 |
| `metalearn.generalize` | yes | static | json_valid, generalization_quality | 16 |
| `retrieval.intent_translate` | no | static | query_quality | 8 |
| `retrieval.query_classify` | no | static | json_valid, accuracy | 8 |
| `retrieval.rerank_cross` | yes | dynamic | json_valid, ndcg_improvement | 16 |
| `retrieval.rerank_nli` | no | dynamic | score_accuracy | 8 |
| `summarize.generate` | yes | static | coherence, coverage | 16 |

Hash type "static" means `system_prompt_hash` is a 64-character SHA-256 hex string. Hash type "dynamic" means the prompt is assembled at runtime and `system_prompt_hash` is set to `"dynamic"`.

## Runner

`docs/tests/ults/runners/ults_runner.py` validates the full set of specs. It shares the canonical reporting infrastructure via `docs/tests/uxts_report.py`.

### Validations performed

1. **Completeness check** — all 16 expected task names have a corresponding spec file
2. **Required fields** — `ults_version`, `task`, `metadata` (and their sub-fields) present
3. **Parity check** — no unknown top-level or prompt-section fields
4. **Version format** — `ults_version` matches semver pattern `^\d+\.\d+\.\d+$`
5. **Task name format** — matches `^[a-z]+\.[a-z_]+$` (`domain.action`)
6. **Prompt hash format** — 64-character lowercase hex or the string `"dynamic"`
7. **Quality metric weights** — sum within 0.01 of 1.0
8. **Hash verification** (optional, `--verify-hashes`) — SHA-256 of the backtick string constant at `system_prompt_source` must match `system_prompt_hash`

### Usage

```bash
# Validate all specs
cd docs/tests/ults && python3 runners/ults_runner.py --spec "specs/*.ults.json"

# Validate with prompt hash verification against source
python3 runners/ults_runner.py --spec "specs/*.ults.json" --verify-hashes --repo-root ../../../..

# Write JSON report
python3 runners/ults_runner.py --spec "specs/*.ults.json" --report report.json
```

The runner exits `0` if all checks pass, `1` otherwise.

## Integration Points

### Training data curation

`output_schema` provides a JSON Schema that training record extraction can validate against. Records whose LLM output does not match the schema are excluded from fine-tuning datasets automatically.

### Benchmark automation

`quality_metrics` and their `threshold` values are the source of truth for automated scoring thresholds in `docs/benchmarks/`. The benchmark runner derives per-task pass/fail criteria from ULTS specs rather than hardcoding them.

### Prompt versioning

`system_prompt_hash` detects when a system prompt changes between code commits. When the hash in the spec no longer matches the source constant, training records collected under the old prompt can be flagged and filtered. Run `--verify-hashes` in CI to catch stale hashes.

### GRPO training

`reward_functions` lists the named reward function implementations that the GRPO training loop uses for this task. This makes the mapping from task to reward discoverable at the spec level rather than buried in training code.

### Phase 10 task registry

ULTS specs are the authoritative task registry. The runtime task registry in Phase 10 is derived from these specs; any task added to Go source code requires a corresponding `.ults.json` spec before training pipelines will recognize it.

## File Locations

| Path | Purpose |
|------|---------|
| `docs/tests/ults/schema/ults.schema.json` | JSON Schema (draft/2020-12) for all `.ults.json` files |
| `docs/tests/ults/runners/ults_runner.py` | Validation runner |
| `docs/tests/ults/specs/*.ults.json` | 16 task spec files |

## Documents Accessed

- `docs/tests/ults/schema/ults.schema.json`
- `docs/tests/ults/runners/ults_runner.py`
- `docs/tests/ults/specs/jiminy_synthesize.ults.json`
- `docs/tests/ults/specs/retrieval_query_classify.ults.json`
- `docs/tests/ults/specs/ape_reflect.ults.json`
- `docs/development/ft-lora/04_BENCHMARK_RL_v2.md`
