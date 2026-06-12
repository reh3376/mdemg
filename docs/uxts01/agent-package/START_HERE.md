# Start Here: UxXTS Bootstrap

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Objective

Get a new coding agent from zero context to first governed framework implementation with minimal assumptions.

## Step 1: Establish Orientation

Read in this exact order:

1. `source/original/UXTS_PORTABLE_AGENT_SPEC.md`
2. `source/original/UXTS_PORTABLE_AGENT_SPEC02.md`
3. `source/uxts01/UxXTS01_PORTABLE_AGENT_SPEC.md`
4. `source/uxts01/UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md`
5. `source/live-frameworks/uats/README.md`
6. `source/live-frameworks/upts/README.md`

## Step 2: Confirm Core Rules

The agent must explicitly confirm understanding of these rules before coding:

1. UxXTS is additive governance, not a replacement for unit/integration/E2E suites.
2. Schema-runner parity is mandatory for active frameworks.
3. Integrity signals and assertion outcomes are independent.
4. Brownfield mode is extension-first.
5. Intent-to-item automation must preserve deterministic output and gate behavior.

## Step 3: Choose First Implementation Target

Recommended first target:

1. `UATS` endpoint item generation from intent packet.

Reason:

1. High recurrence in typical repositories.
2. Fast feedback loop for schema, runner, and CI policy behavior.

## Step 4: Execute the Required Stage Matrix

For every generated item, execute `T0` through `T4`:

1. `T0` Contract Preflight
2. `T1` Runner Validation
3. `T2` Unit Verification
4. `T3` Integration Verification
5. `T4` Governance Verification

## Step 5: Produce Mandatory Artifacts

1. intent packet input
2. generated item artifacts
3. `intent-to-item.manifest.json`
4. `intent-to-item.report.json`
5. CI gate decision output

## Step 6: Exit Criteria for Initial Success

1. One generated item passes required stages according to declared test requirements.
2. Re-run with unchanged input is idempotent.
3. CI gate decision is deterministic and explainable.
