# BACKUP-RESTORE-VERIFY-001 — Tier 3 Live Verification

Date: 2026-06-11 · Branch: `reh3376_dev01` · Live stack: native `bin/mdemg`
(LaunchAgent) + Docker Neo4j/TimescaleDB · Drill space: `backup-rt-test`

## Setup

- Rebuilt binary, `launchctl kickstart -k`; evaluator loaded **20 rules**
  (was 19) — `neo4j_backup_no_recent_success` registered, gated on
  `BACKUP_ENABLED=true`.
- Populated `backup-rt-test` with 50 paced observe calls → **56 nodes**.
  Two incidental live findings while populating:
  1. 60 rapid-fire observes → most rejected (rate limiter) — expected.
  2. Near-identical drill payloads semantically deduplicated onto a single
     node (same `node_id` returned for similar content) — correct system
     behavior; drill switched to 50 genuinely distinct topics.
  3. Every observe spawned an RSIC micro cycle for the scratch space —
     live re-confirmation of the RSIC-STORM-001 trigger race (admission
     state is recorded only after cycle completion).

## Round-trip — three drill runs, two production-blocking defects caught

(Note: partial backups always bundle the protected `mdemg-dev`, so the
scratch-space drill pays whole-database export cost — ~13 min / ~3 GB per
backup. Disclosed limitation.)

### Drill 1 → caught: retention deletes every backup it just made

Backup `bk-20260611-155917` completed (3.11 GB, checksummed) and was
deleted by retention **80 ms later** (`freed_bytes` = exactly the new
backup + manifest). Root cause: `BACKUP_RETENTION_MAX_STORAGE_GB`
comment/code drift (documented 50, code default 2) — below one export, so
RunAfter retention removed each backup at completion. **The backup system
was a no-op for this database.** Fixed (`61d9513`): quota retention never
deletes the newest backup per type (sparse-file pin tests incl. the
single-oversize-backup shape), default 50 GB, and the job-wait timeout
300s→3600s (the live export outruns 5 min — the initial scheduled run
honestly reported failure to jobhealth, proving that wiring, but the
timeout was wrong).

### Drill 2 → caught: restores with observations always failed

- Manifest carried checksum + the new file counts:
  `file_node_count=61644, file_edge_count=299398,
  file_observation_count=128901`, spaces `[backup-rt-test, mdemg-dev]`.
- **Corruption drill PASSED (fail-closed)**: flipped one byte mid-file →
  restore failed with `backup integrity check failed: file checksum
  sha256:09fc86… does not match manifest sha256:a13756… — refusing to
  import`. No graph writes occurred.
- **Real restore FAILED — live-caught defect #2**:
  `ConstraintValidationFailed (space_id='backup-rt-test', path='')`.
  Observations carry `path=NULL` (ignored by `memorynode_path_unique`),
  but the exporter serializes NULL as proto-default `""` and the importer
  wrote it unconditionally — the second observation node in ANY restore
  collided. Every observation-bearing restore had always been broken;
  invisible precisely because no backup had ever been restore-tested.
  Fixed (`95dddab`): `nodeProps` omits empty path/name; unit-pinned.

### Drill 3 (post-fix) → round-trip PASSED

- Artifacts survived retention (3.05 GB + manifest intact); independent
  `shasum -a 256` matched the manifest (pristine).
- `mdemg space delete backup-rt-test` → 0 nodes.
- Restore completed with the full validation block:
  `checksum_verified=true; file_nodes=61644 = nodes_processed=61644
  (50 created + 61594 skipped); file_edges=299398 = edges_processed;
  observations=128901`.
- **All 50 drill notes back** (`content CONTAINS 'Drill2 note'` → 50).
- `mdemg-dev` integrity: every pre-existing node skipped
  (`nodes_created=50`, all in the scratch space). The mdemg-dev count
  moved 77,446 → 79,392 across the ~25-min window from ORGANIC live
  activity (hooks + RSIC cycles — this session is unusually busy), not
  the restore: skip accounting proves the restore created exactly 50.

### Jobhealth + staleness rule

- Failure path proven live: `neo4j-backup | success=f | 300s | "snapshot
  did not complete within 5m0s"` row + high alert (the drill-caught
  timeout defect, honestly reported by the new wiring).
- `neo4j_backup_no_recent_success` (Service
  `scheduled-job-staleness-neo4j`) registered — evaluator loads 20 rules —
  and fired while zero successful runs existed (correct "never ran"
  semantics); the success row from the initial-on-start backup silences it
  (watcher armed; see post.md for the landed row).

## Documents Accessed

- `docs/development/backup-restore-verify-001/sprint_plan_backup_restore_verify_001.md`
- `internal/backup/{full,partial,scheduler,types}.go`, `internal/alert/rules.go`,
  `internal/api/{server,handlers_backup,handlers_jiminy}.go`, `internal/cli/serve.go`
- Live Neo4j (`backup-rt-test`, `mdemg-dev`), `./backups/` artifacts,
  `~/.mdemg/logs/server.log`, TSDB `scheduled_job_events`
