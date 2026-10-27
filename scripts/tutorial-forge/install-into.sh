#!/usr/bin/env bash
# tutorial-forge cross-repo installer.
#
# Copies the framework into a target repo and scaffolds a minimum config.
# Run from inside the TARGET repo (the one that wants a tutorial):
#
#     bash <(curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/tutorial-forge/install-into.sh)
#
# Or, if you've already cloned the mdemg source somewhere, run it with a
# path to the forge checkout:
#
#     bash /path/to/mdemg/scripts/tutorial-forge/install-into.sh
#
# Idempotent: re-running updates the framework files in place but leaves
# your tutorial.config.json + deck source untouched.

set -euo pipefail

# --- Resolve source: either this script's own directory, or a fresh fetch
# -------------------------------------------------------------------------
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
  SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  SRC_DIR=""
fi

if [[ -z "$SRC_DIR" || ! -f "$SRC_DIR/build.mjs" ]]; then
  # Running from `curl | bash` — clone a shallow copy of the source repo.
  echo "[forge-install] fetching tutorial-forge from reh3376/mdemg..."
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT
  git clone --quiet --depth=1 https://github.com/reh3376/mdemg.git "$TMP/src"
  SRC_DIR="$TMP/src/scripts/tutorial-forge"
fi

TARGET_ROOT="$(pwd)"
TARGET_FORGE="$TARGET_ROOT/scripts/tutorial-forge"

echo "[forge-install] source: $SRC_DIR"
echo "[forge-install] target: $TARGET_FORGE"

if [[ -d "$TARGET_FORGE" ]]; then
  echo "[forge-install] updating existing install..."
else
  echo "[forge-install] fresh install..."
  mkdir -p "$TARGET_FORGE"
fi

# Copy framework code (NOT example/, which belongs to each repo).
rsync -a --delete \
  --exclude='node_modules/' \
  --exclude='example/dist/' \
  "$SRC_DIR/" "$TARGET_FORGE/"

# Scaffold the config at repo root if missing.
if [[ ! -f "$TARGET_ROOT/tutorial.config.json" ]]; then
  REPO_NAME="$(basename "$TARGET_ROOT")"
  echo "[forge-install] creating starter tutorial.config.json..."
  cat > "$TARGET_ROOT/tutorial.config.json" <<EOF
{
  "_comment": "Minimum config. See scripts/tutorial-forge/README.md for the full schema.",
  "deck": "docs/tutorial.md",
  "output": "build/tutorial-static",
  "title": "$REPO_NAME Tutorial"
}
EOF
else
  echo "[forge-install] tutorial.config.json already present — left as-is"
fi

# Scaffold a starter deck if missing.
if [[ ! -f "$TARGET_ROOT/docs/tutorial.md" ]]; then
  mkdir -p "$TARGET_ROOT/docs"
  echo "[forge-install] creating starter docs/tutorial.md..."
  cat > "$TARGET_ROOT/docs/tutorial.md" <<'EOF'
# My Project Tutorial

Welcome. Edit this file to build your onboarding deck. See the example at
`scripts/tutorial-forge/example/deck.md` for the full set of conventions
(mermaid diagrams, speaker notes, acronym glossary, pinned anchor IDs).

## Section One

Standard markdown. Fenced code blocks get syntax highlighting:

```python
def hello():
    print("Hello, World!")
```

## Section Two

Mermaid diagrams get pre-rendered to SVG + interactive zoom/pan:

```mermaid
flowchart TD
  A[Start] --> B[Work]
  B --> C[Done]
```

That's it — rebuild with `node scripts/tutorial-forge/cli.mjs build --config tutorial.config.json`.
EOF
else
  echo "[forge-install] docs/tutorial.md already present — left as-is"
fi

# Append .gitignore entries if missing.
if [[ -f "$TARGET_ROOT/.gitignore" ]]; then
  for ignore in "scripts/tutorial-forge/node_modules/" "scripts/tutorial-forge/example/dist/" "build/tutorial-static/" "mdemg-tutorial.zip"; do
    if ! grep -qxF "$ignore" "$TARGET_ROOT/.gitignore"; then
      echo "$ignore" >> "$TARGET_ROOT/.gitignore"
    fi
  done
  echo "[forge-install] appended .gitignore entries"
fi

echo ""
echo "[forge-install] DONE."
echo ""
echo "Next steps:"
echo "  1. cd scripts/tutorial-forge && npm install"
echo "     (downloads Chromium for mermaid; ~150 MB one-time)"
echo "  2. Edit docs/tutorial.md with your content."
echo "  3. node scripts/tutorial-forge/cli.mjs build --config tutorial.config.json"
echo "  4. Serve build/tutorial-static/ with any static host."
echo ""
echo "For every feature + the config schema: scripts/tutorial-forge/README.md"
echo "For the AI-agent reference:            scripts/tutorial-forge/FSD.md"
