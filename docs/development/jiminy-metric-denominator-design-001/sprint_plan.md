# JIMINY-METRIC-DENOMINATOR-DESIGN-001 — Sprint Plan

**Task**: #157
**Type**: design — spec production only, no code
**Wall-clock**: ~2h
**Trigger**: JIMINY-CEILING-INVESTIGATION-002 (#153) identified metric-denominator dominated by classifier-unverifiable rules as the root cause of the follow-rate ceiling gap; Path 2 (process observers) vs Path 3 (metric partitioning) is the pre-retrain design decision.

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-METRIC-DENOMINATOR-DESIGN-001 |
| Task # | #157 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | ZERO — design/spec only |
| Reversible? | N/A (no writes) |
| Related sprints | #99 JIMINY-INFORMATIONAL-CATEGORY-001 (Path 1 escape hatch), #153 JIMINY-CEILING-INVESTIGATION-002 (identified the root cause), #155 Path 1 execution (marked 9 process rules informational), #156 JIMINY-CORRECTION-SINK-INVESTIGATE-001 (confirmed correction sink health) |
| Feeds | Phase 4b retrain gate decision |

## 2. Problem Statement

Post-Path-1 (2026-09-06), the metric denominator for actionable follow rate collapsed to a small set of classifier-verifiable rules. This is a MEASUREMENT REFRAME, not a QUALITY improvement. Before any retrain (Phase 4b) is triggered, we need an honest scoreboard architecture that:

1. Separates rules by verifiability class (classifier / process / human)
2. Grades each class with a signal APPROPRIATE to that class
3. Gates retrain compute on the metric class where retrain CAN help (classifier-verifiable)
4. Provides an honest per-class dashboard so operators can see where improvements are needed

## 3. Scope & Constraints

**In scope**:
- **Full rule taxonomy audit** — classify all 33 live actionable rules by verifiability class
- **Path 2 spec** — process-observation architecture: event catalog, emitter list, new verdict types, TSDB schema, grader design
- **Path 3 spec** — metric partitioning architecture: `verifiability_class` node property, per-class gauges, RecordOutcome routing
- **Cost + risk comparison** — sprint counts, engineering effort, code surface, migration path
- **Recommendation** — Path 2, Path 3, or hybrid; sequenced follow-up sprints

**Out of scope**:
- Implementation (this is a design sprint — the recommendation becomes 1-N follow-up sprints)
- Substrate mutation (no `verifiability_class` writes; no observer wiring; no schema deploys)
- HITL platform extension (Path 2's human-class subset touches HITL; separate design if pursued)

**Constraints preserved**:
- `plan-mode-before-change` — this doc IS the plan; no code touched
- `must-follow-12-section-format` — this sprint plan follows the standard
- `unit-integration-e2e-docs` — N/A (design-only sprint)
- `mandatory-feature-docs` — N/A (no shipped feature; if Path 2/3 ships, feature doc lands with that sprint)
- `never-hardcode-config` — proposed configs in the specs use env-var conventions consistent with shipped patterns

## 4. Dependencies

- Read access to Neo4j (`mdemg-dev` MemoryNode enumeration) — completed
- Read access to shipped sprint docs (#99, #153, #155, #156)
- Zero substrate or code dependencies

## 5. Implementation Plan

Single-flight design work:
1. **Phase A** — enumerate all 33 live actionable rules; per-rule verifiability classification
2. **Phase B** — Path 2 full spec (12 sub-questions covered)
3. **Phase C** — Path 3 full spec (7 sub-questions covered)
4. **Phase D** — hybrid analysis + sequencing
5. **Phase E** — actionable recommendation with follow-up sprint list

## 6. Testing Plan

Design-only sprint — no code, no tests. Verification is that the specs are:
- Complete (all 33 rules classified)
- Actionable (each proposed sprint is scoped enough to be planned in the standard 12-section format)
- Reversible (recommendation names both a preferred path AND a fallback if the preferred path fails during implementation)

## 7. Commit Strategy

Single commit: sprint dir with `sprint_plan.md` + `verdict_and_specs.md` + CHANGELOG entry + CLAUDE.md pin for the taxonomy rule that emerges.

## 8. Verification Checklist

- [x] All 33 rules enumerated from live Neo4j
- [x] Each rule assigned a verifiability class
- [x] Path 2 spec covers 12 sub-questions
- [x] Path 3 spec covers 7 sub-questions
- [x] Hybrid analysis produced
- [x] Recommendation names a preferred path + a fallback
- [x] Sprint dir populated
- [x] CLAUDE.md pin proposed
- [x] CHANGELOG Unreleased entry

## 9. Documentation Update

- `docs/development/jiminy-metric-denominator-design-001/{sprint_plan,verdict_and_specs}.md`
- `CLAUDE.md` — arch note pin
- `CHANGELOG.md` — Unreleased Designed entry

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| The recommendation picks the wrong path | Both specs are fully written; recommendation includes explicit "fallback if pursued path fails" so operator has both paths ready |
| Taxonomy classification is wrong | Each rule includes classifier/process/human justification; operator can re-classify on review; taxonomy is a Path 3 property write, easily amended |
| Path 2 scope is under-estimated | Cost table names per-observer engineering effort; hybrid recommendation sequences small observers first |
| Design doc goes stale before implementation | Path 3 recommendation is ~1 sprint away and can ship immediately after operator approval |

## 11. Rollback

N/A — no writes.

## 12. Documents Accessed

- Live Neo4j `MemoryNode {space_id: 'mdemg-dev', role_type: 'constraint' OR 'correction', NOT is_archived}` — 33 rows for taxonomy
- `docs/development/jiminy-ceiling-investigation-002/verdict.md` — established Path 2 vs Path 3 framing
- `docs/development/jiminy-ceiling-investigation-002-execution/sprint_post.md` — Path 1 execution outcome
- `docs/development/jiminy-correction-sink-investigate-001/verdict.md` — confirmed correction sink health
- `docs/development/jiminy-informational-category-001/` — shipped `is_informational` property + CLI (Path 3's node-property mechanism could extend this)
- CLAUDE.md JIMINY-CEILING-INVESTIGATION-002 arch pin
- CLAUDE.md JIMINY-INFORMATIONAL-CATEGORY-001 pin
- Operator directive: "run the path 2/3 design sprint" (2026-09-07)
