# MDEMG Fine-Tuning Plan — Complete Document Suite

**Date:** 2026-04-07
**Version:** 4.0 (aligned to codebase state PRs #210-#219 + deep-dive strategic analysis)

---

## Document Map

Read in order. Each document builds on the previous.

| # | File | Purpose | Pages |
|---|---|---|---|
| 1 | `01_RESEARCH.md` | Strategic rationale — why fine-tune, what tasks, recursive loop architecture, RAFT pattern | ~18 |
| 2 | `02_M5MAX_HARDWARE.md` | Hardware-specific model selection, memory math, inference/training estimates | ~9 |
| 3 | `03_IMPLEMENTATION_PLAN.md` | The build plan — 11 phases, all files, code-level specs (Phase 1 COMPLETE) | ~25 |
| 4 | `04_BENCHMARK_RL.md` | Phases 10-12 — automated benchmarks, GRPO/DPO, human-in-the-loop | ~18 |
| 5 | `05_DATA_COLLECTION.md` | Training data collection, governance, storage, curation pipeline | ~18 |
| 6 | `06_CORRECTIONS_APPLIED.md` | All corrections from v1.0→v2.0→v3.0, consolidated with resolution status | ~8 |

---

## Key Decisions (v4.0)

| Decision | Rationale |
|---|---|
| **Model: Qwen3-30B-A3B MoE** | 4-5x faster than Qwen3-32B dense at identical quality (82.20% MMLU-Pro CS). Apache 2.0. ~80 tok/s on M5 Max vs ~15 tok/s for dense 32B. |
| **Inference: vllm-mlx** | Production-grade OpenAI-compatible server with prefix caching, continuous batching, reasoning parser. Eliminates 3 custom files. |
| **LLM consumers: 16** (not 11 or 15) | Codebase audit: rerank split into cross-encoder + NLI (2 tasks). jiminy.evaluate_llm is separate from jiminy.evaluate. |
| **Training: MLX bf16 LoRA** | M5 Max 128GB has no production traffic constraint. Full bf16 LoRA quality from day one. |
| **Anti-collapse: α ≥ 0.4** | Peer-reviewed research proves model collapse occurs when exogenous signal vanishes. Minimum 40% non-model-generated data per batch. |
| **Think block stripping** | 9 of 16 consumers parse JSON from LLM response. Think blocks break json.Unmarshal. SanitizeResponse() function required. |
| **Data storage: TimescaleDB** | LLM interactions stored in `llm_interactions` hypertable (not JSONL files). pgx CopyFrom batch inserts, 7-day chunking. |
| **RAFT training pattern** | MDEMG operates in open-book mode (retrieval context in prompts). Training data must include retrieval context for optimal quality. |
| **ULTS spec framework** | Formalize all 16 LLM call contracts as machine-readable specs for validation, curation, and benchmark automation. |
| **Routine retraining** | System prompts evolve, tasks are added, domains shift. Training infrastructure designed for monthly SFT refreshes, not one-time use. |
| **Embedding: separate workstream** | Embedding fine-tuning uses contrastive learning on encoder models (not LoRA). Target: 3072-dim vectors (Neo4j + OpenAI + Ollama standard). Data collection starts now; training later. |
| **No tool-use models** | All 16 tasks are text-in/JSON-out. Tool-use models emit tool-call structures that break json.Unmarshal. Target model must be base or instruct variant, not tool-use. |
| **Default LLM: gpt-4.1-nano** | gpt-5-nano (tool-use) breaks JSON tasks. Switched to gpt-4.1-nano (non-tool-use, 2x cheaper output, 1M context). LoRA target remains Qwen3-30B-A3B. |
| **Curated dataset pipeline** | export → UTDS validate → quality_filter → format_converter → dataset_versioner → train_ft. Validated E2E (10/10 PASS). |
| **Jiminy outcomes as quality signal** | GUIDANCE_OUTCOME edges (followed/partial/ignored/not_applicable) provide direct training quality labels for Jiminy tasks. |
| **Training data version boundary** | v0.7.1 classifier overhaul creates hard boundary. Pre-v0.7.1 Jiminy data is measurement error, not ground truth. Filter by MDEMG version >= v0.7.1. |

---

## Changes from v3.0

| Change | Affected Documents | Status |
|---|---|---|
| Tool-use model constraint added | 01, 02, 06 | ✅ Applied |
| Default LLM: gpt-5-nano → gpt-4.1-nano | 00, 01, 03, 06 | ✅ Applied |
| E2E curated pipeline documented | 05 | ✅ Applied |
| Jiminy outcome quality signals added | 05 | ✅ Applied |
| Training data version boundary documented | 05, 06 | ✅ Applied |
| reward_functions.py (21 functions), quality_report.py documented | 03 | ✅ Applied |
| TSDB migrations 006-010 documented | 03, 05 | ✅ Applied |
| Collection campaign status added | 05 | ✅ Applied |
| Outcome classifier shared task label noted | 03, 06 | ✅ Applied |

---

## Changes from v2.0

| Change | Affected Documents | Status |
|---|---|---|
| Consumer count: 15 → 16 | 01, 03, 04, 05 | ✅ Applied |
| Task names: snake_case → dot-notation (match actual WithContext labels) | 01, 03, 04, 05 | ✅ Applied |
| Phase 1 (Interaction Logger): marked COMPLETE, implementation notes added | 03, 05 | ✅ Applied |
| Data storage: JSONL → TimescaleDB (reflects PR #217 implementation) | 03, 05 | ✅ Applied |
| RAFT training pattern added (retrieval context in training data) | 01, 03, 05 | ✅ Applied |
| ULTS spec framework added (formalize LLM call contracts) | 01, 03, 04 | ✅ Applied |
| System prompt versioning (hash in InteractionRecord) | 03, 05 | ✅ Applied |
| Privacy scrubber: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Guidance ID correlation: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Quality annotation pipeline: marked COMPLETE (PR #219) | 05 | ✅ Applied |
| Data monitoring CLI: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Concurrent inference + training note added | 02 | ✅ Applied |
| Design for routine retraining (not one-time) | 01, 03 | ✅ Applied |
| v3.0 corrections documented | 06 | ✅ Applied |

---

## Implementation Status (as of 2026-03-30)

| Phase | Status | PRs |
|---|---|---|
| Phase 1: Interaction Logger | ✅ COMPLETE | #217, #218, #219 |
| Phase 2: Think Mode + SanitizeResponse | ⬜ NOT STARTED | — |
| Phase 3: vllm-mlx Integration | ⬜ Config only (not activated) | — |
| Phase 4: Teacher Distillation | ⬜ NOT STARTED (needs data accumulation) | — |
| Phase 4A: RAFT Retrieval Context (NEW) | ⬜ NOT STARTED | — |
| Phase 4B: ULTS Spec Framework (NEW) | ⬜ NOT STARTED | — |
| Phase 5: Training Pipeline | ⬜ NOT STARTED | — |
| Phase 6: Recursive Cycle Automation | ⬜ NOT STARTED | — |
| Phase 7: RSIC Integration | ⬜ NOT STARTED | — |
| Phase 8: CLI Commands | 🔄 Partial (`mdemg data` done, `mdemg finetune` not started) | #219 |
| Phase 9: Monitoring | ⬜ NOT STARTED | — |
| Phase 10: Benchmarks | ⬜ NOT STARTED | — |
| Phase 11: Automated RL (GRPO/DPO) | ⬜ NOT STARTED | — |
| Phase 12: Human-in-the-Loop | ⬜ NOT STARTED | — |

### Immediate Actions Required

1. **Config flip (P0, 5 min):** Set `NEURAL_DATA_COLLECTION=true`, `J17_PROTOCOL_DATA_COLLECTION=true`, `TSDB_BACKUP_ENABLED=true` and restart
2. **SanitizeResponse (P1):** Build `internal/llmclient/sanitize.go` — required before switching to any local model
3. **System prompt hash (P1):** Add to InteractionRecord for training data versioning
4. **RAFT context capture (P1):** Enrich InteractionRecord with retrieval context
5. **ULTS specs (P1):** 16 spec files formalizing LLM call contracts
