# FT-DORMANT-CLEANUP-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #1. Q4
follow-up #6.

## Verdict

**Shipped.** V0032 dropped both dormant FT tables live, schema
version bumped 31→32, all 4 code/script references scrubbed,
inventory + verifier updated. Server restart confirmed
`schema_version=32`. Fully reversible via CREATE recovery from
`002_ft_schema.sql` + schema_version rollback.

## What shipped

- **`internal/tsdb/migrations/032_drop_dormant_ft_tables.sql`** —
  `DROP TABLE IF EXISTS ft_benchmarks CASCADE; DROP TABLE IF EXISTS
  ft_hitl_decisions CASCADE; UPDATE tsdb_schema_meta SET value='32'`
- **`internal/config/config.go`** — `TSDBRequiredSchemaVersion` default
  31 → 32 + field comment updated
- **`internal/cli/tsdb.go`** — `mdemg tsdb stats` long-help + tables
  list scrubbed (comment marks the V0032 supersession)
- **`scripts/tsdb_spot_check.sh`** — FT-tables presence check scrubbed
- **`scripts/tsdb_data_review.py`** — `EXPECTED_TABLES` +
  `section_f_ft_tables` SQL scrubbed
- **`scripts/tests/test_tsdb_data_review.py`** — sample data updated
- **`docs/api/tsdb_consumer_inventory.json`** — both entries
  `DORMANT_TO_REMOVE` → `REMOVED` (drop-date noted)
- **`scripts/verify_tsdb_consumers.py`** — `REMOVED` disposition
  added to the valid vocabulary + operator-hint

## Live verification

Migration applied:
```
$ docker exec -i mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics \
    < internal/tsdb/migrations/032_drop_dormant_ft_tables.sql
DROP TABLE
DROP TABLE
UPDATE 1

$ docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c "
    SELECT to_regclass('ft_benchmarks'),
           to_regclass('ft_hitl_decisions'),
           value FROM tsdb_schema_meta WHERE key='schema_version';"
 ft_benchmarks_exists | ft_hitl_decisions_exists | current_schema_version
----------------------+--------------------------+------------------------
                      |                          | 32
```

Server restart log:
```
level=INFO msg="tsdb: running migration" file=032_drop_dormant_ft_tables.sql
level=INFO msg="tsdb: migration complete"
level=INFO msg="TimescaleDB ready" schema_version=32
```
(The re-run on restart is idempotent — DROP IF EXISTS + UPDATE is a
no-op after the manual apply.)

Verifier:
```
$ python3 scripts/verify_tsdb_consumers.py
tables: 26 declared, 26 inventoried; OK — no drift
```

## Rollback

The migration file `002_ft_schema.sql` still contains the original
`CREATE TABLE` for both tables (historical integrity — migration files
are not rewritten). To restore:
```sql
-- 1. Re-execute the CREATE TABLE statements from 002_ft_schema.sql
-- 2. Roll back schema_version:
UPDATE tsdb_schema_meta SET value='31' WHERE key='schema_version';
```
Both writers were empty at drop-time so no data restoration is needed.

## Rules pinned

⚠️ **Do NOT rewrite historical migration files.** DROP-only migrations
add a new file (032 here); the original CREATE stays in 002 for schema
history + rollback source-of-truth. Consequence: the verifier's
declared-tables count includes both dropped tables (via 002's CREATE),
and the inventory retains entries with `disposition=REMOVED`. This is
the intended shape — no drift, historical clarity preserved.

⚠️ **When adding a `REMOVED` disposition to the inventory, extend the
verifier's valid-disposition vocabulary in the same PR.** Otherwise
the verifier's docstring/hint drifts from reality. This sprint added
`REMOVED` to the enum + operator-hint alongside the inventory update.

## Documents Accessed

- `docs/development/dormant-census-002/post.md` (parent — follow-up #1)
- `docs/development/ft-dormant-cleanup-001/sprint_plan.md` (this dir)
- `internal/tsdb/migrations/002_ft_schema.sql` (original definitions)
- `internal/tsdb/migrations/031_contradicted_drafts_applied_node_id.sql`
  (latest migration pattern reference)
- `internal/config/config.go` (schema version + config wire)
- `internal/cli/tsdb.go` (stats listing)
- `scripts/tsdb_spot_check.sh` (spot check)
- `scripts/tsdb_data_review.py` + `scripts/tests/test_tsdb_data_review.py`
- `docs/api/tsdb_consumer_inventory.json` (inventory)
- `scripts/verify_tsdb_consumers.py` (CI drift check)
- Live TSDB verification queries
