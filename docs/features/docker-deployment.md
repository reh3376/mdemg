# Docker Deployment

## Overview

Docker Compose is the only deployment path for MDEMG. All 5 services (mdemg server, Neo4j, TimescaleDB, neural-sidecar, Grafana) run as containers. `mdemg init` scans for free ports, generates `.env`, and starts the stack.

## Problem

MDEMG previously required 3 platform-specific installers (Homebrew, APT/Debian, Scoop/Windows) and 3 native companion apps (macOS menu bar, Linux sidebar, Windows tray). Each platform had its own process supervision (launchd, systemd, Windows Service), creating a 6-repo maintenance burden with cross-platform divergence.

## Architecture

### Compose Services

| Service | Image | Container Port | Health Check |
|---------|-------|----------------|--------------|
| mdemg | `ghcr.io/reh3376/mdemg:latest` | 9999 | `wget /healthz` |
| neo4j | `neo4j:5` (community) | 7687 (Bolt), 7474 (HTTP) | `cypher-shell RETURN 1` |
| timescaledb | `timescale/timescaledb:2.25.1-pg16` | 5432 | `pg_isready` |
| neural-sidecar | Local build (`./neural`) | 8000 | Python `urlopen /health` |
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
[Interactive: confirm ports] → Generate .env → Generate config.yaml →
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

All are dynamically assigned by `mdemg init` — no hardcoded defaults.

### Dockerfile

`deploy/docker/Dockerfile.prod`: 2-stage build (Go 1.26 builder → alpine:3.19 runtime). `LISTEN_PORT` env var configures the healthcheck URL for portability across compose configurations.

## Neo4j Edition

The compose uses `neo4j:5` (community edition). MDEMG is MIT-licensed; shipping enterprise edition would create licensing friction. APOC works with community. Enterprise features (backup, clustering) are documented as optional upgrade.

## Documents Accessed

- `docker-compose.yml` — consolidated compose configuration
- `docker-compose.dev.yml` — dev overlay (neo4j-monitor)
- `deploy/docker/Dockerfile.prod` — production Docker image
- `deploy/docker/docker-compose.prod.yml` — original prod compose (reference)
- `.env.example` — environment variable template
- `internal/cli/init.go` — `mdemg init` implementation
- `internal/cli/docker.go` — `FindFreePort`, `DockerAvailable`, `CheckDockerResources`
- `internal/config/yaml_config.go` — `GenerateConfigYAML`, `InitOptions`
- `docs/user/quickstart-docker.md` — user-facing Docker guide
