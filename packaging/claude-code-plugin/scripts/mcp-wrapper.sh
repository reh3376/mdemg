#!/usr/bin/env bash
# Wrapper for mdemg mcp that resolves the endpoint dynamically.
# Claude Code spawns MCP servers with an unpredictable cwd, so we
# walk upward from the script's own location to find the project root.

set -euo pipefail

# If already set explicitly, just run
if [ -n "${MDEMG_ENDPOINT:-}" ]; then
  exec mdemg mcp "$@"
fi

# Resolve the script's real directory (handles symlinks)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd -P)

# Walk upward from the script location to find project root indicators:
# .mdemg.port, .mdemg/, or .git/
find_project_root() {
  local dir="$SCRIPT_DIR"
  while [ "$dir" != "/" ]; do
    if [ -f "$dir/.mdemg.port" ] || [ -d "$dir/.mdemg" ] || [ -d "$dir/.git" ]; then
      echo "$dir"
      return
    fi
    dir=$(dirname "$dir")
  done
  # Last resort: try git from cwd (may work if Claude Code sets cwd correctly)
  git rev-parse --show-toplevel 2>/dev/null || echo ""
}

PROJECT_ROOT=$(find_project_root)

if [ -z "$PROJECT_ROOT" ]; then
  # No project root found — let mdemg mcp use its own defaults
  exec mdemg mcp "$@"
fi

# Priority: .mdemg.port > sidecar.yaml > default (let mdemg handle it)
if [ -f "$PROJECT_ROOT/.mdemg.port" ]; then
  PORT=$(cat "$PROJECT_ROOT/.mdemg.port" 2>/dev/null | tr -d '[:space:]')
  if [ -n "$PORT" ]; then
    export MDEMG_ENDPOINT="http://localhost:$PORT"
  fi
elif [ -f "$PROJECT_ROOT/.mdemg/sidecar.yaml" ]; then
  EP=$(grep -E '^\s+endpoint:' "$PROJECT_ROOT/.mdemg/sidecar.yaml" 2>/dev/null | head -1 | sed 's/.*endpoint:\s*//' | tr -d '[:space:]"'"'")
  if [ -n "$EP" ]; then
    export MDEMG_ENDPOINT="$EP"
  fi
fi

exec mdemg mcp "$@"
