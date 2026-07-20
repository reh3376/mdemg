# Sprint GRAFANA-PANEL-FILTER-001 — Filter caller-cancellation noise from all Grafana error-rate panels

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | GRAFANA-PANEL-FILTER-001 |
| Sprint Name | Filter caller-cancellation noise from all Grafana error-rate panels |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 0.5 dev-day |
| Sprint Line | grafana-panel-filter-001 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Disclosed follow-up from LLM-HEALTH-INVESTIGATION-001 (2026-07-20) |

## 2. Problem Statement

LLM-HEALTH-INVESTIGATION-001 E3 (2026-07-20) filtered `caller_canceled:%` errors out of the alert-rule signal (`internal/tsdb/dataset_builder.go::LLMPerformance`, `internal/tsdb/exporter.go::computeLLMQuality`, RSIC rule `alert_llm_health`). The `rsic-alert_llm_health` noise flood stopped.

DASHBOARD-TRUTH-002 triage (2026-07-20) confirmed the same filter is missing from **`mdemg-llm-routing.json`** — the "LLM error rate % by task_name" panel still counts every non-empty `error` string:

```sql
SUM(CASE WHEN error IS NOT NULL AND length(error) > 0 THEN 1 ELSE 0 END)
```

Live evidence: `retrieval.rerank_cross` shows 10.8% error rate (23/213 in 24h). **100% of those 23 errors are `caller_canceled:` rows** (0 real errors 24h; 26 canceled / 0 real / 212 clean in 7d). Zero user impact — rerank fails open — but the panel presents the pre-check working correctly as a red 10-25% error rate.

The operator observed the same discrepancy on the MDEMG LLM Routing dashboard and flagged it.

## 3. Scope & Constraints

**In scope**:
- Add `AND error NOT LIKE 'caller_canceled:%'` filter to every Grafana panel that reads a `llm_interactions.error`-based error rate.
- Grep-audit every dashboard JSON under `deploy/docker/grafana/dashboards/` for the patterns `error IS NOT NULL`, `length(error) > 0`, `error != ''`, `error <> ''`.
- Document the filter contract in the dashboards' area of the codebase so future panel authors follow it.
- Pin a CI test (or Go test using a JSON walk) that fails if a new panel reading `llm_interactions.error` lacks the filter.

**Out of scope**:
- Adding new error-string patterns to the filter (recorder emits ONLY `caller_canceled:` today; when it grows, both the alert rule and the panels update together via the pinned test).
- Changing the recorder's tagging behavior.
- Investigating other dashboards' non-`llm_interactions.error` panel defects (that's DASHBOARD-TRUTH-002).

**Constraints**:
- **No hardcoded values.** Any panel-level default that would need parameterization for future noise classes should reference the filter as a config-anchored contract (however the filter itself is a literal SQL clause — that's the recorder's tagged prefix, not a value).
- **Live Tier-3 required**: after edit, reload Grafana and verify the panel value drops from ~10.8% to 0% on `mdemg-dev`.
- **Filter contract source-of-truth**: mirror the alert rule (`dataset_builder.go::LLMPerformance`). Reference it in a code comment near the filter.

## 4. Dependencies & Pre-Conditions

- ✅ LLM-HEALTH-INVESTIGATION-001 shipped (E1 recorder tagging + E3 alert rule filter).
- ✅ Grafana dashboards live-mounted from `deploy/docker/grafana/dashboards/`.
- ✅ `mdemg-dev` substrate has recent `caller_canceled:` rows (23+ in 24h) for the before/after gauge check.
- ✅ Reviewer access to Grafana at `http://localhost:3000`.

## 5. Implementation Plan

**Sequential epics — do NOT parallelize.**

### E0 — Sprint plan committed
Commit this plan.

### E1 — Audit + inventory
Grep every file in `deploy/docker/grafana/dashboards/*.json` for the three patterns above. Report every hit with dashboard/panel/line. Split into:
- **Must-fix**: reads from `llm_interactions.error` — must have the filter.
- **Ambiguous**: reads from other tables' `error` columns — investigate context.
- **Safe**: not error-count SQL (e.g. `error` used as a display label).

**Gate**: audit is captured in `docs/development/grafana-panel-filter-001/audit.md`.

### E2 — Fix panels
For each must-fix panel: apply the SQL patch `error != '' AND error NOT LIKE 'caller_canceled:%'` (matching the exact form used in `dataset_builder.go::LLMPerformance`). Add a `-- GRAFANA-PANEL-FILTER-001` comment above the filter in the SQL so future authors see the contract.

**Gate**: `git diff deploy/docker/grafana/` shows only intended panel edits.

### E3 — Regression pin
Add `deploy/docker/grafana/dashboards_test.go` (or extend existing test) that:
- Loads every `.json` in the directory.
- Walks each panel's `targets[].rawSql`.
- For any query referencing `llm_interactions` AND `error`, asserts the query also contains `NOT LIKE 'caller_canceled:%'` OR the query is explicitly whitelisted with a comment (e.g. debug/introspection panels intentionally showing everything).

**Gate**: pin test PASSES against fixed dashboards; FAILS if a panel is missed.

### E4 — Live Tier-3
Reload Grafana (Grafana auto-reloads mounted dashboards). Open `mdemg-llm-routing`, view "LLM error rate % by task_name" panel, verify `retrieval.rerank_cross` shows **0.00%** (was ~10.8%). Screenshot or SQL cross-check.

**Gate**: before/after numbers captured in `docs/development/grafana-panel-filter-001/live_verification.md`.

### E5 — Canonical docs
- CHANGELOG [Unreleased] > Fixed entry.
- CLAUDE.md: extend the existing LLM-HEALTH-INVESTIGATION-001 note in place — close the disclosed follow-up.
- `docs/features/dashboard-metric-honesty.md` OR extend an existing feature doc — document the filter contract.
- Sprint `post.md`.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit** (E3): the JSON-walk pin test asserts the filter is present on every `llm_interactions.error` panel; simulates a "forgotten filter" case by injecting a synthetic malformed panel and asserting FAIL.

**Tier 2 — Integration**: `deploy/docker/grafana/dashboards_test.go` runs under `go test ./...`; existing dashboards must satisfy the contract on every CI run.

**Tier 3 — Live E2E** (E4):
- Restart Grafana (or wait for the file-watcher).
- SQL cross-check: `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c "SELECT task_name, ROUND(100.0*SUM(CASE WHEN error != '' AND error NOT LIKE 'caller_canceled:%' THEN 1 ELSE 0 END)/NULLIF(COUNT(*),0),2) AS honest_error_pct, ROUND(100.0*SUM(CASE WHEN error != '' THEN 1 ELSE 0 END)/NULLIF(COUNT(*),0),2) AS raw_error_pct FROM llm_interactions WHERE time > NOW() - INTERVAL '24 hours' GROUP BY 1;"` — panel must display the `honest_error_pct` column.
- Screenshot / annotated JSON before + after.

## 7. Commit Strategy

Conventional Commits; one commit per epic.

1. `docs(grafana-panel-filter-001): E0 — sprint plan`
2. `docs(grafana-panel-filter-001): E1 — dashboard audit`
3. `fix(grafana-panel-filter-001): E2 — filter caller-cancellation noise from LLM error-rate panels`
4. `test(grafana-panel-filter-001): E3 — pin test for filter-contract enforcement`
5. `docs(grafana-panel-filter-001): E4 — live Tier-3 verification`
6. `docs(grafana-panel-filter-001): E5 — CHANGELOG + CLAUDE.md + sprint post`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] `go test ./...` clean (pin test green)
- [ ] Working tree clean
- [ ] Live Tier-3 numbers captured in `live_verification.md`
- [ ] CHANGELOG + CLAUDE.md + sprint post committed
- [ ] Pushed to `reh3376_dev01`; auto-PR created

## 9. Documentation Update (Epic E5 — never cut)

- **CHANGELOG.md** [Unreleased] > Fixed: entry with live before/after numbers, references LLM-HEALTH-INVESTIGATION-001 as parent.
- **CLAUDE.md**: extend the existing LLM-HEALTH-INVESTIGATION-001 architecture note in place — add "**dashboard filter follow-up shipped as GRAFANA-PANEL-FILTER-001**" clause; pin the "when adding a panel that reads `llm_interactions.error`, apply the same filter as the alert rule; the pin test enforces this" rule.
- **Feature doc**: `docs/features/dashboard-metric-honesty.md` OR extend existing hooks — filter contract documented; who owns adding new noise classes.
- **Sprint post**: `docs/development/grafana-panel-filter-001/post.md`.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Audit misses a panel because the SQL is dynamically templated | Low | Pin test walks JSON AST including template variables; whitelist explicitly required |
| Grafana file-watcher doesn't reload the JSON edit | Low | `docker restart mdemg-grafana` as fallback; noted in Tier-3 procedure |
| A panel intentionally shows RAW error rate (debug/forensic) and shouldn't be filtered | Low | E3 pin test supports an explicit `-- GRAFANA-PANEL-FILTER-001: intentionally unfiltered` comment as whitelist mechanism |
| Future noise class added to recorder but not to alert-rule / dashboard filter | Medium | E5 doc pins the "recorder→alert→dashboard" contract; the alert rule test in `internal/ape/self_reflect_test.go` and this new pin test become the enforcement pair |
| Filter accidentally hides a real error class that happens to match the LIKE pattern | Very low | Prefix `caller_canceled:` is recorder-controlled; only that recorder emits it |

## 11. Rollback Procedures

- **Data**: N/A (no data changes).
- **Code**: revert the panel-edit commit; the pin test edit is safe to keep or revert independently.
- **Grafana**: dashboards revert on next commit; no persistent Grafana state changes.

## 12. Documents Accessed

- `docs/development/llm-health-investigation-001/` (parent sprint)
- `deploy/docker/grafana/dashboards/mdemg-llm-routing.json`
- `internal/tsdb/dataset_builder.go` (LLMPerformance — filter source-of-truth)
- `internal/tsdb/exporter.go` (computeLLMQuality)
- `internal/ape/self_reflect.go` (alert_llm_health rule)
- `internal/llmclient/client.go` (recorder tagging)
- CLAUDE.md § LLM error-rate honesty
- CHANGELOG.md § LLM-HEALTH-INVESTIGATION-001 entry
- DASHBOARD-TRUTH-002 triage report (this session's findings)
