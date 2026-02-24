# UxTS Framework Matrix

Purpose: canonical map of each UxTS framework to its schema, specs, runner, CI coverage, current status, and known gaps.

Last updated: 2026-02-24

---

## 1) Framework Inventory

| Acronym | Name | Primary Scope | Status |
| ------- | ---- | ------------- | ------ |
| UNTS | Universal Hash Test Specification | Hash integrity registry, verification, revert | active (partial coverage) |
| UDTS | Universal DevSpace Test Specification | gRPC contract tests | active |
| UATS | Universal API Test Specification | HTTP endpoint acceptance contracts | active |
| UPTS | Universal Parser Test Specification | Language parser conformance | active |
| UBTS | Universal Benchmark Test Specification | Throughput/latency/load benchmarking | pilot |
| USTS | Universal Security Test Specification | Security behavior and hardening tests | pilot |
| UAMS | Universal Auth Method Specification | Auth method contracts and conformance | active (docs+tests) |
| UOBS | Universal Observability Specification | Metrics/health/log observability validation | pilot |
| UOTS | Universal Observability Test Specification | Observability contract specs (API-spec track) | pilot (runner gap) |
| UVTS | Universal Validation Test Specification | Semantic retrieval quality benchmarks | spec-only |
| UETS | Universal Emergence Test Specification | LLM emergence concept-naming quality evaluation | active |

---

## 2) Source of Truth by Framework

| Framework | Schema | Specs | Runner / Harness | CI / Automation |
| --------- | ------ | ----- | ---------------- | --------------- |
| UNTS | n/a (registry format in docs) | `docs/specs/unts-hash-verification.md`, `docs/specs/unts-registry.json`, `docs/api/api-spec/udts/specs/unts_hash_verification.udts.json` | `internal/unts/` (Go service + scanner + registry), `tests/udts/contract_test.go` (spec subset) | no dedicated CI gate |
| UDTS | `docs/api/api-spec/udts/schema/udts.schema.json` | `docs/api/api-spec/udts/specs/*.udts.json` | `tests/udts/contract_test.go` | no explicit CI job in `.github/workflows/ci.yml` |
| UATS | `docs/api/api-spec/uats/schema/uats.schema.json` | `docs/api/api-spec/uats/specs/*.uats.json` | `docs/api/api-spec/uats/runners/uats_runner.py` | wired in `.github/workflows/ci.yml` (`continue-on-error`) |
| UPTS | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` | `docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json` | Go harness `cmd/ingest-codebase/languages/upts_test.go`, Python runner | wired in `.github/workflows/parser-tests.yml` |
| UBTS | `docs/tests/ubts/schema/ubts.schema.json` | `docs/tests/ubts/specs/*.ubts.json`, profiles under `docs/tests/ubts/profiles/` | `docs/tests/ubts/runners/ubts_runner.py` | no CI gate |
| USTS | `docs/tests/usts/schema/usts.schema.json` | `docs/tests/usts/specs/*.usts.json` | `docs/tests/usts/runners/usts_runner.py` | no CI gate |
| UAMS | `docs/tests/uams/schema/uams.schema.json` | `docs/tests/uams/specs/*.uams.json` | `internal/auth/uams_test.go` | indirect via `go test`; no dedicated pipeline target |
| UOBS | `docs/tests/uobs/schema/uobs.schema.json` | `docs/tests/uobs/specs/*.uobs.json` | `docs/tests/uobs/runners/uobs_runner.py` | no CI gate |
| UOTS | `docs/api/api-spec/uots/schema/uots.schema.json` | `docs/api/api-spec/uots/specs/*.uots.json` | runner path referenced in README but not present | no CI gate |
| UVTS | `docs/tests/uvts/schema/uvts.schema.json` | no canonical spec set yet | none | none |
| UETS | `docs/tests/uets/schema/uets.schema.json` | `docs/tests/uets/specs/*.uets.json` | `docs/tests/uets/runners/uets_runner.py` | no CI gate |

---

## 3) Main Cross-Framework Gaps

1. Governance docs lag repo state (frameworks and status are stale in older summaries).
2. UOBS and UOTS overlap with divergent schemas and tooling ownership.
3. CI is concentrated on UATS/UPTS; other frameworks are not merge-gated.
4. UNTS scanner coverage currently focuses on manifest and UDTS but not all intended framework artifacts.
5. UAMS specs reference fixture files that are not currently in `docs/tests/uams/fixtures/`.
6. UVTS has schema but no runner/spec/automation path yet.

---

## 4) Target End State (Phase 81-86 Alignment)

| Phase | Outcome |
| ----- | ------- |
| 81 | Governance docs and handoff synchronized to actual framework reality |
| 82 | UOBS/UOTS converged to one canonical observability framework |
| 83 | Unified make/CI orchestration for all active frameworks |
| 84 | UNTS scans and verifies hash artifacts across all framework families |
| 85 | UAMS/USTS/UBTS conformance stabilized with enforceable baselines |
| 86 | UVTS activated as semantic quality gate with trend reporting |

---

## 5) Reference Documents

- `docs/specs/FRAMEWORK_GOVERNANCE.md`
- `docs/specs/unts-hash-verification.md`
- `docs/api/api-spec/uats/README.md`
- `docs/api/api-spec/udts/README.md`
- `docs/lang-parser/lang-parse-spec/upts/README.md`
- `docs/tests/uets/README.md`
- `AGENT_HANDOFF.md` (Governance & Testing Frameworks)
