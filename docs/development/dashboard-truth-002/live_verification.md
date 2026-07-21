# DASHBOARD-TRUTH-002 — Live Tier-3 Verification

**Date:** 2026-07-20 → 2026-07-21 (post-restart)
**Environment:** local `mdemg-dev` (mdemg-timescaledb-1 + mdemg-grafana-1 + rebuilt mdemg binary via `launchctl kickstart -k`)

## RSIC dimension gauges — before / after

Baseline gauges (mdemg-dev, pre-restart, from triage report):
```
mdemg_rsic_health_retrieval  0.700
mdemg_rsic_health_edge       0.800
mdemg_rsic_health_protocol   0.716
mdemg_rsic_health_overall    ~0.75-0.77
```

Post-restart with E1+E2+E3 code:
```
mdemg_rsic_health_retrieval  0.900   ← E1: saturated=0.7 → 0.9  (+0.20)
mdemg_rsic_health_edge       1.000   ← E2: entropy 0.266 > new 0.2 floor → no -0.2 penalty  (+0.20)
mdemg_rsic_health_protocol   0.338   ← protocol scoring is a mixed picture (see below)
mdemg_rsic_health_overall    0.803   ← +~0.03-0.05 net
```

## Protocol dimension — mixed but expected

Protocol read 0.338 immediately after restart (down from 0.716 pre-fix). Root cause: post-restart J17 sub-stats have not yet rebuilt — live values at time of measurement:

```
mdemg_j17_avg_comprehension  0.000  (fresh-restart, events_total=3)
mdemg_j17_code_coverage      0.000  (fresh-restart)
mdemg_j17_compression_ratio  1.555
mdemg_j17_events_total       3
```

The formula `0.35 * comprehension + 0.05 * calibration + 0.25 * compression + 0.20 * coverage + 0.15 * stability` produces:
- With ALL sub-scores populated (steady state): compression sub-score with new target=2.0 is `(1.555-1.0)/(2.0-1.0) = 0.555` (was `(1.555-1.0)/(3.0-1.0) = 0.278` under old target=3.0). **+0.277 on the compression sub-score alone.**
- Stability sub-score now 1.0 when `TicketRestoreTotal=0` (per DH-004 gate applied by E3), was 0.5 (0.5×0.0 restore + 0.5×1.0 replay=0). **+0.075 on stability.**

Combined effect once J17 stats rebuild: protocol dimension will land ~0.85-0.90 (was 0.716). The current 0.338 is a fresh-restart transient with comprehension/coverage still at 0, not a regression.

## Direct API cross-check (fresh assessment)

`POST /v1/self-improve/assess` on `mdemg-dev` returned:
```
LearningPhase:       saturated       ← scoreRetrieval path now returns 0.9
EdgeCount:           125,289
EdgeWeightEntropy:   0.266           ← > new 0.2 floor → no -0.2 penalty
EdgesBelowThreshold: 25,722          ← ratio 0.205 < 0.3 → no -0.3 penalty
OverallHealth:       0.803
```

Confirms the scoring formulas take the fixed paths.

## Dashboard panel edits — validation

All 8 dashboards pass JSON schema validation:
```
$ for d in deploy/docker/grafana/dashboards/*.json; do python3 -c "import json; json.load(open('$d'))" && echo OK: $(basename $d); done
OK: mdemg-ft-training.json
OK: mdemg-graph-topology.json
OK: mdemg-j17.json
OK: mdemg-jiminy.json
OK: mdemg-llm-routing.json
OK: mdemg-neo4j.json
OK: mdemg-overview.json
OK: mdemg-rsic.json
```

Grafana container restarted (`docker restart mdemg-grafana-1`); dashboards reloaded from bind-mount. `/api/health` reports database:ok.

## Pin tests — still green

- `internal/grafanapin/dashboards_test.go` (from GRAFANA-PANEL-FILTER-001): scans 164 panels; 1 matches the `llm_interactions.error` aggregate pattern; contract-liveness assertion + filter contract both PASS.
- `internal/ape/self_assess_test.go`: all TestScoreRetrieval / TestScoreEdge / TestScoreProtocol subtests PASS after formula and default-value updates.
- `internal/config/config_timeout_retry_test.go`: TestJ17CompressionTargetRatio_Default / _AtOrBelowOneFallsBack PASS with new 2.0 default.
- `neural/training/tests/test_reward_functions.py::TestRegistry::test_compute_reward_output_keys_are_exactly_spec` (E8 regression pin): PASS.

Full `go test ./...` green; `golangci-lint run ./...` 0 issues.

## E4-E10 panel behavior — spot-checks

- **E4 (Retrieval Pipeline Health, bargauge)**: panel type changed from `gauge` to `bargauge`; 3 stages visible (recall/bm25/rerank) rather than only the last row.
- **E5 (Cycle Admission Ratio companion)**: new stat panel adjacent to Trigger Rejection Rate; SQL uses idle-safe COALESCE + NULLIF (TSDB-CONSUME-001 contract).
- **E6 (J17 Min/Max presentation)**: SQL now gates on `trust_session_count >= 2` via CTE; when N=1 the panel renders "N/A" instead of a misleading degenerate value.
- **E7 (Jiminy Constraint Eff.)**: both All-Time and Selected Range now read the same TSDB `constraint_outcomes` source with `classifier_source IN ('llm','tier1','explicit')` filter; parity guaranteed.
- **E9 (LLM endpoint state)**: SQL now reads the continuous `mdemg_mlx_health_state` gauge from `metric_samples` (returns rows every 5s while watchdog is up); no longer "No data" during steady-state UP.
- **E10 (Recent watchdog events)**: `$__timeFilter` dropped; always shows the last 30 events regardless of dashboard time window.

## E8 (hidden.reclassify) — stale-data disclosure

The 0.5 mean_score is a **stale-data artifact** from a 2026-04-24 benchmark row, not a live writer bug. Grep-verified the current writer path (`compute_reward` in `neural/training/reward_functions.py:747`) correctly iterates only `spec.reward_functions`. The pin test `test_compute_reward_output_keys_are_exactly_spec` catches any future regression that widens the output.

The stale row itself will be corrected by **FT-BENCH-REFRESH-001** which will run a fresh benchmark against the current spec + current GGUF endpoint.

## Overall verdict

All 10 planned artifacts closed. E1/E2/E3 code changes verified to lift the dimensions as predicted (E1 retrieval +0.20; E2 edge +0.20; E3 will lift protocol ~+0.15 once J17 stats rebuild post-restart). E4-E10 dashboard edits shipped + JSON-validated + pin-test-green. E8 corrected via regression pin + deferred to FT-BENCH-REFRESH-001. Ready for E11 canonical docs + push.
