#!/usr/bin/env bash
# MDEMG prompt-context hook (installed by mdemg hooks install)
# Hook: UserPromptSubmit — recall CMS context + Jiminy guidance for each prompt

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-{{MDEMG_URL}}}"
SPACE_ID="{{SPACE_ID}}"

# Read hook input from stdin
INPUT=$(cat)

# Extract the user's prompt text
USER_PROMPT=$(echo "$INPUT" | jq -r '.user_prompt // empty' 2>/dev/null)
if [ -z "$USER_PROMPT" ]; then
  exit 0
fi

# Skip very short prompts (commands like "y", "ok", etc.)
if [ ${#USER_PROMPT} -lt 15 ]; then
  exit 0
fi

# Check server is up (fast fail)
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 1; then
  exit 0
fi

# Recall relevant context from CMS
RECALL=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/recall" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"query\":$(echo "$USER_PROMPT" | jq -Rs .),\"top_k\":5,\"include_themes\":true,\"include_concepts\":true}" \
  --connect-timeout 3 --max-time 8 2>/dev/null) || exit 0

# Check if there are results
RESULT_COUNT=$(echo "$RECALL" | jq -r 'if type == "array" then length elif .results then (.results | length) else 0 end' 2>/dev/null || echo "0")

if [ "$RESULT_COUNT" -eq 0 ] 2>/dev/null; then
  if [ ${#USER_PROMPT} -gt 15 ]; then
    echo "!! CMS RECALL EMPTY — No relevant memory found for this query."
    echo "!! Consider: POST /v1/conversation/observe to record this topic."
  fi
  HEALTH_RESP=$(curl -sf "${MDEMG_URL}/v1/conversation/session/health?session_id=claude-core" \
    --connect-timeout 1 --max-time 1 2>/dev/null) || true
  if [ -n "$HEALTH_RESP" ]; then
    H_SCORE=$(echo "$HEALTH_RESP" | jq -r '.health_score // "?"' 2>/dev/null || echo "?")
    H_OBS=$(echo "$HEALTH_RESP" | jq -r '.observations_since_resume // "?"' 2>/dev/null || echo "?")
    echo "[Session health: ${H_SCORE} | obs: ${H_OBS}]"
  fi
  exit 0
fi

# Format relevant context
echo "═══ CMS RECALL (relevant to this prompt) ═══"

echo "$RECALL" | jq -r '
  (if type == "array" then . elif .results then .results else [] end)[]? |
  "  • [\(.type // .obs_type // "memory")] (score: \(.score // "?" | tostring | .[0:4])) \(.content // .summary // "no content" | .[0:200])"
' 2>/dev/null || true

echo "═══ END CMS RECALL ═══"

# --- Jiminy inner voice guidance ---
# Always attempts the call. Server returns 503 if disabled — curl fails silently.
# No env var guard needed: the server is the single source of truth.
GUIDANCE=$(curl -sf -X POST "${MDEMG_URL}/v1/jiminy/guide" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"context\":$(echo "$USER_PROMPT" | jq -Rs .),\"session_id\":\"claude-core\"}" \
  --connect-timeout 3 --max-time 6 2>/dev/null) || true

if [ -n "$GUIDANCE" ]; then
  AUGMENTATION=$(echo "$GUIDANCE" | jq -r '.data.prompt_augmentation // empty' 2>/dev/null)
  if [ -n "$AUGMENTATION" ]; then
    echo ""
    echo "$AUGMENTATION"
  fi
fi

# --- Reinforce recalled observations via retrieval co-activation ---
curl -sf -X POST "${MDEMG_URL}/v1/memory/retrieve" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"query_text\":$(echo "$USER_PROMPT" | jq -Rs .),\"candidate_k\":10,\"top_k\":5,\"hop_depth\":2}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &

# Session health ribbon
HEALTH_RESP=$(curl -sf "${MDEMG_URL}/v1/conversation/session/health?session_id=claude-core" \
  --connect-timeout 1 --max-time 1 2>/dev/null) || true
if [ -n "$HEALTH_RESP" ]; then
  H_SCORE=$(echo "$HEALTH_RESP" | jq -r '.health_score // "?"' 2>/dev/null || echo "?")
  H_OBS=$(echo "$HEALTH_RESP" | jq -r '.observations_since_resume // "?"' 2>/dev/null || echo "?")
  echo "[Session health: ${H_SCORE} | obs: ${H_OBS}]"
fi
