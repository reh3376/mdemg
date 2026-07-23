# Sprint FT-RECURSIVE-004 — Phase 9: drift monitoring, loop staleness, dashboard pairs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-RECURSIVE-004 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~2 dev-days (spec §4 Phase 9); several deliverables pre-shipped by 003 |
| Parent | SPEC §3 Monitor / §4 Phase 9 + FT-RECURSIVE-003 post's disclosed follow-ups |

## 2. Problem Statement

Post-promotion, nothing watches for slow drift: the E5 tripwire covers only
the 60-minute canary window, and the only quality re-measurement is the
manual benchmark (nagged by `ft_benchmark_stale`). Spec Phase 9 remainder:
`ft_production_drift` + `ft_loop_never_ran` rules, scheduled benchmark
re-runs, and the ft_* dashboard reader-writer pairs (TSDB-CONSUME-001
removed 4 writerless panels; the writers now exist). Also: the active
`ft_model_versions` baseline row carries score 0 (dishonest — the real
promotion baseline is BASELINE-RECOMPUTE-001's 0.8655), and 003 disclosed
that the E7 sweep misses `rolled_back`-terminal failure fingerprints.
Pre-shipped by 003 (spec row credit): versions writer ✓, issue filer +
dedupe-on-repeat exit criterion ✓.

## 3. Scope & Constraints

**In scope (sequential epics):**
- **E1 — `ft_loop_never_ran`**: purpose-specific staleness rule over
  `ft_training_cycles` (the FT-BENCH-REFRESH-001 lesson: don't force
  non-Go-job tables through `jobStalenessRule`): fires when zero cycle
  events within `FT_LOOP_STALENESS_DAYS` (default 14). Wired ONLY when
  `FT_LOOP_ENABLED` (a disabled actuator must not nag). Idle-safe
  COALESCE aggregate; distinct Service `ft-loop-staleness`.
- **E2 — `ft_production_drift` + gauge**: latest `benchmark_runs` aggregate
  vs the ACTIVE `ft_model_versions.overall_score`, margin
  `FT_DRIFT_MARGIN` (default 0.05). DH-004 no-data gates: skip when active
  score ≤ 0 OR no benchmark rows (returns 0 = no drift). Gauge
  `mdemg_ft_production_drift` (score deficit). Baseline-honesty fix: set
  the active baseline row's score to the recomputed 0.8655 promotion
  baseline (one UPDATE, disclosed).
- **E3 — E7 sweep extension**: fingerprint clustering includes
  `rolled_back` cycles whose stage matches the failure classes
  (`canary_failed`/`promote_failed`/`%_failed`) — the disclosed gap.
- **E4 — scheduled benchmark runner**: supervised loop (default-off,
  `FT_BENCH_SCHEDULE_ENABLED`, interval `FT_BENCH_SCHEDULE_DAYS` default 7)
  invoking the FT-BENCH-REFRESH-001 recipe (`run_benchmark --apply-tsdb`,
  neural venv, `--rows-per-spec 5 --n-runs 1`) with jobhealth
  `scheduled-ft-benchmark` — keeps `ft_benchmark_stale` green and feeds E2
  without operator toil.
- **E5 — dashboard pairs**: `mdemg-ft-training` gains a Model Versions
  table (ft_model_versions) + a Cycle Ledger latest-status panel
  (event-sourced DISTINCT ON — never raw row counts; the ledger-semantics
  lesson) + a Production Drift stat reading the E2 gauge.
- **E6 — live drills**: `ft_loop_never_ran` both-branch SQL proof; drift
  rule SEEDED drill (temporarily set active score 0.99 → rule fires at
  0.9188 < 0.94; restore) — the spec's exit criterion; forced scheduled
  benchmark run landing a real row; sweep-extension drill against the
  existing rolled_back drill rows (expect clustering WITHOUT filing — filer
  stays default-off).
- **E7 — docs** (feature doc §Phase-9, CHANGELOG, CLAUDE.md, post).
**Out of scope:** per-task drift decomposition; auto-remediation beyond
alerting (drift → operator decision, not auto-rollback — outside the
canary window the superseded model may be long gone).
**Constraints:** all alert-rule contracts (idle-safe aggregate+COALESCE, no
LIMIT-1 literal, distinct Services — the pin sweeps auto-cover); grafanapin
passes; flags default-off with `.env` enable-after-smoke.

## 4. Dependencies

✅ `ft_model_versions` + ledger writers (003); ✅ benchmark `--apply-tsdb`
(BENCH-SIDECAR-APPLY-001); ✅ live benchmark rows (0.9188/0.8544) + active
version row for the drills; ✅ rule pin-test sweeps.

## 5. Implementation Plan

Sequential E1→E7 as above; each epic ends with its Tier-3 evidence.

## 6. Testing Plan

Tier 1: rule-shape pins + sweep-extension unit + runner interval math.
Tier 2: `go test ./...` + grafanapin. Tier 3: E6 drills (seeded drift fire
= the spec exit criterion).

## 7. Commit Strategy

`docs(E0)` → per-epic commits with evidence → `docs(E7)`. Surprise defects
get own fix-commits.

## 8. Verification Checklist

never-ran rule: fires on empty-window SQL, silent with events, absent when
loop disabled · drift rule: fires seeded, silent restored, skips score-0 ·
sweep clusters rolled_back failures · scheduled benchmark lands a real row
+ jobhealth · dashboards render (JSON valid + grafanapin) · docs · pushed.

## 9. Documentation Update

`docs/features/ft-recursive-loop.md` §Phase-9; CHANGELOG; CLAUDE.md FT
note (arc COMPLETE); post.md.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Drift rule flaps on benchmark variance | Med | Margin default 0.05 ≈ 2× observed run-to-run delta (0.9188 vs 0.8544 spans harness changes, not variance; scoped-filter runs are tighter); HIGH only, no auto-action |
| Scheduled benchmark saturates the box mid-day | Med | Default-off; interval days; runs report jobhealth; operator picks the enable window |
| Score-0 baseline poisons drift math | Low | DH-004 gate + the honesty UPDATE to 0.8655 |

## 11. Rollback

Rules: config disable/revert. Runner: `FT_BENCH_SCHEDULE_ENABLED=false`.
Baseline UPDATE reversible (documented prior value 0).

## 12. Documents Accessed

SPEC §3 Monitor/§4 Phase 9; FT-RECURSIVE-003 post (disclosed follow-ups);
FT-BENCH-REFRESH-001 (recipe + purpose-specific-rule lesson);
BASELINE-RECOMPUTE-001 (0.8655); live `benchmark_runs`/`ft_model_versions`;
`internal/alert/rules.go` contracts; TSDB-CONSUME-001 (removed panels).
