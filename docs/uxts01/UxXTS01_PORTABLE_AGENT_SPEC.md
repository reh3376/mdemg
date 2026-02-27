# UxXTS Portable Agent Specification (UxXTS01)

Version: 3.3.0-uxxts01
Date: 2026-02-27
Audience: Developers and coding agents implementing UxXTS governance in arbitrary codebases.
Status: Consolidated best-in-class synthesis of v3.2.0 and the final v3.1 bundle.

---

## 0. Purpose

UxXTS defines a reusable governance model for declarative verification frameworks.

It standardizes:

1. Schema-driven spec contracts.
2. Runner obligations and parity behavior.
3. Integrity and drift controls.
4. Machine-readable reporting and governance gates.

It does not replace unit tests, exploratory testing, OpenAPI, Pact, or imperative workflow test frameworks.

---

## 1. Core Problem

Without a unifying model, verification systems drift into fragmented tooling with:

1. Silent drift between expected behavior and executable checks.
2. False confidence from skipped or unimplemented assertions.
3. Reinvention overhead when adding new verification domains.

UxXTS addresses this by enforcing one pattern across domains:

`Schema -> Spec -> Runner -> CI Gate`

---

## 2. Core Model

### 2.1 Four-Layer Architecture

Every UxXTS framework has exactly four layers:

1. `Schema`: authoritative structure.
2. `Specs`: declarative contracts.
3. `Runner`: executable verifier.
4. `CI Gate`: policy for enforcement.

### 2.2 Domain Isolation Rule

One framework owns one concern domain. Overlap requires explicit authority split or deprecation/migration.

### 2.3 Naming Grammar (`U<XX>TS`)

To preserve standardization while supporting specificity, framework acronyms use:

`U[A-Z0-9]{1,2}TS`

Rules:

1. `U` is always the first character.
2. `TS` are always the final two characters.
3. Domain token length is 1 or 2 alphanumeric characters.
4. Existing one-token names remain fully valid (`UATS`, `UPTS`, `UBTS`).
5. Two-token names are allowed for subdomain clarity (`ULPTS`, `UOBS`, `UAMS`).

Examples:

1. `UPTS`: Universal Parser Test Specification.
2. `ULPTS`: Universal Language Parser Test Specification.
3. `UATS`: Universal API Test Specification.

---

## 3. Normative Principles

1. Specs are data, not code.
2. Schema-runner parity is mandatory for active frameworks.
3. Hash integrity and assertion results are independent signals.
4. Unimplemented required behavior must hard-fail, never silently skip.
5. Extend existing frameworks before creating new ones in brownfield repos.
6. CI gate mode and framework maturity status are independent axes.

---

## 4. Spec Contract Requirements

### 4.1 Required Building Blocks

Each framework spec schema should include:

1. Version field (e.g., `uats_version`).
2. Metadata block (identity, tags, status).
3. Domain-specific input/request configuration.
4. Domain-specific expectations/assertions.

Recommended cross-framework blocks:

1. `metadata`
2. `integrity`
3. `execution`

### 4.2 Hash Integrity

Hash behavior is normative:

1. Exclude the hash field itself from hash input.
2. Spec hash uses canonical JSON bytes.
3. Fixture hash uses raw file bytes.
4. Runner always verifies hash when declared.
5. Runner always executes assertions regardless of hash result.
6. Reports must separate `status` from `hash_verified`.

### 4.3 Assertion Grammar

Canonical assertion object:

```json
{
  "path": "$.status",
  "op": "equals",
  "expected": "ok"
}
```

Portable operator baseline:

1. `equals`, `not_equals`
2. `contains`, `not_contains`
3. `matches`
4. `type_is`
5. `greater_than`, `less_than`, `greater_than_or_equal`, `less_than_or_equal`
6. `exists`, `not_exists`
7. `one_of`
8. `length`, `length_greater_than`, `length_less_than`
9. `items_all_match`
10. `schema_match`

Rule: unsupported operators must produce parity failure.

### 4.4 Environment Resolution

Supported token forms:

1. `${VAR_NAME}`
2. `${VAR_NAME:-default_value}`

Resolution order:

1. CLI override
2. Environment variable
3. Inline default
4. Hard failure

Unresolved tokens must error; literal `${VAR}` pass-through is prohibited.

### 4.5 Secrets

Allowed secret references:

1. `${VAR_NAME}` from environment.
2. `${SECRET:path}` from configured secret backend.

If `${SECRET:path}` cannot be resolved, runner must hard-fail with `DEPENDENCY:` category.

### 4.6 Defaults and Composition

`_defaults.json` is optional and merged before execution.

Merge semantics:

1. Objects merge recursively.
2. Scalars override defaults.
3. Arrays replace defaults (RFC7396 semantics), except where explicitly defined otherwise by the framework.

### 4.7 Variants

Specs may include `variants` for parameterized scenarios.

Rules:

1. Each variant requires `name`.
2. Variant merged with base spec using defaults merge semantics.
3. Each variant yields separate report entry.
4. 0/0 prohibition applies per variant.
5. Runner without variant support must parity-fail.

### 4.8 Setup/Teardown Hooks

Bounded lifecycle hooks are allowed:

1. `setup[]` runs before main verification.
2. setup failure yields `status=error` with `SETUP:` category.
3. `teardown[]` runs after execution regardless of pass/fail.
4. Hooks are declarative and bounded; imperative orchestration remains out of scope.
5. Runner without hook support must parity-fail.

### 4.9 Human Intent Layer

`metadata.intent` is allowed for stakeholder readability.

Rules:

1. Advisory only.
2. Must not affect execution result.
3. Reporting tools should support intent-only summaries.

---

## 5. Runner Contract

A UxXTS-compliant runner must:

1. Discover specs (glob or explicit path).
2. Validate schema before execution.
3. Resolve defaults and environment/secret references.
4. Verify hash integrity when declared.
5. Execute assertions/contracts.
6. Emit canonical report JSON that validates against report schema.

Recommended capability output:

`--capabilities` returns supported versions, operators, and optional features.

---

## 6. Report Contract

Runner output must validate against:

1. `schemas/uxxts-report.schema.json`

Aggregate governance output (full governance mode) must validate against:

1. `schemas/uxxts-report-aggregate.schema.json`

Status semantics:

1. `pass`: assertions passed and assertions_evaluated >= 1.
2. `fail`: assertion or parity failure.
3. `skip`: intentionally not executed (tag filters, archived, dependency skip).
4. `error`: execution failed before verification completion.

Integrity semantics:

1. `hash_verified=true`: hash matches.
2. `hash_verified=false`: hash mismatch.
3. `hash_verified=null`: no declared hash.

---

## 7. Schema-Runner Parity

Every schema field used by active frameworks is classified as:

1. `enforced`
2. `advisory`
3. `unimplemented` (must hard-fail if encountered)

Silent ignore of unknown required behavior is prohibited.

---

## 8. Execution Semantics

### 8.1 Parallelism

Default assumption: independent specs should be parallelizable.

### 8.2 Ordering

Per-spec ordering via `depends_on` is allowed.

Rules:

1. Graph must be acyclic.
2. Missing dependency target is an error.
3. Failed dependency causes dependent spec skip with explicit dependency reason.

### 8.3 Retries

Normative fields:

1. `retry.max_attempts` (>=1)
2. `retry.backoff_ms` (>=0)
3. `retry.retryable_statuses` (framework-defined defaults allowed)

Assertion failures are non-retryable unless explicitly opt-in.

---

## 9. Discovery and Bootstrap Model

### 9.1 Operating Modes

1. `greenfield`: no existing UxXTS artifacts.
2. `brownfield`: existing verification/codebase constructs.

### 9.2 Brownfield Rules

In brownfield mode:

1. Run deterministic discovery commands.
2. Build scored opportunity backlog.
3. Implement at least one high-priority remediation or produce formal waiver.
4. Prefer extending existing frameworks over creating new ones.

---

## 10. Governance Model

### 10.1 Maturity Status Axis

1. `spec-only`
2. `pilot`
3. `active`
4. `deprecated`

### 10.2 Gate Mode Axis

1. `observe`
2. `soft`
3. `block`

Status and gate mode are independent and must not be conflated.

### 10.3 Governance Levels (Adoption Friction Control)

To avoid over-bureaucracy while preserving rigor, apply progressive governance:

1. `Core`: schema + specs + runner + canonical report + basic CI invocation.
2. `Managed`: add parity tracking, drift checks, hash policy, version compatibility.
3. `Full`: multi-framework inventory, aggregate report, comprehensive governance artifacts.

Projects with 1-2 frameworks should start at Core/Managed and graduate to Full as scope grows.

---

## 11. Anti-Patterns (Must Prevent)

1. 0/0 false pass.
2. Silent schema ignore.
3. Dialect split inside one framework.
4. Documentation-vs-disk drift.
5. Environment leak via hardcoded endpoints.
6. SKIP/WARN false confidence for unimplemented required behavior.
7. Fixture drift without integrity controls.

---

## 12. Threat Model Minimum

Each framework should assess and mitigate:

1. False pass
2. False fail
3. Coverage blind spot
4. Undetected change
5. Fixture drift
6. Secret exposure
7. Report divergence
8. Schema migration gap

---

## 13. Compliance and Conformance

A runner claiming compliance should pass the conformance suite:

1. `conformance/conformance-suite.json`

This suite includes checks for:

1. pass/fail/error/skip semantics
2. 0/0 prohibition
3. parity hard-fail behavior
4. env/default handling
5. integrity independence
6. variants and hooks behavior
7. report schema compliance

---

## 14. Tooling Surface

Reference tooling in this package:

1. `tools/uxxts_lint.py`
2. `tools/uxxts_init.py`

Recommended broader CLI surface:

1. `uxxts validate`
2. `uxxts drift`
3. `uxxts hash`
4. `uxxts report`
5. `uxxts init`
6. `uxxts migrate`
7. `uxxts parity`
8. `uxxts secret-scan`
9. `uxxts mcp-serve`

---

## 15. Acceptance Criteria (Repository-Level)

A repository is UxXTS-governed when:

1. Active frameworks satisfy schema/spec/runner/CI contract.
2. Schema-runner parity is documented and enforced.
3. No silent false-pass path exists.
4. Runner reports validate against report schema.
5. Integrity is reported independently from assertion outcomes.
6. Drift checks are running in CI at least in soft mode.
7. Version compatibility behavior is defined and enforced.
8. Governance level matches framework count/complexity.

---

## 16. Deliberate Boundaries

UxXTS is not:

1. A replacement for imperative unit/integration/E2E frameworks.
2. A replacement for OpenAPI documentation.
3. A replacement for consumer-dependency tracking (Pact/PactFlow).

UxXTS is the contract-governance layer that complements those tools.

---

## 17. Supporting Files in This Package

This document is the normative anchor for this package directory.

Supporting documents and artifacts:

1. `SUPPORTING_FILES.md` (index and usage map)
2. `UxXTS01_ADOPTION_ROADMAP.md` (phased rollout)
3. `UxXTS01_MERGE_DECISIONS.md` (why this synthesis was chosen)
4. `SOURCE_PROVENANCE_MATRIX.md` (traceability)
5. `DECISION_REGISTER.md` (explicit decisions)
6. `WORKING_CONTEXT_LOG.md` (checkpoint log for context durability)
7. `schemas/uxxts-common.schema.json` (portable common blocks)
8. `schemas/uxxts-report.schema.json` (normative report schema)
9. `schemas/uxxts-report-aggregate.schema.json` (normative aggregate schema)
10. `conformance/conformance-suite.json` (conformance tests)
11. `tools/uxxts_lint.py` (lint/integrity utility)
12. `tools/uxxts_init.py` (framework scaffolder)
13. `examples/` (reference example artifacts)
