# FT-DORMANT-CLEANUP-001 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #1. Q4
follow-up #6.

## 1. Header & Metadata

DROP the two dormant FT tables (`ft_benchmarks`, `ft_hitl_decisions`)
that DORMANT-CENSUS-002 adjudicated as `DORMANT_TO_REMOVE`. Both are
verified 0-row live; both are superseded by shipped alternatives
(`benchmark_runs`+`benchmark_results` V0012 / `review_grades` V0028).
Migration + code + doc scrub. ~30-60 min effort. Schema bump 31→32.

## 2. Problem Statement

Migration `002_ft_schema.sql` (early FT schema) declared two tables
that were never wired to a writer:
- `ft_benchmarks` — superseded by `benchmark_runs` + `benchmark_results`
  (V0012) as the actual benchmark persistence surface
- `ft_hitl_decisions` — superseded by `review_grades` (V0028) as the
  HITL grade persistence surface

DORMANT-CENSUS-002's `tsdb_consumer_inventory.json` adjudicated both
as `DORMANT_TO_REMOVE`. This sprint drops them and scrubs the residual
code + script + doc references.

## 3. Scope

**In scope (single commit):**

- New migration `032_drop_dormant_ft_tables.sql`:
  `DROP TABLE IF EXISTS ft_benchmarks CASCADE; DROP TABLE IF EXISTS
  ft_hitl_decisions CASCADE; UPDATE tsdb_schema_meta SET value='32'`
- Bump `TSDBRequiredSchemaVersion` default 31 → 32 in
  `internal/config/config.go` (also update the field comment)
- Scrub references from:
  - `internal/cli/tsdb.go` — the `mdemg tsdb stats` listing
  - `scripts/tsdb_spot_check.sh` — the FT-tables presence check
  - `scripts/tsdb_data_review.py` — `EXPECTED_TABLES` +
    `section_f_ft_tables` SQL
  - `scripts/tests/test_tsdb_data_review.py` — sample data
- Update `docs/api/tsdb_consumer_inventory.json`: switch disposition
  `DORMANT_TO_REMOVE` → `REMOVED` (add drop-date note)
- Extend `scripts/verify_tsdb_consumers.py` disposition vocabulary to
  include `REMOVED` (documented in docstring + operator hint)
- Apply the migration to the live TSDB; verify server restart accepts
  the new schema version

**Out of scope:**

- Scrubbing the original `CREATE TABLE` from `002_ft_schema.sql` —
  migration files are historical; don't rewrite. The verifier still
  counts them as "declared" via that historical CREATE, and the
  inventory entries stay with `disposition=REMOVED`.

## 4. Method

1. Write the migration file + apply to live TSDB (validates that
   IF EXISTS handles both drop cases cleanly)
2. Bump config default schema version + comment
3. Scrub the 4 code/script files + inventory + verifier
4. Rebuild + restart server, verify `schema_version=32` in log
5. Run verifier + `go test ./internal/tsdb/` + `go build` + lint
6. Docs (post, CHANGELOG, CLAUDE.md pin) + commit

## 5. Testing Plan

- **Tier 1 (unit)**: `go test ./internal/config/` + `./internal/tsdb/`
  green (existing tests cover schema version and migration runner)
- **Tier 2 (integration)**: verifier `scripts/verify_tsdb_consumers.py`
  returns "OK — no drift"
- **Tier 3 (live)**:
  - `docker exec … psql -c "SELECT to_regclass('ft_benchmarks')` returns
    NULL post-migration
  - Server restart log shows `schema_version=32`
  - `mdemg tsdb stats` runs clean without errors on the dropped tables

## 6. Commit Strategy

Single commit under `FT-DORMANT-CLEANUP-001`.

## 7. Verification Checklist

- [x] Migration file written + applied live
- [x] Config default schema version bumped 31→32
- [x] `internal/cli/tsdb.go` scrubbed (long help + table list)
- [x] `scripts/tsdb_spot_check.sh` scrubbed
- [x] `scripts/tsdb_data_review.py` scrubbed (EXPECTED_TABLES + query)
- [x] `scripts/tests/test_tsdb_data_review.py` sample data updated
- [x] `docs/api/tsdb_consumer_inventory.json` disposition → REMOVED
- [x] `scripts/verify_tsdb_consumers.py` REMOVED disposition added
- [x] Live migration applied + verified
- [x] Server accepts schema_version=32
- [x] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

Recreate the two tables from `002_ft_schema.sql`; UPDATE
`tsdb_schema_meta SET value='31'`. Both writers were empty so there is
no data-restore concern.

## 9. Risks

- **Risk**: an external tool references these tables. Grep confirmed
  only the 4 code/script files above; all scrubbed. No prod writer
  exists.
- **Risk**: rollback needed. `CREATE TABLE` reference preserved in
  `002_ft_schema.sql` — historical migration file untouched. Restoring
  = re-executing the CREATE from 002 + rolling `tsdb_schema_meta` back
  to 31.

## 10. Documents Accessed

Filled in `post.md`.
