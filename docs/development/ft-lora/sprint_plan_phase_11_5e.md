# Sprint Plan — Phase 11.5e: Eval Coverage Augmentation

**Sprint:** FT-LORA-PHASE11.5e (Bring 8 data-starved tasks online)
**Date:** 2026-04-30
**Branch:** `reh3376_dev01`
**Status:** PLANNED — awaiting approval
**Predecessors:** Phase 11.5c (`valid_clean.jsonl`), Phase 11.5d (row-sweep fix + Stage-1 distill promoted)
**Successor:** Phase 12 HITL DPO OR re-evaluation of Run 7 / Stage-1 against augmented eval

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.5e |
| Title | Eval coverage augmentation — rescue stale-hash tasks + augment data-starved tasks + investigate zero-data tasks |
| Type | Code-medium (~400-600 LOC: relaxed extractor + synthesis driver + production-feature audit), data-medium (~80-120 new eval rows), compute-light (~40 min full-sweep × 4 models on augmented eval) |
| Risk | LOW (read-only TSDB; eval-set augmentation is additive; no model training) |
| Budget | OpenAI synthetic prompt generation + full-sweep re-baseline: ~$2-3 |
| Output adapter | None — eval-coverage sprint, no model training |
| New TSDB migration | None |
| Post-sprint artifacts | Augmented `valid_clean.jsonl` (180 → ~280 rows, 9 → ~17 tasks); `valid_clean_manifest.json` updated; `scripts/build_clean_eval.py` patched (relaxed hash mode); `scripts/x10_synth_prompt_capture.py` (new); `training_data/eval/synthetic/{specs,prompts,manifest}.json`; per-task feature-deployment audit; updated 4-model full-sweep baselines; sprint post-doc |

---

## 2. Problem Statement

The Phase 11.5c `valid_clean.jsonl` covers **9 of 17 ULTS tasks** with leak-free production data. The other 8 contribute nothing to aggregate scoring on `valid_clean`. Aggregate weighted score is computed across the 9 measured tasks only — meaning every model's "0.85+ aggregate" reflects only ~half the model's claimed task surface.

Per Phase 11.5d Epic 0 + post-merge audit:

| Category | Tasks | TSDB rows (any hash) | Why starved | Sprint strategy |
|---|---|---|---|---|
| **Stale-hash rescue** | `jiminy.evaluate` (338), `jiminy.evaluate_llm` (99) | 437 | TSDB rows have older `system_prompt_hash` not in current spec | Relax extractor; rebuild rows; re-leak-audit |
| **Tiny-data augmentation** | `guardrail.evaluate` (3), `hidden.summarize` (1) | 4 | Production rarely-or-never invokes; existing rows insufficient | Synthetic prompt generation via gpt-5.4-mini |
| **Zero-data investigation** | `consulting.synthesis`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate` | 0 | No production traffic logged with this task_name | Verify feature is deployed; if yes, synthesize; if no, deprecate from benchmark |

**Goal:** bring **all 17 tasks online** (or document deprecations) so future benchmarks measure the full model surface. Re-baseline 4 models (Phase 5, Run 7, Stage-1 distill, gpt-5.4-mini) on the augmented eval; publish updated comparison.

**Why this sprint now:** the user's purpose statement (better decisions through connection-layer quality) is undermined when 8 of 17 connection-layer tasks aren't measured. Every aggregate score reported in 11.5c-d is computed across the 9 well-supplied tasks. We don't know whether the Stage-1 distill adapter improved or hurt `guardrail.evaluate`, `consulting.synthesis`, or `summarize.generate`. With production now serving Stage-1 (PR #363 merged), eval blind spots translate directly to undetected production regressions.

**Forcing function:** Phase 12 HITL DPO consumes pairs from `benchmark_results`. With 8 tasks producing zero rows, Phase 12 starts blind on those tasks. Better to fix eval coverage now than to discover during Phase 12 that DPO has nothing to compare for the missing tasks.

---

## 3. Scope & Constraints

**In scope:**
- Patch `scripts/build_clean_eval.py` — add `--strict-hash {strict, relaxed}` mode. `relaxed` matches by `task_name` only (still applies leak filter), used for stale-hash rescue.
- New script `scripts/x10_synth_prompt_capture.py` — given a ULTS spec, generate N synthetic user prompts via gpt-5.4-mini conditioned on task description + production system prompt; capture gpt-mini's response; reward-audit; emit valid_clean-format rows.
- New script `scripts/audit_task_deployment.py` — for each task_name, sample TSDB for a recent week; check if any row exists. Report deployed-vs-deprecated.
- Augmented `valid_clean.jsonl` — append rescued + synthetic rows; preserve original 180; total ~280-300.
- Updated manifest with provenance per row (`source: "tsdb_strict"` / `"tsdb_relaxed"` / `"synthetic_gpt54mini"`).
- Re-baselines for 4 models on augmented eval.
- Post-doc with strategy verdict per task.

**Out of scope:**
- Model training of any kind (Phase 11.5d adapter remains canonical)
- Reward function changes (use existing `REWARD_REGISTRY`)
- Real production feature deployment for the 4 zero-data tasks (engineering decision, not sprint deliverable)
- Re-running Phase 12 / Phase 11 RL on the augmented eval (separate sprints)
- Re-labeling the 4 ambiguous `retrieval.query_classify` rows from the 11.5d follow-up (independent quick fix; can roll into this sprint or defer)

**Hard constraints (MEMORY):**
- **No hardcoded values** — task list, synthesis count, OpenAI temp all in CLI flags / config
- **CUIDv2** for synthetic-row provenance IDs
- **Sequential epics** — extract before synthesize before re-baseline
- **3-tier testing** — unit (relaxed-mode set-diff math, synthesis JSONL conversion), integration (mocked OpenAI synthesis + mocked TSDB rescue), e2e (full augmented eval run on Phase 5 base, ≥0 leakage audit)
- **min `max_tokens` ≥ 3000, `latency_budget_ms` ≥ 15000** per spec performance config
- **Plan-options pattern** — synthesis count, deprecation-vs-augmentation calls disclosed
- **Single batched commit** at sprint close
- **Sprint summary on PR comment** immediately
- **Read-only on canonical adapter** — `.local-models/qwen3-14b-mdemg-v1-rl/` not modified
- **MLX single-instance** preflight on `127.0.0.1:8101`
- **Original `valid_clean.jsonl` row count preserved** — augmentation is additive (so prior 11.5c/d reports remain comparable to the old subset)

---

## 4. Dependencies

**Consumed (code, pre-existing):**
- `scripts/build_clean_eval.py` (Phase 11.5c) — extended with `--strict-hash` flag
- `scripts/audit_eval_leakage.py` (Phase 11.5c) — reused for leak gate on synthetic rows
- `scripts/x9_distill_capture_v2.py` (Phase 11.5d) — pattern for OpenAI capture + leak filter
- `scripts/x7_gpt54mini_benchmark.py` — OpenAI HTTP transport
- `neural/benchmarks/run_benchmark.py` (Phase 11.5d row-sweep version) — reused unchanged for re-baselines
- `neural/training/reward_functions.py` — reward registry; verified working on synthetic responses in Epic 1
- `.local-models/qwen3-14b-mdemg-v1*` (Phase 5, Stage-1 distill, Run 7 archive) — re-baseline targets
- 17 ULTS specs in `docs/tests/ults/specs/*.ults.json`

**Consumed (data, leakage check sources):**
- All 10 sources from 11.5c (9 train/valid + valid_clean) PLUS the augmented valid_clean.jsonl itself (ensure synthetic rows don't accidentally duplicate existing ones)

**Consumed (compute):**
- TSDB queries: ~10 sec total
- OpenAI synthetic generation: ~10-15 prompts × 6 augmentation tasks × 2 calls each (synthesize + verify gpt-mini response) = ~120 calls × $0.005 = ~$0.60
- OpenAI re-baseline gpt-mini on augmented eval: 17 tasks × 20 rows × 2 runs ≈ 680 calls × $0.005 = ~$3.40
- Local MLX re-baseline Phase 5 / Run 7 / Stage-1 on augmented eval: ~40 min each × 3 = ~2 hr
- Total wall-clock ≈ 3-4 hr; OpenAI ≈ $4

---

## 5. Implementation Plan (Sequential Epics + Gates)

### Epic 0 — Preflight + Feature-Deployment Audit (≈45 min)

1. **Verify production feature deployment for the 4 zero-data tasks** (`consulting.synthesis`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`). For each:
   - `grep -rn "<task_name>"` in `internal/` to find call sites
   - Check feature flags in `internal/config/config.go` (e.g., `EMERGENCE_ENABLED`, `INTENT_ENABLED`)
   - Check Docker compose template for env-gating
   - **Verdict**: deployed-active / deployed-feature-gated-off / not-deployed
2. **Verify the stale-hash hypothesis** for `jiminy.evaluate` + `jiminy.evaluate_llm`:
   - Pull a sample TSDB row's `system_prompt` for each
   - SHA256 it; compare to current Go-source production hash + spec hash
   - Confirm the TSDB hash is NOT in the spec's hash list
3. **Verify reward functions work on synthetic prompts** for the 6 to-be-augmented tasks:
   - For each task, manually craft 1 plausible (prompt, response) pair
   - Call `compute_reward(response, spec.reward_functions, schema=spec.output_schema)`
   - Confirm non-zero reward where appropriate (no scorer bugs analogous to the Phase 10 silent-zero issues)
4. **MLX single-instance check** on `:8101` (none running expected; sprint will start one in Epic 4).
5. **109+ unit tests** still green from prior sprint.

**Gate:** deployment-audit results recorded; reward functions verified for all 6 augmentation tasks; tests green.

### Epic 1 — Stale-Hash Rescue (≈30 min)

1. **Patch `scripts/build_clean_eval.py`**:
   - Add `--strict-hash {strict, relaxed}` argument (default `strict` to preserve 11.5c reproducibility)
   - In `relaxed` mode: skip `system_prompt_hash` filter; match by `task_name` only; still apply leak filter and dedup
   - When operating in mixed mode: write `meta.system_prompt_hash_match: "strict" | "relaxed"` per emitted row for downstream audit
2. **Run rescue extraction** for `jiminy.evaluate` + `jiminy.evaluate_llm`:
   - Target 20 rows each (matching other tasks)
   - Output to a separate file first: `valid_clean_rescued.jsonl`
   - Leak-audit against 9 train/valid sources
3. **Spot-check 3 rescued rows** per task by inspection — confirm prompt is sensible and response is parseable.

**Gate:** ≥10 rescued rows per task; leak audit zero overlap; spot-check passes.

### Epic 2 — Synthetic Prompt Generation (≈1 hr, ~$1 OpenAI)

1. **New script `scripts/x10_synth_prompt_capture.py`**:
   - Args: `--task-name <name>`, `--target N` (default 20), `--out PATH`
   - For each task, build a meta-prompt: "Generate a realistic user prompt for the following task: \<task description from ULTS spec>. Examples of expected output format: \<output_schema>. The user prompt should be ~\<typical-length> characters." Send to gpt-5.4-mini at temp=0.9 (encourage diversity). Generate N user-prompt strings.
   - For each generated prompt, call gpt-5.4-mini AGAIN (this time at the task's regular sampling temp) with the production system prompt + synthetic user prompt; capture response.
   - Compute reward; keep where reward ≥ 0.7 (lower threshold than 11.5d distill since synthesis quality is lower).
   - Write valid_clean-format rows with `meta.source: "synthetic_gpt54mini"` and full provenance.
2. **Generate per-task** for the 6 tasks below. **Plan-options decision** at execution time on whether to include `consulting.synthesis` and `metalearn.generalize` if Epic 0 audit shows them feature-gated-off:
   - `guardrail.evaluate` (J-group, augment to 20 — highest priority; security-relevant)
   - `hidden.summarize` (T-group, 20 — connection-layer)
   - `consulting.synthesis` (T-group, 20 if deployed)
   - `metalearn.generalize` (T-group, 20 if deployed)
   - `retrieval.rerank_nli` (T-group, 20 — NLI ranking is connection-layer)
   - `summarize.generate` (T-group, 20)
3. **Leak audit** synthetic rows against all 10 prior sources.
4. **Manual spot-check** for prompt realism — do they look like prompts a developer would actually send through MDEMG?

**Gate:** synthetic rows generated for ≥4 of 6 tasks (the 2 deferred tasks if Epic 0 verdict is feature-gated-off); leak audit clean; per-task target counts met.

### Epic 3 — Augment `valid_clean.jsonl` + Manifest Update (≈30 min)

1. Append rescued + synthetic rows to `valid_clean.jsonl`. Preserve original 180 rows at the head; new rows at the tail.
2. Update `valid_clean_manifest.json`:
   - Add per-task source breakdown (tsdb_strict / tsdb_relaxed / synthetic)
   - Update file SHA256
   - Bump `manifest_version` to `2.0`
3. Run final leak audit on the augmented file (must show 0 overlap with 9 train/valid sources).
4. Per-task row count summary in the post-doc.

**Gate:** augmented file passes leak audit; manifest schema-valid; row-count breakdown matches Epic 1+2 outputs.

### Epic 4 — Re-baseline 4 Models on Augmented Eval (≈3 hr local + 18 min OpenAI)

Sequential — only one MLX model loaded at a time on `:8101`:

1. **Phase 5 dense** → `training_data/eval/baseline_phase5_clean_v2_fullsweep.json`
2. **Run 7 archive** (load Phase 5 base + `qwen3-14b-mdemg-v1-rl-run7` adapter) → `baseline_run7_clean_v2_fullsweep.json`
3. **Stage-1 distill (canonical)** (load Phase 5 base + `qwen3-14b-mdemg-v1-rl` adapter) → `baseline_stage1_clean_v2_fullsweep.json`
4. **gpt-5.4-mini** (OpenAI) → `baseline_gpt54mini_clean_v2_fullsweep.json`

All with `--rows-per-spec 0`, `--n-runs 2`, `--mlx-timeout-s 300`.

**Gate:** all 4 reports written; aggregate scores recorded; per-task deltas computed.

### Epic 5 — Strategy Report (≈45 min)

1. **`training_data/eval/clean_v2_comparison.md`** — extends 11.5d's clean_vs_golden_delta.md:
   - 4-way per-task table (Phase 5 / Run 7 / Stage-1 / gpt-mini)
   - Per-task source breakdown (which rows are tsdb_strict vs synthetic)
   - Aggregate by source: how does the augmented eval compare to the 11.5d-era 9-task subset?
   - Net effect on Stage-1's gpt-mini-parity claim: still at parity? Now beating? Now lagging?
2. **Production-impact assessment**: any tasks where Stage-1 underperforms gpt-mini by >5pp on augmented eval are flagged for follow-up. The user's purpose statement (better decisions through connection-layer quality) ranks per-task connection role.

**Gate:** report committed; verdict recorded.

### Epic 6 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/phase_11_5e_post.md` — executed-truth.
2. `docs/development/ft-lora/sprint_plan_phase_11_5e.md` — this plan, marked EXECUTED.
3. `00_README_v2.md` v5.10 → v5.11.
4. `04_BENCHMARK_RL_v2.md` — eval-coverage augmentation pattern + per-task source provenance.
5. `AGENT_HANDOFF.md` top entry.
6. `CHANGELOG.md [Unreleased] ### Added`.
7. `CLAUDE.md` Testing section — augmented eval invocation; relaxed-hash mode for build_clean_eval.

**Gate:** all docs committed; cross-refs valid.

---

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit):**
- `tests/scripts/test_build_clean_eval_relaxed.py` — relaxed-hash mode set-diff, source provenance tagging.
- `tests/scripts/test_x10_synth_capture.py` — meta-prompt construction, JSONL conversion, reward filter, leak audit integration.

**Tier 2 (Integration):**
- Mocked OpenAI (mock_http_post) returning canned synthetic prompts → verify rounding-trip JSONL writes correctly.
- Mocked TSDB extraction with stale-hash fixtures → verify relaxed mode picks them up; strict mode rejects them.

**Tier 3 (E2E):**
- Smoke (1 task × 3 prompts × 1 sample, ~$0.02): full pipeline.
- Full Epic 2 + Epic 3 + Epic 4 (Phase 5 only) on real OpenAI + real local MLX.

---

## 7. Commit Strategy

Single batched commit at sprint close:

- Title: `feat(ft-lora): Phase 11.5e — eval coverage augmentation (8 tasks brought online)`
- Body: source breakdown, per-task verdict (strict-hash / relaxed-rescue / synthetic / deprecated), augmented-eval re-baseline numbers, OpenAI cost, plan-options decisions disclosed.
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push → auto-PR → sprint summary on PR comments.

---

## 8. Verification Checklist

- [ ] Epic 0: feature-deployment audit complete; reward functions verified for 6 augmentation tasks; tests green
- [ ] Epic 1: stale-hash rescue ≥10 rows per task × 2 tasks; leak audit zero
- [ ] Epic 2: synthetic generation ≥10 rows per task for ≥4 of 6 tasks; leak audit zero; spot-check passes
- [ ] Epic 3: augmented `valid_clean.jsonl` passes final leak audit; manifest v2.0 written; row counts match
- [ ] Epic 4: 4 model baselines on augmented eval all complete
- [ ] Epic 5: comparison report + production-impact verdict written
- [ ] Epic 6: all docs committed
- [ ] Single commit pushed; auto-PR opened; sprint summary on PR
- [ ] OpenAI spend ≤ $5

---

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 6.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Synthetic prompts don't match production distribution → eval becomes unrepresentative | Medium | Generate at temp=0.9 for diversity; gpt-mini is conditioned on full ULTS task description + production system prompt; spot-check for realism; tag rows with `source: synthetic` so future analyses can subset-by-provenance | Drop synthetic rows from a task if spot-check fails; document gap |
| 2 | gpt-mini's synthesized responses score too low (<0.7) → too few kept pairs | Low-Medium | Reward threshold relaxed to 0.7 (vs 0.8 in 11.5d); synthetic rows are eval (not training) so quality bar can be relaxed | Lower to 0.5; if still fails, flag the reward function for inspection |
| 3 | A "zero-data" task is actually deprecated but Epic 0 audit misses this | Low | Multi-source verification: grep + config flag + compose template | Document conservatively as "active per code"; deprecate later if needed |
| 4 | jiminy.evaluate / jiminy.evaluate_llm relaxed-mode rescue picks up actually-different-task rows (mislabeled) | Medium | The "TSDB rows mislabeled" finding from 11.5c was specifically about jiminy.evaluate/_llm cross-contamination — the fix is to verify rescued rows have the right output shape per spec output_schema | Per-row schema validation gate; reject mismatches |
| 5 | Augmented eval scores diverge significantly from 11.5d baselines (Stage-1 no longer at parity) | Medium-High (this is the discovery this sprint enables) | The whole point of the sprint is to surface this; result drives next-sprint decision | If Stage-1 underperforms badly on augmented tasks → consider rolling back to Phase 5 dense; flag in post-doc |
| 6 | OpenAI spend overruns ($5+) | Low | Hard cap in synthesis driver: stop after configured budget; full-sweep gpt-mini re-baseline ~$3.40 known | Halt early; ship with partial coverage; document gap |
| 7 | Synthetic prompts inadvertently match valid_clean prompts (re-leak) | Low | Leak audit gate before merge into valid_clean.jsonl; dedup at write time | Re-generate the affected synthetic rows |
| 8 | Manifest version bump v1.0 → v2.0 breaks downstream consumers | Low | Manifest is read-only; version field is informational; field schema is purely additive | None needed |

---

## 11. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_5d_post.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_5c_post.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5{c,d}.md`
- `/Users/reh3376/mdemg/training_data/eval/{valid_clean.jsonl, valid_clean_manifest.json, baseline_*_clean_fullsweep.json}`
- `/Users/reh3376/mdemg/scripts/{build_clean_eval.py, audit_eval_leakage.py, x9_distill_capture_v2.py, x7_gpt54mini_benchmark.py}`
- `/Users/reh3376/mdemg/docs/tests/ults/specs/*.ults.json` (17 specs)
- `/Users/reh3376/mdemg/internal/**/*.go` (production system prompts + feature-flag wiring)
- `/Users/reh3376/mdemg/configs/{benchmark_phase10.yaml, config.go}`
- TSDB `llm_interactions` (production traffic per task_name)
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_no_tight_llm_budget_caps.md`, `project_mdemg_purpose.md`

---

## 12. Rollback

All changes additive. No model artifacts created. No TSDB writes.

1. `git revert <final commit SHA>`.
2. Restore `valid_clean.jsonl` to the pre-augmentation 180-row version (backup created in Epic 3 as `valid_clean.jsonl.pre_v2_bak`).
3. Delete additive artifacts: `valid_clean_rescued.jsonl`, `training_data/eval/synthetic/`, `baseline_*_clean_v2_fullsweep.json`, `clean_v2_comparison.md`, `phase_11_5e_post.md`, `scripts/x10_synth_prompt_capture.py`, `audit_task_deployment.py`.
4. Phase 5 / Stage-1 distill / Run 7 archive untouched throughout.

No Neo4j writes.

---

## Time + Budget Projection

| Path | Wall-clock | OpenAI $ |
|---|---|---|
| Best case (all 6 augmentation tasks succeed cleanly) | 5-6 hr | ~$4 |
| Typical (4 of 6 augmented; 2 deprecated per Epic 0) | 4-5 hr | ~$3 |
| Worst case (synthesis quality issues; multiple re-runs) | 8-10 hr | ~$6 |

All paths well within MEMORY $100 cap.

---

## Open Decision Points (Plan-Options at Execution)

1. **Per-task augmentation strategy** for `consulting.synthesis` and `metalearn.generalize` — Epic 0 audit decides
2. **Synthesis count per task** — default 20, may scale up/down based on per-task TSDB supply (some tasks may have >20 rescuable rows in relaxed mode)
3. **Re-label `retrieval.query_classify` ambiguous rows** (4 file-path-edit prompts) — defer or roll into Epic 3?
4. **Reward threshold for synthesis** — default 0.7, may relax to 0.5 if too few rows pass
5. **Re-baseline ordering** — gpt-mini first (parallel) or Phase 5 first (faster turnaround)?
