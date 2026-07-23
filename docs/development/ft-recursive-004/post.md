# FT-RECURSIVE-004 — Sprint Post

**Date:** 2026-07-23 (single evening) | **Branch:** `reh3376_dev01`
**Parent:** SPEC §3 Monitor / §4 Phase 9 + FT-RECURSIVE-003's disclosed
follow-ups. Several Phase-9 deliverables were pre-shipped by 003 (versions
writer; issue filer with dedupe-on-repeat — that half of the spec's exit
criteria was already proven).

## Verdict

**The recursive-retraining arc is COMPLETE.** Phases 6a (observability),
6b (actuator), 7 (promotion/canary/rollback/autonomy/escalation), and 9
(drift monitoring) are all shipped and live-proven. The spec's remaining
Phase-9 exit criterion — *"drift rule fires in a seeded drill"* — passed:
seeded active-score 0.99 → drift 0.0712 > 0.05 margin → fires.

## What shipped

- **E1 `ft_loop_never_ran`** — ledger staleness (`FT_LOOP_STALENESS_DAYS`
  14), wired only when `FT_LOOP_ENABLED`; live both branches (events →
  0.019d silent; empty → 999 fires).
- **E2 `ft_production_drift`** — active version score − latest benchmark
  aggregate, clamped ≥ 0, margin `FT_DRIFT_MARGIN` (0.05), HIGH; DH-004
  no-data gates both directions. Scalar reads via MAX(completed_at)
  correlation (no LIMIT-1 literal; pin sweeps green). **Baseline honesty
  fix**: the active `mdemg-llm-v1` row's score 0 → 0.8655
  (BASELINE-RECOMPUTE-001), noted on the row.
- **E3 filer sweep extension** — clusters from each cycle's LATEST event
  (DISTINCT ON), including `rolled_back` + `%_failed` stages. ⚠️ In-epic
  live catch: the first version read raw event rows, which resurrected the
  E7-neutralized drill cycles — event-sourced tables must be read at
  latest-status for terminal semantics (the ledger-semantics rule,
  now pinned in the sweep-shape test).
- **E4 scheduled benchmark runner** — supervised `scheduled-ft-benchmark`
  loop (default off): due-date math over `benchmark_runs`, the
  FT-BENCH-REFRESH-001 recipe with `--apply-tsdb`, jobhealth under the
  `ft-benchmark` service. **Live: fired autonomously after its delay, run
  `t9czjxhg` aggregate 0.9156 landed in 6m59s, jobhealth green.** Left
  enabled in dev `.env` at the weekly default (enable-after-smoke) —
  disclosed: ~7 min llama-server saturation weekly, and
  `ft_benchmark_stale` should now never fire organically.
- **E5 dashboard pairs** — Production Drift stat (rule-exact SQL,
  margin-matched thresholds), Model Versions table, latest-status Cycle
  Ledger. grafanapin green.

## Live evidence summary

| Check | Result |
|---|---|
| Drift no-data gate (score 0) | 0 ✓ |
| Drift SEEDED (0.99 active) | **0.0712 → fires** ✓ exit criterion |
| Drift honest baseline (0.8655 vs 0.9156) | 0 ✓ (model above baseline) |
| never_ran with events / empty | 0.019d silent / 999 fires ✓ |
| Scheduled run | autonomous, 0.9156, 6m59s, jobhealth ✓ |
| Sweep latest-status view | 3 real fingerprints ×1, neutralized cycles excluded ✓ |

## Follow-ups (disclosed)

- Per-task drift decomposition (aggregate-only today).
- The scheduler's fixed rows-per-spec refresh scope trades depth for cost;
  a monthly full run could complement it.
- Filer restart re-comment (carried from 003; unchanged).

## Documents Accessed

SPEC §3/§4; FT-RECURSIVE-003 post; FT-BENCH-REFRESH-001 +
BENCH-SIDECAR-APPLY-001 (recipe/flags); BASELINE-RECOMPUTE-001 (0.8655);
`internal/alert/rules.go` + pin tests; `internal/ftloop/{issue_filer,
bench_schedule}.go`; live `benchmark_runs`/`ft_model_versions`/ledger;
`deploy/docker/grafana/dashboards/mdemg-ft-training.json`.
