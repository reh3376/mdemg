# Phase 70: Neo4j Backup & Restore (Full & Partial) with Scheduler

**Status:** ✅ Complete (2026-02-07)
**Priority:** High
**Dependencies:** Phase 34 (Incremental Sync/Delta Export)
**Guide:** [`docs/development/NEO4J_BACKUP.md`](../development/NEO4J_BACKUP.md)

---

## Overview

Phase 70 provides automated and on-demand backup of the Neo4j database, supporting full database dumps and partial (space-level) exports. A configurable scheduler handles recurring backups, a retention engine manages cleanup, and a restore service enables disaster recovery.

### Motivation

MDEMG stores long-lived agent memory — decisions, corrections, institutional knowledge — that cannot be regenerated. Loss of this data is catastrophic.

### Scope (P0 — Implemented)

- **Full backup**: Complete Neo4j database dump via `docker exec neo4j-admin database dump`
- **Partial backup**: Space-level export using the existing `.mdemg` format via `transfer.Exporter`
- **Scheduled**: Simple ticker scheduler (full weekly, partial daily — configurable)
- **Retention**: Automatic cleanup by count, age, and storage quota
- **Restore**: From full dump via `neo4j-admin database load`
- **Protected spaces**: `mdemg-dev` always included in partial backups

---

## Implementation

### New Files (7)

| File | Lines | Purpose |
|------|-------|---------|
| `internal/backup/types.go` | ~85 | Config, BackupRecord, BackupManifest, request/response types |
| `internal/backup/service.go` | ~280 | Core orchestrator: Trigger, Get, List, Delete, manifest I/O, Neo4j queries |
| `internal/backup/full.go` | ~160 | Full database dump via `docker exec neo4j-admin` + restore logic |
| `internal/backup/partial.go` | ~90 | Space-level backup via `transfer.Exporter` → `.mdemg` file |
| `internal/backup/retention.go` | ~145 | Count/age/storage-based cleanup engine |
| `internal/backup/scheduler.go` | ~75 | Ticker-based automatic backup scheduler |
| `internal/api/handlers_backup.go` | ~240 | 7 HTTP handlers for backup endpoints |

### Modified Files (4)

| File | Change |
|------|--------|
| `internal/config/config.go` | 11 new `Backup*` config fields + `FromEnv()` parsing |
| `internal/api/server.go` | Backup import, Server struct fields, NewServer init, Shutdown, 7 routes |
| `.env.example` | Backup configuration section with all 11 env vars |
| `.gitignore` | Added `backups/` directory |

### Migration

`migrations/V0013__backup_metadata.cypher` — BackupMeta constraint + index:

```cypher
CREATE CONSTRAINT backup_meta_id IF NOT EXISTS
FOR (b:BackupMeta) REQUIRE b.backup_id IS UNIQUE;

CREATE INDEX backup_meta_started IF NOT EXISTS
FOR (b:BackupMeta) ON (b.started_at);
```

---

## API Endpoints (7 for P0)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/backup/trigger` | Trigger backup (returns 202) |
| `GET` | `/v1/backup/status/{id}` | Backup job progress |
| `GET` | `/v1/backup/list` | List available backups (optional `?type=` filter) |
| `GET` | `/v1/backup/manifest/{id}` | Backup manifest details |
| `DELETE` | `/v1/backup/{id}` | Delete a backup |
| `POST` | `/v1/backup/restore` | Trigger restore (full dump only) |
| `GET` | `/v1/backup/restore/status/{id}` | Restore job progress |

All endpoints return 503 when `BACKUP_ENABLED=false`.

### Request/Response Examples

**Trigger Backup:**

```json
POST /v1/backup/trigger
{
  "type": "partial_space",
  "space_ids": ["mdemg-dev"],
  "keep_forever": false,
  "label": "manual-backup"
}
```

Response (202):

```json
{
  "backup_id": "bk-20260208-022802-partial_space",
  "status": "pending",
  "message": "backup triggered"
}
```

**Backup Status (completed):**

```json
GET /v1/backup/status/bk-20260208-022802-partial_space
{
  "backup_id": "bk-20260208-022802-partial_space",
  "status": "completed",
  "progress": {
    "total": 1,
    "current": 1,
    "percentage": 100,
    "phase": "computing checksum"
  },
  "result": {
    "backup_id": "bk-20260208-022802-partial_space",
    "checksum": "sha256:c00da0f7...",
    "path": "backups/bk-20260208-022802-partial_space.mdemg",
    "size": 105898054
  }
}
```

**List Backups:**

```json
GET /v1/backup/list
{
  "backups": [
    {
      "backup_id": "bk-20260208-022802-partial_space",
      "type": "partial_space",
      "format_version": "1.0",
      "created_at": "2026-02-08T02:28:02Z",
      "checksum": "sha256:c00da0f7...",
      "size_bytes": 105898054,
      "spaces": ["mdemg-dev"],
      "node_count": 21033,
      "edge_count": 232434,
      "keep_forever": false,
      "label": "manual-backup"
    }
  ],
  "count": 1
}
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_ENABLED` | `false` | Enable backup module |
| `BACKUP_STORAGE_DIR` | `./backups` | Directory for backup artifacts |
| `BACKUP_FULL_CMD` | `docker` | Command for full backups |
| `BACKUP_NEO4J_CONTAINER` | `mdemg-neo4j` | Docker container name |
| `BACKUP_FULL_INTERVAL_HOURS` | `168` | Hours between full backups (weekly) |
| `BACKUP_PARTIAL_INTERVAL_HOURS` | `24` | Hours between partial backups (daily) |
| `BACKUP_RETENTION_FULL_COUNT` | `4` | Keep last N full backups |
| `BACKUP_RETENTION_PARTIAL_COUNT` | `14` | Keep last N partial backups |
| `BACKUP_RETENTION_MAX_AGE_DAYS` | `90` | Delete backups older than N days |
| `BACKUP_RETENTION_MAX_STORAGE_GB` | `50` | Storage quota in GB |
| `BACKUP_RETENTION_RUN_AFTER_BACKUP` | `true` | Run retention after each backup |

---

## Backup Types

### Full Backup

Runs `neo4j-admin database dump` inside the Docker container, copies the `.dump` file out, computes SHA256, and writes a manifest sidecar. Requires Neo4j running in Docker.

### Partial (Space-Level) Backup

Exports selected spaces using `transfer.Exporter.Export()` → `transfer.WriteFile()` to produce `.mdemg` JSON files. The protected `mdemg-dev` space is always included. When `space_ids` is empty, all spaces are exported.

---

## Retention Policies

Three policies apply in order after each backup (when `BACKUP_RETENTION_RUN_AFTER_BACKUP=true`):

1. **Count-based**: Keep the newest N full and N partial backups
2. **Age-based**: Delete backups older than `BACKUP_RETENTION_MAX_AGE_DAYS`
3. **Storage-based**: Delete oldest backups until total storage is under quota

Backups with `keep_forever: true` are exempt from all three policies.

---

## Scheduler

Simple ticker-based scheduler that fires at configured intervals:

- Full backups: every `BACKUP_FULL_INTERVAL_HOURS` (default: 168h = weekly)
- Partial backups: every `BACKUP_PARTIAL_INTERVAL_HOURS` (default: 24h = daily)

The first automatic backup fires one interval after server start.

---

## UATS Specs (7)

All specs test 503 response when backup is disabled (matching the scraper pattern):

- `backup_trigger.uats.json`
- `backup_status.uats.json`
- `backup_list.uats.json`
- `backup_manifest.uats.json`
- `backup_delete.uats.json`
- `backup_restore.uats.json`
- `backup_restore_status.uats.json`

---

## E2E Test Results (2026-02-07)

Tested against live `mdemg-dev` space (21,033 nodes, 232,434 edges):

| Test | Result |
|------|--------|
| Trigger partial backup | 202, backup ID returned |
| Status polling | completed, 101MB `.mdemg`, SHA256 verified |
| Manifest on disk | 21,033 nodes, 232,434 edges, schema version 0 |
| List backups | 1 backup with full metadata |
| Manifest by ID | Full manifest JSON returned |
| Delete backup | Files + manifest removed from disk |
| UATS suite | 146/147 passing (99.3%), all 7 backup specs pass |

---

## Acceptance Criteria

- [x] Full backup creates a valid `.dump` file that can be restored with `neo4j-admin database load`
- [x] Partial backup creates valid `.mdemg` file(s) importable via Space Transfer
- [x] Scheduler triggers backups at configured intervals
- [x] Retention engine enforces count, age, and quota limits
- [x] Protected backups (`keep_forever`) are never auto-deleted
- [x] Restore from full dump supported via `neo4j-admin database load`
- [x] `mdemg-dev` space is always included in partial backups
- [x] All 7 API endpoints respond with correct status codes
- [x] UATS specs pass for all endpoints
- [x] Backup files include SHA256 checksum for integrity verification
- [x] `go build ./...` and `go vet ./...` pass cleanly

### Deferred to Future (P1/P2)

- Delta backup using `since_timestamp` from Phase 34
- Restore from partial `.mdemg` file with conflict modes
- Backup history tracked in Neo4j (`:BackupMeta` nodes)
- Missed-schedule detection and catch-up on startup
- Dry-run mode for retention preview
- Schedule management API endpoints
