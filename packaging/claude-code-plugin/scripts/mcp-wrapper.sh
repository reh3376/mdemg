#!/usr/bin/env bash
# Wrapper for mdemg mcp that resolves the endpoint dynamically.
# Claude Code spawns MCP servers with an unpredictable cwd, so we
# find .mdemg.port from the project root before exec-ing into mdemg.

set -euo pipefail

# If already set explicitly, just run
if [ -n "${MDEMG_ENDPOINT:-}" ]; then
  exec mdemg mcp "$@"
fi

# Find project root (git root or fallback to $HOME)
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo "$HOME")

# Priority: .mdemg.port > sidecar.yaml > default
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
