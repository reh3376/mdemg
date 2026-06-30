# ALERT-TRUTH-001 — Post

**Status: SHIPPED.** · 2026-06-30 · branch `reh3376_dev01`

A Grafana dashboard review found 50 pending alerts while the substrate was
healthy (RSIC 0.73–0.79, 0 open breakers, ~0% real LLM error rate, 100% cycle
success). The alert channel was dominated by **miscalibrated rules and dashboard
bugs that fired on normal operation** — not by real regressions. This sprint made
the alerts and panels tell the truth. No behavior-contract change.

## Shipped (Epics 1–5)

1. **Epic 1 — alert-rule SQL hygiene.** The last 4 rules on the
   `ORDER BY time DESC LIMIT 1` anti-pattern (`neo4j_high_cpu`, `neo4j_high_memory`,
   `low_cache_hit_ratio`, `jiminy_follow_rate_drop`) rewritten to windowed `AVG`
   + `COALESCE` (TSDB-CONSUME-001 idle-safe contract). Pins
   `TestAllRules_NoLimitOneAntiPattern` + `TestAllRules_DistinctServicePerSeverity`
   gather every rule group so new rules are auto-covered.
2. **Epic 2 — Neo4j CPU calibration.** `neo4j_high_cpu` extracted to the
   config-driven host-relative `Neo4jCPURule(NEO4J_CPU_ALERT_THRESHOLD_PERCENT`,
   default 500) over a 5-min windowed AVG. The fixed `80` was "% of one core" —
   the 19-alert driver. Distinct Service `neo4j-cpu`.
3. **Epic 3 — RSIC reflect floors.** `llm_error_rate_spike` gained
   `RSIC_LLM_ERROR_MIN_COUNT` (default 5) so sub-5-error transients don't re-fire
   HIGH.
4. **Epic 4 — NLI bias false-emit.** `GetNLICalibrationReport()` returns nil when
   the sidecar isn't operational — fixes the phantom 0.638 `nli_bias_alert` at the
   source (covers both gauge emitters + the RSIC insight in one place).
5. **Epic 5 — histogram + dashboards.** Latency buckets extended 10s→120s (LLM
   paths were clamping at the 10s ceiling). Four panel bugs fixed: `P95 Latency`
   (0.25s→120s), `Neo4j Memory` (512MiB/1GiB→24/27GiB), `Null-Weight Edges`
   (percent→short), `Conversation Coverage` (percent→percentunit, inverted),
   `Avg Comprehension` (mean→lastNotNull).

## Testing

- **Tier 1/2:** alert/ape/jiminy/metrics unit suites green; the two all-rules
  contract pins; `TestNeo4jCPURule`; `TestGetNLICalibrationReport_GatedOnOperational`.
- **Tier 3 (live, on the real binary + services):**
  - Built + launchd-restarted the server (pid 40016→9781, new binary loaded).
  - **Neo4j CPU:** during active consolidation the 5-min windowed AVG was **447**
    (< 500) → the new rule stayed silent, while the old LIMIT-1 rule (latest
    sample 377 > 80) would have fired. **0 new `neo4j`/`neo4j-cpu` alerts in
    70+ min** (newest pre-restart alert 13:10, through a full consolidation).
  - **NLI bias:** the `j17_nli_mean_bias` gauge read **0 on all 10 samples** in
    the 5 min post-restart (was a stale 0.638); no new `nli_bias_alert`.
  - **LLM error floor:** the only post-restart `rsic-alert_llm_health` firing is
    a **truthful** one — `retrieval.rerank_cross` at a real 9.6% / 9-error rate
    over 24h (the floor correctly does not suppress a genuine spike; the 60-min
    recency gate self-bounds it once errors stop).
  - SQL-validated each new rule query directly against the live TSDB.

## Out of scope → follow-up tracks

These are the **real performance** items the truthful alerts now point at:

- **Neo4j consolidation cost** — the full 22-phase re-cluster on 83k nodes runs
  ~38 min (exceeding `CONSOLIDATE_TIMEOUT_MS` 30 min) 2–3×/day. HIDDEN-CHURN-003
  made only the L0-orphan hidden step incremental. (Track 2.)
- **`retrieval.rerank_cross` context-cancellation** — the real 9.6% error rate
  the `llm_error_rate_spike` alert now correctly surfaces. (Track 2.)
- **Jiminy actionability rollout** — the legitimate `guidance-should-follow`
  alert (follow-rate gap); Levers C/B shipped default-off. (Track 3.)

## Documents Accessed
- `internal/alert/rules.go`, `internal/cli/serve.go`
- `internal/ape/self_reflect.go`, `self_assess.go`, `live_collectors.go`
- `internal/jiminy/service.go`, `nli_calibration.go`, `nli_comprehension.go`
- `internal/metrics/registry.go`
- `internal/config/config.go`
- `deploy/docker/grafana/dashboards/{mdemg-overview,mdemg-neo4j,mdemg-j17}.json`
- CLAUDE.md alert-system / TSDB-CONSUME-001 / NOSILENT-001 / DH-004 sections
