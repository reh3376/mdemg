#!/usr/bin/env bash
# MDEMG hook — managed by mdemg hooks install
# Hook: SessionStart — restore CMS memory context on every session start
# This runs automatically before Claude sees any user input.

set -euo pipefail

# Ensure log directory exists for all ingest/hook logging
mkdir -p "$HOME/.mdemg/logs"

# MDEMG server URL: env > .mdemg.port > .env MDEMG_PORT > 9999
if [ -z "${MDEMG_URL:-}" ]; then
    if [ -f ".mdemg.port" ]; then
        _PORT=$(tr -d '[:space:]' < .mdemg.port)
    elif [ -f ".env" ]; then
        _PORT=$(sed -n 's/^MDEMG_PORT=//p' .env 2>/dev/null | tr -d '[:space:]')
    fi
    MDEMG_URL="http://localhost:${_PORT:-9999}"
fi
# Resolve SessionID per conversation: MDEMG_SESSION_ID env (stable-identity
# escape hatch) > Claude Code stdin session_id (per-conversation) >
# ~/.mdemg/.claude-session > claude-core. Realizes J17 per-(session,constraint)
# isolation instead of one shared "claude-core" across all conversations.
HOOK_INPUT=$(cat 2>/dev/null || true)
SESSION_ID="${MDEMG_SESSION_ID:-}"
if [ -z "$SESSION_ID" ]; then
    SESSION_ID=$(printf '%s' "$HOOK_INPUT" | jq -r '.session_id // empty' 2>/dev/null || true)
fi
if [ -z "$SESSION_ID" ] && [ -f "$HOME/.mdemg/.claude-session" ]; then
    SESSION_ID=$(jq -r '.session_id // empty' "$HOME/.mdemg/.claude-session" 2>/dev/null || true)
fi
[ -z "$SESSION_ID" ] && SESSION_ID="claude-core"
# Publish the resolved session id so the agent (skill) + any stdin-less context
# reads the same per-conversation id the hooks use.
printf '{"session_id":"%s","ts":%d}\n' "$SESSION_ID" "$(date +%s)" > "$HOME/.mdemg/.claude-session" 2>/dev/null || true
MAX_OBS=10

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

# Check if MDEMG server is reachable — attempt auto-start if down
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
  # Attempt auto-start (must stay under 15s hook timeout — cap at 10s)
  if [ -x "./bin/mdemg" ]; then
    echo "⚠ MDEMG server not running — attempting auto-start..."
    ./bin/mdemg start --auto-migrate 2>&1 | tail -1 || true
    # Wait for ready (up to 10s: 5 polls × 2s)
    for i in $(seq 1 5); do
      sleep 2
      if curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 1 2>/dev/null; then
        echo "✓ MDEMG server started successfully"
        break
      fi
    done
  fi
  # Re-check after auto-start attempt
  if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
    cat <<'EOF'
⚠ CMS DISCONNECTED — MDEMG server failed to start.
Memory is unavailable. You are operating without persistent context.
Warn the user: "CMS unavailable — auto-start failed."
Do NOT make irreversible decisions without user confirmation.
EOF
    exit 0
  fi
fi

# --- Alert delivery: show critical/high service alerts at session start ---
_ALERT_FILE="${ALERT_FILE_PATH:-${HOME}/.mdemg/alerts/current.json}"
if [ -f "$_ALERT_FILE" ]; then
  _CRIT_COUNT=$(jq '.alerts | map(select(.cleared == false and (.severity == "critical" or .severity == "high"))) | length' "$_ALERT_FILE" 2>/dev/null || echo 0)
  if [ "$_CRIT_COUNT" -gt 0 ] 2>/dev/null; then
    echo ""
    echo "!! MDEMG HIGH/CRITICAL ALERTS [$_CRIT_COUNT] — INVESTIGATE BEFORE PROCEEDING !!"
    jq -r '.alerts[] | select(.cleared == false and (.severity == "critical" or .severity == "high")) |
      "  [\(.severity | ascii_upcase)] \(.service): \(.title)\n    \(.message)"
    ' "$_ALERT_FILE" 2>/dev/null
    echo ""
  fi
fi

# Enhanced healthz: check for degraded subsystems
HEALTHZ_RESP=$(curl -sf "${MDEMG_URL}/healthz" --connect-timeout 2 --max-time 3 2>/dev/null || echo "{}")
HEALTHZ_STATUS=$(echo "$HEALTHZ_RESP" | jq -r '.status // "ok"' 2>/dev/null || echo "ok")
if [ "$HEALTHZ_STATUS" = "degraded" ]; then
  echo "⚠ MDEMG server degraded — some subsystems unhealthy"
  echo "$HEALTHZ_RESP" | jq -r '.checks // {} | to_entries[] | select(.value != "ok") | "  [\(.key)] \(.value)"' 2>/dev/null
fi

# Version mismatch detection (CLI binary vs running server)
if [ -x "./bin/mdemg" ]; then
  LOCAL_COMMIT=$(./bin/mdemg version 2>/dev/null | awk '/commit:/{print $2}')
  SERVER_COMMIT=$(echo "$HEALTHZ_RESP" | jq -r '.commit // empty' 2>/dev/null)
  if [ -n "$LOCAL_COMMIT" ] && [ -n "$SERVER_COMMIT" ] && [ "$LOCAL_COMMIT" != "unknown" ] && [ "$SERVER_COMMIT" != "unknown" ] && [ "$LOCAL_COMMIT" != "$SERVER_COMMIT" ]; then
    echo "⚠ Version mismatch: ./bin/mdemg=$LOCAL_COMMIT server=$SERVER_COMMIT — run: mdemg upgrade --edge"
  fi
fi

# Check TimescaleDB (training data collection depends on this)
TSDB_PORT="${TSDB_PORT:-5433}"
if command -v pg_isready >/dev/null 2>&1; then
  if ! pg_isready -h localhost -p "$TSDB_PORT" -q 2>/dev/null; then
    echo "⚠ TimescaleDB not running — training data collection is paused"
  fi
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

# Phase 80: Meta-cognitive anomaly detection
if [ "$OBS_COUNT" -eq 0 ] 2>/dev/null; then
  # Check if space actually has data (false-positive guard)
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
║       {"space_id":"<space>","tier":"micro"}               ║
║    2. GET /v1/memory/stats?space_id=<space>               ║
║                                                             ║
║  DO NOT PROCEED until investigated.                         ║
╚══════════════════════════════════════════════════════════════╝
ANOMALY

    # Fire-and-forget: auto-trigger micro assessment
    curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
      --connect-timeout 3 --max-time 8 -o /dev/null 2>/dev/null &

    # Record anomaly as CMS error observation
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

# Output recent observations as bullet points
if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Recent observations:"
  echo "$RESUME" | jq -r '.observations[]? | "  • [\(.obs_type // "unknown")] \(.content // .summary // "no content")"' 2>/dev/null || true
fi

# Output themes
if [ "$THEME_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Active themes:"
  echo "$RESUME" | jq -r '.themes[]? | "  • \(.name // "unnamed") (members: \(.member_count // 0))"' 2>/dev/null || true
fi

echo ""
echo "═══ END CMS CONTEXT ═══"

# Ingest claude .md files with content-hash change detection (fire-and-forget)
if [ -x "./bin/mdemg" ]; then
  mkdir -p ~/.mdemg/logs 2>/dev/null || true
  ./bin/mdemg ingest-claude-md --quiet --space-id "${SPACE_ID}" \
    2>> ~/.mdemg/logs/ingest-claude-md.log &
fi

# Staleness check: warn if periodic ingest hasn't run recently
INGEST_LOG_PATH="$HOME/.mdemg/logs/ingest-claude-md.log"
if [ -f "$INGEST_LOG_PATH" ] && [ -s "$INGEST_LOG_PATH" ]; then
  LAST_INGEST_LINE=$(tail -1 "$INGEST_LOG_PATH" 2>/dev/null || true)
  if [ -n "$LAST_INGEST_LINE" ]; then
    LAST_TS=$(echo "$LAST_INGEST_LINE" | grep -oE '^\[[0-9T:Z-]+\]' | tr -d '[]' 2>/dev/null || true)
    if [ -n "$LAST_TS" ]; then
      # Portable date parsing: macOS uses -jf, Linux/GNU uses -d
      if date -jf "%Y-%m-%dT%H:%M:%SZ" "$LAST_TS" "+%s" >/dev/null 2>&1; then
        LAST_EPOCH=$(date -jf "%Y-%m-%dT%H:%M:%SZ" "$LAST_TS" "+%s" 2>/dev/null || echo 0)
      else
        LAST_EPOCH=$(date -d "$LAST_TS" "+%s" 2>/dev/null || echo 0)
      fi
      NOW_EPOCH=$(date "+%s")
      if [ "$LAST_EPOCH" -gt 0 ] 2>/dev/null; then
        STALE_SECS=$(( NOW_EPOCH - LAST_EPOCH ))
        if [ "$STALE_SECS" -gt 7200 ]; then
          STALE_HOURS=$(( STALE_SECS / 3600 ))
          echo "⚠ Auto-ingest stale: last run was ${STALE_HOURS}h ago. Check: launchctl list | grep mdemg"
        fi
      fi
    fi
  fi
elif [ ! -f "$INGEST_LOG_PATH" ]; then
  echo "⚠ Auto-ingest log missing — periodic ingest may never have run."
fi

# Architecture map staleness check (respects UITS-optimized flag)
if command -v python3 >/dev/null 2>&1 && [ -f "scripts/generate_arch_maps.py" ]; then
  MAP_CHECK=$(python3 scripts/generate_arch_maps.py --checksum 2>&1) || {
    echo ""
    echo "⚠ Architecture maps stale — run: python3 scripts/generate_arch_maps.py"
    echo "  ${MAP_CHECK}"
  }
fi

# Phase 80: Auto RSIC health display
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

# Pre-warm Jiminy guidance for first prompt (fire-and-forget)
curl -sf -X POST "${MDEMG_URL}/v1/jiminy/warm" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"context_hint\":\"session-start\",\"session_id\":\"${SESSION_ID}\"}" \
  --connect-timeout 1 --max-time 2 -o /dev/null 2>/dev/null &

# J17: Detect whether J17 is enabled via server healthz (env var may not be in shell)
J17_ENABLED="${J17_ENABLED:-false}"
if [ "$J17_ENABLED" != "true" ]; then
  J17_CHECK=$(curl -sf "${MDEMG_URL}/v1/jiminy/ready" --connect-timeout 2 --max-time 3 2>/dev/null || true)
  if [ -n "$J17_CHECK" ] && echo "$J17_CHECK" | jq -e '.features.j17 == true' >/dev/null 2>&1; then
    J17_ENABLED="true"
  fi
fi

# J17: Bootstrap codification if 0% code coverage
if [ "$J17_ENABLED" = "true" ]; then
  PROTO_METRICS=$(curl -sf "${MDEMG_URL}/v1/jiminy/protocol/metrics" \
    --connect-timeout 2 --max-time 3 2>/dev/null || true)
  if [ -n "$PROTO_METRICS" ]; then
    CODE_COV=$(echo "$PROTO_METRICS" | jq -r '.data.code_coverage // -1' 2>/dev/null || echo "-1")
    TOTAL_EVT=$(echo "$PROTO_METRICS" | jq -r '.data.total_events // 0' 2>/dev/null || echo "0")
    if [ "$CODE_COV" = "0" ] && [ "$TOTAL_EVT" -gt "0" ] 2>/dev/null; then
      echo "J17: 0% code coverage — triggering bootstrap codification"
      curl -sf -X POST "${MDEMG_URL}/v1/self-improve/cycle" \
        -H "Content-Type: application/json" \
        -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"meso\",\"trigger_source\":\"j17-cold-start-bootstrap\"}" \
        --connect-timeout 5 --max-time 60 -o /dev/null 2>/dev/null &
    fi
  fi
fi

# J17: Restore protocol state from saved ticket
J17_STATE_RESTORED=false
if [ "$J17_ENABLED" = "true" ]; then
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
            J17_STATE_RESTORED=true
            echo ""
            echo "═══ J17 PROTOCOL RESTORED ═══"
            echo "$J17_MSG"
            echo "═══ END J17 ═══"
          fi
        fi
      fi
    fi
  fi

  # Gap 4: Cold-start bootstrap fallback — if warm resume failed or no ticket,
  # fetch bootstrap protocol so the agent knows J17 exists
  if [ "$J17_STATE_RESTORED" != "true" ]; then
    J17_BOOTSTRAP=$(curl -sf -X GET "${MDEMG_URL}/v1/jiminy/bootstrap?space_id=${SPACE_ID}" \
      --connect-timeout 3 --max-time 5 2>/dev/null || true)
    if [ -n "$J17_BOOTSTRAP" ]; then
      BOOT_HEADER=$(echo "$J17_BOOTSTRAP" | jq -r '.data.bootstrap // empty' 2>/dev/null || true)
      if [ -n "$BOOT_HEADER" ]; then
        echo ""
        echo "═══ J17 BOOTSTRAP (cold start) ═══"
        echo "$BOOT_HEADER"
        echo "═══ END J17 ═══"
      fi
    fi
  fi
fi

# --- Synergy: fingerprint + Jiminy health check ---
SYNERGY_MIGRATED=false
if [ -f ".mdemg/synergy-migrated.json" ]; then
  SYNERGY_MIGRATED=true
fi

JIMINY_OK=false
JIMINY_HEALTH_RESP=$(curl -sf "${MDEMG_URL}/v1/jiminy/healthz" --connect-timeout 2 2>/dev/null || echo "{}")
if echo "$JIMINY_HEALTH_RESP" | jq -e '.enabled == true and .status == "ok"' >/dev/null 2>&1; then
  JIMINY_OK=true
fi

# Synergy fingerprint observation (fire-and-forget)
CLAUDE_MD_LINES=0
MEMORY_MD_LINES=0
if [ -f "CLAUDE.md" ]; then
  CLAUDE_MD_LINES=$(wc -l < "CLAUDE.md" | tr -d ' ')
fi
for md_path in ~/.claude/projects/*/memory/MEMORY.md; do
  if [ -f "$md_path" ]; then
    MEMORY_MD_LINES=$(wc -l < "$md_path" | tr -d ' ')
    break
  fi
done

curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"Synergy fingerprint: CLAUDE.md=${CLAUDE_MD_LINES}L, MEMORY.md=${MEMORY_MD_LINES}L, jiminy=${JIMINY_OK}, migrated=${SYNERGY_MIGRATED}\",\"obs_type\":\"context_signal\",\"tags\":[\"synergy-fingerprint\"]}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &

# Critical warning if Jiminy down + migrated
if [ "$SYNERGY_MIGRATED" = "true" ] && [ "$JIMINY_OK" = "false" ]; then
  cat <<'SYNERGY_WARN'

!! JIMINY UNHEALTHY — .md files pruned, catastrophic forgetting risk!
Run: mdemg start --auto-migrate
SYNERGY_WARN

  # Baseline snapshot: buffer MEMORY.md to synergy-buffer space (or local JSONL)
  RECOVERY_BUFFER_SPACE="${SYNERGY_RECOVERY_BUFFER_SPACE:-synergy-buffer}"
  RECOVERY_BUFFER_PATH="${SYNERGY_RECOVERY_BUFFER_PATH:-.mdemg/synergy-recovery-buffer.jsonl}"
  MEMORY_PATH=""
  for md_path in ~/.claude/projects/*/memory/MEMORY.md; do
    if [ -f "$md_path" ]; then
      MEMORY_PATH="$md_path"
      break
    fi
  done
  if [ -n "$MEMORY_PATH" ]; then
    MEMORY_CONTENT=$(head -c 500 "$MEMORY_PATH" 2>/dev/null || true)
    if [ -n "$MEMORY_CONTENT" ]; then
      BASELINE_PAYLOAD=$(jq -nc --arg sid "$RECOVERY_BUFFER_SPACE" --arg content "$MEMORY_CONTENT" --arg sess "$SESSION_ID" \
        '{space_id: $sid, session_id: $sess, content: $content, obs_type: "constraint", tags: ["recovery-buffer", "baseline-snapshot", "jiminy-outage"]}')
      # Try Tier 1: CMS buffer space
      if ! curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
        -H "Content-Type: application/json" \
        -d "$BASELINE_PAYLOAD" \
        --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null; then
        # Tier 2: Local JSONL fallback
        mkdir -p "$(dirname "$RECOVERY_BUFFER_PATH")"
        echo "$BASELINE_PAYLOAD" >> "$RECOVERY_BUFFER_PATH"
      fi
    fi
  fi
fi

# Recovery buffer auto-flush: when Jiminy is healthy, promote buffered entries
if [ "$JIMINY_OK" = "true" ]; then
  RECOVERY_BUFFER_SPACE="${SYNERGY_RECOVERY_BUFFER_SPACE:-synergy-buffer}"
  RECOVERY_BUFFER_PATH="${SYNERGY_RECOVERY_BUFFER_PATH:-.mdemg/synergy-recovery-buffer.jsonl}"
  AUTO_FLUSH="${SYNERGY_RECOVERY_AUTO_FLUSH:-true}"
  FLUSH_COUNT=0

  if [ "$AUTO_FLUSH" = "true" ]; then
    # Throttle: max entries per session start and delay between each.
    # Prevents overwhelming Jiminy after a long outage.
    FLUSH_BATCH_SIZE="${SYNERGY_RECOVERY_FLUSH_BATCH_SIZE:-5}"
    FLUSH_DELAY_MS="${SYNERGY_RECOVERY_FLUSH_DELAY_MS:-500}"
    FLUSH_DELAY_SEC=$(echo "scale=3; $FLUSH_DELAY_MS / 1000" | bc -l 2>/dev/null || echo "0.5")
    REMAINING=0

    # Step 1: Flush local JSONL → synergy-buffer space (throttled)
    if [ -f "$RECOVERY_BUFFER_PATH" ] && [ -s "$RECOVERY_BUFFER_PATH" ]; then
      TOTAL_LOCAL=$(wc -l < "$RECOVERY_BUFFER_PATH" | tr -d ' ')
      LINE_NUM=0
      FLUSHED_LINES=""
      while IFS= read -r line; do
        LINE_NUM=$((LINE_NUM + 1))
        if [ "$FLUSH_COUNT" -ge "$FLUSH_BATCH_SIZE" ]; then
          REMAINING=$((TOTAL_LOCAL - LINE_NUM + 1))
          break
        fi
        curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
          -H "Content-Type: application/json" \
          -d "$line" \
          --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null && FLUSH_COUNT=$((FLUSH_COUNT + 1)) || true
        [ "$FLUSH_DELAY_SEC" != "0" ] && sleep "$FLUSH_DELAY_SEC" 2>/dev/null || true
      done < "$RECOVERY_BUFFER_PATH"
      # Remove flushed lines, keep remaining
      if [ "$FLUSH_COUNT" -gt 0 ]; then
        tail -n +"$((FLUSH_COUNT + 1))" "$RECOVERY_BUFFER_PATH" > "${RECOVERY_BUFFER_PATH}.tmp" 2>/dev/null
        mv "${RECOVERY_BUFFER_PATH}.tmp" "$RECOVERY_BUFFER_PATH" 2>/dev/null || true
      fi
    fi

    # Step 2: Promote synergy-buffer → mdemg-dev (only if batch budget remains)
    if [ "$FLUSH_COUNT" -lt "$FLUSH_BATCH_SIZE" ]; then
      PROMOTE_BUDGET=$((FLUSH_BATCH_SIZE - FLUSH_COUNT))
      BUFFER_ENTRIES=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/recall" \
        -H "Content-Type: application/json" \
        -d "{\"space_id\":\"${RECOVERY_BUFFER_SPACE}\",\"query\":\"recovery-buffer\",\"top_k\":${PROMOTE_BUDGET},\"filter_tags\":[\"recovery-buffer\"]}" \
        --connect-timeout 3 --max-time 10 2>/dev/null || echo "{}")

      ENTRY_COUNT=$(echo "$BUFFER_ENTRIES" | jq -r '.results | length // 0' 2>/dev/null || echo "0")
      if [ "$ENTRY_COUNT" -gt 0 ] 2>/dev/null; then
        echo "$BUFFER_ENTRIES" | jq -c '.results[]?' 2>/dev/null | while IFS= read -r entry; do
          CONTENT=$(echo "$entry" | jq -r '.content // empty' 2>/dev/null || true)
          OBS_TYPE=$(echo "$entry" | jq -r '.obs_type // "note"' 2>/dev/null || echo "note")
          if [ -n "$CONTENT" ]; then
            curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
              -H "Content-Type: application/json" \
              -d "$(jq -nc --arg c "$CONTENT" --arg t "$OBS_TYPE" \
                --arg s "$SPACE_ID" --arg sess "$SESSION_ID" '{space_id: $s, session_id: $sess, content: $c, obs_type: $t, tags: ["recovery-buffer", "promoted"]}')" \
              --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null && FLUSH_COUNT=$((FLUSH_COUNT + 1)) || true
            [ "$FLUSH_DELAY_SEC" != "0" ] && sleep "$FLUSH_DELAY_SEC" 2>/dev/null || true
          fi
        done
      fi
    fi

    if [ "$FLUSH_COUNT" -gt 0 ] 2>/dev/null; then
      echo ""
      FLUSH_MSG="Flushed ${FLUSH_COUNT} buffered entries to ${SPACE_ID}"
      if [ "$REMAINING" -gt 0 ] 2>/dev/null; then
        FLUSH_MSG="${FLUSH_MSG} (${REMAINING} remaining — will continue next session)"
      fi
      cat <<EOF
═══ RECOVERY BUFFER FLUSH ═══
${FLUSH_MSG}
═══ END RECOVERY FLUSH ═══
EOF
    fi
  fi
fi

# --- Self-improvement: reinforce recalled observations via co-activation ---
# Each session start that recalls observations should strengthen them.
# Fire-and-forget: create a session-resume observation that co-activates with
# existing observations in the claude-core session, boosting their stability.
if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"Session resumed. ${OBS_COUNT} observations, ${THEME_COUNT} themes, ${CONCEPT_COUNT} concepts recalled. Memory continuity maintained.\",\"obs_type\":\"context\",\"tags\":[\"session-resume\",\"auto-reinforce\"]}" \
    --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
fi

# Also trigger graduation check in background — graduated observations become permanent
curl -sf -X POST "${MDEMG_URL}/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
