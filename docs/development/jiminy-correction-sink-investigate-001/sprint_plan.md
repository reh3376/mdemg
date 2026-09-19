# JIMINY-CORRECTION-SINK-INVESTIGATE-001 — Sprint Plan

**Task**: #156
**Type**: investigation — data-only, no substrate mutation
**Wall-clock**: ~30min
**Trigger**: JIMINY-CEILING-INVESTIGATION-002 (#153) D6 disclosed `guidance_type='correction'` outcome volume dropped 691→3 pre/post arc; open follow-up: "L1 producer silently broken OR L0 detector not matching."

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-CORRECTION-SINK-INVESTIGATE-001 |
| Task # | #156 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | ZERO — read-only |
| Related sprints | #100 JIMINY-CORRECTION-CORPUS-001 (the actual cause), #101 JIMINY-CORRECTION-PRODUCER-001 (L1 producer), #117 JIMINY-CORPUS-AUDIT-004 (2 additional archives), #153 JIMINY-CEILING-INVESTIGATION-002 (disclosed this follow-up) |

## 2. Problem Statement

Investigate root cause of the 691→3 drop in `constraint_outcomes` rows filtered on `guidance_type='correction'`, disclosed by JIMINY-CEILING-INVESTIGATION-002 D6.

## 3. Scope & Constraints

**In scope**: 9 data dimensions:
- D1: per-day `constraint_outcomes` correction volume
- D2: verdict-type split pre vs post arc
- D3: L0 correction obs stream volume (upstream)
- D4: L1 correction node inventory (total / archived / live)
- D5: top correction codes in pre-arc window
- D6: full detail of post-arc correction outcomes
- D7: archive_reason attribution — WHO archived the nodes?
- D8: cross-check against JIMINY-CORRECTION-CORPUS-001 sprint post
- D9: full ledger of live L1 corrections

**Out of scope**: substrate mutation, code fixes, JIMINY-CORRECTION-PRODUCER-001 recovery-test drill

## 4. Dependencies

- Docker TSDB + Neo4j reachable
- Read-only queries only

## 5. Implementation Plan

Single-flight investigation:
1. D1-D9 queries against live TSDB + Neo4j
2. Correlate archive dates with shipping sprints
3. Verdict + adjacent observations

## 6. Testing Plan

Investigation-only sprint; no code to test. All queries quoted verbatim in `verdict.md` for reproducibility.

## 7. Commit Strategy

Single commit with sprint dir + verdict + CHANGELOG entry.

## 8. Verification Checklist

- [x] D1-D9 queries executed
- [x] Archive attribution traced to specific shipping sprints
- [x] Verdict rendered
- [x] Sprint dir populated
- [x] CHANGELOG Unreleased entry

## 9. Documentation Update

- `docs/development/jiminy-correction-sink-investigate-001/{sprint_plan,verdict}.md`
- `CHANGELOG.md` — Unreleased Investigated entry

## 10. Risks & Mitigations

None — read-only.

## 11. Rollback

N/A.

## 12. Documents Accessed

See verdict.md § Documents Accessed.
