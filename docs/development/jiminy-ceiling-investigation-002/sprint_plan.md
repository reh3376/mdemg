# JIMINY-CEILING-INVESTIGATION-002 — Sprint Plan

**Task**: #153
**Type**: investigation — data-only, no substrate mutation
**Wall-clock**: ~1h
**Trigger**: JIMINY-CEILING-BREAK-2 arc trajectory metric (post-Phase 3) landed at +4.74pp vs +15-25pp predicted through Phase 3 alone. Reconcile plan-vs-reality BEFORE committing to Phase 4b's 30-100h retrain.

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-CEILING-INVESTIGATION-002 |
| Task # | #153 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | ZERO — read-only investigation |
| Reversible? | N/A (no writes) |
| Reference sprint | JIMINY-CEILING-INVESTIGATION-001 (the first arc-diagnostic — established the pattern) |

## 2. Problem Statement

JIMINY-CEILING-BREAK-2 shipped 5 substantive phases + 1 ship-dormant classifier probe between 2026-08-11 and 2026-08-14:

| Phase | Sprint | Expected lift | 168h post-arc measured |
|---|---|---|---|
| 1 | JIMINY-CORPUS-003 (corpus purge) | +5pp | see D1 |
| 2 | LEVER-C-TIGHTEN-002 (scope gate) | +5-10pp | see D1 |
| 3 | JIMINY-CLASSIFIER-CONTEXT-002 (mech-scope) | +5-10pp | see D1 |
| 3.5 | JIMINY-CLASSIFIER-META-SCOPE-001 | 0pp (ship-dormant) | flag now ON (⚠️ investigate) |
| 4a | JIMINY-HITL-VELOCITY-001 | 0pp (corpus substrate) | expected null |

**Predicted cumulative through Phase 3**: baseline 12% + 15-25pp = 27-37%.
**Measured** (168h at `constraint_outcomes` on `guidance_type IN ('constraint','correction')`): **16.74%**.

Gap: **~10-20pp short of prediction.** Auto-executing Phase 4b (30-100h retrain + compute) without reconciling the gap risks wasted compute if the ceiling arithmetic is structurally wrong at earlier phases.

## 3. Scope & Constraints

**In scope** (7 data dimensions — all read-only against TSDB + Neo4j):

- **D1** Per-phase attribution — pre-arc baseline vs each shipping-window post-arc
- **D2** Surfacing volume + composition — volume-per-day + actionable share
- **D3** Classifier verdict distribution shift — `outcome_type` + `classifier_source` before/after
- **D4** Top-N ignored constraint codes — where does the ignored fire concentrate?
- **D5** Informational-flag adoption — did the top ignored codes get JIMINY-INFORMATIONAL-CATEGORY-001 escape hatch applied?
- **D6** Guidance-type mix drift — pattern/learning/concept/constraint/correction share
- **D7** Classifier prompt-flag current state (`.env` — CONTEXT + META-SCOPE flags)

**Bonus** — sanity check: recompute follow rate EXCLUDING the top-N process-class codes to isolate action-content follow rate.

**Out of scope**:
- Substrate mutation (except: potential subsequent sprint may apply the recommended fix; not this sprint)
- Corpus quality audit (was done by Fable in JIMINY-CORPUS-AUDIT-004, #117)
- Retraining (that IS Phase 4b, decision-gated on this investigation)

## 4. Dependencies

- Docker Neo4j + TimescaleDB reachable (both live-verified)
- Read-only queries only

## 5. Implementation Plan

Single-flight investigation:
1. Run D1-D7 batched queries
2. Synthesize findings into `verdict.md`
3. Propose forward paths ranked by evidence-strength
4. Commit sprint dir + docs

## 6. Testing Plan

Investigation-only sprint; no code to test. All Tier-3 live queries against real production Neo4j + TSDB. Reproducibility: every query is quoted verbatim in `verdict.md` so any future investigator can re-run.

## 7. Commit Strategy

Single commit with sprint dir + `verdict.md` + CLAUDE.md architecture note + CHANGELOG entry.

## 8. Verification Checklist

- [x] D1-D7 queries executed against live TSDB + Neo4j
- [x] Verdict synthesized with per-dimension findings
- [x] Root-cause hypothesis identified
- [x] Forward-path options ranked with cost + evidence-strength
- [x] Sprint dir populated
- [x] CLAUDE.md architecture note pinned
- [x] CHANGELOG Unreleased entry

## 9. Documentation Update

- `docs/development/jiminy-ceiling-investigation-002/{sprint_plan,verdict}.md` — this sprint
- `CLAUDE.md` — arch note pin (see verdict § for content)
- `CHANGELOG.md` — Unreleased Investigated entry

## 10. Risks & Mitigations

None — read-only investigation. Zero substrate risk.

## 11. Rollback

N/A — no writes.

## 12. Documents Accessed

- Live TSDB `constraint_outcomes` hypertable (720h window scan + per-phase windowed slices)
- Live Neo4j `MemoryNode {space_id: 'mdemg-dev', role_type IN ['constraint','correction']}` (informational-flag audit + top-12 code look-up)
- `.env` (CONTEXT-001, CONTEXT-002, META-SCOPE flag state — read-only)
- `docs/development/jiminy-ceiling-break-2/README.md` — arc plan
- `docs/development/jiminy-ceiling-investigation-001/` — reference sprint (established the diagnostic pattern)
- Predecessor sprint outputs: JIMINY-CORPUS-003, LEVER-C-TIGHTEN-002, JIMINY-CLASSIFIER-CONTEXT-002, JIMINY-CLASSIFIER-META-SCOPE-001, JIMINY-INFORMATIONAL-CATEGORY-001, JIMINY-HITL-VELOCITY-001
- CLAUDE.md pins (§3 sprint plan)
- Operator directive: "run option a" (2026-09-03)
