#!/usr/bin/env bash
# synergy-migrate.sh — Migrate archival content from Claude Code auto-memory to MDEMG CMS.
# See: plan "Claude Code ↔ MDEMG Synergy Optimization"
#
# Usage: ./scripts/synergy-migrate.sh [--dry-run] [--force] [--space-id SPACE]
#
# Requires: MDEMG server running with Jiminy enabled.
# Dev safety: files are moved to ~/mdemg/temp/ (not deleted).

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-http://localhost:9999}"
SPACE_ID="${MDEMG_SPACE_ID:-mdemg-dev}"
DRY_RUN=false
FORCE=false
MIGRATION_FLAG=".mdemg/synergy-migrated.json"

# Parse args
while [[ $# -gt 0 ]]; do
  case $1 in
    --dry-run)  DRY_RUN=true; shift ;;
    --force)    FORCE=true; shift ;;
    --space-id) SPACE_ID="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# ─── Pre-checks ───

# Check migration flag
if [ "$FORCE" = "false" ] && [ -f "$MIGRATION_FLAG" ]; then
  VERSION=$(jq -r '.version // "unknown"' "$MIGRATION_FLAG" 2>/dev/null || echo "unknown")
  DATE=$(jq -r '.migrated_at // "unknown"' "$MIGRATION_FLAG" 2>/dev/null || echo "unknown")
  echo "Already migrated (version: $VERSION, date: $DATE)"
  echo "Use --force to re-run."
  exit 0
fi

# Check server
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
  echo "ERROR: MDEMG server not running at ${MDEMG_URL}"
  echo "Start with: mdemg start --auto-migrate"
  exit 1
fi

# Jiminy health gate
JIMINY_HEALTH=$(curl -sf "${MDEMG_URL}/v1/jiminy/healthz" --connect-timeout 2 2>/dev/null || echo '{}')
JIMINY_ENABLED=$(echo "$JIMINY_HEALTH" | jq -r '.enabled // false' 2>/dev/null || echo "false")
JIMINY_STATUS=$(echo "$JIMINY_HEALTH" | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")

if [ "$JIMINY_ENABLED" != "true" ] || [ "$JIMINY_STATUS" != "ok" ]; then
  echo "ERROR: Jiminy health check failed — aborting to prevent catastrophic forgetting."
  echo "  Enabled: $JIMINY_ENABLED, Status: $JIMINY_STATUS"
  echo "  Ensure JIMINY_ENABLED=true and restart the server."
  exit 1
fi

# Test Jiminy guidance
JIMINY_TEST=$(curl -sf -X POST "${MDEMG_URL}/v1/jiminy/guide" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"'"${SPACE_ID}"'","context":"synergy migration health check"}' \
  --connect-timeout 5 --max-time 40 2>/dev/null || echo "")
if [ -z "$JIMINY_TEST" ]; then
  echo "ERROR: Jiminy guidance test returned empty — aborting."
  exit 1
fi

echo "Jiminy health: OK"

# ─── Find memory directory ───

MEMORY_DIR=""
for candidate in ~/.claude/projects/*/memory/; do
  if [ -f "${candidate}MEMORY.md" ]; then
    MEMORY_DIR="$candidate"
    break
  fi
done

if [ -z "$MEMORY_DIR" ]; then
  echo "ERROR: Could not find MEMORY.md in ~/.claude/projects/*/memory/"
  exit 1
fi

echo "Memory directory: $MEMORY_DIR"

# ─── Prepare temp directories ───

TEMP_BASE="${HOME}/mdemg/temp"
mkdir -p "${TEMP_BASE}/obsolete" "${TEMP_BASE}/migrated" "${TEMP_BASE}/archived"

# ─── Helper: observe to CMS ───

observe_cms() {
  local content="$1"
  local obs_type="$2"
  local tags="$3"

  if [ "$DRY_RUN" = "true" ]; then
    echo "  [dry-run] Would observe: obs_type=$obs_type, tags=$tags"
    return 0
  fi

  curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg s "$SPACE_ID" --arg sid "synergy-migrate" \
      --arg c "$content" --arg t "$obs_type" --argjson tags "$tags" \
      '{space_id: $s, session_id: $sid, content: $c, obs_type: $t, tags: $tags}')" \
    --connect-timeout 3 --max-time 5 -o /dev/null 2>/dev/null || true
}

move_file() {
  local src="$1"
  local dest_dir="$2"
  local filename
  filename=$(basename "$src")

  if [ "$DRY_RUN" = "true" ]; then
    echo "  [dry-run] Would move: $filename → $dest_dir/"
    return 0
  fi

  if [ -f "$src" ]; then
    mv "$src" "${dest_dir}/${filename}"
    echo "  Moved: $filename → $dest_dir/"
  fi
}

# ─── Step 1: Obsolete files (move without ingestion) ───

echo ""
echo "=== Step 1: Obsolete files ==="

OBSOLETE_FILES=("vscode-extension.md" "feedback_plan_before_code.md")
for f in "${OBSOLETE_FILES[@]}"; do
  path="${MEMORY_DIR}${f}"
  if [ -f "$path" ]; then
    move_file "$path" "${TEMP_BASE}/obsolete"
  else
    echo "  SKIP $f (not found)"
  fi
done

# ─── Step 2: Ingest then move files ───

echo ""
echo "=== Step 2: Ingest + move auto-memory files ==="

MIGRATE_PAIRS=(
  "cognitive-architecture.md:learning"
  "feedback_cuidv2_standard.md:constraint"
  "feedback_fix_all_failures.md:constraint"
  "feedback_live_testing_mode.md:preference"
  "feedback_dev_plan_standard_tasks.md:constraint"
  "project_mdemg_windows_docs.md:task"
  "project_menubar_multi_instance.md:task"
  "project_next_tasks_queue.md:task"
)

MIGRATED_FILES=()
for pair in "${MIGRATE_PAIRS[@]}"; do
  f="${pair%%:*}"
  obs_type="${pair##*:}"
  path="${MEMORY_DIR}${f}"

  if [ ! -f "$path" ]; then
    echo "  SKIP $f (not found)"
    continue
  fi

  content=$(cat "$path")
  observe_cms "$content" "$obs_type" '["synergy-migration","v1"]'
  move_file "$path" "${TEMP_BASE}/migrated"
  MIGRATED_FILES+=("$f")
done

# ─── Step 3: Write migration flag ───

echo ""
echo "=== Step 3: Migration flag ==="

if [ "$DRY_RUN" = "true" ]; then
  echo "  [dry-run] Would write $MIGRATION_FLAG"
else
  mkdir -p .mdemg
  FILES_JSON=$(printf '%s\n' "${MIGRATED_FILES[@]}" | jq -R . | jq -s .)
  jq -n --arg v "v1" --arg d "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg s "$SPACE_ID" --argjson files "$FILES_JSON" \
    '{migrated: true, version: $v, migrated_at: $d, space_id: $s, files_migrated: $files}' \
    > "$MIGRATION_FLAG"
  echo "  Written: $MIGRATION_FLAG"
fi

echo ""
echo "=== Migration complete ==="
echo "Files processed: ${#MIGRATED_FILES[@]} ingested, ${#OBSOLETE_FILES[@]} obsolete"
echo ""
echo "Next steps:"
echo "  1. Trim CLAUDE.md (manually or via plan)"
echo "  2. Trim MEMORY.md (manually or via plan)"
echo "  3. Verify: mdemg synergy status"
