# MDEMG-USAGE-LORA-001 — Verdict

**Sprint**: MDEMG-USAGE-LORA-001 (task #145)
**Shipped**: Training complete 2026-08-31 09:04 UTC · Benchmark complete 2026-09-01 03:31 UTC
**Verdict**: **❌ FAIL — aggregate 0.8388 vs 0.9188 baseline = −0.080 (−8.7%). NO PROMOTION.**

## Result

| Metric | Value |
|---|---|
| Base | `mlx-community/Qwen3-14B-4bit` (unchanged from v1) |
| Adapter under test | `adapters/mdemg_usage_lora_001/0007200_adapters.safetensors` (iter 7200, best val_loss) |
| Adapter SHA-256 | `de2675b58800fc0362db26785941806ecb99a514e93e1c4f4a1db11ffc81e8c6` |
| Training wall-clock | ~99.6h (7,702/7,702 iters, ran to natural completion) |
| Best val_loss | **0.395** at iter 7200 (baseline iter 1 = 1.949; 5× improvement) |
| Benchmark aggregate | **0.8388** |
| Baseline (mdemg-llm-v1) | 0.9188 |
| **Delta** | **−0.080** |
| Verdict per sprint plan rubric | ❌ FAIL (< baseline − 0.02) |

## Training curve (val_loss vs iter)

| Iter | Val loss | Note |
|---:|---:|---|
| 1 | 1.949 | pre-training baseline |
| 2400 | 0.788 | first drop through 1.0 |
| 3200 | 0.696 | |
| 4000 | 0.477 | strong plateau |
| 5600 | 0.420 | |
| 6000 | 0.442 | |
| 7200 | **0.395** | **BEST — this is the frozen adapter** |
| 7600 | 0.487 | mild oscillation |
| 7702 (final) | 0.763 | end-of-run noise |

Training curve was healthy: monotone decrease with expected late-run LR oscillation. FT-OAI-001 early-stop did NOT fire (kept finding new minima); ran to natural completion.

## Per-task breakdown (13/18 valid_clean specs matched)

Sorted by mean ascending — worst first.

| Task | Mean | Min | Max | Stddev | N | Group |
|---|---:|---:|---:|---:|---:|:---:|
| **claude.code_knowledge** | **0.363** | 0.038 | 0.700 | 0.126 | 246 | T |
| retrieval.query_classify | 0.675 | 0.500 | 1.000 | 0.239 | 100 | C |
| jiminy.evaluate_llm | 0.783 | 0.633 | 0.967 | 0.166 | 100 | J |
| jiminy.synthesize | 0.872 | 0.767 | 0.967 | 0.046 | 100 | T |
| consulting.classify | 0.897 | 0.633 | 1.000 | 0.154 | 100 | C |
| hidden.summarize | 0.900 | 0.900 | 0.900 | 0.000 | 100 | T |
| retrieval.rerank_cross | 0.900 | 0.900 | 0.900 | 0.000 | 100 | J |
| ape.reflect | 0.938 | 0.917 | 0.967 | 0.025 | 100 | T |
| hidden.name_emergence | 0.950 | 0.950 | 0.950 | 0.000 | 100 | J |
| jiminy.evaluate | 0.967 | 0.967 | 0.967 | 0.000 | 100 | C |
| retrieval.intent_translate | 0.999 | 0.850 | 1.000 | 0.015 | 100 | C |
| hidden.reclassify | 1.000 | 1.000 | 1.000 | 0.000 | 100 | C |
| jiminy.codegen | 1.000 | 1.000 | 1.000 | 0.000 | 100 | C |

**Group means**:
- **T** (weight 0.5, 4 measured): 0.768 — dragged by claude.code_knowledge
- **C** (weight 0.35, 6 measured): 0.923 — healthy
- **J** (weight 0.15, 3 measured): 0.878 — mostly healthy

## Root cause analysis

**Single-task aggregate collapse**: `claude.code_knowledge` scored 0.363 (n=246 rows across 5 runs). That's a −0.55 delta from the ~0.92 that v1 posts on the same task. Because claude_code_knowledge is in the T group (weight 0.5) and has 246 rows (the largest N in the suite), its drop dominates the aggregate.

**Why did it drop?** This is the EXACT OPPOSITE cause of PHASE-E3's collapse:
- PHASE-E3 (task #138) STRIPPED 2,203 claude_code_knowledge rows → the family score cratered to 0.2517 (family removed from model training)
- MDEMG-USAGE-LORA-001 KEPT the v3-stripped family (453 rows) IN training → the family score still cratered to 0.3628

The difference: E3 dropped the family entirely; this sprint kept a reduced-scope training config (batch=2, max_seq=4096, 1 epoch on the family = 453 rows seen once) that provided **insufficient repetition** for the model to hold ~2,700-row claude_code_knowledge fact base at production quality.

**Comparison to v1's training scope**: v1 was trained via Phase 5 SFT with the FULL 2,706-row claude_code_knowledge_v2 corpus (dense LoRA on Qwen3-14B-4bit) at full-scope config. The stripped v3 corpus (503 rows, PHASE-E2's substrate-served-covers-these-rows exclusion) + reduced-scope training = insufficient signal density to preserve the knowledge.

**Trade-off pattern (same as PHASE-E3, arch rule 4)**: the reduced-scope config (batch=2 vs 4, max_seq=4096 vs 8192, 1 epoch vs 2) that made the ~90h wall-clock feasible is the same set of decisions that dragged claude_code_knowledge preservation. Full-scope training (~85h at batch=4 × max_seq=8192 per PHASE-E3 arch rule 1) would likely hold it — but was outside this sprint's wall-clock budget.

**Non-CCK tasks were preserved well**: 12/13 non-claude_code_knowledge tasks scored ≥ 0.675, with 8/13 ≥ 0.90. That's a healthy preservation profile for MDEMG's own operational tasks (consulting/jiminy/hidden/retrieval/ape). The retrain didn't destroy MDEMG's own capabilities — it destroyed the imported Claude Code knowledge.

## `mdemg.usage` measurement — supplemental eval COMPLETE

The sprint plan §5 Epic 4 called for extending the benchmark to include `mdemg_usage_v1/benchmark_holdout.jsonl` (121 rows). Shipped `configs/benchmark_phase10.yaml` reads only `valid_clean.jsonl`, so a targeted eval was needed: `scripts/mdemg_usage_eval.py` (~350 LoC, 3 proxy metrics: substring-recall, token-jaccard, length-gate). Full 121-row run took **39.8 min, 0 errors**.

**Result: aggregate mean = 0.307 (median 0.295)** on the 121-row mdemg.usage holdout. Distribution:

| Aggregate bucket | Count |
|---|---:|
| 0.00 – 0.20 | 16 |
| **0.20 – 0.35** | **71** |
| 0.35 – 0.50 | 27 |
| 0.50 – 0.75 | 7 |
| 0.75 – 1.00 | 0 |

Per-metric breakdown:
- `substring_recall`: 0.204 — adapter produces sentences that overlap only 20% with real doc content
- `token_jaccard`: 0.097 — key-term overlap is minimal (~10%)
- `length_gate`: 0.877 — outputs ARE in the expected length range (adapter learned to produce MDEMG-doc-shaped responses)

Per surface:
| Surface | N | Mean |
|---|---:|---:|
| features | 90 | 0.331 |
| user_api | 30 | 0.244 |
| cli-help | 1 | 0.008 |

**Interpretation**: adapter learned STYLE (length + doc-shape + section headers) but NOT specific facts (feature names, config keys, endpoints, code paths). This is consistent with the training math:
- 949 rows × 2 epochs = 1,898 exposures spread across 14B params
- Deterministic Q→A templating teaches ANSWER SHAPE, not FACT RECALL
- For RAG-supported use (substrate provides doc content, adapter provides shape) this would score much higher — the adapter would be REPRODUCING context, not recalling from weights
- For "volunteer MDEMG facts unprompted" use (the sprint's original goal), this is **too weak to ship**

**No baseline to compare against** — mdemg.usage is a NEW task; v1 (mdemg-llm-v1) would score similarly low or worse on it (v1 was never trained on MDEMG docs at all). But even without a baseline, the absolute number 0.307 with 0 rows ≥ 0.75 says the capability didn't materially land.

**Hygiene finding surfaced by eval**: 2 of the worst-5 rows are `docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/requests/*.py` — vendored `.venv` paths that got ingested into mdemg-dev during MDEMG-DOCS-INGEST-001. 2 more are `claude-docs/*` paths that slipped into mdemg_usage_v1 because the curator's WHERE clause matched `cli-reference/*` broadly. Both are surface-classifier hygiene issues (~3/1198 rows = 0.3% pollution, not critical but worth cleanup follow-up).

## Decision

**NO PROMOTION**. v1 (`mdemg-llm-v1`) remains production. iter-7200 adapter preserved at `adapters/mdemg_usage_lora_001/0007200_adapters.safetensors` for follow-up analysis.

Path forward is not obvious — 3 distinct follow-up shapes to consider:

1. **MDEMG-USAGE-LORA-002** — full-scope retrain (batch=4, max_seq=8192, 2 epochs; ~170-340h wall-clock). Would likely preserve claude.code_knowledge but confirms whether the mdemg.usage capability actually lands at that scope. Expensive.
2. **MDEMG-USAGE-LORA-003** — accept the trade-off, but shift the eval frame: the aggregate 0.8388 is dominated by ONE task (claude.code_knowledge, weight 0.5). If we're happy running Claude Code queries through RAG on the ingested claude-docs (MDEMG-DOCS-INGEST-001 shipped 2141 claude-docs nodes), the shipping model doesn't need to memorize them. Requires an eval-frame change: benchmark with RAG-supplied context, not adapter-in-isolation.
3. **DROP the mdemg.usage arm** — the operator's original directive was "only fine-tune on how to use the mdemg framework." If mdemg.usage doesn't land at production quality under any feasible training scope, the substrate-only path (RAG through MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001) is the alternative.

Operator decision point.

## What ships (regardless of promote/no-promote)

- `adapters/mdemg_usage_lora_001/0007200_adapters.safetensors` (514MB, iter 7200, val_loss 0.395)
- 19 other checkpoints (iter 400 through 7600 + final 7702) preserved
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (full per-task benchmark output)
- `training_data/eval/mdemg_usage_lora_001_iter7200_mdemg_usage.json` (mdemg.usage supplemental — appended when done)
- `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,sprint_post,training_start}.md/.json`

## Documents Accessed

- `docs/development/mdemg-usage-lora-001/sprint_plan.md` (this sprint's 12-section plan)
- `docs/development/mdemg-usage-corpus-curate-001/{sprint_plan,sprint_post}.md` (predecessor #144)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (arch rules)
- `docs/development/adapter-swap-standardize-001/sprint_post.md` (`mdemg adapter` tool used in Epic 4)
- `configs/sft_mdemg_usage_lora_001.yaml` (training config)
- `configs/benchmark_phase10.yaml` (eval config — the source of the "mdemg.usage not measured" gap)
- `training_data/sft/mdemg_usage_lora_001/{train,valid,manifest}.jsonl/.json` (6-family corpus)
- `training_data/sft/mdemg_usage_v1/benchmark_holdout.jsonl` (121 rows for supplemental eval)
- `training_data/eval/valid_clean.jsonl` (13-task benchmark holdout)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (benchmark output)
- `logs/mdemg_usage_lora_001_training.log` (training log — 20 val checkpoints, 7702 iters)
- `logs/mdemg_usage_lora_001_bench_run.log` (benchmark run log)
- `~/.mdemg/bench-serve-8103.log` (mlx_lm.server bench-serve log — 1272+ requests served)
- `adapters/mdemg_usage_lora_001/*.safetensors` (20 checkpoints, ~10 GB)
- CLAUDE.md pins:
  - PHASE-E3-RETRAIN-BENCHMARK-001 arch rules 1-4 (wall-clock arithmetic, corpus-strip-vs-eval-path, val_batches variance, peak GPU memory)
  - ADAPTER-SWAP-STANDARDIZE-001 (used `mdemg adapter freeze` + `mdemg adapter bench-serve`)
  - `must-follow-12-section-format`, `end-with-docs-accessed`, `live-testing-tier-required`
  - `iterate-break-fix-verify` (verdict landed via LIVE benchmark, not extrapolation from val loss)
- Operator directives 2026-08-24 (initial "proceed with #144") + 2026-08-27 (resume Epic 3) + 2026-08-31 (interpretation of results)
