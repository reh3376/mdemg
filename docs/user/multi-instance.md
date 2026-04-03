# Multi-Instance Deployment

Run multiple MDEMG instances on the same machine — one per project.

## How Isolation Works

Each `mdemg init` creates a fully isolated Docker Compose stack:

| Resource | Isolation Method |
|----------|-----------------|
| Containers | `COMPOSE_PROJECT_NAME=mdemg-{directory}` prefixes all container names |
| Volumes | Prefixed by project name (`mdemg-myproject_neo4j_data`) |
| Networks | Prefixed by project name (`mdemg-myproject_mdemg-net`) |
| Ports | `FindFreePort` scans for available ports, no hardcoded defaults |
| Neo4j data | Separate containers with separate volumes |
| TimescaleDB data | Separate containers with separate volumes |
| Config | Per-project `.mdemg/config.yaml` and `.env` |

## Setup

### First Instance (existing)

```bash
cd /path/to/project-one
mdemg init    # Gets ports 9999, 7687, 7474, 5433, 8100, 3000
```

### Second Instance

```bash
cd /path/to/project-two
mdemg init    # Detects port conflicts, assigns 10000, 7688, 7475, 5434, 8101, 3001
```

Port allocation scans the preferred port, then +1 through +100. Interactive mode lets you override.

### Verify

```bash
# Check all instances
curl -s http://localhost:9999/healthz   # Instance 1
curl -s http://localhost:10000/healthz  # Instance 2

# View all MDEMG containers
docker ps --format "table {{.Names}}\t{{.Status}}" | grep mdemg
```

## Resource Requirements

Each instance runs 5 Docker containers. Measured on Apple Silicon with 128 GB RAM:

| Component | Fresh Instance | Mature Instance (30k+ nodes) |
|-----------|---------------|------------------------------|
| Neo4j | 1.5 GiB | 4+ GiB |
| Neural sidecar | 555 MiB | 555 MiB |
| TimescaleDB | 215 MiB | 300 MiB |
| Grafana | 107 MiB | 150 MiB |
| MDEMG server | 9 MiB | 500 MiB |
| **Total** | **~2.3 GiB** | **~5.5 GiB** |

### Recommended Hardware

| Instances | Min RAM | Recommended |
|-----------|---------|-------------|
| 1 | 8 GB | 16 GB |
| 2 | 16 GB | 32 GB |
| 3 | 32 GB | 64 GB |
| 4+ | 64 GB+ | 128 GB |

CPU overhead is minimal (~1% idle per instance).

## Managing Instances

### Start/Stop Individual Instances

```bash
cd /path/to/project-two
docker compose up -d      # Start this instance
docker compose down       # Stop this instance (data preserved)
docker compose down -v    # Stop and delete data volumes
```

Stopping one instance does not affect others.

### View All Instances

```bash
docker ps --format "table {{.Names}}\t{{.Ports}}" | grep mdemg
```

### Check Port Assignments

```bash
grep PORT /path/to/project-two/.env
```

## Known Limitations

### LaunchAgent Labels (macOS only)

`mdemg service install` uses hardcoded labels (`com.mdemg.server`, etc.). Installing from a second project directory **overwrites** the first instance's LaunchAgent configuration.

**Recommendation:** Only use `mdemg service install` for your primary instance. All instances automatically restart via Docker Compose's `restart: unless-stopped` policy — LaunchAgents are not needed for process supervision.

### Teardown

`mdemg teardown` does not currently stop Docker Compose services. Before running teardown, stop the compose stack manually:

```bash
cd /path/to/project
docker compose down -v    # Stop containers, remove volumes
mdemg teardown --yes      # Clean up config files
```

### Log Paths

Docker container logs are fully isolated per instance. LaunchAgent-managed services (if used) share `~/.mdemg/logs/` — this has no impact when using Docker Compose directly.

### Port Range

Each instance scans up to 100 ports from the preferred port. With 6 ports per instance and default preferred ports spread across different ranges (3000, 5433, 7474, 7687, 8100, 9999), practical limit is ~16 instances before manual port assignment is needed.
