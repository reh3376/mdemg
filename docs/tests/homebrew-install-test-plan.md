# Homebrew Install Test Plan

**Version**: v0.2.1
**Date**: 2026-03-10
**Purpose**: Validate that `brew install mdemg` delivers a fully functional MDEMG installation.

---

## Prerequisites

- macOS with Homebrew installed
- Docker Desktop running (required for Neo4j)
- No existing `mdemg` binary in PATH (or willing to unlink it)
- Internet access (downloads release tarball from GitHub)

---

## Phase 1: Clean Install

### 1.1 Remove any existing installation

```bash
# Check if mdemg is already installed
which mdemg
brew list mdemg 2>/dev/null

# If installed, remove it first for a clean test
brew uninstall mdemg 2>/dev/null
brew untap reh3376/mdemg 2>/dev/null
```

**Expected**: No `mdemg` in PATH after cleanup.

### 1.2 Tap and install

```bash
brew tap reh3376/mdemg
brew install mdemg
```

**Expected**:
- [ ] Tap succeeds without errors
- [ ] Install downloads `mdemg_0.2.1_darwin_{arm64|amd64}.tar.gz`
- [ ] SHA256 checksum passes
- [ ] No build-from-source step (binary install only)

### 1.3 Verify binary placement

```bash
which mdemg
ls -la $(brew --prefix)/bin/mdemg
file $(brew --prefix)/bin/mdemg
```

**Expected**:
- [ ] Binary exists at `$(brew --prefix)/bin/mdemg`
- [ ] Binary is a Mach-O 64-bit executable (arm64 or x86_64)
- [ ] Binary is executable

---

## Phase 2: Version and Help

### 2.1 Version check

```bash
mdemg version
```

**Expected**:
- [ ] Outputs version `0.2.1`
- [ ] Shows commit hash (not "unknown")
- [ ] Shows build date (not "unknown")

### 2.2 Help output

```bash
mdemg --help
```

**Expected**:
- [ ] Shows "Multi-Dimensional Emergent Memory Graph" description
- [ ] Lists all subcommands (at least: init, serve, config, ingest, consolidate, db, start, stop, status, hooks, version)
- [ ] No error output

### 2.3 Subcommand help

```bash
mdemg init --help
mdemg serve --help
mdemg db --help
mdemg config --help
mdemg ingest --help
mdemg hooks --help
mdemg start --help
```

**Expected**:
- [ ] Each subcommand shows its own help text with flags and description
- [ ] No panics or missing command errors

---

## Phase 3: Project Init

### 3.1 Initialize a test project

```bash
mkdir /tmp/mdemg-brew-test && cd /tmp/mdemg-brew-test
git init
mdemg init
```

**Expected**:
- [ ] Interactive wizard runs (asks about Neo4j, embedding provider, etc.)
- [ ] If OpenAI selected: prompts for API key (stored in `.env`, not config.yaml)
- [ ] Creates `.mdemg/config.yaml`
- [ ] Creates `.mdemgignore`
- [ ] Creates/updates `.env` with `NEO4J_PASS` and `OPENAI_API_KEY` (if provided)
- [ ] Detects git repository
- [ ] If `.claude/` directory exists, generates `.claude/mcp.json`

### 3.2 Verify generated config

```bash
cat .mdemg/config.yaml
cat .mdemgignore
```

**Expected**:
- [ ] Config YAML has valid structure (neo4j_uri, space_id, etc.)
- [ ] `.mdemgignore` has sensible defaults (`.env`, `node_modules/`, etc.)

### 3.3 Config show

```bash
mdemg config show
mdemg config show --json
```

**Expected**:
- [ ] Shows effective configuration with source annotations (yaml/env/default)
- [ ] JSON output is valid JSON

### 3.4 Config validate

```bash
mdemg config validate
```

**Expected**:
- [ ] Reports YAML syntax status
- [ ] Reports Neo4j reachability (may fail if not running yet — that's OK)
- [ ] Reports embedding provider reachability

---

## Phase 4: Database Management

### 4.1 Start Neo4j

```bash
# Each project gets its own isolated container and volume.
# If port 7687 is in use, an available port is auto-selected (7688, 7689, etc.)
mdemg db start
```

**Expected**:
- [ ] Creates a project-scoped container (e.g., `mdemg-neo4j-<project-name>`)
- [ ] Auto-selects available port if 7687 is busy
- [ ] Updates `.mdemg/config.yaml` with actual bolt URI
- [ ] Reports success with connection info (container, bolt, browser, password, volume)

### 4.2 Database status

```bash
mdemg db status
```

**Expected**:
- [ ] Shows project-scoped container name and status
- [ ] Shows actual mapped bolt and HTTP ports
- [ ] Shows schema version info

### 4.3 Run migrations

```bash
mdemg db migrate
```

**Expected**:
- [ ] Applies all migration files (V0001 through latest)
- [ ] Reports migration count and status
- [ ] No errors

### 4.4 Database shell

```bash
# Quick test — just verify it connects, then exit
echo "RETURN 1;" | mdemg db shell
```

**Expected**:
- [ ] Connects to Neo4j
- [ ] Executes query and returns result

---

## Phase 5: Server Lifecycle

### 5.1 Start server (foreground)

```bash
# Run in background for testing, kill after
mdemg serve &
SERVE_PID=$!
sleep 3
curl -s http://localhost:9999/healthz
kill $SERVE_PID 2>/dev/null
```

**Expected**:
- [ ] Server starts on port 9999 (or configured port)
- [ ] `/healthz` returns `{"status":"ok","version":"0.2.1"}`
- [ ] Server shuts down cleanly on SIGTERM

### 5.2 Start server (daemon mode)

```bash
mdemg start --auto-migrate
```

**Expected**:
- [ ] Server starts in background
- [ ] PID file created at `.mdemg/mdemg.pid`
- [ ] Log file at `.mdemg/logs/mdemg.log`
- [ ] Migrations applied before server start

### 5.3 Server status

```bash
mdemg status
```

**Expected**:
- [ ] Reports server as running
- [ ] Shows PID
- [ ] Shows port
- [ ] Shows uptime or health info

### 5.4 Health check

```bash
curl -s http://localhost:9999/healthz | python3 -m json.tool
curl -s http://localhost:9999/readyz | python3 -m json.tool
```

**Expected**:
- [ ] `/healthz` returns status "ok" with version "0.2.1"
- [ ] `/readyz` returns readiness status

### 5.5 Stop server

```bash
mdemg stop
```

**Expected**:
- [ ] Server stops gracefully
- [ ] PID file removed
- [ ] `mdemg status` confirms not running

### 5.6 Restart server

```bash
mdemg start
mdemg restart
mdemg status
```

**Expected**:
- [ ] Restart stops and starts the server
- [ ] New PID after restart
- [ ] Status confirms running

---

## Phase 6: Core Functionality

### 6.1 Embeddings check

```bash
mdemg embeddings check
```

**Expected**:
- [ ] Reports embedding provider (OpenAI or Ollama)
- [ ] Reports dimensions (e.g., 1536 for OpenAI, 768/1024 for Ollama)
- [ ] If no provider configured, reports clearly

### 6.2 Ingest a codebase

```bash
# Create a minimal test codebase
mkdir -p /tmp/mdemg-brew-test/src
cat > /tmp/mdemg-brew-test/src/main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello MDEMG")
}
EOF

mdemg ingest /tmp/mdemg-brew-test/src --space-id brew-test
```

**Expected**:
- [ ] Ingestion starts and processes files
- [ ] Reports files found and processed
- [ ] Creates MemoryNode entries in Neo4j
- [ ] If no embedding provider, warns but still ingests text

### 6.3 Extract symbols

```bash
mdemg extract-symbols --json /tmp/mdemg-brew-test/src/main.go
```

**Expected**:
- [ ] Extracts Go symbols (package, function main)
- [ ] JSON output with symbol name, type, line number
- [ ] Valid JSON array

### 6.4 Observe and recall (requires server running)

```bash
# Ensure server is running
mdemg start 2>/dev/null

# Store an observation
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"brew-test","session_id":"brew-install-test","content":"Homebrew install test completed successfully","obs_type":"progress"}'

# Recall it
curl -s -X POST http://localhost:9999/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"brew-test","query":"homebrew install","max_results":5}'
```

**Expected**:
- [ ] Observe returns `obs_id` and `node_id`
- [ ] Recall returns results containing the observation (if embeddings available)
- [ ] If no embedding provider, recall returns empty but no errors

### 6.5 Memory stats

```bash
curl -s "http://localhost:9999/v1/memory/stats?space_id=brew-test" | python3 -m json.tool
```

**Expected**:
- [ ] Returns node counts
- [ ] Shows layer distribution
- [ ] Valid JSON response

---

## Phase 7: Hook Management

### 7.1 List hooks

```bash
cd /tmp/mdemg-brew-test
mdemg hooks list
```

**Expected**:
- [ ] Shows available hook types
- [ ] Reports which hooks are installed (if any)

### 7.2 Install hooks

```bash
mdemg hooks install --space-id brew-test
```

**Expected**:
- [ ] Installs git hooks (post-commit at minimum)
- [ ] Reports which hooks were installed
- [ ] Hooks are executable

### 7.3 Uninstall hooks

```bash
mdemg hooks uninstall
```

**Expected**:
- [ ] Removes installed hooks
- [ ] Reports cleanup

---

## Phase 8: Cleanup

### 8.1 Stop everything

```bash
mdemg stop 2>/dev/null
mdemg db stop 2>/dev/null
```

### 8.2 Clean up test artifacts

```bash
rm -rf /tmp/mdemg-brew-test
```

### 8.3 (Optional) Uninstall

```bash
brew uninstall mdemg
brew untap reh3376/mdemg
```

**Expected**:
- [ ] Binary removed from PATH
- [ ] Tap removed
- [ ] `which mdemg` returns nothing

---

## Results Summary

| Phase | Test | Pass/Fail | Notes |
|-------|------|-----------|-------|
| 1 | Clean install | | |
| 1 | Binary placement | | |
| 2 | Version output | | |
| 2 | Help output | | |
| 2 | Subcommand help | | |
| 3 | Project init | | |
| 3 | Config show/validate | | |
| 4 | DB start | | |
| 4 | DB migrate | | |
| 4 | DB shell | | |
| 5 | Server foreground | | |
| 5 | Server daemon (start/stop/restart) | | |
| 5 | Health/readiness endpoints | | |
| 6 | Embeddings check | | |
| 6 | Codebase ingest | | |
| 6 | Symbol extraction | | |
| 6 | Observe + recall | | |
| 6 | Memory stats | | |
| 7 | Hooks install/list/uninstall | | |
| 8 | Clean uninstall | | |

---

## Known Considerations

1. **Embedding provider**: Many tests work without an embedding provider (OpenAI/Ollama). Semantic recall requires embeddings. If not configured, observe/ingest still works but recall returns empty results.

2. **Docker requirement**: Neo4j runs in Docker. If Docker Desktop is not running, Phase 4+ will fail. The test plan assumes Docker is available.

3. **Port conflicts**: Default port 9999. If another service uses this port, set `MDEMG_PORT` or use `--port` flag.

4. **Existing dev installation**: If you have a dev build of `mdemg` in your PATH (e.g., `~/mdemg/bin/mdemg`), ensure the brew-installed version takes precedence by checking `which mdemg` resolves to `$(brew --prefix)/bin/mdemg`.
