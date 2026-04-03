# MDEMG Install Guide

MDEMG runs as a Docker Compose stack. This guide covers installation on all supported platforms.

## Prerequisites (All Platforms)

- **Docker Desktop** (macOS/Windows) or **Docker Engine + Compose v2** (Linux)
- **4 GB RAM** minimum (8 GB recommended)
- **Embedding provider**: OpenAI API key (recommended) or Ollama (local, free)

Verify Docker is ready:

```bash
docker compose version   # Compose v2 required
docker info              # Check Docker daemon is running
```

---

## macOS

### Install via Homebrew

```bash
brew tap reh3376/mdemg
brew install mdemg
mdemg version
```

### Initialize

```bash
cd /path/to/your/project
mdemg init
```

Interactive mode prompts for:
- Neo4j URI and ports
- Service credentials (Neo4j, Grafana, TimescaleDB passwords)
- Embedding provider and API key

For non-interactive setup:

```bash
mdemg init --quick     # Defaults + auto-start
mdemg init --defaults  # Defaults without auto-start
```

### Verify

```bash
docker compose ps                    # All 5 services running
curl -s http://localhost:9999/healthz | python3 -m json.tool
open http://localhost:9999/ui/       # Browser dashboard
```

---

## Linux

### Install via curl

```bash
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
mdemg version
```

### Initialize

```bash
cd /path/to/your/project
mdemg init
```

Same interactive prompts as macOS. Use `mdemg init --quick` for non-interactive.

### Verify

```bash
docker compose ps
curl -s http://localhost:9999/healthz | python3 -m json.tool
xdg-open http://localhost:9999/ui/   # Browser dashboard
```

---

## Windows (WSL2)

MDEMG requires WSL2 on Windows. Native Windows is not supported.

### Step 1: Install WSL2

```powershell
# In PowerShell (Admin)
wsl --install
```

Restart your machine, then open the WSL terminal (Ubuntu).

### Step 2: Install Docker Desktop

Install [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/) with WSL2 backend enabled (Settings > General > Use the WSL 2 based engine).

Verify inside WSL:

```bash
docker compose version
```

### Step 3: Install MDEMG

```bash
# Inside WSL terminal
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
mdemg version
```

### Step 4: Initialize

```bash
cd /path/to/your/project
mdemg init
```

### Verify

```bash
docker compose ps
curl -s http://localhost:9999/healthz | python3 -m json.tool
# Open http://localhost:9999/ui/ in your Windows browser
```

---

## Service URLs & Default Credentials

After `mdemg init`, services are available at dynamically assigned ports. The post-install summary shows the exact URLs.

| Service | Default Port | URL | Credentials |
|---------|-------------|-----|-------------|
| MDEMG Dashboard | 9999 | `http://localhost:9999/ui/` | None |
| MDEMG API | 9999 | `http://localhost:9999` | None |
| Grafana | 3000 | `http://localhost:3000` | admin / admin |
| Neo4j Browser | 7474 | `http://localhost:7474` | neo4j / testpassword |
| Neo4j Bolt | 7687 | `bolt://localhost:7687` | neo4j / testpassword |
| TimescaleDB | 5433 | `localhost:5433` | mdemg / mdemg_metrics |

> **Note**: Ports are dynamically scanned by `mdemg init`. If default ports are busy, the next available port is used. Check your `.env` file for actual port assignments.

Credentials can be customized during interactive `mdemg init`. Defaults are used with `--quick`/`--defaults`.

---

## Updating

### Self-Update (v0.4.0+)

```bash
mdemg upgrade            # Update to latest stable release
mdemg upgrade --edge     # Update to latest edge build (main branch)
mdemg upgrade --dry-run  # Check for updates without installing
```

The `upgrade` command downloads the new binary, verifies its SHA-256 checksum, and replaces the current executable. If a `./bin/` directory exists in the current working directory, the updated binary is also copied there.

### Edge Channel

Edge builds are published on every merge to main. They include the latest features and fixes before a stable release is tagged.

**Install edge via curl:**

```bash
CHANNEL=edge curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
```

**Update to edge:**

```bash
mdemg upgrade --edge
```

Edge binaries include the commit hash in their version string (e.g., `edge-abc1234`).

---

## Build from Source

```bash
git clone https://github.com/reh3376/mdemg.git && cd mdemg
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg version
```

Requires Go 1.26+.

---

## Troubleshooting

### Docker not running

```
Error: docker compose up failed
```

Ensure Docker Desktop (macOS/Windows) or Docker Engine (Linux) is running. On WSL2, ensure Docker Desktop's WSL integration is enabled.

### Port already in use

`mdemg init` automatically scans for free ports. If you see a port conflict after init, check `.env` for port assignments and ensure no other service is using those ports:

```bash
lsof -i :9999   # Check what's using a port
```

### WSL2: Docker command not found

Ensure Docker Desktop's "Use the WSL 2 based engine" is enabled and WSL integration is turned on for your distro (Settings > Resources > WSL Integration).

### Embedding provider errors

- **OpenAI**: Verify `OPENAI_API_KEY` is set in `.env`
- **Ollama**: Ensure Ollama is running (`ollama serve`) and the model is pulled (`ollama pull qwen3-embedding:8b`)

---

## Uninstall / Full Purge

### macOS (Homebrew)

```bash
# Stop services
mdemg service uninstall          # Remove LaunchAgents
docker compose down -v           # Stop containers, delete volumes

# Remove binary
brew uninstall mdemg

# Remove data (optional — destroys all stored memories)
rm -rf ~/.mdemg                  # Config, backups, exports, logs
rm -rf .mdemg                    # Project-level config
rm .env                          # Secrets (API keys, passwords)
rm docker-compose.yml            # Compose file (if written by mdemg init)
rm .mdemgignore                  # Ignore file

# Remove Docker volumes (if not removed by docker compose down -v)
docker volume rm mdemg_neo4j_data mdemg_neo4j_logs mdemg_tsdb_data mdemg_grafana_data 2>/dev/null
```

### Linux

```bash
# Stop services
docker compose down -v
sudo rm /usr/local/bin/mdemg     # Remove binary

# Same data cleanup as macOS above
rm -rf ~/.mdemg .mdemg .env docker-compose.yml .mdemgignore
```

---

## Next Steps

- [Quick Start Guide](quickstart-docker.md) — condensed setup walkthrough
- [Docker Deployment](../features/docker-deployment.md) — architecture and configuration details
- [Browser Dashboard](../features/browser-ui.md) — dashboard tab reference
