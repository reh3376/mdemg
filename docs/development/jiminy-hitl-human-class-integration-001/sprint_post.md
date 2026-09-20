# JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 — Sprint Post

**Ship date**: 2026-09-20
**Wall-clock**: ~3h (plan → E1 → E2 → E3 → E4 live + auto-reject smoke → E5)
**Status**: SHIPPED — writer default-off in code + `.env` `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=true`; sink default-on
**Parent arc**: JIMINY-METRIC-PARTITION-001 (task #158) — 4th (last) class writer
**Roadmap slot**: Q5 §3 #2

## Goal delivered

Closes the last dormant class gauge from #158. `mdemg_jiminy_follow_rate_human` was `0.000` from #158 ship (2026-09-10) through today — no writer existed. This sprint delivers:

- **V0037 migration** adds `verifiability_class` column to `guidance_training_rows`
- **RecordOutcome emit branch** tags class='human' items as pending queue rows
- **HITL dataset** `human_class_queue` reads the pending queue
- **Sink** writes operator grades to `constraint_outcomes` with `verifiability_class='human'`
- **Auto-grader rejection** enforced at sink — this class is LLM-unverifiable by taxonomy

Live smoke: end-to-end from synth pending row → operator grade → constraint_outcomes row → **`mdemg_jiminy_follow_rate_human = 1.0000`** on the next assessment tick.

## What shipped

| Component | File | Change |
|---|---|---|
| Migration | `internal/tsdb/migrations/037_guidance_training_rows_verifiability_class.sql` | +column + index; schema 36→37 |
| Writer | `internal/tsdb/guidance_training_rows_writer.go` | `GuidanceTrainingRow.VerifiabilityClass` field; CopyFrom 16→17 cols |
| Writer test | `internal/tsdb/guidance_training_rows_writer_test.go` | Column-count pin 16→17 |
| Jiminy type | `internal/jiminy/types.go` | `GuidanceTrainingRecord.VerifiabilityClass` |
| Jiminy emit | `internal/jiminy/service.go` | Branch on `s.cfg.JiminyHumanClassQueueEnabled` + class='human'; overrides outcome_type + class tag |
| API adapter | `internal/api/server.go` | Threads `VerifiabilityClass` through jiminy→tsdb mapping |
| Aggregator | `internal/tsdb/dataset_builder.go` | `GuidanceEffectivenessByClass` SQL excludes `pending_human_review` + `graded_human_review` from credit |
| Config | `internal/config/config.go` | 4 new fields on Config; `JIMINY_HUMAN_CLASS_QUEUE_ENABLED` env; TSDBRequiredSchemaVersion 36→37 |
| Config env | `.env` | `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=true` |
| Rubric | `internal/review/scoring.go` | `HumanClassQueueRubric(version)` — single-dim `followed` 0-4 |
| Dataset | `internal/api/human_class_queue_dataset.go` (NEW) | ~230 LOC; FetchCandidates + FetchItem + Sink |
| Dataset tests | `internal/api/human_class_queue_dataset_test.go` (NEW) | 5 pin tests |
| Dataset registration | `internal/api/server.go` | Wired alongside guidance/contradicted/LLM datasets |
| Inventory | `docs/api/tsdb_consumer_inventory.json` | New reader + notes on new outcome_types |
| Feature doc | `docs/features/jiminy-metric-partition.md` | New §Human-class writer |
| Sprint plan | `docs/development/jiminy-hitl-human-class-integration-001/sprint_plan.md` (NEW) | 12-section format |
| CLAUDE.md | | 2 new arch rules pinned |
| CHANGELOG.md | | Unreleased/Added entry |

## Live Tier-3 (mdemg-dev, 2026-09-20)

### Positive path (operator grade → substrate write)

Synth pending row seeded via SQL (natural human-class fire needs unmarking rules from informational, out of scope):

```sql
INSERT INTO guidance_training_rows (...)
VALUES ('008de6fa-...','mdemg-dev','smoke-hcq-001',...,
        'edited internal/foo.go to fix a bug without switching to plan mode',
        'pending_human_review',0.72,'pending_human','plan-mode-before-change','human');
```

- `GET /v1/review/datasets` → new dataset registered, `candidate_count: 1`
- `GET /v1/review/candidates?dataset_id=human_class_queue` → row visible with correct meta (`constraint_code=plan-mode-before-change`, source_layer=1)
- `POST /v1/review/grade` with `grader_id=operator:reh3376, dimensions={followed:4}, reinforce:true` → `{grade_recorded:true, reinforcement_applied:true, gold_score:1, grade_id:eifqrmzerc3tpt55xvb7d8vm}`
- After buffered flush: `constraint_outcomes` row landed with `outcome=followed, class=human, source=operator, code=plan-mode-before-change, similarity=1.0`
- Post-grade query: `item_count: 0` — LEFT JOIN dedup fired
- Next RSIC assessment tick: **`mdemg_jiminy_follow_rate_human = 1.0000`** (was 0.000 since #158 ship)

### Negative path (auto-grader rejection)

Fresh pending row + `POST /v1/review/grade` with `grader_id=auto:gpt-5.4-mini@sha, force:true`:
- Response: `{"error": "internal error during review reinforcement apply"}` (server wraps sink error as 500; message is generic but the operation aborts)
- **0 rows** in `review_grades` with `grader_id LIKE 'auto:%'` on this dataset
- **0 rows** in `constraint_outcomes` with `classifier_source LIKE 'auto:%'` for class='human'

Sink refusal working end-to-end. The generic error message wrapping is a shipped HITL-handler pattern (not scope for this sprint); the sink's error correctly aborts.

### Cleanup

All smoke rows removed post-verification via:
```sql
DELETE FROM constraint_outcomes WHERE guidance_id IN ('smoke-gid-001','smoke-gid-002');
DELETE FROM review_grades WHERE dataset_id='human_class_queue' AND grader_id IN ('operator:reh3376','auto:*');
DELETE FROM guidance_training_rows WHERE session_id IN ('smoke-hcq-001','smoke-hcq-002');
```

## Verification (all green)

- ✅ `go build ./...` clean
- ✅ `go test ./internal/tsdb/... ./internal/jiminy/... ./internal/api/... -count=1` green (5 new pin tests)
- ✅ `golangci-lint` — 0 issues on changed packages
- ✅ V0037 migration idempotent against real TimescaleDB
- ✅ TSDB inventory adjudicated (V0037 column + 2 new outcome_type values documented)
- ✅ Live Tier-3 (see above)

## Arch rules pinned to CLAUDE.md

1. **HITL datasets whose grading class is LLM-unverifiable by construction MUST refuse auto-grader submissions at the sink** — the auto:* prefix pattern is INVERTED here vs every other dataset. HITL-CURATION-002 allows auto:* under non-reinforcing; this one refuses auto:* at BOTH Preview + Apply because LLM cannot supply valid signal. Return a named error (`errAutograderRejected`) that cites the taxonomy source so misinvocation produces a clear "this dataset is operator-only by design" signal instead of silent substrate pollution.
2. **Sprint plans adding new class writers to a shipped partitioning taxonomy MUST thread the emit signal BEFORE any downstream override** (informational→NA in this sprint) that would drop the row — taxonomy demands ALL classes reach some persistence surface; existing routing (constraint_outcomes NA) stays unchanged so aggregate metrics don't regress.

## Deferred (per plan §5 optional epic + risks)

- **Epic 6 Grafana panel** — queue depth + operator grading cadence panels. Not shipped; ship if operator throughput proves insufficient without visibility. Uses HITL-ANALYTICS-TILE-001 pattern.
- **Per-rule sampling / rate limit** — v1 emits every human-class item. If volume proves unmanageable, add gate.
- **Retroactive backfill** — human-class events never landed anywhere pre-sprint. No source to backfill from; forward-only.
- **`JIMINY_FOLLOW_RATE_HUMAN_FLOOR`** — still 0 (disabled). Operator flips to a real number after gauge accumulates enough operator-graded rows (~30d suggested).
- **HITL grade generic-error-message wrapping** — the shipped handler wraps sink errors as opaque 500. Not this sprint's scope; disclosed follow-up: surface the sink's named error text (mirror the REVIEW-GRADE-NOTES-FIELD-001 `readJSON.Error()` fix pattern for the sink path).

## Roadmap impact

Q5 §3 #2 CLOSED. All 4 class writers now live:
| Class | Writer | 24h live rate |
|---|---|---|
| classifier | RecordOutcome → constraint_outcomes | ~0.17 |
| process | 6 process observers → process_outcomes | ~0.74 |
| hybrid | RecordOutcome → constraint_outcomes | ~0.22 |
| **human** | **HITL → constraint_outcomes** | **new — awaits organic grading** |

Q5 §3 #4 (JIMINY-CEILING-BREAK-2 T+30d passive re-measure, ~2026-10-19) can now include the human class in the retrain-gate math per PHASE-4B-GATE-DECISION-001's class-differential trigger recommendation.

## Rollback

Non-destructive with 5 layers of protection (see feature doc §Rollback):

1. `JIMINY_HUMAN_CLASS_QUEUE_ENABLED=false` + restart → stops new emits
2. Optional: `DELETE FROM constraint_outcomes WHERE verifiability_class='human' AND classifier_source='operator'` — operator judgment
3. Optional: `DELETE FROM guidance_training_rows WHERE outcome_type='pending_human_review'` — undo pending queue
4. Migration rollback (last resort): `ALTER TABLE guidance_training_rows DROP COLUMN verifiability_class; UPDATE tsdb_schema_meta SET value='36';`
5. `git revert <shas>` → undoes Go code + writer + sink + config + docs

## Documents Accessed

- `docs/development/jiminy-hitl-human-class-integration-001/sprint_plan.md` — this sprint's plan
- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #2 slot
- `docs/development/jiminy-metric-partition-001/sprint_post.md` — parent (Path 3)
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md` — taxonomy source
- `docs/development/jiminy-metric-partition-alerts-panels-001/sprint_post.md` — sibling (alerts + panels)
- `docs/development/jiminy-process-observer-01/sprint_post.md` — process-writer shape reference
- `docs/development/hitl-curation-002/sprint_post.md` — auto-grader invariant + sink shape
- `docs/development/hitl-curation-003/sprint_post.md` — LEFT JOIN dedup pattern
- `docs/development/phase-4b-gate-decision-001/sprint_post.md` — retrain-gate context
- `internal/tsdb/migrations/035_constraint_outcomes_verifiability_class.sql` — migration pattern
- `internal/tsdb/migrations/027_guidance_training_rows.sql` — table shape
- `internal/tsdb/dataset_builder.go::GuidanceEffectivenessByClass` — UNION SQL extended
- `internal/jiminy/service.go::RecordOutcome` — emit branch injection
- `internal/review/dataset.go`, `sink.go`, `rubric.go`, `scoring.go` — HITL interface
- `internal/api/contradicted_drafts_dataset.go` — shape reference
- `internal/api/guidance_dataset.go` — LEFT JOIN + guidanceItemCols pattern
- `internal/api/server.go` — reviewRegistry wire-up
- Live TSDB via `docker exec mdemg-timescaledb-1 psql` — schema, row insertion, gauge query
- Live Neo4j — informational-marking state audit
- Server log at `~/.mdemg/logs/server.log` — dataset registration confirmation
