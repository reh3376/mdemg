# JIMINY-ARCHIVED-CODE-FILTER-001 — Sprint Post

**Date:** 2026-08-10
**Branch:** `reh3376_dev01`
**Type:** Hot-fix — 5 sibling sites in 4 files, plus pin test.

## Summary

Operator reported "Jiminy guidance follow rate is trending downward and it needs to be investigated." Investigation (see conversation trace + JIMINY-FOLLOW-RATE-REMEASURE-001 for baseline shape) traced the Aug 8 drop-to-6.76% signal to three compounding factors:

1. Small-sample day (74 vs typical 130-590 actionable outcomes)
2. **Archived-constraint drag** (this sprint fixes) — 17 outcomes/day on code `auto-9f5134a1a0c3` which had 4 archived + 1 archived nodes and ZERO live nodes
3. Session-pattern effect (narrow tactical work matches few durable-rule surfaces — not a bug)

Factor 2 was the actionable defect. Shipped as this sprint.

## What we shipped

Filter `NOT coalesce(*.is_archived, false)` on 5 Cypher READ blocks that MATCHed constraint MemoryNodes:

- `internal/jiminy/service.go::loadSpaceConstraintCodes` — the primary bug source (fallback keyword matcher's pool). Also tightened `role_type` to `IN ['constraint','correction']`.
- `internal/jiminy/service.go::buildConstraintGlossary` — LLM prompt glossary.
- `internal/jiminy/persistence.go::findConstraintNodeID` — code→node lookup used by `PersistGuidanceOutcome`'s target-node resolution.
- `internal/jiminy/persistence.go::PersistGuidanceOutcome` MATCH src — defense-in-depth on the edge-creation site.
- `internal/jiminy/persistence.go::FindConstraintCodeForNode` — reverse lookup used by JIMINY-CODE-BACKFILL-001's defensive backfill.
- `internal/jiminy/persistence.go::GetConstraintEffectiveness` — per-constraint stats reader.
- `internal/jiminy/stats.go::computeWeightedEffectiveness` — effectiveness score.

Plus a pin test at `internal/jiminy/archived_code_filter_test.go` that walks all 4 files and asserts every Cypher block MATCHing constraint nodes includes an archive-filter regex match.

## Live verification stats (mdemg-dev)

Pre-fix constraint-code pool: **261 nodes**
- 128 archived (previously leaking to fallback matcher → analytics drag)
- 103 live+correct-role (the honest pool)
- ~30 misc (wrong role_type or edge cases)

Post-fix pool: **103 live+correct-role only** (~49% of the pre-fix pool eliminated).

The pin test caught 5 sibling misses beyond the primary bug — 4 of them I hadn't identified in my initial hand-audit. Regression contract now guarantees future constraint-code Cypher additions include the filter.

## Why the "hand-audit missed 4" is important

The initial audit only checked `loadSpaceConstraintCodes`. When I ran the pin test, it flagged 4 more sites (findConstraintNodeID, PersistGuidanceOutcome MATCH src, FindConstraintCodeForNode, GetConstraintEffectiveness, buildConstraintGlossary, computeWeightedEffectiveness). Every one of these could have been a source of drift if left unfixed — but visually they don't look like "the same bug" until you enumerate them systematically.

**Arch lesson**: for any Cypher-based property filter that has semantic implications (like `is_archived`), the correct sprint shape is (a) identify the pattern, (b) write a pin test that enforces the pattern EVERYWHERE, (c) let the pin test fail and identify all offending sites, (d) fix. Hand-audit + fix is prone to miss siblings.

## Arch rule pinned (new)

⚠️ **Every Cypher block in `internal/jiminy/` that MATCHes a `MemoryNode` by `role_type='constraint'` (or `role_type IN ['constraint','correction']`) for READ purposes MUST also filter `NOT coalesce(*.is_archived, false)`.** Regression-pinned via `TestConstraintCodeCypher_AllPathsFilterArchived`.

Exceptions (allowlisted in the test):
- The archive setter itself (`SET n.is_archived = true`) — obviously can't filter what it's about to write
- BootstrapCodes / BootstrapCorrectionCodes SETTERs (`SET n.constraint_code = $code`) — setting a code on an archived node is a harmless no-op operationally; filtering here would silently skip newly-un-archived nodes

## What this fix does NOT do

- **Does NOT retroactively clean up historical `constraint_outcomes` rows** that were recorded with archived codes pre-fix. Those are honest reflections of past guidance surfaces at the moment they landed. Rate analytics are windowed (7d rolling), so they age out naturally.
- **Does NOT fix the deeper class** of "an archived node still has embeddings + tags that could be matched by other paths." The vector-index paths (matchConstraintCodeByEmbedding, fetchActionableCandidates) already filter `is_archived`, so archived nodes cannot enter guidance via those routes. The keyword-fallback path was the leak; that's closed now.
- **Does NOT change follow-rate calculation semantics.** The observable metric moves only insofar as future outcomes stop landing with archived codes.

## Expected impact

Follow-rate analytics on days that historically had archived-content matches will show honest rates going forward. Not expected to be a "step change" (most days the fallback matcher's noise is bounded); Aug-8-shaped days that concentrate on narrow tactical work will benefit most.

## Documents Accessed

- Investigation conversation trace (this session, 2026-08-10)
- `docs/development/jiminy-follow-rate-remeasure-001/{verdict,post}.md`
- `internal/jiminy/{service,persistence,stats,confidence_updater}.go`
- Live SQL against `constraint_outcomes` + Live Cypher against `MemoryNode` on mdemg-dev
- `CHANGELOG.md` recent entries (JIMINY-*, HITL-*)
- CLAUDE.md HITL / Jiminy architecture-notes section
