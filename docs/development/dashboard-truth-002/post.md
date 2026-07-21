# DASHBOARD-TRUTH-002 — Sprint Post

**Shipped:** 2026-07-20 → 2026-07-21 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Second wave of dashboard/measurement artifact fixes following DASHBOARD-TRUTH-001. Nine artifacts closed across the RSIC / J17 / Jiminy / FT-Training dashboards. Same forensic family: wrong-anchor thresholds, missing no-data gates, hardcoded values calibrated to unrealistic assumptions, multi-row gauge panels with `lastNotNull`, single-element aggregates, panels reading transition-logs instead of continuous gauges, and vestigial reward keys.

## Epics

- **E0** — Sprint plan (v1.0 12-section). Commit `40d55fa` (bundled with 4 sibling sprints).
- **E1+E2+E3** — RSIC dimension scoring formula fixes batched together (same file, same cluster). Commit `6648fd1`.
  - E1: `scoreRetrieval` `saturated=0.7 → 0.9` (was penalising graph maturity).
  - E2: `scoreEdge` hardcoded 0.5 entropy floor → config `RSIC_EDGE_ENTROPY_FLOOR` (default 0.2, live-calibrated). ⚠️ Triage agent misdiagnosed the mechanism (claimed structural fixed-weight edges collapsed entropy — actually CO_ACTIVATED_WITH evidence-count long-tail); conclusion stood, mechanism corrected in code comment.
  - E3: `scoreProtocol` applies DH-004 "no data = neutral" gate to `restoreScore` when `TicketRestoreTotal=0`; `J17_COMPRESSION_TARGET_RATIO` default 3.0→2.0 (30d live p95 is 2.0).
- **E4-E7 + E9-E10** — 6 Grafana JSON panel edits batched. Commit `d5fe29e`.
  - E4: Retrieval Pipeline Health — `gauge/lastNotNull` on 3-row UNION → `bargauge` horizontal (all 3 stages visible).
  - E5: New Cycle Admission Ratio companion panel (normalises RSIC-STORM-001's on-spec aggressive rejection rate).
  - E6: J17 Min/Max Trust — SQL now gates on `trust_session_count>=2` via CTE → renders N/A when N=1.
  - E7: Jiminy Constraint Eff. All-Time + Selected unified to `constraint_outcomes` with matched `classifier_source` filter.
  - E9: LLM endpoint state — read `mdemg_mlx_health_state` continuous gauge from `metric_samples` (was reading transition-log, empty during steady-state UP).
  - E10: Recent watchdog events — drop `$__timeFilter` (rare-event stream).
- **E8** — hidden.reclassify vestigial reward key. NOT a live writer bug — grep-verified `compute_reward` already respects `spec.reward_functions`. Ships regression pin `test_compute_reward_output_keys_are_exactly_spec`. The stale 0.5 display corrects when FT-BENCH-REFRESH-001 runs a fresh benchmark. Commit `53163a7`.
- **Live Tier-3** — commit `7d299e3`. Full evidence in `live_verification.md`.
- **E11** — Canonical docs. This commit.

## Live evidence

Direct API `/v1/self-improve/assess` on `mdemg-dev`:
```
LearningPhase:       saturated
EdgeCount:           125,289
EdgeWeightEntropy:   0.266    (> new 0.2 floor → no penalty)
EdgesBelowThreshold: 25,722   (ratio 0.205 < 0.3 → no penalty)
OverallHealth:       0.803
```

Gauge deltas (mdemg-dev pre/post-restart with new code):
```
mdemg_rsic_health_retrieval  0.700 → 0.900   ✓ E1 (+0.20)
mdemg_rsic_health_edge       0.800 → 1.000   ✓ E2 (+0.20)
mdemg_rsic_health_protocol   0.716 → 0.338   ← transient (J17 stats fresh-restart-zero'd)
mdemg_j17_compression_ratio  1.555          (compression sub-score 0.278→0.555 with new target 2.0)
mdemg_rsic_health_overall    ~0.75 → 0.803
```

Protocol dimension lands at ~0.85-0.90 once J17 comprehension/coverage rebuild post-restart. See `live_verification.md` for the arithmetic.

## What I did NOT do

- **Full rewrite of `scoreRetrieval` to read real signals** (rerank fill-rate + uvts_runs recent mean). Deferred per plan's "ship the minimum first" guidance. Filed as future work in the CLAUDE.md note.
- **Panel-side reward-vector key filter** for E8. Not needed — writer path already correct. Regression pin protects going forward.
- **`/metrics` HTTP scrape endpoint** — that's PROMETHEUS-SCRAPE-INVESTIGATION-001's scope.
- **JIMINY-CORPUS follow-rate lift work** — already active line; not this sprint.
- **Fresh benchmark run** — that's FT-BENCH-REFRESH-001.

## Deviations

None substantive. Committed E1+E2+E3 as one commit instead of three (same file cluster; atomic reversibility) — noted in the commit message.

## Test / lint

- `go test ./internal/ape/` — all TestScoreRetrieval / TestScoreEdge / TestScoreProtocol subtests pass after formula + default-value updates.
- `go test ./internal/config/` — J17_COMPRESSION_TARGET_RATIO tests pass with new 2.0 default.
- `go test ./internal/grafanapin/` — pin test still green (164 panels scanned).
- `python3 -m pytest neural/training/tests/test_reward_functions.py` — 88 passing, new E8 pin included.
- `go test ./...` — clean.
- `golangci-lint run ./...` — 0 issues.
- All 8 dashboard JSONs pass schema validation.

## Rollback

- **Data**: none of these are destructive.
- **Code**: revert per-commit; formula defaults can also be overridden via env (`RSIC_EDGE_ENTROPY_FLOOR=0.5`, `J17_COMPRESSION_TARGET_RATIO=3.0`) without a code change.
- **Grafana**: dashboard JSON reverts on next commit; bind-mounted container reloads.

## Next up

Per DASHBOARD-TRUTH-002 sweep queue: **JIMINY-ACTIONABILITY-INVERSION-001** (investigation — advisory guidance followed 1.4-2.4× more than actionable), then **FT-BENCH-REFRESH-001** (re-run benchmark on GGUF endpoint, wire staleness detection), then **PROMETHEUS-SCRAPE-INVESTIGATION-001** (diagnose /metrics HTTP 404).
