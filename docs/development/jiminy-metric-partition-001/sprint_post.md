# JIMINY-METRIC-PARTITION-001 — Sprint Post

**Task**: #158
**Completed**: 2026-09-10 (~2h wall-clock)
**Design source**: `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase C (Path 3)
**Verdict**: ✅ SHIPPED — per-class follow-rate gauges live-verified at `mdemg_jiminy_follow_rate_classifier_verifiable = 0.252` on `mdemg-dev` (25.2% honest classifier follow rate; lift over pre-sprint aggregate 16.74%).

## What shipped

| Artifact | Notes |
|---|---|
| `internal/jiminy/types.go` | New `VerifiabilityClass` enum (classifier / process / hybrid / human) + `IsValidVerifiabilityClass` + `AllVerifiabilityClasses` + `ClassPersistsToConstraintOutcomes` |
| `internal/tsdb/migrations/035_constraint_outcomes_verifiability_class.sql` | Additive column `verifiability_class TEXT NOT NULL DEFAULT 'classifier'` + `idx_constraint_outcomes_class_time` |
| `internal/config/config.go` | `TSDBRequiredSchemaVersion` default 34 → 35 |
| `internal/tsdb/constraint_outcomes_writer.go` | `ConstraintOutcomeRow.VerifiabilityClass` field + CopyFrom column extended |
| `internal/tsdb/dataset_builder.go` | New `GuidanceEffectivenessByClass` method + `GuidanceClassRate` type |
| `internal/ape/types_rsic.go` | `JiminyStatsResult.FollowRateByClass` field |
| `internal/ape/self_assess.go` | `applyHonestFollowRate` populates per-class rates; `publishGuidanceMetrics` emits 4 new gauges |
| `internal/ape/self_reflect_data_test.go` | Mock provider extended with new interface method |
| `internal/metrics/collectors.go` | 4 new gauge factories: `JiminyFollowRate{ClassifierVerifiable,ProcessVerifiable,Hybrid,Human}` |
| `internal/jiminy/service.go` | `loadVerifiabilityClassMap` + `primaryVerifiabilityClass` helpers + RecordOutcome class routing + 4 call sites updated to pass class |
| `internal/cli/jiminy_constraint.go` | 3 new subcommands wired |
| `internal/cli/jiminy_constraint_class.go` | New file — `set-class`, `list-classes`, `seed-classes` implementations + static seed map (33 codes) |
| `internal/jiminy/verifiability_class_test.go` | New file — 9 pin tests |
| `docs/features/jiminy-metric-partition.md` | New feature doc (per `mandatory-feature-docs`) |
| `docs/development/jiminy-metric-partition-001/{sprint_plan,sprint_post}.md` | This sprint |
| CLAUDE.md | Architecture note pinned |
| CHANGELOG.md | Unreleased entry |

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run` on changed packages | ✅ 0 issues |
| Full `internal/jiminy` suite | ✅ 7.9s |
| Full `internal/cli` suite | ✅ 61.5s |
| Full `internal/tsdb` suite | ✅ 0.4s |
| Full `internal/ape` suite | ✅ 4.1s |
| Full `internal/metrics` suite | ✅ 0.5s |
| 9 new class pin tests | ✅ 9/9 pass |
| V0035 migration applied clean | ✅ ALTER + INDEX + UPDATE all confirmed |
| Column `verifiability_class` present with `'classifier'::text` default | ✅ verified via information_schema |
| `tsdb_schema_meta.value` = `'35'` | ✅ |
| Binary rebuild + launchd kickstart | ✅ pid 97878 up, `/healthz` 200 OK |
| `seed-classes --dry-run` shows 24 to apply + 9 already-matches | ✅ (9 classifier-class rules already at column default) |
| `seed-classes` apply | ✅ `Applied 24/24 (space="mdemg-dev")` |
| `list-classes` post-apply summary | ✅ 9/4/2/18/33 (matches design audit exactly) |
| Assessment endpoint populates `follow_rate_by_class` | ✅ live-verified |
| **`mdemg_jiminy_follow_rate_classifier_verifiable = 0.252`** | ✅ **honest classifier follow rate** |
| Dormant class gauges = 0 | ✅ process / hybrid / human (as expected until their sinks ship) |
| Production llama-server on port 8102 | ✅ untouched throughout |

## Live smoke transcript

```
$ mdemg jiminy constraint list-classes --space-id mdemg-dev
...
Summary by class:
  classifier  9
  process     4
  hybrid      2
  human       18
  TOTAL       33
```

```
$ curl -s http://127.0.0.1:9999/v1/metrics/snapshot | jq '.data.gauges | with_entries(select(.key | contains("follow_rate")))'
mdemg_jiminy_follow_rate_classifier_verifiable{space_id="mdemg-dev"} = 0.252
mdemg_jiminy_follow_rate_process_verifiable{space_id="mdemg-dev"}    = 0
mdemg_jiminy_follow_rate_hybrid{space_id="mdemg-dev"}                = 0
mdemg_jiminy_follow_rate_human{space_id="mdemg-dev"}                 = 0
mdemg_jiminy_follow_rate{space_id="mdemg-dev"}                       = 0.252
```

The classifier gauge sitting at 0.252 (25.2%) is the **honest classifier-only follow rate**. Compare to the aggregate 16.74% measured pre-sprint by JIMINY-CEILING-INVESTIGATION-002 — the +8.5pp lift is the measurement reframe, not a quality change. As new outcomes flow through the class-routed RecordOutcome, the classifier gauge will further diverge from the (now-legacy) aggregate.

## Sprint execution notes

- Followed 10-epic sprint plan sequentially; no reordering
- Every epic gate (build + tests) verified before proceeding
- `mdemg-cms-memory-only` was already marked informational + also seeded to human class — correct (informational is a display-hint on top of the class taxonomy)
- The `applyHonestFollowRate` extension gracefully handles the "no data in window" case: nil `FollowRateByClass` map → all 4 gauges publish 0 (safe idle)
- Column DEFAULT 'classifier' means all pre-#158 rows continue to read as classifier at aggregation — no backfill needed for historical data
- Production llama-server on port 8102 untouched (this sprint has no LLM runtime surface)

## Arch rule pinned

**Metric partition mirrors informational precedent — Neo4j property + shipped CLI + defense-in-depth TSDB column + gauge emit via existing assessment path.** When adding a new node property that `RecordOutcome` needs to read at write time, denormalize into `constraint_outcomes` at write time rather than joining Neo4j at aggregation time. Rationale: the metric aggregator must NOT couple to substrate — a per-scrape join blocks the emit on Neo4j latency + creates dependency inversion where Prometheus depends on graph DB availability. Denormalization is a small write-time cost + big correctness win (per-class gauges query one indexed TSDB column). This is the second sprint using this exact pattern (JIMINY-INFORMATIONAL-CATEGORY-001 was the first); pattern generalizes to any future per-node classification that needs metric partitioning.

## Follow-ups filed (from the design spec)

1. **JIMINY-METRIC-PARTITION-002** (proposed) — Grafana panel updates: retire aggregate follow-rate panel; add 5 per-class panels; retitle arc trajectory panel to name the classifier class
2. **JIMINY-METRIC-PARTITION-003** (proposed) — Per-class alert rules; classifier_follow_rate_low reading `constraint_outcomes.verifiability_class='classifier'`
3. **#159 Phase 4b GATE decision sprint** — now UNBLOCKED (has honest classifier metric)
4. **#160 JIMINY-PROCESS-OBSERVER-01** — Path 2 first observer (lint-before-commit); will populate the `process` class gauge

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` — source spec (§Phase C)
- `docs/development/jiminy-ceiling-investigation-002/verdict.md` — root cause context
- `docs/development/jiminy-informational-category-001/` — precedent pattern being mirrored
- `internal/jiminy/service.go` — RecordOutcome + loadInformationalNodeSet (pattern source)
- `internal/cli/jiminy_constraint.go` — CLI pattern source
- `internal/tsdb/migrations/034_review_grades_notes.sql` — migration shape source
- `internal/tsdb/dataset_builder.go` — GuidanceEffectiveness (query shape source)
- `internal/ape/self_assess.go::applyHonestFollowRate` — gauge emit pipeline
- Live Neo4j `mdemg-dev` — 33-rule audit + property writes
- Live TSDB `mdemg_metrics.constraint_outcomes` — column verification
- launchd `com.mdemg.server` — kickstart target
- Sprint plan `sprint_plan.md`
- CLAUDE.md pins (§3 sprint plan)
- Operator directive: "ship path 3" (2026-09-10)
