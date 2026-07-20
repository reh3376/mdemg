# Sprint CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001 — `applied_node_id` on contradicted_correction_drafts

## 1. Header & Metadata
- **Sprint ID:** CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001
- **Sprint line:** `docs/development/contradicted-bridge-applied-node-id-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.8 (patch — additive V0031 column)
- **Estimated effort:** ~0.3 dev-day, 5 sequential epics
- **OpenAI spend:** $0
- **Risk level:** Low — additive schema + additive column write; existing `applied_obs_id` untouched

## 2. Problem Statement
JIMINY-CONTRADICTED-BRIDGE-001 E3's sink stores `resp.ObsID` in `applied_obs_id` on approve. `conversation.Service.Correct` returns `*ObserveResponse` with BOTH `ObsID` and `NodeID` fields, and they can differ (verified live in PR #508 E5 evidence — draft `c8jvg…` recorded `applied_obs_id=oqqi977em6x29w8tx6n521j0` but the actual Neo4j node was `po2zahas8mh10ahwe0iimmoz`).

For forensic queries the operator can grep either field — but the graph-side lookup uses `node_id`. Missing the direct join key means every downstream trace ("what L1 correction was minted from this bridge-approved draft?") has to resolve `obs_id` → `node_id` via a secondary lookup that could stale-out over consolidation cycles.

Additive fix: persist `applied_node_id` too.

## 3. Scope & Constraints

### In scope
- V0031 migration `internal/tsdb/migrations/031_contradicted_drafts_applied_node_id.sql`:
  - `ALTER TABLE contradicted_correction_drafts ADD COLUMN IF NOT EXISTS applied_node_id TEXT`.
  - Bump `tsdb_schema_meta.schema_version` 30 → 31.
  - No index (low-volume table; grep on `applied_obs_id` still works).
- `internal/tsdb/contradicted_drafts_writer.go`:
  - `ContradictedDraftRow` gains `AppliedNodeID string`.
  - `Flush` includes it in the CopyFrom column set (nullable via `nullableString`).
  - `FetchPendingBySpace` + `FetchByID` return it in the SELECT.
  - `MarkApproved` signature gains `appliedNodeID string` param; UPDATE sets both `applied_obs_id` and `applied_node_id`.
- `internal/api/contradicted_drafts_dataset.go`:
  - Sink `Apply` on approve passes `resp.NodeID` to `MarkApproved` alongside `resp.ObsID`; `reinforcement_detail.Applied` map carries both keys.
  - Dataset `contradictedDraftItem` surfaces `applied_node_id` in `ReviewItem.Meta`.
- `internal/config/config.go` schema version default 30 → 31.
- Tier-1 tests: sink passes both fields to a mock writer that captures them.
- Live Tier-3: schema auto-applies; fresh approve populates both columns.
- Canonical docs.

### Out of scope
- Backfilling historical `applied_obs_id` rows with derived `applied_node_id` (deferrable follow-up CLI if operator wants forensic completeness on the 1 pre-sprint approved draft).
- Renaming `applied_obs_id` (would break existing consumers; additive column is preferred).
- Any change to the sink's Reverse behavior (documented as "L0 obs stays; not a substrate rollback" — unchanged).

### Constraints
- Sequential epics.
- Live Tier-3 required.
- Additive-only schema (no drops).
- CI schema-version validator must pass on the version bump.
- No hardcoded literals beyond column names.

## 4. Dependencies
- **JIMINY-CONTRADICTED-BRIDGE-001** (merged) — provides the sink + writer + V0030 schema this sprint extends.
- No new env vars.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document.

### Epic 1 — V0031 migration + writer
- Write migration file with `ADD COLUMN IF NOT EXISTS`.
- Bump `TSDBRequiredSchemaVersion` default in `config.go` from 30 to 31; update struct comment.
- Writer struct + column list + read scans + `MarkApproved` sig.

### Epic 2 — Sink populates both
- `contradicted_drafts_dataset.go::contradictedDraftsSink.Apply`:
  - Existing: `d.Applied["node_id"] = resp.NodeID` — that map entry is CORRECTLY named "node_id" already, but `MarkApproved(ctx, g.ItemID, obsID)` passes only obsID. Change `MarkApproved` call site to pass both.
  - `contradictedDraftItem` gains `"applied_node_id": r.AppliedNodeID` in Meta.

### Epic 3 — Tier-1 tests
- Extend `contradicted_drafts_dataset_test.go` with a mock writer that captures both args to `MarkApproved`; verify sink passes both correctly on approve.
- Extend the existing `TestContradictedDraftsSink_Apply_ApproveSurfacesCorrectError` for the passthrough shape.

### Epic 4 — Live Tier-3
1. Rebuild + restart mdemg (`launchctl kickstart -k`).
2. Verify schema V0031 auto-applied: `SELECT value FROM tsdb_schema_meta WHERE key='schema_version'` returns `31`; `\d contradicted_correction_drafts` shows the new column.
3. Trigger a fresh contradicted verdict → draft → approve cycle (mirror JIMINY-CONTRADICTED-BRIDGE-001 E5 sequence).
4. Verify: `SELECT applied_obs_id, applied_node_id FROM contradicted_correction_drafts WHERE id='<new draft id>'` returns BOTH non-empty.
5. Verify `applied_node_id` matches the actual Neo4j `n.node_id` for the L0 obs.
6. Capture in `live_verification.md`.

### Epic 5 — Canonical docs
- CLAUDE.md architecture note (small — "applied_node_id lands alongside applied_obs_id; forensic queries can join graph-side directly").
- CHANGELOG `[Unreleased] > Added` entry.
- `docs/features/hitl-review.md` follow-up section update.
- `post.md`.

## 6. Testing (3 tiers)
- **Tier 1** — sink mock-writer capture pin.
- **Tier 2** — existing rerun (no regression on JIMINY-CONTRADICTED-BRIDGE-001 tests).
- **Tier 3** — schema auto-apply + fresh approve → both columns populated.

## 7. Commit Strategy
5 sequential commits on `reh3376_dev01`:
1. `docs(contradicted-bridge-applied-node-id-001): E0 — sprint plan`
2. `feat(contradicted-bridge-applied-node-id-001): E1 — V0031 applied_node_id column + writer`
3. `feat(contradicted-bridge-applied-node-id-001): E2 — sink populates applied_node_id on approve`
4. `test(contradicted-bridge-applied-node-id-001): E3 — sink mock-writer capture pin`
5. `docs(contradicted-bridge-applied-node-id-001): E4+E5 — live Tier-3 + CLAUDE.md/CHANGELOG/post`

Auto-PR fires. Sprint summary comment after E5.

## 8. Verification Checklist
- [ ] E0 committed
- [ ] V0031 migration + `ADD COLUMN IF NOT EXISTS`
- [ ] Schema version bumped 30→31 + CI validator passes
- [ ] Writer's ContradictedDraftRow gains AppliedNodeID
- [ ] Flush includes it in CopyFrom
- [ ] FetchPendingBySpace + FetchByID return it
- [ ] MarkApproved signature updated
- [ ] Sink Apply passes resp.NodeID
- [ ] Dataset Meta surfaces applied_node_id
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] Live: schema V0031 applied on startup
- [ ] Live: fresh approve populates both applied_obs_id + applied_node_id
- [ ] CLAUDE.md note
- [ ] CHANGELOG entry
- [ ] post.md

## 9. Documentation Update — Epic 5.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Additive migration fails on a live DB | Very Low | Low | `ADD COLUMN IF NOT EXISTS`; runs in transaction; no data touched |
| MarkApproved sig change breaks a caller | Very Low | Low | Sink is the sole caller (grep-verified); test-suite catches |
| Historical rows have NULL applied_node_id | Certain | Low | Deliberate — additive, no backfill this sprint; documented |
| ObserveResponse.NodeID could be empty in some cases | Low | Low | Sink stores whatever is returned; if empty, applied_node_id is empty string (matches applied_obs_id's semantics) |

## 11. Documents Accessed
- `internal/tsdb/contradicted_drafts_writer.go` (writer + row struct + all SQL)
- `internal/tsdb/migrations/030_contradicted_correction_drafts.sql` (migration pattern to mirror)
- `internal/api/contradicted_drafts_dataset.go` (sink Apply + dataset Meta)
- `internal/api/contradicted_drafts_dataset_test.go` (existing test file to extend)
- `internal/config/config.go::TSDBRequiredSchemaVersion` (bump target)
- Live: current schema_version=30, `\d` output confirms `applied_obs_id` present + `applied_node_id` absent

## 12. Rollback Procedures
- Additive column; a rolled-back binary ignores it (extra column has no effect).
- If the column write ever caused a real regression: revert code; the column stays; future writes just fill it again on the next re-flip.
- Schema version can be rolled back manually via `UPDATE tsdb_schema_meta SET value='30'`; the column stays (schemaless-friendly rollback).

## Acceptance Criteria
1. `applied_node_id` column exists on `contradicted_correction_drafts` after startup.
2. Fresh approve populates BOTH `applied_obs_id` (existing) and `applied_node_id` (new).
3. `applied_node_id` matches the actual Neo4j `n.node_id` for the L0 correction obs.
4. Full test suite green; lint clean.
5. Canonical docs updated.
