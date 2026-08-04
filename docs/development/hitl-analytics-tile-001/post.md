# HITL-ANALYTICS-TILE-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** Q4-disclosed follow-up "HITL analytics tile — DORMANT-CENSUS-002-family idea; a Grafana panel over `review_grades` for grade-cadence + auto-vs-operator split visibility." Closes the observability loop for the HITL curation arc shipped this session (HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 + AUTOGRADE-SCHEDULE-001).

## Verdict

Shipped. New `mdemg-hitl` Grafana dashboard with 6 panels covering the full HITL curation lifecycle: grade cadence (auto vs operator), 7d share breakdown, pending queue depth, all-time draft-status split, scheduled autograde run history, and 7d substrate-mutation count. Live-verified: dashboard loaded via provisioning reload, all 6 panel SQL queries return real data from mdemg-dev.

## What shipped

`deploy/docker/grafana/dashboards/mdemg-hitl.json` — new dashboard `MDEMG HITL Curation` at `/d/mdemg-hitl/mdemg-hitl-curation`:

| Panel | Type | Signal |
|---|---|---|
| **Grade Cadence — Operator vs Auto (per day)** | Stacked bar timeseries, 30d | Throughput split by grader class. Whitespace between operator bars = stall; steady auto:* + zero operator = HITL-AUTO-DISMISS-001 draining noise but genuine rules pending |
| **Auto vs Operator Share (7d)** | Donut pie | Sanity check that HITL-CURATION-002's autograder + HITL-AUTO-DISMISS-001's drain are producing majority auto:* volume as expected |
| **Pending Drafts Queue** | Stat + thresholds (green<5 / yellow / red>20) | `contradicted_correction_drafts.status='pending'` — the exact signal `hitl-curation` alert reads |
| **Draft Status Split (all-time)** | Stat, 3 values | approved / dismissed / pending — the lifecycle breakdown |
| **Scheduled Autograde — Run History** | Table (last 30) | AUTOGRADE-SCHEDULE-001's supervised loop runs from `scheduled_job_events WHERE job_name LIKE 'scheduled-autograde%'`. Empty when scheduler disabled (default) |
| **Reinforcement Applied (7d) — Substrate Mutations** | Stat + thresholds (red=0 / yellow>=1 / green>=5) | The ONLY grade class that mutates the cognitive substrate. auto:* grades ALWAYS have reinforcement_applied=false (HITL-CURATION-002 invariant + HITL-AUTO-DISMISS-001 status-only NonReinforcingApplier). This is the honest count of operator-driven substrate change |

Descriptions embed the arc's cross-references (`hitl-curation` alert threshold, invariant contracts, sprint names) so operators reading the dashboard understand what the numbers mean without hunting through CLAUDE.md.

## Live Tier-3 (mdemg-dev)

**Grafana provisioning reload**:
```
curl -sS -u admin:admin http://localhost:3000/api/admin/provisioning/dashboards/reload -X POST
→ {"message":"Dashboards config reloaded"}

curl -sS -u admin:admin "http://localhost:3000/api/search?query=hitl"
→ MDEMG HITL Curation → /d/mdemg-hitl/mdemg-hitl-curation
```

**Panel SQL cross-check** (all 6 return real data):
- Grade Cadence: 4 rows (2026-07-20 operator=2, 2026-07-27 auto=4, 2026-07-28 auto=3, 2026-08-04 auto=10)
- 7d Share: `auto=10` (operator=0 — expected; sprint-focus workload)
- Pending Drafts: 5
- Status Split: dismissed=4, approved=2, pending=5
- Scheduled Autograde: 1 run (AUTOGRADE-SCHEDULE-001 smoke, success=t, latency_ms=17595)
- Reinforcement Applied 7d: 0 (correct — no operator approvals this window)

**Grafana embed sync**: `make sync-grafana-embed` synced 16 files (the new dashboard picked up); `make verify-grafana-embed` clean; `internal/grafanapin` pin test passes.

## Rules pinned

⚠️ **When shipping a HITL/substrate-observability dashboard, split `reinforcement_applied=true` counts into their OWN panel** — mixing auto:* dismissals (which never mutate) with operator approvals (which do) into a single "grade count" obscures the "is the substrate actually changing?" signal. The 7d substrate-mutation panel is the honest answer to "did anything real happen?"; the cadence panel is the honest answer to "did anyone show up?"; conflating them hides both stalls (no operator) and mutations (real change).

⚠️ **Panel descriptions should embed cross-references to the arc's invariants + sprint names** — a Grafana operator reading `Draft Status Split` shouldn't need to grep CLAUDE.md to understand what "approved" means for `contradicted_correction_drafts`. The one-time cost of writing rich descriptions pays off every time someone triages an alert against this dashboard. (Sibling pattern: DASHBOARD-TRUTH-002 E1/E2's design-intent numbers embedded in panel titles.)

## Not shipped (intentional)

- **Alert rule for elevated `Reinforcement Applied` count** — the metric IS the "how much substrate mutation happened" honest count, not an anomaly signal. Elevated is desirable during active curation windows.
- **LLM-dataset-specific panels** — the 16 llm:* datasets all use NoopSink (gold-only, zero substrate effect); they'd only show grade cadence not mutation. If HITL-CURATION-003 ships extension paths for these, add per-dataset panels then.
- **Grafana annotation for arc-milestone events** (sprint deploys, invariant flips) — nice-to-have but adds a new instrumentation layer for marginal value.

## Follow-ups disclosed

- **Alert `hitl_curation_stalled` cross-link** — could add a Grafana alert link on the "Pending Drafts Queue" stat panel so operators click through from Grafana to the alert-history view. Small (~15min).
- **Autograde CLI success/failure rate as a stat** — the run-history table is chronological; a stat rollup ("last 7d autograde success rate: 100%") would give a quicker glance. Deferred until scheduled runs accumulate.

## Rollback

Single-commit revert. `deploy/docker/grafana/dashboards/mdemg-hitl.json` + its staged mirror get removed; Grafana provisioning reload removes the dashboard on next reconcile.

## Documents Accessed

- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (dashboard skeleton reference)
- `deploy/docker/grafana/dashboards/mdemg-ft-training.json:387` (existing `review_grades` SQL pattern for HITL grade average)
- `internal/tsdb/migrations/028_review_grades.sql` (schema — grader_id, reinforcement_applied, gold_score, time)
- `internal/tsdb/migrations/030_contradicted_correction_drafts.sql` (schema — status enum: pending / approved / dismissed)
- `internal/review/*` (HITL platform surface + sink invariants)
- `Makefile:520` (sync-grafana-embed + verify-grafana-embed targets)
- HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 + AUTOGRADE-SCHEDULE-001 posts (parent arc)
- Live Grafana + TSDB on mdemg-dev (panel verification)
