# MDEMG CLI Reference

> **Version:** dev (build-time injected via `-ldflags`)
> **Binary:** `mdemg` (or `mdemg.exe` on Windows)
> **Build:** `go build -o bin/mdemg ./cmd/mdemg`

MDEMG (Multi-Dimensional Emergent Memory Graph) is a persistent memory system for LLMs providing vector-based semantic search, graph-based knowledge representation, hidden layer concept abstraction, learning edges (Hebbian reinforcement), and LLM re-ranking for improved retrieval.

---

## Global Flags

These persistent flags are available on every command and subcommand.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose` | bool | `false` | Enable verbose output |
| `--space-id` | string | `""` | Default space ID (overrides `MDEMG_SPACE_ID` env var) |
| `--version` | bool | `false` | Print version information |
| `--help` | bool | `false` | Show help for any command |

---

## Installation

### macOS

```bash
# Via Homebrew (recommended)
brew tap reh3376/mdemg
brew install mdemg

# To update
brew upgrade mdemg
```

### Linux

```bash
# Via APT (Debian/Ubuntu)
curl -fsSL https://reh3376.github.io/apt-mdemg/pubkey.gpg | sudo gpg --dearmor -o /usr/share/keyrings/mdemg.gpg
echo "deb [signed-by=/usr/share/keyrings/mdemg.gpg] https://reh3376.github.io/apt-mdemg stable main" | sudo tee /etc/apt/sources.list.d/mdemg.list
sudo apt update && sudo apt install mdemg

# Via install script
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg_linux/main/install.sh | bash
```

### Windows

```powershell
# Via Scoop (recommended)
scoop bucket add mdemg https://github.com/reh3376/homebrew-mdemg
scoop install mdemg

# To update
scoop update mdemg

# Via install script
irm https://raw.githubusercontent.com/reh3376/homebrew-mdemg/main/Install-MDEMG.ps1 | iex
```

### From Source (all platforms)

```bash
go build -o bin/mdemg ./cmd/mdemg
```

---

## Configuration Priority Chain

Configuration values are resolved in the following order (last wins):

1. **Compiled defaults** -- hardcoded in `config.FromEnv()`
2. **YAML config** -- `.mdemg/config.yaml` (loaded via `config.LoadYAMLConfig`)
3. **System keychain** -- secrets resolved via `go-keyring` (`secrets.ResolveSecrets()`)
4. **`.env` file** -- loaded via `godotenv.Load()`
5. **Environment variables** -- standard `os.Getenv()`
6. **CLI flags** -- Cobra flag values override everything

> **Platform note:** On macOS, secrets are stored in **Keychain Access**. On Linux, they use **Secret Service** (GNOME Keyring / KDE Wallet). On Windows, they use **Windows Credential Manager** (accessible via Control Panel > Credential Manager, or `cmdkey` from the command line). The `mdemg config set-secret` / `get-secret` commands abstract this for you.

Space ID resolution follows its own chain:
1. Local `--space-id` flag (if explicitly passed)
2. Global `--space-id` persistent flag
3. `MDEMG_SPACE_ID` environment variable
4. Local flag's default value (e.g., `"codebase"` for ingest)

---

## Getting Started

### `mdemg init`

**Synopsis:** `mdemg init [flags]`

Interactive project initialization wizard. Detects the local environment (Docker, Neo4j, Ollama, Git, IDE), generates `.mdemg/config.yaml` and `.mdemgignore`, optionally installs git hooks and IDE integration configs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--defaults` | bool | `false` | Accept all defaults without prompting |
| `--yes` | bool | `false` | Alias for `--defaults` |
| `--quick` | bool | `false` | Full auto-setup -- config, Neo4j, server, migrations, ingest, and companion app |
| `--space-id` | string | `""` | Pre-set space ID |
| `--neo4j-uri` | string | `""` | Pre-set Neo4j URI |
| `--embedding-provider` | string | `""` | Pre-set embedding provider (openai/ollama) |
| `--no-hooks` | bool | `false` | Skip git hook installation |
| `--no-ide` | bool | `false` | Skip IDE config generation |
| `--no-menubar` | bool | `false` | Skip menu bar app installation (macOS only) |
| `--no-sidebar` | bool | `false` | Skip sidebar app installation (Linux only) |

**Usage Examples:**
```bash
mdemg init                              # Interactive wizard
mdemg init --defaults                   # Accept all defaults
mdemg init --quick --space-id myproject # Full auto-setup with custom space
mdemg init --neo4j-uri bolt://localhost:7687 --embedding-provider ollama
```

The wizard also configures **Jiminy inner-voice guidance** (enabled by default). Users select a Jiminy-specific LLM model for guidance synthesis and constraint evaluation:
- **OpenAI default:** `gpt-5.4-nano` (cheapest option for small JSON tasks)
- **Ollama default:** `qwen3:8b` (best local model for structured output)

**See Also:** `mdemg config show`, `mdemg config validate`, `mdemg hooks install`

---

### `mdemg version`

**Synopsis:** `mdemg version`

Print the MDEMG version, git commit, build date, Go version, and OS/architecture. No flags.

**Usage Examples:**
```bash
mdemg version
```

---

## Server Lifecycle

### `mdemg start`

**Synopsis:** `mdemg start [flags]`

Start the MDEMG server as a background (daemon) process. The process is detached from the terminal. Logs are written to `.mdemg/logs/mdemg.log` and the PID is stored in `.mdemg/mdemg.pid`. If Docker is available and a Neo4j container exists but is stopped, it will be started automatically.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `0` | Listen port (overrides `LISTEN_ADDR` env var) |
| `--db-uri` | string | `""` | Neo4j URI (overrides `NEO4J_URI` env var) |
| `--auto-migrate` | bool | `false` | Apply pending database migrations before starting |
| `--mcp` | bool | `false` | Start MCP server subprocess alongside HTTP server |
| `--no-db` | bool | `false` | Do not auto-start Neo4j container |

**Usage Examples:**
```bash
mdemg start
mdemg start --port 9999 --auto-migrate
mdemg start --no-db
mdemg start --mcp
```

**See Also:** `mdemg stop`, `mdemg restart`, `mdemg status`, `mdemg serve`

---

### `mdemg stop`

**Synopsis:** `mdemg stop`

Stop the running MDEMG server process. Sends a termination signal (SIGTERM on macOS/Linux, equivalent on Windows) and waits up to 30 seconds for graceful shutdown. If the process does not exit, force kills it. Does **not** stop Neo4j -- use `mdemg db stop` for that. No flags.

**Usage Examples:**
```bash
mdemg stop
```

**See Also:** `mdemg start`, `mdemg restart`, `mdemg db stop`

---

### `mdemg restart`

**Synopsis:** `mdemg restart [flags]`

Stop and restart the MDEMG server. Accepts all the same flags as `mdemg start`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `0` | Listen port (overrides `LISTEN_ADDR` env var) |
| `--db-uri` | string | `""` | Neo4j URI (overrides `NEO4J_URI` env var) |
| `--auto-migrate` | bool | `false` | Apply pending database migrations before starting |
| `--mcp` | bool | `false` | Start MCP server subprocess alongside HTTP server |
| `--no-db` | bool | `false` | Do not auto-start Neo4j container |

**Usage Examples:**
```bash
mdemg restart
mdemg restart --port 9999
```

**See Also:** `mdemg start`, `mdemg stop`

---

### `mdemg status`

**Synopsis:** `mdemg status`

Display the status of the MDEMG server and Neo4j database. Shows PID, port, uptime, log path, health check result, embedding provider, Neo4j container status, and node count. No flags.

**Usage Examples:**
```bash
mdemg status
```

**See Also:** `mdemg start`, `mdemg db status`

---

### `mdemg serve`

**Synopsis:** `mdemg serve [flags]`

Start the MDEMG HTTP API server in the foreground. This is the underlying server process that `mdemg start` launches as a daemon. Loads configuration from `.env` file and environment variables, connects to Neo4j, applies migrations (if `--auto-migrate`), verifies schema version, initializes plugin manager, starts periodic background tasks, and starts the HTTP server with graceful shutdown support.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `0` | Listen port (overrides `LISTEN_ADDR` env var) |
| `--db-uri` | string | `""` | Neo4j URI (overrides `NEO4J_URI` env var) |
| `--auto-migrate` | bool | `false` | Apply pending database migrations before starting |
| `--mcp` | bool | `false` | Start MCP server subprocess alongside HTTP server |

**Usage Examples:**
```bash
mdemg serve
mdemg serve --port 9999 --auto-migrate
mdemg serve --mcp
```

**See Also:** `mdemg start`, `mdemg config show`

---

## Database Management

### `mdemg db`

Parent command for database management subcommands. Use `mdemg db <subcommand>`.

Subcommands: `start`, `stop`, `status`, `shell`, `migrate`, `reset`, `backup`

---

### `mdemg db start`

**Synopsis:** `mdemg db start [flags]`

Start a lightweight Neo4j Docker container for local development. Uses reduced memory settings (1GB heap, 512MB page cache). Data is persisted in a Docker volume. The container name and volume are scoped to the current project directory, so multiple projects can each have their own isolated Neo4j instance. If the default port (7687) is already in use, an available port is automatically selected from the range 7687-7787.

> **Prerequisite:** Docker must be installed and running. On macOS, install via [Docker Desktop for Mac](https://docs.docker.com/desktop/install/mac-install/) or `brew install --cask docker`. On Linux, install Docker Engine. On Windows, install [Docker Desktop for Windows](https://docs.docker.com/desktop/install/windows-install/).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `7687` | Bolt protocol port |
| `--http-port` | int | `7474` | HTTP browser port |
| `--password` | string | `"mdemg-dev"` | Neo4j password |

**Usage Examples:**
```bash
mdemg db start
mdemg db start --port 7688 --password mypassword
```

**See Also:** `mdemg db stop`, `mdemg db status`

---

### `mdemg db stop`

**Synopsis:** `mdemg db stop [flags]`

Stop the MDEMG Neo4j development container. Data volume is preserved unless the container is removed.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--remove` | bool | `false` | Also remove the container (data volume is preserved) |

**Usage Examples:**
```bash
mdemg db stop
mdemg db stop --remove
```

**See Also:** `mdemg db start`, `mdemg db status`

---

### `mdemg db status`

**Synopsis:** `mdemg db status`

Show database container status (running/stopped/not created), port mappings, volume name, and schema migration status. No flags.

**Usage Examples:**
```bash
mdemg db status
```

**See Also:** `mdemg db start`, `mdemg db migrate`

---

### `mdemg db shell`

**Synopsis:** `mdemg db shell`

Open an interactive Cypher shell connected to the running Neo4j container. Uses `docker exec` to run `cypher-shell` inside the container. No flags.

**Usage Examples:**
```bash
mdemg db shell
```

**See Also:** `mdemg db start`, `mdemg db status`

---

### `mdemg db migrate`

**Synopsis:** `mdemg db migrate [flags]`

Apply pending Neo4j schema migrations. By default, uses migrations embedded in the binary. Use `--migrations-dir` to override with a filesystem directory (useful during development).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--status` | bool | `false` | Show migration status only (do not apply) |
| `--dry-run` | bool | `false` | Show what would be applied without applying |
| `--migrations-dir` | string | `""` | Override embedded migrations with filesystem directory |

**Usage Examples:**
```bash
mdemg db migrate                                    # Apply all pending migrations
mdemg db migrate --status                           # Show current/available versions
mdemg db migrate --dry-run                          # Preview what would be applied
mdemg db migrate --migrations-dir ./migrations/cypher
```

**See Also:** `mdemg db status`, `mdemg serve --auto-migrate`

---

### `mdemg db reset`

**Synopsis:** `mdemg db reset [flags]`

Delete nodes from the database, either for a specific space or all non-protected spaces. Protected spaces (e.g., `mdemg-dev` for conversation memory) are never deleted.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--space-id` | string | `""` | Delete only this space ID (required unless `--all`) |
| `--all` | bool | `false` | Delete all non-protected spaces (requires confirmation) |
| `--yes`, `-y` | bool | `false` | Skip confirmation prompt |

**Usage Examples:**
```bash
mdemg db reset --space-id my-test-space
mdemg db reset --all --yes
```

**See Also:** `mdemg space delete`

---

### `mdemg db backup`

Parent command for backup management. Subcommands: `trigger`, `list`, `config`.

Backups run automatically when the server is running (daily partial, weekly full). Backup files are stored in `~/.mdemg/backups/` (gitignored by default). Only the most recent backups per type are retained based on retention configuration.

---

### `mdemg db backup trigger`

**Synopsis:** `mdemg db backup trigger [flags]`

Trigger an immediate database backup.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | `"partial_space"` | Backup type: `partial_space` (Cypher-based, no downtime) or `full` (Docker neo4j-admin dump, requires stopping) |

**Usage Examples:**
```bash
mdemg db backup trigger
mdemg db backup trigger --type full
```

**See Also:** `mdemg db backup list`, `mdemg db backup config`

---

### `mdemg db backup list`

**Synopsis:** `mdemg db backup list [flags]`

List existing backups with their ID, type, size, and creation date.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `10` | Maximum number of backups to show |
| `--type` | string | `""` | Filter by type: `partial_space` or `full` |

**Usage Examples:**
```bash
mdemg db backup list
mdemg db backup list --type full --limit 5
```

**See Also:** `mdemg db backup trigger`, `mdemg db backup config`

---

### `mdemg db backup config`

**Synopsis:** `mdemg db backup config`

Show the current backup configuration including schedule intervals, retention settings, and storage directory. No flags.

**Usage Examples:**
```bash
mdemg db backup config
```

**See Also:** `mdemg db backup trigger`, `mdemg db backup list`

---

## Memory & Ingestion

### `mdemg ingest`

**Synopsis:** `mdemg ingest --path <dir> [flags]`

Walk a codebase and ingest files into MDEMG with optimized batch processing, configurable timeouts, and optional LLM-generated summaries.

#### Core Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--path` | string | *required* | Path to codebase to ingest |
| `--space-id` | string | `"codebase"` | MDEMG space ID |
| `--endpoint` | string | `""` | MDEMG endpoint (default: from `LISTEN_ADDR` in `.env`) |
| `--batch` | int | `100` | Batch size for ingestion (optimal for ~15/s per worker) |
| `--workers` | int | `4` | Number of parallel workers |
| `--timeout` | int | `300` | HTTP timeout in seconds |
| `--delay` | int | `50` | Delay between batches in ms |
| `--retries` | int | `3` | Max retries per batch on failure |
| `--retry-delay` | int | `2000` | Initial retry delay in ms (doubles each retry) |
| `--consolidate` | bool | `true` | Run consolidation after ingestion |
| `--dry-run` | bool | `false` | Print what would be ingested without doing it |
| `--verbose` | bool | `false` | Verbose output |

#### Filter Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--exclude` | string | `".git,vendor,node_modules,.worktrees"` | Comma-separated directories to exclude |
| `--include-tests` | bool | `false` | Include test files (`*_test.go`, `*.test.ts`, `*.spec.ts`) |
| `--include-md` | bool | `true` | Include markdown files (`*.md`) |
| `--include-ts` | bool | `true` | Include TypeScript/JavaScript files |
| `--include-py` | bool | `true` | Include Python files |
| `--include-java` | bool | `true` | Include Java files |
| `--include-rust` | bool | `true` | Include Rust files |
| `--limit` | int | `0` | Limit number of elements to ingest (0 = no limit) |
| `--extract-symbols` | bool | `true` | Extract code symbols (constants, functions, classes) for evidence-locked retrieval |

#### Incremental Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--incremental` | bool | `false` | Only ingest files changed since last commit (uses `git diff`) |
| `--since` | string | `"HEAD~1"` | Git commit to compare against for incremental mode |
| `--archive-deleted` | bool | `true` | Archive nodes for deleted files in incremental mode |

#### Output Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--quiet` | bool | `false` | Suppress all non-error output |
| `--log-file` | string | `""` | Write logs to file instead of stderr |
| `--progress-json` | bool | `false` | Emit structured JSON progress lines to stdout (logs go to stderr) |

#### LLM Summary Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--llm-summary` | bool | `true` | Use LLM to generate semantic summaries (requires `OPENAI_API_KEY`) |
| `--llm-summary-model` | string | `"gpt-4o-mini"` | Model for LLM summaries |
| `--llm-summary-batch` | int | `10` | Files per LLM API call for summaries |
| `--llm-summary-provider` | string | `"openai"` | LLM provider for summaries (`openai`/`ollama`) |

#### Performance Guards

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--max-file-size` | int | `1048576` | Max file size in bytes to process (default: 1MB) |
| `--max-elements-per-file` | int | `500` | Max elements to extract per file |
| `--max-symbols-per-file` | int | `1000` | Max symbols to extract per file |
| `--preset` | string | `""` | Exclusion preset: `default`, `ml_cuda`, `web_monorepo` |
| `--speed` | string | `""` | Speed preset: `fast`, `balanced`, `thorough` |

#### Speed Presets

The `--speed` flag applies a named preset that tunes multiple flags at once. Individual flag overrides still take precedence (e.g., `--speed fast --llm-summary=true` keeps LLM summaries on).

| Setting | `fast` | `balanced` | `thorough` |
|---------|--------|------------|------------|
| `--workers` | `8` | `4` | `2` |
| `--batch` | `200` | `100` | `50` |
| `--delay` | `0` | `50` | `100` |
| `--llm-summary` | `false` | `true` | `true` |
| `--extract-symbols` | `true` | `true` | `true` |
| `--consolidate` | `false` | `true` | `true` |
| `--max-file-size` | `524288` (512KB) | `1048576` (1MB) | `2097152` (2MB) |

- **`fast`** -- Maximizes throughput by disabling LLM summaries and post-ingest consolidation. Best for large codebases or rapid iteration.
- **`balanced`** -- Default-equivalent settings. Good for most projects.
- **`thorough`** -- Fewer workers, larger file limit, all enrichment enabled. Best for pre-benchmark or quality-critical runs.

#### Info Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--list-languages` | bool | `false` | List supported languages and exit |

**Usage Examples:**
```bash
mdemg ingest --path .
mdemg ingest --path ./src --space-id my-project --workers 8
mdemg ingest --path . --incremental --since HEAD~5
mdemg ingest --path . --dry-run --verbose
mdemg ingest --path . --llm-summary=false --exclude ".git,vendor,dist"
mdemg ingest --path . --preset ml_cuda --max-file-size 2097152
mdemg ingest --path . --list-languages

# Fast ingest for large codebases
mdemg ingest --path . --speed fast

# Thorough ingest with maximum quality
mdemg ingest --path . --speed thorough

# Fast speed with LLM summaries kept on
mdemg ingest --path . --speed fast --llm-summary=true
```

**See Also:** `mdemg consolidate`, `mdemg extract-symbols`, `mdemg watch`

---

### `mdemg consolidate`

**Synopsis:** `mdemg consolidate [flags]`

Run the consolidation pipeline to build higher-level concept nodes from ingested data. Supports legacy (co-occurrence based), hidden layer (DBSCAN clustering), and multi-layer consolidation modes.

#### Legacy Mode Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--min-cluster-size` | int | `3` | Minimum observations per cluster |
| `--weight-threshold` | float64 | `0.5` | Minimum edge weight for inclusion |
| `--max-promotions` | int | `50` | Maximum new concept nodes per run |

#### Hidden Layer Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--hidden-layer` | bool | `false` | Enable hidden layer consolidation |
| `--multi-layer` | bool | `false` | Enable multi-layer consolidation (L0-L5) |
| `--hidden-eps` | float64 | `0.3` | DBSCAN epsilon (max distance for neighborhood) |
| `--hidden-min-samples` | int | `3` | DBSCAN minimum samples to form cluster |
| `--hidden-max-nodes` | int | `100` | Max hidden nodes to create per run |
| `--hidden-fwd-alpha` | float64 | `0.6` | Weight of current embedding in forward pass |
| `--hidden-fwd-beta` | float64 | `0.4` | Weight of aggregated embedding in forward pass |
| `--hidden-bwd-self` | float64 | `0.2` | Weight of self in backward pass |
| `--hidden-bwd-base` | float64 | `0.5` | Weight of base signal in backward pass |
| `--hidden-bwd-concept` | float64 | `0.3` | Weight of concept signal in backward pass |

#### Operation Mode Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--legacy` | bool | `false` | Run only legacy (co-occurrence) consolidation |
| `--forward-only` | bool | `false` | Run only the forward pass |
| `--backward-only` | bool | `false` | Run only the backward pass |
| `--cluster-only` | bool | `false` | Run only clustering (no forward/backward) |

#### Common Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `true` | Preview mode -- no modifications |
| `--space-id` | string | `""` | Space ID to process (or set `MDEMG_SPACE_ID`) |

**Usage Examples:**
```bash
mdemg consolidate --space-id codebase --dry-run=false
mdemg consolidate --hidden-layer --space-id codebase --dry-run=false
mdemg consolidate --multi-layer --space-id codebase --dry-run=false
mdemg consolidate --legacy --max-promotions 100
```

**See Also:** `mdemg ingest`, `mdemg decay`, `mdemg prune`

---

### `mdemg embeddings check`

**Synopsis:** `mdemg embeddings check`

Test the embedding pipeline by generating an actual test embedding. Reports the configured provider, model, dimensions, and whether the pipeline is working. No flags.

**Usage Examples:**
```bash
mdemg embeddings check
```

**See Also:** `mdemg config show`, `mdemg config validate`

---

### `mdemg extract-symbols`

**Synopsis:** `mdemg extract-symbols --path <dir> [flags]`

Extract code symbols (functions, classes, constants) from source files using Tree-sitter and store them in the graph for evidence-locked retrieval.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--path` | string | *required* | Path to codebase |
| `--space-id` | string | `"codebase"` | MDEMG space ID |
| `--neo4j-uri` | string | `""` | Neo4j URI |
| `--neo4j-user` | string | `""` | Neo4j username |
| `--neo4j-pass` | string | `""` | Neo4j password |
| `--workers` | int | `8` | Number of parallel workers |
| `--dry-run` | bool | `false` | Print what would be extracted without writing |
| `--verbose` | bool | `false` | Verbose output |
| `--exclude` | string | `""` | Comma-separated directories to exclude |
| `--json` | bool | `false` | Output results as JSON |

**Usage Examples:**
```bash
mdemg extract-symbols --path .
mdemg extract-symbols --path ./src --workers 16 --verbose
mdemg extract-symbols --path . --dry-run --json
```

**See Also:** `mdemg ingest`

---

## Configuration

### `mdemg config`

Parent command for configuration management. Subcommands: `show`, `validate`, `set-secret`, `get-secret`, `list-secrets`.

---

### `mdemg config show`

**Synopsis:** `mdemg config show [flags]`

Display the effective configuration with source annotations (yaml/env/default).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output as JSON |

**Usage Examples:**
```bash
mdemg config show
mdemg config show --json
```

**See Also:** `mdemg config validate`, `mdemg init`

---

### `mdemg config validate`

**Synopsis:** `mdemg config validate`

Validate the configuration by checking YAML syntax, probing Neo4j connectivity, and testing embedding provider reachability. No flags.

**Usage Examples:**
```bash
mdemg config validate
```

**See Also:** `mdemg config show`, `mdemg embeddings check`

---

### `mdemg config set-secret`

**Synopsis:** `mdemg config set-secret <key> [value]`

Store a secret in the system keychain (via `go-keyring`). If `value` is omitted, prompts interactively (stdin is masked). Known keys are mapped to environment variables automatically.

Known secret keys:
- `neo4j-password` -> `NEO4J_PASS`
- `openai-api-key` -> `OPENAI_API_KEY`
- `jwt-secret` -> `AUTH_JWT_SECRET`
- `linear-webhook` -> `LINEAR_WEBHOOK_SECRET`

**Usage Examples:**
```bash
mdemg config set-secret openai-api-key sk-abc123
mdemg config set-secret neo4j-password    # prompts interactively
```

> **Platform note:** Secrets are stored in macOS Keychain, Linux Secret Service (GNOME Keyring / KDE Wallet), or Windows Credential Manager under the service name `mdemg`.

**See Also:** `mdemg config get-secret`, `mdemg config list-secrets`

---

### `mdemg config get-secret`

**Synopsis:** `mdemg config get-secret <key>`

Retrieve a secret from the system keychain and print it to stdout. No flags.

**Usage Examples:**
```bash
mdemg config get-secret openai-api-key
```

**See Also:** `mdemg config set-secret`, `mdemg config list-secrets`

---

### `mdemg config list-secrets`

**Synopsis:** `mdemg config list-secrets`

List all known secret keys and whether they have values stored in the system keychain. No flags.

**Usage Examples:**
```bash
mdemg config list-secrets
```

**See Also:** `mdemg config set-secret`, `mdemg config get-secret`

---

### `mdemg hooks`

Parent command for git hook management. Subcommands: `install`, `uninstall`, `list`.

---

### `mdemg hooks install`

**Synopsis:** `mdemg hooks install [flags]`

Install MDEMG git hooks into the repository's `.git/hooks/` directory.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | `"git"` | Hook type to install: `git`, `claude`, or `all` |
| `--force` | bool | `false` | Overwrite existing hooks |
| `--space-id` | string | `""` | Space ID to configure in hooks |
| `--server-url` | string | `""` | MDEMG server URL to configure in hooks |

**Usage Examples:**
```bash
mdemg hooks install
mdemg hooks install --force --space-id my-project
mdemg hooks install --type claude --server-url http://localhost:9999
mdemg hooks install --type all --space-id my-project --force
```

**See Also:** `mdemg hooks uninstall`, `mdemg hooks list`

---

### `mdemg hooks uninstall`

**Synopsis:** `mdemg hooks uninstall [flags]`

Remove MDEMG git hooks from the repository.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | `"git"` | Hook type to uninstall |

**Usage Examples:**
```bash
mdemg hooks uninstall
```

**See Also:** `mdemg hooks install`, `mdemg hooks list`

---

### `mdemg hooks list`

**Synopsis:** `mdemg hooks list`

List installed MDEMG hooks and their status. No flags.

**Usage Examples:**
```bash
mdemg hooks list
```

**See Also:** `mdemg hooks install`, `mdemg hooks uninstall`

---

### `mdemg sidecar`

Parent command for sidecar lifecycle management. The sidecar manages MDEMG as a dependency of your project with a formal lifecycle: `init -> install -> up -> running -> down -> stopped`.

Subcommands: `init`, `status`, `install`, `up`, `down`, `restart`, `upgrade`, `doctor`, `attach-agent`, `detach-agent`, `generate-hooks`, `quickstart`, `uninstall`

---

### `mdemg sidecar init`

**Synopsis:** `mdemg sidecar init [flags]`

Initialize sidecar configuration for the current project. Generates `~/.mdemg/sidecar.yaml`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--profile` | string | `"local"` | Deployment profile |
| `--agents` | string | `"claude-code"` | Comma-separated agent adapters to configure |
| `--dry-run` | bool | `false` | Preview changes without writing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |
| `--defaults` | bool | `false` | Accept all defaults without prompting |
| `--endpoint` | string | `""` | MDEMG server endpoint |
| `--host` | string | `""` | Host for remote profiles |

**Usage Examples:**
```bash
mdemg sidecar init
mdemg sidecar init --profile local --agents claude-code,cursor
mdemg sidecar init --dry-run --format json
```

**See Also:** `mdemg sidecar install`, `mdemg sidecar status`

---

### `mdemg sidecar status`

**Synopsis:** `mdemg sidecar status [flags]`

Show the current sidecar lifecycle state and configuration summary.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar status
mdemg sidecar status --format json
```

**See Also:** `mdemg sidecar doctor`, `mdemg sidecar up`

---

### `mdemg sidecar install`

**Synopsis:** `mdemg sidecar install [flags]`

Install sidecar dependencies (Neo4j, hooks, IDE configs) based on `sidecar.yaml`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview changes without executing |
| `--no-auto-fix` | bool | `false` | Do not auto-fix detected issues |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar install
mdemg sidecar install --dry-run
```

**See Also:** `mdemg sidecar init`, `mdemg sidecar up`

---

### `mdemg sidecar up`

**Synopsis:** `mdemg sidecar up [flags]`

Start all sidecar services (Neo4j, MDEMG server).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview actions without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar up
mdemg sidecar up --format json
```

**See Also:** `mdemg sidecar down`, `mdemg sidecar restart`

---

### `mdemg sidecar down`

**Synopsis:** `mdemg sidecar down [flags]`

Stop all sidecar services.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview actions without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar down
```

**See Also:** `mdemg sidecar up`, `mdemg sidecar restart`

---

### `mdemg sidecar restart`

**Synopsis:** `mdemg sidecar restart [flags]`

Restart all sidecar services (down + up).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview actions without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar restart
```

**See Also:** `mdemg sidecar up`, `mdemg sidecar down`

---

### `mdemg sidecar upgrade`

**Synopsis:** `mdemg sidecar upgrade [flags]`

Detect version drift and perform a controlled upgrade cycle. Compares the running sidecar version (from lock file) with the current CLI binary version. If they differ, performs: `down -> install -> up`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Report version drift and planned actions without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |
| `--skip-restart` | bool | `false` | Stop at install -- don't call `up` (leaves state as `installed`) |

**Usage Examples:**
```bash
mdemg sidecar upgrade
mdemg sidecar upgrade --dry-run
mdemg sidecar upgrade --skip-restart
```

**See Also:** `mdemg sidecar status`, `mdemg sidecar install`

---

### `mdemg sidecar doctor`

**Synopsis:** `mdemg sidecar doctor [flags]`

Run health checks on the sidecar installation. Checks include: `config.valid`, `neo4j.reachable`, `api.healthy`, `cms.resume`, `cms.observe`, `ollama.reachable`, `ollama.models`. For remote profiles, also checks `ssh.reachable` and `docker-context.valid`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar doctor
mdemg sidecar doctor --format json
```

**See Also:** `mdemg sidecar status`, `mdemg config validate`

---

### `mdemg sidecar attach-agent`

**Synopsis:** `mdemg sidecar attach-agent <adapter-name> [flags]`

Attach an AI agent adapter to the sidecar configuration. Merges MDEMG's MCP server config into the agent's config file with backup, idempotency, and dry-run support.

For `claude-code`, this also automatically sets `enableAllProjectMcpServers: true` in `.claude/settings.local.json` to ensure MCP servers are not silently disabled. Use `--no-settings` to opt out.

Supported adapters: `claude-code`, `codex`

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview changes without executing |
| `--print-only` | bool | `false` | Print the MCP config payload to stdout and exit |
| `--no-settings` | bool | `false` | Skip enabling `enableAllProjectMcpServers` in settings |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar attach-agent claude-code
mdemg sidecar attach-agent codex --dry-run
mdemg sidecar attach-agent claude-code --print-only
mdemg sidecar attach-agent claude-code --no-settings
```

**See Also:** `mdemg sidecar detach-agent`

---

### `mdemg sidecar detach-agent`

**Synopsis:** `mdemg sidecar detach-agent <adapter-name> [flags]`

Detach an AI agent adapter from the sidecar configuration.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview changes without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |

**Usage Examples:**
```bash
mdemg sidecar detach-agent cursor
```

**See Also:** `mdemg sidecar attach-agent`

---

### `mdemg sidecar generate-hooks`

**Synopsis:** `mdemg sidecar generate-hooks [flags]`

Generate Claude Code hooks for CMS integration. Writes two hook scripts using the endpoint and space ID from sidecar configuration, enabling project-scoped CMS memory:

- **`session-start.sh`** -- Restores CMS memory context on every session start (calls `/v1/conversation/resume`), displays RSIC health, and detects anomalies.
- **`prompt-context.sh`** -- Recalls relevant CMS context and Jiminy guidance for each user prompt (calls `/v1/conversation/recall` and `/v1/jiminy/guide`).

Both hooks are automatically registered in `.claude/settings.local.json`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `"text"` | Output format (`text`, `json`) |
| `--dry-run` | bool | `false` | Preview changes without writing |

**Usage Examples:**
```bash
mdemg sidecar generate-hooks
mdemg sidecar generate-hooks --dry-run
mdemg sidecar generate-hooks --format json
```

**See Also:** `mdemg hooks install`

---

### `mdemg sidecar quickstart`

**Synopsis:** `mdemg sidecar quickstart [flags]`

One-command sidecar onboarding. Runs the full setup sequence, skipping steps that are already done and stopping on the first failure:

1. `mdemg sidecar init --defaults`
2. `mdemg sidecar install`
3. `mdemg sidecar up`
4. `mdemg sidecar attach-agent <agents>`
5. `mdemg sidecar generate-hooks`

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--profile` | string | `"local"` | Runtime profile (`local`, `studio-remote`) |
| `--agents` | string | `"claude-code"` | Comma-separated adapter names (`claude-code`, `codex`) |
| `--endpoint` | string | `""` | Override sidecar API endpoint |
| `--format` | string | `"text"` | Output format (`text`, `json`) |
| `--dry-run` | bool | `false` | Show what would happen without making changes |

**Usage Examples:**
```bash
mdemg sidecar quickstart
mdemg sidecar quickstart --profile local --agents claude-code
mdemg sidecar quickstart --endpoint http://macstudio.local:9999
mdemg sidecar quickstart --dry-run
```

**See Also:** `mdemg sidecar init`, `mdemg sidecar status`

---

### `mdemg sidecar uninstall`

**Synopsis:** `mdemg sidecar uninstall [flags]`

Uninstall sidecar and clean up all managed resources.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview changes without executing |
| `--format` | string | `"text"` | Output format (`text`, `json`) |
| `--force` | bool | `false` | Force uninstall even if state is unexpected |
| `--keep-data` | bool | `false` | Preserve data volumes and config files |

**Usage Examples:**
```bash
mdemg sidecar uninstall
mdemg sidecar uninstall --keep-data
mdemg sidecar uninstall --force
```

**See Also:** `mdemg sidecar install`

---

### `mdemg upgrade`

**Synopsis:** `mdemg upgrade [flags]`

Self-update the `mdemg` binary to the latest GitHub release. Downloads the binary for your platform, verifies the SHA256 checksum, and replaces the current executable. On Linux, also updates systemd unit files if present in the archive and already installed on the system (both `/etc/systemd/system/` and `/usr/local/share/mdemg/systemd/`), then runs `systemctl daemon-reload`.

> **Note:** If you installed via a package manager, prefer that manager's update mechanism instead: `brew upgrade mdemg` (macOS), `sudo apt update && sudo apt upgrade mdemg` (Debian/Ubuntu), or `scoop update mdemg` (Windows).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Check for updates without installing |
| `--force` | bool | `false` | Install even if already at latest version |

**Usage Examples:**
```bash
mdemg upgrade
mdemg upgrade --dry-run
mdemg upgrade --force
```

**See Also:** `mdemg version`

---

### `mdemg menubar` (macOS)

Parent command for managing the MDEMG menu bar companion app. The menu bar app provides a system tray icon showing server health status and quick access to MDEMG features.

Subcommands: `start`, `stop`, `restart`, `status`

On `mdemg init`, the menu bar app is automatically downloaded from GitHub releases and installed to `~/Applications`. Use `--no-menubar` to skip this.

> **Platform note:** This command is macOS-only. On Linux, use `mdemg sidebar` instead. On Windows, the companion app is not available via CLI.

### `mdemg menubar start`

**Synopsis:** `mdemg menubar start`

Launch the MDEMG menu bar app. Searches for `MdemgMenuBar.app` in `~/Applications`, `/Applications`, and via Spotlight.

### `mdemg menubar stop`

**Synopsis:** `mdemg menubar stop`

Stop the running menu bar app. Sends SIGTERM, polls for exit (up to 10s), then force kills if needed.

### `mdemg menubar restart`

**Synopsis:** `mdemg menubar restart`

Stop and relaunch the menu bar app.

### `mdemg menubar status`

**Synopsis:** `mdemg menubar status`

Show whether the menu bar app is running (with PID) and its install path.

**Usage Examples:**
```bash
mdemg menubar start
mdemg menubar stop
mdemg menubar restart
mdemg menubar status
```

**See Also:** `mdemg init --no-menubar`

---

### `mdemg sidebar` (Linux)

Parent command for managing the MDEMG Linux sidebar companion app. The sidebar app provides a system tray icon showing server health status and quick access to MDEMG features.

Subcommands: `start`, `stop`, `restart`, `status`

On `mdemg init`, the sidebar app is automatically downloaded from GitHub releases and installed to `/usr/local/bin/`. Use `--no-sidebar` to skip this.

> **Platform note:** This command is Linux-only. On macOS, use `mdemg menubar` instead.

### `mdemg sidebar start`

**Synopsis:** `mdemg sidebar start`

Launch the MDEMG sidebar app. Searches for `mdemg-sidebar` in `/usr/local/bin/`, `~/.local/bin/`, and on `$PATH`.

### `mdemg sidebar stop`

**Synopsis:** `mdemg sidebar stop`

Stop the running sidebar app. Sends SIGTERM, polls for exit (up to 10s), then force kills if needed.

### `mdemg sidebar restart`

**Synopsis:** `mdemg sidebar restart`

Stop and relaunch the sidebar app.

### `mdemg sidebar status`

**Synopsis:** `mdemg sidebar status`

Show whether the sidebar app is running (with PID) and its install path.

**Usage Examples:**
```bash
mdemg sidebar start
mdemg sidebar stop
mdemg sidebar restart
mdemg sidebar status
```

**See Also:** `mdemg init --no-sidebar`

---

## Advanced

### `mdemg mcp`

**Synopsis:** `mdemg mcp`

Start the MCP (Model Context Protocol) server in stdio mode. This is typically launched as a subprocess by `mdemg serve --mcp` or configured in `.claude/mcp.json` for direct IDE integration. Communicates via JSON-RPC over stdin/stdout. No CLI flags.

**Usage Examples:**
```bash
mdemg mcp
```

**See Also:** `mdemg serve --mcp`, `mdemg start --mcp`

---

### `mdemg decay`

**Synopsis:** `mdemg decay [flags]`

Apply time-based decay to Hebbian learning edges (CO_ACTIVATED_WITH) and prune edges that fall below the threshold.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--decay-rate` | float64 | `0.1` | Weight decay rate per day of inactivity |
| `--prune-threshold` | float64 | `0.01` | Weight threshold below which edges are pruned |
| `--min-evidence` | int | `3` | Minimum evidence_count to protect from pruning |
| `--older-than` | int | `7` | Only process edges older than N days |
| `--dry-run` | bool | `true` | Preview mode -- no modifications |
| `--space-id` | string | `""` | Space ID to process (or set `MDEMG_SPACE_ID`) |
| `--batch-size` | int | `1000` | Process items in batches of this size |

**Usage Examples:**
```bash
mdemg decay --space-id codebase --dry-run=false
mdemg decay --decay-rate 0.2 --prune-threshold 0.05 --older-than 14
mdemg decay --space-id codebase --dry-run
```

**See Also:** `mdemg prune`, `mdemg consolidate`

---

### `mdemg prune`

**Synopsis:** `mdemg prune [flags]`

Prune weak edges, tombstone orphan nodes, and optionally merge redundant nodes. Requires `NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASS` environment variables.

#### Edge Pruning

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--weight-threshold` | float64 | `0.01` | Minimum weight to keep (below = prune candidate) |
| `--min-evidence` | int | `3` | Minimum evidence_count to protect from pruning |
| `--older-than-days` | int | `30` | Only prune edges older than N days |

#### Node Tombstoning

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--retention-days` | int | `90` | Days without observation to tombstone |
| `--max-degree` | int | `1` | Max edges for orphan detection |

#### Node Merging

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--similarity-threshold` | float64 | `0.98` | Vector similarity threshold for merge |
| `--merge-enabled` | bool | `false` | Enable node merging (more destructive) |
| `--vector-index` | string | `"memNodeEmbedding"` | Vector index name for similarity search |

#### Processing

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `true` | Preview mode -- no modifications |
| `--space-id` | string | `""` | Space ID to process (or set `MDEMG_SPACE_ID`) |
| `--batch-size` | int | `1000` | Process items in batches of this size |

**Usage Examples:**
```bash
mdemg prune --space-id codebase --dry-run
mdemg prune --space-id codebase --dry-run=false --weight-threshold 0.05
mdemg prune --merge-enabled --similarity-threshold 0.95 --dry-run=false
```

**See Also:** `mdemg decay`, `mdemg consolidate`

---

### `mdemg watch`

**Synopsis:** `mdemg watch [flags]`

Watch a directory for file changes and auto-ingest modified files. Uses `fsnotify` with debouncing.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--space-id` | string | `""` | MDEMG space ID |
| `--path` | string | `"."` | Directory to watch |
| `--endpoint` | string | `""` | MDEMG server endpoint |
| `--extensions` | string | (default list) | Comma-separated file extensions to watch |
| `--exclude` | string | (default list) | Comma-separated directories to exclude |
| `--debounce` | int | `500` | Debounce delay in milliseconds |

**Usage Examples:**
```bash
mdemg watch --space-id my-project --path ./src
mdemg watch --debounce 1000 --extensions ".go,.py,.ts"
```

**See Also:** `mdemg ingest`

---

### `mdemg space`

Parent command for space management. Subcommands: `export`, `import`, `list`, `info`, `serve`, `pull`, `delete`, `rename`, `copy`.

Requires `NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASS` environment variables for all Neo4j operations.

---

### `mdemg space export`

**Synopsis:** `mdemg space export [flags]`

Export a MDEMG space to a `.mdemg` file. Supports selective export via profiles and filters.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--space-id` | string | `""` | Space ID to export (or set `MDEMG_SPACE_ID`) |
| `--output` | string | `""` | Output `.mdemg` file path (default: `<space-id>.mdemg`) |
| `--profile` | string | `"full"` | Export profile: `full`, `codebase`, `cms`, `learned`, `metadata`, `shareable` |
| `--repo` | string | `""` | Git repo path; export fails unless repo is clean and up to date |
| `--skip-git-check` | bool | `false` | Skip pre-export git check even when `--repo` is set |
| `--chunk-size` | int | `500` | Nodes per chunk |
| `--no-embeddings` | bool | `false` | Exclude embedding vectors to reduce size |
| `--no-observations` | bool | `false` | Exclude observations |
| `--no-symbols` | bool | `false` | Exclude symbol nodes |
| `--no-learned-edges` | bool | `false` | Exclude CO_ACTIVATED_WITH edges |
| `--min-layer` | int | `0` | Minimum layer to export (0 = all) |
| `--max-layer` | int | `0` | Maximum layer to export (0 = all) |
| `--since-timestamp` | string | `""` | Export only entities updated after this (ISO8601) |
| `--since-cursor` | string | `""` | Opaque cursor from prior export `next_cursor` |
| `--obs-types` | string | `""` | Filter L0 nodes by obs_type (comma-separated) |
| `--tags` | string | `""` | Filter nodes by tag presence (comma-separated, any match) |
| `--exclude-volatile` | bool | `false` | Skip volatile/ungraduated observations (stability < 0.8) |
| `--only-pinned` | bool | `false` | Only export pinned observations (L1+ always included) |

**Usage Examples:**
```bash
mdemg space export --space-id codebase
mdemg space export --space-id codebase --profile codebase --no-embeddings
mdemg space export --space-id codebase --output backup.mdemg --chunk-size 1000
mdemg space export --space-id codebase --since-timestamp 2025-01-01T00:00:00Z
# Shareable knowledge export (domain knowledge only, no volatile/session data)
mdemg space export --space-id mdemg-dev --profile shareable --output team-knowledge.mdemg
```

**See Also:** `mdemg space import`, `mdemg space pull`

---

### `mdemg space import`

**Synopsis:** `mdemg space import --input <file> [flags]`

Import a MDEMG space from a `.mdemg` file into Neo4j.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--input` | string | *required* | Input `.mdemg` file path |
| `--conflict` | string | `"skip"` | On node collision: `skip`, `overwrite`, or `error` |
| `--consolidate` | bool | `false` | Run hidden layer consolidation after import |
| `--re-embed` | bool | `false` | Re-generate embeddings for imported nodes |
| `--target-space` | string | `""` | Import into a different space_id (remaps all values) |

**Usage Examples:**
```bash
mdemg space import --input codebase.mdemg
mdemg space import --input backup.mdemg --conflict overwrite
# Import shared knowledge into a different space with re-embedding and consolidation
mdemg space import --input team-knowledge.mdemg --target-space my-project --consolidate --re-embed
```

**See Also:** `mdemg space export`

---

### `mdemg space list`

**Synopsis:** `mdemg space list`

List all MDEMG spaces in the Neo4j database with their node count and max layer. No flags.

**Usage Examples:**
```bash
mdemg space list
```

**See Also:** `mdemg space info`

---

### `mdemg space info`

**Synopsis:** `mdemg space info [flags]`

Show detailed information for a specific MDEMG space including node count, edges, observations, symbols, max layer, schema version, embedding dimensions, and edge types.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--space-id` | string | `""` | Space ID (or set `MDEMG_SPACE_ID`) |

**Usage Examples:**
```bash
mdemg space info --space-id codebase
```

**See Also:** `mdemg space list`

---

### `mdemg space serve`

**Synopsis:** `mdemg space serve [flags]`

Run a gRPC server for remote space export/import operations. Optionally enables the DevSpace hub for multi-agent collaboration.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `50051` | gRPC listen port |
| `--enable-devspace` | bool | `false` | Enable DevSpace hub (RegisterAgent, ListExports, PublishExport, PullExport) |
| `--devspace-data-dir` | string | `".devspace/data"` | Directory for DevSpace export files |

**Usage Examples:**
```bash
mdemg space serve
mdemg space serve --port 50052 --enable-devspace
```

**See Also:** `mdemg space pull`, `mdemg space export`

---

### `mdemg space pull`

**Synopsis:** `mdemg space pull --remote <host:port> [flags]`

Pull a space from a remote gRPC server and save it to a `.mdemg` file.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--remote` | string | *required* | Remote gRPC address (`host:port`) |
| `--space-id` | string | `""` | Space ID to pull (or set `MDEMG_SPACE_ID`) |
| `--output` | string | `""` | Output `.mdemg` file path (default: `<space-id>.mdemg`) |

**Usage Examples:**
```bash
mdemg space pull --remote ci-server:50051 --space-id codebase
mdemg space pull --remote ci-server:50051 --space-id codebase --output local-copy.mdemg
```

**See Also:** `mdemg space serve`, `mdemg space import`

---

### `mdemg space delete`

**Synopsis:** `mdemg space delete [flags]`

Delete all nodes and edges belonging to a space from Neo4j. Protected spaces (e.g., `mdemg-dev`) cannot be deleted. Deletion is performed in batches and is irreversible.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--space-id` | string | `""` | Space ID to delete (or set `MDEMG_SPACE_ID`) |
| `--yes`, `-y` | bool | `false` | Skip confirmation prompt |

**Usage Examples:**
```bash
mdemg space delete --space-id my-test-space
mdemg space delete --space-id my-test-space --yes
```

**See Also:** `mdemg db reset`

---

### `mdemg space rename`

**Synopsis:** `mdemg space rename --from <old> --to <new>`

Rename a space by updating the `space_id` property on all its nodes. Protected spaces cannot be renamed. The target space name must not already exist.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | *required* | Current space ID |
| `--to` | string | *required* | New space ID |

**Usage Examples:**
```bash
mdemg space rename --from old-project --to new-project
```

**See Also:** `mdemg space copy`, `mdemg space list`

---

### `mdemg space copy`

**Synopsis:** `mdemg space copy --from <source> --to <target>`

Copy a space by duplicating all its nodes with a new `space_id`. Edges between nodes within the space are also duplicated. New nodes receive fresh `node_id` values to avoid collisions. The target space name must not already exist.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | *required* | Source space ID |
| `--to` | string | *required* | Target space ID |

**Usage Examples:**
```bash
mdemg space copy --from production --to staging
```

**See Also:** `mdemg space rename`, `mdemg space list`

---

### `mdemg plugin`

Parent command for plugin management. Subcommands: `scaffold`, `validate`.

---

### `mdemg plugin scaffold`

**Synopsis:** `mdemg plugin scaffold --name <name> --type <type> [flags]`

Generate a complete plugin scaffold with manifest, handler, and build files. Creates: `manifest.json`, `main.go`, `handler.go`, `Makefile`, `README.md`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | *required* | Plugin name |
| `--type` | string | *required* | Module type: `INGESTION`, `REASONING`, or `APE` |
| `--output` | string | `"./plugins"` | Output directory for the plugin |
| `--version` | string | `"1.0.0"` | Initial version for the plugin |

**Usage Examples:**
```bash
mdemg plugin scaffold --name "My Parser" --type INGESTION
mdemg plugin scaffold --name "Custom Ranker" --type REASONING --output ./my-plugins
mdemg plugin scaffold --name "Background Task" --type APE --version 2.0.0
```

**See Also:** `mdemg plugin validate`

---

### `mdemg plugin validate`

**Synopsis:** `mdemg plugin validate [flags]`

Validate a plugin's manifest, proto compliance, health, and lifecycle. Performs comprehensive validation including manifest structure, gRPC service implementation, health check responsiveness, and complete lifecycle (handshake, health, shutdown).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--plugin` | string | `""` | Path to plugin directory (required unless `--socket` is set) |
| `--socket` | string | `""` | Path to running plugin's Unix socket or named pipe |
| `--manifest-only` | bool | `false` | Validate only the manifest |
| `--proto-only` | bool | `false` | Validate only proto compliance |
| `--health-only` | bool | `false` | Validate only health check (requires `--socket`) |
| `--lifecycle-only` | bool | `false` | Validate only lifecycle (requires `--socket`) |
| `--json` | bool | `false` | Output results as JSON |
| `--verbose` | bool | `false` | Show detailed validation output |
| `--no-color` | bool | `false` | Disable colored output |

**Usage Examples:**
```bash
mdemg plugin validate --plugin ./plugins/my-plugin
mdemg plugin validate --plugin ./plugins/my-plugin --manifest-only
mdemg plugin validate --socket /tmp/mdemg-plugins/my-plugin.sock --health-only
mdemg plugin validate --plugin ./plugins/my-plugin --json
```

> **Platform note:** On macOS/Linux, plugins communicate via Unix sockets (`/tmp/mdemg-plugins/*.sock`). On Windows, plugins use named pipes (`\\.\pipe\mdemg-*`).

**See Also:** `mdemg plugin scaffold`

---

### `mdemg demo`

**Synopsis:** `mdemg demo [flags]`

Seed sample observations into a demo space and demonstrate MDEMG features including recall, consolidation, and knowledge graph capabilities. Requires a running MDEMG server.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--endpoint` | string | `"http://localhost:9999"` | MDEMG server endpoint |

**Usage Examples:**
```bash
mdemg demo
mdemg demo --endpoint http://localhost:8080
```

**See Also:** `mdemg start`, `mdemg ingest`

---

### `mdemg teardown`

**Synopsis:** `mdemg teardown [flags]`

Remove all MDEMG artifacts from the current project.

**Instance scope** (default): Stops server, removes Docker container/volume, deletes Neo4j space data, removes keyring secrets, uninstalls hooks, cleans up IDE configs, removes `.mdemg/` directory, and deregisters from companion apps.

**Full scope** (`--full`): Additionally removes system-level artifacts: binary, plugins, completions, and systemd units (Linux).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Preview what would be removed without executing |
| `--format` | string | `text` | Output format (`text`, `json`) |
| `--yes`, `-y` | bool | `false` | Skip confirmation prompt |
| `--force` | bool | `false` | Allow deletion of protected spaces |
| `--full` | bool | `false` | Include system-level cleanup |
| `--export` | bool | `false` | Export CMS/RSIC/Jiminy data before teardown |
| `--keep-data` | bool | `false` | Preserve Neo4j volume |
| `--space-id` | string | `""` | Target specific space (defaults to project config) |

**Usage Examples:**
```bash
# Preview what will be removed
mdemg teardown --dry-run

# Teardown without prompts
mdemg teardown --yes

# Export data before teardown
mdemg teardown --export --yes

# Full system removal
mdemg teardown --full --yes
```

**See Also:** `mdemg stop`, `mdemg db stop`, `mdemg hooks uninstall`

---

## Neural Training Commands

These Python CLI entrypoints are installed alongside the `mdemg` binary by the sidecar installer. They operate on training data collected by `NEURAL_DATA_COLLECTION` and manage cross-encoder model checkpoints in `NEURAL_MODEL_DIR`.

### `mdemg-neural-train`

Fine-tune a cross-encoder re-ranker model from collected JSONL training data.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-dir` | string | `.mdemg/neural/training-data` | Directory containing training JSONL files |
| `--model-dir` | string | `.mdemg/neural/models` | Output directory for model checkpoints |
| `--base-model` | string | `cross-encoder/ms-marco-MiniLM-L-6-v2` | Pre-trained base model to fine-tune |
| `--epochs` | int | `3` | Number of training epochs |
| `--batch-size` | int | `16` | Training batch size |
| `--val-split` | float | `0.1` | Fraction of data to use for validation |
| `--min-samples` | int | `100` | Minimum number of training samples required |
| `--from-checkpoint` | string | `""` | Resume training from an existing checkpoint path |

**Usage Examples:**
```bash
mdemg-neural-train                              # Train with defaults
mdemg-neural-train --epochs 5 --batch-size 32   # Custom training params
mdemg-neural-train --from-checkpoint .mdemg/neural/models/2026-03-19T10-00-00
```

---

### `mdemg-neural-train-protocol`

Fine-tune a tier prediction model from J17 protocol JSONL data collected during live usage. The model predicts the optimal encoding tier (T1/T2/T3) for each constraint based on agent context and trust score.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-dir` | string | `.mdemg/neural/protocol-data` | Directory containing protocol JSONL files |
| `--model-dir` | string | `.mdemg/neural/models/protocol-tier` | Output directory for versioned model checkpoints |
| `--base-model` | string | `cross-encoder/ms-marco-MiniLM-L-6-v2` | Pre-trained base model to fine-tune |
| `--epochs` | int | `3` | Number of training epochs |
| `--batch-size` | int | `16` | Training batch size |
| `--val-split` | float | `0.1` | Fraction of data for validation |
| `--min-samples` | int | `500` | Minimum samples required before training starts |
| `--from-checkpoint` | string | `""` | Resume training from existing model version |

**Usage Examples:**
```bash
mdemg-neural-train-protocol                           # Train with defaults (needs 500+ samples)
mdemg-neural-train-protocol --min-samples 100         # Lower threshold for testing
mdemg-neural-train-protocol --epochs 5 --batch-size 32
mdemg-neural-train-protocol --from-checkpoint .mdemg/neural/models/protocol-tier/v1
```

Training data is collected automatically by the J17 protocol data collector (`internal/jiminy/protocol_data_collector.go`) during normal Jiminy guidance operations when `J17_ENABLED=true`. Each record contains constraint code, tier used, token count, comprehension score, trust score, and agent action outcome.

---

### `mdemg-neural-evaluate`

Offline evaluation comparing neural cross-encoder scores against baseline LLM re-rank scores.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--model-path` | string | *(required)* | Path to trained model checkpoint directory |
| `--test-data` | string | `.mdemg/neural/training-data` | Directory containing test JSONL files |
| `--base-model` | string | `cross-encoder/ms-marco-MiniLM-L-6-v2` | Base model for comparison baseline |
| `--top-k` | int | `10` | Number of top results to evaluate per query |
| `--output` | string | `""` | Write evaluation report to file (stdout if empty) |

**Usage Examples:**
```bash
mdemg-neural-evaluate --model-path .mdemg/neural/models/2026-03-19T10-00-00
mdemg-neural-evaluate --model-path ./model --top-k 5 --output eval-report.json
```

---

## Environment Variable Reference

The following table lists all environment variables recognized by MDEMG, grouped by category. These are parsed in `config.FromEnv()`.

### Setting Environment Variables

#### macOS / Linux
```bash
# Current session
export NEO4J_URI="bolt://localhost:7687"
export OPENAI_API_KEY="sk-abc123"

# Persist via shell profile (~/.zshrc, ~/.bashrc)
echo 'export OPENAI_API_KEY="sk-abc123"' >> ~/.zshrc
```

#### Windows
```powershell
# PowerShell (current session only)
$env:NEO4J_URI = "bolt://localhost:7687"
$env:OPENAI_API_KEY = "sk-abc123"

# Persist across sessions (user-scoped, no admin required)
[Environment]::SetEnvironmentVariable("OPENAI_API_KEY", "sk-abc123", "User")

# Machine-scoped (requires admin)
[Environment]::SetEnvironmentVariable("NEO4J_URI", "bolt://localhost:7687", "Machine")
```

> **Recommended:** Use `mdemg config set-secret` for sensitive values (API keys, passwords) instead of environment variables. This stores them securely in the system keychain.

### Core / Connection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LISTEN_ADDR` | string | `":9999"` | HTTP server listen address |
| `NEO4J_URI` | string | *required* | Neo4j Bolt URI (e.g., `bolt://localhost:7687`) |
| `NEO4J_USER` | string | *required* | Neo4j username |
| `NEO4J_PASS` | string | *required* | Neo4j password |
| `NEO4J_BOLT_PORT` | int | `7687` | Preferred Bolt port for container creation |
| `NEO4J_HTTP_PORT` | int | `7474` | Preferred HTTP port for container creation |
| `REQUIRED_SCHEMA_VERSION` | int | auto-detect | Required schema version (auto-detects from embedded migrations if unset) |

### Retrieval & Scoring

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `VECTOR_INDEX_NAME` | string | `"memNodeEmbedding"` | Neo4j vector index name |
| `DEFAULT_CANDIDATE_K` | int | `200` | Vector search candidate count |
| `DEFAULT_TOP_K` | int | `20` | Final top-K results returned |
| `DEFAULT_HOP_DEPTH` | int | `2` | Graph traversal hop depth |
| `MAX_NEIGHBORS_PER_NODE` | int | `50` | Max neighbors fetched per node during activation |
| `MAX_TOTAL_EDGES_FETCHED` | int | `5000` | Max total edges fetched per query |
| `SCORING_ALPHA` | float64 | `0.60` | Vector similarity weight |
| `SCORING_BETA` | float64 | `0.20` | Activation weight |
| `SCORING_GAMMA` | float64 | `0.15` | Recency weight |
| `SCORING_DELTA` | float64 | `0.05` | Confidence weight |
| `SCORING_PHI` | float64 | `0.08` | Hub penalty coefficient |
| `SCORING_KAPPA` | float64 | `0.12` | Redundancy penalty coefficient |
| `SCORING_RHO` | float64 | `0.05` | Recency decay rate per day (legacy fallback) |
| `SCORING_RHO_L0` | float64 | `0.05` | Layer 0 decay rate per day |
| `SCORING_RHO_L1` | float64 | `0.02` | Layer 1 decay rate per day |
| `SCORING_RHO_L2` | float64 | `0.01` | Layer 2+ decay rate per day |
| `SCORING_CONFIG_BOOST` | float64 | `1.15` | Score multiplier for config nodes |
| `SCORING_PATH_BOOST` | float64 | `0.15` | Boost coefficient for path-matching nodes |

### Hebbian Learning

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LEARNING_EDGE_CAP_PER_REQUEST` | int | `200` | Max learning edges created per request |
| `LEARNING_MIN_ACTIVATION` | float64 | `0.20` | Minimum activation to create/strengthen edge |
| `LEARNING_ETA` | float64 | `0.02` | Hebbian learning rate |
| `LEARNING_MU` | float64 | `0.01` | Hebbian decay/regularization |
| `LEARNING_WMIN` | float64 | `0.0` | Minimum weight clamp |
| `LEARNING_WMAX` | float64 | `1.0` | Maximum weight clamp |
| `LEARNING_DECAY_PER_DAY` | float64 | `0.05` | Time-based decay per day of inactivity |
| `LEARNING_PRUNE_THRESHOLD` | float64 | `0.05` | Weight below which edges are pruned |
| `LEARNING_MAX_EDGES_PER_NODE` | int | (unset) | Max CO_ACTIVATED_WITH edges per node |

### Top-Level LLM

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LLM_PROVIDER` | string | `"openai"` | Top-level text-gen LLM provider |
| `LLM_MODEL` | string | `"gpt-5-nano"` | Top-level text-gen LLM model |
| `LLM_ENDPOINT` | string | (uses OpenAI endpoint) | Override endpoint for LLM text-generation |

### Embedding Provider

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EMBEDDING_PROVIDER` | string | `""` | Embedding provider: `openai`, `ollama`, or empty (disabled) |
| `OPENAI_API_KEY` | string | `""` | OpenAI API key |
| `OPENAI_MODEL` | string | `"text-embedding-3-large"` | OpenAI embedding model |
| `OPENAI_ENDPOINT` | string | `"https://api.openai.com/v1"` | OpenAI API endpoint |
| `OLLAMA_ENDPOINT` | string | `"http://localhost:11434"` | Ollama API endpoint |
| `OLLAMA_MODEL` | string | `"qwen3-embedding:8b"` | Ollama embedding model |

### Embedding Cache

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EMBEDDING_CACHE_ENABLED` | bool | `true` | Enable embedding LRU cache |
| `EMBEDDING_CACHE_SIZE` | int | `1000` | LRU cache capacity |

### Query Cache

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `QUERY_CACHE_ENABLED` | bool | `true` | Enable query result cache |
| `QUERY_CACHE_CAPACITY` | int | `500` | LRU cache capacity |
| `QUERY_CACHE_TTL_SECONDS` | int | `300` | Cache TTL in seconds |

### Semantic Edge on Ingest

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SEMANTIC_EDGE_ON_INGEST` | bool | `true` | Create semantic edges on ingest |
| `SEMANTIC_EDGE_TOP_N` | int | `5` | Max similar nodes to query |
| `SEMANTIC_EDGE_MIN_SIMILARITY` | float64 | `0.7` | Minimum similarity threshold |
| `SEMANTIC_EDGE_INITIAL_WEIGHT` | float64 | `0.5` | Initial edge weight |

### Batch Ingest

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `BATCH_INGEST_MAX_ITEMS` | int | `500` | Maximum items per batch request |

### HTTP Server

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HTTP_READ_TIMEOUT` | int | `600` | Read timeout in seconds |
| `HTTP_WRITE_TIMEOUT` | int | `600` | Write timeout in seconds |

### Anomaly Detection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ANOMALY_DETECTION_ENABLED` | bool | `true` | Enable anomaly detection |
| `ANOMALY_DUPLICATE_THRESHOLD` | float64 | `0.95` | Vector similarity threshold for duplicates |
| `ANOMALY_OUTLIER_STDDEVS` | float64 | `2.0` | Standard deviations for outlier detection |
| `ANOMALY_STALE_DAYS` | int | `30` | Days after which an update is stale |
| `ANOMALY_MAX_CHECK_MS` | int | `100` | Maximum time for anomaly checks in ms |

### Temporal Reasoning

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TEMPORAL_ENABLED` | bool | `true` | Enable temporal reasoning |
| `TEMPORAL_SOFT_BOOST` | float64 | `3.0` | Gamma multiplier for recency boost |
| `TEMPORAL_HARD_FILTER` | bool | `true` | Enable hard-mode time range filtering |
| `TEMPORAL_SOURCE_TYPE_DECAY` | bool | `false` | Enable source-type-specific decay rates |
| `SCORING_RHO_DOCUMENTATION` | float64 | `0.01` | Decay rate for documentation |
| `SCORING_RHO_CONFIG` | float64 | `0.03` | Decay rate for config |
| `SCORING_RHO_CONVERSATION` | float64 | `0.10` | Decay rate for conversation |
| `SCORING_RHO_CHANGELOG` | float64 | `0.08` | Decay rate for changelog |
| `TEMPORAL_STALE_REF_DAYS` | int | `0` | Stale reference days (0 = disabled) |
| `TEMPORAL_STALE_REF_MAX_PENALTY` | float64 | `0.15` | Max stale reference penalty |

### Hidden Layer

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HIDDEN_LAYER_ENABLED` | bool | `true` | Enable hidden layer processing |
| `HIDDEN_LAYER_CLUSTER_EPS` | float64 | `0.1` | DBSCAN epsilon |
| `HIDDEN_LAYER_MIN_SAMPLES` | int | `3` | DBSCAN min samples |
| `HIDDEN_LAYER_MAX_HIDDEN` | int | `100` | Max hidden nodes per run |
| `HIDDEN_LAYER_MAX_CLUSTER_SIZE` | int | `200` | Max members per cluster |
| `HIDDEN_LAYER_PATH_GROUP_DEPTH` | int | `2` | Path segments for pre-grouping |
| `HIDDEN_LAYER_BATCH_SIZE` | int | `0` | Batch size for clustering (0 = no limit) |
| `HIDDEN_LAYER_FORWARD_ALPHA` | float64 | `0.6` | Forward pass current embedding weight |
| `HIDDEN_LAYER_FORWARD_BETA` | float64 | `0.4` | Forward pass aggregated embedding weight |
| `HIDDEN_LAYER_BACKWARD_SELF` | float64 | `0.2` | Backward pass self weight |
| `HIDDEN_LAYER_BACKWARD_BASE` | float64 | `0.5` | Backward pass base signal weight |
| `HIDDEN_LAYER_BACKWARD_CONC` | float64 | `0.3` | Backward pass concept signal weight |

### Concept Merge

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONCEPT_MERGE_ENABLED` | bool | `true` | Enable concept deduplication |
| `CONCEPT_MERGE_THRESHOLD` | float64 | `0.90` | Cosine similarity threshold for merging |

### Edge-Type Attention

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EDGE_ATTENTION_ENABLED` | bool | `true` | Enable query-aware edge weighting |
| `EDGE_ATTENTION_CO_ACTIVATED` | float64 | `0.85` | Base weight for CO_ACTIVATED_WITH edges |
| `EDGE_ATTENTION_ASSOCIATED` | float64 | `0.65` | Base weight for ASSOCIATED_WITH edges |
| `EDGE_ATTENTION_GENERALIZES` | float64 | `0.65` | Base weight for GENERALIZES edges |
| `EDGE_ATTENTION_ABSTRACTS_TO` | float64 | `0.60` | Base weight for ABSTRACTS_TO edges |
| `EDGE_ATTENTION_TEMPORAL` | float64 | `0.45` | Base weight for TEMPORALLY_ADJACENT edges |
| `EDGE_ATTENTION_CODE_BOOST` | float64 | `1.2` | Multiplier for CO_ACTIVATED in code queries |
| `EDGE_ATTENTION_ARCH_BOOST` | float64 | `1.5` | Multiplier for hierarchical in arch queries |

### Query-Aware Expansion

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `QUERY_AWARE_EXPANSION_ENABLED` | bool | `true` | Enable attention-based neighbor selection |
| `QUERY_AWARE_ATTENTION_WEIGHT` | float64 | `0.5` | Weight of query-node similarity vs edge weight |
| `NODE_EMBEDDING_CACHE_SIZE` | int | `5000` | LRU cache size for node embeddings |

### Edge Type Strategy

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EDGE_TYPE_STRATEGY` | string | `"hybrid"` | Strategy: `all`, `structural_first`, `learned_only`, `hybrid` |
| `HYBRID_SWITCH_HOP` | int | `1` | Hop depth to switch from structural to learned |

### Hybrid Retrieval (BM25)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HYBRID_RETRIEVAL_ENABLED` | bool | `true` | Enable hybrid vector+BM25 retrieval |
| `BM25_TOP_K` | int | `100` | BM25 candidate count |
| `BM25_WEIGHT` | float64 | `0.3` | BM25 weight in RRF fusion |
| `VECTOR_WEIGHT` | float64 | `0.7` | Vector weight in RRF fusion |

### LLM Re-ranking

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RERANK_ENABLED` | bool | `false` | Enable LLM re-ranking |
| `RERANK_PROVIDER` | string | `""` | LLM provider for rerank |
| `RERANK_MODEL` | string | `"gpt-4o-mini"` | Model for re-ranking |
| `RERANK_TOP_N` | int | `30` | Candidates to re-rank |
| `RERANK_WEIGHT` | float64 | `0.4` | Weight of rerank score |
| `RERANK_TIMEOUT_MS` | int | `3000` | Timeout for rerank call in ms |

### LLM Summaries

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LLM_SUMMARY_ENABLED` | bool | `true` | Enable LLM summaries |
| `LLM_SUMMARY_PROVIDER` | string | `"openai"` | Provider for summaries |
| `LLM_SUMMARY_MODEL` | string | `"gpt-4o-mini"` | Model for summaries |
| `LLM_SUMMARY_MAX_TOKENS` | int | `150` | Max tokens per summary |
| `LLM_SUMMARY_BATCH_SIZE` | int | `10` | Files per API call |
| `LLM_SUMMARY_TIMEOUT_MS` | int | `30000` | Request timeout in ms |
| `LLM_SUMMARY_CACHE_SIZE` | int | `5000` | Max cached summaries |

### SME Synthesis (Phase 101)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SYNTHESIS_ENABLED` | bool | `false` | Enable LLM synthesis in `/consult` |
| `SYNTHESIS_PROVIDER` | string | `"openai"` | LLM provider |
| `SYNTHESIS_MODEL` | string | `"gpt-4o-mini"` | Model for synthesis |
| `SYNTHESIS_MAX_TOKENS` | int | `2000` | Max tokens for response |
| `SYNTHESIS_TIMEOUT_MS` | int | `30000` | Timeout in ms |

### Intent Translation (Phase 102)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `INTENT_ENABLED` | bool | `false` | Enable query rewriting |
| `INTENT_PROVIDER` | string | `"openai"` | LLM provider |
| `INTENT_MODEL` | string | `"gpt-4o-mini"` | Model for translation |
| `INTENT_MAX_TOKENS` | int | `150` | Max tokens for rewritten query |
| `INTENT_TIMEOUT_MS` | int | `2000` | Timeout in ms |

### Dynamic Emergence (Phase 103)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EMERGENCE_ENABLED` | bool | `false` | Enable LLM-driven concept naming |
| `EMERGENCE_PROVIDER` | string | `"openai"` | LLM provider |
| `EMERGENCE_MODEL` | string | `"gpt-4o-mini"` | Model for naming |
| `EMERGENCE_MAX_TOKENS` | int | `500` | Max tokens for response |
| `EMERGENCE_TIMEOUT_MS` | int | `10000` | Timeout in ms |
| `EMERGENCE_MIN_WEIGHT` | float64 | `0.3` | Min CO_ACTIVATED_WITH weight for clustering |
| `EMERGENCE_MIN_CLUSTER_SIZE` | int | `3` | Min nodes per cluster |
| `EMERGENCE_MAX_CLUSTERS` | int | `10` | Max clusters per run |

### Active MCP Guardrails (Phase 104)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GUARDRAIL_ENABLED` | bool | `false` | Enable guardrail validation |
| `GUARDRAIL_PROVIDER` | string | `"openai"` | LLM provider |
| `GUARDRAIL_MODEL` | string | `"gpt-4o-mini"` | Model for evaluation |
| `GUARDRAIL_MAX_TOKENS` | int | `1000` | Max tokens for response |
| `GUARDRAIL_TIMEOUT_MS` | int | `5000` | Timeout in ms |
| `GUARDRAIL_MAX_CONSTRAINTS` | int | `10` | Max constraints per evaluation |

### Global Meta-Learning (Phase 105)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `METALEARN_ENABLED` | bool | `false` | Enable cross-space concept promotion |
| `METALEARN_GLOBAL_SPACE_ID` | string | `"mdemg-global"` | Target global space |
| `METALEARN_MIN_LAYER` | int | `4` | Minimum layer for candidates |
| `METALEARN_MIN_UPDATE_COUNT` | int | `5` | Minimum update_count for candidates |
| `METALEARN_PROVIDER` | string | (from `EMERGENCE_PROVIDER`) | LLM provider |
| `METALEARN_MODEL` | string | (from `EMERGENCE_MODEL`) | LLM model |
| `METALEARN_MAX_TOKENS` | int | `500` | Max tokens for response |
| `METALEARN_TIMEOUT_MS` | int | `15000` | Timeout in ms |

### Plugin System

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PLUGINS_ENABLED` | bool | `true` | Enable plugin system |
| `PLUGINS_DIR` | string | `"./plugins"` | Plugin directory path |
| `PLUGIN_SOCKET_DIR` | string | `/tmp/mdemg-plugins` | Unix socket directory (macOS/Linux) or named pipe prefix (Windows: `\\.\pipe\`) |

### Linear Integration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LINEAR_TEAM_ID` | string | `""` | Default team ID for issue creation |
| `LINEAR_WORKSPACE_ID` | string | `""` | Workspace identifier |
| `LINEAR_WEBHOOK_SECRET` | string | `""` | HMAC-SHA256 signing secret |
| `LINEAR_WEBHOOK_SPACE_ID` | string | `""` | Space for webhook observations |

### Webhooks & File Watcher

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WEBHOOK_CONFIGS` | string | `""` | Format: `source:secret:space_id,...` |
| `FILE_WATCHER_ENABLED` | bool | `false` | Enable in-process file watching |
| `FILE_WATCHER_CONFIGS` | string | `""` | Format: `space_id:/path:extensions:debounce_ms,...` |

### Conflict & Orphan Cleanup

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONFLICT_LOG_ENABLED` | bool | `true` | Enable structured conflict logging |
| `ORPHAN_CLEANUP_INTERVAL_HOURS` | int | `0` | Scheduled cleanup interval (0 = disabled) |

### Optimistic Retry

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `OPTIMISTIC_RETRY_ENABLED` | bool | `true` | Enable optimistic locking with retry |
| `OPTIMISTIC_RETRY_MAX_ATTEMPTS` | int | `5` | Max retry attempts |
| `OPTIMISTIC_RETRY_BASE_DELAY_MS` | int | `10` | Initial delay in ms |
| `OPTIMISTIC_RETRY_MAX_DELAY_MS` | int | `1000` | Max delay in ms |
| `OPTIMISTIC_RETRY_MULTIPLIER` | float64 | `2.0` | Exponential backoff multiplier |

### Edge Staleness

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EDGE_STALENESS_CASCADE_ENABLED` | bool | `true` | Enable edge staleness cascade |
| `EDGE_STALENESS_REFRESH_BATCH_SIZE` | int | `100` | Edges per refresh call |
| `EDGE_STALENESS_RECLUSTER_THRESHOLD` | float64 | `0.3` | Centroid drift threshold |

### Capability Gap Detection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GAP_LOW_SCORE_THRESHOLD` | float64 | `0.5` | Avg score below this = poor query |
| `GAP_MIN_OCCURRENCES` | int | `3` | Min occurrences to create a gap |
| `GAP_ANALYSIS_WINDOW_HOURS` | int | `24` | Time window for analysis |
| `GAP_METRICS_WINDOW_SIZE` | int | `1000` | Queries to keep in history |

### RSIC (Self-Improvement)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RSIC_MICRO_ENABLED` | bool | `false` | Enable per-request micro cycles |
| `RSIC_MESO_PERIOD_HOURS` | int | `6` | Hours between meso cycles |
| `RSIC_MESO_PERIOD_SESSIONS` | int | `10` | Sessions between meso cycles |
| `RSIC_MACRO_CRON` | string | `"0 3 * * 0"` | Cron expression for macro cycles |
| `RSIC_MAX_NODE_PRUNE_PCT` | float64 | `0.05` | Max % nodes a single action can prune |
| `RSIC_MAX_EDGE_PRUNE_PCT` | float64 | `0.10` | Max % edges a single action can prune |
| `RSIC_ROLLBACK_WINDOW` | int | `3600` | Seconds to keep rollback snapshots |
| `RSIC_WATCHDOG_ENABLED` | bool | `true` | Enable decay watchdog |
| `RSIC_WATCHDOG_CHECK_SEC` | int | `300` | Seconds between watchdog checks |
| `RSIC_WATCHDOG_DECAY_RATE` | float64 | `0.1` | Decay score increase per hour without cycle |
| `RSIC_NUDGE_THRESHOLD` | float64 | `0.3` | Nudge-level escalation threshold |
| `RSIC_WARN_THRESHOLD` | float64 | `0.6` | Warn-level escalation threshold |
| `RSIC_FORCE_THRESHOLD` | float64 | `0.9` | Force-trigger escalation threshold |
| `RSIC_CALIBRATION_DAYS` | int | `30` | Days of history for calibration |
| `RSIC_MAX_HISTORY_ENTRIES` | int | `1000` | Max calibration history entries per type |
| `RSIC_MIN_CONFIDENCE` | float64 | `0.3` | Minimum confidence to execute an action |
| `RSIC_TRIGGER_COOLDOWN_SEC` | int | `300` | Cooldown between triggers from same source |
| `RSIC_TRIGGER_DEDUPE_SEC` | int | `600` | Dedupe window for identical trigger IDs |
| `RSIC_WATCHDOG_SPACE_ID` | string | `"mdemg-dev"` | Space monitored by watchdog |
| `RSIC_PERSISTENCE_ENABLED` | bool | `true` | Enable write-behind persistence |
| `SPACE_PRUNE_INTERVAL_HOURS` | int | `24` | Auto-prune interval (0 = disabled) |

### Backup & Restore

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `BACKUP_ENABLED` | bool | `true` | Enable backup module |
| `BACKUP_STORAGE_DIR` | string | `"~/.mdemg/backups"` | Backup storage directory |
| `BACKUP_FULL_CMD` | string | `"docker"` | Command for full backups |
| `BACKUP_NEO4J_CONTAINER` | string | `"mdemg-neo4j"` | Docker container name |
| `BACKUP_FULL_INTERVAL_HOURS` | int | `168` | Hours between full backups |
| `BACKUP_PARTIAL_INTERVAL_HOURS` | int | `24` | Hours between partial backups |
| `BACKUP_RETENTION_FULL_COUNT` | int | `4` | Keep last N full backups |
| `BACKUP_RETENTION_PARTIAL_COUNT` | int | `14` | Keep last N partial backups |
| `BACKUP_RETENTION_MAX_AGE_DAYS` | int | `90` | Delete backups older than N days |
| `BACKUP_RETENTION_MAX_STORAGE_GB` | int | `50` | Storage quota in GB |
| `BACKUP_RETENTION_RUN_AFTER_BACKUP` | bool | `true` | Run retention after each backup |

### Relationship Extraction

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `REL_EXTRACT_IMPORTS` | bool | `true` | Extract import relationships |
| `REL_EXTRACT_INHERITANCE` | bool | `true` | Extract inheritance relationships |
| `REL_EXTRACT_CALLS` | bool | `true` | Extract function call relationships |
| `REL_CROSS_FILE_RESOLVE` | bool | `true` | Enable cross-file symbol resolution |
| `GO_TYPES_ANALYSIS_ENABLED` | bool | `false` | Use go/types for accurate analysis |
| `REL_MAX_CALLS_PER_FUNCTION` | int | `50` | Max calls per function |
| `REL_BATCH_SIZE` | int | `500` | Batch size for relationship insertion |
| `REL_RESOLUTION_TIMEOUT_SEC` | int | `60` | Timeout for symbol resolution |

### Topology Hardening

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DYNAMIC_EDGES_ENABLED` | bool | `true` | Enable dynamic edge creation |
| `DYNAMIC_EDGE_DEGREE_CAP` | int | `10` | Max dynamic edges per node |
| `DYNAMIC_EDGE_MIN_CONFIDENCE` | float64 | `0.5` | Min confidence for dynamic edges |
| `L5_EMERGENT_ENABLED` | bool | `true` | Enable Layer 5 emergent concepts |
| `L5_BRIDGE_EVIDENCE_MIN` | int | `1` | Min bridge evidence for L5 promotion |
| `L5_SOURCE_MIN_LAYER` | int | `3` | Min layer for L5/dynamic edge sources |
| `SYMBOL_ACTIVATION_ENABLED` | bool | `true` | Enable symbol-aware activation boost |
| `SECONDARY_LABELS_ENABLED` | bool | `true` | Enable secondary node labels |
| `THEME_OF_EDGE_ENABLED` | bool | `true` | Enable THEME_OF edge creation |
| `CONSOLIDATE_ON_WATCHDOG_ENABLED` | bool | `true` | Trigger consolidation with RSIC force |

### Data Transmission & Pooling

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `COMPRESSION_ENABLED` | bool | `true` | Enable gzip compression |
| `COMPRESSION_MIN_SIZE` | int | `1024` | Min response size to compress (bytes) |
| `PAGINATION_MAX_LIMIT` | int | `500` | Max items per page |
| `PAGINATION_DEF_LIMIT` | int | `50` | Default items per page |
| `NEO4J_MAX_POOL_SIZE` | int | `100` | Max connections in pool |
| `NEO4J_ACQUIRE_TIMEOUT_SEC` | int | `60` | Connection acquire timeout |
| `NEO4J_MAX_CONN_LIFETIME_SEC` | int | `3600` | Max connection lifetime |
| `NEO4J_CONN_IDLE_TIMEOUT_SEC` | int | `0` | Idle timeout (0 = disabled) |

### Embedding Rate Limiting

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EMBEDDING_RATE_LIMIT_ENABLED` | bool | `false` | Enable embedding rate limiting |
| `EMBEDDING_OPENAI_RPS` | float64 | `500` | OpenAI requests per second |
| `EMBEDDING_OPENAI_BURST` | int | `1000` | OpenAI burst allowance |
| `EMBEDDING_OLLAMA_RPS` | float64 | `100` | Ollama requests per second |
| `EMBEDDING_OLLAMA_BURST` | int | `200` | Ollama burst allowance |

### Memory Pressure

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MEMORY_PRESSURE_ENABLED` | bool | `false` | Enable memory backpressure |
| `MEMORY_PRESSURE_THRESHOLD_MB` | int | `4096` | Heap threshold for rejection |

### Production Readiness

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RATE_LIMIT_ENABLED` | bool | `true` | Enable rate limiting |
| `RATE_LIMIT_RPS` | float64 | `100` | Requests per second |
| `RATE_LIMIT_BURST` | int | `200` | Burst allowance |
| `RATE_LIMIT_BY_IP` | bool | `true` | Per-IP vs global rate limiting |
| `CIRCUIT_BREAKER_ENABLED` | bool | `true` | Enable circuit breaking |
| `CIRCUIT_BREAKER_THRESHOLD` | int | `5` | Failures before opening |
| `CIRCUIT_BREAKER_TIMEOUT_SEC` | int | `30` | Seconds before half-open |
| `AUTH_ENABLED` | bool | `false` | Enable authentication |
| `AUTH_MODE` | string | `"none"` | Auth mode: `none`, `apikey`, `bearer` |
| `AUTH_API_KEYS` | string | `""` | Comma-separated API keys |
| `AUTH_JWT_SECRET` | string | `""` | JWT secret for bearer mode |
| `AUTH_JWT_ISSUER` | string | `""` | Expected JWT issuer |
| `AUTH_SKIP_ENDPOINTS` | string | `"/healthz,/readyz"` | Endpoints that bypass auth |
| `CORS_ENABLED` | bool | `false` | Enable CORS |
| `CORS_ALLOWED_ORIGINS` | string | `""` | Allowed origins (comma-separated or `*`) |
| `CORS_ALLOWED_METHODS` | string | `"GET,POST,PUT,DELETE"` | Allowed methods |
| `CORS_ALLOW_CREDENTIALS` | bool | `false` | Allow credentials |
| `TLS_ENABLED` | bool | `false` | Enable HTTPS |
| `TLS_CERT_FILE` | string | `""` | TLS certificate path |
| `TLS_KEY_FILE` | string | `""` | TLS key file path |
| `METRICS_ENABLED` | bool | `true` | Enable Prometheus metrics |
| `METRICS_NAMESPACE` | string | `"mdemg"` | Metrics namespace prefix |
| `GRACEFUL_SHUTDOWN_TIMEOUT_SEC` | int | `30` | Shutdown timeout in seconds |

### Logging

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_FORMAT` | string | `"text"` | Log format: `text` or `json` |
| `LOG_SKIP_HEALTH` | bool | `false` | Skip logging for health endpoints |

### CMS Meta-Cognition

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `METACOG_ENABLED` | bool | `true` | Enable meta-cognitive anomaly detection |
| `METACOG_EMPTY_RESUME_CHECK` | bool | `true` | Check for empty resume anomaly |
| `METACOG_SIGNAL_DECAY_RATE` | float64 | `0.05` | Hebbian decay per ignored signal |
| `METACOG_SIGNAL_BOOST_RATE` | float64 | `0.1` | Hebbian boost per signal response |

### CMS Configurable Defaults

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CMS_RESUME_MAX_OBS` | int | `20` | Default max observations on resume |
| `CMS_RECALL_TOP_K` | int | `10` | Default top-K for recall queries |
| `CMS_SUMMARY_MAX_CHARS` | int | `200` | Max character length for summaries |
| `CMS_JIMINY_BASE_CONFIDENCE` | float64 | `0.5` | Base confidence for Jiminy rationale |

### Context Cooler

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `COOLER_REINFORCEMENT_WINDOW_HOURS` | int | `2` | Reinforcement window |
| `COOLER_STABILITY_INCREASE_PER_REINFORCEMENT` | float64 | `0.15` | Stability increase per reinforcement |
| `COOLER_STABILITY_DECAY_RATE` | float64 | `0.1` | Daily decay for unreinforced nodes |
| `COOLER_TOMBSTONE_THRESHOLD` | float64 | `0.05` | Stability below which nodes tombstoned |
| `COOLER_GRADUATION_THRESHOLD` | float64 | `0.8` | Stability for graduation |

### Constraint Module

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_DETECTION_ENABLED` | bool | `true` | Enable constraint detection |
| `CONSTRAINT_MIN_CONFIDENCE` | float64 | `0.6` | Minimum confidence for constraint tag |
| `CONSTRAINT_PROTECT_FROM_DECAY` | bool | `true` | Protect constraint-tagged obs from tombstoning |

### Web Scraper

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SCRAPER_ENABLED` | bool | `false` | Enable web scraper module |
| `SCRAPER_DEFAULT_SPACE_ID` | string | `"web-scraper"` | Default target space |
| `SCRAPER_MAX_CONCURRENT_JOBS` | int | `3` | Max concurrent scrape jobs |
| `SCRAPER_DEFAULT_DELAY_MS` | int | `1000` | Default delay between requests |
| `SCRAPER_DEFAULT_TIMEOUT_MS` | int | `30000` | Default HTTP timeout |
| `SCRAPER_CACHE_TTL_SECONDS` | int | `3600` | robots.txt cache TTL |
| `SCRAPER_RESPECT_ROBOTS_TXT` | bool | `true` | Respect robots.txt |
| `SCRAPER_MAX_CONTENT_LENGTH_KB` | int | `500` | Max content length in KB |

### Miscellaneous

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `UNTS_ENABLED` | bool | `false` | Enable hash verification REST API |
| `UNTS_BASE_PATH` | string | `"."` | Repository root for file hashing |
| `SYNC_INTERVAL_MINUTES` | int | `0` | Scheduled sync check interval (0 = disabled) |
| `SYNC_STALE_THRESHOLD_HOURS` | int | `24` | Hours before a space is stale |
| `MDEMG_SPACE_ID` | string | `""` | Default space ID (used by CLI) |

### Jiminy Guidance (Phase Jiminy)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `JIMINY_ENABLED` | bool | `true` | Enable Jiminy inner voice guidance |
| `JIMINY_TIMEOUT_MS` | int | `6000` | Overall timeout for Guide() in ms |
| `JIMINY_MAX_ITEMS` | int | `10` | Max guidance items returned |
| `JIMINY_MIN_CONFIDENCE` | float64 | `0.3` | Minimum confidence to include item |
| `JIMINY_INCLUDE_FRONTIERS` | bool | `true` | Include frontier node suggestions |
| `JIMINY_FRONTIER_MIN_SIM` | float64 | `0.5` | Min similarity for frontier nodes |

### J17 AI-to-AI Protocol (Phase J17)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `J17_ENABLED` | bool | `false` | Enable J17 protocol endpoints and 3-tier encoding |
| `J17_CODEGEN_ENABLED` | bool | `false` | Enable LLM-powered constraint code generation |
| `J17_CODEGEN_PROVIDER` | string | (from `LLM_PROVIDER`) | LLM provider for code generation |
| `J17_CODEGEN_MODEL` | string | (from `LLM_MODEL`) | LLM model for code generation |
| `NEURAL_TIER_MODEL` | string | `""` | Path or HuggingFace model name for ML tier prediction (empty = disabled, rule-based fallback) |

### Dynamic Reclassification

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RECLASS_ENABLED` | bool | `false` | Enable dynamic reclassification |
| `RECLASS_THRESHOLD` | float64 | `0.6` | Confidence threshold for reclassification |
| `RECLASS_MAX_SAMPLE_SIZE` | int | `50` | Max nodes to sample per run |
| `RECLASS_MAX_CATEGORIES` | int | `20` | Max categories for reclassification |
| `RECLASS_MAX_ITERATIONS` | int | `5` | Max reclassification iterations |
| `RECLASS_MAX_DEPTH` | int | `3` | Max depth for recursive reclassification |
| `RECLASS_PROVIDER` | string | (from `LLM_PROVIDER`) | LLM provider |
| `RECLASS_MODEL` | string | (from `LLM_MODEL`) | Model for reclassification |
| `RECLASS_MAX_TOKENS` | int | `500` | Max tokens for reclassification response |
| `RECLASS_TIMEOUT_MS` | int | `10000` | Timeout in ms |

### Cluster Summarization

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CLUSTER_SUMMARY_ENABLED` | bool | `false` | Enable cluster summarization |
| `CLUSTER_SUMMARY_PROVIDER` | string | (from `LLM_PROVIDER`) | LLM provider |
| `CLUSTER_SUMMARY_MODEL` | string | (from `LLM_MODEL`) | Model for summaries |
| `CLUSTER_SUMMARY_MAX_TOKENS` | int | `100` | Max tokens per summary |
| `CLUSTER_SUMMARY_TIMEOUT_MS` | int | `5000` | Timeout per call in ms |
| `CLUSTER_SUMMARY_BATCH_SIZE` | int | `50` | Max nodes per consolidation run |

### ANN Learning Optimization

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS` | int | `24` | Skip decay for edges reinforced within this window (0=disabled) |
| `LEARNING_ETA_CONVERSATION_MULT` | float64 | `2.0` | Eta multiplier for conversation observations |
| `LEARNING_ETA_CONFIG_MULT` | float64 | `1.5` | Eta multiplier for config-code edges |
| `LEARNING_ETA_SAME_DIR_MULT` | float64 | `1.2` | Eta multiplier for same-directory nodes |
| `LEARNING_SCHEDULE_ENABLED` | bool | `true` | Enable maturity-based learning rate schedule |
| `LEARNING_SCHEDULE_COLD_MULT` | float64 | `2.0` | Multiplier for cold spaces (0 edges) |
| `LEARNING_SCHEDULE_LEARNING_MULT` | float64 | `1.0` | Multiplier for learning spaces (1-10k edges) |
| `LEARNING_SCHEDULE_WARM_MULT` | float64 | `0.5` | Multiplier for warm spaces (10k-50k edges) |
| `LEARNING_SCHEDULE_SAT_MULT` | float64 | `0.25` | Multiplier for saturated spaces (50k+ edges) |

### ANN Scoring Optimization

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SCORING_ACTIVATION_FLOOR` | float64 | `0.05` | Floor below which activation is zeroed |
| `SCORING_ACTIVATION_SQUARED` | bool | `true` | Enable squared activation for sharper signals |
| `SCORING_BM25_WEIGHT` | float64 | `0.15` | Weight for BM25 component in final score |
| `SCORING_BYPASS_THRESHOLD` | float64 | `0.85` | VectorSim threshold to trigger value residual bypass |
| `SCORING_BYPASS_WEIGHT` | float64 | `0.15` | Weight of bypass bonus |
| `SCORING_BYPASS_CODE_MULT` | float64 | `1.3` | Bypass multiplier for code queries |
| `SCORING_BYPASS_ARCH_MULT` | float64 | `0.5` | Bypass multiplier for architecture queries |

### ANN Activation Optimization

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ACTIVATION_STEPS` | int | `2` | Number of activation hops (1-10) |
| `ACTIVATION_LAMBDA` | float64 | `0.15` | Decay factor per hop (0.0-0.9) |
| `ACTIVATION_HOP0_MIN_WEIGHT` | float64 | `0.5` | Min edge weight for hop 0 |
| `ACTIVATION_HOP1_MIN_WEIGHT` | float64 | `0.2` | Min edge weight for hop 1 |
| `ACTIVATION_HOP2_MIN_WEIGHT` | float64 | `0.05` | Min edge weight for hop 2+ |

### Negative Feedback

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LEARNING_NEGATIVE_WEIGHT` | float64 | `0.15` | Weight reduction per negative feedback |
| `LEARNING_NEGATIVE_DECAY_MULT` | float64 | `2.0` | Decay multiplier for contradicted edges |
| `LEARNING_NEGATIVE_MAX_PER_REQUEST` | int | `20` | Max rejected nodes per request |

### Frontier Detection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `FRONTIER_MIN_EVIDENCE` | int | `3` | Min evidence_count for frontier candidates |
| `FRONTIER_MAX_OUTGOING` | int | `2` | Max outgoing edges for frontier candidates |

### Jina Cross-Encoder Reranking

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RERANK_JINA_API_KEY` | string | (none) | Jina API key |
| `RERANK_JINA_MODEL` | string | `jina-reranker-v2-base-multilingual` | Jina reranker model |
| `RERANK_JINA_URL` | string | `https://api.jina.ai/v1` | Jina API endpoint |

### L5 Grounding

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `L5_GROUNDING_MAX_EDGES` | int | `5` | Max GROUNDED_BY edges per L5 node |
| `L5_GROUNDING_MIN_SIM` | float64 | `0.4` | Min cosine similarity for grounding edge |
| `L5_GROUNDING_INITIAL_WEIGHT` | float64 | `0.5` | Initial weight for GROUNDED_BY edges |
| `EDGE_ATTENTION_GROUNDED_BY` | float64 | `0.70` | Attention weight for GROUNDED_BY edges |

### RRF (Reciprocal Rank Fusion)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RRF_CONSTANT` | int | `60` | RRF k parameter (min: 1) |

### Dynamic Port Allocation

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PORT_RANGE_START` | int | (from `LISTEN_ADDR`) | Start of fallback port range |
| `PORT_RANGE_END` | int | `PORT_RANGE_START + 100` | End of fallback port range |
| `PORT_FILE_PATH` | string | `.mdemg.port` | Port file for client discovery |

### Constraint Enforcement (FSD-2026-001 F1)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GUARDRAIL_HOOK_ENABLED` | bool | `false` | Enable PreToolUse constraint enforcement hook |
| `GUARDRAIL_HOOK_TIMEOUT_MS` | int | `3000` | Timeout for hook enforcement calls |

### Feedback Persistence (FSD-2026-001 F3)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `JIMINY_PERSISTENCE_ENABLED` | bool | `false` | Persist guidance outcomes to Neo4j |
| `CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE` | float64 | `0.03` | Confidence decrease per ignored/contradicted outcome |
| `CONSTRAINT_CONFIDENCE_BOOST_PER_POSITIVE` | float64 | `0.02` | Confidence increase per followed outcome |
| `CONSTRAINT_ARCHIVE_THRESHOLD` | float64 | `0.3` | Archive constraints below this confidence |

### Contradiction Detection (FSD-2026-001 F2)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONTRADICTION_ENABLED` | bool | `true` | Enable contradiction detection in observations |
| `CONTRADICTION_SIM_THRESHOLD` | float64 | `0.75` | Minimum similarity for contradiction candidates |
| `CONTRADICTION_MAX_CANDIDATES` | int | `20` | Max candidates for contradiction check |
| `CONTRADICTION_NLI_ENABLED` | bool | `false` | Use NLI sidecar for contradiction detection |

### Constraint Classification (FSD-2026-001 F6)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_CLASSIFIER_GATE_ENABLED` | bool | `false` | LLM gate to confirm regex constraint detections |
| `CONSTRAINT_NLI_ENABLED` | bool | `false` | Use NLI sidecar for constraint classification |

### Scope Filtering (FSD-2026-001 F7)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_SCOPE_FILTERING_ENABLED` | bool | `false` | Filter constraints by file path scope |

### Conflict Detection (FSD-2026-001 F4)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_CONFLICT_DETECTION_ENABLED` | bool | `false` | Enable pairwise constraint conflict detection |
| `CONSTRAINT_CONFLICT_SIM_THRESHOLD` | float64 | `0.6` | Similarity threshold for conflict detection |
| `CONSTRAINT_CONFLICT_MAX_PAIRS` | int | `500` | Max constraint pairs to evaluate |

### Asymmetric Learning & Determinism (FSD-2026-001 F9)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LEARNING_ASYMMETRIC_ENABLED` | bool | `false` | Enable directional co-activation edges |
| `DETERMINISM_SCORING_ENABLED` | bool | `false` | Enable determinism score computation |

### Jiminy Cache (FSD-2026-001 F10)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `JIMINY_CACHE_ENABLED` | bool | `true` | Enable guidance response cache |
| `JIMINY_CACHE_TTL_SEC` | int | `300` | Cache TTL in seconds |
| `JIMINY_CACHE_SIZE` | int | `200` | Max cache entries (LRU eviction) |
| `JIMINY_PARTIAL_TIMEOUT_MS` | int | `2000` | Timeout for partial guidance results |

### Activation Weights (FSD-2026-001 F11)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ACTIVATION_DIM_SEMANTIC_WEIGHT` | float64 | `0.6` | Semantic dimension weight in activation spreading |
| `ACTIVATION_DIM_TEMPORAL_WEIGHT` | float64 | `0.2` | Temporal dimension weight |
| `ACTIVATION_DIM_COACTIVATION_WEIGHT` | float64 | `0.2` | Co-activation dimension weight |

### Constraint Decay (FSD-2026-001 F13)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_DECAY_ENABLED` | bool | `false` | Enable automatic constraint confidence decay |
| `CONSTRAINT_DECAY_RATE_PER_WEEK` | float64 | `0.01` | Weekly decay rate for unsurfaced constraints |

### Authority Levels (FSD-2026-001 F20)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONSTRAINT_AUTHORITY_ENABLED` | bool | `false` | Enable authority-based constraint filtering |
| `CONSTRAINT_DEFAULT_AUTHORITY` | string | `team_standard` | Default authority level for new constraints |

### Operational (FSD-2026-001 F15-F18)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_HOP_DEPTH` | int | `3` | Maximum graph expansion hops in retrieval |
| `LEARNING_AUTO_PRUNE_EXCESS_ENABLED` | bool | `false` | Auto-prune excess edges during consolidation |

### Neural Re-Ranker (NR)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `NEURAL_DATA_COLLECTION` | bool | `false` | Enable training data collection for neural re-ranker |
| `NEURAL_DATA_DIR` | string | `.mdemg/neural/training-data` | Directory for training data JSONL files |
| `NEURAL_MODEL_DIR` | string | `.mdemg/neural/models` | Directory for trained cross-encoder model checkpoints |
| `NEURAL_RERANK_ENABLED` | bool | `false` | Enable neural re-rank provider |
| `NEURAL_RERANK_URL` | string | `http://localhost:8100` | Python sidecar URL |
| `NEURAL_RERANK_TIMEOUT_MS` | int | `1000` | Timeout for sidecar re-rank calls |
| `NEURAL_RERANK_FALLBACK` | string | *(from `RERANK_PROVIDER`)* | Fallback re-rank provider if sidecar fails |
| `SIDECAR_ENABLED` | bool | `false` | Master switch for Python sidecar integration |

---

## Platform-Specific Notes

### macOS

**Config file locations:**

| File | Location |
|------|----------|
| Config | `~/.mdemg/config.yaml` |
| PID file | `~/.mdemg/mdemg.pid` |
| Log file | `~/.mdemg/logs/mdemg.log` |
| Backups | `~/.mdemg/backups/` |
| Sidecar config | `~/.mdemg/sidecar.yaml` |
| Ignore file | `.mdemgignore` (in project root) |

**Companion app:** The menu bar app (`MdemgMenuBar.app`) is managed via `mdemg menubar`. Installed by default during `mdemg init`.

**Keychain:** Secrets are stored in macOS Keychain Access under the service name `mdemg`.

### Linux

**Config file locations:** Same as macOS (using `~` / `$HOME`).

**Companion app:** The sidebar app (`mdemg-sidebar`) is managed via `mdemg sidebar`. Installed by default during `mdemg init`.

**Keychain:** Secrets are stored via Secret Service (GNOME Keyring / KDE Wallet) under the service name `mdemg`.

**systemd:** When `--full` teardown is used, systemd unit files are also removed.

### Windows

**Path handling:** MDEMG accepts both forward slashes (`/`) and backslashes (`\`) in all path arguments.

**Config file locations:**

| File | Location |
|------|----------|
| Config | `%USERPROFILE%\.mdemg\config.yaml` |
| PID file | `%USERPROFILE%\.mdemg\mdemg.pid` |
| Log file | `%USERPROFILE%\.mdemg\logs\mdemg.log` |
| Backups | `%USERPROFILE%\.mdemg\backups\` |
| Sidecar config | `%USERPROFILE%\.mdemg\sidecar.yaml` |
| Ignore file | `.mdemgignore` (in project root) |

In PowerShell, `%USERPROFILE%` is equivalent to `$HOME` or `$env:USERPROFILE`.

**Docker Desktop:** Docker commands (`docker compose`, `mdemg db start`, etc.) work identically on Windows as long as [Docker Desktop for Windows](https://docs.docker.com/desktop/install/windows-install/) is installed and running. Both WSL 2 and Hyper-V backends are supported.

**Firewall:** If you encounter connection issues, ensure your Windows Firewall allows traffic on the MDEMG server port (default 9999) and Neo4j ports (7687 for Bolt, 7474 for HTTP).

**Keychain:** Secrets are stored in Windows Credential Manager under the service name `mdemg`. You can view them in Control Panel > User Accounts > Credential Manager > Windows Credentials, or via `cmdkey /list:mdemg*`.

**Plugins:** On Windows, plugins communicate via named pipes (`\\.\pipe\mdemg-*`) instead of Unix sockets. The `PLUGIN_SOCKET_DIR` default is `\\.\pipe\` (vs `/tmp/mdemg-plugins` on macOS/Linux).

---

## Command Tree Summary

```
mdemg
  Getting Started:
    init              Initialize MDEMG in the current project
    version           Print version information

  Server Lifecycle:
    start             Start the server in the background (daemon)
    stop              Stop the server
    restart           Restart the server
    status            Show server and database status
    serve             Start the HTTP server in the foreground

  Database Management:
    db
      start           Start a local Neo4j container
      stop            Stop the Neo4j container
      status          Show container and schema status
      shell           Open Cypher shell
      migrate         Apply pending migrations
      reset           Delete nodes from the database
      backup
        trigger       Trigger a manual backup
        list          List existing backups
        config        Show backup configuration

  Memory & Ingestion:
    ingest            Ingest a codebase into MDEMG
    consolidate       Run the consolidation pipeline
    embeddings
      check           Test the embedding pipeline
    extract-symbols   Extract code symbols

  Configuration:
    config
      show            Show effective configuration
      validate        Validate configuration
      set-secret      Store a secret in the system keychain
      get-secret      Retrieve a secret from the system keychain
      list-secrets    List known secret keys
    hooks
      install         Install git hooks
      uninstall       Remove git hooks
      list            List installed hooks
    sidecar
      init            Initialize sidecar configuration
      status          Show sidecar state
      install         Install sidecar dependencies
      up              Start sidecar services
      down            Stop sidecar services
      restart         Restart sidecar services
      upgrade         Upgrade sidecar version
      doctor          Run health checks
      attach-agent    Attach an AI agent adapter
      detach-agent    Detach an AI agent adapter
      generate-hooks  Generate git hooks
      quickstart      One-command sidecar onboarding
      uninstall       Uninstall sidecar
    menubar           Manage menu bar app (macOS)
      start           Launch the menu bar app
      stop            Stop the menu bar app
      restart         Restart the menu bar app
      status          Show menu bar app status
    sidebar           Manage sidebar app (Linux)
      start           Launch the sidebar app
      stop            Stop the sidebar app
      restart         Restart the sidebar app
      status          Show sidebar app status
    upgrade           Self-update the mdemg binary

  Advanced:
    mcp               Start MCP server (stdio mode)
    decay             Apply time-based decay to learning edges
    prune             Prune weak edges and orphan nodes
    watch             Watch directory for file changes
    space
      export          Export a space to .mdemg file
      import          Import a .mdemg file
      list            List all spaces
      info            Show space details
      serve           Run gRPC transfer server
      pull            Pull space from remote server
      delete          Delete a space
      rename          Rename a space
      copy            Copy a space
    plugin
      scaffold        Generate a plugin scaffold
      validate        Validate a plugin
    demo              Seed sample data and demonstrate features
    teardown          Remove all MDEMG artifacts from the project

  Neural Training (Python sidecar):
    mdemg-neural-train              Fine-tune cross-encoder re-ranker
    mdemg-neural-train-protocol     Fine-tune J17 tier prediction model
    mdemg-neural-evaluate           Evaluate cross-encoder vs baseline
```
