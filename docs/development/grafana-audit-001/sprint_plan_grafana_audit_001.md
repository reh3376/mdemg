# Sprint GRAFANA-AUDIT-001 — Dashboard Audit & Fix

## 1. Header & Metadata

- **Sprint ID**: GRAFANA-AUDIT-001
- **Sprint line**: `docs/development/grafana-audit-001/`
- **Date opened**: 2026-05-21
- **Target version**: v0.10.1 (patch — pure observability fixes + new feature doc; no API/breaking changes anticipated)
- **Estimated effort**: 3–5 dev-days
- **OpenAI spend**: $0
- **Risk level**: Low–Medium (data-driven audit; coverage expansion is additive; fixes are JSON-only)

## 2. Problem Statement

The 8 dashboards at `deploy/docker/grafana/dashboards/*.json` (146 panels total) are the operator's primary observability surface for the MDEMG framework. Operator reports the surface is "diminished — many dashboards aren't reporting metrics." Initial spot-check (11-panel sample) found no SQL errors, correctly-wired datasources, populated TSDB tables (57M+ rows in `metric_samples`), and matching template-variable defaults — so either the perception is from older state since fixed, or there are specific broken panels the sample missed. This sprint runs a rigorous per-panel audit so we know exactly what's wrong (if anything), fixes by root cause, then closes the observability coverage gap for V0017–V0021 schema additions that haven't been wired into dashboards yet.

## 3. Scope & Constraints

### In scope

- Per-panel audit: execute every panel's rawSql against the live dev TSDB with safe template-variable substitutions; classify each panel `PASS` / `EMPTY` / `FAIL` / `SKIP`.
- Categorize failures by root cause (wrong column name, missing metric, wrong filter, time-range default mismatch, Grafana variable interpolation bug, deprecated feature).
- Fix per-panel JSON for failing panels — minimum-change edits, no architectural rewrites.
- Coverage expansion: add panels for the 10 unused tables (`sparse_gate_metrics` V0019, `context_catalog_versions` V0020, `model_install_events` V0021, `retrieval_audit` V0017, `embedding_events`, `guidance_conflicts` V0015, `rl_training_runs` V0013, `rl_training_steps` V0013, `uvts_results` V0016, `uvts_runs` V0016, `ft_hitl_decisions`).
- Enrich existing dashboards with high-value but currently-unused columns (e.g., `retrieval_events` has 23 columns, only 3 visualized).
- Tier 3 e2e: operator loads each dashboard in browser post-fix.
- New feature doc: `docs/features/observability-dashboards.md`.

### Out of scope

- New Grafana datasources / alerting backends.
- Browser-side UX work (panel layouts, colors).
- Server-side metric emission changes (gaps documented; new emission is a follow-up sprint).
- JSON → Grafana provisioned-dashboards migration.
- PromQL panel additions (codebase has Prometheus metrics but dashboards stay SQL-targeted per existing convention).

### Constraints

- Sequential epics (memory: `feedback_sequential_epics.md`).
- Tier 3 live testing required (memory: `feedback_live_testing_required.md`) — operator loads each dashboard in browser.
- Doc scaffolding before implementation; final docs Epic 7.
- No-hardcoding rule: any new query that filters on `space_id` / `instance` must use `$space_id` / `$instance` template variables — never literal `'mdemg-dev'` or `'localhost:9999'`.

## 4. Dependencies

- **Live TSDB** at `mdemg-timescaledb-1:5432` (exposed on port 5433 per `.env` `TSDB_PORT`). 22 tables, 57.6M rows in `metric_samples`, 134K in `metrics_hourly`, 88K in `llm_interactions`, all attributed to `space_id=mdemg-dev` / `instance=localhost:9999` in the last-24h window.
- **Live Grafana** at `mdemg-grafana-1` — for Epic 6 browser e2e.
- **Tooling**: `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics ...`; Python 3 for JSON dashboard parsing.
- **Existing dashboards**: `deploy/docker/grafana/dashboards/{mdemg-overview,mdemg-rsic,mdemg-jiminy,mdemg-j17,mdemg-llm-routing,mdemg-graph-topology,mdemg-neo4j,mdemg-ft-training}.json` (8 files, 146 panels).
- **Datasource provisioning** at `deploy/docker/grafana/provisioning/datasources/{timescaledb,neo4j}.yml` (verified correct in Phase 1).

## 5. Implementation Plan

### Epic 0 — Sprint plan + audit harness (~0.5 day) — IN PROGRESS

This document + `scripts/grafana_panel_audit.py` (panel-walking, template-var substitution, SQL execution via psql, classification) + `scripts/grafana_panel_audit_test.py` (Tier 1 unit tests).

### Epic 1 — Per-panel rigorous audit (~1 day)

Run harness against all 146 panels. Classify each:
- `PASS` — SQL executes successfully AND returns ≥1 row in default 24h window
- `EMPTY` — SQL executes successfully but returns 0 rows
- `FAIL` — SQL execution errors out
- `SKIP` — panel type doesn't have rawSql (text, row separator, etc.)

For each EMPTY: drill in (metric_name present? filter too narrow? time-range issue?).
For each FAIL: capture exact SQL error.

Commit `docs/development/grafana-audit-001/audit_results.json` (per-panel verdict + error + execution time) + `audit_summary.md` (per-dashboard verdict counts, per-root-cause breakdown).

### Epic 2 — Triage + operator surface (~0.5 day)

Write `findings.md` grouping failures by root cause:
- (a) SQL bug (typo, wrong column name)
- (b) Schema drift (column renamed / dropped in a migration)
- (c) Missing metric (server-side emission never wired)
- (d) Time-range default too narrow (metric is rare)
- (e) Template-variable interpolation issue
- (f) Stale panel (feature deprecated)

Post findings to PR comment. Operator confirms fix priorities.

### Epic 3 — Fix Tier 1: SQL bugs + schema drift (~1 day, gated on Epic 2)

Per-panel JSON edits to fix category (a) + (b) failures. Minimum-change edits — preserve panel layout/visualization type, only fix SQL. Re-run harness after each fix.

### Epic 4 — Fix Tier 2: missing metrics + time-range tuning (~0.5 day, gated on Epic 2)

- (c) Missing metric: document in feature doc "Known gaps"; defer emission to follow-up.
- (d) Time-range default too narrow: widen to `now() - interval '7 days'` for rare metrics.
- (f) Stale panel: remove with operator confirm.

### Epic 5 — Coverage expansion (~1 day)

Add panels for the 10 unused tables. Slot drafts:
- `model_install_events` (V0021) → new section in `mdemg-overview.json`: pull rate by quant, failure rate, latency
- `sparse_gate_metrics` (V0019) → new panel cluster in `mdemg-rsic.json`: active count distribution, threshold trends, floor/ceiling firing rate
- `context_catalog_versions` (V0020) → new panel in `mdemg-overview.json`: catalog version freshness, build cadence
- `retrieval_audit` (V0017) → enrich `mdemg-rsic.json`: scorer-version split, consensus_strength histogram
- `embedding_events` → enrich `mdemg-overview.json`: embedding rate, cache hit ratio
- `guidance_conflicts` (V0015) → enrich `mdemg-jiminy.json`: conflict count, per-source breakdown
- `rl_training_*` (V0013) → integrate into `mdemg-ft-training.json`: training-step loss curves, reward vector trends
- `uvts_*` (V0016) → new section in `mdemg-overview.json`: per-run aggregate score, A/B verdict counters
- `ft_hitl_decisions` → enrich `mdemg-ft-training.json`: HITL approval rate, queue depth

Each panel uses `$space_id` / `$instance` template variables. Harness PASS-check before commit.

### Epic 6 — Tier 3 live e2e (~0.5 day)

Operator loads each dashboard in browser, confirms panels render with data, screenshots, notes browser-only quirks (Grafana caching, time-range mismatches with harness).

### Epic 7 — Documentation Update — final epic, never cut

- `docs/features/observability-dashboards.md` (new): per-dashboard purpose, panel inventory by table source, refresh expectations, known gaps.
- `CHANGELOG.md` Unreleased → v0.10.1.
- `CLAUDE.md` Architecture Notes "Observability Dashboards" entry under Service Alert System section.
- `packaging/homebrew-mdemg/README.md` What's New v0.10.1.
- `docs/development/grafana-audit-001/post.md` — sprint close.

## 6. Testing Plan (3 tiers — required by memory rule)

**Tier 1 — Audit harness unit tests** (`scripts/grafana_panel_audit_test.py`):
- Template-variable substitution (`$space_id`, `$instance`, `$__timeFilter(time)`)
- Panel SQL extraction (handles `rawSql`, `sql`, nested `targets`, no-SQL panel types)
- Classification (row count thresholds, error capture)

**Tier 2 — JSON schema integrity**: every dashboard JSON parses; datasource UIDs all resolve to provisioned datasources.

**Tier 3 — Live e2e**: operator-confirmed browser load per dashboard + harness re-run post-fix.

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. Epic 1 = single audit-results commit. Epic 3/4 = one commit per dashboard fixed (max 8 commits). Epic 5 = one commit per coverage area added. Final commit promotes CHANGELOG Unreleased → v0.10.1.

## 8. Verification Checklist

- [ ] Harness runs cleanly against all 146 panels with no crashes
- [ ] `audit_results.json` covers every panel with verdict + execution time
- [ ] Every FAIL category has a root-cause classification in `findings.md`
- [ ] Epic 3 re-run shows zero new FAILs and ≥N% recovered (N decided after Epic 2)
- [ ] Epic 5 new panels all PASS the harness on first build
- [ ] Tier 3 browser e2e: 8/8 dashboards load, operator-confirmed
- [ ] No hardcoded `'mdemg-dev'` / `'localhost:9999'` strings in committed panel JSON (grep audit)
- [ ] `docs/features/observability-dashboards.md` ships
- [ ] CHANGELOG, CLAUDE.md, README, post.md updated

## 9. Documentation Update — Epic 7 above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Epic 1 finds zero real breakage (sample was representative) | Medium | Low | Sprint pivots to coverage-only; still delivers value |
| Browser-side breakage harness can't see (Grafana caching, range mismatches) | Medium | Medium | Epic 6 catches; operator screenshot review |
| New panel JSON breaks dashboard load | Low | High | Tier 2 schema test before commit; staged dashboard-by-dashboard |
| Coverage panels reference data-rare tables → look empty | High | Low | Document expected freshness; widen default time-range |
| Operator perception was older state, sprint produces nothing visible | Medium | Low | Epic 5 + feature doc still close coverage gap meaningfully |
| Harness false-positives (panel works for 24h but broken for older data) | Low | Low | Harness also checks `time > now() - interval '7 days'` as alt window |

## 11. Documents Accessed

- `deploy/docker/grafana/dashboards/*.json` (8 files, 146 panels)
- `deploy/docker/grafana/provisioning/datasources/{timescaledb,neo4j}.yml`
- `deploy/docker/grafana/provisioning/dashboards/dashboards.yml`
- `docker-compose.yml` (dashboard volume mounts)
- `internal/tsdb/migrations/011_..021_*.sql` (schema for column-rename audit)
- Live TSDB at `mdemg-timescaledb-1:5432` (table row counts, sample query results)

## 12. Rollback Procedures

- Dashboard JSON changes are pure-JSON; rollback = `git revert` of the specific commit. Grafana auto-reloads from disk (30s interval per `provisioning/dashboards/dashboards.yml`).
- No schema changes; no data migrations.
- New feature doc is additive; no doc rollback needed.
