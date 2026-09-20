# JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint ID**: JIMINY-METRIC-PARTITION-ALERTS-PANELS-001
- **Task**: (linear ticket to follow)
- **Parent arc**: JIMINY-METRIC-PARTITION-001 (task #158) — Path 3 shipped 2026-09-10
- **Roadmap slot**: Q5 §3 #1 (formalized 2026-09-20 in ROADMAP_2026Q5.md)
- **Author**: Roger Henley (via Claude Opus 4.7)
- **Date**: 2026-09-20
- **Effort**: 1 day
- **Impact class**: direct (observability closes the arc loop)
- **Ship gate**: additive alert rules + Grafana panels, backward-compat aggregate

## 2. Problem Statement

JIMINY-METRIC-PARTITION-001 (#158, 2026-09-10) shipped 4 per-verifiability-class follow-rate gauges. Live 24h averages on mdemg-dev:

| Metric | 24h avg | Interpretation |
|---|---|---|
| `mdemg_jiminy_follow_rate` (aggregate — dishonest) | 0.173 | Class-mix-dominated |
| `mdemg_jiminy_follow_rate_classifier_verifiable` | 0.169 | Honest classifier-only rate |
| `mdemg_jiminy_follow_rate_process_verifiable` | 0.737 | Post-OBSERVER arc, ~5:1 followed:missed |
| `mdemg_jiminy_follow_rate_hybrid` | 0.223 | Small denominator |
| `mdemg_jiminy_follow_rate_human` | 0.000 | Dormant (no writer — awaits JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001) |

Two closure gaps for observability:
1. **Alerts**: only the aggregate `jiminy_follow_rate_drop` rule exists (reads `mdemg_jiminy_follow_rate`). Per JIMINY-CEILING-INVESTIGATION-002's pinned arch rule, the aggregate ceiling is bounded by class mix — a class-specific alert is the honest signal.
2. **Panels**: `mdemg-jiminy.json` shows aggregate + Actionable Compliance Rate but no per-class panels. Operators can't see at a glance which class dropped.

## 3. Scope & Constraints

**In-scope**:
- 3 new alert rules (classifier / process / hybrid) via `alert.JiminyFollowRateClassRules(map[string]float64)` factory. Human class deferred (no writer yet; sub-floor at 0.0 always fires unless disabled).
- 3 new config knobs for per-class floors + wire in `serve.go`.
- Retire aggregate: default `JIMINY_FOLLOW_RATE_ALERT_FLOOR` **0.05 → 0** (disables aggregate alert). Aggregate GAUGE retained per #158 pin. Operators with explicit `.env` value keep the aggregate.
- 5 new Grafana panels on `mdemg-jiminy.json`: 4 stat panels + 1 overlaid timeseries. Sync staged embed via `make sync-grafana-embed`.
- Pin tests for the 3 new rules (COALESCE presence, distinct Service, threshold operator, floor≤0 disables).

**Out-of-scope** (deferred / separate sprints):
- `mdemg_jiminy_follow_rate_human` writer — filed as JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (Q5 §3 #2).
- Retrain gate re-evaluation — timer-blocked (Q5 §3 #4/#5).
- Panel-level class-specific narrative bands beyond the alert floor — small addition, could fold into E2 if trivial.

**Constraints**:
- `must-follow-12-section-format` — this doc ✓
- `must-use-cuid2` — no new identifiers minted
- `never-hardcode-config` — all floors env-tunable with sensible defaults
- `must-use-uxts-frameworks-consistently` — no new UxTS specs (alert rules aren't a UxTS surface; existing `alert.rules_test.go` pattern covers unit tests)
- `never-direct-alter-schema` — zero migrations
- `unit-integration-e2e-docs` — 3 tier plan below
- `live-testing-tier-required` — Tier 3 = real evaluator against real TSDB rows, observe fire/quiet per rule
- `end-with-docs-accessed` — §12 populated
- **NOSILENT-001 cooldown-key contract** — each rule MUST have a distinct `Service` label
- **TSDB-CONSUME-001 idle-safe SQL** — `COALESCE(AVG(value), 1.0)` on `lt` operator, no `ORDER BY … LIMIT 1`
- **ALERT-TRUTH-001 pin** — windowed AVG, not LIMIT 1
- **DASHBOARD-TRUTH-002/003 panel-title contract** — embed honest steady-state number in panel titles
- **HEBB-ETA-001** — behavior-changing flags must default off in code AND `.env`. This sprint changes the aggregate alert's DEFAULT off in code; the .env's explicit `JIMINY_FOLLOW_RATE_ALERT_FLOOR=0.05` (if present) is preserved.

## 4. Dependencies

- **Upstream (must be live)**:
  - JIMINY-METRIC-PARTITION-001 (#158) — 4 class gauges emit ✓ (live-verified)
  - JIMINY-PROCESS-OBSERVER-{01..06} (#160-#165) — process gauge has real rows ✓
  - `internal/alert/rules.go::JiminyFollowRateRules` — extraction pattern to mirror ✓
  - `internal/grafanapin` sync target — pin tests + drift check ✓

- **Downstream (unblocked when this ships)**:
  - JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 — sibling; when human writer ships, the human floor can flip from 0 to a real number
  - JIMINY-CEILING-BREAK-2 T+30d passive re-measure — the honest classifier-only alert is the signal that gate decision will re-derive from

## 5. Implementation Plan

Sequential epics per `sequential-epics` rule; each epic completes fully before the next begins.

### Epic 1 — Alert rules factory (backend)

- **E1.1**: New `alert.JiminyFollowRateClassRules(floors map[string]float64) []AlertRule` in `internal/alert/rules.go`. Mirrors `JiminyFollowRateRules` shape. Returns one rule per non-zero floor keyed on class name. Distinct Service per rule (NOSILENT-001).
- **E1.2**: 3 config fields in `internal/config/config.go`:
  - `JiminyFollowRateClassifierFloor` default **0.10** (below live 0.169)
  - `JiminyFollowRateProcessFloor` default **0.50** (below live 0.737 with margin for organic dip)
  - `JiminyFollowRateHybridFloor` default **0.15** (below live 0.223)
- **E1.3**: FromEnv wiring. Env var names: `JIMINY_FOLLOW_RATE_CLASSIFIER_FLOOR`, `_PROCESS_FLOOR`, `_HYBRID_FLOOR`. Add doc-envs allowlist entries if the verifier flags them (should auto-resolve).
- **E1.4**: Change `JiminyFollowRateAlertFloor` default `0.05 → 0` in code (aggregate alert disabled by default). Retain aggregate gauge per #158 pin. Update field comment to explain the retirement + supersession.
- **E1.5**: Wire in `internal/cli/serve.go` — append `JiminyFollowRateClassRules(...)` to the rule set assembled alongside the existing `JiminyFollowRateRules(...)` call.
- **Gate**: `go build ./...` clean; `go test ./internal/alert/...` green; `golangci-lint run ./internal/alert/... ./internal/config/... ./internal/cli/...` 0 issues.

### Epic 2 — Grafana panels

- **E2.1**: Edit `deploy/docker/grafana/dashboards/mdemg-jiminy.json`. Add 5 panels in a new row `"Follow Rate by Verifiability Class (JIMINY-METRIC-PARTITION-001)"`:
  - 4 stat panels (classifier / process / hybrid / human), each reading its gauge over the last 30 min. Thresholds per class-floor. Panel titles embed the honest steady-state number in parens (DASHBOARD-TRUTH-002/003 pattern: e.g., `Classifier-verifiable follow rate (honest ~17% at 2026-09-20)`).
  - 1 timeseries overlaying all 4 classes on a 168h window. Legend calc: mean + lastNotNull.
  - Datasource UID = literal `timescaledb` (ARC-TRAJECTORY-PANEL-001 arch rule: provisioned dashboards use literal UIDs, not `${DS_ENVVAR}` form).
- **E2.2**: `make sync-grafana-embed` to update `internal/cli/grafana_templates/staged/`.
- **E2.3**: `make verify-grafana-embed` clean. `internal/grafanapin` tests green (adds panel-count pin bump).
- **Gate**: Grafana JSON valid; sync clean; `go test ./internal/grafanapin/...` green.

### Epic 3 — Unit + integration pin tests

- **E3.1**: New pin `TestJiminyFollowRateClassRules_ShapeAndDefaults` in `internal/alert/rules_test.go`:
  - Empty/zero floors → nil (skip)
  - Non-zero floors → one rule per non-zero class; distinct Service per rule; `lt` operator; COALESCE in SQL; correct metric_name in each rule's SQL
  - Threshold matches floor exactly
- **E3.2**: Existing `TestAllRules_DistinctServicePerSeverity` + `TestAllRules_NoLimitOneAntiPattern` sweep-tests auto-cover the new rules (both walk `DefaultRules()` return + all extracted factories per ALERT-TRUTH-001).
- **E3.3**: Verify `internal/alert/rules_test.go::TestMetricSamplesRules_UseTimeColumn` still passes (new SQL reads `time` column per TSDB-CONSUME-001).
- **Gate**: `go test ./internal/alert/... -count=1` all green.

### Epic 4 — Live Tier-3 smoke (mdemg-dev)

- **E4.1**: Kickstart server after E1-E3 land in binary.
- **E4.2**: For each new rule, `curl` the evaluator's rule listing (or watch server log for rule count) — verify boot log shows the 3 new rules loaded + `jiminy_follow_rate_drop` shows `Enabled=false` (or missing when floor=0).
- **E4.3**: Force-evaluate each rule via a direct SQL query mirroring the rule body — verify each returns exactly one non-null row on healthy live data. Verify NONE fire (all values above floor).
- **E4.4**: Optional negative smoke — temporarily lower one floor via `launchctl setenv` to above the live value + restart, watch for the fire, then reset. Cleanup any fired alert.
- **Gate**: 3 rules loaded; 3 queries return valid rows; 0 firing on default floors; alert file `~/.mdemg/alerts/current.json` has no new class-fires post-smoke.

### Epic 5 — Documentation

- **E5.1**: Extend `docs/features/jiminy-metric-partition.md` with §"Per-class alerts + panels". Names the 3 new env vars, the 5 panels, the retirement of the aggregate alert. Cross-references JIMINY-CEILING-INVESTIGATION-002 arch rule.
- **E5.2**: New CLAUDE.md architecture note under §Architecture Notes. Mirrors JIMINY-METRIC-PARTITION-001's shape: names 3 env vars, panel list, retirement of aggregate, pinned arch rule.
- **E5.3**: CHANGELOG.md Unreleased/Added entry.
- **E5.4**: Sprint post `docs/development/jiminy-metric-partition-alerts-panels-001/sprint_post.md`.

## 6. Testing Plan

**Tier 1 — Unit (isolated)**:
- `TestJiminyFollowRateClassRules_ShapeAndDefaults` (new, 5+ subcases)
- `TestJiminyFollowRateClassRules_FloorZeroDisablesClass` (new)
- `TestJiminyFollowRateClassRules_DistinctServicesPerClass` (new)
- Existing sweep tests auto-cover new rules (LIMIT-1 anti-pattern, distinct-service cooldown key, `time` column contract)

**Tier 2 — Integration**:
- `go test ./internal/config/...` — new field wiring
- `go test ./internal/alert/...` — full rules suite green
- `go test ./internal/grafanapin/...` — panel-count + datasource-UID + query-shape pin

**Tier 2b — Extended integration (mocked)**:
- N/A this sprint (rules are queried against real TSDB; no mock path needed)

**Tier 3 — Live e2e** (mdemg-dev, real binary against real TSDB):
- Rule loading verified via boot log
- Per-rule direct SQL executed against real `metric_samples` returns 1 non-null row
- Grafana panel renders on the real dashboard (each panel returns data via HTTP fetch to `:3000/api/dashboards/uid/mdemg-jiminy`)
- Alert file inspected — no new class-fires under default floors + healthy live data

## 7. Commit Strategy

Sequential commits per epic; small, verified, one purpose each. Per Phase 11.6.2 rule: surprise bugs during live smoke get their own follow-up fix-commit — do NOT roll into the sprint commit.

1. `feat(alert): per-verifiability-class follow-rate rules + config (E1)`
2. `feat(grafana): per-class follow-rate panels on mdemg-jiminy.json (E2)`
3. `test(alert): pin per-class follow-rate rule shape + defaults (E3)`
4. `docs: JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 sprint post + feature doc + CLAUDE.md + CHANGELOG (E5)`

Live smoke (E4) doesn't produce commits unless it surfaces a defect.

Every commit trailer includes `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./internal/alert/... ./internal/config/... ./internal/grafanapin/... -count=1` green
- [ ] `golangci-lint run ./internal/alert/... ./internal/config/... ./internal/cli/...` 0 issues
- [ ] `python3 scripts/verify_doc_env_vars.py --strict` clean
- [ ] `python3 scripts/verify_metrics_consumers.py` clean (no new metrics declared; new alerts consume existing gauges)
- [ ] `python3 scripts/verify_tsdb_consumers.py` clean (no new tables)
- [ ] `python3 scripts/verify_route_consumers.py` clean (no new routes)
- [ ] `python3 scripts/verify_config_consumers.py` clean (3 new config fields wired)
- [ ] `make sync-grafana-embed` + `make verify-grafana-embed` clean
- [ ] Live Tier-3 smoke: boot log shows `matcher_count=6` + all rule counts unchanged in shape, 3 new class rules registered; direct SQL for each of 3 rules returns 1 row; 0 firing
- [ ] Sprint post reflects actual live-smoke values, not planning-time estimates
- [ ] PR summary comment posted per `must-comment-sprint-summary-on-pr`

## 9. Documentation Update (final epic — never cut)

- Feature doc `docs/features/jiminy-metric-partition.md` — new §Per-class alerts + panels
- CLAUDE.md architecture note (§Architecture Notes)
- CHANGELOG.md Unreleased/Added
- Sprint post `docs/development/jiminy-metric-partition-alerts-panels-001/sprint_post.md`
- ROADMAP_2026Q5.md §3 #1 checkbox update (or verification note if Q5 doc is post-facto-only)

## 10. Risks & Mitigations

| Risk | Class | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Human-class rule fires immediately at 0.0 | class-error | HIGH if not gated | LOW (noisy alert) | Human floor defaults to 0 in factory → skipped entirely. Explicit doc + comment. |
| Process-class steady state above 0.50 is artifact of low sample (~2 outcomes/day) — variance drops the daily mean below the floor once natural traffic accumulates | tuning-drift | MED at T+30d | LOW-MED (noise, not correctness) | Docs name floor as first-cut; passive re-check in 14-30d with recalibrate pattern (mirrors LEVER-C-TIGHTEN-001 T+7d verdict). |
| Aggregate alert retirement surprises operator who relied on `jiminy_follow_rate_drop` firing | operational | LOW | LOW | Operators with explicit `JIMINY_FOLLOW_RATE_ALERT_FLOOR` in `.env` keep aggregate. Code default flip is fresh-install-only. |
| Grafana panel row bumps count and breaks `internal/grafanapin` pin | drift | LOW (CI-visible) | LOW | Update pin count in same commit as JSON edit. |
| New env vars slip DOC-CURRENCY-002 allowlist | doc-drift | LOW | LOW | `verify_doc_env_vars.py --strict` in verification list; add to allowlist with reason if flagged. |

## 11. Rollback Procedures

Non-destructive (no substrate mutation, no migrations, no schema bumps). Every change is code-level + a single Grafana JSON diff. Rollback shape:

1. `git revert <commit-shas>` — undoes rule factory + config wiring + panels + tests + docs
2. `make sync-grafana-embed` — regenerates staged embed from post-revert state
3. Restart mdemg — rules unwire cleanly; alert file `~/.mdemg/alerts/current.json` retains history but no new class-fires
4. Operators who flipped `.env` per-class floors during the sprint window can leave those env vars; they become no-ops post-revert

Zero data loss; zero rollback ambiguity.

## 12. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #1 slot
- `docs/development/jiminy-metric-partition-001/sprint_post.md` — the parent (Path 3)
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md` — taxonomy source
- `docs/development/jiminy-ceiling-investigation-002/sprint_post.md` — arch rule ("aggregate over mixed classes is dishonest")
- `docs/development/phase-4b-gate-decision-001/sprint_post.md` — signal-density gate
- `docs/development/arc-trajectory-panel-001/sprint_post.md` — Grafana datasource-UID contract
- `internal/alert/rules.go` — `JiminyFollowRateRules` extraction pattern
- `internal/alert/rules_test.go` — sweep-tests + shape assertions
- `internal/config/config.go` — `JiminyFollowRateAlertFloor` field + FromEnv
- `internal/cli/serve.go` — rule wire-up site
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` — dashboard target
- `internal/grafanapin/dashboards_test.go` — pin coverage
- `CLAUDE.md` — §Architecture Notes for the new pin
- `CHANGELOG.md` — Unreleased/Added
- Live TSDB via `docker exec mdemg-timescaledb-1 psql` for gauge sample counts + 24h averages
