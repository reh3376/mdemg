# Sprint DASHBOARD-TRUTH-002 — Second wave of dashboard/measurement artifact fixes

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | DASHBOARD-TRUTH-002 |
| Sprint Name | Second wave of dashboard/measurement artifact fixes (9 items across RSIC / J17 / Jiminy / FT-Training) |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 1.5–2 dev-days |
| Sprint Line | dashboard-truth-002 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Follow-up to DASHBOARD-TRUTH-001 (2026-07-03); operator-flagged concerns across 4 dashboards (2026-07-20) |

## 2. Problem Statement

DASHBOARD-TRUTH-001 (2026-07-03) fixed 8 metrics on the RSIC / J17 / Jiminy dashboards where 6 were measurement artifacts. Since then more artifacts have accumulated. A 5-agent read-only triage (2026-07-20) classified 14 operator-flagged concerns across 5 dashboards: **9 are ARTIFACTS worth fixing**, 3 are REAL-LOW already in flight (JIMINY-CORPUS arc), 1 is designed-behavior needing a companion visualization only, and 1 requires new investigation (JIMINY-ACTIONABILITY-INVERSION-001 — separate sprint).

The 9 artifacts share the same forensic family DASHBOARD-TRUTH-001 documented: `lastNotNull` on multi-row queries, `COALESCE(...,0)` masking absent data, stale/degenerate aggregate gauges, wrong-anchor thresholds, hardcoded enum-lookup "scores" that measure maturity not quality, panels reading a transition-log where a continuous gauge is needed, vestigial reward keys, default time-range mismatched to rare-event streams. Each is truthfully misreporting; the underlying substrate is fine.

## 3. Scope & Constraints

**In scope** — 9 artifacts, ordered by dashboard:

**RSIC (4 items):**
- A2. Retrieval dimension score 0.70 — `scoreRetrieval` enum table hardcodes `saturated=0.7`, penalizing maturity. Fix: read from real signals (retrieval_events rerank fill-rate + uvts_runs recent mean) OR at minimum set `saturated=0.9`.
- A3. Edge dimension score 0.80 — fixed-weight structural edges (CALLS=1.0, GENERALIZES=0.987, SPECIALIZES=0.987, ANALOGOUS_TO=0.957, EXTENDS/IMPORTS=1.0) collapse Shannon entropy <0.5, triggering permanent −0.2 penalty. Fix: exclude structural edge classes from entropy calc OR compute per-edge-type entropy and average.
- A4. Protocol dimension score 0.78 — `TicketRestoreSuccessRate=0` scored as 0.5 instead of "no data neutral" (DH-004 already applied this pattern for the same field elsewhere in J17 Protocol Health but not `scoreProtocol`). Fix: apply the DH-004 gate + recalibrate `J17_COMPRESSION_TARGET_RATIO` 3.0→~2.0 (live p95 is 2.0).
- A5. Retrieval Pipeline Health panel — `gauge`/`lastNotNull` on a 3-row UNION displays only the LAST row (rerank 77%); recall (100%) and BM25 (100%) are hidden. Same class as DASHBOARD-TRUTH-001 E1. Fix: convert to bargauge/horizontal, OR split into 3 stat panels.

**J17 (1 item):**
- A6. Min==Max==Avg=23.30% — `TrustSessionCount=1` (one TTL-live session ≥ `J17_TRUST_MIN_FEEDBACK_COUNT=5`). The gauges honestly compute over N=1, but presenting Min/Max stats over a set of one is misleading. Fix: hide Min/Max when `TrustSessionCount<2`, OR collapse to "Live Session Trust + session-count context".

**Jiminy (1 item):**
- A7. All-Time (0.25) vs Selected-Range (0.14) — same metric, different sources (Neo4j GUIDANCE_OUTCOME cumulative including pre-2026-06-11 heuristic half-credit inflation vs TSDB constraint_outcomes windowed) AND different thresholds (HAVING≥5 vs HAVING≥2). Not a real degradation. Fix: unify both panels to TSDB `constraint_outcomes` with matching threshold, OR update panel description to acknowledge the pre-fix inflation.

**FT-Training (3 items):**
- A8. `hidden.reclassify` mean_score always 0.500 — writer persists `classification_accuracy=0` alongside `json_valid=1.0` despite spec declaring only `json_valid`. Panel averages both → exactly 0.5. Fix: at the writer (`neural.benchmarks.run_benchmark`), filter `reward_vector` to spec-declared keys only.
- A9. LLM endpoint state = "No data" — panel reads `llm_endpoint_health_events` (transition log; zero rows in steady-state UP). Fix: rewrite panel SQL to source from `metric_samples.mdemg_mlx_health_state` (continuous gauge; 261 lifetime samples); keep events as annotations overlay.
- A10. Recent Watchdog events = "No data" — 12 lifetime events, newest 15d ago, default time-range excludes them. Fix: drop `$__timeFilter`, use `ORDER BY recorded_at DESC LIMIT 30`.

**Bonus companion (from RSIC #5 REAL classification):**
- Add "Cycle Admission Ratio" companion panel to `mdemg-rsic` (`cycles/(cycles+rejections)` normalized health signal) so the raw Trigger Rejection Rate has interpretive context.

**Out of scope**:
- The `caller_canceled:%` filter (that's GRAFANA-PANEL-FILTER-001 — separate sprint).
- The advisory-followed-more-than-actionable inversion (that's JIMINY-ACTIONABILITY-INVERSION-001 — separate investigation).
- Stale FT benchmark data (that's FT-BENCH-REFRESH-001).
- The Prometheus `/metrics` HTTP 404 (that's PROMETHEUS-SCRAPE-INVESTIGATION-001).
- Guidance follow rate lift (JIMINY-CORPUS arc — active).
- J17 tier/compression/trust cascade (derived from follow rate — will heal upstream).

**Constraints**:
- **No hardcoded values.** Any threshold/anchor becomes a config var with a sensible default; the default MUST be calibrated to live data (as DASHBOARD-TRUTH-001 E3 did for `J17_COMPRESSION_TARGET_RATIO`).
- **RRF-SCALE-001-safe.** No new hardcoded gates on `RetrieveResult.Score`.
- **Live Tier-3 required per epic.** Before/after gauge values captured for every fix.
- **Additive-only for TSDB writer changes.** A8's writer-side filter must not silently drop keys the panel still expects — check every consumer.

## 4. Dependencies & Pre-Conditions

- ✅ DASHBOARD-TRUTH-001 shipped (2026-07-03) — precedent for the fix patterns.
- ✅ DH-004 shipped — the "no data = neutral" gate for `TicketRestoreSuccessRate` in J17 Protocol Health is the template for A4.
- ✅ Triage report captured with live evidence per concern (this session, 2026-07-20).
- ✅ `mdemg-dev` populated (fresh data for before/after gauge checks).
- ⚠️ **Config defaults for J17_TRUST_MIN_FEEDBACK_COUNT** (5) and other DASHBOARD-TRUTH-001 constants are current baselines — do NOT change them here except where an epic explicitly targets them (A4's compression-target recalibration).

## 5. Implementation Plan

Sequential — never parallelize. Each epic is one concern; ~9 epics + docs.

### E0 — Sprint plan
Commit this plan.

### E1 — RSIC Retrieval dimension (A2)
`internal/ape/self_assess.go::scoreRetrieval` — swap the enum-table lookup for one that reads:
- rerank fill-rate from `retrieval_events` (last N calls) OR
- recent `uvts_runs.mean_score` OR
- both, weighted.
Extract weights + lookback window to `RSIC_RETRIEVAL_SCORE_*` config with data-calibrated defaults. If a full rewrite is too invasive, ship the minimum: `saturated→0.9` in the enum table (equal to `warm`, since saturation is not degradation). Add unit test + document the choice.
**Gate**: live gauge `mdemg_rsic_health_retrieval` moves 0.70 → ~0.9 for `mdemg-dev` (saturated, healthy pipeline).

### E2 — RSIC Edge dimension (A3)
`internal/ape/self_assess.go::scoreEdge` — exclude structural edge classes (`CALLS`, `GENERALIZES_TO`, `SPECIALIZES`, `ANALOGOUS_TO`, `EXTENDS`, `IMPORTS`, plus any others in this class) from the Shannon entropy computation. Extract the excluded list to `RSIC_EDGE_ENTROPY_EXCLUDE_TYPES` config with the audited default. Alternative: compute per-edge-type entropy and average; harder to interpret, prefer exclusion.
**Gate**: live gauge `mdemg_rsic_health_edge` moves 0.80 → higher (0.9+) as the entropy-penalty falls off.

### E3 — RSIC Protocol dimension (A4)
`internal/ape/self_assess.go::scoreProtocol` — apply the DH-004 "no data = neutral" gate to `restoreScore` (default 1.0 when `TicketRestoreTotal==0`). Then recalibrate `J17_COMPRESSION_TARGET_RATIO` default 3.0→2.0 (live 30d p95). Both fixes are already established patterns.
**Gate**: live gauge `mdemg_rsic_health_protocol` moves 0.78 → higher; compression sub-score moves from 0.435 → closer to 1.0.

### E4 — RSIC Retrieval Pipeline Health panel (A5)
Edit `deploy/docker/grafana/dashboards/mdemg-rsic.json` — convert the panel from `gauge`/`lastNotNull` to `bargauge` horizontal (shows all 3 stages: recall / bm25 / rerank), OR split into 3 stat panels. Add row/panel description clarifying that rerank drop is EXPECTED when `RerankMinBudgetMs` pre-check fires (LLM-HEALTH-INVESTIGATION-001 E2).
**Gate**: panel displays 100 / 100 / 77 (all 3 stages visible); operator can distinguish "pipeline broken" from "rerank pre-check triggered".

### E5 — RSIC Cycle Admission Ratio companion panel
New stat panel: `cycles/(cycles+rejections)` over the same window. Placed next to Trigger Rejection Rate. Description: "Normalized admission health. RSIC-STORM-001 design intent is aggressive rejection; low ratios are architecturally expected in high-noise windows."
**Gate**: panel renders with a live value; operator can see admission-normalized signal.

### E6 — J17 Min/Max presentation (A6)
Edit `deploy/docker/grafana/dashboards/mdemg-j17.json` — hide Min/Max panels when `TrustSessionCount<2` (Grafana panel-level `hideFrom.reduce.count<2` mapping OR `noValue` gate), OR collapse them into a single "Live Session Trust" panel with a companion session-count stat. Prefer collapse. Optionally relax `J17_TRUST_MIN_FEEDBACK_COUNT` 5→3 to enlarge N — decide only after checking live data supports 2+ sessions clearing feedback≥3.
**Gate**: panel no longer shows "Min 23.30% / Max 23.30% / Avg 23.30%"; when N=1 the display makes the N=1 explicit.

### E7 — Jiminy All-Time vs Selected panel (A7)
Edit `deploy/docker/grafana/dashboards/mdemg-jiminy.json` — unify both panels to read from TSDB `constraint_outcomes` with the same HAVING threshold. Preferred: both use HAVING≥5 (matches the historical Neo4j gauge threshold) and filter by classifier_source to exclude pre-2026-06-11 heuristic-dominated rows. Fallback if unifying is too invasive this sprint: update the All-Time panel description to acknowledge pre-fix inflation.
**Gate**: both panels report the same order-of-magnitude value; the gap between them is < 0.05 for the same time window.

### E8 — FT hidden.reclassify vestigial reward key (A8)
`neural.benchmarks.run_benchmark` — at the writer, filter `reward_vector` output to only the keys declared in the spec's `reward_functions[]`. Alternatively (weaker but faster), filter at the panel SQL side: `WHERE kv.k = ANY(spec.reward_functions)`. Prefer writer-side (single source of truth).
**Gate**: after next `run_benchmark`, `hidden.reclassify.mean_score` reads 1.0 (json_valid only) instead of 0.5. If E8 relies on FT-BENCH-REFRESH-001 to actually re-populate the row: coordinate — FT-BENCH-REFRESH-001 runs first, THIS sprint's E8 fix is applied, then FT-BENCH-REFRESH-001 re-runs the benchmark.

### E9 — FT LLM endpoint state panel (A9)
Edit `deploy/docker/grafana/dashboards/mdemg-ft-training.json` — rewrite the panel SQL to source from `metric_samples`:
```sql
SELECT time, value, labels->>'endpoint_url' AS metric
FROM metric_samples
WHERE metric_name = 'mdemg_mlx_health_state' AND $__timeFilter(time)
ORDER BY time
```
Add `llm_endpoint_health_events` as an `annotations` overlay (transition markers on the continuous line).
**Gate**: panel renders continuous state=0 (up) for the last 24h; no "No data".

### E10 — FT Recent Watchdog events panel (A10)
Edit `deploy/docker/grafana/dashboards/mdemg-ft-training.json` — drop `$__timeFilter(recorded_at)` from the panel; use `ORDER BY recorded_at DESC LIMIT 30`. Update panel title from "last 30 in window" → "last 30 events".
**Gate**: panel renders the 12 lifetime rows (newest 15d ago), no "No data".

### E11 — Canonical docs
- CHANGELOG [Unreleased] > Fixed: single consolidated entry.
- CLAUDE.md: extend the existing DASHBOARD-TRUTH-001 note with a "sequel" clause referencing DASHBOARD-TRUTH-002 + the new architectural rules surfaced (structural-edge-entropy-exclusion, admission-ratio-companion pattern, gauge-vs-transition-log rule).
- `docs/features/dashboard-metric-honesty.md`: expand with the 9 new patterns/rules.
- Sprint `post.md` with per-epic before/after evidence.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit**:
- E1/E2/E3: table-driven tests over `scoreRetrieval`/`scoreEdge`/`scoreProtocol` verifying the new formulas + edge cases (zero data, all-structural-edges, etc.).
- E8: writer-side reward-vector filtering test using a synthetic spec.

**Tier 2 — Integration**:
- Grafana JSON files pass a schema-validation Go test (extend existing if any).
- E9's continuous gauge test: `TestMetricSamplesRules_UseTimeColumn` (from HIDDEN-CHURN-001) — confirm new panel SQL uses the correct column.

**Tier 3 — Live E2E** — REQUIRED for every visible-value change:
- E1: `curl :9999/metrics | grep mdemg_rsic_health_retrieval` before/after; assert value moved as intended. (Note: /metrics may not be wired — PROMETHEUS-SCRAPE-INVESTIGATION-001; fallback is direct `metric_samples` SQL.)
- E2: same for `mdemg_rsic_health_edge`.
- E3: same for `mdemg_rsic_health_protocol` + `mdemg_j17_compression_ratio` sub-score.
- E4/E5/E6/E7/E9/E10: reload Grafana, open dashboard, screenshot / annotated JSON of before + after; verify operator-scannable display.
- E8: run `python -m neural.benchmarks.run_benchmark ...` after fix; verify `hidden.reclassify` row's `reward_vector` has only `json_valid`.
Aggregate all Tier-3 evidence in `docs/development/dashboard-truth-002/live_verification.md`.

## 7. Commit Strategy

One commit per epic; Conventional Commits.
1. `docs(dashboard-truth-002): E0 — sprint plan`
2. `fix(dashboard-truth-002): E1 — RSIC retrieval dimension reads real signals`
3. `fix(dashboard-truth-002): E2 — exclude structural edges from RSIC edge entropy`
4. `fix(dashboard-truth-002): E3 — RSIC protocol dimension applies DH-004 no-data gate + compression recal`
5. `fix(dashboard-truth-002): E4 — retrieval pipeline health panel shows all stages`
6. `feat(dashboard-truth-002): E5 — cycle admission ratio companion panel`
7. `fix(dashboard-truth-002): E6 — J17 min/max/avg presentation honest for N=1`
8. `fix(dashboard-truth-002): E7 — Jiminy all-time vs selected panels unified`
9. `fix(dashboard-truth-002): E8 — hidden.reclassify vestigial reward key filtered at writer`
10. `fix(dashboard-truth-002): E9 — LLM endpoint state reads continuous gauge`
11. `fix(dashboard-truth-002): E10 — recent watchdog events panel drops narrow filter`
12. `docs(dashboard-truth-002): E11 — CHANGELOG + CLAUDE.md + feature doc + sprint post`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] `go test ./...` clean (all new unit tests green)
- [ ] Working tree clean
- [ ] Live Tier-3 evidence captured for every visible-value change
- [ ] CHANGELOG + CLAUDE.md + feature doc + sprint post committed
- [ ] Pushed; auto-PR created

## 9. Documentation Update (Epic E11 — never cut)

- **CHANGELOG.md** [Unreleased] > Fixed: consolidated entry with per-epic 1-line summary + link to sprint plan.
- **CLAUDE.md**: extend the DASHBOARD-TRUTH-001 architecture note in place; NEW architectural rules surfaced:
  1. RSIC dimension scoring functions must read from real signals, NOT enum lookups of maturity/phase.
  2. Shannon entropy over edge weights must exclude structural fixed-weight edge classes.
  3. Any "success rate" metric with a "no data" possibility must gate to neutral (1.0) when the denominator is zero (the DH-004 pattern generalizes).
  4. Multi-row gauge panels must not use `lastNotNull` — use bargauge or split to stat panels.
  5. Aggregate stats (min/max) computed over a live set must gate visibility on set-size floor.
  6. Writers must NOT persist reward-vector keys that the spec doesn't declare.
  7. Panels reading "state" must use the continuous gauge, NOT the transition-log.
- **Feature doc**: `docs/features/dashboard-metric-honesty.md` — expand with the 9 new rules; cite this sprint as the source.
- **Sprint post**: `docs/development/dashboard-truth-002/post.md`.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| E1 rewrite too invasive; ships weak minimum | Low | Explicitly plan the minimum first (saturated=0.9), then upgrade if time permits |
| E3 compression recal breaks existing alert-rule expectations | Low | The recal is *lower* target (easier to hit), rule fires LESS not more; validated by 30d live p95=2.0 |
| E8 writer-side filter breaks a downstream consumer expecting the vestigial key | Medium | grep every consumer of `reward_vector` before change; write pin test |
| A9/A10 Grafana file-watcher doesn't reload | Low | `docker restart mdemg-grafana` fallback in Tier-3 procedure |
| E7 unification retroactively changes historical dashboard reads | Low | Panel is a *display*, not a store; historical data stays in TSDB; consumers who need old computation can query directly |
| One of the 9 fixes surfaces a deeper defect during live smoke | Medium | Follow the Phase 11.6.2 precedent: surprise gets its own fix-commit, sprint continues |
| Effort creeps past 2 dev-days | Medium | E4/E5/E6/E9/E10 are all Grafana-JSON edits (fast); code changes (E1/E2/E3/E8) are the risk; drop E1's full rewrite to the minimum-fix if time-pressured |

## 11. Rollback Procedures

- **Data**: none of these are destructive; A8's writer filter is forward-only (existing rows keep their vestigial keys; panel averages will improve as new rows land).
- **Code**: each epic is one commit; revert individually if a specific fix regresses.
- **Grafana**: dashboard-JSON reverts on next commit; no persistent Grafana state.
- **Config**: default changes (E3 J17_COMPRESSION_TARGET_RATIO 3.0→2.0, RSIC_EDGE_ENTROPY_EXCLUDE_TYPES) are safe to revert via env override; ship documented.

## 12. Documents Accessed

- Prior sprint: `docs/development/dashboard-truth-001/` (parent patterns)
- Prior sprint: `docs/development/alert-truth-001/` (LIMIT-1 / idle-safe SQL contract)
- Feature doc: `docs/features/dashboard-metric-honesty.md`
- CLAUDE.md § DASHBOARD-TRUTH-001, § DH-004, § DH-005, § LLM-HEALTH-INVESTIGATION-001
- Triage report agents (this session, 2026-07-20)
- `internal/ape/self_assess.go` (scoreRetrieval/scoreEdge/scoreProtocol/scoreGuidance)
- `internal/ape/live_collectors.go` (gauge emission)
- `internal/j17/` (trust store, tier calibration, compression)
- `internal/jiminy/{stats.go,protocol_metrics.go,encoder.go}`
- `internal/tsdb/dataset_builder.go` (LLMPerformance / GuidanceEffectiveness)
- `deploy/docker/grafana/dashboards/mdemg-{rsic,j17,jiminy,ft-training}.json`
- `neural/benchmarks/run_benchmark.py` (reward_vector writer)
- `docs/tests/ults/specs/hidden_reclassify.ults.json` (spec that excludes classification_accuracy)
