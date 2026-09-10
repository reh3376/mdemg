# JIMINY-METRIC-PARTITION-001 — Sprint Plan

**Task**: #158
**Predecessor**: JIMINY-METRIC-DENOMINATOR-DESIGN-001 (#157) — this sprint implements Path 3 from its recommendation
**Wall-clock estimate**: ~1 day
**Type**: shipping — code + schema + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-METRIC-PARTITION-001 |
| Task # | #158 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | 1 TSDB migration (additive column) + Neo4j property writes for 15 rules (fully reversible) |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase C |
| Unblocks | #159 Phase 4b gate decision |

## 2. Problem Statement

The single `follow_rate` metric aggregates rules of different verifiability classes, producing a dishonest ceiling that no classifier improvement can move (root cause: JIMINY-CEILING-INVESTIGATION-002). Ship a per-class metric partition so retrain compute (Phase 4b) can gate on the class where LLM improvement CAN help (`classifier`).

## 3. Scope & Constraints

**In scope** (per design spec Path 3):
- New Neo4j property `verifiability_class` on `role_type='constraint'|'correction'` nodes (enum: classifier|process|hybrid|human; default classifier)
- New CLI subcommands under `mdemg jiminy constraint`: `set-class`, `list-classes`, `seed-classes`
- New TSDB column `verifiability_class TEXT` on `constraint_outcomes` (V0035 migration); default 'classifier' for backward-compat
- `RecordOutcome` reads class via new `loadVerifiabilityClassMap()` and:
  - `classifier` (default) → record to constraint_outcomes with class label (current behavior + class annotation)
  - `hybrid` → record with `verifiability_class='hybrid'`
  - `process`/`human` → do NOT persist to constraint_outcomes (log INFO + skip; downstream Path 2 or HITL will grade)
- 4 new metric gauges emitted from windowed `constraint_outcomes` GROUP BY class:
  - `mdemg_jiminy_follow_rate_classifier_verifiable{space_id}`
  - `mdemg_jiminy_follow_rate_process_verifiable{space_id}` (dormant until Path 2)
  - `mdemg_jiminy_follow_rate_hybrid{space_id}`
  - `mdemg_jiminy_follow_rate_human{space_id}` (dormant until HITL routing)
- Static seed application via `mdemg jiminy constraint seed-classes --space-id mdemg-dev`
- Reframe alert rule `guidance_should_follow_rate_low` → `classifier_follow_rate_low` (queries with `verifiability_class='classifier'` filter)

**Out of scope** (deferred per design):
- Grafana panel updates (big JSON diff; separate sprint)
- Process observers themselves (that's Path 2 = JIMINY-PROCESS-OBSERVER-{01..06})
- HITL routing for human class (separate sprint)
- HTTP admin endpoint for class management (CLI-only for now, mirrors informational precedent)

**Constraints preserved**:
- `plan-mode-before-change` — this doc IS the plan
- `unit-integration-e2e-docs` — 3 tiers included
- `live-testing-tier-required` — Tier-3 on real Neo4j + TSDB
- `never-hardcode-config` — 3 new env vars (`JIMINY_VERIFIABILITY_CLASS_DEFAULT`, `JIMINY_VERIFIABILITY_CLASS_SEED_ON_STARTUP`, per-gauge disable flags)
- `never-direct-alter-schema` — V0035 migration file lands with schema
- `must-use-cuid2` — no new IDs introduced (property, not entity)
- `mandatory-feature-docs` — `docs/features/jiminy-metric-partition.md` ships with the code

## 4. Dependencies

- V0034 in place (current schema version 34) — confirmed
- Live Neo4j + TSDB reachable — verified
- No new Go module additions needed

## 5. Implementation Plan (sequential epics per `sequential-epics` rule)

### Epic 1 — Config + type definitions
- `internal/jiminy/types.go` (or nearby): new const block `VerifiabilityClass` enum (`classifier`, `process`, `hybrid`, `human`)
- Config additions: `JIMINY_VERIFIABILITY_CLASS_DEFAULT` (default `classifier`), `JIMINY_VERIFIABILITY_CLASS_SEED_ON_STARTUP` (default false)

### Epic 2 — CLI subcommands
- Extend `internal/cli/jiminy_constraint.go` with 3 new subcommands:
  - `set-class --code X --space-id Y --class Z [--dry-run]`
  - `list-classes --space-id Y`
  - `seed-classes --space-id Y [--dry-run] [--force]`
- `seed-classes` applies static seed from the design spec (9 classifier + 4 process + 2 hybrid + 18 human)

### Epic 3 — V0035 migration + schema version bump
- New migration file `internal/tsdb/migrations/035_constraint_outcomes_verifiability_class.sql`:
  ```sql
  ALTER TABLE constraint_outcomes ADD COLUMN IF NOT EXISTS verifiability_class TEXT NOT NULL DEFAULT 'classifier';
  CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_class_time ON constraint_outcomes (verifiability_class, time DESC);
  ```
- Bump `TSDBRequiredSchemaVersion` default 34 → 35 in `internal/config/config.go`

### Epic 4 — `RecordOutcome` routing
- New method `loadVerifiabilityClassMap(ctx, nodeIDs)` (mirrors `loadInformationalNodeSet` shape)
- Inside RecordOutcome:
  1. Resolve class for each item's source nodes (informational check happens first, unchanged)
  2. For classifier/hybrid: proceed to write with class annotation
  3. For process/human: log INFO with reason + skip persistence
- Extend `RecordedOutcomeMeta` (or the writer's row struct) with a `VerifiabilityClass` field
- Extend `outcomeWriter.Record` + CopyFrom cols + `constraintOutcomeRow` helper to write the class

### Epic 5 — Metric emitters
- New emitters in `internal/metrics/collectors.go` (or equivalent): 4 windowed-query gauges reading `constraint_outcomes` grouped by class
- Each fires on the existing metrics-flush ticker
- Each idle-safe (COALESCE NULLIF 0-guard per TSDB-CONSUME-001 contract)

### Epic 6 — Alert rule reframe
- `guidance_should_follow_rate_low` rule in `internal/alert/rules.go` — extend WHERE clause with `AND verifiability_class = 'classifier'`
- Rename rule ID → `classifier_follow_rate_low` (also update tests + inventory)
- Description updated to name the class

### Epic 7 — Tier 1 unit + Tier 2 integration tests
- New test file `internal/jiminy/verifiability_class_test.go` — 5-8 subcases
- New test file `internal/cli/jiminy_constraint_class_test.go` — 3-5 subcases
- Extend `internal/tsdb/*_test.go` with a column-count pin bump
- Update existing tests referencing the aggregate rule

### Epic 8 — Tier 3 live smoke
- Real binary against real Neo4j + TSDB:
  - Apply migration; verify column added
  - Kickstart server; verify no boot errors
  - Run `mdemg jiminy constraint seed-classes --space-id mdemg-dev --dry-run` — verify 15 targets
  - Apply for real
  - `mdemg jiminy constraint list-classes --space-id mdemg-dev` — verify all 15 seeded
  - Live-record a synthetic outcome (via a real classify call or /v1/jiminy/feedback) — verify constraint_outcomes row lands with class label
  - Query the new gauges via `/v1/metrics/snapshot` or TSDB directly — verify all 4 present + populated

### Epic 9 — Feature doc + CLAUDE.md + CHANGELOG
- `docs/features/jiminy-metric-partition.md` (per `mandatory-feature-docs`)
- CLAUDE.md architecture note
- CHANGELOG Unreleased entry

### Epic 10 — Commit + PR summary

## 6. Testing Plan (3 tiers)

**Tier 1** — Unit
- `TestVerifiabilityClass_Enum` — enum validation
- `TestLoadVerifiabilityClassMap_*` — happy path, empty input, driver-nil, query-error
- `TestRunSetClass_*` — dry-run, apply, unknown-class error, unknown-code error, unset via empty
- `TestRunSeedClasses_*` — static seed apply + idempotency
- `TestConstraintOutcomeRow_ColumnCountPin` — bumped +1

**Tier 2** — Integration
- Full `internal/cli` + `internal/jiminy` test suites regress
- Migration applies cleanly on fresh + existing DB

**Tier 3** — E2E live smoke (Epic 8 above)

## 7. Commit Strategy

Single-commit if scope allows; else 2-3 sequential commits mapped to epic boundaries (Epic 1-4 = "core routing", Epic 5-6 = "metrics + alerts", Epic 7-9 = "tests + docs"). Whichever fits cleanly.

## 8. Verification Checklist

- [ ] Build + lint clean
- [ ] All new + existing tests pass
- [ ] V0035 migration applies (docker exec + psql)
- [ ] Server kickstart clean; boot log shows new config
- [ ] `seed-classes --dry-run` matches 15 rules exactly
- [ ] `seed-classes` apply: 15 nodes' `verifiability_class` set
- [ ] `list-classes` shows 15 rows split by class
- [ ] Live outcome record lands with class label
- [ ] 4 gauges present in `/v1/metrics/snapshot`
- [ ] Alert rule `classifier_follow_rate_low` fires only on classifier-class subset
- [ ] Feature doc + CLAUDE.md pin + CHANGELOG in same commit as code
- [ ] Production llama-server on 8102 untouched
- [ ] PR summary comment posted

## 9. Documentation Update

- `docs/features/jiminy-metric-partition.md` — new
- `docs/development/jiminy-metric-partition-001/{sprint_plan,sprint_post}.md` — this sprint
- `CLAUDE.md` — arch note pin
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Class seed classifies a rule incorrectly | Property is trivially reversible via `set-class --class X`; seed is idempotent + `--force` re-applies |
| `constraint_outcomes` column addition breaks writer contract | Column has `NOT NULL DEFAULT 'classifier'` so old rows + old code stay valid; writer opts into the column explicitly |
| Skipping process/human class outcomes loses signal | Log INFO with reason so operators can audit; Path 2 will provide the missing signal; historical rows are preserved (only new writes route by class) |
| Alert rule reframe breaks operator dashboards | Rule ID renamed (not deleted); Grafana panels reading the OLD rule name will show as unknown — flagged as follow-up (panel update sprint) |

## 11. Rollback

Full rollback for both code + substrate:

- **Code**: `git revert <sha>` — CLI subcommands disappear; RecordOutcome reverts to informational-only routing; gauges stop emitting; alert rule reverts to aggregate query
- **TSDB**: `ALTER TABLE constraint_outcomes DROP COLUMN verifiability_class;` — additive column removal is safe (no other consumer of the column exists post-revert)
- **Neo4j property writes**: `MATCH (c:MemoryNode) WHERE c.verifiability_class IS NOT NULL REMOVE c.verifiability_class, c.verifiability_class_marked_at RETURN count(c);` — removes the property on all affected nodes

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` — source spec
- `docs/development/jiminy-ceiling-investigation-002/verdict.md` — root cause context
- `internal/jiminy/service.go` (esp. RecordOutcome + loadInformationalNodeSet pattern to mirror)
- `internal/cli/jiminy_constraint.go` (CLI pattern to extend)
- `internal/tsdb/migrations/030..034_*.sql` (migration file naming + shape)
- `internal/config/config.go` (config additions + TSDBRequiredSchemaVersion)
- `internal/alert/rules.go` (aggregate alert rule to reframe)
- `internal/metrics/` (gauge emitter location)
- Live Neo4j + TSDB (verification queries)
- CLAUDE.md pins (§3 sprint plan)
- Operator directive: "ship path 3" (2026-09-10)
