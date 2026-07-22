# Ingest Codebase API

> Rewritten by DOC-CURRENCY-002 (2026-07-21) against `internal/api/handlers.go` +
> `internal/models/models.go`. Earlier versions of this doc described a
> `POST /v1/memory/ingest-codebase` endpoint with a nested request schema
> (`source`/`languages`/`symbols`/`exclusions`/`processing`/`llm_summary`/
> `options`/`retry`) — **that endpoint and schema never shipped**. The real
> surface is below.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/memory/ingest/trigger` | Start a codebase ingestion background job |
| GET | `/v1/memory/ingest/status/{job_id}` | Job status + progress |
| POST | `/v1/memory/ingest/cancel/{job_id}` | Cancel a running job |
| GET | `/v1/memory/ingest/jobs` | List all ingestion jobs |
| POST | `/v1/memory/ingest/files` | Re-ingest specific files (sync ≤50 files, background job >50) |

The server executes the job by delegating to the `mdemg ingest` CLI
subprocess with `--progress-json` streaming (binary resolved via
`resolveMdemgBin()`: `MDEMG_BIN` → `os.Executable()` → PATH → `./bin/mdemg` —
INGEST-EXEC-001). Scheduled-sync runs additionally report to
`scheduled_job_events` as `job_name='codebase-sync'` (NOSILENT pattern);
manual API-triggered jobs are visible through the job queue only.

---

## Quick Start

```bash
curl -X POST http://localhost:9999/v1/memory/ingest/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "path": "/path/to/codebase"
  }'
```

**Response (202 Accepted):**

```json
{
  "job_id": "ingest-a1b2c3d4",
  "space_id": "my-project",
  "status": "pending",
  "message": "Ingestion job created. Use GET /v1/memory/ingest/status/ingest-a1b2c3d4 to check progress.",
  "created_at": "2026-07-21T12:00:00Z"
}
```

---

## Trigger Request Schema (`models.IngestTriggerRequest` — flat, not nested)

```json
{
  "space_id": "string (required)",
  "path": "string (required)",
  "batch_size": 100,
  "workers": 4,
  "timeout_seconds": 300,
  "extract_symbols": true,
  "consolidate": true,
  "include_tests": false,
  "incremental": false,
  "since_commit": "HEAD~1",
  "exclude_dirs": [".git", "vendor", "node_modules"],
  "limit": 0,
  "dry_run": false
}
```

| Field | Type | Default | Forwarded as | Description |
|-------|------|---------|--------------|-------------|
| `space_id` | string | required | `--space-id` | Target MDEMG space (1–256 chars) |
| `path` | string | required | `--path` | Local filesystem path to the codebase |
| `batch_size` | int | 100 | `--batch` | Items per batch |
| `workers` | int | 4 | `--workers` | Parallel worker count |
| `timeout_seconds` | int | 300 | `--timeout` | HTTP timeout per batch |
| `extract_symbols` | bool | true | `--extract-symbols` | Extract code symbols (functions, classes, constants) |
| `consolidate` | bool | true | `--consolidate` | Run consolidation after ingestion |
| `include_tests` | bool | false | `--include-tests` | Include test files |
| `incremental` | bool | false | `--incremental` | Only files changed since `since_commit` |
| `since_commit` | string | `HEAD~1` | `--since` | Git ref for incremental comparison |
| `exclude_dirs` | []string | — | `--exclude` (comma-joined) | Directories to skip |
| `limit` | int | 0 (no limit) | `--limit` | Max elements to ingest |
| `dry_run` | bool | false | `--dry-run` | Preview without ingesting |

**Accepted but not forwarded by the trigger handler** (present on the request
model; the handler currently drops them before building the CLI args):
`include_md`, `include_ts`, `include_py`, `archive_deleted`. Use the
`mdemg ingest` CLI directly if you need those toggles
(`--archive-deleted` is honored on the scheduled-sync path).

---

## Job Status

```bash
curl -s http://localhost:9999/v1/memory/ingest/status/ingest-a1b2c3d4
```

**Response (`models.IngestJobStatusResponse`):**

```json
{
  "job_id": "ingest-a1b2c3d4",
  "space_id": "my-project",
  "status": "running",
  "created_at": "2026-07-21T12:00:00Z",
  "started_at": "2026-07-21T12:00:01Z",
  "progress": {
    "total": 4522,
    "current": 1500,
    "percentage": 33.2,
    "phase": "ingesting",
    "rate": 14.5
  }
}
```

`completed_at`, `result`, and `error` appear once terminal.
**Status values:** `pending`, `running`, `completed`, `failed`, `cancelled`.

## List Jobs

```bash
curl -s http://localhost:9999/v1/memory/ingest/jobs
```

Returns `{"jobs": [...], "count": N}` — each entry carries `job_id`,
`status`, `space_id`, `created_at`, `started_at`/`completed_at` when set, and
the same `progress` object.

## Cancel

```bash
curl -X POST http://localhost:9999/v1/memory/ingest/cancel/ingest-a1b2c3d4
```

**200:** `{"job_id": "...", "status": "cancelled", "message": "Job cancellation requested"}`
**404:** job unknown or already completed.

## Re-ingest Specific Files

```bash
curl -X POST http://localhost:9999/v1/memory/ingest/files \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-project", "files": ["internal/api/handlers.go"]}'
```

Synchronous for ≤50 files (results inline); >50 files falls back to a
background job with the status flow above. Also exposed as the MCP tool
`memory_ingest_files`.

---

## Examples

### Incremental update (changed files only)

```json
{
  "space_id": "my-project",
  "path": "/home/user/my-project",
  "incremental": true,
  "since_commit": "HEAD~5"
}
```

### Dry-run preview

```json
{
  "space_id": "test",
  "path": "/home/user/project",
  "dry_run": true,
  "limit": 100
}
```

### Big monorepo

```json
{
  "space_id": "monorepo",
  "path": "/home/user/monorepo",
  "workers": 8,
  "exclude_dirs": ["node_modules", "dist", "build", ".worktrees"]
}
```

---

## Error Responses

| Status | Body |
|--------|------|
| 400 | `{"error": "space_id is required"}` / `{"error": "path is required"}` / `{"error": "job_id required in path"}` |
| 404 | `{"error": "job not found: <id>"}` |
| 405 | empty body (method not allowed) |

## Performance Guidelines

| Codebase Size | Recommended Workers | Expected Rate |
|---------------|--------------------:|:-------------:|
| Small (<1K files) | 2–4 | 10–15 files/s |
| Medium (1K–10K) | 4–8 | 12–18 files/s |
| Large (10K–50K) | 8–12 | 15–20 files/s |

Use `incremental: true` for subsequent updates; `dry_run` + `limit` to
preview scope cheaply. UATS contracts for this surface live under
`docs/api/api-spec/uats/specs/` (`memory_ingest_*`).
