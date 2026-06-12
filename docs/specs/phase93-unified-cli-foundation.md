# Phase 93: Unified CLI Foundation

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Status:** Complete
**Depends On:** Nothing
**Blocked By:** Nothing
**Effort:** XL

---

## Overview

Merged 12 separate Go binaries under `cmd/` into a single `mdemg` binary using the Cobra CLI framework. This is the foundation for every subsequent phase (94-100) in the deployable package roadmap.

### Before

- 12 independent binaries (`server`, `mcp-server`, `ingest-codebase`, `consolidate`, `decay`, `prune`, `extract-symbols`, `watch`, `space-transfer`, `plugin-scaffold`, `plugin-validate`, `reset-db`)
- Each used raw `flag` package with no shared CLI framework
- Duplicated utility functions across binaries (Neo4j type conversions)
- `languages/` package nested inside `cmd/ingest-codebase/` blocking cross-binary imports

### After

- Single `mdemg` binary with Cobra subcommand tree
- All 12 original binaries converted to thin deprecation shims
- Shared `neo4jutil` conversions package
- `languages/` package moved to `internal/languages/`
- Build-time version injection via ldflags

---

## Command Tree

```
mdemg
├── serve                  ← cmd/server
├── mcp                    ← cmd/mcp-server
├── ingest                 ← cmd/ingest-codebase
├── consolidate            ← cmd/consolidate
├── decay                  ← cmd/decay
├── prune                  ← cmd/prune
├── extract-symbols        ← cmd/extract-symbols
├── watch                  ← cmd/watch
├── db
│   └── reset              ← cmd/reset-db
├── space
│   ├── export             ← cmd/space-transfer export
│   ├── import             ← cmd/space-transfer import
│   ├── list               ← cmd/space-transfer list
│   ├── delete             ← cmd/space-transfer delete
│   ├── rename             ← cmd/space-transfer rename
│   └── copy               ← cmd/space-transfer copy
├── plugin
│   ├── scaffold           ← cmd/plugin-scaffold
│   └── validate           ← cmd/plugin-validate
└── version                ← new (build-time version info)
```

---

## Files Created

| File | Description | Lines |
|------|-------------|-------|
| `cmd/mdemg/main.go` | Unified CLI entry point | ~10 |
| `internal/cli/root.go` | Root command, global flags, Execute(), RunLegacyShim() | ~65 |
| `internal/cli/version.go` | Version command with build-time vars | ~25 |
| `internal/cli/serve.go` | Server startup command | ~50 |
| `internal/cli/mcp.go` | MCP server command | ~80 |
| `internal/cli/ingest.go` | Codebase ingestion command (50+ flags) | ~200 |
| `internal/cli/consolidate.go` | Graph consolidation command | ~120 |
| `internal/cli/decay.go` | Temporal decay command | ~80 |
| `internal/cli/prune.go` | Edge/node pruning command | ~120 |
| `internal/cli/extract_symbols.go` | Symbol extraction command | ~70 |
| `internal/cli/watch.go` | File system watcher command | ~60 |
| `internal/cli/db.go` | DB parent + reset subcommand | ~60 |
| `internal/cli/space.go` | Space parent + 6 subcommands | ~120 |
| `internal/cli/plugin.go` | Plugin scaffold + validate commands | ~100 |
| `internal/cli/neo4jutil/conversions.go` | Shared Neo4j type conversions | ~90 |

## Files Moved

| From | To | Reason |
|------|-----|--------|
| `cmd/ingest-codebase/languages/` | `internal/languages/` | Shared by ingest + extract-symbols; can't cross-import cmd packages |

## Files Modified

| File | Changes |
|------|---------|
| `go.mod` | Cobra dependency (already indirect, now direct) |
| `internal/scraper/parser.go` | Import path: `cmd/ingest-codebase/languages` → `internal/languages` |
| `internal/api/handlers_ingest_codebase.go` | Binary path: `./bin/ingest-codebase` → `./bin/mdemg ingest` |
| `internal/api/handlers.go` | Binary path: `./bin/ingest-codebase` → `./bin/mdemg ingest` |
| `Makefile` | `build-cli` target, ldflags, `run` uses `mdemg serve` |
| `.github/workflows/ci.yml` | Builds `cmd/mdemg`, starts server with `mdemg serve` |
| All 12 `cmd/*/main.go` | Converted to deprecation shims via `cli.RunLegacyShim()` |

## Test Files Migrated

| From | To |
|------|-----|
| `cmd/decay/main_test.go` | `internal/cli/decay_test.go` |
| `cmd/watch/main_test.go` | `internal/cli/watch_test.go` |
| `cmd/consolidate/main_test.go` | `internal/cli/consolidate_test.go` |
| `cmd/prune/main_test.go` | `internal/cli/prune_test.go` |
| `cmd/plugin-scaffold/main_test.go` | `internal/cli/plugin_test.go` |

---

## Key Design Decisions

### Deprecation Shim Pattern

Old binaries are not deleted — they become thin wrappers that print a deprecation warning and delegate to the unified CLI:

```go
package main

import "mdemg/internal/cli"

func main() {
    cli.RunLegacyShim("mdemg-decay", "mdemg decay", []string{"decay"})
}
```

This preserves backward compatibility while guiding users to the new interface.

### Shared Neo4j Conversions

Functions `AsString`, `AsFloat64`, `AsInt`, `AsBool`, `AsTime`, `AsStringSlice`, `AsFloat64Slice`, `AsFloat32Slice` were duplicated across consolidate, decay, and prune. Extracted to `internal/cli/neo4jutil/conversions.go` with exported (PascalCase) names.

### Name Collision Resolution

Moving all 12 binaries into the same `cli` package created name collisions. Resolved by prefixing prune-specific types/functions: `edge` → `pruneEdge`, `deleteEdge` → `deletePruneEdge`, `queryEdgeBatch` → `queryPruneEdgeBatch`, `newDriver` → `newPruneDriver`.

### Build-Time Version Injection

Version, commit hash, and build date injected via ldflags:

```makefile
LDFLAGS := -ldflags "-X mdemg/internal/cli.Version=$(VERSION) \
  -X mdemg/internal/cli.Commit=$(COMMIT) \
  -X mdemg/internal/cli.BuildDate=$(BUILD_DATE)"
```

### API Handler Binary Paths

Two API handlers (`handlers_ingest_codebase.go`, `handlers.go`) exec'd `./bin/ingest-codebase` directly. Updated to `./bin/mdemg` with `append([]string{"ingest"}, args...)`.

---

## Build & Usage

```bash
# Build unified CLI
make build-cli

# Or directly
go build -o bin/mdemg ./cmd/mdemg

# Run server
./bin/mdemg serve

# Version info
./bin/mdemg version

# Subcommand help
./bin/mdemg ingest --help
./bin/mdemg space list --help
./bin/mdemg db reset --help

# All subcommands
./bin/mdemg --help
```

---

## Verification Checklist

- [x] `go build ./...` — all packages compile
- [x] `go vet ./...` — no static analysis issues
- [x] `mdemg --help` — shows all subcommands
- [x] `mdemg version` — prints version info
- [x] `mdemg serve` — starts server on port 9999
- [x] `mdemg space list --help` — nested subcommands work
- [x] `mdemg db reset --help` — nested subcommands work
- [x] Old binaries still build (deprecation shims)
- [x] CI pipeline updated

---

## Documents Accessed

- `cmd/server/main.go`, `cmd/mcp-server/main.go`, `cmd/ingest-codebase/main.go`
- `cmd/consolidate/main.go`, `cmd/decay/main.go`, `cmd/prune/main.go`
- `cmd/extract-symbols/main.go`, `cmd/watch/main.go`, `cmd/space-transfer/main.go`
- `cmd/plugin-scaffold/main.go`, `cmd/plugin-validate/main.go`, `cmd/reset-db/main.go`
- `internal/config/config.go`, `internal/validation/validator.go`
- `internal/scraper/parser.go`
- `internal/api/handlers.go`, `internal/api/handlers_ingest_codebase.go`
- `go.mod`, `Makefile`, `.github/workflows/ci.yml`
- `docs/specs/phase92-gap-analysis.md`
- `AGENT_HANDOFF.md`, `CHANGELOG.md`, `README.md`, `CONTRIBUTING.md`
