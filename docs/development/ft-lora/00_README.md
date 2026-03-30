# MDEMG Fine-Tuning Plan — Complete Document Suite

**Date:** 2026-03-27
**Version:** 2.0 (corrections from deep-dive analysis applied to all documents)

---

## Document Map

Read in order. Each document builds on the previous.

| # | File | Purpose | Pages |
|---|---|---|---|
| 1 | `01_RESEARCH.md` | Strategic rationale — why fine-tune, what tasks, recursive loop architecture | ~15 |
| 2 | `02_M5MAX_HARDWARE.md` | Hardware-specific model selection, memory math, inference/training estimates | ~8 |
| 3 | `03_IMPLEMENTATION_PLAN.md` | The build plan — 9 phases, all files, code-level specs | ~20 |
| 4 | `04_BENCHMARK_RL.md` | Phases 10-12 — automated benchmarks, GRPO/DPO, human-in-the-loop | ~18 |
| 5 | `05_DATA_COLLECTION.md` | Training data collection, governance, storage, curation pipeline | ~15 |
| 6 | `06_CORRECTIONS_APPLIED.md` | All corrections from the deep-dive analysis, consolidated with resolution status | ~5 |

---

## Key Decisions (v2.0)

| Decision | Rationale |
|---|---|
| **Model: Qwen3-30B-A3B MoE** | 4-5x faster than Qwen3-32B dense at identical quality (82.20% MMLU-Pro CS). Apache 2.0. ~80 tok/s on M5 Max vs ~15 tok/s for dense 32B. |
| **Inference: vllm-mlx** | Production-grade OpenAI-compatible server with prefix caching, continuous batching, reasoning parser. Eliminates 3 custom files. |
| **LLM consumers: 15** (not 11) | Codebase audit found intent_translator, query_classifier, rerank LLM, and summarize service. |
| **Training: MLX bf16 LoRA** | M5 Max 128GB has no production traffic constraint. Full bf16 LoRA quality from day one. |
| **Anti-collapse: α ≥ 0.4** | Peer-reviewed research proves model collapse occurs when exogenous signal vanishes. Minimum 40% non-model-generated data per batch. |
| **Think block stripping** | 8 of 15 consumers parse JSON from LLM response. Think blocks break json.Unmarshal. New `SanitizeResponse()` function required. |

---

## Changes from v1.0

| Change | Affected Documents | Status |
|---|---|---|
| Model switch: Qwen3-32B → Qwen3-30B-A3B MoE | 01, 02, 03, 04 | ✅ Applied |
| Inference server: custom generator.py → vllm-mlx | 02, 03 | ✅ Applied |
| Consumer count: 11 → 15 | 01, 03, 05 | ✅ Applied |
| StripThinkBlock / SanitizeResponse | 03 (new Phase 2F) | ✅ Applied |
| Format retry logic | 03 (new Phase 2G) | ✅ Applied |
| System prompt compression | 03 (new Phase 2H) | ✅ Applied |
| Anti-collapse protocol formalized | 03 (Phase 6), 05 (§5.3) | ✅ Applied |
| Test data contamination fixes | 05 (§5.4, §5.5) | ✅ Applied |
| Data collection plan added | 05 (new document) | ✅ Applied |
| Think mode GRPO overhead | 04 (Phase 11) | ✅ Applied |
