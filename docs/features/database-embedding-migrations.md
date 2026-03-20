# Database + Embedding + Migrations (Phase 95)

Phase 95 eliminates database bootstrapping friction. Developers no longer need to manually download `cypher-shell`, apply migration files via shell loops, set `REQUIRED_SCHEMA_VERSION`, or manage Docker containers with hostile memory settings.

## Migration Runner

### Embedded Migrations

All 17 Cypher migration files are embedded in the binary via Go's `//go:embed` directive. No external files needed at runtime.

```bash
# Show current migration status
mdemg db migrate --status

# Preview what would be applied
mdemg db migrate --dry-run

# Apply all pending migrations
mdemg db migrate

# Rerun is safe (idempotent)
mdemg db migrate
# "Database is up to date."
```

### Statement Splitting

The migration runner handles complex Cypher files including:

- `//` line comments (stripped)
- `CALL { } IN TRANSACTIONS` blocks (brace-depth tracking)
- Multiple statements separated by `;`
- Auto-commit mode per statement (matching cypher-shell behavior)

### Migration Recording

After each migration succeeds, the runner records a `(:Migration {version: N})` node and updates `(:SchemaMeta {key: 'schema'})`. This is idempotent via `MERGE`.

### Development Override

During development, use a filesystem directory instead of embedded migrations:

```bash
mdemg db migrate --migrations-dir ./migrations
```

## Auto-Detect Schema Version

`REQUIRED_SCHEMA_VERSION` is now optional. If not set (or set to 0), the server automatically detects the latest version from embedded migrations. Existing deployments with explicit values continue to work unchanged.

## `--auto-migrate` on Server Start

```bash
mdemg serve --auto-migrate
```

Applies pending migrations before starting the server. Useful for development and CI.

## Docker Container Management

### Start a Development Neo4j

```bash
mdemg db start
```

Creates a lightweight Docker container with reduced memory settings (1GB heap, 512MB page cache) suitable for development. Data persists in a Docker volume.

```bash
# Custom port and password
mdemg db start --port 7688 --password mypassword

# Check status
mdemg db status

# Stop (data preserved)
mdemg db stop

# Stop and remove container (volume preserved)
mdemg db stop --remove
```

### Interactive Shell

```bash
mdemg db shell
```

Opens `cypher-shell` inside the running container.

## Embedding Provider Check

```bash
mdemg embeddings check
```

Unlike `config validate` (which only does TCP/HTTP probes), `embeddings check` performs an actual test embedding to verify the full pipeline:

```
Embedding Provider Check
========================
Provider: ollama
Endpoint: http://localhost:11434
Model:    qwen3-embedding:8b

Connectivity... ok
Model check... ok
Test embedding... ok

Dimensions: 1536
Status:     working
```

For disabled providers, it shows setup instructions. For failures, it shows specific remediation steps (install URLs, pull commands, missing API keys).

## Improved Error Messages

Schema version mismatches now include actionable guidance:

```
schema version 10 < required 17 — run 'mdemg db migrate' to upgrade
```

## CI Improvements

The CI workflow no longer downloads `cypher-shell` or loops through migration files. It uses the built binary directly:

```yaml
- name: Apply Neo4j migrations
  run: ./bin/mdemg db migrate
```

`REQUIRED_SCHEMA_VERSION` is removed from CI environment variables (auto-detected).

## New Files

| File | Description |
|------|-------------|
| `migrations/embed.go` | `//go:embed *.cypher` filesystem |
| `migrations/version.go` | `MaxVersion()` auto-detection |
| `internal/db/migrate.go` | Migration runner (split, parse, discover, apply) |
| `internal/db/migrate_test.go` | 10 unit tests for migration runner |
| `internal/cli/docker.go` | Docker container management helpers |
| `internal/cli/embeddings.go` | `mdemg embeddings check` command |

## Documents Accessed

- `internal/db/neo4j.go` — AssertSchemaVersion error message
- `internal/db/schema.go` — GetSchemaVersion
- `internal/cli/db.go` — newDBCmd, DB subcommands
- `internal/cli/serve.go` — --auto-migrate flag
- `internal/cli/root.go` — command registration
- `internal/config/config.go` — REQUIRED_SCHEMA_VERSION auto-detect
- `internal/config/yaml_config.go` — schema.version validation message
- `internal/embeddings/embeddings.go` — Embedder interface
- `internal/embeddings/ollama.go` — Ollama provider
- `internal/embeddings/openai.go` — OpenAI provider
- `internal/cli/init.go` — removed dead code (countMigrations, portFromString)
- `migrations/V0001__schema_meta.cypher` — self-recording pattern
- `migrations/V0015__secondary_labels.cypher` — CALL IN TRANSACTIONS
- `.github/workflows/ci.yml` — replaced cypher-shell with mdemg db migrate
- `docs/features/unified-cli.md` — updated with new commands
- `AGENT_HANDOFF.md` — phase registry update
- `CHANGELOG.md` — unreleased entries
- `MEMORY.md` — session state update
