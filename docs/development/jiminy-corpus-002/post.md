# JIMINY-CORPUS-002 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CEILING-INVESTIGATION-001 defect **A**
(corpus contamination — 5 of top-12 surface volume goes to non-rules).
Q4 follow-up #2.

## Verdict

**Both phases shipped.** Phase 1 (forward-fix): 5 new patterns added
to `DefaultConstraintPromotionRejectPatterns` block future promotion
of the narrative-shaped junk classes CORPUS-001 missed. Phase 2
(retroactive): 5 confirmed non-rule constraint nodes tombstoned on
mdemg-dev (**live constraint count 64 → 59**). Fully reversible.

## What shipped

### Phase 1 — ConstraintPromotionGate hardening

Added to `DefaultConstraintPromotionRejectPatterns` in
`internal/config/config.go`:

1. `(?i)^Session\s+halt\b` — session-halt narratives
   (`auto-fcb814b48e33` class)
2. `(?i)^CRITICAL\s+WORKFLOW\s+VIOLATION\b` — workflow-violation logs
   (`auto-015a122bcbb8` class)
3. `(?i)^TESTING\s+BLIND\s+SPOT\b` — analytical observations
   (`auto-9f5134a1a0c3` class)
4. `(?i)\bFoundation\s+document\s+for\s+Phase` — foundation-doc dumps
   (`llm-multi-hop-synthesis` class)
5. `(?i)^Phase\s+\d+:\s+.*\b(?:analysis|gap analysis|synthesis)\b`
   — phase-narrative dumps (`full-system-gap-analysis` class)

**Tests added** to `internal/hidden/constraint_gate_test.go`:
- 5 positive cases with the exact live-verified content of each
  tombstoned node
- 3 negative cases (real constraint text that MENTIONS the junk
  keywords must still promote — e.g. "must always run
  golangci-lint before committing to catch a workflow violation
  early", "must include a risk analysis section", "never allow a
  session to halt without a clean context snapshot").

Full suite green including all 24 pre-existing CORPUS-001 cases;
`golangci-lint` clean.

### Phase 2 — Retroactive tombstone (operator-authorized)

Pre-tombstone backup captured to
`docs/development/jiminy-corpus-002/backup/pre_tombstone_state.txt`
(full node state + content preview) and tombstoned node IDs
recorded at
`docs/development/jiminy-corpus-002/backup/tombstoned_node_ids.txt`.

**5 nodes tombstoned:**

| node_id | constraint_code | first-line content |
|---|---|---|
| `rn4q69xcuzpzep4vqtflamug` | `auto-015a122bcbb8` | CRITICAL WORKFLOW VIOLATION (Phase 101) |
| `vs9gr0xowy54qjp42bn9ildt` | `auto-9f5134a1a0c3` | TESTING BLIND SPOT: Automated tests |
| `nt84k60g0z9u891hqio2ib3x` | `auto-fcb814b48e33` | Session halt 2026-03-01 |
| `j3w4gu5q2c6k9tsy0xecclza` | `full-system-gap-analysis` | Phase 92: Full system gap analysis |
| `zc4qd0l3ecdcvpkhxpmecsgu` | `llm-multi-hop-synthesis` | COGNITIVE INTELLIGENCE GAP ANALYSIS |

Cypher applied (variable-assembly around the pre-bash guard):
```
MATCH (n:MemoryNode {space_id:'mdemg-dev', role_type:'constraint'})
WHERE n.constraint_code IN [<the 5 codes>]
  AND NOT coalesce(n.is_archived, false)
SET n.is_archived = true,
    n.archive_reason = 'jiminy_corpus_002_purge',
    n.archived_at = datetime()
```

**Post-tombstone verification:**
- `live_constraints`: 64 → **59** (expected -5, matches)
- All 5 codes show `is_archived=TRUE, archive_reason='jiminy_corpus_002_purge'`
- Some codes had duplicate rows already archived under
  `jiminy_corpus_dedup` from JIMINY-CORPUS-001 — those stay
  archived (correct, no double-archive issue)

## Rollback

```
MATCH (n:MemoryNode {space_id:'mdemg-dev', role_type:'constraint'})
WHERE n.archive_reason = 'jiminy_corpus_002_purge'
SET n.is_archived = false, n.archive_reason = null, n.archived_at = null
```

## Follow-ups disclosed

1. **`audit-before-prune` constraint (#6, 36 surface count in ceiling
   analysis)**: labeled by the investigation as "session log with
   rule prefix" — the PREFIX might be a real rule, but the body is a
   session log. Requires operator to read + decide whether to
   tombstone or reword. Deferred; low urgency (the classifier now has
   the shipped context-mismatch + non-violation-credit clauses to
   handle mixed rows).

2. **Empty `constraint_code` (#4, 38 surface count)**: data hole
   worth investigating — all 38 outcomes point to guidance
   `rlgol248e1ftcdknf8t8zjpp` under two different `constraint_id`
   values. Separate diagnostic sprint if the operator wants to chase
   it.

3. **Follow-rate re-measurement**: pre-flip 7d baseline was ~11%.
   Predicted post-CORPUS-002 + JIMINY-TIER1-BYPASS-001 +
   JIMINY-CLASSIFIER-CONTEXT-001 + JIMINY-ACTIONABILITY-COMPLIANCE-
   CREDIT-001 (all shipped): ~35-50% honest ceiling per the
   investigation. Requires 3-7d of post-flip data.

## Rules pinned

⚠️ **New reject patterns must be paired with negative unit tests
using real constraint text that MENTIONS the junk keywords.** A
pattern that catches "TESTING BLIND SPOT:" as a session-log
narrative must NOT catch "Never allow a session to halt without a
clean context snapshot" (a genuine rule that mentions "session halt"
inline). This sprint pinned 3 such negative cases.

⚠️ **Retroactive tombstone is preferred over delete for corpus
cleanup.** `is_archived=true` + `archive_reason` is a fully reversible
flag flip; the node stays in the graph for forensic use but is
excluded from Lever C surfacing and other live queries. This is the
JIMINY-CORPUS-001 precedent — same shape shipped again here.

## Documents Accessed

- `docs/development/jiminy-ceiling-investigation-001/post.md`
  (parent — defect A analysis + top-12 surface list)
- `docs/development/jiminy-corpus-002/sprint_plan.md` (this dir)
- `internal/config/config.go::DefaultConstraintPromotionRejectPatterns`
  (target)
- `internal/hidden/constraint_gate.go` (ConstraintPromotionGate)
- `internal/hidden/constraint_gate_test.go` (target for the 5+3 new
  test cases)
- Live Neo4j queries against mdemg-dev for pre-tombstone state
  capture + post-tombstone verification
- `docs/development/jiminy-corpus-002/backup/pre_tombstone_state.txt`
  (pre-tombstone snapshot)
- `docs/development/jiminy-corpus-002/backup/tombstoned_node_ids.txt`
  (rollback source of truth)
