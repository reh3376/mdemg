# MDEMG Beta Testing Guide

**Version under test:** v0.10.1 (CLI) — update this marker each beta cycle
**Date:** _______________
**Tester:** _______________
**Machine specs:** _______________
**OS:** _______________ (macOS / Windows / Linux distro)
**Docker Desktop version:** _______________

---

## Results Summary

| Tier | Section | Tests | Pass | Fail | Skip | Notes |
|------|---------|-------|------|------|------|-------|
| 1 | Installation & Core | 9 | | | | |
| 2 | Ingestion | 9 | | | | |
| 3 | CMS & RSIC | 10 | | | | |
| 4 | Backup & Maintenance | 5 | | | | |
| 5 | Advanced | 9 | | | | |
| **Total** | | **42** | | | | |

---

## What is MDEMG?

MDEMG (Multi-Dimensional Emergent Memory Graph) is a **persistent memory system for AI coding assistants** like Claude Code, Cursor, and GitHub Copilot. Without MDEMG, these AI tools forget everything between sessions. With MDEMG, they remember your codebase, decisions, corrections, and preferences across every conversation.

### How It Works

1. **You code with an AI assistant** — MDEMG runs quietly in the background
2. **Observations are captured** — decisions, corrections, patterns in your code
3. **A knowledge graph grows** — Neo4j stores observations with semantic connections
4. **Higher-level concepts emerge** — MDEMG clusters similar knowledge and strengthens frequently co-activated connections (Hebbian learning)
5. **Your AI assistant gets smarter** — next session, it recalls relevant past context

### Key Concepts

| Concept | What It Means |
|---------|---------------|
| **Space** | An isolated knowledge graph. Each project gets its own space. |
| **Observation** | A unit of knowledge — a decision, correction, error, preference, or learning. |
| **CMS** (Conversation Memory System) | Captures, stores, and retrieves observations from AI sessions. |
| **RSIC** (Reflective Self-Improvement Cycle) | Automated loop that analyzes the knowledge graph, identifies gaps, and optimizes retrieval. |
| **Consolidation** | Clustering similar observations into higher-level "concept" nodes. |
| **Hebbian Learning** | "Neurons that fire together wire together" — co-accessed observations get linked. |
| **Jiminy** | Inner-voice guidance that proactively surfaces relevant context and warnings. |
| **Ingest** | Feeding data into MDEMG — code files, git history, API docs, etc. |
| **Recall** | Querying MDEMG to retrieve relevant past knowledge via semantic search. |
| **MCP** (Model Context Protocol) | Standard for AI tools to communicate with external systems. MDEMG runs as an MCP server. |

---

## Prerequisites

Complete each section below in order. Do not assume anything is pre-installed — verify each item.

### Step 1: Verify Operating System

MDEMG runs on macOS, Windows, and Linux via Docker.

**macOS:**
```bash
sw_vers
# ProductVersion must be 12.0 (Monterey) or higher
```

**Windows (PowerShell):**
```powershell
[System.Environment]::OSVersion.Version
# Must be 10.0.19044 or higher (Windows 10 21H2+)
```

**Linux:**
```bash
cat /etc/os-release
uname -m
# Supported: Ubuntu 20.04+, Debian 11+, Fedora 38+, RHEL 8+, Arch
# Architecture: x86_64 (amd64) or aarch64 (arm64)
```

- [ ] OS version verified: _______________

### Step 2: Install Docker Desktop

Docker Desktop is the **only runtime prerequisite** for MDEMG. All services (server, database, metrics, dashboards) run as Docker containers.

**macOS:**
```bash
# Method A — Homebrew (recommended):
brew install --cask docker

# Method B — direct download from https://www.docker.com/products/docker-desktop/
```

**Windows:**
```powershell
# Requires WSL 2 — install if needed:
wsl --install

# Then install Docker Desktop:
winget install Docker.DockerDesktop
# Or download from https://www.docker.com/products/docker-desktop/
```

> **Windows note:** Docker Desktop requires hardware virtualization (Intel VT-x / AMD-V) enabled in BIOS and WSL 2. See [Docker Desktop Windows requirements](https://docs.docker.com/desktop/setup/install/windows-install/) for details.

**Linux:**
```bash
# Install Docker Engine + Compose plugin
# See https://docs.docker.com/engine/install/ for your distro

# Verify after install:
docker compose version
```

After installation, launch Docker Desktop (macOS/Windows) and verify:

```bash
docker info
# Should show "Server: Docker Desktop" (or Docker Engine on Linux)

docker compose version
# Must be Compose v2+
```

> **Resource check:** MDEMG requires at least 4 GB RAM and 2 CPUs allocated to Docker. On Docker Desktop: Settings > Resources > set Memory ≥ 4 GB.

- [ ] Docker Desktop/Engine installed and running, version: _______________
- [ ] Compose v2+ available

### Step 3: Install MDEMG CLI

**macOS (Homebrew):**
```bash
brew tap reh3376/mdemg
brew install mdemg
```

**Windows (PowerShell 7+):**
```powershell
# Requires PowerShell 7+ (pwsh). Install if needed:
winget install Microsoft.PowerShell

# Then install MDEMG:
irm https://raw.githubusercontent.com/reh3376/mdemg-windows/main/install.ps1 | iex
```

**Linux (APT — Debian/Ubuntu):**
```bash
curl -fsSL https://reh3376.github.io/apt-mdemg/pubkey.gpg | sudo gpg --dearmor -o /usr/share/keyrings/mdemg.gpg
echo "deb [signed-by=/usr/share/keyrings/mdemg.gpg] https://reh3376.github.io/apt-mdemg stable main" | sudo tee /etc/apt/sources.list.d/mdemg.list
sudo apt update && sudo apt install mdemg
```

**Linux (direct binary):**
```bash
# Download from GitHub Releases
curl -fsSL -o /tmp/mdemg "https://github.com/reh3376/mdemg/releases/latest/download/mdemg-linux-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
chmod +x /tmp/mdemg && sudo mv /tmp/mdemg /usr/local/bin/
```

Verify:
```bash
mdemg version
```

- [ ] MDEMG CLI installed, version: _______________

### Step 4: Internet Access

Required to pull Docker images (~1 GB total on first run) and optionally connect to OpenAI API.

```bash
curl -s https://api.github.com/repos/reh3376/mdemg/releases/latest | grep tag_name
```

- [ ] Internet access confirmed

### Optional Prerequisites

#### OpenAI API Key (Tier 2-3: recall, consolidation, memory retrieval)

Required for embedding-powered features. Without a key, these features run in degraded mode.

1. Sign up at [platform.openai.com](https://platform.openai.com)
2. Create an API key at [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
3. Save the key — you'll configure it during `mdemg init`

- [ ] OpenAI API key obtained (or will skip embedding tests)

#### Ollama (Alternative to OpenAI)

Local-only alternative — no API key or internet required after initial download.

**macOS:** `brew install ollama`
**Windows:** Download from [ollama.com/download](https://ollama.com/download)
**Linux:** `curl -fsSL https://ollama.ai/install.sh | sh`

```bash
# Pull the recommended embedding model
ollama pull qwen3-embedding:8b

# Verify
ollama list
```

> **Dimension note:** MDEMG requires 3072-dimension embeddings. OpenAI `text-embedding-3-large` produces 3072 natively. For Ollama, `qwen3-embedding:8b` (4096 native) is automatically truncated to 3072. Run `mdemg embeddings check` after setup to verify.

- [ ] Ollama installed (or using OpenAI, or will skip embedding tests)

#### Git (Tier 2: hooks, incremental ingest)

Required for git hooks, incremental ingest (`--since`), and test project setup.

```bash
git --version
```

- [ ] Git installed, version: _______________
- [ ] **SKIP** — will skip git-dependent tests (T2.4, T2.5)

### Set Up Test Project

```bash
mkdir -p ~/mdemg-test && cd ~/mdemg-test

# With Git:
git init
git config user.email "tester@example.com"
git config user.name "Beta Tester"

cat > main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello from MDEMG beta test")
}
EOF

git add . && git commit -m "initial commit"
```

> **Without Git:** Skip the `git init/add/commit` lines. Create the directory and file manually. You will need to skip tests T2.4 and T2.5.

- [ ] Test project directory created at `~/mdemg-test`

### Prerequisites Checklist Summary

| # | Requirement | Status | Notes |
|---|-------------|--------|-------|
| 1 | OS supported | | macOS 12+, Windows 10 21H2+, or Linux (amd64/arm64) |
| 2 | Docker Desktop/Engine + Compose v2 | | `docker compose version` succeeds |
| 3 | MDEMG CLI installed | | `mdemg version` returns version info |
| 4 | Internet access | | For pulling Docker images and API access |
| — | *OpenAI API key (optional)* | | Enables semantic recall, consolidation naming |
| — | *Ollama (optional)* | | Local embedding alternative |
| — | *Git (optional)* | | For hooks and incremental ingest tests |
| — | Test project created | | `test -d ~/mdemg-test && echo OK` |

---

## Reference Documentation

| Guide | What it covers |
|-------|---------------|
| [Docker Deployment Guide](quickstart-docker.md) | Docker Compose setup, dynamic ports, multi-instance |
| [CLI Reference](cli-reference.md) | All commands, flags, defaults, environment variables |
| [API Reference](api-reference.md) | Every HTTP endpoint with request/response shapes and curl examples |
| [CMS & RSIC Guide](cms-rsic-guide.md) | Conversation memory, Jiminy, observation types, self-improvement cycles |
| [Ingestion Guide](ingestion-guide.md) | All 8 ingestion methods — codebase, scraper, Linear, webhooks, file watcher, API |

---

## Tier 1: Installation & Core (~30 min)

### T1.1: Installation

Verify the CLI binary installed in Step 3 above is working.

```bash
which mdemg    # or: where.exe mdemg (Windows)
mdemg version
```

**Expected output:**

```
mdemg v0.3.x
  commit:  <short-hash>
  built:   <date>
  go:      go1.26.x
  os/arch: <your-os>/<your-arch>
```

- [ ] **PASS** — `mdemg version` displays correct OS/arch

---

### T1.2: Initialize Project (Docker)

```bash
cd ~/mdemg-test
mdemg init
```

**Expected:** Interactive wizard that:
1. Checks Docker is running and has adequate resources
2. Prompts for OpenAI API key (if not in env)
3. Scans for 6 free TCP ports (MDEMG, Neo4j Bolt, Neo4j HTTP, TimescaleDB, Neural, Grafana)
4. Presents assigned ports for confirmation
5. Generates `.env` with port assignments
6. Generates `.mdemg/config.yaml`
7. Runs `docker compose up -d`
8. Waits for health check

> **Important:** All TCP ports are **dynamically assigned**. `mdemg init` scans for free ports — no hardcoded defaults. If a preferred port (e.g., 7687) is in use, it automatically finds an alternative.

```bash
# Verify files exist
ls -la .mdemg/config.yaml .env
cat .env | grep _PORT
```

- [ ] **PASS** — init completes, `.env` and `.mdemg/config.yaml` created, ports assigned

**For non-interactive setup (CI or repeat installs):**
```bash
mdemg init --quick
```

---

### T1.3: Docker Services Running

```bash
docker compose ps
```

**Expected:** 5 services running: `mdemg`, `neo4j`, `timescaledb`, `neural-sidecar`, `grafana`.

```bash
# Check each service
docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
```

- [ ] **PASS** — all 5 services show as running with assigned ports

---

### T1.4: Health Checks

```bash
# Read your assigned MDEMG port from .env
source .env 2>/dev/null  # bash/zsh
# Or manually check: grep MDEMG_PORT .env

# Health check
curl -s http://localhost:${MDEMG_PORT}/healthz

# Readiness check
curl -s http://localhost:${MDEMG_PORT}/readyz
```

**Expected:** Both return `{"status":"ok"}` (or similar JSON with healthy status).

- [ ] **PASS** — both endpoints respond with OK status
- [ ] **Port used:** _______________

> **Note:** Throughout the remaining tests, replace `9999` with your actual `MDEMG_PORT` from `.env`. The guide uses `${MDEMG_PORT}` as a placeholder.

---

### T1.5: Database Migrations

```bash
mdemg db migrate
```

**Expected:** Migrations apply without errors. Output shows "applied N migrations" or "already up to date."

> **Note:** If you used `mdemg init`, migrations run automatically via `--auto-migrate`. This test verifies the explicit command works.

- [ ] **PASS** — migrations complete successfully

---

### T1.6: Configuration Display & Validation

```bash
mdemg config show
mdemg config validate
```

**Expected:** `config show` displays effective configuration with source annotations (yaml/env/default). `config validate` probes Neo4j connectivity and reports results.

- [ ] **PASS** — config show displays settings, validate confirms Neo4j reachable

---

### T1.7: Embedding Provider Check

```bash
mdemg embeddings check
```

**Expected (with OpenAI key configured):** Reports embedding provider, model, and dimension count (3072 for text-embedding-3-large).

**Expected (without key):** Reports "no embedding provider configured" or similar warning. Acceptable — note in results.

- [ ] **PASS** — embedding check runs and reports status
- [ ] **SKIP** — no embedding provider configured

---

### T1.8: Service Restart

```bash
# Stop all services (data preserved)
docker compose down

# Start again
docker compose up -d

# Verify health
curl -s http://localhost:${MDEMG_PORT}/healthz
```

**Expected:** Services stop cleanly and restart. Health check passes after restart (~30s for Neo4j to become ready).

- [ ] **PASS** — stop/start cycle works, health check passes after restart

---

### T1.9: Service Logs

```bash
# All logs
docker compose logs --tail=20

# MDEMG server logs only
docker compose logs -f mdemg
# Press Ctrl+C to stop following
```

**Expected:** Logs display without errors. Server log shows startup messages, health check registrations.

- [ ] **PASS** — logs accessible, no unexpected errors

---

## Tier 2: Ingestion (~20 min)

> **Reference:** [Ingestion Guide](ingestion-guide.md) covers all 8 ingestion methods. [API Reference](api-reference.md#codebase-ingestion-api) has full endpoint documentation.

### T2.1: Codebase Ingestion (CLI)

```bash
cd ~/mdemg-test
mdemg ingest --path . --space-id beta-test
```

**Expected:** Ingests files from the test project. Output shows files processed, observations created.

- [ ] **PASS** — ingest completes, shows file count and observations

---

### T2.2: Single Observation (API)

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "session_id": "beta-session",
    "content": "This is a test observation from beta testing",
    "obs_type": "learning"
  }'
```

**Expected:** Returns JSON with `node_id` and `status` fields.

- [ ] **PASS** — observation created, node_id returned

---

### T2.3: Batch Ingest (API)

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/memory/ingest/batch \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "observations": [
      {"content": "Batch test item 1", "obs_type": "learning", "session_id": "beta-session"},
      {"content": "Batch test item 2", "obs_type": "learning", "session_id": "beta-session"}
    ]
  }'
```

**Expected:** Returns JSON with count of ingested nodes.

- [ ] **PASS** — batch ingest returns success with node count

---

### T2.4: Incremental Ingest

> **Requires:** Git

```bash
# Modify test file
echo "// Updated for incremental test" >> main.go
git add . && git commit -m "incremental test change"

# Incremental ingest
mdemg ingest --path . --space-id beta-test --incremental --since HEAD~1
```

**Expected:** Only the modified file is re-ingested.

- [ ] **PASS** — incremental ingest processes only changed files
- [ ] **SKIP** — Git not installed

---

### T2.5: Git Hooks

> **Requires:** Git

```bash
# Install hooks
mdemg hooks install --space-id beta-test

# Verify
mdemg hooks list

# Make a commit — hook should trigger auto-ingest
echo "// Hook trigger test" >> main.go
git add . && git commit -m "hook test"
```

**Expected:** `hooks list` shows post-commit hook installed. After commit, hook triggers background ingest.

- [ ] **PASS** — hooks install, list shows installed, commit triggers ingest
- [ ] **SKIP** — Git not installed

---

### T2.6: File Watcher

Open a **second terminal:**

```bash
cd ~/mdemg-test
mdemg watch --path . --space-id beta-test
```

In the **original terminal:**

```bash
echo "// New file for watcher test" > watcher_test.go
```

**Expected:** The watcher terminal shows the new file was detected and ingested.

Press `Ctrl+C` in the watcher terminal when done.

- [ ] **PASS** — watcher detects file creation and ingests it

---

### T2.7: Web Scraper

> **Skip** if no target URL is available.

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "url": "https://example.com",
    "max_pages": 1
  }'
```

**Expected:** Returns a job ID. Check status with `GET /v1/scraper/jobs/{job_id}`.

- [ ] **PASS** — scraper job created
- [ ] **SKIP** — no URL configured

---

### T2.8: Linear Integration

> **Skip** if no `LINEAR_API_KEY` is configured.

```bash
curl -s http://localhost:${MDEMG_PORT}/v1/linear/issues?space_id=beta-test
```

**Expected:** Returns issues list or empty array.

- [ ] **PASS** — Linear endpoint responds
- [ ] **SKIP** — no LINEAR_API_KEY configured

---

### T2.9: Speed Presets

**T2.9.1: Fast Preset Dry-Run**
```bash
mdemg ingest --path . --speed fast --dry-run
```
Expected: Workers=8, batch=250, LLM summaries disabled, symbol extraction disabled.

**T2.9.2: Thorough Preset Dry-Run**
```bash
mdemg ingest --path . --speed thorough --dry-run
```
Expected: Workers=8, batch=200, LLM summaries enabled, batch=20, symbols enabled.

**T2.9.3: Flag Override**
```bash
mdemg ingest --path . --speed fast --llm-summary=true --dry-run
```
Expected: Fast settings BUT LLM summaries still enabled (flag override takes precedence).

**T2.9.4: Combined Presets**
```bash
mdemg ingest --path . --speed fast --preset ml_cuda --dry-run
```
Expected: Speed preset (workers, batch, LLM) + exclusion preset (ml_cuda dirs/patterns) both applied.

- [ ] **PASS** — all 4 speed preset tests show correct settings

---

## Tier 3: CMS & RSIC (~20 min)

> **Reference:** [CMS & RSIC Guide](cms-rsic-guide.md) explains the full workflow. [API Reference](api-reference.md#conversation-memory) has all endpoint shapes.

### T3.1: Observe (Multiple Types)

```bash
# Decision observation
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "session_id": "beta-session",
    "content": "Decided to use Docker for all MDEMG deployments",
    "obs_type": "decision"
  }'

# Error observation
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "session_id": "beta-session",
    "content": "Build failed: missing dependency xyz",
    "obs_type": "error"
  }'
```

**Expected:** Both return JSON with `node_id`.

- [ ] **PASS** — multiple obs_types accepted (decision, error)

---

### T3.2: Resume Session

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "session_id": "beta-session",
    "max_observations": 10
  }'
```

**Expected:** Returns previously observed content from the session.

- [ ] **PASS** — resume returns prior observations

---

### T3.3: Recall (Semantic Query)

> **Requires:** Embedding provider (OpenAI or Ollama)

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "query": "What decisions were made during testing?",
    "top_k": 5
  }'
```

**Expected:** Returns relevant observations ranked by semantic similarity.

- [ ] **PASS** — recall returns relevant results
- [ ] **SKIP** — no embedding provider (degraded mode)

---

### T3.4: Correct

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/conversation/correct \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "session_id": "beta-session",
    "incorrect": "dependency xyz is version 1.0",
    "correct": "dependency xyz is actually version 2.0"
  }'
```

**Expected:** Returns JSON confirming the correction was recorded.

- [ ] **PASS** — correction accepted and stored

---

### T3.5: Consolidation

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id": "beta-test"}'
```

**Expected:** Returns consolidation results (hidden nodes created, edges formed). Without an LLM key, concept naming may be degraded but consolidation still runs.

- [ ] **PASS** — consolidation completes

---

### T3.6: Session Health

```bash
curl -s "http://localhost:${MDEMG_PORT}/v1/conversation/session/health?space_id=beta-test&session_id=beta-session"
```

**Expected:** Returns health metrics for the session (observation count, freshness, etc.).

- [ ] **PASS** — session health returned with metrics

---

### T3.7: RSIC Assess

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/self-improve/assess \
  -H "Content-Type: application/json" \
  -d '{"space_id": "beta-test"}'
```

**Expected:** Returns assessment with scores and recommendations.

- [ ] **PASS** — assessment returned

---

### T3.8: RSIC Cycle (Dry Run)

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{"space_id": "beta-test", "dry_run": true}'
```

**Expected:** Returns what the self-improvement cycle *would* do, without making changes.

- [ ] **PASS** — dry run cycle returns plan

---

### T3.9: RSIC Health

```bash
curl -s "http://localhost:${MDEMG_PORT}/v1/self-improve/health?space_id=beta-test"
```

**Expected:** Returns RSIC health metrics.

- [ ] **PASS** — RSIC health returned

---

### T3.10: Learning Freeze / Unfreeze

```bash
# Freeze
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/learning/freeze \
  -H "Content-Type: application/json" \
  -d '{"space_id": "beta-test", "reason": "beta testing", "frozen_by": "tester"}'

# Check status
curl -s "http://localhost:${MDEMG_PORT}/v1/learning/freeze/status?space_id=beta-test"

# Unfreeze
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/learning/unfreeze \
  -H "Content-Type: application/json" \
  -d '{"space_id": "beta-test"}'
```

**Expected:** Freeze returns confirmation, status shows frozen=true, unfreeze returns confirmation.

- [ ] **PASS** — freeze/status/unfreeze cycle completes

---

## Tier 4: Backup & Maintenance (~10 min)

### T4.1: Backup Trigger

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type": "full", "space_ids": ["beta-test"]}'
```

**Expected:** Returns backup job ID or confirmation.

- [ ] **PASS** — backup triggered

---

### T4.2: Backup List

```bash
curl -s "http://localhost:${MDEMG_PORT}/v1/backup/list?space_id=beta-test"
```

**Expected:** Returns list of backups (may include the one just created).

- [ ] **PASS** — backup list returned

---

### T4.3: Decay (Dry Run)

```bash
mdemg decay --space-id beta-test --dry-run
```

**Expected:** Shows what edges would be decayed without making changes.

- [ ] **PASS** — decay dry run shows results

---

### T4.4: Prune (Dry Run)

```bash
mdemg prune --space-id beta-test --dry-run
```

**Expected:** Shows what edges/nodes would be pruned without making changes.

- [ ] **PASS** — prune dry run shows results

---

### T4.5: Space List

```bash
mdemg space list
```

**Expected:** Lists all spaces including `beta-test`.

- [ ] **PASS** — space list shows beta-test

---

## Tier 5: Advanced (~15 min)

> **Reference:** [CLI Reference](cli-reference.md) has full flag details. [API Reference](api-reference.md#mcp-server-tools) covers MCP server tools.

### T5.1: Secrets

```bash
# Store a test secret
mdemg config set-secret TEST_BETA_KEY "beta-test-value-12345"

# Retrieve it
mdemg config get-secret TEST_BETA_KEY

# List all secrets
mdemg config list-secrets
```

**Expected:** Secret is stored in the OS credential store (macOS Keychain, Windows Credential Manager, or Linux keyring), retrieved correctly, and listed.

- [ ] **PASS** — set/get/list secrets works

---

### T5.2: Memory Retrieval

```bash
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "beta-test",
    "query_text": "beta testing",
    "top_k": 5
  }'
```

**Expected:** Returns retrieved memory nodes.

- [ ] **PASS** — memory retrieval returns results
- [ ] **SKIP** — no embedding provider

---

### T5.3: Demo

```bash
mdemg demo
```

**Expected:** Interactive demo runs, shows MDEMG capabilities. Follow on-screen prompts.

- [ ] **PASS** — demo runs to completion

---

### T5.4: Extract Symbols

```bash
mdemg extract-symbols --path .
```

**Expected:** Extracts code symbols (functions, types, etc.) from files in the directory.

- [ ] **PASS** — symbols extracted and listed

---

### T5.5: Consolidation (CLI)

```bash
mdemg consolidate --space-id beta-test --hidden-layer --dry-run
```

**Expected:** Shows consolidation plan without executing.

- [ ] **PASS** — consolidation dry run shows plan

---

### T5.6: MCP Server

```bash
mdemg mcp
```

**Expected:** MCP server starts and listens for JSON-RPC input on stdin. Press `Ctrl+C` to exit.

- [ ] **PASS** — MCP server starts, responds to Ctrl+C

---

### T5.7: Upgrade Check

```bash
mdemg upgrade --dry-run
```

**Expected:** Reports current version and latest available version.

- [ ] **PASS** — upgrade check runs and reports version information

---

### T5.8: Space Export/Import (API)

```bash
# Preview what would be exported
curl -s "http://localhost:${MDEMG_PORT}/v1/admin/spaces/export/preview?space_id=beta-test&profile=full"

# Export the space
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/admin/spaces/export \
  -H "Content-Type: application/json" \
  -d '{"space_id":"beta-test","profile":"metadata"}' > /tmp/beta-export.json

# Verify export has chunks
cat /tmp/beta-export.json | python3 -m json.tool | head -20

# Import to a new space (empty chunks for validation)
curl -s -X POST http://localhost:${MDEMG_PORT}/v1/admin/spaces/import \
  -H "Content-Type: application/json" \
  -d '{"space_id":"beta-test-import","conflict":"skip","chunks":[]}'
```

**Expected:**
- Preview returns `estimated_nodes`, `profile`, and `filters_applied`
- Export returns JSON with `header.format: "mdemg-space-transfer"`, `chunks` array, and `summary`
- Import returns `nodes_created: 0` (empty chunks), `warnings: []`

```bash
# CLI export/import (alternative)
mdemg space export --space-id beta-test --output /tmp/beta-test.mdemg --profile metadata
mdemg space import --input /tmp/beta-test.mdemg --target-space beta-test-cli-import
```

- [ ] **PASS** — API export preview returns valid JSON
- [ ] **PASS** — API export returns chunks with `mdemg-space-transfer` format
- [ ] **PASS** — API import accepts empty chunks and returns 200
- [ ] **PASS** — CLI export creates `.mdemg` file
- [ ] **PASS** — CLI import succeeds with target space

---

### T5.9: Teardown Dry Run (CLI)

```bash
cd ~/mdemg-test
mdemg teardown --dry-run
```

**Expected:** Lists all artifacts that would be removed (Docker containers, volumes, hooks, MCP configs, `.mdemg/` directory) without making any changes.

- [ ] **PASS** — dry run lists artifacts without making changes

---

## Grafana Dashboards (Optional)

If Grafana is running (check with `docker compose ps`), access the dashboards:

```bash
# Open in browser (use your GRAFANA_PORT from .env)
echo "http://localhost:${GRAFANA_PORT}"
```

Default credentials: **admin / admin**

Pre-provisioned dashboards:
- **MDEMG Overview** — request rates, latencies, error rates
- **Neo4j** — query performance, memory usage, connection pools
- **Graph Topology** — node/edge counts, layer distribution
- **Jiminy** — guidance synthesis metrics
- **RSIC** — self-improvement cycle metrics

- [ ] **PASS** — Grafana accessible, dashboards load

---

## Cleanup / Teardown

### Recommended: `mdemg teardown`

```bash
cd ~/mdemg-test
mdemg teardown --yes
```

This handles: stopping Docker services, removing containers/volumes, uninstalling hooks, cleaning MCP/IDE configs, backing up and removing `.mdemg/`.

### Manual cleanup (fallback)

```bash
cd ~/mdemg-test

# Stop and remove all containers + volumes
docker compose down -v

# Remove MDEMG config
rm -rf .mdemg .env .mdemg.port

# Uninstall git hooks (if installed)
mdemg hooks uninstall 2>/dev/null

# Clean up test secret
mdemg config set-secret TEST_BETA_KEY ""
```

### Final cleanup

```bash
rm -rf ~/mdemg-test
```

---

## Known Limitations

### 1. Docker Desktop Must Be Running

Docker Desktop does not auto-start by default. MDEMG cannot function without it.

**Fix:** Enable auto-start in Docker Desktop > Settings > General > "Start Docker Desktop when you sign in."

### 2. Docker Resource Constraints

Neo4j or TimescaleDB may fail if Docker has insufficient memory.

**Fix:** Docker Desktop > Settings > Resources > set Memory ≥ 4 GB.

### 3. Dynamic Port Discovery

All TCP ports are dynamically assigned during `mdemg init`. If you restart Docker Compose from a directory without a valid `.env`, services will fail to start.

**Fix:** Always run Docker Compose from the project directory where `mdemg init` was executed, or re-run `mdemg init`.

### 4. Features Requiring an LLM API Key

The following return degraded or empty results without an OpenAI or Ollama embedding provider:

- `recall` — semantic search returns no results
- `consolidation` — concept naming uses fallback (generic names)
- `SME consult` — consulting service unavailable
- `meta-learn` — cross-space generalization unavailable

**Fix:** Set an OpenAI key in `.env`:

```bash
echo 'OPENAI_API_KEY=sk-...' >> .env
docker compose restart mdemg
```

### 5. First Start Slow

First `docker compose up -d` pulls ~1 GB of images (Neo4j, TimescaleDB, Grafana, Neural sidecar). Subsequent starts use cached images.

### 6. Windows: WSL 2 Required

Docker Desktop on Windows requires WSL 2. If virtualization is disabled in BIOS or WSL 2 is not installed, Docker Desktop will not start.

---

## Feedback & Issue Reporting

### Filing Issues

File issues at: **https://github.com/reh3376/mdemg/issues**

**Title format:** `[Beta] <brief description>`

**Labels:** Add `beta-testing` and your OS label (`macos`, `windows`, `linux`)

### Include in Every Report

```
**Environment:**
- OS: (macOS version / Windows version / Linux distro + kernel)
- Architecture: (arm64 / amd64)
- MDEMG version: (output of `mdemg version`)
- Docker version: (output of `docker --version`)
- Docker Compose version: (output of `docker compose version`)

**Steps to Reproduce:**
1. <exact command>
2. <exact command>

**Expected Result:**
<what should have happened>

**Actual Result:**
<what actually happened — paste full output>

**Docker Logs (if applicable):**
<output of: docker compose logs --tail=50>
```

### Severity Guide

| Severity | Meaning | Example |
|----------|---------|---------|
| **Critical** | Cannot install or start | Docker services won't start, CLI crashes |
| **High** | Core feature broken | Ingest fails, observations not stored |
| **Medium** | Feature degraded | Hooks don't fire, config show incomplete |
| **Low** | Cosmetic or edge case | Minor formatting issue, help text typo |

---

## End of Testing

After completing all tiers, fill in the Results Summary table at the top of this document and submit it along with any issues filed.

Thank you for beta testing MDEMG!

## Documents Accessed

- `packaging/homebrew-mdemg/mdemg_beta_testing_mac.md` — macOS beta testing guide (consolidated)
- `packaging/mdemg-windows/mdemg_beta_testing.md` — Windows beta testing guide (consolidated)
- `packaging/mdemg_linux/mdemg_beta_testing_linux.md` — Linux beta testing guide (consolidated)
- `docs/user/quickstart-docker.md` — Docker deployment guide
- `docs/user/api-reference.md` — API reference
- `docs/user/cli-reference.md` — CLI reference
