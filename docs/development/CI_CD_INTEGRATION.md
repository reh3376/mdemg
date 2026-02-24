# CI/CD Integration Guide

This guide shows how to integrate MDEMG codebase ingestion into your CI/CD pipelines.

## GitHub Actions: Incremental Ingest on Push

```yaml
name: MDEMG Incremental Ingest
on:
  push:
    branches: [main, develop]

jobs:
  ingest:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 2  # Need parent commit for diff

      - name: Download ingest-codebase
        run: |
          # Download the latest ingest-codebase binary
          # Replace with your artifact URL or build step
          go build -o ./bin/ingest-codebase ./cmd/ingest-codebase

      - name: Run incremental ingest
        env:
          MDEMG_ENDPOINT: ${{ secrets.MDEMG_ENDPOINT }}
        run: |
          ./bin/ingest-codebase \
            --path . \
            --space-id "${{ github.event.repository.name }}" \
            --endpoint "$MDEMG_ENDPOINT" \
            --incremental \
            --since HEAD~1 \
            --archive-deleted \
            --quiet \
            --log-file /tmp/mdemg-ingest.log

      - name: Upload ingest log
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: ingest-log
          path: /tmp/mdemg-ingest.log
          retention-days: 7
```

## Git Hook: Automatic Post-Commit Ingest

Install the provided git hook for automatic incremental ingestion on every commit:

```bash
# Install the hook
./scripts/install-mdemg-hook.sh

# Or manually:
cp scripts/mdemg-git-hook .git/hooks/post-commit
chmod +x .git/hooks/post-commit
```

### Configuration

Set environment variables to configure the hook:

| Variable | Default | Description |
|----------|---------|-------------|
| `MDEMG_SPACE_ID` | repo basename | Target space ID |
| `MDEMG_ENDPOINT` | auto-detect | MDEMG API endpoint |
| `MDEMG_INGEST_BIN` | auto-detect | Path to ingest-codebase binary |
| `MDEMG_SKIP_PATTERNS` | `.git,node_modules,vendor` | Comma-separated patterns to skip |
| `MDEMG_VERBOSE` | `false` | Enable verbose output |
| `MDEMG_LOG_FILE` | (none) | Write logs to file |
| `MDEMG_DISABLED` | `false` | Disable the hook |

## CLI Flags for CI/CD

The `ingest-codebase` CLI has flags designed for CI/CD use:

| Flag | Description |
|------|-------------|
| `--quiet` | Suppress all non-error output (ideal for hooks and CI) |
| `--log-file <path>` | Redirect logs to a file for later inspection |
| `--progress-json` | Emit structured JSON progress events to stdout |
| `--incremental` | Only process files changed since `--since` commit |
| `--since <commit>` | Git commit to diff against (default: `HEAD~1`) |
| `--archive-deleted` | Archive graph nodes for deleted files |
| `--dry-run` | Preview changes without ingesting |

## Scheduled Sync (Server-Side)

The MDEMG server can automatically re-ingest stale spaces on a schedule:

```bash
# Enable scheduled sync every 60 minutes
export SYNC_INTERVAL_MINUTES=60

# Only monitor specific spaces (empty = all)
export SYNC_SPACE_IDS=my-project,other-project

# Consider a space stale after 24 hours without ingest
export SYNC_STALE_THRESHOLD_HOURS=24

# Map spaces to their repository paths for auto-ingest
export SYNC_REPO_PATHS=my-project=/home/user/my-project,other-project=/home/user/other-project
```

## Checking Freshness

Use the freshness API to check if a space needs re-ingestion:

```bash
curl -s http://localhost:9999/v1/memory/spaces/my-project/freshness | jq
```

Response:

```json
{
  "space_id": "my-project",
  "last_ingest_at": "2026-02-03T15:30:00Z",
  "last_ingest_type": "codebase-ingest",
  "ingest_count": 12,
  "is_stale": false,
  "stale_hours": 8,
  "threshold_hours": 24
}
```

The MCP tool `memory_space_freshness` provides the same information from within your IDE.
