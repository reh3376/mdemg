# UxXTS01 Intent-to-Item Pipeline Remediation Specification

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Version: 0.1.0-draft
Date: 2026-02-28
Audience: Developers and coding agents implementing or extending UxXTS frameworks.
Status: Draft
Authority: Additive companion to `UxXTS01_PORTABLE_AGENT_SPEC.md`.

---

## 0. End Goal (Normative)

The end goal of this remediation is to enable a developer or coding agent to provide a concise intent statement and programmatically produce a governed UxXTS item that:

1. creates or updates the correct framework artifacts,
2. runs required verification stages (including unit and integration tests where required), and
3. reaches a deterministic CI gate decision with minimal assumptions.

This goal is additive to the core UxXTS philosophy. It does not replace or relax existing schema, runner, integrity, parity, or governance requirements.

---

## 1. Scope and Compatibility

1. This specification defines an explicit intent-to-item pipeline contract for all `U<XX>TS` frameworks.
2. This specification is additive and subordinate to `UxXTS01_PORTABLE_AGENT_SPEC.md`.
3. Any conflict is resolved in favor of the core portable spec.
4. Brownfield rule remains unchanged: extend existing frameworks before creating new frameworks.

---

## 2. Pipeline Contract Overview

The pipeline has five required phases:

1. `Intent Intake`: parse a machine-readable intent packet.
2. `Planning`: resolve extension target, required artifacts, and required stage matrix.
3. `Generation`: create or update framework artifacts deterministically.
4. `Verification`: execute required stages `T0` through `T4`.
5. `Promotion`: apply CI gate policy (`observe`, `soft`, `block`) and emit final decision.

Canonical flow:

`Intent Packet -> Plan -> Artifact Bundle -> Stage Reports -> CI Gate Decision`

---

## 3. Generator Input Contract (`intent_packet`)

The generator must accept one input document (`json` or `yaml`) with at least these required fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `framework_id` | string | yes | Lowercase framework id, e.g. `uats`, `upts`, `uots` |
| `item_id` | string | yes | Stable slug for generated item |
| `intent` | string | yes | Human-readable requested behavior |
| `target` | object | yes | Domain-specific target descriptor |
| `contract` | object | yes | Domain-specific expected behavior assertions |
| `test_requirements` | object | yes | `unit`, `integration`, `smoke` booleans |
| `governance` | object | yes | `maturity_status`, `gate_mode`, `risk_level` |
| `context_refs` | array[string] | no | Paths or identifiers used for grounding |
| `change_policy` | object | no | `extend_existing` and related controls |

Minimum `governance` fields:

1. `maturity_status`: `spec-only` | `pilot` | `active` | `deprecated`
2. `gate_mode`: `observe` | `soft` | `block`
3. `risk_level`: `low` | `medium` | `high` | `critical`

Framework-specific target minimums:

| Framework | Required `target` fields |
|---|---|
| `uats` | `method`, `path` |
| `upts` | `language`, `fixture_path`, `parser_entrypoint` |
| `uots` | `artifact_type`, `scope` |

Framework-specific contract minimums:

| Framework | Required `contract` fields |
|---|---|
| `uats` | `expected_status`, `assertions` |
| `upts` | `expected.symbol_count`, `expected.symbols` |
| `uots` | `assertions` |

---

## 4. Artifact Output Contract

The generator must emit a deterministic artifact bundle with these required outputs:

1. Framework spec artifact (`*.u?ts.json`) that validates against framework schema.
2. Fixture artifacts or fixture references required by the new item.
3. Integrity updates (spec hash and fixture hash where applicable).
4. Required test artifacts (created or updated):
   `unit` tests when `test_requirements.unit=true`;
   `integration` tests when `test_requirements.integration=true`.
5. CI wiring updates if the new item would otherwise be excluded from configured pipeline coverage.
6. Intent-to-item manifest:
   `.uxxts/generated/<framework_id>/<item_id>/intent-to-item.manifest.json`.
7. Intent-to-item stage report:
   `.uxxts/generated/<framework_id>/<item_id>/intent-to-item.report.json`.

Required manifest fields:

| Field | Type |
|---|---|
| `framework_id` | string |
| `item_id` | string |
| `intent_packet_path` | string |
| `created_files` | array[string] |
| `updated_files` | array[string] |
| `deleted_files` | array[string] |
| `stage_matrix` | object |
| `timestamp` | string (date-time) |

---

## 5. Required Verification Stages

Every generated item must execute these stages in order:

| Stage | Name | Minimum requirement | Hard-fail conditions |
|---|---|---|---|
| `T0` | Contract Preflight | Validate spec schema, parity, and integrity field placement | schema invalid, parity failure, malformed hash field |
| `T1` | Runner Validation | Execute framework runner for generated item | runner error, 0/0 false pass, report contract violation |
| `T2` | Unit Verification | Run required unit tests when `test_requirements.unit=true` | failing unit tests, missing required unit evidence |
| `T3` | Integration Verification | Run required integration tests when `test_requirements.integration=true` | failing integration tests, missing required integration evidence |
| `T4` | Governance Verification | Run hash verify, drift/parity checks, report schema validation | unresolved drift, parity regression, invalid report schema |

Stage result statuses:

1. `pass`
2. `fail`
3. `not_applicable` (allowed only when requirement flag is false)

---

## 6. CI Gate Contract

Safety gates are always blocking in every mode:

1. Missing manifest or stage report.
2. Schema invalid output.
3. Parity failure.
4. Runner crash or non-categorized execution error.
5. Invalid canonical report structure.

Mode-specific gate behavior:

| Gate mode | Blocking conditions | Non-blocking conditions |
|---|---|---|
| `observe` | safety gates only | assertion failures and hash mismatches (must still be reported) |
| `soft` | safety gates + `status=error` outcomes | `status=fail` and hash mismatches (must be visible in CI summary) |
| `block` | safety gates + any `status=fail` or `status=error` + hash mismatches on touched artifacts + missing required stages | none |

---

## 7. Determinism and Idempotency Requirements

1. Re-running generation with unchanged `intent_packet` must be idempotent.
2. Hidden mutations are prohibited. Every change must appear in manifest.
3. Generator must prefer extension of existing framework assets before creating new top-level constructs.
4. If generation cannot complete a required stage, generator must emit a machine-readable waiver artifact with explicit owner and expiration.

Waiver path convention:

`.uxxts/generated/<framework_id>/<item_id>/intent-to-item.waiver.json`

---

## 8. UATS Example (Intent to Endpoint Item)

Example `intent_packet` (minimal):

```json
{
  "framework_id": "uats",
  "item_id": "conversation-resume-safe",
  "intent": "Add resume endpoint contract with anomaly-safe behavior and regression coverage.",
  "target": {
    "method": "POST",
    "path": "/v1/conversation/resume"
  },
  "contract": {
    "expected_status": 200,
    "assertions": [
      { "path": "$.ok", "op": "equals", "expected": true },
      { "path": "$.data.resume_id", "op": "exists" }
    ]
  },
  "test_requirements": {
    "unit": true,
    "integration": true,
    "smoke": true
  },
  "governance": {
    "maturity_status": "active",
    "gate_mode": "soft",
    "risk_level": "high"
  }
}
```

Expected output bundle includes:

1. `docs/api/api-spec/uats/specs/conversation_resume_safe.uats.json`
2. Required fixture updates under framework fixture conventions.
3. Unit test artifact changes for endpoint logic.
4. Integration test artifact changes for endpoint behavior.
5. Intent-to-item manifest and stage report under `.uxxts/generated/uats/conversation-resume-safe/`.

---

## 9. Priority Remediation Plan for Current Repository

Priority order for enabling this contract in the current codebase:

`P0`

1. Align assertion dialect across portable spec, common schema, conformance suite, and active framework runners.
2. Enforce full schema validation in UATS runner before execution (not only required-field checks).
3. Resolve or formally de-scope parity-gapped schema features (for example `setup`, `teardown`, `chain`, `body_file`, `body_schema`, OAuth2).
4. Implement `${VAR:-default}` environment token support where required by spec.
5. Align tag filtering behavior with canonical tag locations.
6. Remove documentation drift in executable examples and sample report contracts.

`P1`

1. Add item-level generator command surface (intent packet input, deterministic artifact output).
2. Emit required manifest and stage report artifacts for every generated item.
3. Add an orchestrated stage runner for `T0` to `T4`.

`P2`

1. Add framework-specific item templates for high-frequency constructs (for example UATS endpoints, UOTS observability checks).
2. Add optional auto-remediation suggestions when stage failures are deterministic.

---

## 10. Acceptance Criteria for This Remediation

This remediation is complete when:

1. End goal statement is present and unchanged in this document.
2. Intent packet contract is machine-checkable and implemented by generator tooling.
3. Artifact output contract is enforced with manifest emission.
4. Required stage matrix `T0` through `T4` is executed and reported.
5. CI gate behavior is deterministic for `observe`, `soft`, and `block`.
6. Brownfield extension-first behavior is preserved.
7. Core UxXTS philosophy and boundaries remain unchanged.

