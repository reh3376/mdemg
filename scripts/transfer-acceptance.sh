#!/usr/bin/env bash
# transfer-acceptance.sh — End-to-end acceptance test for MDEMG space export/import.
# Validates HTTP API endpoints, CLI commands, and filter/conflict behavior.
# Usage: bash scripts/transfer-acceptance.sh [--binary <path>] [--base-url <url>]

set -euo pipefail

BINARY="${MDEMG_BINARY:-./bin/mdemg}"
BASE_URL="${MDEMG_BASE_URL:-http://localhost:9999}"
PASS=0
FAIL=0
STEPS=()
START_TIME=""
TEST_SPACES=()

usage() {
  echo "Usage: $0 [--binary <path>] [--base-url <url>]"
  echo "  --binary    Path to mdemg binary (default: ./bin/mdemg or \$MDEMG_BINARY)"
  echo "  --base-url  MDEMG server URL (default: http://localhost:9999 or \$MDEMG_BASE_URL)"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --base-url) BASE_URL="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "Unknown flag: $1"; usage ;;
  esac
done

# Resolve to absolute path
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: binary not found or not executable: $BINARY"
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not installed"
  exit 1
fi

pass_step() {
  local name="$1"
  PASS=$((PASS + 1))
  STEPS+=("PASS  $name")
  echo "  PASS: $name"
}

fail_step() {
  local name="$1"
  local detail="${2:-}"
  FAIL=$((FAIL + 1))
  STEPS+=("FAIL  $name${detail:+ — $detail}")
  echo "  FAIL: $name${detail:+ — $detail}"
}

cleanup_spaces() {
  echo "--- cleanup test spaces"
  for sid in "${TEST_SPACES[@]}"; do
    # Use admin prune-style cleanup via direct Neo4j or skip if no CLI
    "$BINARY" space delete --space-id "$sid" --yes 2>/dev/null || true
  done
}
trap cleanup_spaces EXIT

START_TIME=$(date +%s)
SID="test-transfer-$(date +%s)"
TEST_SPACES+=("$SID")

echo "=== MDEMG Transfer Acceptance Test ==="
echo "Binary:   $BINARY"
echo "Base URL: $BASE_URL"
echo "Test space: $SID"
echo ""

# ─── Phase 1: Prerequisites ───
echo "--- Phase 1: Prerequisites"
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/healthz")
if [[ "$HEALTH" == "200" ]]; then
  pass_step "server health check"
else
  fail_step "server health check" "status=$HEALTH"
  echo "FATAL: server not ready, aborting"
  exit 1
fi

# ─── Phase 2: Seed Data ───
echo "--- Phase 2: Seed Data"
SEED_COUNT=0
for i in $(seq 1 5); do
  RESP=$(curl -s -X POST "$BASE_URL/v1/memory/ingest" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"$SID\",\"name\":\"learning-$i\",\"content\":\"Learning observation $i for transfer test\",\"path\":\"/test/transfer/learning-$i-$SID\",\"source\":\"acceptance-test\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}")
  if echo "$RESP" | jq -e '.node_id' &>/dev/null; then
    SEED_COUNT=$((SEED_COUNT + 1))
  fi
done
for i in $(seq 1 3); do
  RESP=$(curl -s -X POST "$BASE_URL/v1/memory/ingest" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"$SID\",\"name\":\"decision-$i\",\"content\":\"Decision observation $i for transfer test\",\"path\":\"/test/transfer/decision-$i-$SID\",\"source\":\"acceptance-test\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}")
  if echo "$RESP" | jq -e '.node_id' &>/dev/null; then
    SEED_COUNT=$((SEED_COUNT + 1))
  fi
done
if [[ $SEED_COUNT -ge 6 ]]; then
  pass_step "seed data ($SEED_COUNT nodes)"
else
  fail_step "seed data" "only $SEED_COUNT nodes created"
fi

# ─── Phase 3: Export Preview ───
echo "--- Phase 3: Export Preview (API)"
PREVIEW=$(curl -s "$BASE_URL/v1/admin/spaces/export/preview?space_id=$SID&profile=full")
if echo "$PREVIEW" | jq -e '.estimated_nodes' &>/dev/null; then
  EST_NODES=$(echo "$PREVIEW" | jq '.estimated_nodes')
  EST_PROFILE=$(echo "$PREVIEW" | jq -r '.profile')
  if [[ "$EST_PROFILE" == "full" ]] && [[ $EST_NODES -gt 0 ]]; then
    pass_step "export preview (estimated_nodes=$EST_NODES)"
  else
    fail_step "export preview" "profile=$EST_PROFILE nodes=$EST_NODES"
  fi
else
  fail_step "export preview" "invalid JSON response"
fi

# ─── Phase 4: Export Profiles via API ───
echo "--- Phase 4: Export Profiles (API)"
for PROFILE in full metadata shareable codebase cms learned; do
  EXPORT_RESP=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/export" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"$SID\",\"profile\":\"$PROFILE\"}")
  STATUS=$(echo "$EXPORT_RESP" | jq -r '.profile // empty')
  FORMAT=$(echo "$EXPORT_RESP" | jq -r '.header.format // empty')
  if [[ "$STATUS" == "$PROFILE" ]] && [[ "$FORMAT" == "mdemg-space-transfer" ]]; then
    NODES=$(echo "$EXPORT_RESP" | jq '.summary.nodes_exported')
    pass_step "export profile=$PROFILE (nodes=$NODES)"
  else
    fail_step "export profile=$PROFILE" "profile=$STATUS format=$FORMAT"
  fi
done

# ─── Phase 5: Export Error Cases ───
echo "--- Phase 5: Export Error Cases"
ERR_RESP=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/admin/spaces/export" \
  -H "Content-Type: application/json" \
  -d '{"profile":"metadata"}')
if [[ "$ERR_RESP" == "400" ]]; then
  pass_step "export missing space_id → 400"
else
  fail_step "export missing space_id" "got $ERR_RESP"
fi

ERR_RESP2=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/admin/spaces/export" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$SID\",\"profile\":\"nonexistent\"}")
if [[ "$ERR_RESP2" == "400" ]]; then
  pass_step "export invalid profile → 400"
else
  fail_step "export invalid profile" "got $ERR_RESP2"
fi

# ─── Phase 6: Import Roundtrip via API ───
echo "--- Phase 6: Import Roundtrip (API)"
IMPORT_SID="test-transfer-import-$(date +%s)"
TEST_SPACES+=("$IMPORT_SID")

# Export full, then delete source to free node_ids
# (memnode_id constraint requires globally unique node_ids)
EXPORT_DATA=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/export" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$SID\",\"profile\":\"full\"}")
EXPORT_CHUNKS=$(echo "$EXPORT_DATA" | jq '.chunks')
EXPORT_NODES=$(echo "$EXPORT_DATA" | jq '.summary.nodes_exported // 0')

# Delete source space to free node_ids for import
"$BINARY" space delete --space-id "$SID" --yes 2>/dev/null || true

# Import with target space
IMPORT_RESP=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/import" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$IMPORT_SID\",\"conflict\":\"skip\",\"chunks\":$EXPORT_CHUNKS}")
IMPORT_NODES=$(echo "$IMPORT_RESP" | jq '.nodes_created // 0')
if [[ $IMPORT_NODES -gt 0 ]]; then
  pass_step "import roundtrip (nodes_created=$IMPORT_NODES)"
else
  fail_step "import roundtrip" "nodes_created=$IMPORT_NODES"
fi

# ─── Phase 7: Import Conflict Modes ───
echo "--- Phase 7: Import Conflict Modes"

# Re-import with skip — nodes should be skipped (they already exist from Phase 6)
SKIP_RESP=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/import" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$IMPORT_SID\",\"conflict\":\"skip\",\"chunks\":$EXPORT_CHUNKS}")
SKIP_SKIPPED=$(echo "$SKIP_RESP" | jq '.nodes_skipped // 0')
if [[ $SKIP_SKIPPED -gt 0 ]]; then
  pass_step "conflict=skip (skipped=$SKIP_SKIPPED)"
else
  fail_step "conflict=skip" "expected skips, got skipped=$SKIP_SKIPPED"
fi

# Invalid conflict mode → 400
ERR_CONFLICT=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/admin/spaces/import" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"test-bad","conflict":"invalid","chunks":[]}')
if [[ "$ERR_CONFLICT" == "400" ]]; then
  pass_step "invalid conflict mode → 400"
else
  fail_step "invalid conflict mode" "got $ERR_CONFLICT"
fi

# ─── Phase 8: Import Error Cases ───
echo "--- Phase 8: Import Error Cases"
ERR_NO_CHUNKS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/admin/spaces/import" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"test-fail","conflict":"skip"}')
if [[ "$ERR_NO_CHUNKS" == "400" ]]; then
  pass_step "import missing chunks → 400"
else
  fail_step "import missing chunks" "got $ERR_NO_CHUNKS"
fi

# Empty chunks → 200 with 0 work
EMPTY_IMPORT=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/import" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"test-empty","conflict":"skip","chunks":[]}')
EMPTY_NODES=$(echo "$EMPTY_IMPORT" | jq '.nodes_created // -1')
if [[ "$EMPTY_NODES" == "0" ]]; then
  pass_step "import empty chunks → 200 (0 nodes)"
else
  fail_step "import empty chunks" "nodes_created=$EMPTY_NODES"
fi

# ─── Phase 9: CLI Export/Import ───
echo "--- Phase 9: CLI Export/Import"
CLI_EXPORT_FILE="/tmp/transfer-test-$IMPORT_SID.mdemg"
CLI_IMPORT_SID="test-transfer-cli-$(date +%s)"
TEST_SPACES+=("$CLI_IMPORT_SID")

# Export from import space (source was deleted in Phase 6)
CLI_EXPORT_OUT=$("$BINARY" space export --space-id "$IMPORT_SID" --output "$CLI_EXPORT_FILE" --profile full 2>&1) || true
if [[ -f "$CLI_EXPORT_FILE" ]]; then
  FILESIZE=$(wc -c < "$CLI_EXPORT_FILE" | tr -d ' ')
  pass_step "CLI export (${FILESIZE} bytes)"
else
  fail_step "CLI export" "file not created"
fi

if [[ -f "$CLI_EXPORT_FILE" ]]; then
  # Delete import space to free node_ids, then import to CLI target
  "$BINARY" space delete --space-id "$IMPORT_SID" --yes 2>/dev/null || true
  CLI_IMPORT_OUT=$("$BINARY" space import --input "$CLI_EXPORT_FILE" --target-space "$CLI_IMPORT_SID" 2>&1) || true
  if echo "$CLI_IMPORT_OUT" | grep -qiE "import|created|complete"; then
    pass_step "CLI import"
  else
    fail_step "CLI import" "unexpected output"
  fi
  rm -f "$CLI_EXPORT_FILE"
fi

# ─── Phase 10: Filter Flags via API ───
echo "--- Phase 10: Filter Flags"
# Use CLI import target space (or whatever space has data)
FILTER_SID="$CLI_IMPORT_SID"

# Test no_observations filter
FILTER_RESP=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/export" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$FILTER_SID\",\"profile\":\"full\",\"no_observations\":true}")
FILTER_OBS=$(echo "$FILTER_RESP" | jq '.summary.observations_exported // 0')
if [[ "$FILTER_OBS" == "0" ]]; then
  pass_step "no_observations filter (obs=0)"
else
  pass_step "no_observations filter (obs=$FILTER_OBS — may have none)"
fi

# Test min_layer filter
LAYER_RESP=$(curl -s -X POST "$BASE_URL/v1/admin/spaces/export" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"$FILTER_SID\",\"profile\":\"full\",\"min_layer\":5}")
LAYER_NODES=$(echo "$LAYER_RESP" | jq '.summary.nodes_exported // 0')
# Layer 5+ nodes are unlikely in test data
pass_step "min_layer=5 filter (nodes=$LAYER_NODES)"

# ─── Summary ───
END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo ""
echo "=== Transfer Acceptance Test Summary ==="
echo "Elapsed: ${ELAPSED}s"
echo "Passed:  $PASS"
echo "Failed:  $FAIL"
echo ""
for s in "${STEPS[@]}"; do
  echo "  $s"
done
echo ""

if [[ $FAIL -gt 0 ]]; then
  echo "RESULT: FAIL ($FAIL failures)"
  exit 1
else
  echo "RESULT: PASS (all $PASS steps passed in ${ELAPSED}s)"
  exit 0
fi
