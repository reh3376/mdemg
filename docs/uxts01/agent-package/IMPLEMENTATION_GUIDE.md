# UxXTS Implementation Guide (Start-to-Finish)

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Status: Draft
Date: 2026-02-28

## End Goal

Implement UxXTS so recurring constructs are generated and governed programmatically with minimal assumptions.

The final state must support:

1. intent-driven item creation,
2. deterministic artifact generation,
3. required unit/integration verification,
4. schema/parity/integrity enforcement, and
5. deterministic CI gate outcomes.

## Phase 0: Baseline Installation

1. Copy this `agent-package/` directory into the target repository.
2. Ensure Python 3 is available.
3. Install tooling dependency:
   - `pip install jsonschema`
4. Run a lint/hash smoke check:
   - `python3 source/uxts01/tools/uxxts_lint.py --framework-dir source/uxts01/examples/frameworks/uats --spec source/uxts01/examples/frameworks/uats/specs/health.uats.json --check-hash --allow-unresolved-env`

## Live Reference Artifacts

Use real framework implementations as portability references:

1. `source/live-frameworks/uats/`
   - `README.md`, `uats.schema.json`, `uats_runner.py`, `health.uats.json`
2. `source/live-frameworks/upts/`
   - `README.md`, `upts.schema.json`, `upts_runner.py`, `go.upts.json`, `go_test_fixture.go`

## Phase 1: Framework Contract Alignment

1. Select target framework (`uats`, `upts`, `uots`, etc.).
2. Confirm schema and runner parity status for selected framework.
3. Classify schema fields as `enforced`, `advisory`, or `unimplemented`.
4. Fail fast on unimplemented required features.

Mandatory output:

1. parity matrix artifact (`enforced/advisory/unimplemented`).

## Phase 2: Intent-to-Item Pipeline Activation

Use the additive contract in:

- `source/uxts01/UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md`

Implement:

1. intent packet ingestion
2. deterministic generation
3. stage runner for `T0`-`T4`
4. CI gate policy mapping (`observe`, `soft`, `block`)

Mandatory generated artifacts per item:

1. `.uxxts/generated/<framework_id>/<item_id>/intent-to-item.manifest.json`
2. `.uxxts/generated/<framework_id>/<item_id>/intent-to-item.report.json`
3. optional waiver file when required stage cannot execute

## Phase 3: Verification and Governance

For each generated item:

1. Run `T0`: schema + parity + integrity preflight.
2. Run `T1`: runner execution and canonical report validation.
3. Run `T2`: required unit tests.
4. Run `T3`: required integration tests.
5. Run `T4`: governance checks (hash/drift/parity/report schema).

CI gate behavior:

1. `observe`: block only on safety gate failures.
2. `soft`: block on safety gates and execution `error` outcomes.
3. `block`: block on safety gates, any fail/error, hash mismatches on touched artifacts, and missing required stages.

## Phase 4: Brownfield Expansion

1. Discover recurring patterns in current codebase.
2. Score opportunities by frequency x risk x maintenance drag.
3. Extend existing framework first.
4. Create new framework only with explicit overlap/ownership justification.

## Phase 5: Operational Hardening

1. Add deterministic conformance checks to CI.
2. Add drift checks for schema/spec/runner consistency.
3. Add periodic integrity enforcement.
4. Track waivers with owner + expiration.

## Reference Commands

```bash
# Lint and hash check a spec
python3 source/uxts01/tools/uxxts_lint.py --framework-dir <framework_dir> --spec <spec_file> --check-hash

# Print computed hash only
python3 source/uxts01/tools/uxxts_lint.py --framework-dir <framework_dir> --spec <spec_file> --print-hash

# Initialize a new framework skeleton
python3 source/uxts01/tools/uxxts_init.py --root . --framework-id uats --framework-title "Universal API Test Specification"
```

## Required Deliverables Checklist

1. Framework schema(s)
2. Declarative specs
3. Runner implementation with parity behavior
4. Canonical report output
5. CI gate wiring
6. Intent-to-item manifest/report generation
7. Conformance evidence
8. Drift/integrity evidence

## Common Failure Modes

1. Schema declares features runner does not enforce.
2. Hash failure blocks assertion execution (incorrect behavior).
3. Assertion grammar drift across docs/schema/runner.
4. Missing unit/integration stage despite declared requirement.
5. Untracked manual edits outside manifest.

## Definition of Done

A target repository is complete when:

1. at least one high-frequency construct is generated from intent end-to-end,
2. all required stages execute with deterministic outcomes,
3. CI gate policy is enforced per mode,
4. parity is explicit and no silent ignore path remains.
