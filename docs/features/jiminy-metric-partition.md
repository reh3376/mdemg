# Jiminy Metric Partition (JIMINY-METRIC-PARTITION-001)

**Sprint**: JIMINY-METRIC-PARTITION-001 · task #158 · shipped 2026-09-10
**Design source**: `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase C
**Master arc**: #95 JIMINY-CEILING-BREAK-2

## Why

`JIMINY-CEILING-INVESTIGATION-002` identified that MDEMG's single `follow_rate` metric was **structurally dishonest** — it aggregated outcomes across rules of different **verifiability classes**, producing a ceiling bounded by the class mix rather than the classifier's quality on any one class. **97% of graded rows were from meta/process rules the LLM classifier structurally cannot verify from action-text** (rules like `plan-mode-before-change`, `iterate-break-fix-verify` — process rules that govern the agent's flow, not any single action).

This sprint splits the metric by class so future arcs (like Phase 4b retrain) can gate on the class where LLM improvement CAN actually help.

## Choices

### Follow the shipped `is_informational` pattern

Every design choice mirrors JIMINY-INFORMATIONAL-CATEGORY-001 (task #99): property lives on the Neo4j MemoryNode, CLI writes it, RecordOutcome reads it. Zero new API surface; substrate-side property with defense-in-depth column mirror in TSDB.

### Denormalize class into `constraint_outcomes` at write time

Alternative was querying Neo4j at aggregation time — rejected as slow (blocks the metric emitter on every scrape) and wrong-pattern (metric aggregator shouldn't couple to substrate). The V0035 column duplicates the class from Neo4j at RecordOutcome time; aggregator groups by column. Small denormalization + big correctness win.

### Process + human classes route AWAY from `constraint_outcomes`

Rules classed as `process` or `human` do NOT persist to constraint_outcomes — logged INFO + skipped. The Path 2 grader (process) and HITL platform (human) will grade these via their own sinks. Cleaner separation of concerns than trying to overload one table with three grading pipelines.

### Static seed + operator override

The static seed map lives in `internal/cli/jiminy_constraint_class.go` — every classification is a code diff (reviewable) not a live mutation. New rules default to `classifier` at RecordOutcome time (safe backward-compat). Operators can override any code via `set-class`.

## How it works

### Data model

Neo4j property on `MemoryNode {role_type IN ['constraint','correction']}`:
- `verifiability_class`: `classifier` | `process` | `hybrid` | `human` (default: `classifier`)
- `verifiability_class_marked_at`: ISO8601 datetime

TSDB column (V0035) on `constraint_outcomes`:
- `verifiability_class TEXT NOT NULL DEFAULT 'classifier'` — denormalized copy of the node's class at write time
- Index: `(verifiability_class, time DESC)` for per-class gauge queries

### Runtime routing (`internal/jiminy/service.go::RecordOutcome`)

For each outcome:
1. Batch-load `verifiability_class` for all source node IDs (one Cypher round-trip; mirrors `loadInformationalNodeSet`)
2. Pick class for item's primary source node
3. If class is `classifier` or `hybrid`: write to `constraint_outcomes` with class tag
4. If class is `process` or `human`: log INFO with reason + SKIP the write (Path 2 / HITL grader will persist via their own sinks)

### Aggregation

`internal/tsdb/dataset_builder.go::GuidanceEffectivenessByClass` — same followed=1.0 / partial_compliance=0.5 math as `GuidanceEffectiveness` but `GROUP BY verifiability_class`.

`internal/ape/self_assess.go::applyHonestFollowRate` populates `JiminyStatsResult.FollowRateByClass`. The assessment emitter (`publishGuidanceMetrics`) publishes the 4 class-tagged gauges.

### Gauges (Prometheus-style)

- `mdemg_jiminy_follow_rate_classifier_verifiable{space_id}` — the metric a retrain CAN improve
- `mdemg_jiminy_follow_rate_process_verifiable{space_id}` — dormant until Path 2 observers ship
- `mdemg_jiminy_follow_rate_hybrid{space_id}` — classifier+process composite
- `mdemg_jiminy_follow_rate_human{space_id}` — dormant until HITL routing ships

Aggregate `mdemg_jiminy_follow_rate` is retained for backward compatibility but is **honestly labeled dishonest** in its help text.

## How to use

### List current class assignments

```bash
mdemg jiminy constraint list-classes --space-id mdemg-dev
mdemg jiminy constraint list-classes --space-id mdemg-dev --class classifier
```

### Change a single rule's class

```bash
mdemg jiminy constraint set-class \
  --code lint-before-commit \
  --space-id mdemg-dev \
  --class process \
  --dry-run
# then apply for real:
mdemg jiminy constraint set-class \
  --code lint-before-commit \
  --space-id mdemg-dev \
  --class process
```

### Apply the static seed (the 33-rule bootstrap from the design sprint)

```bash
mdemg jiminy constraint seed-classes --space-id mdemg-dev --dry-run
mdemg jiminy constraint seed-classes --space-id mdemg-dev
# re-run any time; idempotent unless --force
mdemg jiminy constraint seed-classes --space-id mdemg-dev --force
```

### Query the honest classifier follow rate

Via the metrics snapshot:
```bash
curl -s http://127.0.0.1:9999/v1/metrics/snapshot | jq '.data.gauges | with_entries(select(.key | contains("follow_rate")))'
```

Or directly against TSDB:
```sql
SELECT
    verifiability_class,
    COUNT(*) AS n,
    ROUND(100.0 * SUM(CASE WHEN outcome_type='followed' THEN 1
                           WHEN outcome_type='partial_compliance' THEN 0.5
                           ELSE 0 END) / NULLIF(COUNT(*),0), 2) AS follow_pct
FROM constraint_outcomes
WHERE time >= now() - interval '168 hours'
  AND guidance_type IN ('constraint','correction')
GROUP BY verifiability_class
ORDER BY follow_pct DESC;
```

### Rollback

Full rollback in reverse order:

```sql
-- TSDB: drop the column
ALTER TABLE constraint_outcomes DROP COLUMN verifiability_class;
DROP INDEX IF EXISTS idx_constraint_outcomes_class_time;
UPDATE tsdb_schema_meta SET value = '34' WHERE key = 'schema_version';
```

```cypher
// Neo4j: remove the property from all nodes
MATCH (c:MemoryNode)
WHERE c.verifiability_class IS NOT NULL
REMOVE c.verifiability_class, c.verifiability_class_marked_at
RETURN count(c);
```

```bash
# Code: git revert the sprint commit
git revert <sha>
```

## What ships in this sprint

- V0035 migration + schema bump 34 → 35
- Neo4j property `verifiability_class` (default `classifier`)
- `mdemg jiminy constraint {set-class,list-classes,seed-classes}` CLI subcommands
- Static seed for the 33-rule set (9 classifier, 4 process, 2 hybrid, 18 human)
- `RecordOutcome` class routing + column tag on TSDB rows
- 4 per-class gauges wired into the ape assessment path
- 9 pin tests

## What is deferred (design spec Phase 2/3 follow-ups)

- **Grafana panel updates** — retire the aggregate follow-rate panel + add 5 per-class panels (separate sprint; big JSON diff)
- **Per-class alert rules** — `classifier_follow_rate_low` reading `constraint_outcomes.verifiability_class='classifier'` (separate sprint)
- **Path 2 observers** — `JIMINY-PROCESS-OBSERVER-{01..06}` will populate the `process` class gauge (currently dormant at 0)
- **HITL human-class routing** — `JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001` will populate the `human` class gauge (currently dormant at 0)

## Live verification (mdemg-dev, 2026-09-10)

Post-shipping snapshot:
```
mdemg_jiminy_follow_rate_classifier_verifiable{space_id="mdemg-dev"} = 0.252
mdemg_jiminy_follow_rate_process_verifiable{space_id="mdemg-dev"}    = 0
mdemg_jiminy_follow_rate_hybrid{space_id="mdemg-dev"}                = 0
mdemg_jiminy_follow_rate_human{space_id="mdemg-dev"}                 = 0
mdemg_jiminy_follow_rate{space_id="mdemg-dev"}                       = 0.252
```

The classifier follow rate is **25.2%** — a meaningful lift over the pre-sprint aggregate of 16.74% (Path 4+1 measured value), obtained by routing the classifier-unverifiable class OUT of the numerator+denominator. This is the honest scoreboard on which any future retrain (Phase 4b) should be evaluated.
