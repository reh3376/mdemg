# Sprint Plan — ALERT-TRUTH-001 (Alert & Dashboard Correctness)

## 1. Header & Metadata
- **Sprint ID**: ALERT-TRUTH-001
- **Sprint line**: `docs/development/alert-truth-001/`
- **Date opened**: 2026-06-30
- **Target version**: v0.11.1 (patch — correctness fixes, no behavior contract change)
- **Estimated effort**: 1–2 dev-days
- **OpenAI spend**: $0
- **Risk level**: Medium (recalibrating the Neo4j CPU alert must not blind a real
  regression — mitigated by keeping the dashboard panel truthful and deferring
  the actual consolidation-cost reduction to a separate follow-up track)

## 2. Problem Statement
A Grafana review (2026-06-30) found 50 pending alerts while the substrate is
healthy (RSIC overall 0.73–0.79, 0 open breakers, ~0% real LLM error rate, 100%
cycle success). The alert channel is dominated by **miscalibrated rules and
dashboard bugs that fire on normal operation**, drowning real signal:

- `neo4j_high_cpu` (19 firings) — threshold `80` is "% of one core"; on multi-core
  hardware normal consolidation runs 200–837%, so it trips on any real graph work,
  and it reads a single latest sample via `ORDER BY time DESC LIMIT 1` (flappy).
- `alert_llm_health` (23 firings) — `llm_error_rate_spike` re-fires HIGH every RSIC
  cycle on what is only **2** "context canceled" errors on `retrieval.rerank_cross`
  (5.7% of 35 calls); the floor is rate-only (`>5% & >10 calls`), no absolute count.
- J17 `nli_bias_alert` (continuous) — `mdemg_j17_nli_mean_bias` emits a stale 0.638
  while the NLI sidecar has **0 requests** (gated off); the DH-004 `IsOperational`
  guard covers only one of the emit paths.
- Latency panels are pinned red — the histogram tops out at `le=10.0s` while
  LLM-backed retrieval runs 10–76s, so p95/p99 clamp at 10.
- Dashboard threshold/unit/calc bugs: Neo4j Memory (512 MiB/1 GiB red while 11.7 GiB
  is healthy), Null-Weight Edges + Conversation Coverage (unit=percent + CPU
  thresholds copy-pasted), Avg Comprehension (`mean` calc shows 0.43 vs real 0.858).

This sprint makes the alerts and panels tell the truth so genuine regressions
surface. It is a **correctness pass**, not a performance change.

## 3. Scope & Constraints
**In scope** (Bucket-2 measurement defects only):
1. The 4 alert rules still on `ORDER BY time DESC LIMIT 1` → aggregate + `COALESCE`
   (TSDB-CONSUME-001 idle-safe contract): `neo4j_high_cpu`, `neo4j_high_memory`,
   `low_cache_hit_ratio`, `jiminy_follow_rate_drop`.
2. `neo4j_high_cpu` threshold → config-driven, host-relative, windowed aggregate.
3. `llm_error_rate_spike` → absolute minimum-error-**count** floor.
4. NLI bias gauge + alert → gate ALL emit paths on `nliScorer.IsOperational()`.
5. Latency histogram buckets → extend to cover LLM-path latency; recalibrate the
   affected dashboard panel thresholds.
6. Dashboard threshold/unit/calc bugs (the 4 above).

**Out of scope** (the real performance tracks — separate sprints):
- Consolidation cost / the 38.7 min run exceeding the 30 min `CONSOLIDATE_TIMEOUT_MS`.
- `retrieval.rerank_cross` context-cancellation root cause.
- Jiminy actionability rollout (Levers C/B enablement).
- Orphan maintenance run + UBENCH benchmark run (zero-code quick wins, on request).

**Constraints**: no hardcoded values (every threshold → config with a calibrated
default); sequential epics, docs in the final epic; live Tier-3 required; dev
branch `reh3376_dev01` → auto-PR; distinct `Service` per alert rule (NOSILENT-001).

## 4. Dependencies
- TSDB-CONSUME-001 idle-safe SQL contract (aggregate + COALESCE, never LIMIT 1).
- NOSILENT-001 distinct-`Service` cooldown-key contract.
- DH-004 NLI `IsOperational` guard (extend to all emit paths).
- The metrics registry histogram bucket config (`internal/metrics/registry.go`).
- The 8 Grafana dashboard JSONs + the `docs/tests/uobs` dashboard spec.

## 5. Implementation Plan (sequential epics + gates)
- **Epic 0** — this plan committed.
- **Epic 1 — Alert-rule SQL hygiene** (`internal/alert/rules.go`): rewrite the 4
  LIMIT-1 rules to windowed `AVG`/`MAX` + `COALESCE(...,<safe default>)` so each
  returns exactly one non-NULL row on an idle window. *Gate*: a pin test greps the
  rule set and asserts no `ORDER BY ... LIMIT 1` remains.
- **Epic 2 — Neo4j CPU calibration** (`rules.go` + `internal/config/config.go`):
  `NEO4J_CPU_ALERT_THRESHOLD_PERCENT` (default calibrated from the live 7-day
  distribution; host-relative, sustained `AVG` over the 5-min window). *Gate*: rule
  fires on a seeded pathological sustained window, silent on normal consolidation.
- **Epic 3 — RSIC reflect floors** (`internal/ape/self_reflect.go`):
  `llm_error_rate_spike` adds `RSIC_LLM_ERROR_MIN_COUNT` (default 5); gate the
  `nli_mean_bias` insight on sidecar-operational. *Gate*: 2-error window no longer
  raises the insight; NLI insight silent when sidecar off.
- **Epic 4 — NLI gauge emit gating** (`internal/ape/self_assess.go`,
  `internal/ape/live_collectors.go`): only `Set` the bias gauge + `nli_bias_alert`
  when `IsOperational()`; otherwise emit nothing (idle-safe). *Gate*: gauge
  absent/real when sidecar off, never stale 0.638.
- **Epic 5 — Histogram + dashboards** (`internal/metrics/registry.go` + 8 JSONs):
  extend buckets (add 20/30/60/120 s); fix the 4 panel threshold/unit/calc bugs;
  recalibrate latency-panel thresholds for the LLM-inclusive paths. *Gate*: each
  fixed panel renders the correct color against live data.
- **Epic 6 — Documentation** (final, never cut): feature doc / extend the
  alert-system section, CLAUDE.md Architecture Note, CHANGELOG, uobs spec refresh.

## 6. Testing Plan (3 tiers)
- **Tier 1 (unit)**: rule-SQL one-non-NULL-row-on-idle pins; the min-error-count
  floor truth-table; NLI gate (operational vs not); histogram bucket boundaries.
- **Tier 2 (integration)**: evaluator against a seeded TSDB with idle + burst +
  sustained windows — assert each rule's fire/silent verdict; confirm the CPU rule
  is silent on normal consolidation load but fires on a pathological sustained one.
- **Tier 3 (live — required)**: restart the server, observe across a real
  consolidation cycle that `neo4j_high_cpu` no longer fires on normal 200–400% load
  (dashboard still shows the true %); `alert_llm_health` does not re-fire on the 2
  rerank errors; the NLI gauge reads real/absent (not 0.638); load each fixed
  Grafana dashboard in the browser and confirm panel colors; clear the alert
  backlog and confirm it does not immediately refill.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`; the final commit promotes the CHANGELOG
Unreleased entry to v0.11.1. Push → auto-PR.

## 8. Verification Checklist
- [ ] 0 `ORDER BY time DESC LIMIT 1` in `rules.go` (pin test)
- [ ] `neo4j_high_cpu` host-relative + windowed, config-driven default
- [ ] `llm_error_rate_spike` min-error-count floor in effect
- [ ] all NLI bias emit paths gated on `IsOperational()`
- [ ] histogram buckets extended; latency panels recalibrated
- [ ] 4 dashboard threshold/unit/calc bugs fixed
- [ ] `golangci-lint run ./...` clean
- [ ] config-consumer guard passes
- [ ] live Tier-3: alert backlog cleared and stable; dashboards correct in-browser
- [ ] CLAUDE.md + CHANGELOG + feature doc + uobs spec updated

## 9. Documentation Update — Epic 6 above.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Raising the CPU threshold blinds a real Neo4j regression | Medium | High | Host-relative + windowed (not blinded); the dashboard panel stays truthful; the actual consolidation-cost reduction is the explicit track-2 follow-up |
| Extending the global histogram buckets shifts other percentiles | Low | Medium | Added high buckets are empty for fast paths — p95/p99 unchanged; pin tests on existing percentiles |
| The min-count floor hides the real rerank issue | Low | Medium | Floor only de-spams the re-fire; the insight is still recorded; the rerank root cause is track-2 |
| A dashboard JSON edit breaks panel rendering | Low | Low | Validate each panel live in the browser (Tier 3) + the uobs spec refresh |

## 11. Documents Accessed
- `internal/alert/rules.go`
- `internal/ape/self_reflect.go`, `self_assess.go`, `live_collectors.go`, `types_rsic.go`
- `internal/jiminy/service.go`, `nli_comprehension.go`, `nli_calibration.go`
- `internal/metrics/registry.go`, `collectors.go`, `recorder.go`
- `deploy/docker/grafana/dashboards/*.json` (all 8)
- CLAUDE.md alert-system + TSDB-CONSUME-001 + DH-004 sections

## 12. Rollback Procedures
All changes are config-defaulted code or dashboard JSON; revert the commit(s). No
schema/migration. Alert rules are code-defined with no persisted state to undo.
