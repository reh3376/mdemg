# MDEMG-ALERT-NOISE-REDUCTION-001 — Sprint Plan

**Started**: 2026-10-22
**Branch**: `reh3376_dev01`
**PR target**: open on push via auto-PR

Operator directive on session resume: 50 pending alerts with the stack healthy. Operator asked for durable fixes, not stale-alert clearing, to avoid tech-debt accrual later.

---

## 1 Header & Metadata

- **Sprint line**: `alert-noise-reduction-001` (new line)
- **Owner**: reh3376
- **Prerequisites**: NOSILENT-001, ORPHAN-ALERT-001, TSDB-CONSUME-001, JIMINY-METRIC-PARTITION-001, FT-BENCH-REFRESH-001 — this sprint tightens their behavior; no new infra
- **Related arcs**: dashboard-truth family (every prior round found most "scary" alerts were measurement artifacts; this round is the same shape at the alert-rule layer)

## 2 Problem Statement

Live triage on this session's resumed state surfaced 50 pending alerts on the operator's alert panel while every HTTP probe against the stack returned 200. Investigation into TSDB on the `mdemg-forge` instance revealed:

1. **Jiminy class-partition gauges emit `0.0` for dormant classes** (hybrid, process). The alert rule `JiminyFollowRateClassRules` uses `COALESCE(AVG(value), 1.0) < floor`; a real `0.0` sample defeats the COALESCE and trips the floor. On this instance, `constraint_outcomes` has 0 hybrid rows + 0 process rows (expected — those writers aren't enabled by default; `JIMINY-PROCESS-OBSERVER-01..06` arc default-off). Alert fires chronically on classes with no data to grade.
2. **`ft_benchmark_stale` fires on empty `benchmark_runs`**. The current SQL coalesces `MAX(completed_at)` to `999.0 days` when zero rows — tripping the 7-day threshold immediately on any fresh install that never ran a benchmark. Operator intent (per the function's docstring) is "ensure visibility when a stale row surfaces" — not "page an operator who hasn't benchmarked yet."

Both are the exact class ORPHAN-ALERT-001 opened: a healthy "no data" state misclassified as a regression.

## 3 Scope & Constraints

- Fix the two sites with the smallest-possible structural changes.
- Preserve all shipped alert contracts: NOSILENT-001 distinct-Service cooldown keys, TSDB-CONSUME-001 idle-safe aggregates, no `ORDER BY … LIMIT 1`.
- No gauge or alert rule removed — only emit/aggregate semantic sharpened.
- No `.env` or config knob changes needed.
- Zero substrate mutation.

## 4 Dependencies

None. Both changes are in Go source; no schema, migration, or compose template touched.

## 5 Implementation Plan

### Epic 1 — Jiminy class gauges emit honestly

**File**: `internal/ape/self_assess.go::publishGuidanceMetrics`

Before this sprint:

```go
for _, class := range []string{"classifier", "process", "hybrid", "human"} {
    rate := stats.FollowRateByClass[class].Rate
    // Always Set — even when Samples == 0
    switch class { … m.JiminyFollowRateHybrid(spaceID).Set(rate) … }
}
```

After:

```go
for _, class := range []string{"classifier", "process", "hybrid", "human"} {
    cr := stats.FollowRateByClass[class]
    if cr.Samples == 0 {
        continue  // dormant class → don't fabricate a 0.0 sample
    }
    switch class { … m.JiminyFollowRateHybrid(spaceID).Set(cr.Rate) … }
}
```

The alert rule's `COALESCE(AVG(value), 1.0)` becomes the single source of the "no-data = healthy" semantic: when no row lands in `metric_samples` for a space × class, AVG = NULL, COALESCE returns 1.0, no fire.

### Epic 2 — FT benchmark staleness distinguishes never-ran from stale

**File**: `internal/alert/rules.go::FTBenchmarkStalenessRule`

Before: `COALESCE(EXTRACT(...) / 86400, 999.0) AS age_days`. Empty table → 999 → fires.

After: `CASE WHEN COUNT(*) = 0 THEN 0.0 ELSE COALESCE(..., 0.0) END`. Empty table → 0.0 (healthy). "Ran once + is 30 days old" → real age → fires correctly.

Docstring extended to name the never-ran-vs-stale distinction + point operators who want a "benchmark never ran" alert at filing a separate rule.

### Epic 3 — Tests

- `TestPublishGuidanceMetrics_SkipsClassGaugeWhenZeroSamples` — seed each class gauge with a sentinel value, publish with Samples=0 across all classes, assert values unchanged.
- `TestPublishGuidanceMetrics_PublishesWhenSamplesNonZero` — mixed Samples (some 0, some > 0); real-data classes update; dormant classes preserved.
- `TestFTBenchmarkStalenessRule` extended: asserts SQL contains `COUNT(*) = 0 THEN 0.0` (the never-ran branch).

Pattern mirrors the shipped `TestPublishProtocolMetrics_SkipsZeroEvent` (DASHBOARD-TRUTH-002 E2) that uses sentinel values to prove Set is skipped.

### Epic 4 — Documentation

- `sprint_plan.md` (this doc)
- `sprint_post.md` — results, follow-ups
- CLAUDE.md pin — append to the alert-correctness family (ALERT-TRUTH-001, DASHBOARD-TRUTH-001/002/003, ORPHAN-ALERT-001, NODE-DROP-CALIBRATION-001)

## 6 Testing Plan

### Tier 1 (unit)
- `go test ./internal/ape/` — new + existing `self_assess_test.go` pins
- `go test ./internal/alert/` — extended `TestFTBenchmarkStalenessRule`

### Tier 2 (integration)
- `go build ./...` — compile clean
- Full `go test ./...` on the two package neighborhoods

### Tier 3 (live-smoke, deferred until next session with running mdemg)
- Restart local mdemg
- Observe `/v1/metrics/snapshot` — confirm per-class gauges with no writer are NOT emitting rows (not just reading as 0)
- Observe `/healthz` + `GET /v1/alerts?status=pending` — the specific 7 alert rules under test (jiminy_follow_rate_drop_{process,hybrid}, ft_benchmark_stale) do NOT re-fire within an hour of restart on an instance with empty tables
- Clear stale pre-fix alerts via `POST /v1/alerts/clear` (operator decision — this sprint does not clear alarms, only prevents re-fire)

## 7 Commit Strategy

Single commit on `reh3376_dev01` (fixes are coupled by common theme + both small). Auto-PR fires on push.

## 8 Verification Checklist

- [x] Epic 1: `publishGuidanceMetrics` skips Set when Samples==0
- [x] Epic 2: `FTBenchmarkStalenessRule` SQL has never-ran gate
- [x] Epic 3: 3 pin tests landed; all pass
- [x] Epic 4: sprint_plan.md written
- [ ] Epic 4b: CLAUDE.md pin (as part of this commit)
- [ ] Epic 4c: sprint_post.md after commit lands
- [ ] Tier 3 live smoke: deferred to next session (needs running mdemg on an instance with populated classes to prove classifier gauge still emits)
- [ ] 50 stale alerts cleared: operator decision; the FIX prevents re-fire; the EXISTING alerts remain until operator fires `POST /v1/alerts/clear`

## 9 Risks & Mitigations

- **R1**: If a downstream consumer reads one of the per-class gauges with an implicit "always-published" assumption, the publish-skip would break them. **Mitigation**: grep confirmed only two publishers (`publishGuidanceMetrics` + `live_collectors.publishGuidanceGauges`, which does NOT publish the per-class gauges). The only reader is Grafana + the alert rule. Both tolerate missing samples correctly (Grafana "No data"; alert COALESCE).
- **R2**: An instance that had REAL data in a class, then churns to dormant (no new rows), will keep publishing the last known value from the Grafana cache but no new metric_samples row. The alert will go quiet (COALESCE path kicks in after the 30-min alert window expires). Correct behavior — class is dormant, nothing to alert on.
- **R3**: Never-ran benchmark gate ships a `0.0` for empty table. If someone wires a different alert rule on `age_days = 0` as "healthy only when benchmarks ran recently", they'd lose the never-ran signal. **Mitigation**: no such rule exists today; docstring names the follow-up class.

## 10 Rollback Procedures

Not destructive. If a regression appears:
1. Revert the commit (`git revert <sha>` on dev01).
2. Pre-fix behavior restored byte-identical.

No substrate state to roll back.

## 11 Documents Accessed

- `CLAUDE.md` — alert-correctness family pins (ALERT-TRUTH-001, DASHBOARD-TRUTH-001/002/003, ORPHAN-ALERT-001)
- `internal/ape/self_assess.go` — publishGuidanceMetrics + applyHonestFollowRate
- `internal/ape/live_collectors.go` — publishGuidanceGauges (confirmed does NOT publish per-class)
- `internal/alert/rules.go` — FTBenchmarkStalenessRule + JiminyFollowRateClassRules
- `internal/alert/rules_test.go` — existing test shape to extend
- `internal/tsdb/dataset_builder.go` — GuidanceClassRate struct (confirmed `Samples int` field)
- `internal/metrics/collectors.go` + `registry.go` — Gauge.Value() for test assertions
- `docs/development/dashboard-truth-002/` — DASHBOARD-TRUTH-002 E2 sentinel-value test pattern (sibling precedent)
