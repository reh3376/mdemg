# JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 — Sprint Post

**Ship date**: 2026-09-20
**Wall-clock**: ~2h (plan → E1 → E2 → E3 → E4 live-recal → E5)
**Status**: SHIPPED default-on (per-class rules ENABLED; aggregate rule DISABLED by code default)
**Parent arc**: JIMINY-METRIC-PARTITION-001 (task #158)
**Roadmap slot**: Q5 §3 #1

## Goal delivered

Closes the observability loop the parent sprint left as a follow-up:

- **3 active alert rules** (classifier / process / hybrid) reading the class-partitioned gauges shipped in #158, with distinct Service names per NOSILENT-001 cooldown-key contract.
- **1 dormant rule** (human) at floor=0, awaiting JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 to ship the writer.
- **Aggregate alert retired** (code default 0.05 → 0). Class-mix-dominated aggregate is dishonest per JIMINY-CEILING-INVESTIGATION-002; per-class rules are the honest scoreboard.
- **5 new Grafana panels** on `mdemg-jiminy.json` — 4 stat + 1 timeseries overlay — in the row `Follow Rate by Verifiability Class`.

## What shipped

| Component | File | Change |
|---|---|---|
| Alert factory | `internal/alert/rules.go` | New `JiminyFollowRateClassRules(map[string]float64)` — mirrors `JiminyFollowRateRules` shape; distinct Service per class; floor ≤ 0 skips |
| Aggregate rule doc | `internal/alert/rules.go` | Docstring updated to name the supersession + retirement mechanism |
| Config fields | `internal/config/config.go` | 4 new fields: `JiminyFollowRate{Classifier,Process,Hybrid,Human}Floor` + FromEnv wiring |
| Aggregate default | `internal/config/config.go` | `JIMINY_FOLLOW_RATE_ALERT_FLOOR` default 0.05 → 0 (disables) |
| Rule wire-up | `internal/cli/serve.go` | Append `JiminyFollowRateClassRules(...)` after the aggregate call |
| Grafana panels | `deploy/docker/grafana/dashboards/mdemg-jiminy.json` | +6 panels (row header + 4 stat + 1 timeseries) |
| Grafana embed | `internal/cli/grafana_templates/staged/` | `make sync-grafana-embed` |
| Unit tests | `internal/alert/rules_test.go` | 3 new: shape+defaults, floor-zero-disables, distinct-services |
| Feature doc | `docs/features/jiminy-metric-partition.md` | New §Per-class alerts + panels |
| Roadmap | `docs/development/roadmap/ROADMAP_2026Q5.md` | §3 #1 closed (this sprint) |
| CLAUDE.md | new arch note | Two rules pinned |
| CHANGELOG.md | Unreleased/Added entry | Per-sprint entry |

## Config defaults shipped

| Env var | Default | Rationale |
|---|---|---|
| `JIMINY_FOLLOW_RATE_CLASSIFIER_FLOOR` | 0.10 | Below live 24h 0.169; retrain-gate class per PHASE-4B-GATE-DECISION-001 |
| `JIMINY_FOLLOW_RATE_PROCESS_FLOOR` | **0.25** (recalibrated live) | Live 30-min window 0.397-0.43; 24h 0.737 |
| `JIMINY_FOLLOW_RATE_HYBRID_FLOOR` | 0.15 | Below live 24h 0.223 |
| `JIMINY_FOLLOW_RATE_HUMAN_FLOOR` | 0 | Disabled — writer pending |
| `JIMINY_FOLLOW_RATE_ALERT_FLOOR` (aggregate) | 0 (was 0.05) | Retired — class-mix-dominated |

## Live Tier-3 (mdemg-dev, 2026-09-20)

```
alert evaluator started rules=35 interval=30s

mdemg_jiminy_follow_rate_classifier_verifiable  = 0.175  (floor 0.10)  OK
mdemg_jiminy_follow_rate_process_verifiable     = 0.397  (floor 0.25)  OK
mdemg_jiminy_follow_rate_hybrid                 = 0.400  (floor 0.15)  OK
mdemg_jiminy_follow_rate_human                  = 0      (disabled)    skipped

new class fires in ~/.mdemg/alerts/current.json: 0
```

All 3 active rules query real TSDB via `COALESCE(AVG(value), 1.0) FROM metric_samples WHERE metric_name = '...' AND time > now() - interval '30 minutes'` and return exactly one non-null row per rule.

## Live-caught recalibration (worth pinning)

**Defect surfaced mid-smoke**: process floor's first-cut default was **0.50**, calibrated against the 24h average (0.737). Live 30-min window returned **0.432** — the rule's actual query — which fired the alert on healthy data.

**Root cause**: The **FOLLOW-RATE-CALIBRATE-001 arch rule** requires floor to sit BELOW the *live steady state* — but "steady state" is window-dependent. For sparse denominators (process class has ~2 outcomes/day/matcher × 6 matchers ≈ 12/day), the 30-min window swings widely while the 24h mean smooths those swings.

**Fix**: recalibrated to **0.25** — well below observed 30-min window on healthy data. Documentation named the recalibration as a live-smoke lesson.

**Arch rule pinned**: per-class alert rules MUST calibrate against the SAME time window the rule queries, not the smoothed dashboard number. Verify live SQL matching the rule body before shipping.

## Verification checklist (all green)

- ✅ `go build ./...` clean
- ✅ `go test ./internal/alert/... ./internal/config/... ./internal/grafanapin/... ./internal/cli/... -count=1` green
- ✅ `golangci-lint run ./internal/alert/... ./internal/config/... ./internal/cli/...` 0 issues
- ✅ `python3 scripts/verify_doc_env_vars.py --strict` clean (4 new env vars)
- ✅ `python3 scripts/verify_metrics_consumers.py` clean (no new metrics; alerts consume existing gauges)
- ✅ `python3 scripts/verify_config_consumers.py` clean (4 new config fields wired)
- ✅ `python3 scripts/verify_tsdb_consumers.py` clean (no new tables)
- ✅ `python3 scripts/verify_route_consumers.py` clean (no new routes)
- ✅ `make sync-grafana-embed` + `make verify-grafana-embed` clean
- ✅ Live Tier-3: boot log `rules=35`, 3 rules load, direct SQL returns non-null, 0 firing, alert file quiet
- ✅ Sprint post reflects actual live-smoke values (not planning-time estimates)

## Arch rules pinned to CLAUDE.md

1. **Per-class alert rules MUST calibrate against the same time window the rule queries**, not the smoothed 24h dashboard number — sparse-denominator metrics swing widely on short windows and a floor sitting between window-observed and 24h-observed fires chronically. Verify live SQL matching the rule body before shipping.
2. **When retiring an aggregate rule with a shipped default floor**, retire it by flipping the code default to 0 (disables) rather than removing the factory function — operators with explicit `.env` values keep working, the aggregate GAUGE stays (some panels still read it), and rollback is a one-line config default change.

## Not shipped (deferred)

- **`mdemg_jiminy_follow_rate_human` writer** — sibling sprint JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (Q5 §3 #2)
- **T+30d passive re-check** — process floor may drift; recalibrate downward if denominator widens
- **Per-class narrative bands beyond the alert floor** — panels use a simple 2-color threshold (yellow≥floor / green≥floor+0.05); more nuanced bands (dashboard-truth-shape) can ship later if operator sees value

## Rollback

Non-destructive. `git revert <commit-shas>` undoes rule factory + config wiring + panels + tests + docs. `make sync-grafana-embed` regenerates staged embed from post-revert state. Restart mdemg — rules unwire cleanly. Any per-class `.env` values set during the sprint window become no-ops post-revert. Zero data loss.

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #1 slot
- `docs/development/jiminy-metric-partition-001/sprint_post.md` — parent
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md` — taxonomy source
- `docs/development/jiminy-ceiling-investigation-002/sprint_post.md` — arch rule
- `docs/development/phase-4b-gate-decision-001/sprint_post.md` — signal-density gate
- `docs/development/arc-trajectory-panel-001/sprint_post.md` — Grafana datasource-UID contract
- `docs/development/follow-rate-calibrate-001/` — floor-calibration precedent
- `internal/alert/rules.go` — extraction pattern
- `internal/alert/rules_test.go` — sweep-tests + shape assertions
- `internal/config/config.go` — field + FromEnv wiring
- `internal/cli/serve.go` — rule wire-up
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` — dashboard target
- `internal/grafanapin/dashboards_test.go` — pin coverage
- `CLAUDE.md` — arch notes
- `CHANGELOG.md` — Unreleased/Added
- Live TSDB via `docker exec mdemg-timescaledb-1 psql` — sample counts + 30m + 24h averages
- Live server log at `~/.mdemg/logs/server.log` for boot-time rule count verification
