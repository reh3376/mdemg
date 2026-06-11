# BACKUP-RESTORE-VERIFY-001 — Sprint Post

Closed: 2026-06-11 · Branch: `reh3376_dev01` · Roadmap: Q3 Phase 2 #2.

## The sprint's premise, demonstrated by the sprint itself

The roadmap rated this work non-deferrable because "a memory substrate
whose backups have never been restore-tested is one corruption away from
permanent memory loss." The mandatory live round-trip then proved it
twice over — **two production-blocking defects had been sitting in the
backup chain all along, invisible because nothing ever restored**:

1. **Retention deleted every backup ~80 ms after completion**
   (`61d9513`): the 2 GB default storage quota (comment/code drift vs the
   documented 50) was smaller than one whole-database export, and
   RunAfter retention removed each new backup immediately. Quota
   retention now never deletes the newest backup per type.
2. **Every observation-bearing restore failed** (`95dddab`):
   NULL→`""` path coercion in export/import collided with
   `memorynode_path_unique` on the second observation node.

## Shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan + recon | `3732210` |
| 1–4 | Checksum gate, snapshot polling, file-count validation, dockerbin routing | `85af35d` |
| 5 | neo4j-backup jobhealth + `jobStalenessRule` factory + `neo4j_backup_no_recent_success` | `cc570e5` |
| 5b | Initial backup on start (`BACKUP_INITIAL_DELAY_MIN`) | `516b7f7` |
| fix | Retention floor + quota 50 GB + wait timeout 3600s (drill-caught) | `61d9513` |
| fix | Import omits empty path/name (drill-caught) | `95dddab` |
| 6 | Tier 3: 3 drill runs — corruption-reject, full round-trip, jobhealth | `verification.md` |
| 7 | Feature doc + CHANGELOG + CLAUDE.md + post | (docs commit) |

## Tier 3 result (drill 3)

Backup 3.05 GB → corrupt → **refused** (checksum mismatch, no graph
writes) → pristine → space deleted → restore **completed**:
`checksum_verified=true`, file counts = processed counts exactly
(61,644 nodes = 50 created + 61,594 skipped; 299,398 edges), all 50
drill notes recovered, `mdemg-dev` pre-existing nodes 100% skipped.

## Jobhealth evidence

The failure path fired live before the success path existed: the initial
scheduled run honestly recorded `success=f, "snapshot did not complete
within 5m0s"` (the old 300s timeout — itself a drill-caught defect) to
`scheduled_job_events` + a high alert. The staleness rule fired during
the zero-success window (correct "never ran" semantics) and goes silent
once the initial-on-start backup's success row lands.

## New config

`BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC` (3600) · `BACKUP_INITIAL_DELAY_MIN`
(5) · `BACKUP_JOB_STALENESS_HOURS` (0 = partial interval × 2) ·
`BACKUP_RETENTION_MAX_STORAGE_GB` default corrected 2 → 50.

## Disclosed limitations / follow-ups

- Partial backups always bundle `mdemg-dev` — scratch-space testing pays
  whole-database cost (candidate: exclude flag for test tooling).
- `.dump` legacy restores have no integrity validation (warned at run).
- Restore remains API-only (no CLI command) — UX follow-up.
- The drill re-confirmed the RSIC micro-cycle trigger race live
  (every scratch-space observe spawned a cycle) — RSIC-STORM-001
  proposal stands as the next sprint candidate.
- Plan deviation disclosed: Epics 1–4 landed as one commit (shared
  restore-path signatures made per-epic commits artificial).

## Documents Accessed

See `sprint_plan_backup_restore_verify_001.md` §11 + `verification.md`.
