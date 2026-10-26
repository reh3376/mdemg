# U8 — Learning / Training Data (7 slides)

**Intent**: `/ui/ Learning` + `/ui/ Training Data` tabs + the shipped FT line + the recursive-retrain loop (Phase 6a/6b/7/9). Operators here see what MDEMG has learned, what's in the pipeline, when the next retrain fires, and the current production model's honest aggregate.
**Target slides**: 7
**Mermaid**: 0 (tables + CLI examples carry it)

---

## U8.1 — Section Landing

**Title**: `U8 · Learning + Training Data — How The Model Gets Better`

**Body**:
- Intent: "MDEMG's production LLM is `mdemg-llm-v1` (Qwen3-14B-4bit + Phase 5 SFT), aggregate 0.9188 on the honest 16-task eval. The retrain pipeline (Phase 6b actuator SHIPPED default-off) can produce the next generation when triggered."
- Preview: FT line state → Learning tab → Training Data tab → recursive-retrain loop → UBENCH freshness → gotchas
- Points back to U1.5 "How does it continuously improve?" — this is loop #4 (model-level improvement; the slowest and most expensive loop)

**Provenance**:
- CLAUDE.md "MDEMG Fine-Tuning — Shipped State & Policies"
- CLAUDE.md "Recursive-retrain loop Phase 6a/6b/7/9" (FT-RECURSIVE-001-004)

---

## U8.2 — The Shipped FT Line (Production)

**Title**: `What's Running — mdemg-llm-v1`

**Body**:
- Production model: `mdemg-llm-v1`
- Base: `mlx-community/Qwen3-14B-4bit` (dense, after the 2026-04-22 MoE→dense pivot)
- Training: Phase 5 SFT single-tier LoRA (rank=32 α=64, 7 dense target modules)
- Serving: GGUF Q5_K_M via llama-server at `:8102` (Phase 13.5 cutover from mlx_lm.server)
- Honest aggregate: **0.9188** on augmented eval (16 tasks; APE-REFLECT-EVAL-REFRESH-001 refreshed truncation-poisoned rows 2026-07-21)
- Comparison: beats gpt-5.4-mini (0.8317) by +0.087; beats Phase 5 base (0.8338) by +0.085

**Provenance**:
- CLAUDE.md "FT line SHIPPED through production cutover" section
- CLAUDE.md "APE-REFLECT-EVAL-REFRESH-001"

**note**:
- Operator cue: "The 0.9188 is the HONEST number. Previous iterations claimed higher via leaked eval; the clean eval reset the baseline."

---

## U8.3 — /ui/ Learning Tab

**Title**: `The Learning Tab`

**Body**:
- Where: `/ui/` → **Learning**
- Shows: current adapter info, Phase 5 SFT config, scheduled benchmark state, FT loop status (if `FT_LOOP_ENABLED` on), class-5 CapabilityGap issues from the filer
- Interaction: read-only; drills into benchmark run history, retrain cycle events, drift observations
- Related: Grafana `mdemg-ft-training` dashboard carries the historical panels

**Provenance**:
- CLAUDE.md "FT-RECURSIVE-003" (Phase 7 promotion + CapabilityGap filer)
- `internal/api/ui/tabs/learning.js`
- `deploy/docker/grafana/dashboards/mdemg-ft-training.json`

**note**:
- Operator cue: "This tab answers 'what's the model doing right now, and what's queued?'"

---

## U8.4 — /ui/ Training Data Tab

**Title**: `The Training Data Tab`

**Body**:
- Where: `/ui/` → **Training Data**
- Shows: UAITS-curated datasets (SFT/DPO/RAFT/curriculum paradigms), export job state, retention schedule, scrub status
- Interaction:
  - Trigger `mdemg data export-auto` runs
  - View dataset manifests (counts, SHAs, lineage)
  - Inspect UBENCH latest run aggregate
- Related CLI: `mdemg data curate` (UAITS spec-driven), `mdemg data export` (archive), `mdemg data clean --space-id` (TSDB scrub)

**Provenance**:
- CLAUDE.md "UAITS" + "UTDS" + "Beta corpus loop (BETA-IMPORT-001)"
- `internal/api/ui/tabs/training_data.js`

**note**:
- Operator cue: "If you're a beta tester, this tab is where YOUR exported contribution gets inspected on the maintainer side (via `mdemg beta-import`)."

---

## U8.5 — The Recursive-Retrain Loop (Phases 6a → 7 → 9)

**Title**: `When MDEMG Retrains Itself`

**Body**:
- Phase 6a (SHIPPED, FT-RECURSIVE-001): manual-path observability — operator runs each stage, each stage records to `scheduled_job_events`
- Phase 6b (SHIPPED default-off, FT-RECURSIVE-002): actuator — `FT_LOOP_ENABLED=true` opts in; controller walks export→curate→train→convert→gate as supervised subprocesses
- Phase 7 (SHIPPED, FT-RECURSIVE-003): promotion is REAL + three-layer defended — pre-swap structural canary → fail-closed `SwapServing` → post-swap tripwire auto-rollback
- Phase 9 (SHIPPED, FT-RECURSIVE-004): drift-triggered retrain — `ft_production_drift` + `ft_loop_never_ran` rules fire RSIC `trigger_training_pipeline`
- Trigger sources (2): FT-RECURSIVE-001 training_data_ready (DATA readiness) + DRIFT-TRIGGER-001 production_drift_detected (MODEL quality drift)
- Operator ceremony: `mdemg ft-loop promote` is the final human confirm (unless `FT_LOOP_AUTO_PROMOTE_AFTER` is non-zero)

**Provenance**:
- CLAUDE.md "Recursive-retrain loop Phase 6a/6b/7/9" (FT-RECURSIVE-{001,002,003,004})
- CLAUDE.md "DRIFT-TRIGGER-001"
- `docs/features/ft-recursive-loop.md`

**note**:
- Operator cue: "Default: the loop is OFF. Flip `FT_LOOP_ENABLED=true` + `_AUTO_PROMOTE_AFTER=0` to opt into observing cycles without auto-promoting."

---

## U8.6 — UBENCH Freshness

**Title**: `UBENCH — Keep The Benchmark Fresh`

**Body**:
- `benchmark_runs` TSDB hypertable: one row per aggregate run (CSV: run_id, started_at, completed_at, aggregate, model_name)
- FT-BENCH-REFRESH-001 (2026-07-21) alert rule: `ft_benchmark_stale` fires HIGH if newest row > 7d old (direct `benchmark_runs` read; staleness distinguishes "never ran" from "stale" per ALERT-NOISE-REDUCTION-001 E2)
- Freshness panel on `mdemg-ft-training` dashboard: "Latest Benchmark Age" (green <3d, yellow 3-7d, red >7d)
- Scheduled benchmark runner: `FT_BENCH_SCHEDULE_*` env knobs; weekly in dev `.env`; jobhealth `scheduled-ft-benchmark`
- CLI: `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml --out training_data/eval/<name>.json --apply-tsdb`
- ⚠️ requires `psycopg` — pinned in `neural/pyproject.toml [training]`; run via `neural/.venv/bin/python`

**Provenance**:
- CLAUDE.md "FT benchmark freshness (FT-BENCH-REFRESH-001)" + "BENCH-SIDECAR-APPLY-001"

**note**:
- Operator cue: "The current `FT Benchmark Not Refreshed` HIGH alert on your session fires because the last run was > 7d ago. Run the scheduled benchmark or `mdemg ft-bench run` to clear it (if that CLI shipped — otherwise the python command above)."

---

## U8.7 — Gotchas + Where To Go Deeper

**Title**: `Learning — Gotchas + More`

**Body**:
- Gotchas:
  - MoE→dense pivot (2026-04-22): Metal 499K cap is ARCHITECTURAL (not quant-specific); DO NOT propose MoE-Sieve work against the current spec
  - Phase 13.5 moved serving from mlx_lm.server to llama-server (port 8101 decommissioned, port 8102 is production); DO NOT reference the old port
  - KV slot bound (`--ctx-size × --parallel`): prompt + output share the slot — bound reflection prompts via `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS` (APE-PROMPT-BUDGET-001)
  - FT_LOOP_ENABLED default is FALSE — the loop is dormant until operator opts in; staying dormant is a FEATURE not a bug
- Feature docs:
  - `docs/features/ft-recursive-loop.md` — the master doc for Phases 6-9
  - `docs/features/ft-benchmark-freshness.md`
  - `docs/features/rsic-reflection-prompt-budget.md`
- CLAUDE.md pins: all FT-RECURSIVE-* sprints, PHASE-E1/E2/E3 corpus audits, HOMEBREW-INSTALLER-QWEN-UPDATE-001
- Where this feeds into: U13 Grafana (benchmark + FT loop dashboards), U15 APIs (recursive-retrain gRPC trigger surface), U16 Alerts (FT benchmark staleness + loop never-ran rules)

**note**:
- Operator cue: "If you're thinking about model updates, read `docs/features/ft-recursive-loop.md` cover-to-cover first. The three-layer promotion defense is the piece most newcomers underestimate."
- Transition: "U9 next — Backup + Ops, the day-to-day maintenance surfaces."
