# IDE + Repo Integration

Phase 96 adds standalone hook management, Claude Code MCP configuration, and `--mcp` co-location for `mdemg serve`.

---

## Git Hook Management

### Install Hooks

```bash
# Install post-commit hook with default space ID (directory name)
mdemg hooks install

# Install with custom space ID
mdemg hooks install --space-id my-project

# Overwrite an existing hook
mdemg hooks install --force
```

The post-commit hook runs `mdemg ingest` incrementally in the background after each commit. It is non-blocking and silently skips if the `mdemg` binary is not found.

To temporarily disable the hook without removing it:

```bash
MDEMG_DISABLED=true git commit -m "skip ingestion"
```

### Uninstall Hooks

```bash
mdemg hooks uninstall
```

Only removes hooks installed by MDEMG (identified by the `# MDEMG` marker in the first few lines). Non-MDEMG hooks are left untouched.

### List Hook Status

```bash
mdemg hooks list
```

Example output:

```
MDEMG Hook Status
=================
  post-commit hook:  installed (mdemg)
  hook script:       present (scripts/mdemg-git-hook)
```

States for post-commit hook:

- `installed (mdemg)` — MDEMG-managed hook is active
- `installed (non-mdemg)` — a hook exists but wasn't installed by MDEMG
- `not installed` — no post-commit hook present

---

## IDE Configuration

### Automatic Setup via `mdemg init`

`mdemg init` detects your IDE and generates MCP configuration automatically:

| IDE | Detection | Config File |
|-----|-----------|-------------|
| Cursor | `.cursor/` directory | `.cursor/mcp.json` |
| VS Code | `.vscode/` directory | `.vscode/mcp.json` |
| Claude Code | `.claude/` directory | `.claude/mcp.json` |

The generated config connects your IDE's AI agent to the MDEMG MCP server:

```json
{
  "mcpServers": {
    "mdemg": {
      "command": "mdemg",
      "args": ["mcp"],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:9999"
      }
    }
  }
}
```

Config files are only written if they don't already exist — existing configs are never overwritten.

Skip IDE config generation with `--no-ide`:

```bash
mdemg init --defaults --no-ide
```

### Manual MCP Setup

If your IDE wasn't detected during `mdemg init`, create the MCP config manually:

```bash
# For Claude Code
mkdir -p .claude
echo '{"mcpServers":{"mdemg":{"command":"mdemg","args":["mcp"],"env":{"MDEMG_ENDPOINT":"http://localhost:9999"}}}}' > .claude/mcp.json
```

---

## MCP Server Co-location

### `mdemg serve --mcp`

Start the HTTP API server and MCP server together:

```bash
mdemg serve --mcp
```

This launches `mdemg mcp` as a subprocess alongside the HTTP server. The subprocess automatically receives the correct `MDEMG_ENDPOINT` environment variable pointing to the HTTP server's port.

On shutdown (SIGINT/SIGTERM), both the HTTP server and MCP subprocess are stopped gracefully.

This is useful when an IDE launches the server process directly — the MCP stdio interface is available on the same process's stdin/stdout.

### Standalone MCP

For manual or development use, start the MCP server separately:

```bash
# Terminal 1: HTTP server
mdemg serve

# Terminal 2: MCP server (auto-discovers HTTP endpoint)
mdemg mcp
```

The MCP server discovers the HTTP endpoint via priority chain: `MDEMG_ENDPOINT` env var > `.mdemg.port` file > `LISTEN_ADDR` > default (<http://localhost:9999>).

---

## Repository Integration

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

Generated automatically by `mdemg init` with sensible defaults for the detected project type.

### Standalone Hook Script

For advanced hook workflows, a full-featured hook script is available at `scripts/mdemg-git-hook`. It supports:

- Dynamic port discovery (env var > `.mdemg.port` file > default)
- Configurable skip patterns (`MDEMG_SKIP_PATTERNS`)
- Verbose mode (`MDEMG_VERBOSE=true`)
- Log file output (`MDEMG_LOG_FILE`)
- Legacy binary fallback

Install it manually:

```bash
cp scripts/mdemg-git-hook .git/hooks/post-commit
chmod +x .git/hooks/post-commit
```

Or use the CLI (recommended):

```bash
mdemg hooks install
```
