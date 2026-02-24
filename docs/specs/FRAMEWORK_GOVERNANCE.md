# Framework Governance (UxTS)

Purpose: govern UxTS test and verification frameworks consistently across API contracts, parser conformance, observability, security, auth, benchmarks, and hash integrity.

---

## Canonical Matrix

Use this governance file as policy and `docs/development/UXTS_FRAMEWORK_MATRIX.md` as the operational inventory with schema/spec/runner/CI mappings.

---

## Framework Summary

| Acronym | Name | Primary Use | Current State |
| ------- | ---- | ----------- | ------------- |
| UNTS | Universal Hash Test Specification | Hash verification registry, verify-now, revert | active (coverage expansion pending) |
| UDTS | Universal DevSpace Test Specification | gRPC contract and integration tests | active |
| UATS | Universal API Test Specification | HTTP acceptance contract tests | active |
| UPTS | Universal Parser Test Specification | Parser contract conformance across languages | active |
| UBTS | Universal Benchmark Test Specification | Throughput/latency/load regression testing | pilot |
| USTS | Universal Security Test Specification | Security behavior and hardening checks | pilot |
| UAMS | Universal Auth Method Specification | Auth method contracts and conformance | active (docs + tests) |
| UOBS | Universal Observability Specification | Metrics/health/log observability validation | pilot |
| UOTS | Universal Observability Test Specification | API-spec observability contract track | pilot (runner gap) |
| UVTS | Universal Validation Test Specification | Semantic retrieval quality validation | spec-only |
| UETS | Universal Emergence Test Specification | LLM emergence concept-naming quality | active |

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
- Policy: promote to active only after CI orchestration and baseline threshold governance.
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
- Policy: fixture-backed conformance and registry coverage required for active status.
- References:
  - `docs/tests/uams/README.md`
  - `docs/tests/uams/schema/uams.schema.json`
  - `docs/tests/uams/specs/`
  - `internal/auth/uams_test.go`

### UOBS and UOTS — Observability Governance

- Scope: observability validation (metrics, health, logs, dashboards, alerts).
- Policy: maintain one canonical observability framework; the non-canonical track must be deprecated or migrated.
- References:
  - `docs/tests/uobs/README.md`
  - `docs/api/api-spec/uots/README.md`
  - `docs/development/UXTS_FRAMEWORK_MATRIX.md`

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
