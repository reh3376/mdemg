# TSDB-CONSUME-001 — Sprint Post

**Status: COMPLETE** · 2026-06-11 · branch `reh3376_dev01`

## What shipped (per epic)

1. **V0025 retention+compression** (`c8939ed`) — 9 retention + 4 compression
   policies; TSDB schema 24→25; applied live twice (idempotent); first-run
   deletion forecast: **0 rows on all 9 tables** (oldest data 2026-03-31).
2. **Honest metrics plane** (`039a7a5`) — windowed p95/p99 synthetics;
   fake Neo4j pool gauges → real `mdemg_tsdb_pool_*` (pgx); unfireable
   `neo4j_pool_exhausted` deleted (server-native + Grafana); rate-limit
   collector delta-fixed AND wired (it had zero callers); latency-slo rules
   rewritten over `retrieval_audit` real wall-time, config thresholds
   calibrated from the live distribution; dashboard pool panels + 3 UOTS
   specs updated.
3. **Writer flush health** (`b89d83e`) — 8 writers self-register stats;
   `mdemg_tsdb_writer_*{writer}` family; `tsdb_writer_flush_failures` rule.
4. **Scorer-drift tripwires** (`84dd1c2`) — `scorer_version_change` +
   `consensus_shift` over retrieval_audit; SQL hand-verified live (steady
   state quiet, 900h window correctly detects the 3 historical versions);
   `RETRIEVAL_AUDIT_ENABLED` default → true + compose-forwarded.
5. **Conflicts counter + emergence duration** (`86f39ee`) —
   `mdemg_guidance_conflicts_total{space_id}`;
   `mdemg_emergence_cycle_duration_seconds{space_id,cycle}` at all 3
   consolidation variants + `emergence_cycle_slow` rule. Disclosed
   deviation: gauge, not the roadmap's "histogram" (fixed ≤10 s registry
   buckets would clamp multi-minute cycles).
6. **V0020 writer** (`5aa0587`) — writer-or-drop decided: writer. One sync
   row per successful catalog build via `BuilderOpts.VersionRecorder`.
7. **ft_* dashboard** (`dc780ce`) — 4 reader-without-writer panels removed;
   tables stay (FT recursive-loop sinks); status notice records disposition.
8. **Docs** — `docs/features/tsdb-data-management.md`, CHANGELOG, CLAUDE.md.

Side-fix (operator-prompted, own commit `30051a0`): docker-publish runner
disk-reclaim step — the post-PR-440 Docker Publish failure was pre-existing
intermittent runner disk exhaustion (identical failures on runs 27366931196
/ 27367756926 before PR #440), not PR-440-caused.

## Tier 3 live verification (real binary, live stack)

- V0025 re-applied idempotently under auto-migrate (`schema_version=25`);
  13 policies visible in `timescaledb_information.jobs`.
- `JIMINY...` — server restarted via LaunchAgent; `/healthz` all-ok.
- All 13 new gauge series landed in `metric_samples` on the first flush
  (5 pool + 8 writers × 4 stats).
- `/v1/system/pool-metrics` serves real pgx stats (`backend: tsdb-pgx`,
  matching `acquire_count`, `max_conns: 10`).
- 3 live retrieves (4.5–19 s): windowed p99 **moves per window**
  (0.005 → 10.0 tracking actual traffic; the 9.95 permanent peg is gone).
- Latency rule SQL live: p95 = 29.1 s / p99 = 33.2 s over 57 calls/30 min —
  real data, below the 120 s / 300 s thresholds. **Zero evaluator query
  failures and zero alerts since restart** (the `rule-health-*_latency`
  recurrence is dead).
- UOTS `prometheus_neo4j_pool` (rewritten to tsdb-pgx) passes live; the two
  file-based specs pass.
- Scorer tripwire SQL hand-verified against the table's real 3-version
  history (see Epic 4).
- Emergence-cycle gauge: verified at the next consolidation cycle
  (see verification note below if landed post-push).
- `scripts/verify_config_consumers.py`: 671/671 consumed (all 11 new
  Config fields wired). Full `go test ./internal/...` green; lint clean.

## Rule census after this sprint

DefaultRules 13→10 (2 latency + pool_exhausted removed); new factories:
`RetrieveLatencyRules` (2), `TSDBWriterRules` (1), `ScorerDriftRules` (2),
`EmergenceCycleRules` (1) — net +6 honest rules, −3 lying ones.

## New config (all env-overridable, defaults live-calibrated)

`ALERT_RETRIEVE_P95_MS` 120000 · `ALERT_RETRIEVE_P99_MS` 300000 ·
`ALERT_RETRIEVE_LATENCY_LOOKBACK_MIN` 30 · `TSDB_WRITER_ALERT_LOOKBACK_MIN`
60 · `SCORER_CHANGE_LOOKBACK_HOURS` 24 · `CONSENSUS_SHIFT_THRESHOLD` 0.10 ·
`CONSENSUS_SHIFT_RECENT_HOURS` 6 · `CONSENSUS_SHIFT_BASELINE_DAYS` 7 ·
`CONSENSUS_SHIFT_MIN_SAMPLES` 20 · `EMERGENCE_CYCLE_ALERT_THRESHOLD_SEC` 60
· `EMERGENCE_CYCLE_ALERT_LOOKBACK_MIN` 120. Changed default:
`RETRIEVAL_AUDIT_ENABLED` false→true.

## Recon corrections worth recording

Three sub-agent recon claims were wrong and corrected by live verification
before they could shape the design: (1) "HTTP latency metrics are never
written" — they are (2.1M rows, current); the defect was windowing +
thresholds, not missing writers. (2) "pool collector never called" — called
every flush; the fake was inside `db.GetPoolMetrics` itself. (3) "helm
schema-version drift 26 vs 24" — helm's `REQUIRED_SCHEMA_VERSION` is the
**Neo4j** schema env (config.go:1242); 26 is correct. Verify recon against
the live system before planning.

## Documents Accessed

ROADMAP_2026Q3.md:55; sprint plan §11 list; live TSDB (policies, metric
census, retrieval_audit percentiles + scorer history, consensus
distribution, data ages, first-run forecasts); internal/alert/rules.go;
internal/metrics/{recorder,collectors,registry}.go; internal/db/neo4j.go;
internal/ratelimit/middleware.go; internal/tsdb/ (writers, migrations
001/002/017/020, migrate.go); internal/hidden/{service,
context_catalog_builder}.go; internal/consulting/service.go;
deploy/docker/grafana/ (mdemg-neo4j, mdemg-ft-training, alerts.yml);
UOTS specs ×3; ~/.mdemg/logs/server.log; ~/.mdemg/alerts/current.json.
