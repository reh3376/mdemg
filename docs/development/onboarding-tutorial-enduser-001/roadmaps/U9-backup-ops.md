# U9 — Backup + Ops (6 slides)

**Intent**: `/ui/ Backup` tab + scheduled backup/maintenance/restore + manual CLI. Operator-facing durable-safety surface — what runs when, what the retention policy is, how to restore.
**Target slides**: 6
**Mermaid**: 0

---

## U9.1 — Section Landing

**Title**: `U9 · Backup + Ops`

**Body**:
- Intent: "Backups + scheduled maintenance (decay + prune) are the operator's safety net. BACKUP-RESTORE-VERIFY-001 made the restore path provably correct; NOSILENT-001 ensures silent failures ALARM."
- Preview: schedulers → UI → CLI → retention → gotchas → where deeper

**Provenance**:
- CLAUDE.md "Backups provably restore (BACKUP-RESTORE-VERIFY-001)" + "NOSILENT-001"
- `docs/features/backup-restore.md`

---

## U9.2 — Scheduled Jobs — The Three

**Title**: `What Runs On Schedule`

**Body**:
- **TSDB backup scheduler** — periodic `pg_dump` of the TimescaleDB; schedule via `TSDB_BACKUP_SCHEDULE_*`; dockerbin resolver (`MDEMG_DOCKER_BIN` → PATH → well-known) handles the launchd minimal PATH that previously broke it silently
- **Neo4j backup** — scheduled via `NEO4J_BACKUP_*`; dumps to `backups/` with staleness guard rule `neo4j_backup_no_recent_success` fires HIGH if silent
- **Weekly maintenance** — LaunchAgent runs `mdemg maintenance` (combined decay + prune) with `--dry-run=false` on the SCHEDULE (CLI default stays `--dry-run=true` for safety); MAINT-LIVE-001 made this real
- Jobhealth: all 3 record every run to `scheduled_job_events` (V0024 hypertable) via `internal/jobhealth.Report`; failures fire HIGH on distinct Services per NOSILENT-001 cooldown-key contract

**Provenance**:
- CLAUDE.md "Scheduled-job health (NOSILENT-001)" + "Scheduled maintenance is LIVE (MAINT-LIVE-001)"

**note**:
- Operator cue: "Pending `scheduled-job-staleness-neo4j: No Successful Neo4j Backup In Window` HIGH alert on your session fires because the Neo4j backup hasn't succeeded recently. Check `/ui/ Backup` or `docker compose logs neo4j`."

---

## U9.3 — /ui/ Backup Tab

**Title**: `The Backup Tab`

**Body**:
- Where: `/ui/` → **Backup**
- Shows: recent backup history (success/failure per DB), scheduled next-run time, manifest-SHA verifications, retention counts
- Interaction:
  - Trigger a manual backup run
  - View manifest content (file counts, SHAs)
  - Delete expired backups past retention
- Companion CLI: `mdemg backup create/list/verify/restore`

**Provenance**:
- `internal/api/ui/tabs/backup.js`

**note**:
- Operator cue: "Backups are honest — a failed `scheduled_job_events` row MEANS the backup didn't happen. Don't paper over this."

---

## U9.4 — Backup/Restore CLI

**Title**: `Backup + Restore From The CLI`

**Body**:
- CLI:
  ```bash
  $ mdemg backup create --db neo4j --out /path/to/backups/
  $ mdemg backup list
  $ mdemg backup verify --manifest /path/to/manifest.json
  $ mdemg backup restore --manifest /path/to/manifest.json --db neo4j
  ```
- Restore invariants (BACKUP-RESTORE-VERIFY-001):
  - Manifest SHA-256 checksum gate BEFORE import
  - Pre-restore safety snapshot is POLLED to completion (was a 2s-sleep race pre-sprint)
  - File-completeness validation against new `file_*_count` manifest fields
  - Quota retention NEVER deletes the newest backup per type
- `transfer.nodeProps` fix: observations carry `path=NULL`; proto serializes NULL as `""`; writer OMITS empty path/name to avoid collision with `memorynode_path_unique`

**Provenance**:
- CLAUDE.md "Backups provably restore (BACKUP-RESTORE-VERIFY-001)"

**note**:
- Operator cue: "If restore fails SHA verification, the backup is corrupt — DO NOT force. Use an older verified backup."

---

## U9.5 — Maintenance (`mdemg maintenance`)

**Title**: `Weekly Maintenance`

**Body**:
- CLI (manual preview):
  ```bash
  $ mdemg maintenance --space-id mdemg-dev --dry-run
  decay: would reduce weight on 1234 CO_ACTIVATED_WITH edges
  prune: would archive 56 orphan nodes (role_type IN [...])
  ```
- CLI (live run):
  ```bash
  $ mdemg maintenance --space-id mdemg-dev --dry-run=false
  decay: reduced weight on 1234 edges
  prune: archived 56 nodes with reason='scheduled_maintenance'
  ```
- LaunchAgent schedules the live run weekly; MAINT-LIVE-001 made this ACTUALLY RUN (pre-sprint it was dry-run only — never executed in the project's history)
- Orphan disposition is CONTEXT-DEPENDENT: `--exclude-role-types` or `PRUNE_EXCLUDE_ROLE_TYPES` env; schedule excludes `constraint,conversation_observation`
- Alert `maintenance_no_live_run` fires on the only-ever-dry-runs pattern

**Provenance**:
- CLAUDE.md "Scheduled maintenance is LIVE (MAINT-LIVE-001)"

**note**:
- Operator cue: "`--dry-run=true` default for the CLI is the safety net. The SCHEDULE is what must not silently no-op."

---

## U9.6 — Gotchas + Where To Go Deeper

**Title**: `Backup + Ops — Gotchas + More`

**Body**:
- Gotchas:
  - Scheduled backup WAITS on the queue-async job and reports jobhealth `job_name='neo4j-backup'` — don't assume async-fire-forget success
  - Backup quota default is 2 GB (comment/code drift was deleting every 3 GB backup ~80 ms after completion; BACKUP-RESTORE-VERIFY-001 Tier 3 drill caught it)
  - Batched `CALL { … } IN TRANSACTIONS` Cypher runs via `session.Run` (implicit tx); INSIDE `ExecuteWrite` Neo4j raises TransactionStartFailed (MAINT-LIVE-001 live catch)
  - MAINT-LIVE-001 schedule excludes `constraint,conversation_observation` for prune — intentional; operator can override
- Feature docs:
  - `docs/features/backup-restore.md`
  - `docs/features/scheduled-maintenance.md`
  - `docs/features/scheduled-job-health.md`
- Where this feeds into: U13 Grafana (`mdemg-overview` + `mdemg-neo4j` panels), U16 Alerts (scheduled-job staleness + maintenance_no_live_run)

**note**:
- Operator cue: "Backup health is a leading indicator of DATA LOSS risk. If `scheduled-job-staleness-*` alerts fire, pause other work and triage."
- Transition: "U10 next — Logs — when stethoscope + alerts aren't enough and you need the raw details."
