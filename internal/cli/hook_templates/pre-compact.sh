#!/usr/bin/env bash
# MDEMG pre-compact hook (installed by mdemg hooks install)
# Hook: PreCompact — save current work state to CMS before context compaction.
# This ensures critical context survives the compaction boundary.

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-{{MDEMG_URL}}}"
SPACE_ID="{{SPACE_ID}}"
SESSION_ID="claude-core"

# Check server is up (fast fail)
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 1; then
  exit 0
fi

# Read hook input from stdin
INPUT=$(cat)

# Extract transcript path and session info
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty' 2>/dev/null || true)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty' 2>/dev/null || true)

# Build a richer context message by examining recent state
CONTEXT_PARTS="Pre-compaction state save."

# Include working directory context
if [ -n "$CWD" ]; then
  CONTEXT_PARTS="${CONTEXT_PARTS} Working directory: ${CWD}."
fi

# If we can read the transcript tail, extract recent tool activity
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ]; then
  RECENT=$(tail -5 "$TRANSCRIPT_PATH" 2>/dev/null | jq -r '.content // empty' 2>/dev/null | head -3 || true)
  if [ -n "$RECENT" ]; then
    CONTEXT_PARTS="${CONTEXT_PARTS} Recent activity: $(echo "$RECENT" | tr '\n' ' ' | head -c 300)"
  fi
fi

# Fetch current volatile stats to include in snapshot
VSTATS=$(curl -sf "${MDEMG_URL}/v1/conversation/volatile/stats?space_id=${SPACE_ID}" --connect-timeout 1 --max-time 2 2>/dev/null || true)
if [ -n "$VSTATS" ]; then
  VOL_COUNT=$(echo "$VSTATS" | jq -r '.volatile_count // 0' 2>/dev/null || echo "0")
  PERM_COUNT=$(echo "$VSTATS" | jq -r '.permanent_count // 0' 2>/dev/null || echo "0")
  CONTEXT_PARTS="${CONTEXT_PARTS} Memory state: ${VOL_COUNT} volatile, ${PERM_COUNT} permanent observations."
fi

# Health snapshot before compaction
SESSION_HEALTH=$(curl -sf "${MDEMG_URL}/v1/conversation/session/health?session_id=${SESSION_ID}" \
  --connect-timeout 1 --max-time 2 2>/dev/null || true)
if [ -n "$SESSION_HEALTH" ]; then
  S_SCORE=$(echo "$SESSION_HEALTH" | jq -r '.health_score // 0' 2>/dev/null || echo "0")
  CONTEXT_PARTS="${CONTEXT_PARTS} Session health: ${S_SCORE}."
  if [ "$(echo "$S_SCORE < 0.3" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Session health critically low (${S_SCORE}). CMS may not be properly integrated."
  fi
fi

# Save the observation
curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d "{
    \"space_id\": \"${SPACE_ID}\",
    \"session_id\": \"${SESSION_ID}\",
    \"content\": $(echo "$CONTEXT_PARTS" | jq -Rs .),
    \"obs_type\": \"context\",
    \"tags\": [\"pre-compaction\", \"auto-save\"]
  }" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null || true

# Also run consolidation in background — compaction is a natural breakpoint
curl -sf -X POST "${MDEMG_URL}/v1/conversation/consolidate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 10 -o /dev/null 2>/dev/null &

# Trigger graduation check too
curl -sf -X POST "${MDEMG_URL}/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &

exit 0
