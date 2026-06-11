# Sprint Plan — TSDB-CONSUME-001: Consume the Telemetry Plane (Retention, Honest Gauges, Tripwires)

## 1. Header & Metadata
Sprint: TSDB-CONSUME-001 · 2026-06-11 · branch `reh3376_dev01` ·
Roadmap Q3 Phase 2 (last committed Phase 2/3 member before MCP-REVIVE-001) ·
effort 3.5d · risk medium (adds a TSDB migration with retention policies;
rewrites alert rules that currently fire).

## 2. Problem Statement
Half the telemetry plane is write-only and unbounded, and several alert
rules consume data that does not mean what they think it means. Live
evidence (2026-06-11):
- `embedding_events` is **2.7 GB with no retention/compression**; only
  V0001/V0002 tables have policies. Everything V0006+ grows forever.
- The `latency-slo` rules are triply broken: they read synthetic p95/p99
  gauges computed from **lifetime-cumulative** histogram buckets (live
  value: constant **9.95** = top-bucket clamp — one slow call pegs it
  forever), they use `ORDER BY time DESC LIMIT 1` which returns **zero
  rows on idle windows** (the recurring `rule-health-*_latency` "no rows
  in result set" noise), and the 0.25/0.5 s thresholds are absurd against
  real retrieve latency (retrieval_audit 7d: p50 20.4 s, p95 61.6 s).
- The Neo4j "pool" gauges are fake by construction:
  `StartPoolMetricsCollector` (`internal/db/neo4j.go:38`) just probes
  `VerifyConnectivity` every 10 s — `pool_acquired_total` is an uptime
  counter wearing a pool name; Active/Idle/Waiting are perpetual zeros, so
  `neo4j_pool_exhausted` can never fire. The real pgx (TSDB) pool stats
  exist (`pgxpool.Stat()`) and are unexported.
- `CollectRateLimitMetrics` does `Add(RejectedTotal())` — adds the
  **cumulative** total every flush instead of the delta (quadratic
  inflation the first time anything is rejected; zero rows until then).
- 7 of 9 buffered TSDB writers have no flush-failure visibility (only
  ReinforcementEventsWriter is instrumented) — a wedged writer drops rows
  silently.
- `retrieval_audit` carries `scorer_version` + `consensus_strength`
  (indexed) but has **no reader** — the table that would have auto-caught
  the RRF-scale regression class detects nothing.
- `guidance_conflicts` rows land (ConflictTracker) but no counter/metric
  exists, so idea 09's go/no-go criterion is unmeasurable.
- Emergence cycle wall-time is computed (`ConsolidationResult.TotalDuration`)
  and discarded — the DBSCAN O(n²) deferral condition (>60 s cycles) is
  unobservable.
- V0020 `context_catalog_versions` is created, indexed, and has **zero
  writes ever** (builder persists to Neo4j only).
- `mdemg-ft-training.json` has 4 panels reading tables with no writer
  (`ft_model_versions`, `ft_benchmarks`, `ft_training_cycles` — all 0 rows).

## 3. Scope & Constraints
**In**: V0025 retention+compression migration; recorder windowed-percentile
fix (delta-bucket synthetics); latency rules rewritten over
`retrieval_audit` real wall-time with config thresholds; honest pool
metrics (real pgx gauges, fake Neo4j pool gauges removed, probe metrics
renamed honestly, `neo4j_pool_exhausted` rule deleted with rationale);
rate-limit collector delta fix + rule hardening; unified
`mdemg_tsdb_writer_*` flush stats for all buffered writers + flush-failure
rule; scorer_version-change + consensus-shift tripwire rules
(+ `RETRIEVAL_AUDIT_ENABLED` default flip to true); guidance_conflicts
counter; emergence-cycle duration gauge/histogram + >60 s rule; V0020
writer (wire, not drop — CONTEXT-LIVE-001 is its named consumer);
ft_* dashboard dead-panel deletion. All new thresholds/windows
env-configurable (no-hardcoding rule).
**Out**: Grafana dashboards beyond the ft_* disposition; the
`low_cache_hit_ratio` rule (data live, fires legitimately); HTTP histogram
bucket-boundary retune; Neo4j-driver pool instrumentation (no public API);
idea 09 itself (this sprint only makes it measurable); helm
`REQUIRED_SCHEMA_VERSION` (verified: that is the **Neo4j** schema env,
config.go:1242 — value 26 is correct, recon-agent "drift" was false).

## 4. Dependencies
`internal/tsdb/migrations/` (V0025, schema 24→25 — bump config.go:3988
default + CI validator auto-checks file count); `internal/metrics/`
(recorder.go synthetics, collectors.go pool/ratelimit collectors);
`internal/alert/rules.go` + evaluator (distinct Service per rule —
cooldown-key constraint); `internal/db/neo4j.go` probe collector;
`internal/tsdb/*_writer.go` (7 writers); `internal/consulting/service.go`
conflict path; `internal/hidden/service.go::RunConsolidation`;
`internal/hidden/context_catalog_builder.go::persistCatalog`;
`deploy/docker/grafana/dashboards/mdemg-ft-training.json`.

## 5. Implementation Plan
Epic 0 plan (this doc) · **Epic 1** V0025 retention/compression (windows
below; oldest live data 2026-03-31 ⇒ 90 d policies delete ~0 rows on first
run — verified before ship with a per-table forecast query) · **Epic 2**
honest metrics plane: windowed percentile synthetics, pgx pool gauges,
fake-gauge removal, rate-limit delta fix, latency rules over
retrieval_audit · **Epic 3** writer flush stats + flush-failure rule ·
**Epic 4** retrieval_audit tripwires (scorer-change, consensus-shift) +
default flip · **Epic 5** guidance_conflicts counter + emergence duration
+ rule · **Epic 6** V0020 writer · **Epic 7** ft_* dashboard panel
deletion + status-notice update · **Epic 8** docs
(`docs/features/tsdb-data-management.md`, CHANGELOG, CLAUDE.md, post.md),
live Tier 3, push.

Retention/compression windows (data-decided; disclosed in PR):
| Table | Retain | Compress | Rationale |
|---|---|---|---|
| embedding_events | 90 d | 7 d | 2.7 GB, pure telemetry |
| retrieval_events | 90 d | 7 d | telemetry |
| reinforcement_events | 180 d | 14 d | Hebbian forensics (EVENTGRAPH line) |
| retrieval_audit | 180 d | 14 d | scorer forensics window |
| sparse_gate_metrics | 180 d | — | small; retune source |
| constraint_outcomes | 365 d | — | idea-09 3-month observation window ×4 |
| guidance_conflicts | 365 d | — | same window |
| scheduled_job_events | 180 d | — | jobhealth history |
| llm_endpoint_health_events | 180 d | — | watchdog history |
| uvts_*/benchmark_*/rl_*/model_install_events/ft_*/context_catalog_versions | none | — | tiny, audit/benchmark history |

## 6. Testing Plan
Tier 1: recorder windowed-percentile tests (two flushes, delta math; empty
window → no synthetic emit); rate-limit delta-collector test; rule pin
tests extending the `TestMetricSamplesRules_UseTimeColumn` convention
(retrieval_audit rules must use `recorded_at`, metric_samples rules must
use `time`; latency/tripwire rules always-return-a-row via aggregate +
COALESCE); writer stats tests; conflict-counter + emergence-duration unit
tests. Tier 2: full `go test ./internal/...`; V0025 applies via
auto-migrate on the live TSDB. Tier 3 (live): policies visible in
`timescaledb_information.jobs`; first-run deletion forecast executed and
recorded; restart server → new gauges land in `metric_samples`; evaluator
runs ≥10 min with **zero** `rule-health-*_latency` failures; scorer
tripwire SQL hand-verified against the table's real two-version history;
emergence duration sample lands after a consolidation cycle.

## 7. Commit Strategy
One commit per epic (1, 2, 3, 4, 5+6 may merge if small, 7, 8) · lint
before each commit · push once (auto-PR) · sprint summary comment ·
surprise live-smoke bugs get their own fix commits.

## 8. Verification Checklist
- [ ] V0025 applied; retention+compression jobs exist per the table above
- [ ] First-run deletion forecast per table recorded (expected ≈0)
- [ ] `TSDB_REQUIRED_SCHEMA_VERSION` 24→25 (config.go; CI validator green)
- [ ] p95/p99 synthetics are windowed (live values move, no 9.95 clamp)
- [ ] latency rules read retrieval_audit, always return a row, config thresholds
- [ ] live smoke: evaluator ≥10 min, zero rule-health-*_latency failures
- [ ] fake Neo4j pool gauges gone; real `mdemg_tsdb_pool_*` gauges live in metric_samples
- [ ] `neo4j_pool_exhausted` rule deleted with rationale; rate-limit collector delta-correct
- [ ] `mdemg_tsdb_writer_*{writer}` stats live for all buffered writers + flush-failure rule
- [ ] scorer-change + consensus-shift rules ship; RETRIEVAL_AUDIT_ENABLED default true
- [ ] guidance_conflicts counter increments on conflict detection
- [ ] emergence cycle duration observed live; >60 s rule registered
- [ ] V0020 row written on catalog build (live)
- [ ] ft_* dead panels removed; status notice updated
- [ ] feature doc + CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update — Epic 8 (never cut).

## 10. Risks & Mitigations
Retention deletes data → windows chosen above oldest-data age (first run
≈0 deletions, forecast verified live before push; TSDB backup scheduler is
live + restore-proven per BACKUP-RESTORE-VERIFY-001). Compression
constrains future ALTERs on compressed chunks → only applied to
stable-schema, high-volume tables. Rewritten latency rules go quiet
(wrong thresholds) → defaults calibrated from live percentiles and
env-overridable; rule-health meta-alert (SUPERVISOR-002) still watches SQL
validity. New rules sharing a Service would mask each other → each new
rule gets a unique Service label (NOSILENT-001 cooldown-key rule).
Windowed synthetics change the meaning of `*_p95/_p99` series → disclosed
in PR; Grafana panels reading them get truthful (windowed) values.

## 11. Documents Accessed
ROADMAP_2026Q3.md:55 (scope line); 3 recon agents over internal/tsdb,
internal/alert, internal/metrics, grafana dashboards (two material agent
errors corrected by live verification: HTTP metrics ARE written; helm
schema value is Neo4j, not TSDB); live TSDB queries (hypertable sizes,
policy jobs, metric_name census, retrieval_audit percentiles + scorer
history, data ages); internal/alert/rules.go; internal/metrics/
{recorder,collectors}.go; internal/db/neo4j.go; internal/ratelimit/
middleware.go; migrations 001/002/017/020.

## 12. Rollback Procedures
Migration: retention/compression policies are removable
(`remove_retention_policy`/`remove_compression_policy`; compressed chunks
decompressible) — but **deleted-by-retention rows are only recoverable
from backups**; hence the first-run forecast gate. Code: revert commits.
Rules: each new rule is a named entry, independently deletable.
