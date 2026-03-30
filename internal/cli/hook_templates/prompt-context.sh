#!/usr/bin/env bash
# Hook: UserPromptSubmit — recall relevant CMS context for each user prompt
# Reads the user's prompt from stdin JSON and queries CMS for relevant memory.

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-{{MDEMG_URL}}}"

get_space_id() {
    if [ -n "${MDEMG_SPACE_ID:-}" ]; then
        echo "$MDEMG_SPACE_ID"
    elif [ -f ".mdemg/config.yaml" ]; then
        grep -oP 'space_id:\s*\K\S+' .mdemg/config.yaml 2>/dev/null || echo "{{SPACE_ID}}"
    else
        echo "{{SPACE_ID}}"
    fi
}
SPACE_ID=$(get_space_id)

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
  echo "⚠ CMS unavailable — no memory context for this prompt"
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
  # Phase 80: Empty recall warning for non-trivial queries
  if [ ${#USER_PROMPT} -gt 15 ]; then
    echo "!! CMS RECALL EMPTY — No relevant memory found for this query."
    echo "!! Consider: POST /v1/conversation/observe to record this topic."
  fi
  # Session health ribbon (1s timeout)
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

# Handle both array response and object-with-results response
echo "$RECALL" | jq -r '
  (if type == "array" then . elif .results then .results else [] end)[]? |
  "  • [\(.type // .obs_type // "memory")] (score: \(.score // "?" | tostring | .[0:4])) \(.content // .summary // "no content" | .[0:200])"
' 2>/dev/null || true

echo "═══ END CMS RECALL ═══"

# --- Jiminy inner voice guidance (event-driven warm/latest pattern) ---
# Reads pre-computed guidance instantly (<100ms) from warm store.
# Then triggers async warm-up for NEXT prompt with current context.
# NOTE: Guidance responses may contain control chars (U+0000-U+001F) inside JSON
# string values. Shell variable expansion + echo corrupts these bytes, so we write
# to a temp file and parse with jq directly from the file.
GUIDANCE_TMP=$(mktemp /tmp/jiminy-guidance-XXXXXX.json 2>/dev/null || echo "/tmp/jiminy-guidance-$$.json")
curl -sf "${MDEMG_URL}/v1/jiminy/latest?space_id=${SPACE_ID}" \
  --connect-timeout 1 --max-time 2 2>/dev/null | \
  perl -pe 's/[\x00-\x08\x0b\x0c\x0e-\x1f]//g' > "$GUIDANCE_TMP" 2>/dev/null || true

if [ -s "$GUIDANCE_TMP" ]; then
  WARM=$(jq -r '.warm // false' "$GUIDANCE_TMP" 2>/dev/null)
  AUGMENTATION=$(jq -r '.data.prompt_augmentation // empty' "$GUIDANCE_TMP" 2>/dev/null)
  AGE_MS=$(jq -r '.age_ms // "?"' "$GUIDANCE_TMP" 2>/dev/null)

  # J17: Capture guidance_id for feedback loop closure
  GUIDANCE_ID=$(jq -r '.data.guidance_id // empty' "$GUIDANCE_TMP" 2>/dev/null)
  if [ -n "$GUIDANCE_ID" ]; then
    mkdir -p ~/.mdemg 2>/dev/null || true
    printf '{"guidance_id":"%s","space_id":"%s","session_id":"claude-core","ts":%d}\n' \
      "$GUIDANCE_ID" "$SPACE_ID" "$(date +%s)" > ~/.mdemg/.jiminy-guidance-state 2>/dev/null || true
  else
    rm -f ~/.mdemg/.jiminy-guidance-state 2>/dev/null || true
  fi

  if [ -n "$AUGMENTATION" ] && [ "$WARM" = "true" ]; then
    echo ""
    printf '%s\n' "$AUGMENTATION"
    if [ "$AGE_MS" != "?" ] && [ "$AGE_MS" -gt 60000 ] 2>/dev/null; then
      echo "[guidance age: ${AGE_MS}ms — may be stale]"
    fi
  fi
fi
GUIDANCE_BYTES=$(wc -c < "$GUIDANCE_TMP" 2>/dev/null | tr -d ' ' || echo "0")
rm -f "$GUIDANCE_TMP" 2>/dev/null || true

# Fire-and-forget: warm guidance for NEXT prompt with current context
curl -sf -X POST "${MDEMG_URL}/v1/jiminy/warm" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"context_hint\":$(echo "$USER_PROMPT" | head -c 500 | jq -Rs .),\"session_id\":\"claude-core\"}" \
  --connect-timeout 1 --max-time 2 -o /dev/null 2>/dev/null &

# --- Reinforce recalled observations via retrieval co-activation ---
# The retrieve endpoint triggers spreading activation which creates learning
# edges between co-retrieved nodes, strengthening frequently-accessed memories.
# Fire-and-forget in background — don't slow down prompt delivery.
curl -sf -X POST "${MDEMG_URL}/v1/memory/retrieve" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"query_text\":$(echo "$USER_PROMPT" | jq -Rs .),\"candidate_k\":10,\"top_k\":5,\"hop_depth\":2}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &

# Phase 80: Session health ribbon
HEALTH_RESP=$(curl -sf "${MDEMG_URL}/v1/conversation/session/health?session_id=claude-core" \
  --connect-timeout 1 --max-time 1 2>/dev/null) || true
if [ -n "$HEALTH_RESP" ]; then
  H_SCORE=$(echo "$HEALTH_RESP" | jq -r '.health_score // "?"' 2>/dev/null || echo "?")
  H_OBS=$(echo "$HEALTH_RESP" | jq -r '.observations_since_resume // "?"' 2>/dev/null || echo "?")
  echo "[Session health: ${H_SCORE} | obs: ${H_OBS}]"
fi

# Synergy: token count footer for recall + guidance
RECALL_TOKENS=$(echo "${RECALL:-}" | wc -c | tr -d ' ')
echo "[synergy-meta: recall_tokens=${RECALL_TOKENS}, guidance_tokens=${GUIDANCE_BYTES:-0}]"
