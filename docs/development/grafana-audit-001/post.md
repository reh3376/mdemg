# Sprint GRAFANA-AUDIT-001 — Sprint Close Post

**Sprint ID**: GRAFANA-AUDIT-001
**Opened**: 2026-05-21
**Closed**: 2026-05-21
**Duration**: ~1 dev-day (estimated 3–5; under-budget because most panels were already correct + Epic 5 coverage expansion deferred)
**OpenAI spend**: $0

## Outcome

Operator's "dashboards are not properly reporting" complaint **validated and resolved** for panel-side bugs. Of 165 target executions across 146 panels:

| Verdict | Pre-sprint | Post-Epic-3 | Δ |
|---|---|---|---|
| PASS | 125 | 130 | +5 |
| EMPTY | 19 | 17 | -2 (the other 5 EMPTYs revealed server-side regression) |
| FAIL | 3 | 0 | -3 |
| SKIP | 18 | 18 | — |

5 panels recovered: 3 SQL bugs on `mdemg-llm-routing` (hardcoded `mdemg-dev` instead of `'$space_id'`), 2 schema-drift filters on `mdemg-j17` (metric_type mismatch) and `mdemg-rsic` (status label mismatch).

## Process

The original framing (audit-first) was the right choice. The Phase 1 11-panel sample missed every real failure because it happened to land on PASS panels. The rigorous per-panel harness caught all 3 FAILs and surfaced the schema-drift EMPTYs on first sweep.

The harness had a subtle bug of its own — `$__interval` substitution wrapped the value in quotes, but Grafana convention is for panel SQL to provide its own outer quotes, producing doubled quotes (`''1 minute''`) and 18 false-positive FAILs. Fixed mid-Epic-1 (bare substitution); re-run dropped 20→3 real FAILs. The unit test was updated to lock in the new behavior.

## Findings

### Smooth parts

1. **Datasource wiring is correct.** All 8 dashboards reference `datasource: {type: postgres, uid: timescaledb}` consistently; provisioning files declare matching UIDs. No drift here.
2. **Template variable defaults match real data.** `$space_id=mdemg-dev`, `$instance=localhost:9999` correctly resolve against 173K rows in the last 24h window. No empty-default regressions.
3. **Continuous aggregates work.** `metrics_hourly` and `metrics_daily` views have data + are queried correctly by 5 panels.
4. **Most dashboards are mostly green.** `mdemg-neo4j` 100%, `mdemg-j17` 97%, `mdemg-jiminy` 95%, `mdemg-rsic` 86%, `mdemg-overview` 85%.

### Friction / surprises

1. **`mdemg-llm-routing` was the only failing dashboard** — 3 panels, single root cause (`mdemg-dev` hardcoded in SQL instead of `'$space_id'` template variable). Authoring error caught by audit; also breaches the no-hardcoding rule documented in memory.
2. **Schema drift on `mdemg_j17_events_total`** — Prometheus naming convention (`_total` = counter) led the panel author to filter `metric_type='counter'`, but the server emits as `'gauge'`. Server-side mismatch with naming intent.
3. **Schema drift on `mdemg_rsic_action_total.status` label** — panel wanted success/failed split, server emits only `'completed'`. Either panel was authored speculatively before label was finalized, OR server-side simplified the status semantics without dashboard update.
4. **May 7-8 emission regression** — 4 metrics (`mdemg_rsic_calibration_confidence`, `_snapshot_created_total`, `_trigger_rejected_total`, `_safety_blocked_total`) have substantial historical data but stopped emitting. **Current codebase grep finds zero references to these metric names** — emission code was removed somewhere without dashboard updates. This is a real observability regression, not a panel-side bug. Server-side investigation queued.
5. **Coverage gap on 11 unused TSDB tables.** Tables like `sparse_gate_metrics` (V0019), `context_catalog_versions` (V0020), `model_install_events` (V0021), `retrieval_audit` (V0017) have ZERO dashboard panels referencing them. Epic 5 coverage expansion was planned but deferred — many of these tables are currently sparse or zero on the dev TSDB (the operator runs training etc. elsewhere), so adding panels would create more EMPTYs, defeating the goal of "dashboards report data." Expansion gated on per-table data accumulation OR explicit operator priority.

### Sprint plan vs reality

- **Epic 0** (sprint plan + audit harness): on-budget, 0.5 day. Harness has 17 Tier 1 unit tests + the $__interval-quoting bug-fix loop.
- **Epic 1** (per-panel audit): on-budget, ~1 day including the harness bug fix.
- **Epic 2** (triage): under-budget; findings.md + audit_summary.md done in same pass as Epic 1 commit.
- **Epic 3** (Tier 1 fixes): under-budget — only 5 panels needed fixing, all single-line JSON edits.
- **Epic 4** (Tier 2 fixes): pivoted from "widen time-ranges" to "document gaps" since data is zero (not sparse) on this TSDB. Feature doc carries the disposition.
- **Epic 5** (coverage expansion): deferred (rationale above).
- **Epic 6** (Tier 3 browser e2e): deferred to operator (can run any time; not blocking sprint close).
- **Epic 7** (documentation): feature doc + this post + CHANGELOG.

## Current state

- **Panel-side**: 0 FAIL, 17 EMPTY (12 known-cause-documented, 5 = server-side regression queued as follow-up).
- **Server-side observability gaps**: 4 metrics stopped emitting around 2026-05-07/08 + 2 never-emitted. All documented in feature doc.
- **Audit harness**: committed at `scripts/grafana_panel_audit.py` with 17 unit tests. Re-runnable any time; suitable for CI integration.
- **Coverage gap**: 11 TSDB tables without panels. Documented; expansion deferred.

## Risks & opportunities (forward)

| Risk | Disposition |
|---|---|
| Operator perception of "diminished observability" persists if browser-side caching shows old panels | Operator does Epic 6 e2e per their schedule |
| Schema drift recurs as new migrations / metric renames land | Run `grafana_panel_audit.py` after each TSDB migration; CI integration recommended |
| May 7-8 emission regression isn't fixed by next sprint | Documented in feature doc; alarm clock on operator |

Opportunities:
- **Nightly CI harness run** against a snapshot TSDB; alert on net-new FAILs (~30 min sprint).
- **Restore the 4 regressed metrics' emission** (~1 day; needs server-side code investigation + emit-restore).
- **Add 2 missing metrics** (`_p50` latency + rate limit counter, ~30 min).
- **Per-table coverage panels** as TSDB data accumulates (gated on operator request).
- **Browser-side e2e screenshot capture** for the 8 dashboards — visual proof of pass/fail beyond SQL execution.

## Acceptance checklist

- [x] Audit harness ran cleanly against all 146 panels with no crashes
- [x] `audit_results.json` covers every panel target with verdict + execution time
- [x] Every FAIL category has a root-cause classification in `findings.md`
- [x] Epic 3 re-run shows zero FAILs (3 → 0)
- [-] Epic 5 new panels — deferred (most target tables have zero data; would worsen EMPTY count)
- [-] Tier 3 browser e2e — deferred to operator
- [x] No hardcoded `'mdemg-dev'` / `'localhost:9999'` strings in committed panel JSON (grep audit: 1 hit, in template variable default — that's the legitimate Grafana pattern, not a bug)
- [x] `docs/features/observability-dashboards.md` shipped
- [x] CHANGELOG entry promoted

## Sprint commits

| Commit | Epic |
|---|---|
| `7679fec` | 0 — sprint plan + audit harness |
| `4c437d9` | 1 + 2 — full audit + findings (+ harness quoting fix mid-flight) |
| `0a1e8e1` | 3 — 5 panel JSON edits (3 FAIL→PASS + 2 EMPTY→PASS) |
| (next) | 7 — feature doc + post.md + CHANGELOG |
