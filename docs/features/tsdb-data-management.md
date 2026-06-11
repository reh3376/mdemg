# TSDB Data Management & Telemetry-Plane Honesty (TSDB-CONSUME-001)

## Why

Before this sprint, half the telemetry plane was write-only and unbounded,
and several alert rules consumed data that did not mean what they claimed:

- Only `metric_samples` (V0001) and `llm_interactions` (V0002) had
  retention/compression. Everything since grew forever — `embedding_events`
  had reached **2.7 GB** live.
- The `latency-slo` rules read synthetic p95/p99 gauges computed from
  **lifetime-cumulative** histogram buckets. One slow call ever pegged the
  gauge permanently (live value: a constant 9.95 — the top-bucket clamp),
  firing a perpetual false CRITICAL; and their `ORDER BY time DESC LIMIT 1`
  SQL returned zero rows whenever the system was idle, producing the
  recurring `rule-health-*_latency` "no rows in result set" alerts.
- The `mdemg_neo4j_pool_*` gauges were fake by construction: the neo4j Go
  driver exposes no pool-stats API, so a probe loop counted
  `VerifyConnectivity` successes as "acquisitions" while Active/Idle/Waiting
  stayed perpetual zeros — making `neo4j_pool_exhausted` unfireable.
- `CollectRateLimitMetrics` had zero callers AND added the cumulative
  rejection total every flush (quadratic inflation) — `rate_limiting_active`
  could never fire.
- 7 of 9 buffered writers had no flush-failure visibility; a wedged writer
  dropped rows silently.
- `retrieval_audit` carried `scorer_version` + `consensus_strength`
  (indexed) with **no reader** — the table that would have auto-caught the
  RRF-scale regression class detected nothing.
- `ConsolidationResult.TotalDuration` was computed and discarded — the
  DBSCAN O(n²) deferral condition (>60 s cycles) was unobservable.
- V0020 `context_catalog_versions` had zero writes ever; 4 `ft_*` dashboard
  panels read tables with no writer.

## What ships

### Retention & compression (migration V0025, TSDB schema 24→25)

| Table | Retain | Compress after | Rationale |
|---|---|---|---|
| embedding_events | 90 d | 7 d | pure telemetry (was 2.7 GB) |
| retrieval_events | 90 d | 7 d | telemetry |
| reinforcement_events | 180 d | 14 d | Hebbian forensics (EVENTGRAPH) |
| retrieval_audit | 180 d | 14 d | scorer forensics |
| sparse_gate_metrics | 180 d | — | retune source, small |
| scheduled_job_events | 180 d | — | jobhealth history |
| llm_endpoint_health_events | 180 d | — | watchdog history |
| constraint_outcomes | 365 d | — | idea-09 observation window ×4 |
| guidance_conflicts | 365 d | — | same window |
| uvts_* / benchmark_* / rl_* / model_install_events / ft_* / context_catalog_versions | none | — | tiny; audit/benchmark history |

First-run deletion forecast at ship time: **0 rows on every table** (oldest
live data 2026-03-31). Policies are removable
(`remove_retention_policy` / `remove_compression_policy`) but retention-
deleted rows are only recoverable from backups. The migration is idempotent
(guarded ALTERs; `if_not_exists` policies) — auto-migrate re-runs it safely.

### Honest metrics plane

- **Windowed percentiles**: synthetic `*_p95` / `*_p99` gauges are now
  computed over the bucket delta between flushes and emitted only when the
  window saw observations. Idle windows leave honest gaps; rules must
  aggregate + `COALESCE`, never `LIMIT 1`.
- **Real pool gauges**: `mdemg_tsdb_pool_{total,idle,acquired,max}_conns` +
  `mdemg_tsdb_pool_empty_acquire_total` from `pgxpool.Stat()`. The fake
  Neo4j pool gauges, their probe loop, and the unfireable
  `neo4j_pool_exhausted` rule (server-native + Grafana) are deleted.
  `GET /v1/system/pool-metrics` serves the real stats
  (`connection_pool.backend = "tsdb-pgx"`).
- **Rate limiting**: collector is delta-correct and actually wired.

### Latency SLO over real wall-time

`retrieve_p95_latency` (medium) / `retrieve_p99_latency` (critical) compute
windowed `percentile_cont` over `retrieval_audit.total_latency_ms`. Config:

- `ALERT_RETRIEVE_P95_MS` (default 120000)
- `ALERT_RETRIEVE_P99_MS` (default 300000)
- `ALERT_RETRIEVE_LATENCY_LOOKBACK_MIN` (default 30)

Defaults are calibrated against the live distribution (7 d: p50 20.4 s,
p95 61.6 s, p99 90.0 s — local-LLM rerank dominates) so they catch
regressions, not steady state. The Grafana mirror rules were rewritten the
same way.

### Writer flush health

Every buffered writer self-registers its `Stats()` at construction
(`internal/tsdb/writer_stats.go`); the metrics pre-flush hook publishes
`mdemg_tsdb_writer_{flush_success,flush_failures,rows_flushed,rows_dropped}_total{writer=<hypertable>}`.
Rule `tsdb_writer_flush_failures` (high, service `tsdb-writer`) fires on
per-writer failure growth within `TSDB_WRITER_ALERT_LOOKBACK_MIN` (60).
`metric_samples`' own writer is deliberately excluded — its failure is
self-evident (every gauge goes stale; the evaluator's global discriminator
fires).

### Scorer-drift tripwires (the RRF-SCALE-001 class becomes self-detecting)

- `scorer_version_change` (medium, service `scorer-drift`): >1 distinct
  `scorer_version` within `SCORER_CHANGE_LOOKBACK_HOURS` (24). Fires while
  old and new versions coexist — the deliberate prompt to re-audit every
  `RetrieveResult.Score` consumer per the score-scale contract.
- `consensus_shift` (medium, service `consensus-shift`):
  |recent − baseline| mean `consensus_strength` >
  `CONSENSUS_SHIFT_THRESHOLD` (0.10 ≈ 1σ live), gated on
  `CONSENSUS_SHIFT_MIN_SAMPLES` (20) in both windows
  (`CONSENSUS_SHIFT_RECENT_HOURS` 6 / `CONSENSUS_SHIFT_BASELINE_DAYS` 7).

`RETRIEVAL_AUDIT_ENABLED` default flipped to **true** (the tripwires need
the feed) and is forwarded in the compose templates.

### Idea-09 measurability + DBSCAN deferral observability

- `mdemg_guidance_conflicts_total{space_id}` increments whenever
  `consulting.Suggest` detects conflicts.
- `mdemg_emergence_cycle_duration_seconds{space_id,cycle}` records every
  completed consolidation cycle (`cycle` ∈ consolidation / conversation /
  conversation_full). Rule `emergence_cycle_slow` fires when the window MAX
  exceeds `EMERGENCE_CYCLE_ALERT_THRESHOLD_SEC` (60) within
  `EMERGENCE_CYCLE_ALERT_LOOKBACK_MIN` (120). This is a **gauge, not the
  roadmap's "histogram"** — the registry's fixed ≤10 s latency buckets would
  clamp multi-minute cycles exactly like the HTTP percentiles this sprint
  un-broke.

### V0020 + ft_* dashboard dispositions

- `context_catalog_versions`: **writer wired** (one sync row per successful
  catalog build via `BuilderOpts.VersionRecorder`; nil-safe, best-effort).
- `mdemg-ft-training.json`: the 4 panels reading writerless `ft_*` tables
  removed; tables stay (designated sinks for the FT recursive-retraining
  loop); status-notice panel records the disposition.

## Rule authoring rules (learned here, pinned by tests)

1. `metric_samples` time column is `time`; `retrieval_audit`'s is
   `recorded_at`. Cross them and the rule silently errors every evaluation.
2. Rules must ALWAYS return one non-NULL row: aggregate + `COALESCE`,
   never `ORDER BY … LIMIT 1` (idle window → "no rows" → rule-health noise).
3. Never alert on lifetime-cumulative percentile synthetics.
4. One `Service` label per rule — the dispatcher cooldown key is
   `(Service, Severity)`.

## Sprint record

`docs/development/tsdb-consume-001/` — plan, post, live verification.
