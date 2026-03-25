#!/usr/bin/env bash
# Hook: SessionStart — restore CMS memory context on every session start
# This runs automatically before Claude sees any user input.

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-http://localhost:9999}"
SESSION_ID="claude-core"
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
║       {"space_id":"mdemg-dev","tier":"micro"}               ║
║    2. GET /v1/memory/stats?space_id=mdemg-dev               ║
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
  ./bin/mdemg ingest-claude-md --quiet --space-id "${SPACE_ID}" 2>/dev/null &
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
  1. GET /v1/memory/stats?space_id=mdemg-dev
  2. GET /v1/learning/stats?space_id=mdemg-dev
  3. POST /v1/self-improve/cycle {"space_id":"mdemg-dev","tier":"meso"}
DEGRADED
  fi
fi

# J17: Restore protocol state from saved ticket
J17_STATE_RESTORED=false
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
      BASELINE_PAYLOAD=$(jq -nc --arg sid "$RECOVERY_BUFFER_SPACE" --arg content "$MEMORY_CONTENT" \
        '{space_id: $sid, session_id: "claude-core", content: $content, obs_type: "constraint", tags: ["recovery-buffer", "baseline-snapshot", "jiminy-outage"]}')
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
                '{space_id: "mdemg-dev", session_id: "claude-core", content: $c, obs_type: $t, tags: ["recovery-buffer", "promoted"]}')" \
              --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null && FLUSH_COUNT=$((FLUSH_COUNT + 1)) || true
            [ "$FLUSH_DELAY_SEC" != "0" ] && sleep "$FLUSH_DELAY_SEC" 2>/dev/null || true
          fi
        done
      fi
    fi

    if [ "$FLUSH_COUNT" -gt 0 ] 2>/dev/null; then
      echo ""
      FLUSH_MSG="Flushed ${FLUSH_COUNT} buffered entries to mdemg-dev"
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
