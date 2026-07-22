---
created: 2026-04-03
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "DOCKER-P2"
---

# Docker Deployment

## Summary

**Feature**: Docker Deployment
**Summary**: Primary deployment model using Docker Compose with 5 services (mdemg, neo4j, timescaledb, neural-sidecar, grafana), dynamic port allocation, and health check orchestration.


## Overview

Docker Compose is the only deployment path for MDEMG. All 5 services (mdemg server, Neo4j, TimescaleDB, neural-sidecar, Grafana) run as containers. `mdemg init` scans for free ports, prompts for service credentials, generates `.env`, and starts the stack.

**Windows users**: Install [WSL2](https://learn.microsoft.com/en-us/windows/wsl/install) first, then run all commands inside WSL2. Native Windows is not supported.

## Problem

MDEMG previously required 3 platform-specific installers (Homebrew, APT/Debian, Scoop/Windows) and 3 native companion apps (macOS menu bar, Linux sidebar, Windows tray). Each platform had its own process supervision (launchd, systemd, Windows Service), creating a 6-repo maintenance burden with cross-platform divergence.

## Architecture

### Compose Services

| Service | Image | Container Port | Health Check |
|---------|-------|----------------|--------------|
| mdemg | `ghcr.io/reh3376/mdemg:latest` | 9999 | `wget /healthz` |
| neo4j | `neo4j:5` (community) | 7687 (Bolt), 7474 (HTTP) | `cypher-shell RETURN 1` |
| timescaledb | `timescale/timescaledb:2.25.1-pg16` | 5432 | `pg_isready` |
| neural-sidecar | `ghcr.io/reh3376/mdemg-neural-sidecar:latest` | 8000 | Python `urlopen /health` |
| grafana | `grafana/grafana:10.2.2` | 3000 | `wget /api/health` |

### Dynamic Port Assignment

All host ports are dynamically assigned by `mdemg init`. There are no hardcoded defaults in the compose file — this prevents conflicts with existing Neo4j, PostgreSQL, or other services on the user's machine.

Port assignment flow:
1. `FindFreePort(preferred, rangeStart, rangeEnd)` tries the preferred port, then scans +100 range
2. In interactive mode, user can override each port (a port may appear free only because a normally-running container is stopped)
3. Assigned ports are written to `.env` which compose reads
4. Config.yaml is updated with host-mapped ports for CLI client discovery

### Multi-Instance Isolation

Each project gets its own `COMPOSE_PROJECT_NAME` (derived from directory name: `mdemg-{slug}`). Docker Compose auto-generates container names as `{project}-{service}-{replica}`. No `container_name` directives are used — hardcoded names would force the same name across instances, breaking isolation.

### Internal vs Host Networking

| Variable | Value | Used By |
|----------|-------|---------|
| `NEO4J_URI` (compose env) | `bolt://neo4j:7687` | mdemg container → neo4j container |
| `NEO4J_URI` (config.yaml) | `bolt://localhost:<bolt_port>` | CLI → host-mapped port |
| `TSDB_PORT` (compose env) | `5432` | mdemg container → timescaledb container |
| `TSDB_HOST_PORT` (.env) | Dynamic (preferred: 5433) | Host → timescaledb container |

### Process Supervision

Docker's `restart: unless-stopped` replaces launchd/systemd for all services. `depends_on` with `condition: service_healthy` ensures startup ordering (mdemg waits for neo4j + timescaledb).

### Docker Image CI

`.github/workflows/docker-publish.yml` builds multi-arch images (linux/amd64, linux/arm64) on:
- Release tag (`v*`) → `ghcr.io/reh3376/mdemg:v0.3.4`, `:latest`
- Push to `main` → `ghcr.io/reh3376/mdemg:main`

Uses Docker Buildx with GitHub Actions cache (`type=gha`) for layer reuse.

## Init Flow

`mdemg init` (Docker-first, no flag needed):

```
Check Docker available → Check resources → Scan 6 ports →
[Interactive: confirm ports] → Prompt service credentials →
Generate .env (ports + credentials) → Generate config.yaml →
Generate .mdemgignore → docker compose up -d → Health check loop
```

`mdemg init --quick` / `mdemg init --defaults` skips interactive prompts.

`mdemg init --native` falls back to legacy native deployment (dev-only).

## Configuration

### Required Environment Variables (.env)

| Variable | Description |
|----------|-------------|
| `COMPOSE_PROJECT_NAME` | Multi-instance isolation slug |
| `MDEMG_PORT` | Host port for MDEMG server |
| `NEO4J_BOLT_PORT` | Host port for Neo4j Bolt |
| `NEO4J_HTTP_PORT` | Host port for Neo4j HTTP |
| `TSDB_HOST_PORT` | Host port for TimescaleDB |
| `NEURAL_PORT` | Host port for neural sidecar |
| `GRAFANA_PORT` | Host port for Grafana |
| `GRAFANA_PASSWORD` | Grafana admin password (default: admin) |
| `TSDB_PASSWORD` | TimescaleDB password (default: mdemg_metrics) |

Port variables are dynamically assigned by `mdemg init`. Credential variables are prompted interactively (or use defaults with `--defaults`/`--quick`).

### Neo4j Memory Tiering

`mdemg init` detects system RAM and auto-configures Neo4j memory via `.env`:

| System RAM | `NEO4J_PAGECACHE` | `NEO4J_HEAP_INIT` | `NEO4J_HEAP_MAX` | `NEO4J_MAX_POOL_SIZE` |
|---|---|---|---|---|
| ≤16 GB | 512M | 256M | 512M | 50 |
| ≤32 GB | 1G | 512M | 1G | 75 |
| ≤64 GB | 2G | 512M | 2G | 100 |
| ≥65 GB | 2G | 1G | 2G | 100 |

The compose file defaults (`512M/256M/512M`) are conservative — safe for manual `docker compose up` without running `mdemg init`. Running `mdemg init` writes the tier-appropriate values to `.env`, overriding the compose defaults.

### Dockerfile

`deploy/docker/Dockerfile.prod`: 2-stage build (Go 1.26 builder → alpine:3.19 runtime). `LISTEN_PORT` env var configures the healthcheck URL for portability across compose configurations.

## TimescaleDB Configuration

TimescaleDB stores operational metrics, LLM interaction logs, embedding events, and retrieval pipeline traces. All TSDB data collection requires `TSDB_ENABLED=true`.

### Core Flags

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| Master switch | `TSDB_ENABLED` | `false` | Must be `true` for any data collection |
| Optional mode | `TSDB_OPTIONAL` | `true` | Server starts even if TSDB is unavailable |
| Auto-migrate | `TSDB_AUTO_MIGRATE` | `false` | Run schema migrations on startup (env var, not in Config struct) |
| Host | `TSDB_HOST` | `localhost` | TimescaleDB hostname (use `timescaledb` in Docker) |
| Port | `TSDB_PORT` | `5432` | TimescaleDB port (internal Docker network) |
| User | `TSDB_USER` | `mdemg` | Database user |
| Password | `TSDB_PASSWORD` | `mdemg_metrics` | Database password |
| Database | `TSDB_DATABASE` | `mdemg_metrics` | Database name |
| SSL mode | `TSDB_SSL_MODE` | `disable` | PostgreSQL SSL mode |
| Max conns | `TSDB_MAX_CONNS` | `10` | Connection pool size |
| Flush interval | `TSDB_FLUSH_INTERVAL_SEC` | `60` | Seconds between batch flushes to TSDB |
| Schema version | `TSDB_REQUIRED_SCHEMA_VERSION` | `31` | Required schema version (server refuses to start if behind) |

### Event Logging Flags

These control which event types are captured. All require `TSDB_ENABLED=true`.

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| LLM logging | `LLM_INTERACTION_LOGGING` | `true` | Log LLM calls (provider, model, latency, tokens, prompts) |
| Embedding logging | `EMBEDDING_EVENT_LOGGING` | `true` | Log embedding calls (ingest/query, call site, cache hit) |
| Retrieval logging | `RETRIEVAL_EVENT_LOGGING` | `true` | Log retrieval pipeline stages (recall, BM25, rerank, results) |

### Query Classification

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| Enabled | `QUERY_CLASSIFY_ENABLED` | `false` | Enable LLM query type classification |
| Provider | `QUERY_CLASSIFY_PROVIDER` | (LLM provider) | LLM provider for classification |
| Model | `QUERY_CLASSIFY_MODEL` | (LLM model) | Model for classification |
| Max tokens | `QUERY_CLASSIFY_MAX_TOKENS` | `500` | Max tokens for classification response |
| Timeout | `QUERY_CLASSIFY_TIMEOUT_MS` | `5000` | Timeout in ms |

### Data Retention

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| Raw retention | `TSDB_RAW_RETENTION_DAYS` | `90` | Days to keep raw metric samples |
| Hourly retention | `TSDB_HOURLY_RETENTION_DAYS` | `365` | Days to keep hourly aggregates |

### Backup

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| Backup enabled | `TSDB_BACKUP_ENABLED` | `false` | Enable TSDB backup scheduler |
| Storage dir | `TSDB_BACKUP_STORAGE_DIR` | `.mdemg/backups/tsdb` | Backup file location |
| Interval | `TSDB_BACKUP_INTERVAL_HOURS` | `24` | Hours between scheduled backups |
| Retention count | `TSDB_BACKUP_RETENTION_COUNT` | `14` | Number of backups to keep |
| Retention age | `TSDB_BACKUP_RETENTION_MAX_AGE_DAYS` | `90` | Maximum backup age in days |

### Dependency Chain

```
TSDB_ENABLED=true (master gate)
  ├── metric_samples writer (always active when TSDB connected)
  ├── LLM_INTERACTION_LOGGING=true → llm_interactions writer
  ├── EMBEDDING_EVENT_LOGGING=true → embedding_events writer
  ├── RETRIEVAL_EVENT_LOGGING=true → retrieval_events writer
  └── TSDB_BACKUP_ENABLED=true → backup scheduler
```

### Docker Compose Defaults

`docker-compose.yml` sets `TSDB_ENABLED=true`, `TSDB_AUTO_MIGRATE=true`, and `TSDB_OPTIONAL=true` for the mdemg service. This ensures Docker deployments collect data out of the box, auto-create the schema on first startup, and tolerate TimescaleDB being temporarily unavailable.

## Neo4j Edition

The compose uses `neo4j:5` (community edition). MDEMG is MIT-licensed; shipping enterprise edition would create licensing friction. APOC works with community. Enterprise features (backup, clustering) are documented as optional upgrade.

## Backup UI

The browser dashboard includes a Backup tab (9th tab) wrapping all 7 backup REST endpoints. Features:
- **Trigger Backup**: Space selector + type dropdown (full/partial_space)
- **Backup History**: Filterable table with status/type badges, delete button per row
- **Restore**: Dropdown of completed backups with confirmation dialog
- **Active Operations**: Live polling of in-progress backup/restore operations

Returns 503 when backup module is disabled (`BACKUP_ENABLED=false`).

## Documents Accessed

- `docker-compose.yml` — consolidated compose configuration
- `docker-compose.dev.yml` — dev overlay (neo4j-monitor)
- `deploy/docker/Dockerfile.prod` — production Docker image
- `deploy/docker/docker-compose.prod.yml` — original prod compose (reference)
- `.env.example` — environment variable template
- `internal/cli/init.go` — `mdemg init` implementation (credential prompts, Docker .env write)
- `internal/cli/docker.go` — `FindFreePort`, `DockerAvailable`, `CheckDockerResources`
- `internal/config/yaml_config.go` — `GenerateConfigYAML`, `InitOptions` (GrafanaPassword, TSDBPassword)
- `docs/user/quickstart-docker.md` — user-facing Docker guide
- `docs/user/install-guide.md` — platform-specific installation guide
