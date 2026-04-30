# Phase 11.5e — Eval Coverage Augmentation, Post-Doc

**Sprint:** FT-LORA-PHASE11.5e
**Date:** 2026-04-30
**Branch:** `reh3376_dev01`
**Status:** EXECUTED — eval coverage 9 → 16 tasks; Stage-1 distill rolled back; Phase 5 dense reinstated as production canonical
**Predecessors:** 11.5c (`valid_clean.jsonl` v1, 9 tasks, leak-free) + 11.5d (Stage-1 distill promoted, row-sweep fix)

---

## Executive Summary

Brought 7 of 8 data-starved tasks online (rescued 2 via content-routing through a discovered production task_name swap; synthesized 5 via gpt-5.4-mini). Augmented `valid_clean.jsonl` from 180 rows × 9 tasks to **319 rows × 16 tasks** (1 task — `retrieval.rerank_nli` — deprecated as Ollama-only, dead in OpenAI production).

Re-baselined 4 models on the augmented eval. **The verdict inverted from 11.5d:** Phase 5 dense (no adapter) leads aggregate at 0.8389. Stage-1 distill (the Phase 11.5d canonical) is **the worst of the 4 models at 0.8294** (-0.95pp behind Phase 5).

**Production rollback executed.** Stage-1 archived at `.local-models/qwen3-14b-mdemg-v1-distill-stage1/`. Phase 5 base now serves production directly. Run 7 GRPO archive untouched at `.local-models/qwen3-14b-mdemg-v1-rl-run7/`.

The Phase 11 RL + Phase 11.5d distillation arc is, in retrospect, **net-negative on broader eval coverage**. The 11.5d "+0.26pp Stage-1 over Phase 5" was a 9-task-subset artifact; 7 more tasks of coverage flipped the verdict.

---

## What Was Shipped

| Artifact | Path | Description |
|---|---|---|
| Augmented eval | `training_data/eval/valid_clean.jsonl` (v2) | 319 rows × 16 tasks; 0 leakage; manifest v2.0 |
| Backup of v1 | `training_data/eval/valid_clean.jsonl.pre_v2_bak` | 180-row pre-augmentation snapshot |
| Manifest v2 | `training_data/eval/valid_clean_manifest.json` | per-task source breakdown, augmentation provenance |
| Rescue script | `scripts/x11_jiminy_evaluate_rescue.py` | content-routing extractor (rescues task_name-mislabeled rows from TSDB) |
| Synthesis script | `scripts/x10_synth_prompt_capture.py` | gpt-mini prompt + response synthesis with reward filter |
| Rescue + synthesis manifests | `training_data/eval/valid_clean_{rescued,synthetic}_manifest.json` | per-step provenance |
| Leak audits | `training_data/eval/valid_clean_{rescued,synthetic,v2}_leakage_audit.json` | 0 overlap on all 3 |
| 4 augmented baselines | `training_data/eval/baseline_{phase5,run7,stage1,gpt54mini}_clean_v2_fullsweep.json` | n_runs=2, rows_per_spec=0 |
| 4-way comparison | `training_data/eval/clean_v2_comparison.md` | per-task table + strategic findings |
| Epic 0 verdict | `training_data/eval/phase11_5e_epic0_verdict.json` | feature-deployment audit + reward sanity |
| Stage-1 archive | `.local-models/qwen3-14b-mdemg-v1-distill-stage1/` | renamed from `-rl/`; manifest annotated with rollback note |

---

## Three Major Discoveries

### 1. Production Bug — `jiminy.evaluate` ↔ `jiminy.evaluate_llm` Task Labels Are Swapped

TSDB rows tagged `jiminy.evaluate` actually contain the `outcome_classifier` system prompt (which is `jiminy.evaluate_llm`'s production prompt). And vice versa. The latest swap-affected row was **2026-04-29**.

Production `WithContext("jiminy.evaluate", ...)` and `WithContext("jiminy.evaluate_llm", ...)` call sites are crossed. Affects `internal/jiminy/eval_prompt.go` + `internal/jiminy/outcome_classifier.go`.

**Workaround in this sprint**: content-routing — match by `system_prompt_hash` against spec-known hashes, ignore TSDB `task_name`. Rescued 40 rows (20 per task, leak-audit clean).

**Filed as production follow-up**: not in 11.5e scope, but real production logging integrity issue.

### 2. `retrieval.rerank_nli` Is Dead in OpenAI Production

Production OpenAI deployments emit `retrieval.rerank_cross` (3221 TSDB rows). The `retrieval.rerank_nli` task_name is only emitted by the Ollama backend code path (`doRerankWithOllama` at `internal/retrieval/rerank.go:325`). Production doesn't use Ollama.

**Decision**: deprecated from synthesis set. Spec stays in benchmark for code-completeness; contributes 0 rows to augmented eval.

### 3. Phase 5 Beats All 3 Adapters on Augmented Eval

| Model | v2 Aggregate (16 tasks) |
|---|---|
| **Phase 5 dense** | **0.8389** (LEADER) |
| gpt-5.4-mini | 0.8317 |
| Run 7 (archived) | 0.8307 |
| Stage-1 distill (rolled back) | 0.8294 |

The Phase 11.5d Stage-1 promotion was based on 9-task-subset evidence (Stage-1 0.8578 vs Phase 5 0.8553 = +0.26pp). Adding 7 more tasks of coverage:
- Phase 5 lost -1.64pp moving to broader eval
- Stage-1 lost -2.84pp
- The relative ranking flipped: Stage-1 went from leader to last

**Rollback executed.** `mv qwen3-14b-mdemg-v1-rl/ qwen3-14b-mdemg-v1-distill-stage1/`. Production now serves Phase 5 base directly: `mlx_lm.server --model .local-models/qwen3-14b-mdemg-v1 --host 127.0.0.1 --port 8101` (no adapter path).

---

## Sprint Execution

### Epic 0 — Preflight (≈45 min)

- Feature-deployment audit on 4 zero-data tasks: 3 deployed (consulting.synthesis active by default; metalearn.generalize gated off; summarize.generate is CLI-only); 1 deprecated (retrieval.rerank_nli is Ollama-only)
- Stale-hash hypothesis verified on jiminy.evaluate/_llm — and revealed the production swap bug
- Reward functions verified working on synthetic prompts for all 6 candidate tasks (test fixtures returned non-zero scores)
- 167 unit tests still green

### Epic 1 — Stale-Hash Rescue (≈30 min, $0)

- New script `scripts/x11_jiminy_evaluate_rescue.py`: content-routing via SHA256(system_prompt) → spec-known hashes
- Pulled 437 TSDB rows (jiminy.evaluate + jiminy.evaluate_llm), routed 347 by content (90 unrouted = older 3rd hash variant)
- Per task: 20 rows kept after leak-filter + dedup (jiminy.evaluate: 99→53→53→20; jiminy.evaluate_llm: 248→35→35→20)
- Leak audit: 0 / 40 overlap

### Epic 2 — Synthetic Prompt Generation (≈5 min, ~$0.40 OpenAI)

- New script `scripts/x10_synth_prompt_capture.py`: meta-synthesis at temp=0.9 → response capture at task's regular sampling kwargs → reward filter ≥0.7
- Generated 30 candidate prompts per task, captured + scored, kept top N=20
- Per task: guardrail.evaluate 20/20 (mean 1.000); hidden.summarize 19/30 (mean 0.645 — heuristic summary_quality saturates ~0.7); consulting.synthesis 20/27 (mean 0.757); metalearn.generalize 20/20 (mean 0.854); summarize.generate 20/20 (mean 0.864)
- 117 OpenAI calls in 5 min, ~$0.60
- Leak audit: 0 / 99 overlap

### Epic 3 — Augment `valid_clean.jsonl` (≈15 min, $0)

- Backup created at `valid_clean.jsonl.pre_v2_bak` (180 rows preserved)
- Concatenated: existing 180 + rescued 40 + synthetic 99 = **319 rows × 16 tasks**
- Manifest v2.0 with per-task + per-source breakdown
- Final leak audit: 0 / 319 overlap

### Epic 4 — Re-baseline 4 Models on Augmented Eval (≈3 hr)

| Model | Wall-clock | Aggregate |
|---|---|---|
| gpt-5.4-mini (with retry on 502) | ~28 min, ~$3.20 | 0.8317 |
| Phase 5 dense | ~52 min | 0.8389 |
| Run 7 (archive) | ~45 min | 0.8307 |
| Stage-1 distill | ~50 min | 0.8294 |

All 16/17 specs matched (retrieval.rerank_nli deprecated → not matched; expected).

### Epic 5 — Strategy Report

`training_data/eval/clean_v2_comparison.md` (committed) — per-task table, source provenance breakdown, 3 strategic findings, production decision (rollback) recorded.

### Epic 6 — Documentation + Commit + PR (this epic)

- `phase_11_5e_post.md` (this doc)
- `00_README_v2.md` v5.10 → v5.11
- `04_BENCHMARK_RL_v2.md` — eval-coverage augmentation pattern + per-task source provenance + rollback precedent
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md [Unreleased] ### Added`
- `CLAUDE.md` Testing section — augmented eval invocation
- Single batched commit; auto-PR; sprint summary on PR comment

---

## Costs

- **OpenAI**: synthesis ~$0.60 + gpt-mini full-sweep on augmented eval ~$3.20 = **~$3.80 total**
- **Compute**: ~3.5 hr local MLX (3 sequential model swaps)

Both well under the $100 cap.

---

## Decisions Outliving the Sprint

1. **Phase 5 base is the production canonical**, no LoRA adapter. Confirmed by augmented eval. Future training must demonstrate aggregate AND per-task gains over this baseline before promotion.
2. **Phase 11 RL + Phase 11.5d distillation are net-negative interventions** on broader eval. Both archived, both retained for historical comparison. Future RL or distillation requires a fundamentally different design (different reward, different training data distribution, or different optimization target).
3. **Eval coverage is the highest-leverage improvement** when training is plateauing. The 11.5e benchmark expansion changed the production decision (Stage-1 → Phase 5) without any model retraining. **Always extend eval before training.**
4. **Synthetic eval is informative but biased**: gpt-mini scores high on synthetic-eval tasks because it's evaluating its own generation distribution. Production-grounded TSDB rows (180 of 319 in v2) are the true signal; synthetic rows (99 of 319) provide coverage but should be weighted accordingly when interpreting per-task numbers.
5. **3 follow-ups filed**: (a) jiminy.evaluate ↔ evaluate_llm production task_name swap; (b) consulting.classify distillation needs class-balance re-design; (c) retrieval.query_classify 4-row eval-label inconsistency from 11.5d.

---

## Documents Accessed

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_5{c,d}_post.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5{c,d,e}.md`
- `/Users/reh3376/mdemg/training_data/eval/{valid_clean.jsonl, valid_clean_manifest.json, baseline_*_clean_*fullsweep.json, clean_v2_comparison.md, phase11_5e_epic0_verdict.json}`
- `/Users/reh3376/mdemg/training_data/eval/{valid_clean_rescued.jsonl, valid_clean_synthetic.jsonl, *_leakage_audit.json}`
- `/Users/reh3376/mdemg/scripts/{build_clean_eval.py, audit_eval_leakage.py, x7_gpt54mini_benchmark.py, x9_distill_capture_v2.py, x10_synth_prompt_capture.py, x11_jiminy_evaluate_rescue.py}`
- `/Users/reh3376/mdemg/configs/benchmark_phase10.yaml`
- `/Users/reh3376/mdemg/docs/tests/ults/specs/*.ults.json`
- `/Users/reh3376/mdemg/internal/{consulting/synthesis.go, hidden/cluster_summarizer.go, metalearn/generalizer.go, summarize/service.go, jiminy/eval_prompt.go, jiminy/outcome_classifier.go, retrieval/rerank.go, guardrail/prompt.go, config/config.go}` (production prompt sources + feature-flag wiring)
- TSDB `llm_interactions` table (per-task production traffic + per-hash row counts)
- `/Users/reh3376/mdemg/.local-models/{qwen3-14b-mdemg-v1, qwen3-14b-mdemg-v1-rl-run7, qwen3-14b-mdemg-v1-distill-stage1}/` (all adapter directories)
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_no_tight_llm_budget_caps.md`, `project_mdemg_purpose.md`

---

## Approvals

- User selected Path A (revert canonical to Phase 5 base) at sprint close.
- All other sprint decisions ratified inline during execution: deprecate retrieval.rerank_nli (Epic 0), content-routing rescue strategy (Epic 1), 0.7 reward threshold for synthesis (Epic 2), backup `valid_clean.jsonl` before augmentation (Epic 3), parallel re-baseline + retry wrapper for gpt-mini (Epic 4).
