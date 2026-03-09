# Unified CLI

Phase 93 merges 12 separate Go binaries into a single `mdemg` command using Cobra. Instead of building and managing individual tools (`mdemg-server`, `mdemg-ingest`, `mdemg-decay`, etc.), everything is accessible through one binary with subcommands.

## Getting Started

### Build

```bash
make build-cli
```

This produces `bin/mdemg` with build-time version info. You can also build directly:

```bash
go build -o bin/mdemg ./cmd/mdemg
```

### Verify Installation

```bash
./bin/mdemg version
# mdemg v0.93.0
#   commit:    abc1234
#   built:     2026-02-23T12:00:00Z
#   go:        go1.25.6
#   os/arch:   darwin/arm64
```

### See All Commands

```bash
./bin/mdemg --help
```

Every subcommand supports `--help` for detailed flag documentation.

---

## Default Space ID

Almost every command requires `--space-id`. Rather than passing it every time, you can set a default:

```bash
# Option 1: Environment variable (recommended for scripting)
export MDEMG_SPACE_ID=my-project
./bin/mdemg consolidate --hidden-layer          # uses my-project
./bin/mdemg decay                                # uses my-project
./bin/mdemg ingest --path=.                      # uses my-project (overrides "codebase" default)

# Option 2: Global CLI flag
./bin/mdemg --space-id=my-project consolidate --hidden-layer
./bin/mdemg --space-id=my-project prune

# Option 3: Per-command flag (always wins)
./bin/mdemg --space-id=default ingest --space-id=other --path=.  # uses "other"
```

**Resolution order**: command `--space-id` flag > global `--space-id` flag > `MDEMG_SPACE_ID` env var > command default (e.g., "codebase" for ingest).

---

## Project Initialization

Initialize a new MDEMG project in the current directory:

```bash
mdemg init                    # Interactive wizard
mdemg init --defaults         # Non-interactive with sensible defaults
mdemg init --yes              # Alias for --defaults
```

The wizard:

1. Detects your environment (Neo4j, Ollama, Git, IDE)
2. Prompts for space ID, Neo4j URI, embedding provider
3. Generates `.mdemg/config.yaml` and `.mdemgignore`
4. Optionally installs a git post-commit hook
5. Optionally writes IDE MCP configs (`.cursor/mcp.json`, `.vscode/mcp.json`, `.claude/mcp.json`)

Override specific settings:

```bash
mdemg init --defaults --neo4j-uri bolt://db:7687 --embedding-provider openai
mdemg init --defaults --no-hooks --no-ide
```

### `.mdemg/config.yaml`

The YAML config exposes ~20 commonly-adjusted settings. It is read before `.env` and `FromEnv()`, setting env vars only when not already set:

```yaml
neo4j:
  uri: bolt://localhost:7687
  user: neo4j
server:
  port: 9999
embedding:
  provider: ollama
  model: qwen3-embedding:4b
  endpoint: http://localhost:11434
schema:
  version: 17
```

**Priority** (lowest → highest): defaults → `.mdemg/config.yaml` → keychain → `.env` → env vars → CLI flags.

Secrets (passwords, API keys) should use the system keychain (`mdemg config set-secret`) or stay in `.env`/env vars — never in `config.yaml`.

### `.mdemgignore`

Gitignore-style exclusion patterns for `mdemg ingest`. Place in the project root:

```
# Dependencies
node_modules/
vendor/

# Build artifacts
build/
dist/

# Binary files
*.exe
*.dll
*.min.js
```

Patterns are applied during the file walk phase, before any file is parsed. Supports `#` comments, `!` negation, and directory patterns ending with `/`.

---

## Configuration Management

### Show Effective Config

```bash
mdemg config show             # Human-readable table with sources
mdemg config show --json      # Machine-readable JSON
```

Output shows each setting's value and where it came from (`yaml`, `env`, or `default`). Secrets are masked.

### Validate Config

```bash
mdemg config validate
```

Checks:

- YAML syntax and field values
- Neo4j reachability (TCP probe on configured URI)
- Embedding provider reachability (HTTP probe)

### Secret Management

Store secrets in the system keychain instead of plaintext `.env` files:

```bash
# Store a secret (prompts for hidden input if value omitted)
mdemg config set-secret neo4j-password
mdemg config set-secret openai-api-key sk-abc123

# Retrieve a secret
mdemg config get-secret neo4j-password

# List known secrets and their keychain status
mdemg config list-secrets
```

Known secret keys are automatically resolved to env vars on startup:

| Key | Env Var |
|-----|---------|
| `neo4j-password` | `NEO4J_PASS` |
| `openai-api-key` | `OPENAI_API_KEY` |
| `jwt-secret` | `AUTH_JWT_SECRET` |
| `linear-webhook` | `LINEAR_WEBHOOK_SECRET` |

Keychain is opportunistic — if unavailable, MDEMG falls back silently to `.env` and env vars. See [docs/features/secret-management.md](secret-management.md) for details.

---

## Embedding Provider

### Check Provider Status

```bash
./bin/mdemg embeddings check
```

Performs an actual test embedding (not just a connectivity probe) and reports dimensions, provider, and status. Shows setup instructions when disabled, remediation steps on failure.

---

## Starting the Server

### Background (Daemon Mode)

```bash
./bin/mdemg start                          # Start in background
./bin/mdemg start --port=9999              # Custom port
./bin/mdemg start --auto-migrate           # Apply migrations on startup
./bin/mdemg start --mcp                    # Start MCP server alongside
./bin/mdemg start --no-db                  # Don't auto-start Neo4j
```

The server runs as a detached process. Logs go to `.mdemg/logs/mdemg.log`, PID to `.mdemg/mdemg.pid`. If Docker is available and a Neo4j container exists but is stopped, it's started automatically.

### Lifecycle Commands

```bash
./bin/mdemg stop                           # Stop server (SIGTERM, 30s timeout)
./bin/mdemg restart                        # Stop then start
./bin/mdemg restart --port=8080            # Restart with new settings
./bin/mdemg status                         # Show server/DB status
```

`mdemg stop` stops the MDEMG server only — Neo4j is left running.

```
MDEMG Status
============
  Server:    running (pid=12345)
  Port:      9999
  Uptime:    2h 15m
  Log:       .mdemg/logs/mdemg.log
  Health:    ok
  Neo4j:     running (mdemg-neo4j-dev)
```

See [docs/features/process-lifecycle.md](process-lifecycle.md) for details.

### Foreground Mode

```bash
./bin/mdemg serve                          # Foreground (for development/debugging)
./bin/mdemg serve --port=8080
./bin/mdemg serve --db-uri=bolt://other-host:7687
./bin/mdemg serve --auto-migrate
```

The server reads configuration from environment variables (via `.env` file), with CLI flag overrides for common settings.

The server will:

1. Load configuration (defaults → yaml → keychain → .env → env vars → flags)
2. Connect to Neo4j
3. Apply pending migrations (if `--auto-migrate`)
4. Verify schema version (auto-detected if `REQUIRED_SCHEMA_VERSION` not set)
5. Initialize plugins (if enabled)
6. Start background tasks (consolidation, sync, RSIC, pruning)
7. Listen on the configured port (default `:9999`)

Graceful shutdown on `SIGINT`/`SIGTERM`.

---

## Ingesting a Codebase

### Basic Ingestion

```bash
./bin/mdemg ingest --space-id=my-project --path=/path/to/repo
```

### Incremental Ingestion (Changed Files Only)

```bash
./bin/mdemg ingest --space-id=my-project --path=/path/to/repo --incremental
```

This uses `git diff` to detect files changed since the last commit. Control the diff base with `--since`:

```bash
./bin/mdemg ingest --space-id=my-project --path=. --incremental --since=HEAD~5
```

### Dry Run (Preview Without Changes)

```bash
./bin/mdemg ingest --space-id=my-project --path=. --dry-run
```

### Quiet Mode (For Hooks and CI)

```bash
./bin/mdemg ingest --space-id=my-project --path=. --quiet
```

### Logging to File

```bash
./bin/mdemg ingest --space-id=my-project --path=. --log-file /tmp/ingest.log
```

### Tuning Performance

```bash
./bin/mdemg ingest --space-id=my-project --path=. \
  --workers=8 \
  --batch=200 \
  --delay=25 \
  --timeout=600
```

| Flag | Default | Description |
|------|---------|-------------|
| `--workers` | 4 | Parallel ingestion workers |
| `--batch` | 100 | Files per batch (optimal ~15/s per worker) |
| `--delay` | 50 | Milliseconds between batches |
| `--timeout` | 300 | HTTP timeout in seconds |
| `--retries` | 3 | Max retries per batch on failure |
| `--retry-delay` | 2000 | Initial retry delay in ms (doubles each retry) |

### Language Selection

All major languages are included by default. Toggle specific languages:

```bash
# Exclude tests, include everything else
./bin/mdemg ingest --space-id=my-project --path=. --include-tests=false

# See all supported languages
./bin/mdemg ingest --list-languages
```

Flags: `--include-ts`, `--include-py`, `--include-java`, `--include-rust`, `--include-md`, `--include-tests`.

### Exclusion Presets

```bash
# ML/CUDA project (excludes model weights, datasets)
./bin/mdemg ingest --space-id=ml-project --path=. --preset=ml_cuda

# Web monorepo (excludes dist, build, .next)
./bin/mdemg ingest --space-id=webapp --path=. --preset=web_monorepo
```

### LLM-Powered Summaries

Generate semantic summaries for ingested files using an LLM:

```bash
./bin/mdemg ingest --space-id=my-project --path=. \
  --llm-summary \
  --llm-summary-provider=openai \
  --llm-summary-model=gpt-4o-mini \
  --llm-summary-batch=10
```

Requires `OPENAI_API_KEY` in your environment.

---

## Graph Consolidation

Consolidation creates concept layers from raw observations. Two modes are available.

### Hidden Layer Consolidation (Recommended)

```bash
# Dry run — preview what would happen
./bin/mdemg consolidate --space-id=my-project --hidden-layer

# Live run — create concept nodes
./bin/mdemg consolidate --space-id=my-project --hidden-layer --dry-run=false

# Full multi-layer (L0-L5)
./bin/mdemg consolidate --space-id=my-project --hidden-layer --multi-layer --dry-run=false

# Forward pass only
./bin/mdemg consolidate --space-id=my-project --hidden-layer --forward-only --dry-run=false
```

Tunable DBSCAN clustering parameters:

| Flag | Default | Description |
|------|---------|-------------|
| `--hidden-eps` | 0.3 | DBSCAN epsilon (max distance between points) |
| `--hidden-min-samples` | 3 | Minimum samples per cluster |
| `--hidden-max-nodes` | 100 | Maximum hidden nodes to create |

### Legacy Consolidation

```bash
./bin/mdemg consolidate --space-id=my-project --legacy

# Adjust thresholds
./bin/mdemg consolidate --space-id=my-project --legacy \
  --weight-threshold=0.5 \
  --min-cluster-size=3 \
  --max-promotions=50
```

All consolidation commands default to `--dry-run=true`. You must explicitly pass `--dry-run=false` to apply changes.

---

## Temporal Decay

Apply exponential decay to learning edges based on time since last activation.

```bash
# Dry run (default) — see what would decay
./bin/mdemg decay --space-id=my-project

# Live run
./bin/mdemg decay --space-id=my-project --dry-run=false

# Custom decay parameters
./bin/mdemg decay --space-id=my-project \
  --decay-rate=0.1 \
  --older-than=7 \
  --prune-threshold=0.01 \
  --min-evidence=3 \
  --batch-size=1000
```

The decay formula: `w_new = w_old * exp(-decay_rate * days_since_activation)`

Edges are pruned only when ALL of these are true:

- Weight below `--prune-threshold`
- Evidence count below `--min-evidence`
- Edge is not pinned

Run across all spaces by omitting `--space-id`.

---

## Pruning

Remove weak edges, tombstone orphan nodes, and optionally merge redundant nodes.

```bash
# Dry run — preview pruning plan
./bin/mdemg prune --space-id=my-project

# Live run
./bin/mdemg prune --space-id=my-project --dry-run=false

# Enable node merging (more aggressive)
./bin/mdemg prune --space-id=my-project --dry-run=false \
  --merge-enabled \
  --similarity-threshold=0.98
```

| Flag | Default | Description |
|------|---------|-------------|
| `--weight-threshold` | 0.01 | Minimum edge weight to keep |
| `--older-than-days` | 30 | Only prune edges older than N days |
| `--retention-days` | 90 | Days without observation before tombstoning |
| `--max-degree` | 1 | Max edges for orphan detection |
| `--merge-enabled` | false | Enable node merging (destructive) |
| `--similarity-threshold` | 0.98 | Vector similarity threshold for merge |

### Recommended Workflow: Decay then Prune

Decay and prune are designed to run in sequence. Decay weakens stale edges based on time; prune removes the weakened edges and orphaned nodes. Running prune without first running decay may leave edges that should have been weakened but weren't.

```bash
# Step 1: Decay weakens old edges
./bin/mdemg decay --space-id=my-project --dry-run=false

# Step 2: Prune removes weak edges and orphaned nodes
./bin/mdemg prune --space-id=my-project --dry-run=false
```

Always run with `--dry-run` first (the default) to preview changes before applying them.

---

## Extracting Code Symbols

Extract constants, functions, and classes from source code using tree-sitter.

```bash
# Extract and store in Neo4j
./bin/mdemg extract-symbols --path=/path/to/repo --space-id=my-project

# Dry run
./bin/mdemg extract-symbols --path=. --space-id=my-project --dry-run

# JSON output for UPTS testing
./bin/mdemg extract-symbols --json path/to/file.go

# Parallel extraction
./bin/mdemg extract-symbols --path=. --space-id=my-project --workers=8
```

Symbols are stored as Neo4j nodes linked to their source file nodes, enabling evidence-locked retrieval (queries can cite specific function definitions).

---

## File Watching

Monitor a directory for file changes and automatically ingest them.

```bash
# Watch current directory
./bin/mdemg watch --space-id=my-project

# Watch a specific path with custom debounce
./bin/mdemg watch --space-id=my-project --path=./src --debounce=500

# Custom extensions and exclusions
./bin/mdemg watch --space-id=my-project \
  --extensions=".go,.py,.ts" \
  --exclude="node_modules,.git,vendor"
```

The watcher debounces rapid file changes (default 500ms) and automatically ingests modified files into the specified space.

---

## Database Management

### Migrations

```bash
# Show migration status (current version, pending)
./bin/mdemg db migrate --status

# Preview what would be applied
./bin/mdemg db migrate --dry-run

# Apply all pending migrations
./bin/mdemg db migrate

# Use filesystem migrations instead of embedded
./bin/mdemg db migrate --migrations-dir ./migrations
```

Migrations are embedded in the binary. Re-running is idempotent (0 applied if up to date).

### Docker Container

```bash
# Start a lightweight Neo4j for development
./bin/mdemg db start
./bin/mdemg db start --port=7688 --password=mypassword

# Check container and schema status
./bin/mdemg db status

# Stop the container (data volume preserved)
./bin/mdemg db stop

# Stop and remove container
./bin/mdemg db stop --remove

# Open interactive cypher-shell
./bin/mdemg db shell
```

`db start` creates a container with 1GB heap and 512MB page cache (suitable for development). Data persists in a Docker volume (`mdemg-neo4j-data`).

### Reset a Space

```bash
# Delete a specific space
./bin/mdemg db reset --space-id=my-test-space

# Delete all non-protected spaces (interactive confirmation)
./bin/mdemg db reset --all

# Skip confirmation prompt
./bin/mdemg db reset --all --yes
```

Protected spaces (`mdemg-dev` — conversation memory) are never deleted, even with `--all`.

---

## Space Management

Spaces are isolated graph namespaces. Export, import, list, and transfer them.

### List Spaces

```bash
./bin/mdemg space list
```

### Export a Space

```bash
# Full export
./bin/mdemg space export --space-id=my-project

# Export to specific file
./bin/mdemg space export --space-id=my-project --output=backup.mdemg

# Profile-based export (smaller files)
./bin/mdemg space export --space-id=my-project --profile=codebase
./bin/mdemg space export --space-id=my-project --profile=cms
./bin/mdemg space export --space-id=my-project --profile=metadata

# Exclude embeddings for smaller archives
./bin/mdemg space export --space-id=my-project --no-embeddings

# Incremental export (only changes since timestamp)
./bin/mdemg space export --space-id=my-project --since-timestamp=2026-02-01T00:00:00Z
```

Export profiles:

| Profile | Includes |
|---------|----------|
| `full` | Everything (default) |
| `codebase` | Code nodes, symbols, edges |
| `cms` | Conversation observations |
| `learned` | Learning edges only |
| `metadata` | Node metadata without embeddings |

### Import a Space

```bash
# Import with skip-on-conflict (default)
./bin/mdemg space import --input=backup.mdemg

# Overwrite existing nodes
./bin/mdemg space import --input=backup.mdemg --conflict=overwrite

# Fail on any collision
./bin/mdemg space import --input=backup.mdemg --conflict=error
```

### Space Info

```bash
./bin/mdemg space info --space-id=my-project
```

### Delete a Space

```bash
# Delete with interactive confirmation
./bin/mdemg space delete --space-id=my-test-space

# Skip confirmation
./bin/mdemg space delete --space-id=my-test-space --yes
```

Protected spaces (`mdemg-dev`) cannot be deleted. Deletion is batched and irreversible.

### Rename a Space

```bash
./bin/mdemg space rename --from=old-project --to=new-project
```

Updates `space_id` on all nodes in batch. The target name must not already exist. Protected spaces cannot be renamed.

### Copy a Space

```bash
./bin/mdemg space copy --from=production --to=staging
```

Duplicates all nodes and edges from the source space. New nodes receive fresh `node_id` values to avoid collisions. The target space must not already exist.

### Remote Space Transfer

```bash
# Start a gRPC server for remote pulls
./bin/mdemg space serve

# Pull a space from a remote server
./bin/mdemg space pull --remote=other-host:50051 --space-id=my-project
```

---

## Plugin Management

### Scaffold a New Plugin

```bash
# Create an ingestion plugin
./bin/mdemg plugin scaffold --name="My Parser" --type=INGESTION

# Create a reasoning plugin in a custom directory
./bin/mdemg plugin scaffold --name="Custom Ranker" --type=REASONING --output=./my-plugins

# Create an APE (background task) plugin
./bin/mdemg plugin scaffold --name="Background Task" --type=APE --version=2.0.0
```

This generates a complete plugin scaffold:

- `manifest.json` — plugin metadata
- `main.go` — entrypoint
- `handler.go` — business logic
- `Makefile` — build automation
- `README.md` — documentation

### Validate a Plugin

```bash
# Full validation (manifest, proto, health, lifecycle)
./bin/mdemg plugin validate --plugin=./plugins/my-plugin

# Manifest only
./bin/mdemg plugin validate --plugin=./plugins/my-plugin --manifest-only

# Health check a running plugin
./bin/mdemg plugin validate --socket=/var/run/mdemg/my-plugin.sock --health-only

# JSON output (for CI)
./bin/mdemg plugin validate --plugin=./plugins/my-plugin --json
```

---

## Git Hook Management

Manage MDEMG git hooks for automatic code ingestion on commit.

### Install

```bash
mdemg hooks install                      # Install with default space ID
mdemg hooks install --space-id myproj    # Install with custom space ID
mdemg hooks install --force              # Overwrite existing hook
```

### Uninstall

```bash
mdemg hooks uninstall                    # Remove MDEMG hooks only
```

Only removes hooks installed by MDEMG (identified by the `# MDEMG` marker). Non-MDEMG hooks are left untouched.

### List

```bash
mdemg hooks list                         # Show hook status
```

See [docs/features/ide-repo-integration.md](ide-repo-integration.md) for full details.

---

## Self-Update

Update the `mdemg` binary to the latest release from GitHub:

```bash
# Check for updates (dry run)
./bin/mdemg upgrade --dry-run

# Upgrade to latest release
./bin/mdemg upgrade

# Force upgrade even if already on latest version
./bin/mdemg upgrade --force
```

The upgrade command checks GitHub Releases for the latest version, downloads the appropriate binary for your platform, verifies the SHA256 checksum, and replaces the current binary using a backup-and-replace strategy. The previous binary is preserved as a backup in case rollback is needed.

---

## MCP Server (IDE Integration)

Start the MCP server for integration with AI coding assistants (Cursor, Claude Code, etc.):

```bash
./bin/mdemg mcp
```

The MCP server runs in stdio mode for agent communication. It provides memory tools (recall, observe, ingest) through the Model Context Protocol.

### Co-located with HTTP Server

Start both the HTTP API and MCP server in one process:

```bash
./bin/mdemg serve --mcp
```

The MCP subprocess receives the correct `MDEMG_ENDPOINT` automatically. Both are shut down gracefully together.

---

## Global Flags

All commands support:

| Flag | Description |
|------|-------------|
| `--verbose` | Enable verbose output |
| `--help` | Show help for any command |
| `--version` | Print version (root command only) |

---

## Shell Completion

Generate shell completions for tab-completion support:

```bash
# Bash (system-wide, requires root)
sudo ./bin/mdemg completion bash > /etc/bash_completion.d/mdemg

# Bash (user-local, Homebrew on macOS)
./bin/mdemg completion bash > "$(brew --prefix)/etc/bash_completion.d/mdemg"

# Bash (user-local, no Homebrew)
mkdir -p ~/.local/share/bash-completion/completions
./bin/mdemg completion bash > ~/.local/share/bash-completion/completions/mdemg

# Zsh
./bin/mdemg completion zsh > "${fpath[1]}/_mdemg"

# Fish
./bin/mdemg completion fish > ~/.config/fish/completions/mdemg.fish
```

---

## Migrating from Legacy Binaries

All old binaries (`mdemg-server`, `mdemg-ingest`, `mdemg-decay`, etc.) still work but print a deprecation warning and delegate to the unified CLI. Update your scripts:

| Old Command | New Command |
|-------------|-------------|
| `mdemg-server` | `mdemg serve` |
| `mdemg-mcp` | `mdemg mcp` |
| `mdemg-ingest --space-id=X --path=Y` | `mdemg ingest --space-id=X --path=Y` |
| `mdemg-consolidate --space-id=X` | `mdemg consolidate --space-id=X` |
| `mdemg-decay --space-id=X` | `mdemg decay --space-id=X` |
| `mdemg-prune --space-id=X` | `mdemg prune --space-id=X` |
| `mdemg-extract-symbols --path=X` | `mdemg extract-symbols --path=X` |
| `mdemg-watch --space-id=X` | `mdemg watch --space-id=X` |
| `mdemg-reset-db --space-id=X` | `mdemg db reset --space-id=X` |
| `mdemg-space-transfer export ...` | `mdemg space export ...` |
| `mdemg-plugin-scaffold ...` | `mdemg plugin scaffold ...` |
| `mdemg-plugin-validate ...` | `mdemg plugin validate ...` |

All flags are identical — only the command prefix changes.
