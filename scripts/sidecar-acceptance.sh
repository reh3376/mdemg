#!/usr/bin/env bash
# sidecar-acceptance.sh — End-to-end acceptance test for MDEMG sidecar CLI.
# Validates the full sidecar command surface on a temp directory.
# Usage: bash scripts/sidecar-acceptance.sh [--binary <path>]

set -euo pipefail

BINARY="${MDEMG_BINARY:-./bin/mdemg}"
PASS=0
FAIL=0
STEPS=()
START_TIME=""

usage() {
  echo "Usage: $0 [--binary <path>]"
  echo "  --binary  Path to mdemg binary (default: ./bin/mdemg or \$MDEMG_BINARY)"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "Unknown flag: $1"; usage ;;
  esac
done

# Resolve to absolute path before cd'ing to temp dir
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

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT
cd "$TMPDIR"

# Initialize as a git repo (required by sidecar init)
git init -q .

START_TIME=$(date +%s)

echo "=== MDEMG Sidecar Acceptance Test ==="
echo "Binary: $BINARY"
echo "Temp dir: $TMPDIR"
echo ""

# Step 1: version
echo "--- version"
VERSION_OUT=$("$BINARY" version 2>&1) || true
if echo "$VERSION_OUT" | grep -qE "mdemg|version|v[0-9]"; then
  pass_step "version"
else
  fail_step "version" "unexpected output"
fi

# Step 2: sidecar init
echo "--- sidecar init"
"$BINARY" sidecar init --profile local --agents claude-code >/dev/null 2>&1 || true
if [[ -f .mdemg/sidecar.yaml ]]; then
  pass_step "sidecar init"
else
  fail_step "sidecar init" ".mdemg/sidecar.yaml not created"
fi

# Step 3: sidecar install --dry-run
echo "--- sidecar install --dry-run"
INSTALL_OUT=$("$BINARY" sidecar install --dry-run --format json 2>&1) || true
if echo "$INSTALL_OUT" | jq -e '.result' &>/dev/null; then
  DRY_RESULT=$(echo "$INSTALL_OUT" | jq -r '.result')
  if [[ "$DRY_RESULT" == "dry-run" ]]; then
    pass_step "sidecar install --dry-run"
  else
    pass_step "sidecar install --dry-run (result=$DRY_RESULT)"
  fi
else
  fail_step "sidecar install --dry-run" "invalid JSON output"
fi

# Step 4: sidecar doctor
echo "--- sidecar doctor"
DOCTOR_OUT=$("$BINARY" sidecar doctor --format json 2>&1) || true
if echo "$DOCTOR_OUT" | jq -e '.checks' &>/dev/null; then
  CHECK_COUNT=$(echo "$DOCTOR_OUT" | jq '.checks | length')
  pass_step "sidecar doctor ($CHECK_COUNT checks)"
else
  fail_step "sidecar doctor" "invalid JSON output"
fi

# Step 5: Write lock state for attach/detach (state guard requires "installed")
echo "--- write lock state"
mkdir -p .mdemg
cat > .mdemg/sidecar.lock <<'LOCKEOF'
{
  "state": "installed",
  "profile": "local",
  "endpoint": "http://localhost:9999",
  "config_hash": "testhash",
  "mdemg_version": "dev",
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-01T00:00:00Z"
}
LOCKEOF
if [[ -f .mdemg/sidecar.lock ]]; then
  pass_step "write lock state"
else
  fail_step "write lock state" "lock file not written"
fi

# Step 6: sidecar attach-agent --dry-run
echo "--- sidecar attach-agent --dry-run"
ATTACH_OUT=$("$BINARY" sidecar attach-agent claude-code --dry-run --format json 2>&1) || true
if echo "$ATTACH_OUT" | jq -e '.result' &>/dev/null; then
  ATTACH_RESULT=$(echo "$ATTACH_OUT" | jq -r '.result')
  if [[ "$ATTACH_RESULT" == "dry-run" ]]; then
    pass_step "sidecar attach-agent --dry-run"
  else
    pass_step "sidecar attach-agent --dry-run (result=$ATTACH_RESULT)"
  fi
else
  fail_step "sidecar attach-agent --dry-run" "invalid JSON output"
fi

# Step 7: sidecar detach-agent --dry-run
echo "--- sidecar detach-agent --dry-run"
DETACH_OUT=$("$BINARY" sidecar detach-agent claude-code --dry-run --format json 2>&1) || true
if echo "$DETACH_OUT" | jq -e '.result' &>/dev/null; then
  DETACH_RESULT=$(echo "$DETACH_OUT" | jq -r '.result')
  if [[ "$DETACH_RESULT" == "dry-run" ]]; then
    pass_step "sidecar detach-agent --dry-run"
  else
    pass_step "sidecar detach-agent --dry-run (result=$DETACH_RESULT)"
  fi
else
  fail_step "sidecar detach-agent --dry-run" "invalid JSON output"
fi

# Step 8: upgrade --dry-run
echo "--- upgrade --dry-run"
UPGRADE_OUT=$("$BINARY" sidecar upgrade --dry-run --format json 2>&1) || true
if echo "$UPGRADE_OUT" | jq -e '.command' &>/dev/null; then
  pass_step "upgrade --dry-run"
else
  fail_step "upgrade --dry-run" "invalid JSON output"
fi

# Step 9: uninstall --dry-run
echo "--- uninstall --dry-run"
UNINSTALL_OUT=$("$BINARY" sidecar uninstall --dry-run --format json 2>&1) || true
if echo "$UNINSTALL_OUT" | jq -e '.command' &>/dev/null; then
  pass_step "uninstall --dry-run"
else
  fail_step "uninstall --dry-run" "invalid JSON output"
fi

# Summary
END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo ""
echo "=== Acceptance Test Summary ==="
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
