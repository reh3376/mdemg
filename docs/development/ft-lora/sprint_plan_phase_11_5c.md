# Sprint Plan — Phase 11.5c: Clean Eval Construction + Honest Re-Baseline

**Sprint:** FT-LORA-PHASE11.5c (Benchmark Integrity)
**Date:** 2026-04-28
**Branch:** `reh3376_dev01`
**Status:** PLANNED — awaiting execution approval
**Predecessors:** Phase 11.5 diagnostic (X1–X7), Branch B paused mid-Epic-1 due to leakage discovery
**Successor:** unblocks Branch B, Branch A, or any future training-vs-baseline work

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.5c |
| Title | Clean held-out eval set + Phase 5 re-baseline against memorization-stripped scoring |
| Type | Code-medium (~400-600 LOC), data-heavy (extract + de-dup ~56K TSDB rows) |
| Risk | MEDIUM (no destructive changes; the bigger risk is discovering the "real" baseline is much lower than 0.8338) |
| Budget | OpenAI re-baseline of gpt-5.4-mini against clean eval: ~$1; Phase 5 re-baseline is local-MLX (free) |
| Output adapter | None — eval-construction sprint, no model training |
| New TSDB migration | None |
| Post-sprint artifacts | `training_data/eval/valid_clean.jsonl` + `valid_clean_manifest.json`; `training_data/eval/baseline_phase5_clean.json`; `training_data/eval/baseline_gpt54mini_clean.json`; `training_data/eval/clean_vs_golden_delta.md`; `scripts/build_clean_eval.py`; `scripts/audit_eval_leakage.py`; sprint post doc |

---

## 2. Problem Statement

The current Phase 10 benchmark (`training_data/eval/valid_golden.jsonl`, 108 rows) has documented prompt leakage with Phase 5 SFT training data:

- **`retrieval.query_classify`**: 4 of 5 valid_golden prompts also appear in `training_data/sft/tier1/train.jsonl` (which Phase 5 was trained on per `phase_5_sft_post.md:61`).
- **`consulting.classify`**: 2 of 6 valid_golden prompts in family valid set.
- **`guardrail.evaluate`**: 3 valid_golden rows are 1 unique user prompt — single-prompt eval.
- **`metalearn.generalize`, `summarize.generate`, `consulting.synthesis`** etc: never in TSDB; valid_golden rows likely synthetic.

The Phase 5 baseline of **0.8338** therefore reflects a mixture of generalization + memorization. Every prior +Xpp claim (Run 5 +1.76pp, X7 gpt-mini +5.31pp, RL plateau analysis) measures movement against a contaminated reference. The Phase 11 → 11.5 plateau may be partially an artifact: GRPO ratchets the policy in ways that can hurt memorized retrieval while marginally helping generalization, and the benchmark over-weighs the former.

Until eval and train are disjoint, we cannot answer:
- Is Phase 5 SFT actually undertrained on `retrieval.query_classify`, or is the 0.60 score memorization noise?
- Does gpt-5.4-mini's 0.8769 reflect superior reasoning, or coincidental match against unseen-by-it-but-seen-by-Phase-5 prompts?
- What is the realistic ceiling for any future intervention (distill, re-SFT, GRPO)?

**Goal of this sprint:** produce a leak-free `valid_clean.jsonl` evaluation set, run Phase 5 + gpt-5.4-mini against it, publish a `clean_vs_golden` delta report. **No model training.** The deliverable is honest measurement infrastructure.

---

## 3. Scope & Constraints

**In scope:**
- New extractor `scripts/build_clean_eval.py` — pulls TSDB `llm_interactions` rows, applies prompt-level set difference against ALL known train/valid sources, writes leak-free JSONL.
- New auditor `scripts/audit_eval_leakage.py` — given any eval JSONL + a list of train/valid sources, reports per-task overlap counts. Reusable for any future eval.
- Per-task target: 20 unique user_prompt rows where TSDB supply allows; less where data-starved.
- Re-baseline runs: Phase 5 dense + gpt-5.4-mini against `valid_clean.jsonl` using existing Phase 10 `run_benchmark.py` (no code change to runner).
- Delta report `clean_vs_golden_delta.md` — per-task and aggregate score deltas, root-cause hypothesis per task.
- Documentation: known limitations on data-starved tasks (`guardrail.evaluate`, `summarize.generate`, etc).

**Out of scope:**
- Model training of any kind (Branch A/B paused)
- Synthetic prompt generation (LLM-driven prompt augmentation deferred to follow-up)
- Re-running Run 5 / Run 7 RL adapters against the clean eval (deferred — requires a separate "RL-vs-clean" comparison sprint)
- TSDB migration (no schema changes)
- Eval framework rewrite (still uses Phase 10 `run_benchmark.py`)

**Hard constraints (MEMORY):**
- **No hardcoded values** — TSDB connection from `.env`, paths from `configs/`, target-row-count per task as CLI flag with sensible default.
- **CUIDv2 for run_ids** in baseline reports.
- **Sequential epics** — eval construction (Epic 1) before audit (Epic 2) before baseline runs (Epic 3).
- **3-tier testing** — unit (set-diff math), integration (mocked TSDB), e2e (real TSDB extraction + real benchmark run).
- **min `max_tokens` ≥ 3000, min `latency_budget_ms` ≥ 15000** — applies to gpt-mini re-baseline (inherited from `configs/benchmark_phase10.yaml`).
- **Plan-options pattern** — data-starved task handling (drop / synthetic / accept) disclosed at PR.
- **Single batched commit** at sprint close.
- **Sprint summary on PR comments** immediately after push.
- **Existing artifacts read-only** — `valid_golden.jsonl` not deleted; clean eval is additive.
- **MLX single-instance** preflight on `127.0.0.1:8101` for Phase 5 baseline run.

---

## 4. Dependencies

**Consumed (code, pre-existing):**
- TSDB `llm_interactions` table (56,390 rows confirmed live, schema verified): `task_name`, `system_prompt_hash`, `user_prompt`, `response`, `quality`, `instance_id`, `time`.
- `neural/benchmarks/run_benchmark.py` — Phase 10 runner; reused for both baselines via `--golden-path` override.
- `scripts/x7_gpt54mini_benchmark.py` — OpenAI HTTP transport.
- `neural/benchmarks/run_benchmark.Spec` — `system_prompt_hash` per task for matching.
- 17 ULTS specs in `docs/tests/ults/specs/*.ults.json`.
- `.local-models/qwen3-14b-mdemg-v1/` — Phase 5 dense base for re-baseline.
- `mlx_lm.server` — local inference for Phase 5 baseline (port 8101).

**Consumed (data, leakage sources to compute set-difference against):**
- `training_data/sft/tier1/train.jsonl` (3,150 rows) — Phase 5 trained on this
- `training_data/sft/tier1/valid.jsonl` (350 rows) — Phase 5 internal eval
- `training_data/sft/family_classify_notink/{train,valid}.jsonl`
- `training_data/sft/family_reasoning_think/{train,valid}.jsonl`
- `training_data/sft/family_structured_notink/{train,valid}.jsonl`
- `training_data/eval/valid_golden.jsonl` (108 rows, current Phase 10 eval)

**Consumed (compute):**
- TSDB queries: ~10 sec total (indexed lookups by task_name)
- gpt-5.4-mini re-baseline: ~17 tasks × 20 rows × 1 sample ≈ 340 calls × ~3s = ~17 min, ~$0.10
- Phase 5 re-baseline: 17 tasks × 20 rows ≈ 340 MLX inferences × ~5s = ~28 min local

---

## 5. Implementation Plan (Sequential Epics + Gates)

### Epic 0 — Preflight (≈30 min)

1. Confirm TSDB up: `PGPASSWORD=$TSDB_PASSWORD psql -h localhost -p 5433 -U mdemg -d mdemg_metrics -c "select count(*) from llm_interactions;"` → returns 56390.
2. Per-task row count audit (already done in investigation): `ape.reflect: 44878, retrieval.query_classify: 772, guardrail.evaluate: 3, ...`. Save as `training_data/eval/clean_eval_preflight.json` with timestamps.
3. Confirm 17 ULTS specs load + each has a `system_prompt_hash`.
4. MLX single-instance check (port 8101).
5. `pytest neural/training/rl/tests/ neural/training/dpo/tests/` still green (109 tests).

**Gate:** all checks green; preflight report written; per-task TSDB supply enumerated.

### Epic 1 — Build Clean Eval Extractor (≈2 hr)

1. New script `scripts/build_clean_eval.py`:
   - Args: `--target-per-task N` (default 20), `--out training_data/eval/valid_clean.jsonl`, `--config configs/benchmark_phase10.yaml`.
   - For each ULTS spec:
     - Query: `SELECT user_prompt, response, system_prompt, quality, time, trace_id FROM llm_interactions WHERE task_name=$1 AND system_prompt_hash=$2 AND length(response) > 0 ORDER BY quality DESC NULLS LAST, time DESC LIMIT 200`.
     - Build "known prompts" set per task: union of `user_prompt`s from all 7 train/valid files (loaded once, indexed by `task_name`).
     - Filter TSDB rows: `user_prompt NOT IN known_prompts`.
     - De-dup TSDB rows by `user_prompt` (keep highest-quality first occurrence).
     - Pick top N (default 20) clean rows.
   - Write JSONL row format matching `valid_golden.jsonl` schema (system+user+assistant `messages` array; meta with `task_name`, `source: "tsdb"`, `trace_id`, `quality`).
   - Track per-task: TSDB total, after-system-hash, after-leak-filter, after-dedup, final.
2. Manifest `valid_clean_manifest.json`:
   - Per-task: TSDB supply, leak-filter drops, dedup drops, final count, prompt SHAs (so future audits can verify)
   - Sources hashed: SHAs of all 7 train/valid files at build time
   - Build CUIDv2 + timestamp
   - Known limitations: tasks below threshold (e.g. `guardrail.evaluate`, `summarize.generate`) flagged with `data_starved: true`
3. Schema validate output rows match expected Phase 10 input format (call `match_rows_for_spec` smoke test on the new file).

**Gate:** valid_clean.jsonl written; per-task counts ≥ target where possible; data-starved tasks flagged in manifest; schema-valid for runner.

### Epic 2 — Leakage Audit Tool + Cross-Verification (≈1.5 hr)

1. New script `scripts/audit_eval_leakage.py`:
   - Args: `--eval <jsonl path>`, `--against <comma-separated jsonl paths>`, `--out <json report>`.
   - For each task in eval, computes:
     - `total`: rows in eval for this task
     - `overlap_per_source`: count of eval prompts present in each source file
     - `overlap_any`: count present in ANY source
     - Examples (first 3 overlapping prompts truncated to 80 chars)
   - Exit code 0 if `overlap_any == 0` for all tasks; non-zero otherwise.
2. Run audit against valid_clean: must report 0 overlaps with all 7 train/valid sources.
3. **Bonus run** — audit valid_golden against the 7 sources; commits the leakage report we already discovered as `training_data/eval/valid_golden_leakage_audit.json` for the historical record.

**Gate:** valid_clean shows 0 overlap; valid_golden audit confirms documented leakage levels.

### Epic 3 — Re-Baseline Phase 5 + gpt-5.4-mini (≈45 min)

1. **Phase 5 dense base on clean eval:**
   - Start `mlx_lm.server :8101` against `.local-models/qwen3-14b-mdemg-v1/`.
   - Invoke: `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml --golden-path training_data/eval/valid_clean.jsonl --out training_data/eval/baseline_phase5_clean.json --persist-tsdb=false --mlx-model-name qwen3-14b-mdemg-v1`.
   - Capture: aggregate, per-task scores.

2. **gpt-5.4-mini on clean eval:**
   - Adapt `scripts/x7_gpt54mini_benchmark.py` → `scripts/x7b_gpt54mini_clean.py` (single arg change to `--golden-path valid_clean.jsonl`, output to `baseline_gpt54mini_clean.json`). Or extend existing script with env-var override.
   - Run, save per-task + aggregate.

3. **Delta report `training_data/eval/clean_vs_golden_delta.md`:**

   | Task | Phase 5 (golden) | Phase 5 (clean) | Δ | gpt-mini (golden) | gpt-mini (clean) | Δ |
   |---|---|---|---|---|---|---|
   | `retrieval.query_classify` | 0.90 | ? | ? | 0.90 | ? | ? |
   | … (17 tasks) | | | | | | |
   | **Aggregate** | **0.8338** | **?** | **?** | **0.8769** | **?** | **?** |

   Plus written analysis: which tasks moved, root-cause hypotheses (memorization stripped vs novel prompt difficulty), implications for Branch A/B/C.

**Gate:** both baselines complete; delta report committed; aggregate scores honest.

### Epic 4 — Strategic Recommendations (≈45 min)

1. Based on Epic 3 numbers, propose next-sprint options in `docs/development/ft-lora/phase_11_5c_post.md`:
   - **If Phase 5 clean ≈ Phase 5 golden** (delta < 1pp): memorization wasn't significant; Branch B as originally scoped is viable (just leak-aware).
   - **If Phase 5 clean drops 3-5pp**: meaningful memorization; Run 5's "+1.76pp gain" was largely real. Branch B distillation is still viable but with revised target.
   - **If Phase 5 clean drops > 5pp**: dramatic memorization; Phase 5 SFT may have overfit. Recommend Path B (Phase 5 retrain with task-balancing) over Path A (distill).
   - Identify the per-task score drops and rank-order remediation candidates.

**Gate:** options + recommendation written; user decision recorded before any subsequent training sprint.

### Epic 5 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/phase_11_5c_post.md` — executed-truth: extraction stats, baseline numbers, leakage audit results, proposed next-sprint paths.
2. `docs/development/ft-lora/branch_b_implementation_plan.md` — append "PAUSED — see 11.5c" + summary of why.
3. `00_README_v2.md` — add §11.5c entry under Phase 11.5.
4. `04_BENCHMARK_RL_v2.md` — section "Eval Integrity (clean vs golden)" with the delta table.
5. `AGENT_HANDOFF.md` top entry: clean eval shipped; next sprint TBD pending re-baseline review.
6. `CHANGELOG.md [Unreleased] ### Added`: `valid_clean.jsonl`, leakage audit tool, clean-eval baselines.
7. `CLAUDE.md` Testing section — add clean-eval invocation alongside Phase 10 benchmark.

**Gate:** all docs committed; cross-refs valid.

---

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit):**
- `tests/scripts/test_build_clean_eval.py` — set-diff math on synthetic prompt sets, dedup ordering by quality, manifest schema, data-starved flag logic.
- `tests/scripts/test_audit_eval_leakage.py` — overlap counting, exit-code routing.

**Tier 2 (Integration):**
- Mocked-TSDB extraction (in-memory rows) — verify per-task counts, leak-filter behavior, output JSONL schema.
- Audit tool against fixture eval + fixture sources — verify per-task overlap report.

**Tier 3 (E2E):**
- Full extraction against live TSDB — verify counts match preflight expectations.
- Audit valid_clean against all 7 sources → must show 0 overlap.
- Smoke run: Phase 10 runner on a 3-task slice of valid_clean (e.g. T={ape.reflect}, C={retrieval.query_classify}, J={guardrail.evaluate}) — verify scores compute, JSON shape valid.

---

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Phase 11.5c — clean eval construction + Phase 5 honest re-baseline`
- Body: motivation (Phase 10 leakage discovery), extraction stats, leakage audit results, Phase 5 + gpt-mini delta numbers, proposed next-sprint paths.
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push → auto-PR → sprint summary on PR comments immediately.

---

## 8. Verification Checklist

- [ ] Epic 0: TSDB preflight green; 109 tests still green; MLX single-instance
- [ ] Epic 1: valid_clean.jsonl + manifest written; per-task counts ≥ 20 where TSDB allows; data-starved tasks flagged
- [ ] Epic 2: audit shows 0 overlap clean vs sources; valid_golden audit committed for record
- [ ] Epic 3: Phase 5 clean baseline + gpt-mini clean baseline run; delta report written
- [ ] Epic 4: post-doc recommendation written; user decision recorded
- [ ] Epic 5: all docs updated
- [ ] Single commit pushed; auto-PR opened; sprint summary on PR
- [ ] OpenAI spend ≤ $1

---

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 5: `phase_11_5c_post.md`, branch_b plan paused-marker, 00_README §11.5c, 04_BENCHMARK eval-integrity section, AGENT_HANDOFF top entry, CHANGELOG, CLAUDE.md.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Some tasks have <20 clean TSDB rows after leak-filter | High (already known: guardrail.evaluate=3, summarize.generate=0, metalearn.generalize=0) | Per-task `data_starved` flag in manifest; runner skips or marks low-confidence; document explicitly | Build follow-up "synthetic prompt generation" sprint for data-starved tasks |
| 2 | TSDB rows have stale system prompts (from old MDEMG versions) that no longer match current spec hash | Medium | `system_prompt_hash` filter ensures only matching rows; report drops | Lower hash strictness OR exclude stale-version rows |
| 3 | Phase 5 clean baseline drops dramatically (e.g. < 0.50) | Medium | Documented as expected outcome — that's the **point** of this sprint | Path B (Phase 5 retrain) becomes top priority |
| 4 | Phase 5 clean baseline ≈ golden (< 1pp delta) | Low | Means memorization wasn't significant; Branch B can resume as planned | Resume Branch B Epic 1 with leak-aware filter |
| 5 | gpt-mini clean baseline regresses too | Medium | gpt-mini also benefits from prompt familiarity (web-trained) | Compare gpt-mini's golden-vs-clean delta to Phase 5's; relative ranking matters more than absolute |
| 6 | TSDB extraction misses rows due to system_prompt_hash drift between runtime and spec | Medium | Compare hashes from spec.ults.json vs TSDB sample; if mismatch, recompute spec hashes from current production system prompts | Re-derive spec hashes from production → patch ULTS specs → re-run extract |
| 7 | The 56K rows are mostly low-quality (errors, truncations, etc) | Low (sampled output looked OK) | Filter `length(response) > 0` AND `error IS NULL` AND `quality IS NULL OR quality > 0` | Inspect sample, tighten filter |
| 8 | Phase 5 baseline run on clean eval surfaces a regression bug in `run_benchmark.py` we didn't notice with golden | Low | Phase 10 runner is mature; clean eval uses identical schema | Patch runner; document in post |
| 9 | Re-baseline numbers are non-reproducible (sampling stochasticity) | Low | Phase 10 runner uses C-group temp=0.7 stochastic; we accept ±1pp variance and document | Re-run multiple seeds if delta < 1pp |
| 10 | Building clean eval surfaces that some "production" rows in TSDB are themselves test/synthetic (Phase 10 self-pollution) | Medium | Filter TSDB by `instance_id` to exclude rows from Phase 10 benchmark runs (they have specific instance_ids) | Inspect benchmark_runs joined to llm_interactions; exclude runs created during benchmark windows |

---

## 11. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/branch_b_implementation_plan.md` (Branch B paused mid-Epic-1)
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_5_sft_post.md` (training data lineage)
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5.md` (predecessor diagnostic plan)
- `/Users/reh3376/mdemg/training_data/eval/valid_golden.jsonl` (current eval, 108 rows)
- `/Users/reh3376/mdemg/training_data/sft/tier1/{train,valid}.jsonl` (Phase 5 SFT source)
- `/Users/reh3376/mdemg/training_data/sft/family_*/`{train,valid}.jsonl` (Phase 5 family splits)
- TSDB schema: `internal/tsdb/migrations/005_interaction_enrichment.sql`, `008_instance_id.sql`
- `/Users/reh3376/mdemg/neural/benchmarks/run_benchmark.py` (runner reused unchanged)
- `/Users/reh3376/mdemg/scripts/x7_gpt54mini_benchmark.py` (transport reused)
- TSDB queries: `\d llm_interactions`, per-task row counts (56,390 total)
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_sprint_summary_on_pr.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_plan_options_pattern.md`

---

## 12. Rollback

All changes additive. No model artifacts created. No TSDB writes (read-only).

1. `git revert <final commit SHA>`.
2. Delete additive artifacts: `valid_clean.jsonl`, `valid_clean_manifest.json`, `baseline_phase5_clean.json`, `baseline_gpt54mini_clean.json`, `clean_vs_golden_delta.md`, `phase_11_5c_post.md`, `scripts/build_clean_eval.py`, `scripts/audit_eval_leakage.py`, `valid_golden_leakage_audit.json`.
3. Phase 5/10/11 artifacts untouched throughout. valid_golden.jsonl untouched.

No Neo4j writes. No remote pushes beyond branch (auto-PR).

---

## Appendix A — Per-Task TSDB Supply (preflight snapshot)

From `SELECT task_name, count(*), sum(used_for_train), sum(response IS NOT NULL) FROM llm_interactions GROUP BY task_name`:

| Task | TSDB total | Has response | After system_hash filter (TBD Epic 1) |
|---|---|---|---|
| `ape.reflect` | 44,878 | 43,847 | likely high |
| `consulting.classify` | 3,354 | 2,975 | likely high |
| `retrieval.rerank_cross` | 3,175 | 3,160 | likely high |
| `hidden.name_emergence` | 2,072 | 2,022 | likely high |
| `retrieval.intent_translate` | 945 | 924 | likely high |
| `retrieval.query_classify` | 772 | 770 | likely high |
| `hidden.reclassify` | 357 | 357 | likely high |
| `jiminy.evaluate` | 338 | 338 | likely high |
| `jiminy.codegen` | 240 | 239 | likely high |
| `jiminy.synthesize` | 158 | 156 | likely high |
| `jiminy.evaluate_llm` | 97 | 97 | medium |
| `consulting.synthesis` | (none in summary) | — | **NOT IN TSDB** — data-starved |
| `guardrail.evaluate` | 3 | 3 | **DATA-STARVED** |
| `hidden.summarize` | 1 | 1 | **DATA-STARVED** |
| `metalearn.generalize` | (none in summary) | — | **NOT IN TSDB** — data-starved |
| `retrieval.rerank_nli` | (none in summary) | — | **NOT IN TSDB** — data-starved |
| `summarize.generate` | (none in summary) | — | **NOT IN TSDB** — data-starved |

**Coverage analysis:** 11 of 17 tasks have ≥97 TSDB rows (rich for clean-eval extraction). 6 of 17 are data-starved (≤3 rows or missing entirely) — these need synthetic augmentation in a follow-up sprint, or accept benchmark gaps.

