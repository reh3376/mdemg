# PHASE-E3-REBENCHMARK-001 — Sprint Post

**Task**: #149
**Completed**: 2026-09-02 (~15 min data-comparison; no new benchmark runs required)
**Verdict**: **E3 FAIL VERDICT STANDS with revised magnitude — same-runtime delta is −0.079 (not the reported −0.153; the difference was cross-runtime cost from #145's arch rule 5).**

Full analysis + per-task table at `verdict.md`. This post captures ship state + follow-ups.

## What shipped

- `docs/development/phase-e3-rebenchmark-001/{verdict,sprint_post}.md` — restated verdict + narrative correction
- CHANGELOG entry
- PR summary comment
- Task #149 → completed
- **Zero code changes.** No new benchmark runs (E3's shipped JSON + v1's #145 same-runtime baseline were both on mlx_lm.server → direct comparison legit).

## Verification (via data)

| Check | Result |
|---|---|
| E3 output on mlx_lm.server | ✅ verified (mlx_base_url = http://127.0.0.1:8103/v1, model_path = qwen3-14b-4bit-base + adapter) |
| v1 baseline on mlx_lm.server | ✅ verified (same URL, model_path = qwen3-14b-mdemg-v1 fused MLX) |
| Both matched 13/18 specs | ✅ same 13 valid_clean.jsonl tasks |
| E3 n_runs=2 vs v1 n_runs=5 | ⚠️ different variance but comparable means (E3 was earlier, before n_runs was tuned upward) |

## Key finding

**E3's reported gap was inflated 2× by the same silent cross-runtime bug that hit #145:**

| Component | Value |
|---|---:|
| Reported gap (E3 0.7658 vs 0.9188 baseline) | −0.1530 |
| Runtime cost (v1 llama.cpp → mlx_lm.server) | +0.0739 |
| **Real adapter-quality delta (same-runtime)** | **−0.0790** |

**E3 was still a REAL FAIL at −0.079** (much worse than #145's −0.006). The FAIL verdict is unchanged; only the magnitude and root-cause narrative shift.

## Root-cause narrative revision

E3's original sprint_post attributed the FAIL to:
1. Single-task claude.code_knowledge collapse from v3-stripped corpus
2. Reduced-scope config (batch=2, max_seq=4096)

**Revised** (same-runtime data now available):
1. **Real capability loss** — E3 destroyed 4 tasks by ≥0.11: hidden.reclassify (−0.54), retrieval.rerank_cross (−0.34), claude.code_knowledge (−0.135), retrieval.query_classify (−0.11)
2. **Reduced-scope config ALONE isn't the driver** — #145 used identical scope (batch=2, max_seq=4096) and hit parity. The material difference was **TRAINING DEPTH**: E3 was ~1 epoch (iter 1200 of 3376, early-stopped) vs #145's 2 epochs (iter 7200 of 7702, completed)
3. **CCK stripping is a contributor but not the biggest** — hidden.reclassify (−0.54) and retrieval.rerank_cross (−0.34) each moved the aggregate more
4. **Reported −0.153 was 2× inflated** by runtime cost

## Comparison to #145

| Sprint | Config | Training depth | Same-runtime delta | Verdict |
|---|---|---:|---:|---|
| E3 (#138) | batch=2, max_seq=4096 | ~1 epoch (iter 1200) | **−0.079** | ❌ FAIL |
| #145 mine | batch=2, max_seq=4096 | 2 epochs (iter 7200) | **−0.006** | ⚠️ PARITY |

**Training depth is the primary axis that separated E3 (FAIL) from #145 (parity)**, not config scope. This is a NEW finding this sprint enables — E3's original attribution to config-scope was wrong.

## Arch rule pinned (proposed for CLAUDE.md next PR)

**Training depth matters more than reduced-scope config for LoRA adapter capability preservation.** At batch=2 + max_seq=4096, **1 epoch** produces broad capability degradation (E3: hidden.reclassify −0.54, retrieval.rerank_cross −0.34) but **2 epochs** produces same-runtime parity (#145). Future LoRA arcs at reduced-scope config: budget for ≥2 epochs, NOT 1. If wall-clock forces 1 epoch, expect real capability loss on tasks with weaker signal density in the training corpus.

Extends PHASE-E3's original arch rules (wall-clock arithmetic, imported-domain preservation requires full-scope density) and #145's arch rule 5 (same-runtime comparison requirement). Together they form the "LoRA adapter shipping viability" cluster.

## Follow-ups filed

None. This sprint is a data-comparison book-close for E3's arc; E3's arc is already complete (task #138 completed 2026-08-22). This document serves as the historical revision note.

## Documents Accessed

- `training_data/eval/e3_benchmark.json` (E3 shipped benchmark output, run_id `bmkl2qrzxdvyodb2xkbt67nyv`)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (v1 same-runtime baseline from #145, run_id `xdmce6ya7om1dvuob4tnflbyu`)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (E3 shipped record)
- `docs/development/mdemg-usage-lora-001/verdict.md` (#145 revised verdict — source of cross-runtime bug diagnosis)
- `docs/development/mdemg-usage-lora-001-gguf/verdict.md` (#150 arc close)
- `docs/development/mdemg-usage-lora-001-q8/verdict.md` (#151 arc close)
- CLAUDE.md pins:
  - PHASE-E3 arch rule 5 (same-runtime comparison, from #145)
  - PHASE-E3 arch rules 1-4 (wall-clock, corpus-strip-vs-eval-path, val_batches variance, peak GPU memory)
  - `iterate-break-fix-verify`, `must-master-data-pipelines`
- Operator directive 2026-09-01 ("proceed with #149")
