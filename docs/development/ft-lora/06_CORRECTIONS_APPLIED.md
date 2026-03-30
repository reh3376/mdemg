# Corrections Applied: Deep-Dive Analysis Resolution

**Date:** 2026-03-30
**Versions covered:** v1.0 → v2.0 → v3.0
**Status:** All corrections applied. 19 total issues, 19 resolved, 0 open.

---

## v1.0 → v2.0 Corrections (2026-03-27)

Source: Deep-dive analysis against v1.0 plans using web research, real benchmark data, and ICLR 2026 RSI Workshop findings.

### Critical Issues (5)

**ISSUE 1: Model Choice — Qwen3-32B Dense Too Slow ✅ RESOLVED**

Real benchmarks showed Qwen3-32B dense at 10-22 tok/s on Apple Silicon, not the estimated 24 tok/s. Qwen3-30B-A3B MoE runs at 64-88 tok/s at identical quality (82.20% MMLU-Pro CS). All documents updated to MoE.

**ISSUE 2: Model Collapse Risk ✅ RESOLVED**

Anti-Collapse Protocol formalized: α ≥ 0.4 exogenous ratio, entropy monitoring, fresh injection every 3 cycles, diversity sampling in GRPO. RSIC Patterns 27-28 added.

**ISSUE 3: Custom Inference Server Reinvents vllm-mlx ✅ RESOLVED**

Phase 3 reduced to config + service file. Three custom files eliminated. vllm-mlx provides OpenAI-compatible API with prefix caching, continuous batching, and Qwen3 reasoning parser.

**ISSUE 4: Test Data Contamination ✅ RESOLVED**

Temporal splits enforced, prompt deduplication via SHA-256, anchor dataset excluded from test, dataset manifests track provenance.

**ISSUE 5: Think Mode GRPO Overhead ✅ RESOLVED**

Split GRPO into no-think tasks (group_size=8) and think tasks (group_size=4, think_budget=200). Total overhead reduced ~60%.

### Moderate Issues (4)

**ISSUE 6: Missing Inference Server Options Analysis ✅ RESOLVED** — vllm-mlx selected.
**ISSUE 7: Training Data Not Versioned ✅ RESOLVED** — dataset_versioner.py with manifests.
**ISSUE 8: Reward Function Validation Missing ✅ RESOLVED** — test_reward_functions.py added.
**ISSUE 9: No Dead-Man's Switch ✅ RESOLVED** — 3 consecutive rejections → fallback to external LLM.

### Additional Issues (4)

**ISSUE 10: Consumer Count Wrong (11 → 15) ✅ RESOLVED** — 4 additional consumers found.
**ISSUE 11: Think Blocks Break JSON Parsing ✅ RESOLVED** — SanitizeResponse function specified.
**ISSUE 12: No Format Retry Logic ✅ RESOLVED** — CompleteJSON helper added.
**ISSUE 13: System Prompts Waste Tokens After Fine-Tuning ✅ RESOLVED** — Progressive compression strategy.

---

## v2.0 → v3.0 Corrections (2026-03-30)

Source: Deep-dive analysis against codebase state (PRs #210-#219) + 2026 LoRA/GRPO/RAFT research.

### ISSUE 14: Implementation Diverged from Plan Phase 1 ✅ RESOLVED

**Problem:** PRs #217-#219 implemented the interaction logger using a different (better) pattern than the plan specified:
- Plan: LLMCompleter interface + InteractionLogger wrapper + JSONL collector
- Reality: InteractionRecorder interface + SetDefaultRecorder pattern + TimescaleDB writer (pgx CopyFrom)

The SetDefaultRecorder pattern is superior — it requires zero changes to any LLM consumer. The TSDB storage is superior to JSONL — it supports SQL queries, joins, indexes, and the guidance_id correlation that JSONL cannot provide.

**Resolution:** All documents updated to reflect actual implementation. Phase 1 marked COMPLETE.

### ISSUE 15: Consumer Count 15 → 16 ✅ RESOLVED

**Problem:** v2.0 counted 15 consumers, but the actual codebase has 16:
- Rerank was split into `retrieval.rerank_cross` and `retrieval.rerank_nli` (2 separate WithContext labels)
- `jiminy.evaluate_llm` (LLM tier-2 revalidation in evaluator.go) is a separate consumer from `jiminy.evaluate` (outcome classification)

**Resolution:** All consumer tables, task registries, reward function mappings, and benchmark configs updated to 16 tasks.

### ISSUE 16: Task Names Mismatch ✅ RESOLVED

**Problem:** v2.0 plan used snake_case names (`rsic_reflection`, `constraint_classification`, etc.) but the implementation uses dot-notation (`ape.reflect`, `consulting.classify`, etc.). Every reference to task names in the plan was wrong.

**Resolution:** All task names updated to match actual `WithContext` labels used in the codebase. This affects tables in 01_RESEARCH, 03_IMPLEMENTATION_PLAN, 04_BENCHMARK_RL, and 05_DATA_COLLECTION.

### ISSUE 17: Missing RAFT Pattern ✅ RESOLVED

**Problem:** Training data captures LLM I/O but not retrieval context. The model trains in closed-book mode but operates in open-book mode (with retrieved graph context). UC Berkeley RAFT research (COLM 2024) proves this gap causes significant quality loss. The fine-tuned model would learn to answer prompts in isolation instead of learning to work with MDEMG's retrieval system.

**Resolution:** New Phase 4A (RAFT Training Data Enrichment) added. RetrievalContext struct added to InteractionRecord. Wire into consulting/service.go and jiminy/service.go. Migration 006 adds retrieval columns to llm_interactions. Training data curated with 80/20 oracle/distractor split.

### ISSUE 18: No LLM Call Contract Framework ✅ RESOLVED

**Problem:** 16 LLM tasks have implicit contracts (defined by system prompt constants) but no machine-readable specification for runtime validation, training data curation, or benchmark generation. The Phase 10 task registry was maintained separately from the contracts — dual sources of truth.

**Resolution:** New ULTS (Universal LLM Task Specification) framework added as Phase 4B. 16 spec files, JSON schema, Python runner. ULTS becomes the single source of truth for task contracts, quality metrics, and reward function mappings. Phase 10 task registry becomes derivable from ULTS specs.

### ISSUE 19: No System Prompt Versioning ✅ RESOLVED

**Problem:** MDEMG evolves rapidly (~4-5 PRs/day). When system prompts change — new output fields, refined formats, edge case handling — all training data generated under the old prompt becomes noise. No mechanism tracked which prompt version generated each training record.

**Resolution:** `system_prompt_hash` (SHA-256) added to InteractionRecord (Phase 2H). During dataset curation, records are filtered by prompt version via ULTS spec's `system_prompt_hash` field. Stale data from old prompt versions is excluded automatically.

---

## Summary

| Version | Severity | Found | Resolved | Open |
|---|---|---|---|---|
| v1.0 → v2.0 | Critical | 5 | 5 | 0 |
| v1.0 → v2.0 | Moderate | 4 | 4 | 0 |
| v1.0 → v2.0 | Additional | 4 | 4 | 0 |
| v2.0 → v3.0 | Strategic | 6 | 6 | 0 |
| **Total** | | **19** | **19** | **0** |

All v3.0 documents are internally consistent and cross-referenced. Task names, consumer counts, data storage locations, and implementation status reflect the actual codebase state as of PR #219.
