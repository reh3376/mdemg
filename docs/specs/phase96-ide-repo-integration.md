# Phase 96: IDE + Repo Integration

**Status**: Complete
**Branch**: `mdemg-dev01`
**Depends on**: Phase 94 (Config + Project Init), Phase 95 (Database + Embedding)

---

## Overview

Phase 96 closes Gap 8 (IDE Integration) and Gap 12 (Repo Integration) from `docs/specs/phase92-gap-analysis.md`. It adds standalone git hook management (`mdemg hooks install/uninstall/list`), Claude Code MCP config generation, and an `--mcp` flag on `mdemg serve` for co-located MCP subprocess management.

**Key finding**: Most Phase 96 functionality already existed from Phases 93-94:

- MCP server (`internal/cli/mcp.go` — 1434 lines, 20 tools, stdio mode)
- `mdemg init` writes `.cursor/mcp.json` and `.vscode/mcp.json` (`writeIDEConfigs()`)
- `.mdemgignore` fully implemented (`ParseIgnoreFile`, `FindIgnoreFile`, `MatchesIgnorePatterns`)
- `mdemg watch` command (`internal/cli/watch.go` — 319 lines, fsnotify)
- Git hook installation in `mdemg init` (`installGitHook()`)
- Git hook script (`scripts/mdemg-git-hook` — 98 lines)

**What was actually missing** (and implemented here):

1. `mdemg hooks install/uninstall/list` commands (standalone hook management)
2. `.claude/mcp.json` generation in `writeIDEConfigs()`
3. MCP auto-start with `mdemg serve --mcp` (subprocess)

---

## Implementation

### 1. `internal/cli/hooks.go` — Hook Management Commands (~230 lines)

New file. Top-level `mdemg hooks` command with 3 subcommands.

**`mdemg hooks install`** — flags: `--type git|all` (default: git), `--force`, `--space-id`

- Calls shared `InstallGitHook(dir, spaceID, force)` function
- `--force` overwrites existing hooks (unlike init which refuses)
- Reports what was installed

**`mdemg hooks uninstall`** — flags: `--type git|all`

- Removes `.git/hooks/post-commit` only if it contains the `# MDEMG` marker
- Non-MDEMG hooks are left untouched

**`mdemg hooks list`**

- Reports post-commit hook status (installed mdemg / installed non-mdemg / not installed)
- Reports standalone hook script presence

**Refactoring**: `installGitHook()` moved from `init.go` to `hooks.go` as exported `InstallGitHook()`. `init.go` calls `InstallGitHook(cwd, opts.SpaceID, false)`.

### 2. Claude Code MCP Config (`.claude/mcp.json`)

- Added `hasClaude` field to `environmentInfo` struct
- `detectEnvironment()` checks for `.claude/` directory
- `writeIDEConfigs()` writes `.claude/mcp.json` with same format as Cursor/VS Code
- IDE detection prompt updated to include "Claude Code" when detected

### 3. `mdemg serve --mcp` Flag

- Added `--mcp` bool flag to `newServeCmd()`
- When enabled, launches `mdemg mcp` as a subprocess after server binds to port
- Subprocess receives `MDEMG_ENDPOINT` env var pointing to the HTTP server
- Subprocess stdin/stdout/stderr connected to parent (stdio mode for IDE communication)
- Graceful shutdown: sends SIGTERM to MCP subprocess, waits 5s, then SIGKILL

### 4. Pre-existing Lint Fixes

Fixed 3 pre-existing `errcheck` lint violations:

- `config_cmd.go:208`: `conn.Close()` → `_ = conn.Close()`
- `init.go:290`: `conn.Close()` → `_ = conn.Close()`
- `yaml_config.go:155`: `os.Setenv()` → checked with error return

---

## Files Created

| File | Description |
|------|-------------|
| `internal/cli/hooks.go` | `mdemg hooks install/uninstall/list` commands + shared `InstallGitHook`/`UninstallGitHook` |
| `docs/specs/phase96-ide-repo-integration.md` | This spec |
| `docs/features/ide-repo-integration.md` | Feature documentation |

## Files Modified

| File | Changes |
|------|---------|
| `internal/cli/init.go` | Removed `installGitHook()` (moved to hooks.go), added `hasClaude` detection, added `.claude/mcp.json` to `writeIDEConfigs()`, fixed `conn.Close()` errcheck |
| `internal/cli/serve.go` | Added `--mcp` flag, MCP subprocess start/stop, added `os/exec` import |
| `internal/cli/root.go` | Registered `newHooksCmd()` |
| `internal/cli/config_cmd.go` | Fixed `conn.Close()` errcheck |
| `internal/config/yaml_config.go` | Fixed `os.Setenv()` errcheck |
| `docs/features/unified-cli.md` | Added hooks subcommands and `--mcp` flag docs |
| `AGENT_HANDOFF.md` | Updated Phase 96 from Planned to Complete |
| `CHANGELOG.md` | Added Phase 96 entry |
| `README.md` | Added hooks/MCP info to quickstart |

---

## Verification

### Build & Lint

- `go build ./...` — clean
- `go vet ./...` — clean
- `golangci-lint run ./...` — 0 issues

### E2E Tests

1. `mdemg hooks list` — shows hook status correctly
2. `mdemg hooks install --space-id test` — installs post-commit hook
3. `mdemg hooks list` — shows "installed (mdemg)"
4. `mdemg hooks uninstall` — removes hook
5. `mdemg hooks list` — shows "not installed"
6. `mdemg hooks install --force` — overwrites existing hook
7. `mdemg init --defaults` with `.claude/` dir present — generates `.claude/mcp.json`
8. `mdemg serve --help` — shows `--mcp` flag

---

## Documents Accessed

- `docs/specs/phase92-gap-analysis.md` — Gap 8 (IDE Integration), Gap 12 (Repo Integration)
- `AGENT_HANDOFF.md` — Phase 96 entry
- `internal/cli/init.go` — `installGitHook()`, `writeIDEConfigs()`, `detectEnvironment()`
- `internal/cli/mcp.go` — MCP server, stdio mode, endpoint resolution
- `internal/cli/serve.go` — `runServe()`, `--auto-migrate` pattern
- `internal/cli/root.go` — command registration
- `internal/cli/config_cmd.go` — `conn.Close()` errcheck fix
- `internal/config/yaml_config.go` — `os.Setenv()` errcheck fix
- `scripts/mdemg-git-hook` — full git hook script (98 lines)
- `docs/features/unified-cli.md` — current CLI docs
- `CHANGELOG.md` — changelog format
- `README.md` — quickstart section
