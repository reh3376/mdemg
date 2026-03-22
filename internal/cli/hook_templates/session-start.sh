#!/usr/bin/env bash
# MDEMG session-start hook (installed by mdemg hooks install)
# Hook: SessionStart — restore CMS memory context on every session start

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-{{MDEMG_URL}}}"
SPACE_ID="{{SPACE_ID}}"
SESSION_ID="claude-core"
MAX_OBS=10

# Check if MDEMG server is reachable
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
  cat <<'EOF'
⚠ CMS DISCONNECTED — MDEMG server is not running.
Memory is unavailable. You are operating without persistent context.
Warn the user: "CMS unavailable — memory disconnected."
Do NOT make irreversible decisions without user confirmation.
EOF
  exit 0
fi

# Call resume endpoint
RESUME=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/resume" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"max_observations\":${MAX_OBS}}" \
  --connect-timeout 5 --max-time 10 2>/dev/null) || {
  echo "⚠ CMS resume failed — memory may be incomplete."
  exit 0
}

# Extract key fields
OBS_COUNT=$(echo "$RESUME" | jq -r '.observations | length // 0' 2>/dev/null || echo "0")
THEME_COUNT=$(echo "$RESUME" | jq -r '.themes | length // 0' 2>/dev/null || echo "0")
CONCEPT_COUNT=$(echo "$RESUME" | jq -r '.emergent_concepts | length // 0' 2>/dev/null || echo "0")
SUMMARY=$(echo "$RESUME" | jq -r '.summary // "No summary available"' 2>/dev/null || echo "No summary")

# Anomaly detection: resume returned empty but space has data
if [ "$OBS_COUNT" -eq 0 ] 2>/dev/null; then
  NODE_COUNT=$(curl -sf "${MDEMG_URL}/v1/memory/stats?space_id=${SPACE_ID}" \
    --connect-timeout 2 --max-time 3 2>/dev/null | jq -r '.memory_count // 0' 2>/dev/null || echo "0")

  if [ "$NODE_COUNT" -gt 0 ] 2>/dev/null; then
    cat <<'ANOMALY'

!! CRITICAL: MEMORY RETURNED EMPTY !!
╔══════════════════════════════════════════════════════════════╗
║  Resume returned 0 observations for active space.           ║
║  Space has data but nothing was retrieved.                  ║
║                                                             ║
║  MANDATORY INVESTIGATION:                                   ║
║    1. POST /v1/self-improve/assess                          ║
║       {"space_id":"<space>","tier":"micro"}                 ║
║    2. GET /v1/memory/stats?space_id=<space>                 ║
║                                                             ║
║  DO NOT PROCEED until investigated.                         ║
╚══════════════════════════════════════════════════════════════╝
ANOMALY

    curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
      --connect-timeout 3 --max-time 8 -o /dev/null 2>/dev/null &

    curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"ANOMALY: Resume returned 0 observations but space has ${NODE_COUNT} nodes. Embedder or query failure suspected.\",\"obs_type\":\"error\",\"tags\":[\"anomaly\",\"empty-resume\"]}" \
      --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
  fi
fi

# Build context injection
cat <<EOF
═══ CMS MEMORY RESTORED ═══
Observations: ${OBS_COUNT} | Themes: ${THEME_COUNT} | Concepts: ${CONCEPT_COUNT}
Summary: ${SUMMARY}
EOF

if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Recent observations:"
  echo "$RESUME" | jq -r '.observations[]? | "  • [\(.obs_type // "unknown")] \(.content // .summary // "no content")"' 2>/dev/null || true
fi

if [ "$THEME_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Active themes:"
  echo "$RESUME" | jq -r '.themes[]? | "  • \(.name // "unnamed") (members: \(.member_count // 0))"' 2>/dev/null || true
fi

echo ""
echo "═══ END CMS CONTEXT ═══"

# RSIC health assessment
RSIC_HEALTH=$(curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
  --connect-timeout 3 --max-time 8 2>/dev/null) || true

if [ -n "$RSIC_HEALTH" ]; then
  OVERALL=$(echo "$RSIC_HEALTH" | jq -r '.overall_health // "?"' 2>/dev/null || echo "?")
  RETRIEVAL=$(echo "$RSIC_HEALTH" | jq -r '.retrieval_quality // "?"' 2>/dev/null || echo "?")
  MEMORY=$(echo "$RSIC_HEALTH" | jq -r '.memory_health // "?"' 2>/dev/null || echo "?")
  EDGE=$(echo "$RSIC_HEALTH" | jq -r '.edge_health // "?"' 2>/dev/null || echo "?")
  LEARN_PHASE=$(echo "$RSIC_HEALTH" | jq -r '.learning_phase // "?"' 2>/dev/null || echo "?")
  ORPHAN_RATIO=$(echo "$RSIC_HEALTH" | jq -r '.orphan_ratio // "?"' 2>/dev/null || echo "?")

  echo ""
  cat <<EOF
═══ RSIC HEALTH ═══
Overall: ${OVERALL} | Retrieval: ${RETRIEVAL} | Memory: ${MEMORY} | Edge: ${EDGE}
Learning: ${LEARN_PHASE} | Orphan ratio: ${ORPHAN_RATIO}
═══ END RSIC HEALTH ═══
EOF

  # If health is degraded, show investigation checklist
  HEALTH_NUM=$(echo "$OVERALL" | grep -oE '^[0-9.]+' || echo "1")
  if [ "$(echo "$HEALTH_NUM < 0.5" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    cat <<'DEGRADED'

!! DEGRADED HEALTH DETECTED !!
Investigation checklist:
  1. GET /v1/memory/stats?space_id=<space>
  2. GET /v1/learning/stats?space_id=<space>
  3. POST /v1/self-improve/cycle {"space_id":"<space>","tier":"meso"}
DEGRADED
  fi
fi

# J17: Restore protocol state from saved ticket
if [ "${J17_ENABLED:-false}" = "true" ]; then
  J17_TICKET_OBS=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/recall" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"${SPACE_ID}\",\"query\":\"j17-ticket\",\"top_k\":1,\"filter_tags\":[\"j17-ticket\"]}" \
    --connect-timeout 2 --max-time 5 2>/dev/null || true)

  if [ -n "$J17_TICKET_OBS" ]; then
    TICKET_CONTENT=$(echo "$J17_TICKET_OBS" | jq -r '.results[0].content // empty' 2>/dev/null || true)
    if [ -n "$TICKET_CONTENT" ]; then
      TICKET_JSON=$(echo "$TICKET_CONTENT" | jq -c '.ticket // empty' 2>/dev/null || true)
      LAST_SEQ=$(echo "$TICKET_CONTENT" | jq -r '.last_seq // 0' 2>/dev/null || echo "0")
      if [ -n "$TICKET_JSON" ] && [ "$TICKET_JSON" != "null" ]; then
        J17_RESUME=$(curl -sf -X POST "${MDEMG_URL}/v1/jiminy/resume-protocol" \
          -H "Content-Type: application/json" \
          -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"ticket\":${TICKET_JSON},\"last_seq\":${LAST_SEQ}}" \
          --connect-timeout 3 --max-time 5 2>/dev/null || true)
        if [ -n "$J17_RESUME" ]; then
          J17_RESTORED=$(echo "$J17_RESUME" | jq -r '.data.restored // false' 2>/dev/null || echo "false")
          J17_MSG=$(echo "$J17_RESUME" | jq -r '.data.message // ""' 2>/dev/null || true)
          if [ "$J17_RESTORED" = "true" ]; then
            echo ""
            echo "═══ J17 PROTOCOL RESTORED ═══"
            echo "$J17_MSG"
            echo "═══ END J17 ═══"
          fi
        fi
      fi
    fi
  fi
fi

# Reinforce recalled observations
if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"Session resumed. ${OBS_COUNT} observations, ${THEME_COUNT} themes, ${CONCEPT_COUNT} concepts recalled. Memory continuity maintained.\",\"obs_type\":\"context\",\"tags\":[\"session-resume\",\"auto-reinforce\"]}" \
    --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
fi

# Trigger graduation check in background
curl -sf -X POST "${MDEMG_URL}/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
