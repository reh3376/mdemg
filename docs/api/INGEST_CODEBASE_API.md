# Ingest Codebase API

## Endpoint: `POST /v1/memory/ingest-codebase`

Triggers codebase ingestion as a background job. Returns immediately with a job ID for status tracking.

---

## Quick Start

### Minimal Request

```bash
curl -X POST http://localhost:9999/v1/memory/ingest-codebase \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "path": "/path/to/codebase"
  }'
```

### Response

```json
{
  "job_id": "a1b2c3d4",
  "status": "queued",
  "space_id": "my-project",
  "path": "/path/to/codebase"
}
```

---

## Full Request Schema

```json
{
  "space_id": "string (required)",
  "path": "string (required)",

  "source": {
    "type": "local | git",
    "branch": "string",
    "since": "string (default: HEAD~1)"
  },

  "languages": {
    "typescript": true,
    "python": true,
    "java": true,
    "rust": true,
    "go": true,
    "markdown": true,
    "include_tests": false
  },

  "symbols": {
    "extract": true,
    "max_per_file": 1000
  },

  "exclusions": {
    "preset": "default | ml_cuda | web_monorepo",
    "directories": [".git", "vendor", "node_modules"],
    "max_file_size": 1048576
  },

  "processing": {
    "batch_size": 100,
    "workers": 4,
    "max_elements_per_file": 500,
    "delay_ms": 50
  },

  "llm_summary": {
    "enabled": false,
    "provider": "openai | ollama",
    "model": "gpt-4o-mini",
    "batch_size": 10
  },

  "options": {
    "incremental": false,
    "archive_deleted": true,
    "consolidate": true,
    "dry_run": false,
    "verbose": false,
    "limit": 0
  },

  "retry": {
    "max_attempts": 3,
    "initial_delay_ms": 2000,
    "timeout_seconds": 300
  }
}
```

---

## Options Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `space_id` | string | MDEMG space identifier |
| `path` | string | Local filesystem path to codebase |

### Source Configuration (`source`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"local"` | Source type: `"local"` or `"git"` |
| `branch` | string | - | Git branch (for git sources) |
| `since` | string | `"HEAD~1"` | Commit to compare for incremental mode |

### Language Filters (`languages`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `typescript` | bool | `true` | Include .ts, .tsx, .js, .jsx files |
| `python` | bool | `true` | Include .py files |
| `java` | bool | `true` | Include .java files |
| `rust` | bool | `true` | Include .rs files |
| `go` | bool | `true` | Include .go files |
| `markdown` | bool | `true` | Include .md files |
| `include_tests` | bool | `false` | Include test files (*_test.go,*.spec.ts) |

### Symbol Extraction (`symbols`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `extract` | bool | `true` | Extract code symbols (functions, classes, constants) |
| `max_per_file` | int | `1000` | Maximum symbols to extract per file |

### Exclusions (`exclusions`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `preset` | string | `"default"` | Preset: `"default"`, `"ml_cuda"`, `"web_monorepo"` |
| `directories` | []string | `[".git", "vendor", "node_modules", ".worktrees"]` | Directories to exclude |
| `max_file_size` | int | `1048576` | Max file size in bytes (1MB default) |

**Presets:**

- `default`: Standard exclusions for most projects
- `ml_cuda`: Excludes large model files, CUDA kernels
- `web_monorepo`: Excludes node_modules, dist, build artifacts

### Processing (`processing`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `batch_size` | int | `100` | Files per batch (optimal for ~15/s per worker) |
| `workers` | int | `4` | Parallel worker count |
| `max_elements_per_file` | int | `500` | Max code elements per file |
| `delay_ms` | int | `50` | Delay between batches in milliseconds |

### LLM Summary (`llm_summary`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Generate semantic summaries using LLM |
| `provider` | string | `"openai"` | LLM provider: `"openai"` or `"ollama"` |
| `model` | string | `"gpt-4o-mini"` | Model for summary generation |
| `batch_size` | int | `10` | Files per LLM API call |

**Note:** Requires `OPENAI_API_KEY` environment variable when using OpenAI.

### General Options (`options`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `incremental` | bool | `false` | Only ingest files changed since last commit |
| `archive_deleted` | bool | `true` | Archive nodes for deleted files |
| `consolidate` | bool | `true` | Run consolidation after ingestion |
| `dry_run` | bool | `false` | Preview without actually ingesting |
| `verbose` | bool | `false` | Verbose logging |
| `limit` | int | `0` | Limit elements to ingest (0 = unlimited) |

### Retry Configuration (`retry`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | `3` | Maximum retry attempts per batch |
| `initial_delay_ms` | int | `2000` | Initial retry delay (doubles each retry) |
| `timeout_seconds` | int | `300` | HTTP timeout for batch requests |

---

## Job Management

### Check Job Status

```bash
GET /v1/memory/ingest-codebase/{job_id}
```

**Response:**

```json
{
  "job_id": "a1b2c3d4",
  "status": "running",
  "space_id": "my-project",
  "path": "/path/to/codebase",
  "stats": {
    "files_found": 4522,
    "files_processed": 1500,
    "symbols_extracted": 8234,
    "errors": 0,
    "rate": 14.5,
    "duration": "1m45s"
  }
}
```

**Status values:** `queued`, `running`, `completed`, `failed`, `cancelled`

### List All Jobs

```bash
GET /v1/memory/ingest-codebase
```

**Response:**

```json
{
  "jobs": [
    {"job_id": "a1b2c3d4", "status": "completed", ...},
    {"job_id": "e5f6g7h8", "status": "running", ...}
  ]
}
```

### Cancel Job

```bash
DELETE /v1/memory/ingest-codebase/{job_id}
```

**Response:**

```json
{
  "status": "cancelled",
  "job_id": "a1b2c3d4"
}
```

---

## Examples

### TypeScript Project (Fast)

```json
{
  "space_id": "my-ts-app",
  "path": "/home/user/projects/my-ts-app",
  "languages": {
    "typescript": true,
    "python": false,
    "java": false,
    "rust": false,
    "markdown": true
  },
  "processing": {
    "workers": 8
  }
}
```

### ML/CUDA Project

```json
{
  "space_id": "ml-training",
  "path": "/home/user/ml-project",
  "exclusions": {
    "preset": "ml_cuda"
  },
  "symbols": {
    "extract": true,
    "max_per_file": 500
  }
}
```

### Incremental Update

```json
{
  "space_id": "my-project",
  "path": "/home/user/my-project",
  "options": {
    "incremental": true
  },
  "source": {
    "since": "HEAD~5"
  }
}
```

### With LLM Summaries

```json
{
  "space_id": "documented-project",
  "path": "/home/user/project",
  "llm_summary": {
    "enabled": true,
    "provider": "openai",
    "model": "gpt-4o-mini"
  }
}
```

### Dry Run Preview

```json
{
  "space_id": "test",
  "path": "/home/user/project",
  "options": {
    "dry_run": true,
    "verbose": true,
    "limit": 100
  }
}
```

---

## Error Responses

### 400 Bad Request

```json
{"error": "space_id is required"}
{"error": "path is required"}
{"error": "path does not exist: /invalid/path"}
```

### 404 Not Found

```json
{"error": "job not found"}
```

### 405 Method Not Allowed

```json
"Method not allowed"
```

---

## Performance Guidelines

| Codebase Size | Recommended Workers | Expected Rate | Estimated Time |
|---------------|--------------------:|:-------------:|----------------|
| Small (<1K files) | 2-4 | 10-15/s | < 2 min |
| Medium (1K-10K) | 4-8 | 12-18/s | 5-15 min |
| Large (10K-50K) | 8-12 | 15-20/s | 30-60 min |
| Monorepo (50K+) | 12-16 | 15-25/s | 1-3 hours |

**Tips:**

- Use `incremental: true` for subsequent updates
- Use `preset: "ml_cuda"` for ML repos with large binaries
- Increase `batch_size` for faster networks
- Reduce `max_file_size` to skip large generated files
