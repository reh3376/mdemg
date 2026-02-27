# UxTS Framework Gap Assessment (2026-02-26)

## Scope Reviewed
- Governance and inventory:
  - `docs/specs/FRAMEWORK_GOVERNANCE.md`
  - `docs/development/UXTS_FRAMEWORK_MATRIX.md`
- Core frameworks:
  - UPTS (`docs/lang-parser/lang-parse-spec/upts/` + `UPTS_README.md` + parser harness)
  - UATS (`docs/api/api-spec/uats/` + runner + schema)
- Extended UxTS frameworks:
  - UDTS, UOTS, UOBS, UBTS, USTS, UAMS, UETS, UVTS
- Hash framework:
  - UNTS (`docs/specs/unts-hash-verification.md`, `internal/unts/`, API handlers)

## Remediation Progress (2026-02-26)
- Priority 1 completed: UPTS schema parity fixed and drift checker added.
  - New check: `scripts/verify_upts_schema_parity.py`
  - Wired into local/CI parser path (`Makefile`, `.github/workflows/parser-tests.yml`)
- Priority 2 in progress: dialect separation started.
  - Non-canonical UDTS scenario specs moved to `docs/api/api-spec/udts/drafts/`
  - Non-canonical UVTS draft moved to `docs/tests/uvts/drafts/`
  - UVTS runner now fails cleanly with explicit error for legacy draft shape (no traceback crash)
  - Added canonical dialect guard: `scripts/verify_uxts_canonical_specs.py`
  - Added CI workflow for canonical UDTS/UVTS split: `.github/workflows/uxts-canonical-specs.yml`
- High-risk false-pass remediated:
  - UOBS logging test path now fails explicitly until implemented (no more `0/0` PASS behavior)
- Priority 3 started: observability framework convergence hardening.
  - Added runnable UOTS harness: `docs/api/api-spec/uots/runners/uots_runner.py`
  - Added explicit UOBS/UOTS authority split in READMEs to reduce ownership ambiguity.
  - Fixed unstable UOTS metric bound (`mdemg_neo4j_container_cpu_percent` no longer hard-capped at 100).
- Priority 4 started: UATS schema-runner alignment hardening.
  - Implemented support for active spec fields: `config.response_time_max_ms`, `config.follow_redirects`, and `variants[].variables`.
  - Added fail-fast detection for known unimplemented schema fields (e.g., `setup`, `teardown`, `chain`, `request.body_file`, `expected.body_schema`, OAuth2 mode) to prevent silent false-passes.
- Portable agent spec refinement:
  - Added fixture-governance requirements to `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` so fixture integrity/lifecycle is first-class in cross-repo implementations.

## Why This Matters (UPTS + UATS First)

### UPTS importance
UPTS makes parser validation uniform across languages. It converts parser quality from subjective/manual checks into reproducible contract tests, which is foundational for reliable ingestion and graph construction.

### UATS importance
UATS makes API behavior explicit, testable, and regression-resistant through declarative request/response contracts, variants, and hash-protected specs. For a fast-moving service, this is the main guardrail against silent behavior drift.

### Shared goals across UxTS
- Declarative test contracts per concern domain (parsing, API, security, observability, benchmarking, etc.)
- Reproducible, automatable validation
- CI-gated regression detection
- Hash integrity + provenance
- Portable conventions so new capabilities can be added without inventing one-off test systems

## Developer Advantages When Implemented Correctly
- Faster onboarding: each concern has one schema/spec/runner path.
- Lower regressions: changes are validated against explicit contracts.
- Better change review: diffs in specs show behavioral intent.
- Cleaner automation: CI can gate by framework and severity.
- Safer scale-out: new frameworks can reuse the same governance pattern.

## Findings (Ordered by Severity)

### Critical
1. **UPTS schema is out of sync with active UPTS specs.**
- Evidence:
  - 9 active UPTS spec languages are not allowed by schema enum (e.g., `terraform`, `protobuf`, `graphql`, `openapi`, `csharp`, `kotlin`, `lua`, `makefile`, `scraper-markdown`).
  - Active symbol types and pattern codes in specs are also outside schema enums.
- Risk:
  - Any strict schema-based tooling or external adopter will reject real production specs.
  - “Schema as source of truth” is currently false.

2. **UDTS currently has incompatible spec dialects under one extension.**
- Evidence:
  - 4/11 `.udts.json` specs do not contain required schema fields (`service`, `method`, `expected`), using alternate `api` + `test_cases` formats instead.
- Risk:
  - Contract coverage appears broader than executable coverage.
  - Runner/harness fragmentation; tests can silently miss entire spec subsets.

3. **UVTS has split formats and runner crash path.**
- Evidence:
  - `dynamic_emergence_quality.phase103.uvts.json` does not match UVTS schema-required structure and crashes the current runner (`KeyError: 'validation'`).
- Risk:
  - Broken validation pipeline and false confidence in semantic quality framework maturity.

### High
4. **UOBS logging checks are effectively non-validating and still PASS.**
- Evidence:
  - Logging tests return `PASS` with `0/0` checks and no live validation requirement.
- Risk:
  - False-positive observability compliance, especially dangerous for incident readiness.

5. **UATS runner does not enforce many schema-defined capabilities.**
- Evidence:
  - `setup`, `teardown`, `chain`, `body_schema`, strict body/header controls, retry policies, and step types are defined in schema but not executed by runner.
- Risk:
  - Specs communicate guarantees that runtime validation does not actually enforce.

6. **UPTS runner/harness does not enforce several declared contract controls.**
- Evidence:
  - `require_all_symbols`, `allow_extra_symbols`, relationships, and pattern coverage are not hard-enforced in runner pass/fail logic.
- Risk:
  - Parser quality gates can pass while violating intended strictness.

7. **UOBS/UOTS overlap remains unresolved; UOTS has no runner.**
- Evidence:
  - UOTS has schema/specs but no runnable harness in repo.
  - UOBS has runner/specs with different model and semantics.
- Risk:
  - Split governance and uncertain source of truth for observability contracts.

### Medium
8. **UNTS documentation and implementation state are inconsistent.**
- Evidence:
  - Spec doc still marks UNTS as “implementation not started,” while `internal/unts` + API handlers exist and are used.
- Risk:
  - Operational confusion and incorrect planning assumptions.

9. **UNTS scanner coverage is partial vs stated framework ambition.**
- Evidence:
  - Scanner currently registers from manifest + UDTS only; no automatic scan for UATS/UPTS/UBTS/USTS/UOTS/UAMS/UETS/UVTS hash fields.
- Risk:
  - Integrity framework gives incomplete protection while appearing global.

10. **UAMS fixture/runner packaging is incomplete.**
- Evidence:
  - `docs/tests/uams/fixtures/` and `docs/tests/uams/runners/` are empty while specs reference fixtures.
- Risk:
  - Contract-to-fixture traceability and reproducible method conformance are weakened.

11. **UETS documentation claims E4 (description quality), runner does not score it.**
- Evidence:
  - README/pattern matrix includes E4, but validator enforces E1/E2/E3/E5 only.
- Risk:
  - Model quality reported as pass while one documented quality dimension is unvalidated.

12. **Documentation drift in counts/status is widespread (especially UATS).**
- Evidence:
  - UATS README variants still describe ~45 specs while active set is 124 specs.
  - Archive duplicates contain older parallel narratives.
- Risk:
  - New contributors and coding agents may follow stale pathways and incorrect assumptions.

## Planning Phase 1: Deep-Dive Vulnerability/Gaps Program

### Objective
Convert current findings into a systematic, evidence-backed hardening program across all UxTS families.

### Workstreams
1. **Schema-Conformance Audit**
- Build per-framework conformance checks: schema vs active specs vs runner-supported fields.
- Output: machine-readable mismatch report and severity tags.

2. **Runner Capability Audit**
- Enumerate every schema field and classify as: implemented, ignored, partially implemented.
- Output: feature coverage matrix (`field -> runner behavior`).

3. **False-Pass / False-Fail Risk Tests**
- Add adversarial specs to prove no-op checks, unsupported assertions, and crash paths.
- Output: regression tests preventing recurrence.

4. **Hash Integrity Expansion Audit**
- Map every hash-bearing framework artifact and compare to UNTS scanner coverage.
- Output: prioritized backlog for full scanner parity.

5. **Governance/CI Enforcement Audit**
- Classify frameworks by CI gate status, block policy, and release impact.
- Output: staged CI gating plan by maturity/risk.

### Priority Order
1. UPTS schema parity
2. UDTS/UVTS dialect consolidation
3. UOBS/UOTS canonicalization
4. UATS schema-runner alignment
5. UNTS full-framework scan coverage
6. UAMS/UETS completion gaps

## Planning Phase 2: Portable Coding-Agent Specification Program

### Objective
Produce a portable UxTS implementation specification that allows a coding agent to stand up this governance model in any codebase.

### Required Sections for the Portable Spec
1. **Framework Taxonomy**
- Core vs optional frameworks
- Canonical responsibilities and boundaries (avoid overlap)

2. **Contract Model Template (Per Framework)**
- Required: schema path, spec path, runner/harness path, CI binding, hash strategy, ownership

3. **Lifecycle and Change Rules**
- Rules for introducing, evolving, deprecating frameworks
- Mandatory update checklist when behavior changes

4. **Schema-Runner Compatibility Contract**
- Policy: no schema field without runner behavior classification
- Compatibility levels: strict, advisory, deprecated

5. **Hash Integrity Model**
- Standard hash field convention
- Registry schema
- Scanner architecture and coverage guarantees

6. **CI Governance Modes**
- Blocker, soft-fail, monitor-only modes
- Promotion criteria between modes

7. **Threat Model by Framework Type**
- API, parser, security, observability, performance, semantic quality
- False-pass and blind-spot risks

8. **Agent Operating Procedure**
- Discovery algorithm in new repo
- Bootstrapping steps
- Ongoing maintenance loop
- Required artifacts for handoff

9. **Reference Deliverables**
- `framework_matrix.json`
- `schema_runner_coverage.json`
- `hash_coverage_report.json`
- `ci_gating_manifest.json`

### Output Target
Create a canonical document that a coding agent can execute against as an implementation playbook, not just conceptual guidance.

## Immediate Recommendations
1. Freeze nonconforming schema changes until parity checks are added.
2. Consolidate dual-dialect specs (UDTS, UVTS) into single canonical formats with migration notes.
3. Treat no-op validations as failures in observability/security classes.
4. Add automated drift checks: spec count, schema conformance, runner feature coverage.
5. Establish one observability framework authority (UOBS or UOTS) and formally deprecate/migrate the other.
