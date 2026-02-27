# UxTS Framework Matrix

Purpose: canonical map of each UxTS framework to its schema, specs, runner, CI coverage, current status, and known gaps.

Last updated: 2026-02-27

---

## 1) Framework Inventory

| Acronym | Name | Primary Scope | Status | Specs |
| ------- | ---- | ------------- | ------ | ----- |
| UNTS | Universal Hash Test Specification | Hash integrity registry, verification, revert | active | N/A (registry) |
| UDTS | Universal DevSpace Test Specification | gRPC contract tests | active | 7 canonical, 4 drafts |
| UATS | Universal API Test Specification | HTTP endpoint acceptance contracts | active | 124 canonical, 7 drafts |
| UPTS | Universal Parser Test Specification | Language parser conformance | active | 27 |
| UBTS | Universal Benchmark Test Specification | Throughput/latency/load benchmarking | active | 3 specs, 3 profiles |
| USTS | Universal Security Test Specification | Security behavior and hardening tests | pilot | 3 canonical, 2 drafts |
| UAMS | Universal Auth Method Specification | Auth method contracts and conformance | spec-only | 4 |
| UOBS | Universal Observability Specification | Runtime observability behavior checks | active | 3 canonical, 1 draft |
| UOTS | Universal Observability Test Specification | Artifact-level observability contracts | active | 5 |
| UVTS | Universal Validation Test Specification | Semantic retrieval quality benchmarks | spec-only | 1 canonical, 1 draft |
| UETS | Universal Emergence Test Specification | LLM emergence concept-naming quality | active | 8 |

---

## 2) Source of Truth by Framework

| Framework | Schema | Specs | Runner / Harness | CI / Automation |
| --------- | ------ | ----- | ---------------- | --------------- |
| UNTS | n/a (registry format in docs) | `docs/specs/unts-hash-verification.md`, `docs/specs/unts-registry.json` | `internal/unts/` (Go gRPC service + scanner + registry) | no dedicated CI gate |
| UDTS | `docs/api/api-spec/udts/schema/udts.schema.json` | Canonical: `docs/api/api-spec/udts/specs/` (7); Drafts: `docs/api/api-spec/udts/drafts/` (4) | `tests/udts/contract_test.go` (hand-coded per RPC) | canonical dialect guard via `uxts-canonical-specs.yml` |
| UATS | `docs/api/api-spec/uats/schema/uats.schema.json` | `docs/api/api-spec/uats/specs/` (124) | `docs/api/api-spec/uats/runners/uats_runner.py` v1.1.0 | CI-gated in `ci.yml` |
| UPTS | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` | `docs/lang-parser/lang-parse-spec/upts/specs/` (27) | `docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py` | CI-gated in `parser-tests.yml` |
| UBTS | `docs/tests/ubts/schema/ubts.schema.json` | `docs/tests/ubts/specs/` (3), profiles under `docs/tests/ubts/profiles/` (3) | `docs/tests/ubts/runners/ubts_runner.py` v1.1.0 | CI smoke gate in `ci.yml` (soft-fail) |
| USTS | `docs/tests/usts/schema/usts.schema.json` | Canonical: `docs/tests/usts/specs/` (3); Drafts: `docs/tests/usts/drafts/` (2) | `docs/tests/usts/runners/usts_runner.py` | no CI gate |
| UAMS | `docs/tests/uams/schema/uams.schema.json` | `docs/tests/uams/specs/` (4) | none (spec-only, no runner/fixtures) | no CI gate |
| UOBS | `docs/tests/uobs/schema/uobs.schema.json` | Canonical: `docs/tests/uobs/specs/` (3); Drafts: `docs/tests/uobs/drafts/` (1) | `docs/tests/uobs/runners/uobs_runner.py` | no CI gate |
| UOTS | `docs/api/api-spec/uots/schema/uots.schema.json` | `docs/api/api-spec/uots/specs/` (5) | `docs/api/api-spec/uots/runners/uots_runner.py` | Makefile target `test-uots`; no CI gate |
| UVTS | `docs/tests/uvts/schema/uvts.schema.json` | Canonical: `docs/tests/uvts/specs/` (1); Drafts: `docs/tests/uvts/drafts/` (1) | none (spec-only; runner stub exists but is setup-only, not functional) | canonical dialect guard via `uxts-canonical-specs.yml` |
| UETS | `docs/tests/uets/schema/uets.schema.json` | `docs/tests/uets/specs/` (8) | `docs/tests/uets/runners/uets_runner.py` | no CI gate |

---

## 3) Schema-Runner Parity Status

| Framework | Schema Fields Enforced | Unimplemented Fields | Fail-Fast Detection |
| --------- | --------------------- | -------------------- | ------------------- |
| UATS | Most fields | `setup`, `teardown`, `chain`, `body_file`, `body_schema`, `oauth2`, several config keys | Yes (hard fail on unimplemented fields) |
| UPTS | `line_tolerance`, `validate_signature`, `validate_value`, `validate_parent`, `require_all_symbols`, `allow_extra_symbols` | `relationships` | Yes (hard fail on `relationships`) |
| UBTS | All threshold fields, `min_success_rate`, `max_p99_degradation_pct` | `setup.seed_data`, `ramp_up_seconds`, `duration_seconds` | Yes (warnings for unimplemented) |
| UETS | E1-E5 all enforced (E4 description quality added) | none | N/A |
| UOBS | `metrics`, `health`, `dependency` | `logging` (draft), `tracing` | Yes (parity hard-fail for unimplemented types) |
| UOTS | `prometheus_metrics`, `grafana_dashboard`, `alert_rules` | `log_format`, `trace_propagation` | Yes (explicit fail for unimplemented types) |
| UDTS | Hand-coded per RPC | N/A (not spec-driven) | N/A |
| UVTS | N/A (spec-only, no functional runner) | All | N/A |
| USTS | `rate_limiting`, `data_exposure`, `injection` | `authentication` (draft, needs USTS_AUTH_ENABLED), `test_cases` format (draft) | Yes (parity hard-fail for auth, test_cases) |
| UAMS | N/A (no runner) | All | N/A |

---

## 4) UOBS / UOTS Authority Split

| Dimension | UOBS | UOTS |
|-----------|------|------|
| **What it validates** | Live runtime behavior | Static artifact structure |
| **Requires running server** | Yes | No (except prometheus_metrics) |
| **Examples** | Health probes, dependency checks, tracing headers | Dashboard JSON, alert rule YAML, metric definitions |

---

## 5) Cross-Framework Gaps (Post-Remediation)

1. ~~Governance docs lag repo state~~ — **Remediated**: All counts and statuses updated.
2. ~~UOBS/UOTS overlap~~ — **Remediated**: Authority split documented (UOBS = runtime, UOTS = artifacts).
3. CI concentrated on UATS/UPTS; UBTS now has soft-fail CI. Other frameworks still lack CI gates.
4. ~~UNTS scanner limited to manifest + UDTS~~ — **Remediated**: Scanner now covers all 8 UxTS frameworks.
5. ~~UAMS claims fixtures/runner that don't exist~~ — **Remediated**: Marked spec-only, phantom claims removed.
6. ~~UVTS runner is setup-only~~ — **Demoted**: UVTS reclassified as spec-only (runner stub non-functional).
7. ~~USTS not audited for schema-runner parity~~ — **Remediated**: Parity checks added. Auth/guardrail specs moved to drafts.

---

## 6) Reference Documents

- `docs/specs/FRAMEWORK_GOVERNANCE.md`
- `docs/specs/unts-hash-verification.md`
- `docs/api/api-spec/uats/README.md`
- `docs/api/api-spec/udts/README.md`
- `docs/lang-parser/lang-parse-spec/upts/README.md`
- `docs/tests/uets/README.md`
- `docs/tests/uobs/README.md`
- `docs/api/api-spec/uots/README.md`
- `docs/tests/uams/README.md`
- `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md`
- `AGENT_HANDOFF.md` (Governance & Testing Frameworks)
