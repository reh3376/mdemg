#!/usr/bin/env bash
# Hook: PreCompact — save current work state to CMS before context compaction.
# This ensures critical context survives the compaction boundary.

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-http://localhost:9999}"
SESSION_ID="claude-core"

get_space_id() {
    if [ -n "${MDEMG_SPACE_ID:-}" ]; then
        echo "$MDEMG_SPACE_ID"
    elif [ -f ".mdemg/config.yaml" ]; then
        grep -oP 'space_id:\s*\K\S+' .mdemg/config.yaml 2>/dev/null || echo "mdemg-dev"
    else
        echo "mdemg-dev"
    fi
}
SPACE_ID=$(get_space_id)

# Check server is up (fast fail)
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 1; then
  exit 0
fi

# Read hook input from stdin
INPUT=$(cat)

# Extract transcript path and session info
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty' 2>/dev/null || true)
SESSION_ID_INPUT=$(echo "$INPUT" | jq -r '.session_id // empty' 2>/dev/null || true)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty' 2>/dev/null || true)

# Build a richer context message by examining recent state
CONTEXT_PARTS="Pre-compaction state save."

# Include working directory context
if [ -n "$CWD" ]; then
  CONTEXT_PARTS="${CONTEXT_PARTS} Working directory: ${CWD}."
fi

# If we can read the transcript tail, extract recent tool activity
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ]; then
  # Get last few lines to identify recent activity (tool calls, files edited)
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

# Phase 80: Health snapshot before compaction
SESSION_HEALTH=$(curl -sf "${MDEMG_URL}/v1/conversation/session/health?session_id=${SESSION_ID}" \
  --connect-timeout 1 --max-time 2 2>/dev/null || true)
if [ -n "$SESSION_HEALTH" ]; then
  S_SCORE=$(echo "$SESSION_HEALTH" | jq -r '.health_score // 0' 2>/dev/null || echo "0")
  CONTEXT_PARTS="${CONTEXT_PARTS} Session health: ${S_SCORE}."
  # If health is critically low, append warning
  if [ "$(echo "$S_SCORE < 0.3" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Session health critically low (${S_SCORE}). CMS may not be properly integrated."
  fi
fi

# RSIC health assessment before compaction
RSIC_ASSESS=$(curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
  --connect-timeout 2 --max-time 5 2>/dev/null || true)
if [ -n "$RSIC_ASSESS" ]; then
  RSIC_OVERALL=$(echo "$RSIC_ASSESS" | jq -r '.overall // 0' 2>/dev/null || echo "0")
  RSIC_CONFIDENCE=$(echo "$RSIC_ASSESS" | jq -r '.confidence // 0' 2>/dev/null || echo "0")
  RSIC_RETRIEVAL=$(echo "$RSIC_ASSESS" | jq -r '.retrieval // 0' 2>/dev/null || echo "0")
  RSIC_MEMORY=$(echo "$RSIC_ASSESS" | jq -r '.memory // 0' 2>/dev/null || echo "0")
  RSIC_EDGE=$(echo "$RSIC_ASSESS" | jq -r '.edge // 0' 2>/dev/null || echo "0")
  RSIC_TASK=$(echo "$RSIC_ASSESS" | jq -r '.task // 0' 2>/dev/null || echo "0")
  RSIC_GUIDANCE=$(echo "$RSIC_ASSESS" | jq -r '.guidance // 0' 2>/dev/null || echo "0")
  RSIC_PROTOCOL=$(echo "$RSIC_ASSESS" | jq -r '.protocol // 0' 2>/dev/null || echo "0")
  RSIC_SYNERGY=$(echo "$RSIC_ASSESS" | jq -r '.synergy // 0' 2>/dev/null || echo "0")
  JIMINY_HEALTHY=$(echo "$RSIC_ASSESS" | jq -r '.jiminy_healthy // false' 2>/dev/null || echo "false")

  CONTEXT_PARTS="${CONTEXT_PARTS} RSIC health: overall=${RSIC_OVERALL} confidence=${RSIC_CONFIDENCE} retrieval=${RSIC_RETRIEVAL} memory=${RSIC_MEMORY} edge=${RSIC_EDGE} task=${RSIC_TASK} guidance=${RSIC_GUIDANCE} protocol=${RSIC_PROTOCOL} synergy=${RSIC_SYNERGY} jiminy_healthy=${JIMINY_HEALTHY}."

  echo "═══ PRE-COMPACT HEALTH ═══"
  echo "Overall: ${RSIC_OVERALL} | Confidence: ${RSIC_CONFIDENCE}"
  echo "Retrieval: ${RSIC_RETRIEVAL} | Memory: ${RSIC_MEMORY} | Edge: ${RSIC_EDGE} | Task: ${RSIC_TASK}"
  echo "Guidance: ${RSIC_GUIDANCE} | Protocol: ${RSIC_PROTOCOL} | Synergy: ${RSIC_SYNERGY}"
  echo "Jiminy: ${JIMINY_HEALTHY}"
  echo "═══ END PRE-COMPACT HEALTH ═══"

  # Flag degraded subsystems
  if [ "$(echo "${RSIC_OVERALL} < 0.5" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo "WARNING: Overall health degraded (${RSIC_OVERALL} < 0.5)"
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Overall health degraded (${RSIC_OVERALL})."
  fi
  if [ "$(echo "${RSIC_GUIDANCE} < 0.4" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo "WARNING: Guidance subsystem degraded (${RSIC_GUIDANCE} < 0.4)"
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Guidance degraded (${RSIC_GUIDANCE})."
  fi
  if [ "$(echo "${RSIC_PROTOCOL} < 0.4" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo "WARNING: Protocol subsystem degraded (${RSIC_PROTOCOL} < 0.4)"
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Protocol degraded (${RSIC_PROTOCOL})."
  fi
  if [ "$(echo "${RSIC_SYNERGY} < 0.4" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo "WARNING: Synergy subsystem degraded (${RSIC_SYNERGY} < 0.4)"
    CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Synergy degraded (${RSIC_SYNERGY})."
  fi
fi

# Jiminy health check before compaction
JIMINY_STATUS=$(curl -sf "${MDEMG_URL}/v1/jiminy/healthz" --connect-timeout 1 --max-time 2 2>/dev/null || echo "{}")
JIMINY_OK=$(echo "$JIMINY_STATUS" | jq -e '.enabled == true and .status == "ok"' 2>/dev/null || echo "false")
if [ "$JIMINY_OK" != "true" ]; then
  CONTEXT_PARTS="${CONTEXT_PARTS} WARNING: Jiminy unhealthy during compaction — guidance pipeline offline."
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

# Emergency: ingest all claude .md files before compaction wipes context
# Uses --force to skip hash check — we want latest content persisted regardless
if [ -x "./bin/mdemg" ]; then
  ./bin/mdemg ingest-claude-md --force --quiet --space-id "${SPACE_ID}" 2>/dev/null || true
fi

# J17: Detect whether J17 is enabled via server healthz (env var may not be in shell)
J17_ENABLED="${J17_ENABLED:-false}"
if [ "$J17_ENABLED" != "true" ]; then
  J17_CHECK=$(curl -sf "${MDEMG_URL}/v1/jiminy/ready" --connect-timeout 2 --max-time 3 2>/dev/null || true)
  if [ -n "$J17_CHECK" ] && echo "$J17_CHECK" | jq -e '.features.j17 == true' >/dev/null 2>&1; then
    J17_ENABLED="true"
  fi
fi

# J17: Issue session ticket before compaction for state persistence
if [ "$J17_ENABLED" = "true" ]; then
  J17_TICKET=$(curl -sf -X POST "${MDEMG_URL}/v1/jiminy/checkpoint" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\"}" \
    --connect-timeout 2 --max-time 5 2>/dev/null || true)

  if [ -n "$J17_TICKET" ]; then
    # Store ticket as CMS observation tagged j17-ticket for session-start to retrieve
    TICKET_JSON=$(echo "$J17_TICKET" | jq -c '.data.ticket // empty' 2>/dev/null || true)
    LAST_SEQ=$(echo "$J17_TICKET" | jq -r '.data.last_seq // 0' 2>/dev/null || echo "0")
    if [ -n "$TICKET_JSON" ] && [ "$TICKET_JSON" != "null" ]; then
      TICKET_CONTENT=$(jq -n --argjson ticket "$TICKET_JSON" --arg last_seq "$LAST_SEQ" \
        '{ticket: $ticket, last_seq: ($last_seq | tonumber)}' 2>/dev/null || true)
      if [ -n "$TICKET_CONTENT" ]; then
        curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
          -H "Content-Type: application/json" \
          -d "{
            \"space_id\": \"${SPACE_ID}\",
            \"session_id\": \"${SESSION_ID}\",
            \"content\": $(echo "$TICKET_CONTENT" | jq -Rs .),
            \"obs_type\": \"context\",
            \"tags\": [\"j17-ticket\", \"auto-save\"]
          }" \
          --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null || true
      fi
    fi
  fi
fi

# Also run consolidation in background — compaction is a natural breakpoint
# for clustering new observations into themes
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
