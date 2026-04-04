---
created: 2026-03-20
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "95"
---

# Database + Embedding + Migrations

## Summary

**Feature**: Database Bootstrapping & Migration System
**Summary**: Eliminates database bootstrapping friction with embedded Cypher migrations, auto-detect schema version, Docker container management, and embedding provider health checks. Developers no longer need manual migration scripts or external tools.

## Vision & Goals

A zero-friction developer experience is essential for adoption. Phase 95 ensures that `mdemg db migrate` and `mdemg serve --auto-migrate` handle all database setup automatically. Embedded migrations mean the binary is self-contained — no external files needed at runtime. This supports both development (fast iteration) and production (Docker Compose with `AUTO_MIGRATE=true`).

## Current State

### Architecture

**Embedded Migrations** — All Cypher migration files are embedded in the binary via Go's `//go:embed` directive. The migration runner handles complex Cypher including `//` comments, `CALL { } IN TRANSACTIONS` blocks (brace-depth tracking), and multi-statement files.

**Migration Recording** — After each migration succeeds, records a `(:Migration {version: N})` node and updates `(:SchemaMeta {key: 'schema'})` via idempotent `MERGE`.

**Auto-Detect Schema Version** — `REQUIRED_SCHEMA_VERSION` is optional. If not set (or set to 0), the server automatically detects the latest version from embedded migrations.

### Workflow

**Migration Flow:**

```bash
mdemg db migrate --status    # Show current status
mdemg db migrate --dry-run   # Preview pending migrations
mdemg db migrate             # Apply all pending (idempotent, safe to rerun)
```

**Server Start with Auto-Migrate:**

```bash
mdemg serve --auto-migrate   # Apply pending migrations, then start server
```

**Docker Container Management:**

```bash
mdemg db start               # Lightweight Neo4j container (1GB heap, 512MB page cache)
mdemg db start --port 7688   # Custom port
mdemg db status              # Check container status
mdemg db stop                # Stop (data preserved)
mdemg db stop --remove       # Stop and remove container (volume preserved)
mdemg db shell               # Open cypher-shell in running container
```

**Embedding Provider Check:**

```bash
mdemg embeddings check       # Full pipeline test (connectivity + model + test embedding)
```

Shows provider, endpoint, model, dimensions, and status. For failures, shows specific remediation steps.

**Development Override:**

```bash
mdemg db migrate --migrations-dir ./migrations   # Use filesystem instead of embedded
```

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- Migration runner is append-only — no rollback support
- `mdemg db start` creates a single-container Neo4j, not Docker Compose (use `mdemg init` for full stack)

### Risks & Gaps

- Schema version mismatch between deploy configs and actual migrations (identified in assessment as P0-2)

### Future Improvements

- Migration rollback support
- CI schema version validation

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/readyz` | Checks schema version, Neo4j connectivity | `specs/readiness.uats.json` |
| GET | `/v1/embedding/health` | Tests embedding provider | N/A |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg db migrate` | Apply pending migrations |
| `mdemg db migrate --status` | Show migration status |
| `mdemg db migrate --dry-run` | Preview pending migrations |
| `mdemg db migrate --migrations-dir <path>` | Use filesystem migrations |
| `mdemg db start [--port N] [--password P]` | Start development Neo4j container |
| `mdemg db stop [--remove]` | Stop Neo4j container |
| `mdemg db status` | Check container status |
| `mdemg db shell` | Open cypher-shell |
| `mdemg serve --auto-migrate` | Apply migrations then start server |
| `mdemg embeddings check` | Full embedding provider health check |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `REQUIRED_SCHEMA_VERSION` | auto-detect | Required schema version (0 = auto-detect from embedded migrations) |
| `AUTO_MIGRATE` | `true` (Docker) | Apply pending migrations on startup |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Neo4j | Requires — all migrations target Neo4j |
| Docker | Optional — `mdemg db start` requires Docker for container management |
| Embedding Provider | Optional — `embeddings check` requires configured provider |

## Related Files

- `migrations/embed.go` - `//go:embed *.cypher` filesystem
- `migrations/version.go` - `MaxVersion()` auto-detection
- `internal/db/migrate.go` - Migration runner (split, parse, discover, apply)
- `internal/db/migrate_test.go` - 10 unit tests for migration runner
- `internal/cli/docker.go` - Docker container management helpers
- `internal/cli/db.go` - DB subcommands
- `internal/cli/embeddings.go` - `mdemg embeddings check` command
- `internal/cli/serve.go` - `--auto-migrate` flag
- `internal/config/config.go` - REQUIRED_SCHEMA_VERSION auto-detect
