#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}"
REPO_DIR=""

usage() {
  cat <<'EOF'
Usage:
  BUILD_PACKAGE.sh [--repo-root <path>] [--output <path>]

Options:
  --repo-root <path>  Repository root that contains docs/uxts01 and live framework files.
                      If omitted, auto-discovery walks upward from the script directory.
  --output <path>     Output directory for the package. Defaults to this script directory.
  --help              Show this help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || { echo "ERROR: --repo-root requires a value" >&2; exit 2; }
      REPO_DIR="$(cd "$2" && pwd)"
      shift 2
      ;;
    --output)
      [[ $# -ge 2 ]] || { echo "ERROR: --output requires a value" >&2; exit 2; }
      ROOT_DIR="$(mkdir -p "$2" && cd "$2" && pwd)"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${REPO_DIR}" ]]; then
  probe="${SCRIPT_DIR}"
  while [[ "${probe}" != "/" ]]; do
    if [[ -d "${probe}/docs/uxts01" && -f "${probe}/docs/specs/UXTS_PORTABLE_AGENT_SPEC.md" ]]; then
      REPO_DIR="${probe}"
      break
    fi
    probe="$(cd "${probe}/.." && pwd)"
  done
fi

if [[ -z "${REPO_DIR}" ]]; then
  echo "ERROR: could not auto-discover repo root. Provide --repo-root explicitly." >&2
  exit 2
fi

required_sources=(
  "${REPO_DIR}/docs/specs/UXTS_PORTABLE_AGENT_SPEC.md"
  "${REPO_DIR}/docs/specs/UXTS_PORTABLE_AGENT_SPEC02.md"
  "${REPO_DIR}/docs/api/api-spec/uats/README.md"
  "${REPO_DIR}/docs/api/api-spec/uats/schema/uats.schema.json"
  "${REPO_DIR}/docs/api/api-spec/uats/runners/uats_runner.py"
  "${REPO_DIR}/docs/api/api-spec/uats/specs/health.uats.json"
  "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/README.md"
  "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json"
  "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py"
  "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/specs/go.upts.json"
  "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/fixtures/go_test_fixture.go"
  "${REPO_DIR}/docs/uxts01/README.md"
)

for src in "${required_sources[@]}"; do
  [[ -f "${src}" ]] || { echo "ERROR: missing source file: ${src}" >&2; exit 2; }
done

# When writing to an external output directory, copy package docs/scaffolding first.
if [[ "${ROOT_DIR}" != "${SCRIPT_DIR}" ]]; then
  rsync -a --delete --exclude 'source/' --exclude 'PACKAGE_MANIFEST.md' "${SCRIPT_DIR}/" "${ROOT_DIR}/"
fi

rm -rf "${ROOT_DIR}/source"
mkdir -p "${ROOT_DIR}/source/original"
mkdir -p "${ROOT_DIR}/source/uxts01"
mkdir -p "${ROOT_DIR}/source/live-frameworks/uats"
mkdir -p "${ROOT_DIR}/source/live-frameworks/upts"

cp "${REPO_DIR}/docs/specs/UXTS_PORTABLE_AGENT_SPEC.md" "${ROOT_DIR}/source/original/"
cp "${REPO_DIR}/docs/specs/UXTS_PORTABLE_AGENT_SPEC02.md" "${ROOT_DIR}/source/original/"

cp "${REPO_DIR}/docs/api/api-spec/uats/README.md" "${ROOT_DIR}/source/live-frameworks/uats/"
cp "${REPO_DIR}/docs/api/api-spec/uats/schema/uats.schema.json" "${ROOT_DIR}/source/live-frameworks/uats/"
cp "${REPO_DIR}/docs/api/api-spec/uats/runners/uats_runner.py" "${ROOT_DIR}/source/live-frameworks/uats/"
cp "${REPO_DIR}/docs/api/api-spec/uats/specs/health.uats.json" "${ROOT_DIR}/source/live-frameworks/uats/"

cp "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/README.md" "${ROOT_DIR}/source/live-frameworks/upts/"
cp "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json" "${ROOT_DIR}/source/live-frameworks/upts/"
cp "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py" "${ROOT_DIR}/source/live-frameworks/upts/"
cp "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/specs/go.upts.json" "${ROOT_DIR}/source/live-frameworks/upts/"
cp "${REPO_DIR}/docs/lang-parser/lang-parse-spec/upts/fixtures/go_test_fixture.go" "${ROOT_DIR}/source/live-frameworks/upts/"

rsync -a --delete --delete-excluded --exclude '__pycache__/' --exclude 'agent-package' "${REPO_DIR}/docs/uxts01/" "${ROOT_DIR}/source/uxts01/"

manifest="${ROOT_DIR}/PACKAGE_MANIFEST.md"
{
  echo '# UxXTS Agent Package Manifest'
  echo
  echo "Generated: $(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  echo
  echo '## Inventory'
  echo
  echo '| File | Bytes | SHA-256 |'
  echo '|---|---:|---|'
  while IFS= read -r f; do
    bytes=$(wc -c < "$f" | tr -d ' ')
    sha=$(shasum -a 256 "$f" | awk '{print $1}')
    if [[ "${f}" == "${REPO_DIR}/"* ]]; then
      rel="${f#${REPO_DIR}/}"
    else
      rel="${f#${ROOT_DIR}/}"
    fi
    echo "| \`${rel}\` | ${bytes} | \`${sha}\` |"
  done < <(find "${ROOT_DIR}" -type f ! -name 'PACKAGE_MANIFEST.md' | sort)
} > "${manifest}"

echo "Package refreshed at ${ROOT_DIR}"
echo "Repo source root: ${REPO_DIR}"
