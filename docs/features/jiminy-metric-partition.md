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

- ✅ ~~**Grafana panel updates**~~ — SHIPPED as JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 (2026-09-20). See §Per-class alerts + panels below.
- ✅ ~~**Per-class alert rules**~~ — SHIPPED as JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 (2026-09-20). See §Per-class alerts + panels below.
- ✅ ~~**Path 2 observers**~~ — JIMINY-PROCESS-OBSERVER-{01..06} SHIPPED 2026-09-18 → 2026-09-19; the `process` class gauge now has 6 live matchers (live 24h ~74%).
- ✅ ~~**HITL human-class routing**~~ — SHIPPED as JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (2026-09-20). See §Human-class writer below.

## Per-class alerts + panels (JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 — 2026-09-20)

Closes the observability loop the parent sprint left as a follow-up. Ships:

- **3 new alert rules** via `alert.JiminyFollowRateClassRules(map[string]float64)` (`internal/alert/rules.go`):
  - `jiminy_follow_rate_drop_classifier` — Service `jiminy-classifier`, reads `mdemg_jiminy_follow_rate_classifier_verifiable`
  - `jiminy_follow_rate_drop_process` — Service `jiminy-process`, reads `mdemg_jiminy_follow_rate_process_verifiable`
  - `jiminy_follow_rate_drop_hybrid` — Service `jiminy-hybrid`, reads `mdemg_jiminy_follow_rate_hybrid`
  - (`jiminy_follow_rate_drop_human` — Service `jiminy-human`, disabled at floor=0 until the writer ships)

  Every rule uses `COALESCE(AVG(value), 1.0)` idle-safe SQL over a 30-min window (TSDB-CONSUME-001), reads the `time` column, and `lt` operator. Distinct Service per rule (NOSILENT-001 cooldown-key contract).

- **4 new config knobs** in `internal/config/config.go`:
  - `JIMINY_FOLLOW_RATE_CLASSIFIER_FLOOR` default **0.10** (below live 24h ~0.17)
  - `JIMINY_FOLLOW_RATE_PROCESS_FLOOR` default **0.25** (live-smoke-calibrated below the sparse 30-min window ~0.43; 24h avg ~0.74)
  - `JIMINY_FOLLOW_RATE_HYBRID_FLOOR` default **0.15** (below live 24h ~0.22)
  - `JIMINY_FOLLOW_RATE_HUMAN_FLOOR` default **0** (disabled until JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001)

- **Aggregate alert retired**: `JIMINY_FOLLOW_RATE_ALERT_FLOOR` code default flipped 0.05 → 0 (rule disabled on fresh installs). The class-mix-dominated aggregate can't tell you which class dropped; the per-class rules are the honest scoreboard per JIMINY-CEILING-INVESTIGATION-002. Operators with explicit `.env` value keep the aggregate rule active — no silent breakage.

- **5 new Grafana panels** on `mdemg-jiminy.json` in the row `Follow Rate by Verifiability Class (JIMINY-METRIC-PARTITION-001)`:
  - 4 stat panels (classifier / process / hybrid / human), thresholds pinned to each class's alert floor
  - 1 overlaid timeseries showing all 4 classes on the selected time range

  Panel titles embed the live steady-state number in parens (DASHBOARD-TRUTH-002/003 pattern: e.g., `Classifier-verifiable (honest ~17%, target retrain-gate)`). Datasource UID = literal `timescaledb` (ARC-TRAJECTORY-PANEL-001 contract).

**Live Tier-3 (mdemg-dev, 2026-09-20)**: post-restart, all 3 active rules query real TSDB and return non-null: classifier=0.175, process=0.397, hybrid=0.400 — all above floor. Zero class-fires in `~/.mdemg/alerts/current.json` in 45s post-restart. Alert evaluator started with `rules=35`.

**Recalibration lesson (pinned in-plan)**: the process floor's first-cut value (0.50, calibrated on 24h avg) fired the alert on the live 30-min window (0.43). The **FOLLOW-RATE-CALIBRATE-001 arch rule** — floor must sit BELOW the live steady state, not below the smoothed long-window mean — was live-caught mid-smoke and recalibrated to 0.25 before ship. Watch-item at T+30d: process floor may drift as denominator accumulates.

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

## Human-class writer (JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 — 2026-09-20)

The 4th (last) class gauge (`mdemg_jiminy_follow_rate_human`) now has a writer — the HITL platform. Human-verifiability rules are defined as the class the LLM classifier CANNOT verify from action-text (design spec #157 taxonomy); the honest signal is operator judgment.

### Shape

1. **V0037** adds `verifiability_class TEXT NOT NULL DEFAULT 'classifier'` to `guidance_training_rows` (mirrors V0035 on `constraint_outcomes`).
2. **RecordOutcome emit branch** (`internal/jiminy/service.go`): when `s.cfg.JiminyHumanClassQueueEnabled=true` AND the source node's class='human', the training-row emit tags:
   - `outcome_type = 'pending_human_review'`
   - `verifiability_class = 'human'`

   The constraint_outcomes routing above is UNCHANGED — informational→NA override still fires. This means aggregate follow-rate + escalation + corpus-audit signals stay byte-identical.
3. **HITL dataset** `human_class_queue` (`internal/api/human_class_queue_dataset.go`):
   - `FetchCandidates`: `SELECT ... FROM guidance_training_rows LEFT JOIN review_grades ... WHERE outcome_type='pending_human_review' AND verifiability_class='human' AND r.item_id IS NULL` (HITL-CURATION-003 dedup pattern — pending rows exit the queue automatically once graded at the current rubric_version).
   - `Rubric`: single dimension `followed` 0-4 (`hc-v1`).
   - `Sink`: writes to `constraint_outcomes` with `verifiability_class='human'` + `classifier_source='operator'`. Grade mapping: `>=3 → followed`, `<=1 → ignored`, `==2 → defer/no-op`.
4. **Auto-grader REJECTED at sink** — human class is by construction LLM-unverifiable. `Preview` + `Apply` both refuse `grader_id LIKE 'auto:%'` with named error `errAutograderRejected` citing the taxonomy source. This is the sole HITL dataset with this inversion; every other dataset allows auto:* grades under HITL-CURATION-002's non-reinforcing invariant.
5. **`GuidanceEffectivenessByClass` SQL** extended to exclude `outcome_type IN ('pending_human_review','graded_human_review')` from credit — pending backlog does not contribute to the class rate.

### Config

- `JIMINY_HUMAN_CLASS_QUEUE_ENABLED` — writer-side gate. Default `false` in code AND `.env` (HEBB-ETA-001 contract). Operator flips in `.env` after live smoke.
- `JIMINY_FOLLOW_RATE_HUMAN_FLOOR` — alert floor for `mdemg_jiminy_follow_rate_human`. Default `0` (disabled). Operator raises to a real number after the gauge accumulates enough operator-graded rows to establish a steady state (~30d suggested).

### Live Tier-3 (mdemg-dev, 2026-09-20)

End-to-end verified:
- Synthesized pending row → visible in `/v1/review/candidates?dataset_id=human_class_queue`
- Operator grade (dim `followed=4`, `reinforce=true`) → `grade_recorded=true, reinforcement_applied=true`
- Buffered writers flushed → `constraint_outcomes` row landed with `outcome=followed, class=human, source=operator`
- LEFT JOIN dedup fired → `item_count: 1 → 0` post-grade
- **`mdemg_jiminy_follow_rate_human = 1.0000`** on the next RSIC assessment tick — the dormant-since-#158 gauge moved off 0
- Auto-grader smoke (fresh row + `force:true` to bypass 409): sink refused with server-side error; **0 rows in `review_grades` with `grader_id LIKE 'auto:%'` on this dataset; 0 constraint_outcomes rows with auto:* source**

### Semantics of the pending queue

The pending row's `outcome_type='pending_human_review'` is a queue signal, not an outcome. `GuidanceEffectivenessByClass` explicitly excludes it from the credit computation — the class rate reflects only operator-graded outcomes in `constraint_outcomes`. This is by design: the human class gauge answers "of the actions the operator has graded against this class of rule, what fraction followed?" — NOT "how big is the backlog?"

To see queue depth, use the shipped HITL analytics tile pattern (HITL-ANALYTICS-TILE-001) or run:

```sql
SELECT count(*)
FROM guidance_training_rows g
LEFT JOIN review_grades r ON r.dataset_id='human_class_queue'
    AND r.item_id=g.row_id AND r.reversed=FALSE AND r.rubric_version='hc-v1'
WHERE g.space_id='<space>'
  AND g.outcome_type='pending_human_review'
  AND g.verifiability_class='human'
  AND r.item_id IS NULL;
```

### Deferred

- **Grafana panel for queue depth + operator grading cadence** — optional Epic 6 in the sprint plan; ship if operator throughput proves insufficient without visibility. Uses the shipped HITL-ANALYTICS-TILE-001 pattern.
- **Per-rule sampling / rate limit** — v1 emits every human-class item that fires the classifier. If volume proves unmanageable, add a sampling gate (mirror the `PROCESS_MATCHER_*_ENABLED` per-rule pattern).
- **Retroactive backfill** — human-class events never landed anywhere pre-sprint (the informational→NA route dropped them entirely). No source to backfill from; forward-only is honest.

### Rollback

Non-destructive with two flags of protection:
1. `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=false` + restart → stops new emits.
2. `DELETE FROM constraint_outcomes WHERE verifiability_class='human' AND classifier_source='operator'` → undo human-class grade rows (operator judgment; grading is real data, may want to keep).
3. `DELETE FROM guidance_training_rows WHERE outcome_type='pending_human_review'` → undo pending queue rows.
4. Migration rollback (last resort): `ALTER TABLE guidance_training_rows DROP COLUMN verifiability_class; UPDATE tsdb_schema_meta SET value='36';`.
5. `git revert <shas>` → undoes Go code + writer + sink + config + docs.

Reversibility preserved end-to-end.
