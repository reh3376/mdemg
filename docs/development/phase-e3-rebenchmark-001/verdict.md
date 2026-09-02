# PHASE-E3-REBENCHMARK-001 — Verdict

**Sprint**: PHASE-E3-REBENCHMARK-001 (task #149)
**Shipped**: 2026-09-02 (data-comparison only; no new benchmark runs — E3's shipped output + v1's same-runtime baseline from #145 both use mlx_lm.server on port 8103)
**Verdict**: **❌ E3 FAIL VERDICT STANDS — but for a NUANCED reason.** The reported −0.153 gap was inflated by the same cross-runtime bug that hit #145 initially. Real same-runtime delta is **−0.079** (roughly half the reported gap). E3's adapter DID degrade quality on 4 tasks by 0.1+ vs v1, so the FAIL conclusion is correct — but the magnitude was overstated.

## The bug in E3's original verdict

Same pattern as MDEMG-USAGE-LORA-001 (#145) initial FAIL:

| | Setup | Runtime | Aggregate |
|---|---|---|---:|
| E3's "baseline" | v1 fused GGUF Q5_K_M (shipped) | **llama.cpp** | 0.9188 |
| E3's "candidate" | E3 adapter iter-1200 | **mlx_lm.server** | 0.7658 |
| E3's reported delta | | **CROSS-RUNTIME** | −0.153 |

**Runtime cost decomposition** (from #145's fair-comparison baseline):

| Component | Value |
|---|---:|
| Reported gap (E3 vs 0.9188) | −0.1530 |
| Runtime cost (v1 llama.cpp → mlx_lm.server) | +0.0739 |
| **E3's actual adapter-quality delta** | **−0.0790** |

## Fair-comparison — E3 vs v1 same-runtime (both mlx_lm.server)

**E3 aggregate: 0.7658** (n_runs=2)
**v1 fused: 0.8449** (n_runs=5; from #145)
**Delta: −0.0790**

E3 was still a REAL FAIL at −0.079, but the reported magnitude was ~2× too pessimistic. Compare to #145 (mine, same-runtime delta −0.006): E3 lost 13× more capability than my #145 adapter did.

## Per-task detail (E3 vs v1 same-runtime; sorted by delta asc)

| Task | v1 | E3 | Δ (E3−v1) |
|---|---:|---:|---:|
| **hidden.reclassify** | 0.9400 | 0.4000 | **−0.5400** |
| **retrieval.rerank_cross** | 0.9000 | 0.5617 | **−0.3383** |
| **claude.code_knowledge** | 0.3867 | 0.2517 | **−0.1350** |
| **retrieval.query_classify** | 0.7750 | 0.6625 | **−0.1125** |
| jiminy.synthesize | 0.8844 | 0.8573 | −0.0271 |
| ape.reflect | 0.9547 | 0.9333 | −0.0213 |
| jiminy.evaluate_llm | 0.7667 | 0.7500 | −0.0167 |
| jiminy.codegen | 1.0000 | 0.9850 | −0.0150 |
| consulting.classify | 0.8827 | 0.8808 | −0.0018 |
| jiminy.evaluate | 0.9667 | 0.9667 | 0.0000 |
| hidden.name_emergence | 0.9500 | 0.9500 | 0.0000 |
| hidden.summarize | 0.9000 | 0.9000 | 0.0000 |
| retrieval.intent_translate | 0.9780 | 0.9900 | **+0.0120** |

**Group means**:

| Group | Weight | v1 | E3 | Δ |
|---|---:|---:|---:|---:|
| C (classify_notink) | 0.35 | 0.9237 | 0.8142 | **−0.1096** |
| J (structured_notink) | 0.15 | 0.8722 | 0.7539 | **−0.1183** |
| T (reasoning_think) | 0.5 | 0.7815 | 0.7356 | **−0.0459** |

## Interpretation

**E3's FAIL verdict is CORRECT but the root-cause narrative shifts nuance**:

E3's original sprint_post attributed the FAIL to "single-task collapse of claude.code_knowledge (0.2517) driven by v3-stripped corpus". That's ONE contributor but NOT the biggest one — **hidden.reclassify (−0.54) and retrieval.rerank_cross (−0.34) each moved the aggregate more** than CCK did. E3 destroyed MDEMG-operational capabilities that #145 preserved or improved (my #145 adapter posted +0.06 on hidden.reclassify vs E3's −0.54 — a 0.60 gap).

**What made E3 different from #145 (which was at parity)?**
- **E3 config**: batch=2, max_seq=4096, **1 epoch**, iter-1200 checkpoint (from 3376-iter run, early-stopped)
- **#145 config**: batch=2, max_seq=4096, **2 epochs**, iter-7200 checkpoint (from 7702-iter completed run, best val_loss)
- **E3 corpus**: 5 families, stripped v3 CCK (453 rows)
- **#145 corpus**: 6 families = E3's 5 + mdemg_usage_v1 (949 rows)

The material differences: **E3 was HALF the training length of #145** (1 epoch vs 2 epochs; 1200 iters vs 7200 iters). At ~2 epochs of training, the adapter developed MDEMG-operational capability parity with v1. At ~1 epoch (iter 1200 out of 3376), it was under-trained and lost capability broadly across MDEMG tasks — especially the ones with less signal density in the training corpus (hidden.reclassify and retrieval.rerank_cross specifically).

## Comparison to #145 findings

| Sprint | Same-runtime delta | Real adapter delta | Verdict |
|---|---:|---:|---|
| #145 (mdemg_usage_lora_001) | −0.006 | Parity | ⚠️ PARITY (but NO-PROMOTE via GGUF #150) |
| **#138 E3 (this sprint's rebench)** | **−0.079** | Real capability loss | ❌ FAIL (RESTATED, still holds) |

**Two same-runtime measurements confirm the runtime cost is real (+0.074 for v1) and same-runtime is the correct comparison frame.** E3's rebench validates PHASE-E3 arch rule 5.

## What ships

- `docs/development/phase-e3-rebenchmark-001/verdict.md` — this document (the RESTATED verdict)
- `docs/development/phase-e3-rebenchmark-001/sprint_post.md`
- CHANGELOG entry
- PR summary comment
- Task #149 → completed
- **No code changes.** E3's original `docs/development/phase-e3-retrain-benchmark-001/` records remain in place; this sprint adds a REVISION note pointing to the same-runtime numbers.

## E3's original verdict narrative — what to update

The original E3 sprint_post attributed the FAIL primarily to:
1. Single-task claude.code_knowledge collapse from v3-stripped corpus
2. Reduced-scope config (batch=2 vs 4, max_seq=4096 vs 8192)

**The REVISED root cause narrative** should say:
1. **Adapter-quality loss is REAL** at −0.079 same-runtime — E3 did degrade capability
2. **Reduced-scope config** ALONE isn't sufficient — #145 used identical scope and hit parity. The difference was TRAINING DEPTH: E3 was ~1 epoch (iter 1200) vs #145's 2 epochs (iter 7200)
3. **Reported gap 0.153 was 2× inflated** by the mlx_lm.server → llama.cpp runtime cost (0.074 of the 0.153)
4. **CCK stripping** is a contributor (−0.135 on claude.code_knowledge) but NOT the biggest (hidden.reclassify −0.54, retrieval.rerank_cross −0.34 are larger)

The E3 verdict itself (❌ FAIL, NO PROMOTION) remains correct. The narrative around WHY improves with same-runtime data.

## Follow-up: no re-verdict revision to E3 sprint docs

E3's arc is closed. Not re-writing E3's shipped sprint records — this sprint's verdict.md serves as the historical revision note. Future readers who compare E3 to any new adapter's benchmark should reference this document alongside E3's original.

## Documents Accessed

- `training_data/eval/e3_benchmark.json` (E3 shipped benchmark, task #138)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (v1 same-runtime baseline, #145)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (E3 shipped record)
- `docs/development/mdemg-usage-lora-001/verdict.md` (#145 revised verdict — the source of the cross-runtime-bug diagnosis)
- `docs/development/mdemg-usage-lora-001-gguf/verdict.md` (#150 verdict)
- CLAUDE.md pins:
  - PHASE-E3 arch rule 5 (same-runtime comparison — #145; this sprint validates it)
  - `iterate-break-fix-verify`, `must-master-data-pipelines`
- Operator directive 2026-09-01 ("proceed with #149")
