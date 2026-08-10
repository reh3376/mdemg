# JIMINY-ARCHIVED-CODE-FILTER-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-ARCHIVED-CODE-FILTER-001
- **Date:** 2026-08-10
- **Branch:** `reh3376_dev01`
- **Author:** Roger Henley (via Claude Opus 4.7)
- **Parent trigger:** Operator report "Jiminy guidance follow rate is trending downward and it needs to be investigated" (2026-08-10 session). Investigation revealed archived constraint nodes were leaking into the fallback keyword-matcher's pool.
- **Effort:** ~1h (hot-fix + pin test + docs).

## 2. Problem Statement

`loadSpaceConstraintCodes` in `internal/jiminy/service.go` loaded ALL constraint codes for a space without filtering `is_archived`. When the primary vector-index matcher (`matchConstraintCodeByEmbedding`, which correctly filtered archived nodes) returned no match, the fallback keyword matcher (`matchConstraintCode`) ran against this unfiltered pool and could assign an ARCHIVED constraint's code to a fresh guidance item.

Downstream effect: `constraint_outcomes` writer records the outcome tagged with the archived code. Follow-rate analytics see it as a real constraint outcome, drag the daily rate against codes that have zero live nodes.

**Live-caught 2026-08-08 on mdemg-dev**: constraint_code `auto-9f5134a1a0c3` had 4 archived + 1 archived nodes, ZERO live nodes, still producing 17 outcomes/day (all `ignored` — the archived constraint's semantics didn't match the agent's actions, but the row landed and dragged the rate).

## 3. Scope & Constraints

- **In-scope:** Filter `is_archived` in every Cypher block that MATCHes constraint MemoryNodes for READ purposes. Add a pin test that walks the source files and asserts the filter is present. Docs.
- **Out-of-scope:** Refactoring the code-assignment pipeline; adding an analytics-side filter on `constraint_outcomes`; retroactive cleanup of already-recorded archived-code outcomes (historical noise, not a bug going forward).
- **Constraint:** BOTH readers AND setters can appear the same file. Setters need the archive filter for a DIFFERENT reason (they set is_archived=true) — kept out of scope for the READ filter but allowlisted in the pin test.

## 4. Dependencies

- No new schema.
- Neo4j must have `is_archived` property on all constraint nodes (may be null on legacy nodes; `coalesce(*, false)` handles that).
- Prerequisites shipped: JIMINY-CORPUS-001 (archive_reason property), JIMINY-CORPUS-002 (tombstoning workflow), CORRECTION-CODE-GEN-001 (correction nodes carry codes).

## 5. Implementation Plan (single-epic hot-fix)

Sequential steps (no parallelism):

1. **Audit** — grep every Cypher block in `internal/jiminy/{service,persistence,stats,confidence_updater}.go` that MATCHes constraint nodes. Result: 8 hits.
2. **Classify** — 3 sites already had the filter (matchConstraintCodeByEmbedding, fetchActionableCandidates, archiveNodesByCode). 5 sites did not (loadSpaceConstraintCodes, buildConstraintGlossary, findConstraintNodeID, PersistGuidanceOutcome MATCH src, FindConstraintCodeForNode, GetConstraintEffectiveness, computeWeightedEffectiveness).
3. **Fix** — add `AND NOT coalesce(*.is_archived, false)` to every unfiltered READ block. For `loadSpaceConstraintCodes` also tighten role_type filter to `IN ['constraint','correction']` (previously matched any node with a constraint_code).
4. **Pin test** — `TestConstraintCodeCypher_AllPathsFilterArchived` walks the 4 files, splits on backticks to extract Cypher blocks, asserts every block targeting constraint MemoryNodes includes an archive-filter regex match. Allowlist for legitimate exceptions (archive setter; BootstrapCodes / BootstrapCorrectionCodes SETTERs).
5. **Live verify** — Neo4j stats query confirms 128 archived / 261 total code-bearing nodes on mdemg-dev; post-fix pool = 103 live+correct-role.
6. **Docs** — sprint_plan.md + post.md; CLAUDE.md arch-rule pin; CHANGELOG entry.

## 6. Testing Plan

**Tier 1 — unit/pin tests (colocated in `internal/jiminy/`):**
- `TestConstraintCodeCypher_AllPathsFilterArchived` — grep-shaped regression pin. Adding a new Cypher block that MATCHes constraint nodes without the archive filter → CI fail.

**Tier 2 — package integration:** `go test ./internal/jiminy/... -count=1` — no new integration tests; the pin test + existing test coverage validate the change.

**Tier 3 — live smoke on mdemg-dev:**
- Neo4j query pre/post: `MATCH (c:MemoryNode {space_id:'mdemg-dev'}) WHERE c.constraint_code IS NOT NULL AND c.constraint_code <> '' RETURN count(*) AS total, count(CASE WHEN NOT coalesce(c.is_archived, false) AND c.role_type IN ['constraint','correction'] THEN 1 END) AS live` → total=261, live=103 (128 archived correctly excluded).
- Follow-rate re-measurement is deferred; the shipped fix stops the forward-going bleed but historical constraint_outcomes rows with archived codes remain (they are honest reflections of past guidance surfaces at the moment they landed).

## 7. Commit Strategy

Single commit: `fix(jiminy): filter archived constraint nodes in ALL code-fetching Cypher paths`. Files: `internal/jiminy/{service,persistence,stats}.go` + `internal/jiminy/archived_code_filter_test.go` (new).

Docs land in a separate commit (this file + post.md + CLAUDE.md pin + CHANGELOG entry) to keep the code-fix commit focused.

## 8. Verification Checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/jiminy/` = 0 issues
- [x] `go test ./internal/jiminy/... -count=1` = all pass (incl. new pin)
- [x] Pin test catches added coverage — was RED before fix on all 5 sibling sites; ALLOWLIST for 3 SETTER-context blocks documented inline
- [x] Live Neo4j stats query on mdemg-dev confirms 128:103 archive:live split
- [x] CLAUDE.md arch rule pinned
- [x] CHANGELOG Unreleased entry
- [x] Sprint dir populated (this file + post.md)

## 9. Documentation Update

- `docs/development/jiminy-archived-code-filter-001/sprint_plan.md` — this file
- `docs/development/jiminy-archived-code-filter-001/post.md` — sprint post with live results
- `CLAUDE.md` — new arch rule pin (Architecture Notes section, near HITL / Jiminy pins)
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

**R1:** The role_type tightening in `loadSpaceConstraintCodes` (added `IN ['constraint','correction']` filter) could exclude legacy nodes that carry a constraint_code without a proper role_type.
*Mitigation:* The 3 vector-index paths already used this exact filter and worked correctly. The fallback matcher's pool is intended to be constraint/correction nodes only — anything else was noise. Empirical: on mdemg-dev the tighter filter reduces the pool from 261 → 103 (correct — 128 archived + ~30 misc with wrong role_type were legitimately excluded).

**R2:** Historical constraint_outcomes rows with archived codes remain. If analytics dashboards read those, the rate drag persists retroactively.
*Mitigation:* Rate analytics are windowed (7d rolling most panels); historical outcomes age out. The fix stops the forward-going bleed. Retroactive cleanup is a separate, larger operation not warranted for this class.

**R3:** Pin test's regex-based approach may false-positive or miss subtle variants of the archive filter.
*Mitigation:* Three acceptable filter patterns accepted (`NOT coalesce(*.is_archived, false)`, `is_archived IS NULL`, `is_archived = false`). If a future edit uses a fourth variant, extend the allowlist in the test — cleaner than under-strict.

## 11. Rollback Procedures

Revert the commit; no schema changes; no persistent state changes. Historical `constraint_outcomes` rows with archived codes are unaffected. Neo4j `is_archived` property continues to be authoritative.

## 12. Documents Accessed

- Live SQL against `constraint_outcomes` on mdemg-dev (7d window + hourly)
- Live Cypher against Neo4j `MemoryNode` role_type + is_archived
- `internal/jiminy/{service,persistence,stats,confidence_updater}.go`
- `CHANGELOG.md` (JIMINY-FOLLOW-RATE-REMEASURE-001, HITL-CURATION-003 recent entries)
- CLAUDE.md HITL/Jiminy sections
