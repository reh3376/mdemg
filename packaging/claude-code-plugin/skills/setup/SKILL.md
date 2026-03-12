---
name: setup
description: Guided setup for MDEMG memory graph in this project
user_invocable: true
---

# MDEMG Setup

Help the user set up MDEMG for this project. Run through these steps:

## 1. Check prerequisites

Run these checks and report status:

```bash
# Check mdemg binary
which mdemg && mdemg version || echo "NOT INSTALLED — install via: brew install mdemg"

# Check Docker (needed for Neo4j)
docker info --format '{{.ServerVersion}}' 2>/dev/null || echo "Docker not running"

# Check if already initialized
ls -la .mdemg/sidecar.yaml 2>/dev/null && echo "Already initialized" || echo "Not initialized"
```

## 2. Initialize (if needed)

If not initialized, run:
```bash
mdemg sidecar init --profile local --agents claude-code
```

This creates `.mdemg/sidecar.yaml` with the user's configuration.

## 3. Install dependencies

```bash
mdemg sidecar install
```

This checks Docker, pulls Neo4j image, validates the config.

## 4. Start the sidecar

```bash
mdemg sidecar up
```

## 5. Run initial ingest

```bash
mdemg ingest --path .
```

## 6. Verify

```bash
# Check health
curl -sf "http://localhost:$(cat .mdemg.port 2>/dev/null || echo 9999)/healthz" | jq

# Check memory stats
curl -sf "http://localhost:$(cat .mdemg.port 2>/dev/null || echo 9999)/v1/memory/stats?space_id=$(basename $(pwd) | tr '[:upper:]' '[:lower:]')" | jq
```

## 7. Report

Tell the user:
- MDEMG is running and ingested
- The plugin's SessionStart hook will restore memory context automatically
- MCP tools (memory_recall, memory_store, etc.) are available
- Next session will auto-connect
