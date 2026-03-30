# Corrections Applied: Deep-Dive Analysis Resolution

**Date:** 2026-03-27
**Source:** Deep-dive analysis conducted against v1.0 plans using web research, real benchmark data, and ICLR 2026 RSI Workshop findings.
**Status:** All corrections applied to v2.0 documents.

---

## Critical Issues (5)

### ISSUE 1: Model Choice — Qwen3-32B Dense Too Slow ✅ RESOLVED

**Problem:** Real benchmarks showed Qwen3-32B dense at 10-22 tok/s on Apple Silicon, not the estimated 24 tok/s. Qwen3-30B-A3B MoE runs at 64-88 tok/s at identical quality (82.20% MMLU-Pro CS).

**Resolution:** All documents updated to Qwen3-30B-A3B MoE. Apache 2.0 license preserved. Speed improvement: 4-5x. Inference memory impact: negligible (~20GB vs ~18GB at Q4).

**Documents updated:** 01_RESEARCH.md §3, 02_M5MAX_HARDWARE.md §1-2, 03_IMPLEMENTATION_PLAN.md (throughout), 04_BENCHMARK_RL.md (throughout).

### ISSUE 2: Model Collapse Risk ✅ RESOLVED

**Problem:** Nature (2024) and ICLR RSI Workshop (2026) prove recursive self-training causes model collapse when exogenous signal vanishes. The v1.0 plan mentioned "anchor data" but didn't formalize anti-collapse requirements.

**Resolution:** Anti-Collapse Protocol formalized:
- Minimum exogenous ratio α ≥ 0.4 per dataset (enforced in `dataset_versioner.py`)
- Entropy monitoring across versions (new `entropy_monitor.py`)
- Fresh exogenous injection every 3 cycles (in `cycle_runner.py`)
- Diversity sampling in GRPO: temperature ≥ 0.8, top_p = 0.95
- RSIC Pattern 27 (entropy decay alert) and Pattern 28 (exogenous ratio violation → block training)

**Documents updated:** 01_RESEARCH.md §2.2, 03_IMPLEMENTATION_PLAN.md Phase 6, 05_DATA_COLLECTION.md §5.3.

### ISSUE 3: Custom Inference Server Reinvents vllm-mlx ✅ RESOLVED

**Problem:** v1.0 Phase 3 built a custom `generator.py` + `schemas_generate.py` + `mlx.go` (3 files). vllm-mlx (EuroMLSys '26) already provides OpenAI-compatible API with prefix caching, continuous batching, and Qwen3 reasoning parser.

**Resolution:** Phase 3 reduced to config + service file. Three files eliminated. `llmclient`'s existing OpenAI provider connects to vllm-mlx at `http://localhost:8100/v1` — no new provider needed.

**Documents updated:** 02_M5MAX_HARDWARE.md §4, 03_IMPLEMENTATION_PLAN.md Phase 3.

### ISSUE 4: Test Data Contamination ✅ RESOLVED

**Problem:** Random train/valid/test splits allow temporal leakage and prompt duplication between splits.

**Resolution:**
- Temporal splits enforced (train before valid before test by timestamp)
- Prompt deduplication via SHA-256 hash
- Anchor dataset explicitly excluded from test
- Dataset manifests track all provenance

**Documents updated:** 05_DATA_COLLECTION.md §5.4, §5.5, §6.4.

### ISSUE 5: Think Mode GRPO Overhead ✅ RESOLVED

**Problem:** GRPO with think mode generates 75% more tokens per completion. For 9,400 prompts × group_size 4 × 3 epochs, that's ~38M extra tokens — ~132 hours on M5 Max.

**Resolution:** Split GRPO into no-think tasks (7 tasks, group_size=8) and think tasks (8 tasks, group_size=4, think_budget=200 tokens). Total GRPO token overhead reduced by ~60%.

**Documents updated:** 04_BENCHMARK_RL.md §11.2.

---

## Moderate Issues (4)

### ISSUE 6: Missing Inference Server Options Analysis ✅ RESOLVED

Documented vllm-mlx / oMLX / mlx_lm.server trade-offs. vllm-mlx selected for prefix caching benefit.

**Documents updated:** 02_M5MAX_HARDWARE.md §4.

### ISSUE 7: Training Data Not Versioned ✅ RESOLVED

Added `dataset_versioner.py` with manifests tracking provenance, source breakdown, exogenous ratio, temporal split boundaries, and quality statistics.

**Documents updated:** 05_DATA_COLLECTION.md §4, §6.4.

### ISSUE 8: Reward Function Validation Missing ✅ RESOLVED

Added `neural/tests/test_reward_functions.py` with unit tests + calibration checks for every reward function.

**Documents updated:** 04_BENCHMARK_RL.md §11.4.

### ISSUE 9: No Dead-Man's Switch ✅ RESOLVED

Added consecutive-rejection fallback: after 3 rejected cycles, switch to external LLM, regenerate training data from base model, and resume.

**Documents updated:** 03_IMPLEMENTATION_PLAN.md Phase 6E.

---

## Additional Issues Found in Final Review (4)

### ISSUE 10: Consumer Count Wrong (11 → 15) ✅ RESOLVED

**Problem:** Codebase audit found 4 additional LLM consumers missed in v1.0:
- `internal/retrieval/intent_translator.go` (query rewriting)
- `internal/retrieval/query_classifier.go` (query type classification)
- `internal/retrieval/rerank.go` (LLM-based reranking)
- `internal/summarize/service.go` (code element summarization)

**Resolution:** All documents updated to 15 consumers. Task registry, benchmarks, reward functions, and data collection all cover 15 tasks.

### ISSUE 11: Think Blocks Break JSON Parsing ✅ RESOLVED

**Problem:** 9 of 15 consumers call `json.Unmarshal` on raw LLM response. Qwen3's think mode returns `<think>...</think>\n{"json"}` — breaking all JSON parsers. The existing `StripCodeFence` only handles markdown fences.

**Resolution:** New `SanitizeResponse()` function in `internal/llmclient/sanitize.go` that calls `StripThinkBlock()` then `StripCodeFence()`. All 9 JSON-parsing consumers updated to use `SanitizeResponse` instead of `StripCodeFence`.

**Documents updated:** 01_RESEARCH.md §1.1 (JSON Parse column), 03_IMPLEMENTATION_PLAN.md Phase 2D-2E.

### ISSUE 12: No Format Retry Logic ✅ RESOLVED

**Problem:** Fine-tuned models produce malformed JSON more often than external LLMs. No consumer retries on parse failure.

**Resolution:** New `CompleteJSON()` helper in `llmclient` that validates JSON, and on failure retries once with a format correction prompt.

**Documents updated:** 03_IMPLEMENTATION_PLAN.md Phase 2F.

### ISSUE 13: System Prompts Waste Tokens After Fine-Tuning ✅ RESOLVED

**Problem:** 15 system prompts total ~200 lines explaining MDEMG's domain. After fine-tuning, the model already knows this — those tokens are wasted prefill.

**Resolution:** Progressive prompt compression strategy with three modes (full/compact/minimal) controlled by `LLM_PROMPT_MODE` config. Switch to "compact" after benchmarks prove ≥95% quality parity.

**Documents updated:** 03_IMPLEMENTATION_PLAN.md Phase 2G.

---

## Summary

| Severity | Found | Resolved | Open |
|---|---|---|---|
| Critical | 5 | 5 | 0 |
| Moderate | 4 | 4 | 0 |
| Additional | 4 | 4 | 0 |
| **Total** | **13** | **13** | **0** |

All v2.0 documents are internally consistent and cross-referenced.
