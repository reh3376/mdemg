# Sprint Plan — GRAFANA-AUDIT-002

## 1. Header & Metadata
- **Sprint ID:** GRAFANA-AUDIT-002
- **Line:** `docs/development/grafana-audit-002/`
- **Date:** 2026-06-25
- **Target version:** patch (observability — dashboard correctness + new-metric panels)
- **Effort:** ~1–1.5 dev-days
- **OpenAI spend:** $0
- **Risk:** Low (Grafana dashboard JSON edits + panel additions; no app/schema change). Provisioned dashboards reload on Grafana restart.

## 2. Problem Statement
The 8 Grafana dashboards were last audited 2026-05-21 (GRAFANA-AUDIT-001). ~10 metric-changing sprints have shipped since. Phase-1 re-audit (`audit_summary.md`): **139 PASS / 7 EMPTY / 0 FAIL / 18 SKIP** — no broken queries, but 7 EMPTY panels need triage and several new operator-valuable gauges have **no panel** (notably `mdemg_jiminy_surfaced_actionable_fraction`/`_abstraction_fraction`, `neo4j_graph_null_weight_edges`, `neo4j_conversation_coverage_ratio`).

## 3. Scope & Constraints
**In scope:**
- **EMPTY triage:** for each of the 7 EMPTY panels, classify legit-empty (no events in-window — document with a panel note / leave) vs dead (wrong metric/column — fix or remove). Specifically verify `overview/Request Latency Distribution` + the `ft-training` watchdog panels.
- **New-metric panels:** dedup the code-vs-dashboard inventory to the genuinely-new, valuable, currently-unpanelled gauges, and add panels on the right dashboard (jiminy_surfaced_* → `mdemg-jiminy`; null-weight-edges + coverage → `mdemg-graph-topology`/`mdemg-neo4j`; etc.).
- **UOBS/UOTS gate:** update the dashboard test spec(s) so the new panels + the audit baseline are CI-checkable (the drift the UXTS-CI-001 zombie-hunt cares about).
- Re-run `grafana_panel_audit.py` to confirm post-change verdict; commit `audit_results.json` + `audit_summary.md` (Phase 1) and the post-change re-audit.

**Out of scope:**
- App/metric code changes (this is a dashboard sprint — if a metric is missing entirely, that's a separate producer sprint, noted not built).
- New metric *instrumentation*; only panels for already-emitted metrics.
- Alert-rule changes (owned by the evaluator; TSDB-CONSUME-001 line).
- The genuinely-legit-empty panels stay (documented), not deleted.

**Constraints:** sequential epics; no-hardcoding (panels use template vars `$space_id`/`$instance`); provisioned JSON must stay valid (Grafana schema); Tier-3 = the live re-audit + a Grafana reload showing the new panels render.

## 4. Dependencies
- `scripts/grafana_panel_audit.py` (harness, GRAFANA-AUDIT-001) — present, just used.
- `deploy/docker/grafana/dashboards/*.json` (8 dashboards).
- Live `mdemg-timescaledb-1` + the metrics already emitted by the running server.
- `docs/tests/uobs/` (UOBS) + the UOTS dashboard spec for the CI gate.

## 5. Implementation Plan (sequential epics)
**Epic 0 — Phase-1 audit + plan (this + `audit_summary.md`).** Done.

**Epic 1 — EMPTY triage + unpanelled-metric dedup.** For each EMPTY panel, run its substituted SQL with a wide window + check the underlying table; classify legit-empty vs dead. Dedup the 83-candidate inventory to the real unpanelled set (filter test_* fixtures; resolve `mdemg_`-prefix/Prometheus false positives by checking each against the dashboard exprs). Output a decision table (fix / add-panel / remove / document). *Gate: every EMPTY + every new metric has an adjudicated disposition.*

**Epic 2 — Panel fixes + additions.** Apply Epic 1's dispositions to the dashboard JSON: fix/remove dead-EMPTY panels; add panels for the new gauges with correct datasource, `$space_id` templating, units, and thresholds. *Gate: JSON valid; Grafana reload renders each new/changed panel.*

**Epic 3 — Re-audit + UOBS gate.** Re-run `grafana_panel_audit.py` → confirm EMPTY count drops / no new FAIL; update the UOBS/UOTS dashboard spec so the new panels are CI-covered (drift-gated). *Gate: re-audit clean; spec updated + hash-pinned.*

**Epic 4 — Documentation.** `post.md`, CHANGELOG, CLAUDE.md observability note, OUTSTANDING_BACKLOG ledger update (mark C done).

## 6. Testing Plan (3 tiers)
- **Tier 1:** `grafana_panel_audit.py` unit tests (existing 17) stay green; JSON schema-validity of edited dashboards.
- **Tier 2:** the per-panel SQL re-audit (substituted-query execution against live TSDB).
- **Tier 3 (live):** Grafana (`:3000`) reload — each new/changed panel renders with data (or a documented legit-empty); the re-audit verdict recorded.

## 7. Commit Strategy
Per-epic commits. Phase-1 artifacts (audit_results.json, audit_summary.md, plan) in the Epic 0 commit. Final commit promotes CHANGELOG + backlog ledger.

## 8. Verification Checklist
- [ ] Each of the 7 EMPTY panels adjudicated (fixed / removed / documented legit-empty)
- [ ] `overview/Request Latency Distribution` returns data or its query is corrected
- [ ] New panels added for the deduped unpanelled gauges (jiminy_surfaced_* at minimum)
- [ ] All edited dashboard JSON is schema-valid; Grafana reloads cleanly
- [ ] Re-audit: 0 FAIL, EMPTY reduced, recorded in audit_summary
- [ ] UOBS/UOTS dashboard spec updated + hash-pinned (CI drift gate)
- [ ] CHANGELOG + CLAUDE.md + backlog ledger updated

## 9. Documentation Update — Epic 4 above.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| A panel "EMPTY" is actually a dead metric mistaken for legit-empty | Med | Med | Epic 1 checks the underlying table directly, not just the windowed query |
| New panel uses wrong datasource/templating → renders blank | Med | Low | Mirror an existing working panel on the same dashboard; Tier-3 Grafana reload verifies |
| Inventory false-positives waste effort on already-panelled metrics | Med | Low | Epic 1 dedup step resolves prefix/Prometheus cases before any panel work |
| Provisioned JSON invalidated → Grafana fails to load dashboard | Low | High | JSON schema-validate each edit; reload check in Tier-3 |

## 11. Documents Accessed
- `scripts/grafana_panel_audit.py`, `docs/development/grafana-audit-002/audit_results.json` + `audit_summary.md`
- `deploy/docker/grafana/dashboards/*.json` (8), `docs/development/grafana-audit-001/` (prior audit)
- `internal/metrics/collectors.go` + per-sprint metric registrations; `docs/tests/uobs/`

## 12. Rollback Procedures
Dashboard JSON is provisioned + version-controlled — revert the Epic 2 commit to restore prior panels; Grafana reloads the reverted JSON. No data/app impact.
