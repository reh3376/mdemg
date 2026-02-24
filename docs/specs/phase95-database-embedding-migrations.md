# Phase 95: Database + Embedding + Migrations

**Status:** Complete
**Branch:** `mdemg-dev01`
**Depends on:** Phase 93 (Unified CLI), Phase 94 (Config + Project Init)
**Covers:** Gap 4 (Database Management), Gap 6 (Embedding Provider), Gap 7 (Schema & Migration Management) from `docs/specs/phase92-gap-analysis.md`

---

## Overview

Phase 95 eliminates database bootstrapping friction. Before this phase, developers had to:

1. Download `cypher-shell` externally
2. Apply 17 migration files via a shell loop
3. Manually set `REQUIRED_SCHEMA_VERSION` correctly
4. Manage Docker containers with a hostile 12GB memory allocation

After Phase 95, a developer runs `mdemg db start && mdemg db migrate` and is ready.

---

## New Commands

| Command | Description |
|---------|-------------|
| `mdemg db migrate` | Apply pending schema migrations |
| `mdemg db migrate --status` | Show current/available versions and pending list |
| `mdemg db migrate --dry-run` | Preview without applying |
| `mdemg db migrate --migrations-dir DIR` | Use filesystem instead of embedded |
| `mdemg db start` | Start a lightweight Neo4j dev container |
| `mdemg db stop` | Stop the dev container |
| `mdemg db stop --remove` | Stop and remove container (volume preserved) |
| `mdemg db status` | Show container state + schema version |
| `mdemg db shell` | Open interactive cypher-shell |
| `mdemg embeddings check` | Verify embedding provider (actual test embed) |
| `mdemg serve --auto-migrate` | Apply migrations before starting server |

---

## Design Decisions

### 1. Statement Splitting

Migration files contain multiple Cypher statements separated by `;`, with `//` comments and `CALL { } IN TRANSACTIONS` blocks (V0015, V0016). The splitter:

- Strips `//` line comments
- Tracks brace depth (`{`/`}`)
- Splits on `;` only at brace depth 0
- Each statement executes in auto-commit mode (matching `cypher-shell` behavior)

### 2. Migration Recording (Runner-Side)

After each migration file's statements succeed, the runner records:

```cypher
MERGE (m:Migration {version: $ver})
ON CREATE SET m.name=$name, m.applied_at=datetime(), m.applied_by='mdemg-migrate'
```

And updates SchemaMeta:

```cypher
MERGE (s:SchemaMeta {key: 'schema'})
ON MATCH SET s.current_version = CASE WHEN s.current_version < $ver THEN $ver ELSE s.current_version END
```

This is idempotent (MERGE) — safe for V0001-V0014 which also self-record.

### 3. Embedded Migrations (`//go:embed`)

`migrations/embed.go` embeds all `*.cypher` files. `migrations/version.go` provides `MaxVersion()` for auto-detect. `internal/db/migrate.go` uses the embedded FS by default with a `--migrations-dir` flag override.

### 4. REQUIRED_SCHEMA_VERSION Made Optional

`config.FromEnv()` no longer fatally exits if unset. If 0/unset, auto-detects from `migrations.MaxVersion()`. Existing deployments with explicit values keep working.

### 5. Docker Management via CLI Subprocess

`mdemg db start` uses `os/exec` to call `docker run` (not Docker SDK). Container name: `mdemg-neo4j-dev`, volume: `mdemg-neo4j-data`, image: `neo4j:5`. Dev profile: 1GB heap, 512MB page cache.

### 6. Embedding Check: Actual Test Embed

`mdemg embeddings check` performs an actual embedding call (not just a probe) to verify the full pipeline works and reports dimensions.

---

## Files Created

| File | Description | Lines |
|------|-------------|-------|
| `migrations/embed.go` | `//go:embed *.cypher` filesystem | ~10 |
| `migrations/version.go` | `MaxVersion()` auto-detection | ~55 |
| `internal/db/migrate.go` | Migration runner core | ~260 |
| `internal/db/migrate_test.go` | 10 unit tests | ~165 |
| `internal/cli/docker.go` | Docker container management helpers | ~100 |
| `internal/cli/embeddings.go` | `mdemg embeddings check` command | ~190 |
| `docs/specs/phase95-database-embedding-migrations.md` | This spec | — |
| `docs/features/database-embedding-migrations.md` | Feature doc | — |

## Files Modified

| File | Changes |
|------|---------|
| `internal/cli/db.go` | 5 new subcommands: migrate, start, stop, status, shell |
| `internal/cli/serve.go` | `--auto-migrate` flag, `migrations.FS` import |
| `internal/cli/root.go` | Register `newEmbeddingsCmd()` |
| `internal/cli/init.go` | Removed dead code (`countMigrations`, `portFromString`), removed unused `strconv` import |
| `internal/db/neo4j.go` | Improved schema mismatch error message |
| `internal/config/config.go` | `REQUIRED_SCHEMA_VERSION` auto-detect via `migrations.MaxVersion()` |
| `internal/config/yaml_config.go` | Changed `schema.version` warning to info level |
| `.github/workflows/ci.yml` | Replaced cypher-shell with `mdemg db migrate`, removed `REQUIRED_SCHEMA_VERSION` |
| `docs/features/unified-cli.md` | Added db subcommands, embeddings command, `--auto-migrate` |
| `AGENT_HANDOFF.md` | Phase 95 entry updated |
| `CHANGELOG.md` | Unreleased entries |
| `README.md` | Updated quickstart |

---

## Testing

### Unit Tests (10 passing)

- `TestSplitStatements_Simple` — basic semicolon splitting
- `TestSplitStatements_Comments` — `//` comment stripping
- `TestSplitStatements_CALLInTransactions` — brace-depth tracking
- `TestSplitStatements_NestedBraces` — nested `{ }` blocks
- `TestSplitStatements_Empty` — empty/comment-only input
- `TestSplitStatements_NoTrailingSemicolon` — missing trailing `;`
- `TestParseMigrationFile` — version/name extraction
- `TestParseMigrationFile_Invalid` — 4 sub-tests for bad filenames
- `TestDiscoverMigrations` — sorted discovery from fs.FS
- `TestDiscoverMigrations_Empty` — no migration files

### E2E Verification

1. `mdemg db migrate --status` — shows current/available/pending
2. `mdemg db migrate` — applies all 17 migrations
3. `mdemg db migrate` (rerun) — idempotent, 0 applied
4. `mdemg db status` — shows container + schema info
5. `mdemg embeddings check` — validates configured provider
6. UATS: 190/190 variants pass (100%). UNTS hash-verification specs (19 variants) tested separately via `make test-unts-uats` (requires `UNTS_ENABLED=true`)

---

## Documents Accessed

- `internal/db/neo4j.go` — NewDriver, AssertSchemaVersion, VerifyConnectivity
- `internal/db/schema.go` — GetSchemaVersion
- `internal/cli/db.go` — newDBCmd, newDBResetCmd, protectedSpaceList
- `internal/cli/serve.go` — runServe, schema check, auto-migrate insertion point
- `internal/cli/root.go` — command registration
- `internal/config/config.go` — RequiredSchemaVersion, FromEnv
- `internal/config/yaml_config.go` — ValidateConfigFile, schema.version warning
- `internal/embeddings/embeddings.go` — Embedder interface, Config, New()
- `internal/embeddings/openai.go` — OpenAI provider
- `internal/embeddings/ollama.go` — Ollama provider
- `internal/cli/config_cmd.go` — config validate embedding probes
- `internal/cli/init.go` — countMigrations, Ollama detection
- `migrations/V0001__schema_meta.cypher` — self-recording pattern
- `migrations/V0015__secondary_labels.cypher` — CALL IN TRANSACTIONS (no self-recording)
- `migrations/V0017__dynamic_edge_indexes.cypher` — no self-recording
- `docker-compose.yml` — Neo4j container config
- `.github/workflows/ci.yml` — migration application, cypher-shell download
- `docs/specs/phase92-gap-analysis.md` — Gap 4, 6, 7 requirements
- `docs/features/unified-cli.md` — current CLI docs
- `AGENT_HANDOFF.md` — phase artifact index
- `CHANGELOG.md` — changelog format
- `README.md` — quickstart section
