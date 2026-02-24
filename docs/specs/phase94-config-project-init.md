# Phase 94: Config Simplification + Project Init

**Status**: Complete
**Depends on**: Phase 93 (Unified CLI Foundation)
**Effort**: M

## Summary

Phase 94 introduces `mdemg init` (project scaffolding wizard) and `mdemg config` (config management) to simplify the first-run experience. The core innovation is a YAML-to-env-var bridge: `.mdemg/config.yaml` exposes ~20 commonly-adjusted settings, which are converted to env vars before the existing `config.FromEnv()` is called — zero changes to the 160+ env var parsing logic.

## Design Decisions

### YAML → Env Var Bridge (Not Viper)

Rather than adding Viper, we use a simple approach: YAML file is read and converted to env vars via `os.Setenv` (only when not already set). This preserves all existing config behavior.

**Layered config resolution** (lowest → highest priority):

1. Hardcoded defaults (in `FromEnv()`)
2. `.mdemg/config.yaml` (new — sets env vars before FromEnv)
3. `.env` file (godotenv.Load)
4. Environment variables (os.Getenv)
5. CLI flags (Cobra flag overrides)

### `.mdemg/` Directory Convention

```
.mdemg/
  config.yaml         ← Project config (tracked in git, no secrets)
  .gitkeep            ← Ensures directory is tracked
.mdemgignore          ← Exclusion patterns for ingestion (tracked in git)
```

Secrets (passwords, API keys) stay in `.env` (gitignored) or env vars — never in `config.yaml`.

## Command Tree Additions

```
mdemg
├── init                   ← Project initialization wizard
├── config
│   ├── show               ← Display effective configuration
│   └── validate           ← Check configuration validity
└── (existing commands...)
```

## New Files

| File | Description | Lines |
|------|-------------|-------|
| `internal/config/yaml_config.go` | YAML config loader, finder, generator, validator, ignore file support | ~600 |
| `internal/cli/config_loader.go` | Shared `loadConfig()` helper | ~25 |
| `internal/cli/init.go` | `mdemg init` command with interactive wizard | ~300 |
| `internal/cli/config_cmd.go` | `mdemg config show/validate` | ~180 |

## Modified Files

| File | Changes |
|------|---------|
| `internal/cli/root.go` | Register `init` and `config` commands |
| `internal/cli/serve.go` | Use `loadConfig()` helper |
| `internal/cli/db.go` | Use `loadConfig()` helper |
| `internal/cli/ingest.go` | YAML config loading + `.mdemgignore` support |
| `internal/cli/consolidate.go` | YAML + godotenv loading before Neo4j config |
| `internal/cli/decay.go` | YAML + godotenv loading before `parseNeo4jEnv()` |
| `internal/cli/prune.go` | YAML + godotenv loading before inline env reads |
| `internal/cli/space.go` | YAML + godotenv loading in `newDriver()` helper |
| `.env.example` | Fix schema version 4→17 |
| `scripts/mdemg-git-hook` | Prefer `mdemg ingest` over legacy `ingest-codebase` |

## Key Features

### `mdemg init`

- Interactive wizard (skip with `--defaults`)
- Auto-detects: Neo4j (TCP probe), Ollama (HTTP), Git, IDE (.cursor/.vscode)
- Generates: `.mdemg/config.yaml`, `.mdemgignore` (seeded from `.gitignore`)
- Optionally installs git post-commit hook and IDE MCP configs
- Flags: `--defaults`, `--yes`, `--space-id`, `--neo4j-uri`, `--embedding-provider`, `--no-hooks`, `--no-ide`

### `mdemg config show`

- Displays effective config values with source annotations (yaml/env/default)
- Masks secrets (passwords, API keys)
- `--json` flag for machine-readable output

### `mdemg config validate`

- Validates YAML syntax and field values
- Tests Neo4j reachability (TCP probe)
- Tests embedding provider reachability (HTTP probe)
- Reports errors and warnings

### `.mdemgignore`

- gitignore-style pattern syntax (line-based, `#` comments, `!` negation)
- Applied during `mdemg ingest` file walk phase
- Patterns for both files and directories
- Merged with `--exclude` flag patterns

## YAML Config Structure

```yaml
neo4j:
  uri: bolt://localhost:7687
  user: neo4j
server:
  port: 9999
embedding:
  provider: ollama
  model: nomic-embed-text
  endpoint: http://localhost:11434
retrieval:
  candidate_k: 200
  top_k: 20
  hop_depth: 2
learning:
  eta: 0.1
  decay_per_day: 0.05
  max_edges_per_node: 100
plugins:
  enabled: false
  dir: .mdemg/plugins
schema:
  version: 17
```

Each key maps to an env var via the `yamlEnvMapping` table in `yaml_config.go`.

## Verification

1. `go build ./...` — compiles clean
2. `go vet ./...` — no issues
3. `mdemg init --defaults` — creates `.mdemg/config.yaml` + `.mdemgignore` + git hook
4. `mdemg config show` — displays effective config with sources
5. `mdemg config validate` — reports validation status
6. `mdemg serve` with `.mdemg/config.yaml` — reads YAML values
7. Existing tests pass: `go test ./internal/config/...`

## Documents Accessed

- `internal/config/config.go` — Config struct, FromEnv()
- `internal/cli/root.go` — Command registration
- `internal/cli/serve.go`, `db.go`, `ingest.go`, `consolidate.go`, `decay.go`, `prune.go`, `space.go` — Config loading patterns
- `.env.example` — Current env var template
- `scripts/mdemg-git-hook`, `scripts/install-git-hook` — Git hook logic
- `docs/specs/phase92-gap-analysis.md` — Gap 3 and Gap 5 requirements
- `docs/features/unified-cli.md` — CLI documentation
- `go.mod` — Dependencies
- `migrations/V*.cypher` — 17 migration files
- `AGENT_HANDOFF.md`, `CHANGELOG.md`, `README.md`
