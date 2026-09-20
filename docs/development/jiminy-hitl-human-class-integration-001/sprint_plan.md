# JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint ID**: JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001
- **Task**: (linear ticket to follow)
- **Parent arc**: JIMINY-METRIC-PARTITION-001 (task #158) — Path 2/3 sibling
- **Roadmap slot**: Q5 §3 #2 (formalized 2026-09-20 in ROADMAP_2026Q5.md)
- **Author**: Roger Henley (via Claude Opus 4.7)
- **Date**: 2026-09-20
- **Effort**: 2-3 days
- **Impact class**: direct (fourth class writer — closes the honest-scoreboard invariant)
- **Ship gate**: E2E from RecordOutcome emit → HITL grading → constraint_outcomes verifiability_class='human' row → gauge non-zero

## 2. Problem Statement

JIMINY-METRIC-PARTITION-001 (#158) shipped 4 per-verifiability-class follow-rate gauges. Three now have writers:

| Class | Writer status | Live 24h |
|---|---|---|
| classifier | ✅ `RecordOutcome` writes to `constraint_outcomes` with class tag | 0.169 |
| process | ✅ 6 process observers write to `process_outcomes` (V0036, JIMINY-PROCESS-OBSERVER-{01..06}) | 0.737 |
| hybrid | ✅ Same as classifier (routes to `constraint_outcomes`) | 0.223 |
| **human** | ❌ **No writer** — RecordOutcome logs INFO and skips | **0.000** |

Per JIMINY-METRIC-PARTITION-001 CLAUDE.md pin:
> classifier + hybrid persist to `constraint_outcomes` with class tag; process + human skip persistence + log INFO (Path 2 grader / HITL platform will persist via their own sinks — separate follow-ups #160 + JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001)

Path 2 (#160-#165) shipped. This sprint closes the last dormant writer.

Live state on mdemg-dev complicates the design:
- All 18 human-class rules are marked `is_informational=true` (JIMINY-CEILING-INVESTIGATION-002-execution Path 1)
- Informational rules route to `outcome=not_applicable` in `RecordOutcome`
- `not_applicable` outcomes are filtered from BOTH `constraint_outcomes` AND `guidance_training_rows` writes (service.go:1730,1762)

Result: **human-class items never persist anywhere queryable today. No queue exists for HITL to consume from.**

## 3. Scope & Constraints

**In-scope**:
- V0037 TSDB migration adding `verifiability_class TEXT NOT NULL DEFAULT 'classifier'` column to `guidance_training_rows`
- `RecordOutcome` branch: when source class is human, emit a `guidance_training_rows` entry (tagged `outcome_type='pending_human_review'` + `verifiability_class='human'`) BEFORE the informational→NA override strips it. Constraint_outcomes still gets the NA route unchanged (fully backward-compat on aggregate metrics)
- New HITL dataset `human_class_queue` implementing `ReviewableDataset` — surfaces pending human-review rows for operator grading
- Rubric: single dimension "did the agent follow this rule?" 0-4 scale
- Sink writes to `constraint_outcomes` with `verifiability_class='human'`, `classifier_source='operator'`, `outcome_type` derived from grade
- 3 unit tests: emit-when-human-class, sink-writes-correct-class, no-op-when-not-human
- 1 e2e integration test: synthesize a human-class outcome → HITL grade → verify class='human' row in `constraint_outcomes`
- Live Tier-3 smoke: real operator grades 1-2 items on mdemg-dev, `mdemg_jiminy_follow_rate_human` gauge moves off 0

**Out-of-scope** (deferred):
- Per-rule-code queue sampling (v1 surfaces all pending human-class items; sampling can ship if volume proves impractical)
- Operator UI panel for pending human-queue count on `mdemg-jiminy.json` (nice-to-have; ship if bandwidth allows in E7)
- Retroactive backfill of past skipped human-class events (they never landed anywhere so there's no source to backfill from; forward-only is honest)
- Sampling budget / rate limit on emit (v1 emits every human-class item; operator can throttle by marking rules informational-in-the-classic-path if volume overwhelms)
- Auto-grader for human class (by definition, human class = LLM can't grade; explicit non-goal)
- Path 4 META-SCOPE flag revisit (unrelated)

**Constraints**:
- `must-follow-12-section-format` — this doc ✓
- `must-use-cuid2` — every new identifier is CUIDv2 (row_id, item_id in HITL surface)
- `never-hardcode-config` — all knobs env-tunable with defaults
- `never-direct-alter-schema` — V0037 migration file, schema bump 36 → 37
- `unit-integration-e2e-docs` — 3 tier plan below
- `live-testing-tier-required` — Tier 3 = real operator grade lands, gauge moves
- `end-with-docs-accessed` — §12 populated
- **NOSILENT-001** — no new alert rule; existing `jiminy-human` alert reactivates once floor > 0 (operator's call after seeing volume)
- **TSDB-CONSUME-001** — DORMANT-CENSUS-002 adjudicate the new `guidance_training_rows` column semantics + `pending_human_review` outcome_type in `tsdb_consumer_inventory.json`
- **HEBB-ETA-001** — behavior-changing knobs default OFF in code AND `.env` (this sprint ships default-off — `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=false`; operator flips in `.env` after live smoke)
- **`grader_id LIKE 'auto:%'` invariant from HITL-CURATION-002** — auto-grade NEVER for human class (LLM can't grade); enforced via sink refusing autograder writes
- **`NonReinforcingApplier` contract from HITL-AUTO-DISMISS-001** — human grade DOES reinforce (writes to `constraint_outcomes` = substrate telemetry). Standard `Apply` sink, not the auto-dismiss shape.

## 4. Dependencies

**Upstream (must be live)**:
- JIMINY-METRIC-PARTITION-001 (#158) — 4 class gauges + verifiability_class Neo4j property ✓
- HITL platform (`internal/review/`, HITL-REVIEW-001) — `ReviewableDataset` interface + registry + sink ✓
- HITL-CURATION-002 auto-grade invariants — for enforcement + telemetry hygiene ✓
- JIMINY-CEILING-INVESTIGATION-002-execution Path 1 — the informational-marking that today prevents human-class items from landing anywhere. This sprint threads AROUND the informational route.

**Downstream (unblocked when this ships)**:
- JIMINY-CEILING-BREAK-2 T+30d passive re-measure — with 4 real class gauges, retrain-gate math sees the whole taxonomy
- HITL corpus growth on the human class — feeds retrain, feeds substrate-quality drift alerts

## 5. Implementation Plan

Sequential epics per `sequential-epics` rule.

### Epic 1 — V0037 migration + schema wiring

- **E1.1**: New `internal/tsdb/migrations/037_guidance_training_rows_verifiability_class.sql`:
  ```sql
  ALTER TABLE guidance_training_rows
    ADD COLUMN IF NOT EXISTS verifiability_class TEXT NOT NULL DEFAULT 'classifier';
  UPDATE tsdb_schema_meta SET value = '37' WHERE key = 'schema_version';
  CREATE INDEX IF NOT EXISTS idx_guidance_training_rows_class_time
    ON guidance_training_rows (space_id, verifiability_class, time DESC);
  ```
  Mirrors V0035 pattern; column-default backfills existing rows to `classifier`.
- **E1.2**: Bump `TSDBRequiredSchemaVersion` 36 → 37 in `internal/config/config.go`.
- **E1.3**: Extend `GuidanceTrainingRowsWriter.Record` struct + writer CopyFrom cols to include `verifiability_class`.
- **E1.4**: Adjudicate the new column + `pending_human_review` outcome_type value in `docs/api/tsdb_consumer_inventory.json` (DORMANT-CENSUS-002 contract).
- **Gate**: `go build ./...` clean; migration applies cleanly against real TSDB; `verify_tsdb_consumers.py` clean; `TSDB_REQUIRED_SCHEMA_VERSION` gate green post-migration.

### Epic 2 — RecordOutcome emit branch

- **E2.1**: In `internal/jiminy/service.go::RecordOutcome`, BEFORE the informational→NA override, check the source node's `verifiability_class`. If class='human' AND `JiminyHumanClassQueueEnabled=true`, emit a row to `guidance_training_rows` with:
  - `outcome_type='pending_human_review'`
  - `verifiability_class='human'`
  - `classifier_source='pending_human'`
  - All other fields as usual (guidance_id, action_summary, similarity, etc.)

  Then continue through the informational override path — `constraint_outcomes` still gets `not_applicable` (or no row via the shipped filter). No metric-behavior change on `mdemg_jiminy_follow_rate` or actionable aggregates.
- **E2.2**: New config knob `JIMINY_HUMAN_CLASS_QUEUE_ENABLED` default false in code, opt-in via `.env` post-live-smoke.
- **E2.3**: `GuidanceEffectivenessByClass` SQL update — exclude `outcome_type='pending_human_review'` from the credit computation (pending rows are not-yet-graded; shouldn't contribute to the class rate).
- **Gate**: unit tests green; `RecordOutcome` doesn't regress classifier/process/hybrid paths (existing suite passes).

### Epic 3 — HITL dataset + sink

- **E3.1**: New `internal/api/human_class_queue_dataset.go` implementing `ReviewableDataset`:
  - `ID = "human_class_queue"`
  - `FetchCandidates(query CandidateQuery)` reads pending rows: `SELECT ... FROM guidance_training_rows WHERE space_id=$1 AND outcome_type='pending_human_review' AND verifiability_class='human' [+ LEFT JOIN review_grades AND r.item_id IS NULL AT rubric_version — HITL-CURATION-003 dedup pattern]`
  - `FetchItem`: returns full row for detail view
  - `Rubric`: single dimension `followed` 0-4 anchors ("clearly followed" / "partially followed" / "unclear" / "partially violated" / "clearly violated")
  - `Sink`: writes to `constraint_outcomes` with `verifiability_class='human'`, `classifier_source='operator'`, `outcome_type` derived (≥3 → followed, ==2 → partial_compliance, ≤1 → ignored). Also updates the source `guidance_training_rows` row to `outcome_type='graded_human_review'` so it exits the pending pool.
  - Reject auto-grader (`grader_id LIKE 'auto:%'`) — human class is by definition not LLM-gradable; enforce at sink entry with named error.
- **E3.2**: Register in `s.reviewRegistry` alongside guidance + contradicted_drafts + LLM call sites in `server.go`.
- **Gate**: HITL dataset registered; `GET /v1/review/datasets` returns `human_class_queue`; grading via `POST /v1/review/grade` writes to constraint_outcomes; auto-grader rejected with named error.

### Epic 4 — Live Tier-3 smoke (mdemg-dev)

- **E4.1**: Enable flag in `.env`: `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=true`. Kickstart server.
- **E4.2**: Trigger a natural (or synthesized) action that surfaces a human-class rule. Verify one pending row lands in `guidance_training_rows` with `verifiability_class='human'` + `outcome_type='pending_human_review'`.
- **E4.3**: Operator grades via `POST /v1/review/grade` OR /ui/review. Verify:
  - Grade row lands in `review_grades` with `dataset_id='human_class_queue'` and NOT `grader_id LIKE 'auto:%'`
  - New row in `constraint_outcomes` with `verifiability_class='human'` + `classifier_source='operator'` + correct `outcome_type` per grade
  - `guidance_training_rows` row updates to `outcome_type='graded_human_review'` and exits the pending pool
- **E4.4**: Wait for next `publishGuidanceMetrics` tick; verify `mdemg_jiminy_follow_rate_human` gauge moves off 0 to the correct fraction.
- **E4.5**: Auto-grader rejection smoke: run `mdemg review autograde --dataset human_class_queue` — expect NAMED error, zero grade rows landed.
- **Gate**: gauge non-zero; per-class alert `jiminy_follow_rate_drop_human` becomes eligible (operator flips `JIMINY_FOLLOW_RATE_HUMAN_FLOOR` to a real number if desired).

### Epic 5 — Docs

- **E5.1**: Extend `docs/features/jiminy-metric-partition.md` §Per-class alerts + panels — human-class deferred section → human writer shipped.
- **E5.2**: New CLAUDE.md architecture note.
- **E5.3**: CHANGELOG.md Unreleased/Added.
- **E5.4**: Sprint post reflecting actual live-smoke gauge value.

### Epic 6 (optional, if bandwidth) — UI + operator ergonomics

- **E6.1**: Grafana panel row: pending human-review queue depth (COUNT of pending rows in `guidance_training_rows`) + grading cadence (grades/day from `review_grades` filtered on dataset='human_class_queue').
- **E6.2**: If /ui/rules already lists constraints, add a "human class pending grade" per-rule counter (nice-to-have; ship only if trivial).

## 6. Testing Plan

**Tier 1 — Unit (isolated)**:
- `TestRecordOutcome_HumanClassEmitsPendingRow` — mock class='human' source, verify guidance_training_rows writer receives one row with pending_human_review + class='human'
- `TestRecordOutcome_HumanClassDisabledSkipsEmit` — flag=false → no emit
- `TestHumanClassQueueDataset_SinkWritesClassHuman` — grade at each dimension level → correct outcome_type + verifiability_class='human'
- `TestHumanClassQueueDataset_RejectsAutograder` — sink rejects `grader_id LIKE 'auto:%'`

**Tier 2 — Integration**:
- `internal/jiminy/service_test.go` — existing suite still green
- `internal/review/registry_test.go` — new dataset registered
- `internal/tsdb/dataset_builder_test.go` — GuidanceEffectivenessByClass excludes pending_human_review from credit

**Tier 3 — Live e2e** (mdemg-dev, real binary against real TSDB + Neo4j + HITL):
- End-to-end from RecordOutcome emit → HITL grade → constraint_outcomes row → gauge move
- Auto-grader rejection smoke on real endpoint

## 7. Commit Strategy

Sequential commits per epic:

1. `feat(tsdb): V0037 verifiability_class on guidance_training_rows (E1)`
2. `feat(jiminy): human-class queue emit in RecordOutcome + config (E2)`
3. `feat(review): human_class_queue HITL dataset + sink (E3)`
4. `test: pin human-class integration end-to-end (E1-E3 test bundles)`
5. `docs: JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 sprint plan + post + feature doc + CLAUDE.md + CHANGELOG (E5)`
6. `feat(grafana): human-class queue depth + grading cadence panels (E6, if shipped)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./internal/{alert,config,tsdb,jiminy,review,api,cli,grafanapin}/... -count=1` green
- [ ] `golangci-lint run ./internal/...` 0 issues
- [ ] `python3 scripts/verify_doc_env_vars.py --strict` clean
- [ ] `python3 scripts/verify_metrics_consumers.py` clean
- [ ] `python3 scripts/verify_tsdb_consumers.py` clean (V0037 inventoried; pending_human_review outcome_type adjudicated)
- [ ] `python3 scripts/verify_route_consumers.py` clean (no new routes — reuses `/v1/review/*`)
- [ ] `python3 scripts/verify_config_consumers.py` clean (new knob wired)
- [ ] Live Tier-3 (see §5 E4): end-to-end from emit → grade → constraint_outcomes → gauge
- [ ] Auto-grader rejection smoke live
- [ ] Sprint post reflects actual live-smoke gauge value

## 9. Documentation Update

- Feature doc: `docs/features/jiminy-metric-partition.md` §Human-class writer
- CLAUDE.md architecture note
- CHANGELOG.md Unreleased/Added
- Sprint post `docs/development/jiminy-hitl-human-class-integration-001/sprint_post.md`
- ROADMAP_2026Q5.md §3 #2 checkbox update

## 10. Risks & Mitigations

| Risk | Class | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Emit volume overwhelms HITL queue | operational | MED | MED | Ship default-off; operator flips `.env` after seeing 24h volume. Per-rule sampling deferred to a follow-up if needed. |
| Retroactive breakage of `constraint_outcomes` semantic (adding class='human' rows shifts follow-rate distributions) | measurement | LOW-MED | LOW | Semantic is honest per JIMINY-CEILING-INVESTIGATION-002; adding human rows is exactly the point. Aggregate `mdemg_jiminy_follow_rate` gauge was already retired via alert (JIMINY-METRIC-PARTITION-ALERTS-PANELS-001); no chronic-alert risk. |
| Operator never grades → gauge stays at 0 → HITL-CURATION-002 stall alert fires | operational | HIGH (grading is manual) | LOW (alert is informational) | The `hitl-curation` stall rule counts operator grades ≥ threshold, not gauge > 0; explicit design. If it fires, that's the honest signal ("no one's grading the human class this week"). |
| Migration V0037 fails on prod-like TSDB with a stubborn compress policy | schema | LOW | HIGH | Mirror V0025 + V0035's guarded DO $$ blocks; test migration idempotency locally before push; rollback = DROP COLUMN + revert schema version. |
| RecordOutcome hot path takes new writer round-trip | perf | LOW | LOW | Reuses the shipped async CopyFrom writer; no synchronous DB hit. |
| Auto-grader gets confused if operator invokes `mdemg review autograde --dataset human_class_queue` | operational | LOW | LOW | Sink rejects with named error; smoke-tested in E4.5. |

## 11. Rollback Procedures

Semi-destructive (V0037 adds a column + writer emits new rows). Rollback:

1. Set `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=false` in `.env` + restart — stops new emits immediately
2. Optional: `DELETE FROM constraint_outcomes WHERE verifiability_class='human' AND classifier_source='operator'` — undo human-class grade rows (operator judgment; grading is real data, may want to keep)
3. Optional: `DELETE FROM guidance_training_rows WHERE outcome_type IN ('pending_human_review','graded_human_review')` — undo pending queue rows
4. Migration rollback: `ALTER TABLE guidance_training_rows DROP COLUMN IF EXISTS verifiability_class; UPDATE tsdb_schema_meta SET value='36';` — reverse V0037 (only if operator wants full-column revert; column-default=classifier makes it a no-op for downstream readers otherwise)
5. `git revert <commit-shas>` — undoes Go code + writer + sink + config + docs

Reversibility is preserved; no cascade to other spaces or classes.

## 12. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #2 slot
- `docs/development/jiminy-metric-partition-001/sprint_post.md` — parent
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md` — taxonomy source
- `docs/development/jiminy-metric-partition-alerts-panels-001/sprint_post.md` — sibling (alerts + panels)
- `docs/development/jiminy-process-observer-01/sprint_post.md` — process-writer shape (mirror for the queue-emit pattern)
- `docs/development/hitl-review-001/sprint_post.md` — HITL platform interface
- `docs/development/hitl-curation-002/sprint_post.md` — auto-grader invariant + HITL sink shape
- `docs/development/hitl-curation-003/sprint_post.md` — guidance-dataset extension pattern
- `docs/development/hitl-auto-dismiss-001/sprint_post.md` — NonReinforcingApplier vs standard Apply contract
- `internal/tsdb/migrations/035_verifiability_class.sql` — migration pattern to mirror
- `internal/tsdb/migrations/027_guidance_training_rows.sql` — table shape
- `internal/tsdb/dataset_builder.go::GuidanceEffectivenessByClass` — UNION SQL to extend
- `internal/jiminy/service.go::RecordOutcome` — emit branch injection point
- `internal/review/dataset.go` — ReviewableDataset interface
- `internal/api/server.go` — reviewRegistry wire-up
- `internal/cli/jiminy_constraint_class.go` — 33-rule seed map with 18 human-class rules
- `CLAUDE.md` — arch notes
- `CHANGELOG.md` — Unreleased/Added
- Live TSDB via `docker exec mdemg-timescaledb-1 psql` — schema inspection + current row counts
- Live Neo4j — informational-marking state (18/18 human nodes marked)
