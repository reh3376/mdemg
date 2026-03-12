#!/usr/bin/env bash
# Hook: SessionStart — restore CMS memory context on every session start
# Part of: mdemg Claude Code plugin
# Runs automatically before Claude sees any user input.

set -euo pipefail

# ── Dependency check ─────────────────────────────────────────────
if ! command -v jq &>/dev/null; then
  echo "MDEMG plugin requires jq but it is not installed."
  echo "Install via: brew install jq (macOS) or apt install jq (Linux)"
  exit 0
fi

# ── Project root resolution ──────────────────────────────────────
# Used by both endpoint and space ID resolution.
# git root is authoritative; fallback to cwd for non-git dirs.
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")

# ── Dynamic endpoint resolution ──────────────────────────────────
# Priority: MDEMG_ENDPOINT env > .mdemg.port file > sidecar.yaml > default
resolve_endpoint() {
  if [ -n "${MDEMG_ENDPOINT:-}" ]; then
    echo "$MDEMG_ENDPOINT"
    return
  fi

  if [ -f "$PROJECT_ROOT/.mdemg.port" ]; then
    local port
    port=$(cat "$PROJECT_ROOT/.mdemg.port" 2>/dev/null | tr -d '[:space:]')
    if [ -n "$port" ]; then
      echo "http://localhost:$port"
      return
    fi
  fi

  if [ -f "$PROJECT_ROOT/.mdemg/sidecar.yaml" ]; then
    local ep
    ep=$(grep -E '^\s+endpoint:' "$PROJECT_ROOT/.mdemg/sidecar.yaml" 2>/dev/null | head -1 | sed 's/.*endpoint:\s*//' | tr -d '[:space:]"'"'" )
    if [ -n "$ep" ]; then
      echo "$ep"
      return
    fi
  fi

  echo "http://localhost:9999"
}

# ── Space ID resolution ──────────────────────────────────────────
# Matches sidecar's "repo-basename" strategy: lowercase basename of git root.
# Sanitized to [a-z0-9-] to be safe in JSON and URL contexts.
resolve_space_id() {
  basename "$PROJECT_ROOT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g'
}

MDEMG_URL=$(resolve_endpoint)
SPACE_ID=$(resolve_space_id)
SESSION_ID="claude-core"
MAX_OBS=10

# ── Health check ─────────────────────────────────────────────────
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
  cat <<'EOF'
MDEMG DISCONNECTED — server is not running.
Memory is unavailable. You are operating without persistent context.
Warn the user: "MDEMG unavailable — memory disconnected."
Do NOT make irreversible decisions without user confirmation.
EOF
  exit 0
fi

# ── Resume conversation context ──────────────────────────────────
RESUME=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/resume" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"max_observations\":${MAX_OBS}}" \
  --connect-timeout 5 --max-time 10 2>/dev/null) || {
  echo "MDEMG resume failed — memory may be incomplete."
  exit 0
}

# ── Extract fields ───────────────────────────────────────────────
OBS_COUNT=$(echo "$RESUME" | jq -r '.observations | length // 0' 2>/dev/null || echo "0")
THEME_COUNT=$(echo "$RESUME" | jq -r '.themes | length // 0' 2>/dev/null || echo "0")
CONCEPT_COUNT=$(echo "$RESUME" | jq -r '.emergent_concepts | length // 0' 2>/dev/null || echo "0")
SUMMARY=$(echo "$RESUME" | jq -r '.summary // "No summary available"' 2>/dev/null || echo "No summary")

# ── Anomaly detection ────────────────────────────────────────────
if [ "$OBS_COUNT" -eq 0 ] 2>/dev/null; then
  NODE_COUNT=$(curl -sf "${MDEMG_URL}/v1/memory/stats?space_id=${SPACE_ID}" \
    --connect-timeout 2 --max-time 3 2>/dev/null | jq -r '.memory_count // 0' 2>/dev/null || echo "0")

  if [ "$NODE_COUNT" -gt 0 ] 2>/dev/null; then
    echo "ANOMALY: Resume returned 0 observations but space has ${NODE_COUNT} nodes. Embedder or query failure suspected."

    # Trigger self-improvement in background
    curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
      --connect-timeout 3 --max-time 8 -o /dev/null 2>/dev/null &

    curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"ANOMALY: Resume returned 0 observations but space has ${NODE_COUNT} nodes.\",\"obs_type\":\"error\",\"tags\":[\"anomaly\",\"empty-resume\"]}" \
      --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
  fi
fi

# ── Output context ───────────────────────────────────────────────
cat <<EOF
CMS MEMORY RESTORED
Observations: ${OBS_COUNT} | Themes: ${THEME_COUNT} | Concepts: ${CONCEPT_COUNT}
Summary: ${SUMMARY}
EOF

if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Recent observations:"
  echo "$RESUME" | jq -r '.observations[]? | "  [\(.obs_type // "unknown")] \(.content // .summary // "no content")"' 2>/dev/null || true
fi

if [ "$THEME_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Active themes:"
  echo "$RESUME" | jq -r '.themes[]? | "  \(.name // "unnamed") (members: \(.member_count // 0))"' 2>/dev/null || true
fi

# ── RSIC health ──────────────────────────────────────────────────
RSIC_HEALTH=$(curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
  --connect-timeout 3 --max-time 8 2>/dev/null) || true

if [ -n "$RSIC_HEALTH" ]; then
  OVERALL=$(echo "$RSIC_HEALTH" | jq -r '.overall_health // "?"' 2>/dev/null || echo "?")
  RETRIEVAL=$(echo "$RSIC_HEALTH" | jq -r '.retrieval_quality // "?"' 2>/dev/null || echo "?")
  MEMORY=$(echo "$RSIC_HEALTH" | jq -r '.memory_health // "?"' 2>/dev/null || echo "?")
  LEARN_PHASE=$(echo "$RSIC_HEALTH" | jq -r '.learning_phase // "?"' 2>/dev/null || echo "?")

  echo ""
  echo "RSIC Health: ${OVERALL} | Retrieval: ${RETRIEVAL} | Memory: ${MEMORY} | Learning: ${LEARN_PHASE}"

  HEALTH_NUM=$(echo "$OVERALL" | grep -oE '^[0-9.]+' || echo "1")
  if awk "BEGIN {exit !($HEALTH_NUM < 0.5)}" 2>/dev/null; then
    echo "DEGRADED HEALTH — run: POST /v1/self-improve/cycle {\"space_id\":\"${SPACE_ID}\",\"tier\":\"meso\"}"
  fi
fi

# Trigger graduation check in background
curl -sf -X POST "${MDEMG_URL}/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
