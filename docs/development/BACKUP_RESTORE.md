# MDEMG Backup, Restore & Space Import/Export Guide

This guide covers how to back up, restore, export, and import MDEMG memory spaces. Use these tools to create snapshots, share spaces with other developers, or migrate data between MDEMG instances.

---

## Quick Reference

| Task | Command |
|------|---------|
| Export a space | `POST /v1/backup/trigger` with `type: "partial_space"` |
| Export all spaces | `POST /v1/backup/trigger` with `type: "partial_space"` (no `space_ids`) |
| Full database backup | `POST /v1/backup/trigger` with `type: "full"` |
| Check backup status | `GET /v1/backup/status/{backup_id}` |
| List all backups | `GET /v1/backup/list` |
| Restore from backup | `POST /v1/backup/restore` |
| Check restore status | `GET /v1/backup/restore/status/{restore_id}` |

---

## Prerequisites

1. MDEMG server running with backup enabled:

   ```env
   BACKUP_ENABLED=true
   BACKUP_STORAGE_DIR=./backups
   BACKUP_NEO4J_CONTAINER=mdemg-neo4j
   ```

2. A `backups/` directory in your MDEMG root (created automatically if `BACKUP_STORAGE_DIR=./backups`).

3. For full backups only: Docker running with the Neo4j container accessible.

---

## 1. Space Export (Partial Backup)

Partial backups use the `.mdemg` format — a self-contained JSON file with all nodes, edges, observations, symbols, and embeddings for the selected spaces. This is the recommended format for sharing with dev teams.

### Export specific spaces

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "type": "partial_space",
    "space_ids": ["whk-wms"],
    "keep_forever": true,
    "label": "whk-wms-export-20260216"
  }'
```

**Response:**

```json
{
  "backup_id": "bk-20260216-143022-partial_space",
  "status": "pending",
  "message": "backup triggered"
}
```

### Export all spaces

Omit `space_ids` to export every space in the database:

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "type": "partial_space",
    "keep_forever": true,
    "label": "full-export-all-spaces"
  }'
```

### Check export progress

Exports run asynchronously. Poll the status endpoint:

```bash
curl -s http://localhost:9999/v1/backup/status/bk-20260216-143022-partial_space
```

**Response (in progress):**

```json
{
  "backup_id": "bk-20260216-143022-partial_space",
  "status": "running",
  "progress": {
    "total": 2,
    "current": 1,
    "percentage": 50,
    "phase": "exporting space whk-wms"
  }
}
```

**Response (completed):**

```json
{
  "backup_id": "bk-20260216-143022-partial_space",
  "status": "completed"
}
```

### Locate the export file

Export files are saved to `BACKUP_STORAGE_DIR` (default: `./backups/`):

```
backups/
  bk-20260216-143022-partial_space.mdemg           # Data file
  bk-20260216-143022-partial_space.manifest.json   # Metadata sidecar
```

The `.mdemg` file is what you share with dev teams. The `.manifest.json` contains metadata (checksums, node/edge counts, space list).

### What's inside the .mdemg file

The `.mdemg` format is a JSON file containing sequenced chunks:

| Chunk Type | Contents |
|-----------|----------|
| `METADATA` | Schema version, export timestamp, lineage |
| `NODES` | Batches of MemoryNode data (all layers, all properties) |
| `EDGES` | All relationships (RELATES_TO, ABSTRACTS_TO, CO_ACTIVATED_WITH, etc.) |
| `OBSERVATIONS` | Conversation observations (CMS data) |
| `SYMBOLS` | Code symbols extracted during ingestion |
| `SUMMARY` | Export summary with counts and completion time |

Embeddings are included by default so the importing instance doesn't need to regenerate them.

---

## 2. Full Database Backup

Full backups use `neo4j-admin dump` to create a complete database snapshot. This captures everything including indexes and constraints.

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "type": "full",
    "keep_forever": true,
    "label": "pre-migration-snapshot"
  }'
```

**Important limitations:**

- Neo4j Community Edition requires the database to be stopped for `neo4j-admin dump`. If your database is running, this will fail. Use partial backup (space export) instead.
- Neo4j Enterprise Edition supports online dumps.
- Full backup produces a `.dump` file (Neo4j native format), not `.mdemg`.
- Restoring a full backup replaces the entire database.

**When to use full backup:**

- Disaster recovery snapshots (stop DB, backup, restart)
- Before major schema migrations

**When to use partial backup instead:**

- Sharing spaces with dev teams
- Migrating specific spaces between instances
- Regular automated backups of a running system

---

## 3. Restore from Backup

### Restore a partial backup (.mdemg)

```bash
curl -s -X POST http://localhost:9999/v1/backup/restore \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": "bk-20260216-143022-partial_space",
    "snapshot_before": true
  }'
```

| Field | Description |
|-------|-------------|
| `backup_id` | The ID of the backup to restore (from `/v1/backup/list`) |
| `snapshot_before` | If `true`, creates a safety backup before restoring (recommended) |

**Response:**

```json
{
  "restore_id": "rst-20260216-150000",
  "backup_id": "bk-20260216-143022-partial_space",
  "status": "pending",
  "message": "restore triggered"
}
```

### Check restore progress

```bash
curl -s http://localhost:9999/v1/backup/restore/status/rst-20260216-150000
```

### Conflict handling

When importing data into a database that already has nodes, the importer handles conflicts:

| Mode | Behavior |
|------|----------|
| `CONFLICT_SKIP` | Skip nodes that already exist (default) |
| `CONFLICT_OVERWRITE` | Replace existing nodes with imported data |
| `CONFLICT_CRDT` | Merge learning edges (CO_ACTIVATED_WITH) using CRDT semantics |
| `CONFLICT_ERROR` | Fail on any conflict |

By default, restore uses `CONFLICT_SKIP` — existing data is preserved and only new nodes/edges are added.

---

## 4. Importing a .mdemg File from Another Instance

When a dev team member shares a `.mdemg` file with you:

### Step 1: Place the file in your backups directory

```bash
cp /path/to/shared-export.mdemg ./backups/
```

### Step 2: Create a manifest (if not provided)

If you only received the `.mdemg` file without a manifest, create a minimal one:

```bash
BACKUP_ID="imported-whk-wms"
cat > ./backups/${BACKUP_ID}.manifest.json << 'EOF'
{
  "backup_id": "imported-whk-wms",
  "type": "partial_space",
  "format_version": "1.0",
  "created_at": "2026-02-16T00:00:00Z",
  "checksum": "",
  "size_bytes": 0,
  "spaces": ["whk-wms"],
  "node_count": 0,
  "edge_count": 0,
  "schema_version": 0,
  "keep_forever": true,
  "label": "imported from team"
}
EOF
```

Rename the `.mdemg` file to match the backup_id:

```bash
mv shared-export.mdemg ./backups/imported-whk-wms.mdemg
```

### Step 3: Trigger restore

```bash
curl -s -X POST http://localhost:9999/v1/backup/restore \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": "imported-whk-wms",
    "snapshot_before": true
  }'
```

### Step 4: Verify

```bash
# Check restore completed
curl -s http://localhost:9999/v1/backup/restore/status/{restore_id}

# Verify the space was imported
curl -s http://localhost:9999/v1/neo4j/overview | python3 -m json.tool
```

---

## 5. Managing Backups

### List all backups

```bash
curl -s http://localhost:9999/v1/backup/list | python3 -m json.tool
```

Filter by type:

```bash
curl -s "http://localhost:9999/v1/backup/list?type=partial_space" | python3 -m json.tool
```

### View backup details

```bash
curl -s http://localhost:9999/v1/backup/manifest/{backup_id} | python3 -m json.tool
```

### Delete a backup

```bash
curl -s -X DELETE http://localhost:9999/v1/backup/{backup_id}
```

Backups marked `keep_forever: true` cannot be deleted. This protects important snapshots from accidental removal.

---

## 6. Automated Backups

MDEMG includes a built-in scheduler for automatic backups. Configure via environment variables:

```env
# Backup intervals
BACKUP_FULL_INTERVAL_HOURS=168       # Full backup every 7 days
BACKUP_PARTIAL_INTERVAL_HOURS=24     # Partial backup every 24 hours

# Retention policies (automatic cleanup)
BACKUP_RETENTION_FULL_COUNT=4        # Keep last 4 full backups
BACKUP_RETENTION_PARTIAL_COUNT=14    # Keep last 14 partial backups
BACKUP_RETENTION_MAX_AGE_DAYS=90     # Delete backups older than 90 days
BACKUP_RETENTION_MAX_STORAGE_GB=50   # Storage quota
BACKUP_RETENTION_RUN_AFTER_BACKUP=true  # Run retention after each backup
```

Backups marked `keep_forever: true` are exempt from all retention policies.

---

## 7. Protected Spaces

The `mdemg-dev` space (Claude's conversation memory) has special protections:

- **Always included** in partial backups, even if not specified in `space_ids`
- **Cannot be deleted** via the API (hardcoded protection)
- **Import safe** — importing into a database that already has `mdemg-dev` uses conflict resolution (skip by default)

---

## Common Workflows

### Share a codebase space with a teammate

```bash
# 1. Export the space
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","space_ids":["whk-wms"],"keep_forever":true,"label":"for-teammate"}'

# 2. Wait for completion
curl -s http://localhost:9999/v1/backup/status/{backup_id}

# 3. Share the .mdemg file from backups/
```

### Pre-migration safety snapshot

```bash
# Take a snapshot of everything before making changes
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","keep_forever":true,"label":"pre-migration"}'
```

### Restore after a bad operation

```bash
# 1. Find the backup to restore
curl -s http://localhost:9999/v1/backup/list | python3 -m json.tool

# 2. Restore with safety snapshot
curl -s -X POST http://localhost:9999/v1/backup/restore \
  -H "Content-Type: application/json" \
  -d '{"backup_id":"bk-20260216-143022-partial_space","snapshot_before":true}'

# 3. Monitor progress
curl -s http://localhost:9999/v1/backup/restore/status/{restore_id}
```

---

## API Reference

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/backup/trigger` | POST | Trigger a new backup (full or partial) |
| `/v1/backup/status/{id}` | GET | Check backup job status |
| `/v1/backup/list` | GET | List all backups (optional `?type=` filter) |
| `/v1/backup/manifest/{id}` | GET | Get detailed backup manifest |
| `/v1/backup/{id}` | DELETE | Delete a backup (blocked if `keep_forever`) |
| `/v1/backup/restore` | POST | Trigger a restore operation |
| `/v1/backup/restore/status/{id}` | GET | Check restore job status |

All endpoints return `503 Service Unavailable` when `BACKUP_ENABLED=false`.

---

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| `backup not enabled` | `BACKUP_ENABLED` not set | Add `BACKUP_ENABLED=true` to `.env` and restart |
| Full backup fails: "database in use" | Neo4j Community Edition limitation | Use partial backup instead, or stop Neo4j first |
| Full backup fails: "/backup not found" | Missing dir in container | `docker exec mdemg-neo4j mkdir -p /backup` |
| Restore shows 0 nodes created | All nodes already exist | Expected with `CONFLICT_SKIP`; use `CONFLICT_OVERWRITE` to replace |
| Large export file (>500MB) | Embeddings included by default | Normal — embeddings are ~90% of file size |
