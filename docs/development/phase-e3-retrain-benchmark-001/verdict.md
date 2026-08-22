# PHASE-E3-RETRAIN-BENCHMARK-001 — Verdict

**Sprint**: PHASE-E3-RETRAIN-BENCHMARK-001 (Task #138)
**Date**: 2026-08-22
**Verdict**: 🔴 **FAIL** (per sprint plan §Epic 5 rubric: aggregate < 0.9188 − 0.010)

---

## Result

| Metric | E3 (iter 1200) | v1 baseline | Delta |
|---|---|---|---|
| **Aggregate weighted score** | **0.7658** | 0.9188 | **−0.1530 (−16.6%)** |
| Group T mean (weight 0.50) | 0.7356 | ~0.92+ | ≈ −0.19 |
| Group C mean (weight 0.35) | 0.8142 | ~0.94+ | ≈ −0.13 |
| Group J mean (weight 0.15) | 0.7539 | ~0.90+ | ≈ −0.15 |

Benchmark: `training_data/eval/valid_clean.jsonl` (290 rows, 13 matched tasks, 2 runs × 40-100 rows/task = **580 total calls**, 0 errors, 4 truncated rows all on hidden.reclassify at the 3000-token output cap).

Rubric mapping (sprint plan §Epic 5):
- Aggregate delta = −0.153, well below the −0.010 threshold → **FAIL**
- Per-task regression on TASK families > 5% → also **FAIL** on that clause

## Per-task table (sorted worst→best)

| Task | Group | n | E3 mean | E3 stddev | E3 min | Notes |
|---|---|---|---|---|---|---|
| **claude.code_knowledge** | — | 100 | **0.2517** | 0.107 | 0.074 | **DOMINANT DRIVER** — family stripped in E1/E2 |
| hidden.reclassify | T | 40 | 0.4000 | 0.490 | 0.000 | 4 truncated rows at 3000 tok output cap; high variance |
| retrieval.rerank_cross | T | 40 | 0.5617 | 0.390 | 0.000 | max_seq=4096 truncation cost (Phase 11.5d observed prompts up to 5,899 tok) |
| retrieval.query_classify | C | 40 | 0.6625 | 0.234 | 0.500 | |
| jiminy.evaluate_llm | J | 40 | 0.7500 | 0.159 | 0.633 | |
| jiminy.synthesize | J | 40 | 0.8573 | 0.051 | 0.767 | |
| consulting.classify | C | 40 | 0.8808 | 0.161 | 0.633 | |
| hidden.summarize | T | 40 | 0.9000 | 0.000 | 0.900 | pinned; likely OK |
| ape.reflect | T | 40 | 0.9333 | 0.061 | 0.583 | strong |
| hidden.name_emergence | C | 40 | 0.9500 | 0.000 | 0.950 | pinned; likely OK |
| jiminy.evaluate | C | 40 | 0.9667 | 0.000 | 0.967 | pinned; likely OK |
| jiminy.codegen | J | 40 | 0.9850 | 0.056 | 0.700 | strong |
| retrieval.intent_translate | C | 40 | 0.9900 | 0.039 | 0.800 | strong |

## Root-cause analysis

Two distinct causes, one architectural + one pragmatic — both pre-documented in the sprint plan:

### 1. Architectural — the strip hypothesis doesn't survive standalone benchmark (dominant)

The claude.code_knowledge collapse to 0.2517 (100 rows, tightest cluster of any task) is exactly what E1/E2's strip hypothesis PREDICTED for a standalone-model benchmark:

- **E1 audit hypothesis**: "The substrate can now retrieve these 2,203 Claude Code facts (81.4% coverage via `include_content=true` retrieval per INGEST-TOPOLOGY-REPAIR-001), so the LoRA doesn't need them baked in."
- **E2 strip execution**: dropped those 2,203 rows from the training corpus. E3 corpus went from 9,988 → 6,753 train rows.
- **E3 benchmark reality**: `valid_clean.jsonl` measures the LoRA **in isolation**. It doesn't consult the substrate. The stripped facts are no longer in the LoRA. Naturally the LoRA scores near-zero on the exact 100 fact-recall rows the E1 audit adjudicated as substrate-covered.
- **v1 baseline (0.9188) included all 2,706 v2 rows in training**, so it scores high on this task because it was over-fit on the facts — precisely the redundancy E1 flagged.

**This isn't a training failure — it's a measurement architecture mismatch.** The strip is CORRECT for the intended runtime path (Jiminy synthesis + guardrail + consulting all call retrieval before/alongside the LLM); it's WRONG for a standalone-LLM benchmark. But we don't have a substrate-aware benchmark; `valid_clean.jsonl` is what we have.

### 2. Pragmatic — reduced-scope training compounds the loss on T-group tasks

The T-group tasks (rerank_cross, reclassify, query_classify) took collateral hits from the reduced-scope config we adopted after the first-run wall-clock revelation:

- **`max_seq_length: 4096`** (from 8192): 3 sequences >4096 truncated (max 7,214 tokens observed). rerank_cross specifically observed 5,899-tok prompts in Phase 11.5d; these lost (input, teacher-response) alignment. Reflects in rerank_cross = 0.5617 vs baseline ~0.95+.
- **`batch_size: 2`** (from 4): halves batch size → higher gradient variance → less stable learning.
- **`iters: 3376` (1 epoch)** vs planned 3378 (2 epochs at batch=4): shallower training depth.

All three trade-offs were documented in `configs/sft_phase_e3_v1_base_v3.yaml` §"Reduced scope" comment and the sprint plan §Epic 3 addendum. The `hidden.reclassify` 0.400 with 4 truncated outputs shows the model was in unfamiliar territory on that task's output shape.

## Verdict → next step

Ship the FAIL verdict. **DO NOT promote iter 1200 as `mdemg-llm-v2` production adapter.** v1 remains production.

The strip hypothesis is not disproved — it's untested on the runtime path the strip was designed for. See §Follow-ups.

## Follow-ups (each disclosed; operator picks the next attempt)

### 🟡 PHASE-E3-RETRAIN-002 — Retrain without the E1/E2 strip (baseline sanity)

The single-most-informative next run: retrain against the SAME `.local-models/qwen3-14b-4bit-base` with the FULL 9,988-row v2 corpus (no strip), same reduced-scope config. If aggregate lands close to 0.9188, the reduced-scope config is neutral and the strip is the sole cause. If it lands substantially below 0.9188, the reduced-scope config itself is the issue and we need to spend the ~85h wall-clock at 2-epoch × batch=4 × max_seq=8192 to match baseline shape. Data-decidable in ~10-15h.

### 🟡 PHASE-E3-EVAL-SUBSTRATE-AWARE-001 — Build a substrate-aware benchmark harness

Extend `run_benchmark` (or ship a sibling `run_benchmark_substrate.py`) that hits MDEMG's `/v1/consulting/*` or `/v1/jiminy/*` endpoints for the ape.reflect / consulting.classify / jiminy.* tasks (which are the runtime path) instead of the raw LLM endpoint. Score against the same `valid_clean.jsonl` gold labels. E3 adapter's aggregate on the substrate-aware benchmark tests the actual strip hypothesis. If E3 aggregate ≥ baseline there, the strip is validated for the intended runtime; ship E3 as the production adapter.

### 🟢 PHASE-E3-RETRAIN-003 — Restore max_seq_length=8192 (spend the ~85h)

If (2) validates the strip hypothesis + (1) shows the reduced-scope config bit signal, run the full-scope training as originally spec'd. Only rational if we know the ceiling is worth the compute.

### 🟢 Cancel E3 line; move to another frontier

If E3-002 (baseline sanity) also underperforms, the corpus + training setup itself is suspect and we may need to reconsider the base (v2 27B via ADAPTER-SWAP-STANDARDIZE-001 + qwen3_5 arch check) or the corpus mix.

## What ships this sprint

- Adapter safetensors at `adapters/phase_e3_v1_base_v3/0001200_adapters.safetensors` (SHA `219c84ab73344975…`, 514 MB, val_loss 0.489, frozen at iter 1200)
- Benchmark result JSON at `training_data/eval/e3_benchmark.json` (580 rows, 0 errors)
- TSDB run row: `benchmark_runs` `run_id=bmkl2qrzxdvyodb2xkbt67nyv` (581 statements applied via `--apply-tsdb`)
- Verdict document (this file)
- Sprint post + CLAUDE.md pin candidates

**PHASE-E4-GATE-PROMOTE-001 REMAINS UNBLOCKED but E3 adapter does NOT feed it** — E4 will promote the next E3 attempt if one PASSES, or a v2-base adapter (task #91 → adapter arc) if that path opens.

## Documents Accessed

- `training_data/sft/e3_v1_base_v3/{train,valid,manifest}` (assembled corpus — Epic 1 output)
- `training_data/eval/valid_clean.jsonl` (290-row eval)
- `training_data/eval/e3_benchmark.json` (this sprint's output)
- `adapters/phase_e3_v1_base_v3/adapters.safetensors` (iter 1200 frozen)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,leak_audit,train.log,benchmark.log,train.aborted-8192-batch4.log}.{md,json,log}` (this sprint's artifacts)
- `configs/{sft_phase_e3_v1_base_v3,benchmark_phase10}.yaml`
- `docs/development/phase-e1-corpus-audit-001/` (E1 audit's strip-list + coverage argument — the hypothesis this benchmark stress-tests)
- `docs/development/phase-e2-corpus-curation-001/` (E2 strip execution)
- CLAUDE.md pins: FT-OAI-001 policy, Phase 11.5c/d eval choices, `mdemg-llm-v1` shipped state + 0.9188 baseline per APE-REFLECT-EVAL-REFRESH-001
- Live TSDB queries against `llm_interactions` + `benchmark_runs`
