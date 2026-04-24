# Sprint FT-LORA-PHASE10 — Automated Benchmark Framework (MVP)

## Context

Phase 5 SFT shipped `.local-models/qwen3-14b-mdemg-v1/` on 2026-04-23 (PR #347 merged). Dual regression gate PASSED (baseline 0.9805 / fresh dense 0.9505 → 0.9856; 16/16 ULTS tasks), unblocking Phase 10. Per `04_BENCHMARK_RL_v2.md` §10 and `03_IMPLEMENTATION_PLAN_v2.md` §5E/Phase 10, the next sprint is the **automated benchmark framework** — the reward-signal factory that:

1. Verifies each new merged model against the 17-task ULTS suite with deterministic reward functions, producing per-task pass-rate + aggregate weighted score.
2. **Unblocks Sprint F (GRPO/DPO)** by emitting (prompt, response, reward) tuples in the shape GRPO consumes, with per-task reward variance bounds to normalize policy-gradient advantages.
3. Supersedes the ad-hoc `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` pair (stale; referenced in `CLAUDE.md` but driven by deprecated flow).

**Why MVP scope (Option B):** The planning survey confirmed `neural/training/evaluate_ft.py` (843 lines), `regression_gate.py` (294 lines), and `reward_functions.py` (468 lines, 22-entry `REWARD_REGISTRY`, 285 lines of unit tests) are production-ready — Phase 5 shipped on them. What's missing is: (1) an **LLM-as-judge** path for coherence/depth/relevance (spec'd but never implemented — `evaluate_ft.py` currently uses hardcoded heuristic evaluators that duplicate `reward_functions.py` logic), (2) **sampling-group-aware** inference that honors J-group `presence_penalty=1.5` and T-group `temp=0.6/top_p=0.95/top_k=20` per ULTS `inference.sampling`, (3) a **benchmark runner** distinct from the one-shot eval script, and (4) TSDB persistence of per-task pass-rate + reward variance for trend/stagnation detection. Scheduler + CLI wiring + launchd plist deferred to a follow-up — manual trigger via `python -m neural.benchmarks.run_benchmark` is sufficient for Phase 10's forcing function (GRPO unblock).

**Phase dependency chain:** Phase 5 (done) → **Phase 10 (this)** → Phase 11 (GRPO/DPO automated) → Phase 12 (HITL DPO). Phase 11 GRPO requires reward variance bounds per task (computed here, N≥5 runs) to normalize advantages; running GRPO without variance bounds inverts the dependency graph.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE10 |
| Title | Automated Benchmark Framework — LLM-judge + sampling-aware runner + reward variance |
| Date | 2026-04-23 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-PHASE5 (PR #347, merged 2026-04-23); Sprint E reward_functions.py + regression_gate.py |
| Successors | Sprint FT-LORA-PHASE11 (GRPO/DPO) |
| Type | Code-medium (new `neural/benchmarks/` module); infra-light (1 TSDB migration); compute-light (17 tasks × N runs, local MLX + `gpt-5.4-mini` judge ≈ 30-45 min wall-clock per benchmark) |
| Risk | Medium (evaluate_ft.py refactor touches Phase 5 regression path — shadow-run mitigation required) |
| Budget | LLM judge: `gpt-5.4-mini`, ~4-8K tokens/task × 17 tasks × N runs ≈ $2-5/benchmark; under $100 cap |
| Model under test | `.local-models/qwen3-14b-mdemg-v1` (dense SHA pin `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5`) |
| MLX port | `127.0.0.1:8101` (single-instance constraint per Phase 5) |
| New TSDB migration | **V0012** (current head is V0011) — `benchmark_results` + `benchmark_runs` tables |
| Post-sprint artifacts | `neural/benchmarks/run_benchmark.py`, `llm_judge.py`, `sampling_policy.py`, `variance.py`; refactored `evaluate_ft.py` (dual-path shadow mode); migration V0012; updated Grafana panel; sprint docs |

## 2. Problem Statement

Build a repeatable, CI-compatible benchmark that, given a merged MLX model, produces:

1. **Per-task deterministic pass-rate** — exercise each of 17 ULTS specs with its declared `reward_functions` list and `quality_metrics` thresholds, invoking the model with its spec-declared `inference.sampling` (T/C/J group behavior preserved).
2. **LLM-judge scores** for subjective metrics (coherence, depth, relevance, naturalness) that `reward_functions.py` can't verify mechanically. Judge: `gpt-5.4-mini`, `temperature=0`, `seed=<run_id>`, fixed prompt templates checked into `neural/benchmarks/judge_prompts/`, judge output logged to TSDB for auditability.
3. **Aggregate weighted score** using Phase 5's sampling-group weights (T:0.50, C:0.35, J:0.15) — same aggregator as `regression_gate.py`.
4. **Reward variance bound per task** — run each task N=5 times (configurable), compute mean ± stddev of reward; persist both to TSDB. Phase 11 GRPO reads `stddev` as advantage-normalization denominator.
5. **Stagnation signal** — compare last 3 benchmarks: if |aggregate_delta| < 0.5% across all 3 OR any task regresses > 2%, raise stagnation flag (configurable thresholds, not constants).
6. **Persistence** — write one row per (task, run_idx) to new `benchmark_results` TSDB table (V0012 migration); aggregate row to `benchmark_runs`.

Every model checkpoint — Phase 5, future Phase 11 RL outputs, Phase 12 HITL iterations — runs through this same harness. Output shape is the GRPO input shape.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Benchmark runner | `neural/benchmarks/run_benchmark.py` |
| 2 | LLM judge | `neural/benchmarks/llm_judge.py` |
| 3 | Sampling-group-aware inference policy | `neural/benchmarks/sampling_policy.py` |
| 4 | Reward variance aggregator | `neural/benchmarks/variance.py` |
| 5 | Judge prompt templates (4 metrics) | `neural/benchmarks/judge_prompts/{coherence,depth,relevance,naturalness}.txt` |
| 6 | Refactored evaluator (shadow path) | `neural/training/evaluate_ft.py` — add `--scorer={heuristic,registry,dual}` flag; shadow-run registry path in parallel when flag = `dual` |
| 7 | TSDB migration V0012 | `internal/storage/tsdb/migrations/V0012__benchmark_results.sql` |
| 8 | Benchmark config | `configs/benchmark_phase10.yaml` (N_runs=5, stagnation thresholds, judge model, sampling-group weights) |
| 9 | Golden validation holdout | `training_data/eval/valid_golden.jsonl` (carve 10-20% from Phase 5 `valid` splits, seeded) |
| 10 | Baseline benchmark capture | `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` (first authoritative Phase 10 scoring of qwen3-14b-mdemg-v1) |
| 11 | Updated Grafana panel | `grafana/dashboards/mdemg-ft-training.json` — add per-task pass-rate + variance + stagnation panels |
| 12 | Unit + integration + e2e tests | `neural/benchmarks/tests/test_*.py` |
| 13 | Sprint docs | `docs/development/ft-lora/sprint_plan_ft_lora_phase10.md`, `phase_10_benchmark_post.md` |
| 14 | Doc updates | `00_README_v2.md` v5.6 → v5.7; `03_IMPLEMENTATION_PLAN_v2.md §Phase 10` marked EXECUTED with SHA stamps; `AGENT_HANDOFF.md`; `CHANGELOG.md`; `CLAUDE.md` — remove stale `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` references |

**Out of scope (deferred to Phase 10 follow-up or Phase 11):**
- `neural/benchmarks/benchmark_scheduler.py` (cron/launchd scheduled benchmarking)
- `packaging/launchd/com.mdemg.benchmark.plist`
- `mdemg finetune benchmark` CLI command wiring
- GRPO reward emission pipeline (Phase 11)
- Nightly automated benchmarking (Phase 11)

**Constraints (hard):**
- **Shadow-run safety net for evaluate_ft.py refactor** — when `--scorer=dual`, run heuristic + registry paths in parallel; require per-task `|delta| < 1%` on Phase 5 dev-set replay before any production run switches to `--scorer=registry`. Phase 5 regression path must remain bit-identical until dual-run validation confirms equivalence. Default stays `heuristic` this sprint. See Risk #1.
- **Judge determinism** — `gpt-5.4-mini` called with `temperature=0`, `seed=<run_idx>`, `max_tokens=4000`, `latency_budget_ms=30000` (MEMORY: never < 3000 tokens / < 15000 ms). Judge prompt + model ID + seed logged per row in `benchmark_results` for replay.
- **Judge uses its own fixed sampling** — does NOT inherit task's `inference.sampling`. Prevents J-group `presence_penalty=1.5` from leaking into judge reasoning and biasing scores.
- **ULTS preflight** — before run, assert every spec in `docs/tests/ults/specs/*.ults.json` has non-empty `sampling_group ∈ {T, C, J}`, non-empty `reward_functions`, and `inference.sampling` keys matching the group recipe. Abort on any missing field with per-spec diff.
- **MLX single-instance** — pre-flight `ps` for exactly one `mlx_lm.server` on `127.0.0.1:8101`.
- **Base model read-only** — never overwrite `.local-models/qwen3-14b-mdemg-v1/`; benchmarks are read-only consumers.
- **SHA pins asserted** — adapter SHA (Phase 5 manifest) + base SHA + dataset SHA stamped in every `benchmark_results` row and aggregate report.
- **No hardcoded values** (MEMORY) — N_runs, stagnation thresholds, judge model, judge temperature, judge seed derivation, sampling-group weights all live in `configs/benchmark_phase10.yaml` with sensible defaults + CLI overrides.
- **TSDB migration additive** — V0012 creates new tables only; no ALTER on `llm_interactions` or `training_metrics`.
- **Sequential epics** (MEMORY).
- **Single batched commit at sprint close** (MEMORY).
- **3-tier testing** (MEMORY) — unit / integration (mocked MLX + mocked judge) / e2e (real MLX + real `gpt-5.4-mini`).
- **CUIDv2 for run_id** (MEMORY) — never UUID. Python: `cuid2` PyPI package (primary); fallback to `time.time_ns()` + blake2b hash if dep rejected. User confirms choice at plan exit.

## 4. Dependencies

**Consumed (code, pre-existing):**
- `neural/training/reward_functions.py` — `REWARD_REGISTRY` (22 entries), `get_reward_function(name)`, `compute_reward(response, reward_names, **kwargs)`.
- `neural/training/regression_gate.py` — thresholds + verdict; reused as aggregator for pass/fail.
- `neural/training/evaluate_ft.py` — refactored (dual-path shadow mode), not rewritten.
- `docs/tests/ults/specs/*.ults.json` — 17 ULTS specs with `sampling_group`, `reward_functions`, `quality_metrics`, `inference.sampling`.
- `.local-models/qwen3-14b-mdemg-v1/` + Phase 5 manifest with SHAs.
- `configs/` + existing YAML loading utilities.
- `internal/storage/tsdb/migrations/` — V0011 is current head.

**Consumed (data):**
- Phase 5 `valid` splits under `training_data/sft/{tier1,family_*}/valid.jsonl` — 10-20% carved into `valid_golden.jsonl` (seeded) for Phase 10 benchmarks, preventing train/eval leak.
- 17 ULTS specs (read-only).

**External services:**
- Local MLX server on `127.0.0.1:8101` (model under test).
- OpenAI API — `gpt-5.4-mini` as judge; pay-per-token; budget tracked in `configs/benchmark_phase10.yaml`.
- TSDB — reads V0011 schema; writes V0012 new tables only.

No Neo4j writes. No network writes beyond OpenAI + localhost.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean; venv `mdemg-ft-lora` active; `mlx_lm==0.31.2`; Phase 5 model present + SHA matches pin; TSDB reachable at V0011; OpenAI key in `.env`; 17 ULTS specs pass preflight.

### Epic 0 — Preflight + Config Scaffolding

1. ULTS preflight script: `python -m neural.benchmarks.preflight` — asserts 17 specs have `sampling_group ∈ {T,C,J}`, non-empty `reward_functions`, `inference.sampling` keys matching group recipe; writes `preflight_report.json`. Abort on any missing field with per-spec diff.
2. `configs/benchmark_phase10.yaml` — N_runs=5, `aggregate_delta_threshold=0.005`, `per_task_regression_threshold=0.02`, `judge_model="gpt-5.4-mini"`, `judge_temperature=0`, `judge_max_tokens=4000`, `judge_latency_budget_ms=30000`, sampling-group weights (T:0.50, C:0.35, J:0.15).
3. Carve golden holdout: `python -m neural.benchmarks.carve_golden --source training_data/sft/*/valid.jsonl --out training_data/eval/valid_golden.jsonl --fraction 0.15 --seed 0`. Deterministic; SHA stamped.

**Gate:** preflight all-green; config loads; golden holdout SHA recorded.

### Epic 1 — Variance + Judge + Sampling Policy (supporting modules)

1. `variance.py` — `RunAggregator`: accepts N=5 (task_id, run_idx, response, reward_vector) tuples; emits mean/stddev/min/max per task; persists to `benchmark_results`.
2. `sampling_policy.py` — `resolve_sampling(ults_spec)` → MLX-compatible kwargs dict. T: `temp=0.6/top_p=0.95/top_k=20`; C: `temp=0.7/top_p=0.8/top_k=20`; J: `temp=0.0/top_p=1.0/top_k=-1/presence_penalty=1.5` (mandatory).
3. `llm_judge.py` — `judge(response, metric, context, run_idx)` → (score 0-1, rationale). OpenAI client with `temp=0`, `seed=run_idx`, `max_tokens=4000`, `latency_budget_ms=30000`. Prompts from `judge_prompts/{metric}.txt`. **Never accepts `presence_penalty` from caller** — explicit assertion.
4. Unit tests — mock OpenAI for determinism; variance math against fixture; sampling policy covers all 17 specs.

**Gate:** unit suite green; judge determinism verified (same prompt+seed → bit-identical logged response).

### Epic 2 — Benchmark Runner

1. CLI: `python -m neural.benchmarks.run_benchmark --model .local-models/qwen3-14b-mdemg-v1 --config configs/benchmark_phase10.yaml --golden training_data/eval/valid_golden.jsonl --out training_data/eval/benchmark_qwen3_14b_v1_baseline.json`.
2. For each 17 ULTS specs × N runs: resolve sampling policy → invoke MLX :8101 → `reward_functions.compute_reward()` for spec's reward list → judge for coherence/depth/relevance/naturalness `quality_metrics` entries → row in `benchmark_results`.
3. Aggregate: mean/stddev per task, group weighted sum for aggregate, compare last 3 benchmarks for stagnation. Write `benchmark_runs` aggregate row.
4. Emit JSON report with SHAs, per-task pass/fail, aggregate weighted score, stagnation verdict.

**Gate:** integration test (mocked MLX + mocked judge, 3-task subset) passes; 85 rows (17×5) in `benchmark_results`; aggregate weighted score matches group weighted sum within 1e-6.

### Epic 3 — TSDB V0012 Migration

1. `V0012__benchmark_results.sql` — create `benchmark_results` (columns: run_id CUIDv2, task_id, run_idx, sampling_group, response_text, reward_vector JSONB, judge_scores JSONB, adapter_sha, base_sha, dataset_sha, recorded_at TIMESTAMPTZ); `benchmark_runs` (run_id PK, model_path, started_at, completed_at, aggregate_weighted_score, stagnation_flag, config_sha).
2. Hypertable on `recorded_at`.
3. Up + down migrations; `mdemg tsdb migrate --dry-run` green; additive only.

**Gate:** V0011 → V0012 forward + V0012 → V0011 reverse both green on test DB; `llm_interactions` + `training_metrics` untouched.

### Epic 4 — Refactor `evaluate_ft.py` (Shadow Mode)

1. Add `--scorer={heuristic,registry,dual}` flag. Default `heuristic` (bit-identical Phase 5 path).
2. `registry` path calls `reward_functions.compute_reward()` instead of 149-407 hardcoded heuristics.
3. `dual` path runs both in parallel, writes both reports, asserts per-task `|delta| < 1%`. Any divergence > 1% → exit 2 with diff.
4. Replay Phase 5 dev set with `--scorer=dual`; save `shadow_run_report.json`. If all-green, mark `registry` path production-ready in docs (do NOT change default in this sprint — default flip deferred to Phase 11 after ≥3 benchmark rounds confirm parity).

**Gate:** shadow-run report shows `|delta| < 1%` per task across all 17; heuristic path bit-identical; `--scorer` default stays `heuristic`.

### Epic 5 — Baseline Capture + Grafana

1. Run full Phase 10 benchmark against `.local-models/qwen3-14b-mdemg-v1` — authoritative Phase 10 baseline (supersedes Phase 5 regression for future comparisons).
2. Update Grafana: 3 new panels — per-task pass-rate table, per-task reward variance (stddev), stagnation flag indicator. Reads V0012 tables.
3. `bash scripts/tsdb_spot_check.sh` — confirm 85+ rows in `benchmark_results`, 1 in `benchmark_runs`.

**Gate:** baseline JSON exists; `stagnation_flag=false` (first benchmark); Grafana JSON committable; TSDB spot-check green.

### Epic 6 — Testing (3 Tiers)

**Tier 1 (Unit):** `pytest -xvs neural/benchmarks/tests/test_variance.py test_sampling_policy.py test_llm_judge.py test_preflight.py`.

**Tier 2 (Integration):**
- `test_run_benchmark_mocked.py` — runner against mocked MLX + mocked judge, 3-task × N=2 subset; DB rows, aggregate math, stagnation logic.
- `test_evaluate_ft_shadow.py` — dual-scorer on fixture; delta assertion.

**Tier 3 (E2E):**
- T-subset smoke test (7 tasks, budget-conscious) against real MLX + real `gpt-5.4-mini` before full baseline.
- Full 17-task × 5-run for Epic 5 baseline.
- TSDB spot-check.

**Gate:** all three tiers green.

### Epic 7 — Documentation (Final Epic — Never Cut)

1. Copy plan → `docs/development/ft-lora/sprint_plan_ft_lora_phase10.md`.
2. Author `phase_10_benchmark_post.md` — executed-truth doc: baseline scores, judge-spend actual, variance summary, stagnation formula verified, shadow-run delta table.
3. `00_README_v2.md` v5.6 → v5.7: Phase 10 EXECUTED; link runner + post report.
4. `03_IMPLEMENTATION_PLAN_v2.md §Phase 10`: mark EXECUTED with SHA stamps; note deferred items.
5. `AGENT_HANDOFF.md` top entry: Phase 10 complete; Phase 11 GRPO unblocked.
6. `CHANGELOG.md [Unreleased] ### Added`: benchmark module, V0012 migration, LLM judge, shadow refactor, golden holdout, baseline, Grafana panels.
7. **`CLAUDE.md` cleanup** — remove stale `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` references in Testing section; replace with `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml`.

**Gate:** all docs in repo; cross-refs valid; `grep -r run_benchmark_v4\|test_questions_120 docs/ CLAUDE.md` returns zero stale hits.

## 6. Testing Plan (Three Tiers)

Covered in Epic 6. **State restoration (MEMORY):** all changes additive. Rollback = revert commit; `rm -rf neural/benchmarks/ training_data/eval/benchmark_*.json training_data/eval/valid_golden.jsonl configs/benchmark_phase10.yaml`; `mdemg tsdb migrate --target V0011`; revert `evaluate_ft.py` changes.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Sprint FT-LORA-PHASE10 — automated benchmark framework (LLM-judge + variance + shadow refactor)`
- Body: MVP scope summary, new module tree, V0012 migration note, baseline score table (17 tasks × group), shadow-run delta table (evaluate_ft.py bit-compatibility proof), judge spend actual vs budget, policy compliance (CUIDv2, no hardcoded values, sequential epics, 3-tier testing, docs complete).
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: preflight green; config loads; golden holdout SHA recorded
- [ ] Epic 1: variance/sampling/judge unit tests green; judge determinism verified
- [ ] Epic 2: runner integration green; 85-row mocked DB insert verified
- [ ] Epic 3: V0012 up+down clean; hypertable on `recorded_at`
- [ ] Epic 4: shadow-run `|delta| < 1%` per task; heuristic bit-identical; default still `heuristic`
- [ ] Epic 5: Phase 10 baseline captured; Grafana panels render; TSDB spot-check green
- [ ] Epic 6: all 3 test tiers green; T-subset smoke before full baseline
- [ ] Epic 7: sprint plan + post report + v5.7 + §Phase 10 EXECUTED + AGENT_HANDOFF + CHANGELOG + CLAUDE.md cleanup (grep verified)
- [ ] Commit pushed; auto-PR opened; **sprint summary posted to PR immediately**
- [ ] OpenAI spend logged, under $100 cap

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7: `sprint_plan_ft_lora_phase10.md`, `phase_10_benchmark_post.md`, `00_README_v2.md` v5.6→v5.7, `03_IMPLEMENTATION_PLAN_v2.md §Phase 10` EXECUTED, `AGENT_HANDOFF.md` prepended, `CHANGELOG.md [Unreleased] ### Added`, `CLAUDE.md` cleanup.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| **evaluate_ft.py refactor breaks Phase 5 regression parity** | Medium-High | Shadow-mode `--scorer=dual` runs both paths in parallel; per-task `\|delta\| < 1%` gate; default stays `heuristic`; flip only in Phase 11 after ≥3 benchmark rounds confirm parity | Revert flag; keep 149-407 heuristic path; defer registry unification to Phase 11 |
| **LLM judge non-determinism / drift** | Medium | `temp=0` + explicit `seed=run_idx`; judge model + seed + prompt SHA + raw response logged per row — full replay possible | Fall back to deterministic heuristics for coherence/depth/relevance; gate judge behind `--enable-judge` flag |
| **J-group `presence_penalty=1.5` leaks into judge sampling** | Low (addressed) | Judge uses own fixed sampling kwargs; never inherits task `inference.sampling`; explicit assertion in `llm_judge.py`; unit-tested | N/A (preventive) |
| **OpenAI budget exceeded** | Low | Hard $100 cap (MEMORY); est. $2-5/benchmark × 10-15 benchmarks ≈ $25-75 | Swap judge to local Qwen3-14B for cheaper rounds |
| **V0012 migration conflicts with V0011 rows** | Low | Migration is additive — creates new tables; no ALTER | `mdemg tsdb migrate --target V0011` cleanly rolls back |
| **N=5 too few for GRPO advantage normalization** | Medium | N is config-driven; bump to 10 if Phase 11 GRPO shows instability; exit criterion checks stddev is non-zero on all 17 | Re-run benchmark with higher N; Phase 11 reads latest row — no schema churn |
| **Stagnation false positive on early benchmarks** | Low | Stagnation flag informational in Phase 10; only blocks in Phase 11 when `benchmark_runs.count() >= 3` | Hard-gate Phase 11 GRPO entry on history count |
| **ULTS spec missing `sampling_group`** | Low | Preflight aborts with per-spec diff; no fallback — fix the spec | N/A (forcing function) |
| **MLX server crash mid-benchmark** | Low-Medium | Preflight `ps` check; per-task retry (1x, then skip); partial results written with `stop_reason=interrupted` | Restart MLX; resume from last `run_idx` via CLI flag |
| **CUIDv2 Python library not approved** | Low | Primary: `cuid2` PyPI package. Fallback: `time.time_ns()` + blake2b hash with sort-stability | User confirms at plan exit |
| **Grafana panel perf as `benchmark_results` grows** | Low | Hypertable on `recorded_at`; continuous aggregates deferred to Phase 11 | Add continuous aggregate when row count exceeds ~10K |

## 11. Documents Accessed (during planning)

**Read during planning:**
- `/Users/reh3376/mdemg/docs/development/ft-lora/04_BENCHMARK_RL_v2.md` §10 lines 39-261 (sampling recipes, task registry, new file list)
- `/Users/reh3376/mdemg/docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` §Phase 10 + §5E
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` v5.6 Phase 5 completion block
- `/Users/reh3376/mdemg/neural/training/evaluate_ft.py` — 843 lines; hardcoded evaluators 149-407 (refactor target)
- `/Users/reh3376/mdemg/neural/training/regression_gate.py` — 294 lines; verdict 31-34
- `/Users/reh3376/mdemg/neural/training/reward_functions.py` — 468 lines; `REWARD_REGISTRY` 416-443; `compute_reward()` 451
- `/Users/reh3376/mdemg/neural/training/tests/test_reward_functions.py` — 285 lines
- `/Users/reh3376/mdemg/docs/tests/ults/specs/*.ults.json` — 17 specs (field completeness)
- `/Users/reh3376/mdemg/internal/storage/tsdb/migrations/` — V0011 current head (confirmed)
- `/Users/reh3376/mdemg/CLAUDE.md` — stale benchmark references to clean up
- `/Users/reh3376/mdemg/adapters/tier1/manifest.json` — Phase 5 SHAs for V0012 stamping
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_5_sft_post.md` — Phase 5 dual regression gate context
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `project_phase5_moe_pivot.md`

## 12. Rollback

All changes additive.

1. `git revert <final commit SHA>`.
2. `rm -rf neural/benchmarks/ training_data/eval/benchmark_*.json training_data/eval/valid_golden.jsonl configs/benchmark_phase10.yaml`.
3. `mdemg tsdb migrate --target V0011` (reverse V0012; drops `benchmark_results` + `benchmark_runs` only).
4. Revert `evaluate_ft.py` `--scorer` flag (heuristic path bit-identical — no data corruption risk).
5. Revert Grafana JSON to Phase 5 version.

Phase 5 model + Phase 5 regression path untouched throughout. No Neo4j writes. TSDB V0012 rows dropped by reverse migration (auditable beforehand).

---

## Post-Sprint — Phase 11 (GRPO/DPO) Unblocks

On merge, Phase 11 planning begins. Phase 11 consumes:
- `benchmark_results.reward_vector` per (task, run_idx) → GRPO advantage estimator
- `benchmark_results.stddev` → advantage normalization
- Stagnation signal → automatic Phase 11 exit criterion
- `evaluate_ft.py --scorer=registry` (production-ready after shadow validation) — Phase 11 flips default

Phase 10 is intentionally MVP: scheduler, CLI wiring, launchd automation, and nightly runs are deferred because Phase 11 only needs on-demand, reproducible reward signal — which this sprint delivers. The deferred pieces become Phase 11 operational scaffolding, not Phase 11 blockers.
