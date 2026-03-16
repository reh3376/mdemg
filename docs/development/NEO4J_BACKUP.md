# Neo4j Backup & Restore (Phase 70)

MDEMG stores irreplaceable agent memory — decisions, corrections, institutional knowledge. This module provides automated and on-demand backup with retention policies.

## Overview

Two backup modes are available:

| Mode | Method | Output | Use Case |
|------|--------|--------|----------|
| **Full** | Logical export of all spaces via `transfer.Exporter` | `.mdemg` file | Complete database backup, disaster recovery |
| **Partial (space-level)** | `transfer.Exporter.Export()` per space | `.mdemg` file | Incremental space exports, selective restore |

Both modes produce a `.manifest.json` sidecar with checksum, node/edge counts, and metadata.

## Quick Start

### Enable Backup

Set the environment variable before starting the server:

```bash
BACKUP_ENABLED=true BACKUP_STORAGE_DIR=./backups ./mdemg-server
```

Or add to your `.env` file:

```bash
BACKUP_ENABLED=true
BACKUP_STORAGE_DIR=./backups
```

### Trigger a Manual Backup

**Partial backup (specific spaces):**

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","space_ids":["mdemg-dev"],"label":"manual-backup"}'
```

**Partial backup (all spaces):**

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","label":"all-spaces-backup"}'
```

When `space_ids` is omitted, all spaces are exported. The protected `mdemg-dev` space is always included regardless.

**Full database backup (all spaces):**

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"full","label":"full-backup"}'
```

Full backups export all spaces using the same logical export pipeline as partial backups. This works with a live database — no downtime required.

**Mark a backup as permanent:**

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","keep_forever":true,"label":"milestone-v1.0"}'
```

Backups with `keep_forever: true` are exempt from all retention cleanup.

### Check Backup Status

Backups run asynchronously. Poll the status endpoint:

```bash
curl -s http://localhost:9999/v1/backup/status/<backup_id>
```

Response includes `status` (pending/running/completed/failed), progress, checksum, and file size.

### List All Backups

```bash
# All backups
curl -s http://localhost:9999/v1/backup/list

# Filter by type
curl -s "http://localhost:9999/v1/backup/list?type=full"
curl -s "http://localhost:9999/v1/backup/list?type=partial_space"
```

### View Manifest

```bash
curl -s http://localhost:9999/v1/backup/manifest/<backup_id>
```

Returns node count, edge count, spaces included, schema version, checksum, and size.

### Delete a Backup

```bash
curl -s -X DELETE http://localhost:9999/v1/backup/<backup_id>
```

Backups marked `keep_forever` cannot be deleted until that flag is removed.

### Restore from Backup

```bash
# Restore (works with both full and partial backups)
curl -s -X POST http://localhost:9999/v1/backup/restore \
  -H "Content-Type: application/json" \
  -d '{"backup_id":"<backup_id>","snapshot_before":true}'

# Check restore status
curl -s http://localhost:9999/v1/backup/restore/status/<restore_id>
```

Setting `snapshot_before: true` takes a safety snapshot of the current database before restoring. Restore auto-detects file format: `.mdemg` (logical import via `transfer.Importer`) or legacy `.dump` (physical restore via `neo4j-admin`).

## Configuration Reference

All settings use environment variables with sensible defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_ENABLED` | `false` | Enable the backup module |
| `BACKUP_STORAGE_DIR` | `./backups` | Directory for backup artifacts |
| `BACKUP_FULL_CMD` | `docker` | Command used for full backups |
| `BACKUP_NEO4J_CONTAINER` | `mdemg-neo4j` | Docker container name for Neo4j |
| `BACKUP_FULL_INTERVAL_HOURS` | `168` | Hours between automatic full backups (168 = weekly) |
| `BACKUP_PARTIAL_INTERVAL_HOURS` | `24` | Hours between automatic partial backups (24 = daily) |
| `BACKUP_RETENTION_FULL_COUNT` | `4` | Keep the last N full backups |
| `BACKUP_RETENTION_PARTIAL_COUNT` | `14` | Keep the last N partial backups |
| `BACKUP_RETENTION_MAX_AGE_DAYS` | `90` | Delete backups older than N days |
| `BACKUP_RETENTION_MAX_STORAGE_GB` | `50` | Delete oldest backups when storage exceeds N GB |
| `BACKUP_RETENTION_RUN_AFTER_BACKUP` | `true` | Run retention cleanup after each backup completes |

### Changing Backup Frequency

To change how often automatic backups run, set the interval variables:

```bash
# Full backup every 3 days, partial every 6 hours
BACKUP_FULL_INTERVAL_HOURS=72
BACKUP_PARTIAL_INTERVAL_HOURS=6
```

The scheduler uses simple tickers — the first automatic backup fires after one interval elapses from server start.

### Tuning Retention

Retention runs automatically after each backup (when `BACKUP_RETENTION_RUN_AFTER_BACKUP=true`). Three policies apply in order:

1. **Count-based**: Keep the newest N full and N partial backups. Older ones are marked for deletion.
2. **Age-based**: Any backup older than `BACKUP_RETENTION_MAX_AGE_DAYS` is marked for deletion.
3. **Storage-based**: If total storage exceeds the quota, the oldest non-exempt backups are deleted until under the limit.

Backups with `keep_forever: true` are exempt from all three policies.

Example — aggressive retention for limited disk:

```bash
BACKUP_RETENTION_FULL_COUNT=2
BACKUP_RETENTION_PARTIAL_COUNT=7
BACKUP_RETENTION_MAX_AGE_DAYS=30
BACKUP_RETENTION_MAX_STORAGE_GB=10
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/backup/trigger` | Trigger a new backup |
| GET | `/v1/backup/status/{id}` | Get backup job status |
| GET | `/v1/backup/list` | List all backups (optional `?type=` filter) |
| GET | `/v1/backup/manifest/{id}` | Get full manifest for a backup |
| DELETE | `/v1/backup/{id}` | Delete a backup |
| POST | `/v1/backup/restore` | Trigger a restore from any backup (.mdemg or legacy .dump) |
| GET | `/v1/backup/restore/status/{id}` | Get restore job status |

All endpoints return 503 when `BACKUP_ENABLED=false`.

## File Layout

```
backups/
  bk-20260208-030000-full.mdemg             # Full backup (all spaces)
  bk-20260208-030000-full.manifest.json     # Manifest sidecar
  bk-20260208-060000-partial_space.mdemg    # Partial space export
  bk-20260208-060000-partial_space.manifest.json
```

The `backups/` directory is gitignored.

## Architecture

```
internal/backup/
  types.go       — Config, BackupRecord, BackupManifest, request/response types
  service.go     — Core orchestrator: trigger, list, get, delete, manifest I/O
  full.go        — Full backup (delegates to partial for logical export), restore logic
  partial.go     — Space-level export via transfer.Exporter
  retention.go   — Count/age/storage-based cleanup engine
  scheduler.go   — Ticker-based automatic backup scheduler

internal/api/
  handlers_backup.go — 7 HTTP handlers

migrations/
  V0013__backup_metadata.cypher — BackupMeta constraint + index
```

## Troubleshooting

**Full backup fails**: Check MDEMG server logs. Full backups use the same logical export pipeline as partial backups and work with a live database. Legacy `.dump` files from older versions are still supported for restore.

**Partial backup is slow**: Large spaces with many nodes produce large `.mdemg` files. Consider backing up specific spaces instead of all.

**Retention not cleaning up**: Check that `BACKUP_RETENTION_RUN_AFTER_BACKUP=true` and that backups aren't marked `keep_forever`.

**503 on all endpoints**: Set `BACKUP_ENABLED=true` in your environment.
