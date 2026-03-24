# Framework Governance (UxTS)

Purpose: govern UxTS test and verification frameworks consistently across API contracts, parser conformance, observability, security, auth, benchmarks, and hash integrity.

---

## Canonical Matrix

Use this governance file as policy, `docs/development/UXTS_FRAMEWORK_MATRIX.md` as the operational inventory with schema/spec/runner/CI mappings, and [`docs/guides/UXTS_DEVELOPER_GUIDE.md`](../guides/UXTS_DEVELOPER_GUIDE.md) as the comprehensive developer reference.

---

## Framework Summary

| Acronym | Name | Primary Use | Current State |
| ------- | ---- | ----------- | ------------- |
| UNTS | Universal Hash Test Specification | Hash verification registry, verify-now, revert | active |
| UDTS | Universal DevSpace Test Specification | gRPC contract and integration tests | active |
| UATS | Universal API Test Specification | HTTP acceptance contract tests | active (124 specs, CI-gated) |
| UPTS | Universal Parser Test Specification | Parser contract conformance across languages | active (27 specs, CI-gated) |
| UBTS | Universal Benchmark Test Specification | Throughput/latency/load regression testing | active (CI smoke, soft-fail) |
| USTS | Universal Security Test Specification | Security behavior and hardening checks | pilot |
| UAMS | Universal Auth Method Specification | Auth method contracts and conformance | spec-only (no runner/fixtures) |
| UOBS | Universal Observability Specification | Runtime observability behavior checks | active |
| UOTS | Universal Observability Test Specification | Artifact-level observability contracts | active |
| UVTS | Universal Validation Test Specification | Semantic retrieval quality validation | pilot (setup-only runner) |
| UETS | Universal Emergence Test Specification | LLM emergence concept-naming quality | active (E1-E5 all enforced) |
| UITS | Universal Iterative-Improvement Test Specification | T1 encoding comprehension validation | active (11 specs, soft-fail CI) |

---

## Framework Policy

1. Every active framework must define and maintain:
   - schema location
   - spec location
   - runnable harness/runner
   - execution path in local commands and CI
2. Any new phase that changes behavior in scope of a framework must include:
   - spec updates
   - runner/harness validation
   - documentation updates in `AGENT_HANDOFF.md`
3. Hash-protected artifacts should be discoverable by UNTS with explicit source references.
4. Overlapping frameworks must be converged or deprecated with migration notes (applies to UOBS/UOTS).
5. **Schema-runner parity is mandatory for promotion to `active` status.** Every field defined in a framework's schema must be either:
   - Enforced by the runner (used in pass/fail logic), OR
   - Detected as unimplemented with a hard fail (not silent ignore, not SKIP/WARN)
6. **Promotion criteria** (pilot → active): schema, specs, runner with full parity, CI gate (at minimum soft-fail), documented authority scope.

---

## Per-Framework Governance

### UNTS — Hash Verification

- Scope: hash integrity for framework-managed artifacts and registry-based verify/revert workflows.
- Policy: maintain history (last 3), auditability, and source reference for each tracked hash.
- References:
  - `docs/specs/unts-hash-verification.md`
  - `docs/specs/unts-registry.json`
  - `internal/unts/`
  - `api/proto/unts.proto`

### UDTS — gRPC Contracts

- Scope: all gRPC API contracts and integration compatibility.
- Policy: each gRPC method must have at least one UDTS spec; proto hash verification should be enforced where applicable.
- References:
  - `docs/api/api-spec/udts/README.md`
  - `docs/api/api-spec/udts/schema/udts.schema.json`
  - `docs/api/api-spec/udts/specs/`
  - `tests/udts/contract_test.go`

### UATS — HTTP Contracts

- Scope: all public HTTP endpoints and expected request/response behavior.
- Policy: merge-impacting endpoint changes must include UATS updates and hash validation.
- References:
  - `docs/api/api-spec/uats/README.md`
  - `docs/api/api-spec/uats/schema/uats.schema.json`
  - `docs/api/api-spec/uats/specs/`
  - `docs/api/api-spec/uats/runners/uats_runner.py`

### UPTS — Parser Contracts

- Scope: symbol extraction and parser conformance across language parsers.
- Policy: parser or grammar changes must update UPTS specs/fixtures and pass harness checks.
- References:
  - `docs/lang-parser/lang-parse-spec/upts/README.md`
  - `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json`
  - `docs/lang-parser/lang-parse-spec/upts/specs/`
  - `cmd/ingest-codebase/languages/upts_test.go`

### UBTS — Benchmark

- Scope: performance SLO and regression validation.
- Status: **active** — promoted from pilot after CI smoke gate and profile assertion enforcement added.
- Policy: p99 degradation uses the spec's `p99_ms` threshold as fixed baseline (deterministic, no ambiguity from "previous run" comparisons).
- References:
  - `docs/tests/ubts/README.md`
  - `docs/tests/ubts/schema/ubts.schema.json`
  - `docs/tests/ubts/specs/`
  - `docs/tests/ubts/runners/ubts_runner.py`

### USTS — Security

- Scope: auth boundaries, injection resilience, rate limiting, and sensitive-data handling.
- Policy: critical/high failures block release once CI gating is enabled.
- References:
  - `docs/tests/usts/README.md`
  - `docs/tests/usts/schema/usts.schema.json`
  - `docs/tests/usts/specs/`
  - `docs/tests/usts/runners/usts_runner.py`

### UAMS — Auth Method Contracts

- Scope: auth method spec contracts and method conformance tests.
- Status: **spec-only** — schema and 4 specs exist, no runner or fixtures implemented.
- Promotion criteria: requires Go test runner (`uams_runner.go`), credential fixtures, and CI gate.
- References:
  - `docs/tests/uams/README.md`
  - `docs/tests/uams/schema/uams.schema.json`
  - `docs/tests/uams/specs/`

### UOBS — Runtime Observability Behavior

- Scope: **runtime** service observability behavior checks.
- Authority: health endpoints, dependency probes, runtime metric endpoint availability, tracing/logging runtime behavior.
- Policy: UOBS validates live service behavior at test time (active probes against running services).
- References:
  - `docs/tests/uobs/README.md`
  - `docs/tests/uobs/runners/uobs_runner.py`
  - `docs/tests/uobs/specs/`

### UOTS — Artifact-Level Observability Contracts

- Scope: **artifact-level** observability contracts and configuration validation.
- Authority: Prometheus metric contract sets, Grafana dashboard JSON structure, alert rule YAML validation.
- Policy: UOTS validates static artifacts (files, exported configs) against schema; does not require a running service.
- References:
  - `docs/api/api-spec/uots/README.md`
  - `docs/api/api-spec/uots/runners/uots_runner.py`
  - `docs/api/api-spec/uots/specs/`

### UOBS/UOTS Authority Split

| Dimension | UOBS | UOTS |
|-----------|------|------|
| **What it validates** | Live runtime behavior | Static artifact structure |
| **Requires running server** | Yes | No (except prometheus_metrics) |
| **Examples** | Health probes, dependency checks, tracing headers | Dashboard JSON, alert rule YAML, metric definitions |
| **Failure mode** | Service behavior deviates from spec | Artifact structure/content invalid |

### UITS — Iterative-Improvement Encoding

- Scope: T1-encoded content comprehension validation via LLM-judged iterative testing.
- Status: **active** — schema, 11 specs, runner with full parity, soft-fail CI gate.
- Policy: LLM-dependent, non-deterministic. Convergence requires 3 consecutive runs with mean ≥9.0/10 and 0 WEAK questions (<7.0). Scoring uses versioned profiles (comprehension, compaction, token_efficiency, fidelity weights).
- References:
  - `docs/tests/uits/README.md`
  - `docs/tests/uits/schema/uits.schema.json`
  - `docs/tests/uits/specs/`
  - `docs/tests/uits/runners/uits_runner.py`

### UVTS — Semantic Validation

- Scope: retrieval and answer quality validation for memory-assisted workflows.
- Policy: remains spec-only until runner/spec-set/automation exist; target activation under the UxTS hardening plan.
- References:
  - `docs/tests/uvts/schema/uvts.schema.json`

### UETS — Emergence Quality

- Scope: evaluating LLM-driven concept naming quality for dynamic emergence (Phase 103).
- Policy: each candidate model must have a UETS spec; thresholds define minimum quality for production use.
- References:
  - `docs/tests/uets/README.md`
  - `docs/tests/uets/schema/uets.schema.json`
  - `docs/tests/uets/specs/`
  - `docs/tests/uets/runners/uets_runner.py`

---

## Phase Alignment

- Phase 81: governance reconciliation
- Phase 82: observability convergence (UOBS/UOTS)
- Phase 83: orchestration and CI expansion
- Phase 84: UNTS full-framework coverage
- Phase 85: auth/security/performance conformance stabilization
- Phase 86: UVTS activation
