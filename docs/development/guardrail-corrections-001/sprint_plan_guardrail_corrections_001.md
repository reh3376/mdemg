# Sprint GUARDRAIL-CORRECTIONS-001 — corrections join guardrail retrieval

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | GUARDRAIL-CORRECTIONS-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~0.5 dev-day |
| Parent | GUARDRAIL-PRODUCER-001 disclosed follow-up: `role_type='correction'` nodes (35 live on mdemg-dev, minted by JIMINY-CORRECTION-PRODUCER-001) are durable rules invisible to guardrail retrieval |

## 2. Problem Statement

Guardrail evaluates diffs against `role_type='constraint'` only. The
correction partition carries equally durable operator-taught rules
("never `mdemg db start`", "max_completion_tokens for gpt-5.x", …) that the
evaluator never sees — so producer rows can't learn them and the MCP
`validate_changes` verdicts ignore them. Corrections have **no
`constraint_type`** (verified: 0/35), which conveniently maps them to
Warning tier via the existing `isBlockingType("")=false` path — advisory
strength, the right semantic for learned lessons vs hard constraints.

## 3. Scope & Constraints

**In scope:** retrieval union (`role_type IN ['constraint','correction']`)
in BOTH semantic (partition cosine) and keyword phases, behind
`GUARDRAIL_INCLUDE_CORRECTIONS` (code default **false**, `.env` flip after
smoke — the standing contract); Cypher renders `constraint_type` as
`'correction'` for correction nodes (prompt clarity + Warning mapping);
unit tests; live Tier-3 producer run matching a real correction; docs.
**Out of scope:** structured-correction prompt rendering
(`correction_incorrect/correct` fields — future refinement); Block-tier
corrections; scope/authority changes (corrections pass the existing
coalesce clauses as unscoped `team_standard`).
**Constraints:** default-off flag; the sync MCP path changes identically
(one retrieval function — disclosed, not gated separately); fail-open
untouched.

## 4. Dependencies

✅ Partition-cosine retrieval (GUARDRAIL-PRODUCER-001 fix); ✅ 35 live
corrections, 34 embedded; ✅ producer live for the smoke.

## 5. Implementation Plan (sequential)

- **E0** this plan. **E1** config knob + retrieval union + type coalesce +
  unit tests. **E2** live Tier-3: flag on → hook fixture matching a real
  correction (e.g. `mdemg db start`) → row with correction in prompt;
  flag off → constraints-only (byte-identical legacy). **E3** docs
  (feature-doc §, CHANGELOG, post).

## 6. Testing Plan

Tier 1: unit (flag off = legacy Cypher; on = union + coalesced type).
Tier 2: `go test ./...`. Tier 3: E2 live smoke both flag states.

## 7. Commit Strategy

`docs(E0)` → `feat(E1)` → `docs(E2+E3)`.

## 8. Verification Checklist

unit green · build+lint · live: correction surfaced in a producer prompt ·
flag-off legacy-identical · docs · pushed.

## 9. Documentation Update

`docs/features/guardrail-producer.md` follow-up § marked shipped;
CHANGELOG; post.md.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Verdict drift on the sync MCP path | Low | Corrections cap at Warning (never Block); default-off flag; disclosed |
| Prompt growth (more matches) | Low | `GUARDRAIL_MAX_CONSTRAINTS` cap unchanged (10) |

## 11. Rollback

`GUARDRAIL_INCLUDE_CORRECTIONS=false` (or revert).

## 12. Documents Accessed

`internal/guardrail/{constraint_retrieval,guardrail,prompt,response_builder}.go`;
live Neo4j correction-partition counts; GUARDRAIL-PRODUCER-001 post;
JIMINY-CORRECTION-PRODUCER-001 note (CLAUDE.md).
