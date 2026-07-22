# Neo4j Backup & Verified Restore

## Why

MDEMG's graph is the memory substrate — losing it is losing the system's
mind. Before BACKUP-RESTORE-VERIFY-001 the backup chain had a silent
failure at every link: the manifest checksum was written but never read
(a corrupted file restored silently), the pre-restore safety snapshot was
raced with a 2-second sleep, nobody compared restored counts to the
manifest, the legacy `.dump` path shelled out to bare `"docker"` (broken
under launchd's minimal PATH), and the default-ON scheduler had zero
jobhealth coverage — a backup that failed or never ran was invisible.
And no backup had ever been restore-tested.

## How it works

**Backup** (`internal/backup/`): the scheduler (supervised since
SUPERVISOR-002) runs full backups every `BACKUP_FULL_INTERVAL_HOURS` and
partial backups every `BACKUP_PARTIAL_INTERVAL_HOURS`, plus an **initial
partial backup `BACKUP_INITIAL_DELAY_MIN` (default 5) minutes after
start** so a fresh install is never backup-less for a day. Partial
backups always include the protected `mdemg-dev` space. Each backup
writes a `.mdemg` logical export plus a manifest carrying a SHA-256
checksum, whole-database counts (informational), and **file content
counts** (`file_node_count` / `file_edge_count` /
`file_observation_count`, counted from the exported chunks — these are
the restore-validation reference).

**Restore** (`POST /v1/backup/restore`, async job):

1. **Safety snapshot** (`snapshot_before: true`): a keep-forever full
   backup, and the restore now *waits for that job to complete* —
   failing closed on snapshot failure or timeout
   (`BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC`, default 3600).
2. **Checksum gate**: the file's SHA-256 must match the manifest before
   any import. Legacy manifests without a checksum warn and proceed.
3. **Completeness check**: the file's actual chunk counts must match the
   manifest's `file_*` counts (hard fail = truncated/corrupt file).
   Legacy manifests without file counts downgrade to warn-only.
4. **Import** under `CONFLICT_SKIP` (existing nodes are never
   overwritten — restore into a live graph is additive).
5. **Validation block** in the job result: file counts, processed
   counts, created/skipped/merged breakdowns. Importer-accounting
   divergence is warn-level (legitimate under skip semantics);
   read-truncation is a hard failure.

**Legacy `.dump` restore** routes docker through `internal/dockerbin`
(`MDEMG_DOCKER_BIN` → PATH → well-known locations) unless the operator
set a non-default `BACKUP_FULL_CMD`. No checksum/count validation exists
for this path — it logs a warning saying so.

**Jobhealth**: every scheduled run (and the initial run) reports
`job_name='neo4j-backup'` to the V0024 `scheduled_job_events` hypertable
via `jobhealth.Report` — failure fires a high-severity alert. The
scheduler *waits* on each triggered job before reporting (the trigger is
queue-async; fire-and-forget would always claim success). The evaluator
rule `neo4j_backup_no_recent_success` (Service
`scheduled-job-staleness-neo4j`) fires when no successful run exists in
the window — the "job never ran" guarantee, generalized from
NOSILENT-001's tsdb-backup rule via a shared factory.

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `BACKUP_ENABLED` | true | Neo4j backup subsystem |
| `BACKUP_INITIAL_DELAY_MIN` | 5 | initial partial backup after start (0 = off) |
| `BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC` | 3600 | max wait for triggered backup jobs (safety snapshot + scheduled-run reporting) |
| `BACKUP_JOB_STALENESS_HOURS` | 0 | staleness alert window; 0 = partial interval × 2 |

## How to use

- Trigger: `mdemg db backup trigger [--type full|partial_space]` or
  `POST /v1/backup/trigger`.
- Status/manifest: `GET /v1/backup/status/<id>`,
  `GET /v1/backup/manifest/<id>`, `mdemg db backup list`.
- Restore: `POST /v1/backup/restore` `{"backup_id":"…",
  "snapshot_before":true}` → poll `GET /v1/backup/restore/status/<id>`;
  inspect the `validation` block in the result.
- Watch `~/.mdemg/alerts/current.json` for
  `scheduled-job-staleness-neo4j` / `scheduled-job-failure` entries.

## Known limitations

- `partial_space` always bundles `mdemg-dev`, so even a tiny-space backup
  pays whole-protected-space cost (candidate follow-up: an explicit
  exclude flag for test tooling).
- The `.dump` legacy path has no integrity validation (disclosed at
  restore time in the log).
- Restore is API-only (no `mdemg db backup restore` CLI yet).

Sprint: `docs/development/backup-restore-verify-001/` (plan, recon,
live round-trip verification).
