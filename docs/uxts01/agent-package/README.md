# UxXTS Agent Implementation Package

Status: Draft
Date: 2026-02-28
Audience: AI coding agents and developers new to UxXTS/UxXTS.

## End Goal

Use this package to implement UxXTS in any repository from zero context to production governance.

A successful implementation means:

1. a framework-specific item can be created from intent,
2. schema/parity/integrity checks execute deterministically,
3. required unit/integration validation is enforced, and
4. CI gates produce a clear `observe` / `soft` / `block` decision.

## Package Structure

- `START_HERE.md`
  - Minimal bootstrap path for first-time agents.
- `IMPLEMENTATION_GUIDE.md`
  - End-to-end implementation instructions and phase plan.
- `AGENT_BOOTSTRAP_PROMPT.md`
  - Copy/paste prompt for assigning work to another coding agent.
- `RELEASE_NOTES_A002_A005.md`
  - Short remediation summary for development teams.
- `PACKAGE_MANIFEST.md`
  - Inventory and integrity checksums for packaged files.
- `BUILD_PACKAGE.sh`
  - Refresh script that re-syncs source docs and regenerates manifest.
- `source/original/`
  - Original UXTS portable specs (`UXTS_PORTABLE_AGENT_SPEC*.md`).
- `source/uxts01/`
  - Consolidated UxXTS01 package, schemas, examples, conformance suite, tools.
- `source/live-frameworks/`
  - Real framework reference artifacts from this repo:
    - `uats/` (README, schema, runner, sample spec)
    - `upts/` (README, schema, runner, sample spec + fixture)

## Required Reading Order

1. `START_HERE.md`
2. `source/original/UXTS_PORTABLE_AGENT_SPEC.md`
3. `source/original/UXTS_PORTABLE_AGENT_SPEC02.md`
4. `source/uxts01/UxXTS01_PORTABLE_AGENT_SPEC.md`
5. `source/uxts01/UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md`
6. `source/live-frameworks/uats/README.md`
7. `source/live-frameworks/upts/README.md`
8. `IMPLEMENTATION_GUIDE.md`

## Quick Validation

Run these checks after copying this package into a target repository:

```bash
python3 source/uxts01/tools/uxxts_lint.py --framework-dir source/uxts01/examples/frameworks/uats --spec source/uxts01/examples/frameworks/uats/specs/health.uats.json --check-hash --allow-unresolved-env
```

If `jsonschema` is missing, install it first:

```bash
pip install jsonschema
```

## Refresh This Package

From repository root:

```bash
bash docs/uxts01/agent-package/BUILD_PACKAGE.sh
```

Portable mode (explicit source repo + external output path):

```bash
bash docs/uxts01/agent-package/BUILD_PACKAGE.sh \
  --repo-root /path/to/mdemg \
  --output /tmp/uxxts-agent-package
```
