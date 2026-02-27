# UxTS Portable Agent Specification

Version: 2.0.0-draft
Date: 2026-02-27
Audience: Coding agents implementing Universal-x Test Specification governance in arbitrary codebases.

---

## 1. The Problem UxTS Solves

Every non-trivial codebase accumulates recurring verification needs: API endpoints need contract tests, parsers need conformance checks, observability configs need structure validation, benchmarks need regression gates. Without a unifying methodology, each concern spawns its own ad-hoc test system — different formats, different runners, different CI wiring, different standards for what "passing" means.

This creates three compounding failures:

**Silent drift.** When tests are hand-written scripts rather than declarative contracts, behavior changes slip through because the test and the code can diverge without anyone noticing. There is no single artifact that says "this endpoint MUST return this shape" that can be diffed, reviewed, and machine-validated.

**False confidence.** A test suite that reports "all passing" but silently ignores half its assertions is worse than no tests at all. It provides the illusion of coverage. This happens when runners don't enforce every field in the spec, when schemas define capabilities runners don't implement, or when specs are written but never wired into CI. In one real codebase, an observability test framework reported 100% pass rate on specs with zero executable assertions — a `0/0 = pass` false positive.

**Reinvention tax.** When each concern domain invents its own format, runner, and CI approach, adding a new domain (say, security tests) requires solving all the same problems again: how to define specs, how to validate them against a schema, how to run them, how to gate CI, how to manage integrity. The overhead compounds with every new concern.

UxTS solves all three by defining a single pattern — **declarative specs validated by executable runners governed by explicit schemas** — that applies uniformly across every concern domain. Each domain gets its own framework (UATS for APIs, UPTS for parsers, UBTS for benchmarks, etc.), but all frameworks share the same structural contract, lifecycle, governance rules, and CI integration pattern.

---

## 2. What UxTS IS

UxTS (Universal-x Test Specification) is a methodology for organizing programmatic verification into domain-specific frameworks that share a common architecture. The "x" is a wildcard — each framework replaces it with a letter representing its concern domain.

### 2.1 Core Concepts

**Spec** — A declarative JSON file that defines a single verifiable contract. A spec says "given this input, expect this output" or "this artifact must have this structure." Specs are data, not code. They can be diffed, reviewed, generated, and machine-validated.

**Schema** — A JSON Schema that defines the valid structure of specs within a framework. The schema is the canonical definition of what fields exist, what they mean, and what values are legal. Every spec must validate against its framework's schema.

**Runner** — An executable program (script or binary) that reads specs, executes the verification they describe, and reports pass/fail results. The runner is the only component that actually "does" anything — specs and schemas are pure data.

**Fixture** — A static input file that a spec references for testing. For example, a parser spec might reference a `.go` source file as its fixture. Fixtures have their own integrity controls (hashes) because a tampered fixture can silently invalidate test results.

**Framework** — The combination of schema + specs + runner + CI wiring for one concern domain. Each framework owns exactly one domain. Framework overlap is prohibited — when two frameworks claim the same domain, one must be deprecated or the domains must be split.

### 2.2 The Four-Layer Architecture

Every UxTS framework has exactly four layers:

```
┌──────────────────────────────────────┐
│  CI Gate                             │  Automation: when/how tests run
│  (workflow, Makefile target)         │
├──────────────────────────────────────┤
│  Runner                              │  Execution: reads specs, reports results
│  (Python script, Go binary, etc.)   │
├──────────────────────────────────────┤
│  Specs                               │  Contracts: declarative test definitions
│  (*.u?ts.json files)                │
├──────────────────────────────────────┤
│  Schema                              │  Authority: defines valid spec structure
│  (*.schema.json)                    │
└──────────────────────────────────────┘
```

Data flows upward: the schema constrains what specs can say, the runner interprets what specs say, and CI decides when the runner executes. Each layer is independently versionable and auditable.

### 2.3 The Naming Convention

Framework names follow the pattern `U<X>TS` where `<X>` identifies the domain:

| Letter | Domain | Example |
|--------|--------|---------|
| A | API contracts | UATS — HTTP endpoint acceptance tests |
| P | Parser conformance | UPTS — language parser symbol extraction |
| B | Benchmarks | UBTS — throughput/latency/load regression |
| S | Security | USTS — auth boundaries, injection resilience |
| O | Observability (artifacts) | UOTS — metric definitions, dashboard structure |
| OB | Observability (runtime) | UOBS — health probes, dependency checks |
| D | DevSpace / gRPC | UDTS — gRPC contract tests |
| V | Validation quality | UVTS — semantic retrieval quality |
| E | Emergence quality | UETS — LLM concept-naming quality |
| AM | Auth methods | UAMS — authentication method contracts |
| N | Hash integrity | UNTS — hash verification registry |

You are not limited to these. Any recurring verification pattern in your codebase can become a `U<X>TS` framework. The methodology is the pattern, not the specific letters.

---

## 3. A Worked Example: Building a Framework from Zero

This section walks through creating a minimal UxTS framework for a hypothetical API health check. The goal is to show the concrete artifacts involved, not the governance overhead.

### Step 1: Write the spec

A spec is a JSON file that declares a verifiable contract. Here is a minimal API test spec:

```json
{
  "uats_version": "1.0.0",
  "api": {
    "name": "Health Check",
    "base_url": "${BASE_URL}",
    "tags": ["health", "smoke"]
  },
  "metadata": {
    "description": "Validates the health endpoint returns ok",
    "priority": "high"
  },
  "config": {
    "timeout_ms": 15000
  },
  "request": {
    "method": "GET",
    "path": "/healthz"
  },
  "expected": {
    "status": 200,
    "body_assertions": [
      { "path": "$.status", "equals": "ok" }
    ]
  }
}
```

This is data, not code. It describes the contract: `GET /healthz` must return HTTP 200 with `{"status": "ok"}`. Any runner that understands this format can execute it. Any schema can validate its structure. Any CI system can trigger the runner.

### Step 2: Write the schema

The schema defines what fields are legal in specs of this framework:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["uats_version", "api", "request", "expected"],
  "properties": {
    "uats_version": { "type": "string" },
    "api": {
      "type": "object",
      "required": ["name", "base_url"],
      "properties": {
        "name": { "type": "string" },
        "base_url": { "type": "string" },
        "tags": { "type": "array", "items": { "type": "string" } }
      }
    },
    "request": {
      "type": "object",
      "required": ["method", "path"],
      "properties": {
        "method": { "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"] },
        "path": { "type": "string" }
      }
    },
    "expected": {
      "type": "object",
      "required": ["status"],
      "properties": {
        "status": { "type": "integer" },
        "body_assertions": { "type": "array" }
      }
    }
  }
}
```

The schema is the source of truth. If the schema says `request.method` must be one of five values, the runner must reject specs that use anything else.

### Step 3: Write the runner

The runner reads specs and executes the verification. In Python:

```python
#!/usr/bin/env python3
"""Minimal UATS runner — validates API specs against live endpoints."""
import json, sys, requests

def run_spec(spec_path, base_url):
    spec = json.loads(open(spec_path).read())
    url = base_url + spec["request"]["path"]
    resp = requests.request(spec["request"]["method"], url,
                            timeout=spec.get("config", {}).get("timeout_ms", 5000) / 1000)
    failures = []
    if resp.status_code != spec["expected"]["status"]:
        failures.append(f"status: got {resp.status_code}, expected {spec['expected']['status']}")
    for assertion in spec["expected"].get("body_assertions", []):
        # JSONPath evaluation against response body
        actual = extract_jsonpath(resp.json(), assertion["path"])
        if actual != assertion.get("equals"):
            failures.append(f"{assertion['path']}: got {actual}, expected {assertion['equals']}")
    return {"spec": spec_path, "status": "pass" if not failures else "fail", "failures": failures}
```

The runner is the only layer that has dependencies (here: `requests`). The spec and schema are pure JSON.

### Step 4: Wire into CI

Add a Makefile target and a CI workflow step:

```makefile
test-api:
	python3 runners/uats_runner.py validate-all \
		--spec-dir specs/ --base-url $(BASE_URL)
```

```yaml
# .github/workflows/ci.yml
- name: Run API contract tests
  run: make test-api BASE_URL=http://localhost:9999
```

That is a complete UxTS framework: schema validates spec structure, specs declare contracts, runner executes contracts, CI automates execution.

---

## 4. Why This Architecture Works

### 4.1 Specs are diffable contracts

When behavior changes, the spec diff shows exactly what changed. A PR that modifies an API endpoint and updates the corresponding spec makes the behavioral change visible in code review. Without specs, behavioral changes hide inside implementation code where reviewers may not notice them.

### 4.2 Schemas prevent spec rot

Without a schema, specs accumulate fields that nothing validates. Over time, specs diverge from what runners actually check, creating the illusion of thorough coverage. The schema forces every field to be intentional — if it exists in the schema, the runner must handle it.

### 4.3 Runners are replaceable

Because specs are data (not code), you can swap runners without rewriting tests. A Python runner can be replaced with a Go binary, or a local runner with a cloud-based service, as long as it interprets the same spec format. This decouples test logic from test infrastructure.

### 4.4 Governance scales horizontally

Once you have the pattern (schema + spec + runner + CI) for one domain, adding a new domain is mechanical. You don't reinvent test infrastructure — you instantiate the same pattern with a new schema and runner. This is how a single codebase can govern 11 different verification domains without 11 different test philosophies.

### 4.5 Hash integrity catches tampering

Specs and fixtures can include SHA256 hashes. If a spec file is modified outside the normal workflow (manual edit, merge conflict, tooling bug), hash verification catches it before the test runs. This matters most for high-stakes specs (security, benchmark baselines) where a silently modified spec could hide a regression.

---

## 5. Core Principles

1. **One concern domain per framework.** Mixing API tests and parser tests in one framework guarantees confusion about ownership, schema design, and failure semantics.

2. **Declarative specs, executable runners, CI linkage are mandatory.** A framework with specs but no runner is documentation, not verification. A framework with a runner but no CI is verification that nobody runs.

3. **Schema and runner must be explicitly aligned.** Every schema field must be classified as `enforced` (affects pass/fail), `advisory` (warning only), or `unimplemented` (not yet handled). An `active` framework must not have unimplemented fields that are silently ignored — this is the single largest source of false confidence.

4. **Hash integrity is first-class.** Spec files and fixture files should carry SHA256 hashes. Runners should verify hashes before execution.

5. **Fixtures are first-class test inputs.** When a spec references a static file (source code for parsing, config file for validation), that file has its own integrity controls. A stale or tampered fixture is a test that silently validates the wrong thing.

6. **Framework overlap must be resolved by canonical ownership.** If two frameworks both claim "observability," define exactly what each owns (e.g., one owns runtime behavior, the other owns artifact structure). Document the split. Ambiguity here causes specs to land in the wrong framework and get validated by the wrong runner.

---

## 6. Framework Discovery and Bootstrap

When entering a new codebase, the agent should systematically discover what verification needs exist and map them to framework candidates.

### 6.1 Discovery Phase

Scan the codebase for recurring patterns that need verification:

| Pattern | Signals | Candidate Framework |
|---------|---------|-------------------|
| HTTP/REST endpoints | Route definitions, handler functions, OpenAPI specs | API contract tests (UATS-like) |
| gRPC services | `.proto` files, generated stubs | gRPC contract tests (UDTS-like) |
| Parsers / transformers | Language grammars, AST builders, symbol extractors | Parser conformance tests (UPTS-like) |
| Metric endpoints | Prometheus exposition, StatsD clients, metric registrations | Observability artifact tests (UOTS-like) |
| Health/readiness probes | `/healthz`, `/readyz`, dependency checks | Runtime observability tests (UOBS-like) |
| Auth flows | Login handlers, token validation, RBAC checks | Auth contract tests (UAMS-like) |
| Security boundaries | Input validation, CORS, rate limiting, injection guards | Security behavior tests (USTS-like) |
| Performance SLOs | Latency budgets, throughput targets, load test configs | Benchmark regression tests (UBTS-like) |
| LLM/AI quality | Model output scoring, generation quality checks | Quality evaluation tests (UETS-like) |
| Data validation | Schema conformance, semantic quality, retrieval accuracy | Validation quality tests (UVTS-like) |

Not every codebase needs all of these. Start with the domains that have the highest risk of silent drift — typically API contracts and whatever the core processing pipeline is.

### 6.2 Bootstrap Procedure

For each identified domain:

1. **Define the schema.** What fields does a spec in this domain need? Start minimal — you can extend the schema later. Required sections should be: version, metadata, the domain-specific input/config, and the domain-specific expected output/assertions.

2. **Write baseline specs from live code.** Don't write specs from documentation or assumptions. Run the actual code, capture the actual behavior, and encode that behavior as a spec. Specs generated from assumed behavior are the leading cause of false-fail on first run.

3. **Build the runner.** The runner reads a spec, performs the verification, and reports structured results (pass/fail/skip, failure details, timing). Start with a `validate` command for one spec and a `validate-all` command for a directory.

4. **Add schema validation to the runner.** Before executing any spec, the runner should validate it against the framework's JSON schema. This catches structural errors early.

5. **Wire into local automation.** Add a Makefile target (or equivalent) so developers can run the framework locally with one command.

6. **Wire into CI.** Add a workflow step that runs the Makefile target. Start with `soft` gating (report but don't block) until you have confidence in the spec set.

7. **Add hash integrity.** Compute SHA256 for each spec file and embed it in the spec's config section. Have the runner verify hashes on load.

8. **Document governance.** Create a governance matrix entry recording the framework's status, schema location, spec directory, runner command, and CI gate mode.

### 6.3 Recommended Repository Layout

```
docs/
├── specs/
│   ├── FRAMEWORK_GOVERNANCE.md          # Policy: ownership, lifecycle, rules
│   └── <domain>-spec.md                 # Per-framework design spec
├── development/
│   └── UXTS_FRAMEWORK_MATRIX.md         # Operational inventory of all frameworks
├── <domain-group>/
│   └── <framework>/
│       ├── schema/
│       │   └── <framework>.schema.json  # JSON Schema
│       ├── specs/
│       │   └── *.u?ts.json              # Canonical specs
│       ├── drafts/
│       │   └── *.u?ts.json              # Non-canonical / in-progress specs
│       ├── fixtures/                     # Static test inputs (if applicable)
│       │   └── <fixture-files>
│       └── runners/
│           └── <framework>_runner.py     # Executable runner
scripts/
├── verify_uxts_canonical_specs.py       # Guard: no drafts in specs/
└── verify_uxts_drift.py                 # Guard: on-disk reality matches docs
.github/workflows/
└── *.yml                                # CI pipelines per framework
Makefile                                 # Local automation targets
```

The `specs/` vs `drafts/` split is important: specs in `specs/` are canonical and subject to CI gating. Specs in `drafts/` are work-in-progress and excluded from automated runs. This prevents draft specs with incomplete structure from crashing runners or inflating pass counts.

---

## 7. The Canonical Framework Contract

For every framework with status `active`, the agent MUST ensure all of the following exist and are current:

| Requirement | What it means |
|-------------|---------------|
| Schema exists | A `.schema.json` file that defines valid spec structure |
| Spec directory exists | At least one canonical `.u?ts.json` spec file |
| Runner exists and is executable | A script or binary that can read specs and report results |
| CI gate exists | A workflow step or Makefile target that runs the framework |
| Ownership is defined | One team/person/agent responsible for the framework |
| Hash strategy is defined | How spec/fixture integrity is verified (or explicitly waived) |
| Schema-runner parity is documented | Every schema field classified as enforced/advisory/unimplemented |

Required metadata for each framework in the governance matrix:

| Field | Description |
|-------|-------------|
| `acronym` | Framework abbreviation (e.g., UATS) |
| `name` | Full name |
| `domain` | What verification concern it owns |
| `schema_path` | Path to JSON Schema file |
| `spec_glob` | Glob pattern for spec files (e.g., `*.uats.json`) |
| `fixture_glob` | Glob pattern for fixture files (if applicable) |
| `runner_command` | Command to execute the runner |
| `ci_job` | CI workflow/job name |
| `gate_mode` | `block` (failures fail CI), `soft` (report only), `observe` (metrics only) |
| `hash_field_convention` | JSON path to hash field in specs (e.g., `config.sha256`) |
| `status` | `active`, `pilot`, `spec-only`, `deprecated` |

---

## 8. Schema-Runner Parity

This is the single most important governance concept in UxTS. It is also the most frequently violated.

### 8.1 The Problem

A schema defines fields like `setup`, `teardown`, `body_schema`, `retry_policy`. A spec uses some of these fields. The runner reads the spec — but if the runner doesn't implement `setup`, it silently ignores the field. The spec "passes" even though the setup step never ran. This is a false pass.

False passes are worse than test failures because they create the belief that something is verified when it is not. Every UxTS framework that matured through real use discovered this problem.

### 8.2 The Rule

Every schema field MUST be classified in the runner:

| Classification | Runner behavior | Allowed in `active` status? |
|---------------|----------------|---------------------------|
| `enforced` | Field affects pass/fail logic | Yes |
| `advisory` | Field produces a warning but does not affect pass/fail | Yes (for supplementary data) |
| `unimplemented` | Runner detects the field and **hard fails** with an explicit error | Only if field is deprecated |

The critical constraint: **`unimplemented` fields must cause hard failures, not silent skips.** A runner that encounters a field it doesn't implement must refuse to execute the spec, not quietly ignore the field and report "pass."

### 8.3 Implementation Pattern

In the runner, maintain a set of known/handled fields. Before executing any spec, scan for unknown fields:

```python
_KNOWN_FIELDS = {"version", "api", "metadata", "config", "request", "expected", "variants"}

def validate_supported_features(spec: dict) -> list[str]:
    """Detect spec fields the runner does not implement."""
    errors = []
    unknown = set(spec.keys()) - _KNOWN_FIELDS
    if unknown:
        errors.append(f"Unimplemented fields: {unknown} — runner cannot validate this spec")
    # Also check nested fields (e.g., config sub-keys, assertion types)
    return errors
```

If `validate_supported_features` returns errors, the runner must report the spec as FAIL (not SKIP, not WARN), with the unimplemented fields listed in the failure message.

### 8.4 Parity Tracking

Maintain a parity table in the framework matrix documenting which fields are enforced, advisory, and unimplemented per framework. This table is the primary audit artifact for schema-runner alignment.

---

## 9. Framework Maturity Lifecycle

Frameworks progress through four statuses. Each transition has explicit criteria.

```
spec-only ──→ pilot ──→ active ──→ block
```

### spec-only
- Schema and specs exist.
- No runner, no CI, no automation.
- Specs are documentation of intent, not executable verification.
- **Transition to pilot requires:** functional runner, at least one passing spec.

### pilot
- Schema, specs, and runner all exist and function.
- CI may exist but is not gating (soft-fail or observe mode).
- Schema-runner parity may be incomplete.
- **Transition to active requires:** schema-runner parity >= 95%, CI gate enabled (at minimum soft-fail), documented authority scope, no critical false-pass paths.

### active
- Full schema-runner parity. Every schema field is enforced or detected-and-failed.
- CI gate is operational (soft-fail or block).
- Spec set has meaningful coverage (not just one trivial spec).
- **Transition to block requires:** demonstrated stability (no flaky false-fails), low false-pass rate, sufficient spec coverage for the domain.

### block
- CI gate mode is `block` — failures stop the pipeline.
- This is the production-grade state. Treat spec changes with the same rigor as code changes.

### deprecated
- Framework is superseded or no longer relevant.
- Existing specs are archived. Runner may be removed.
- Migration path to successor framework must be documented.

---

## 10. Anti-Patterns and Lessons Learned

These are failure modes discovered through real implementation. They are the hardest-won insights in this specification.

### 10.1 The 0/0 False Pass

A spec defines assertion categories (logging, metrics, tracing) but the runner only implements one category. Specs using unimplemented categories have zero assertions to check, so they pass with 0 failures out of 0 checks. The pass rate is 100% and the coverage is zero.

**Fix:** Runners must require at least one executable assertion per spec. A spec with zero executable assertions must fail, not pass.

### 10.2 The Silent Schema Ignore

A schema defines `setup` and `teardown` blocks. Specs use them. The runner doesn't implement them. The runner reads the spec, ignores the setup/teardown fields, executes the core assertions, and reports "pass." The setup that was supposed to seed test data never ran. The assertions passed against whatever data happened to exist.

**Fix:** Schema-runner parity enforcement (Section 8). Unknown fields cause hard fails.

### 10.3 The Dialect Split

Multiple spec authors use different structures for the same framework. Half the specs use `{"service": "...", "method": "..."}` and the other half use `{"api": "...", "test_cases": [...]}`. The runner handles one dialect; the other silently passes without real validation.

**Fix:** One schema per framework. Specs that don't conform go to `drafts/`, not `specs/`. A canonical guard script rejects non-conforming specs in the `specs/` directory. CI runs the guard on every push.

### 10.4 The Phantom Directory

A framework's documentation claims it has `fixtures/` and `runners/` directories. Neither actually exists. The framework appears complete in the governance matrix but has no executable verification.

**Fix:** Drift checker that validates on-disk reality against documented claims. Every runner path and spec directory declared in the governance matrix must actually exist on disk.

### 10.5 The Ambiguous Baseline

A benchmark framework defines `max_p99_degradation_pct` to catch performance regressions. But degradation compared to what? If the baseline is "the previous run," results are non-deterministic and depend on what was run before. Different CI environments produce different baselines.

**Fix:** Use the spec's own threshold as the fixed baseline. If the spec says `p99_ms: 500`, then `max_p99_degradation_pct: 10` means "fail if actual p99 exceeds 550ms." The baseline is declarative, deterministic, and visible in the spec diff.

### 10.6 The SKIP/WARN False Confidence

A runner encounters a field it can't validate and reports SKIP or WARN. The overall spec result is "pass with warnings." Over time, teams stop reading warnings. The spec has three SKIP'd assertions and one passing trivial assertion — it reports "pass."

**Fix:** Unimplemented fields must cause hard fails in `active` frameworks. The only exception is `advisory` fields (supplementary metadata that genuinely doesn't affect correctness). For structural assertions the runner can't execute, the result must be FAIL, not SKIP.

### 10.7 The Stale Documentation

The governance matrix says "124 specs." The disk has 89. The README says "45 specs." Nobody knows the real number. When counts diverge, every statement about coverage is suspect.

**Fix:** Automated drift checker that compares on-disk spec counts to documented counts. Run in CI. Discrepancies block the pipeline (or at minimum produce loud warnings).

---

## 11. Framework Category Templates

These templates describe the assertion patterns and controls appropriate for common verification domains. Adapt them to your codebase — they are starting points, not rigid requirements.

### 11.1 API Contract Tests

**Domain:** HTTP endpoint behavior (request/response contracts).

Assertions:
- Response status code matches expected.
- Response body matches path-based assertions (JSONPath or equivalent).
- Error variants return correct status codes and error structures.
- Auth boundary variants (missing auth, expired token, wrong role) return correct rejections.
- Response time within threshold (optional, per priority tier).

Controls:
- Base URL resolution via environment variable (not hardcoded).
- Tag-based filtering for selective execution (smoke, regression, full).
- Schema validation before execution (reject malformed specs).
- Variant support (one spec, multiple parameterized executions).
- Hash verification on spec files.
- Fixture-backed request/response payloads with hash verification when files are used.

### 11.2 Parser / Transformer Conformance Tests

**Domain:** Symbol extraction, AST construction, or data transformation correctness.

Assertions:
- Expected symbols/entities are present in output.
- Symbol types match expected types.
- Positional data (line numbers) within configured tolerance.
- Parent/container relationships correct (when declared).
- Signature/value extraction correct (when declared).
- Exclusion policy: symbols marked as excluded must NOT appear.

Controls:
- Strictness flags: `require_all_symbols` (fail if any expected symbol missing), `allow_extra_symbols` (whether unexpected symbols are acceptable).
- Fixture existence and hash verification before parser execution.
- Schema-level enum validation (accepted languages, types, patterns).
- Relationship validation when declared in spec.

### 11.3 Observability Tests

**Domain:** Metric definitions, health endpoints, dashboards, alert rules.

Split into two sub-domains where applicable:
- **Runtime behavior** (requires running service): health probes, dependency checks, runtime metric availability, tracing headers.
- **Artifact structure** (static validation): dashboard JSON structure, alert rule YAML validity, metric definition sets.

Assertions:
- Metric exists with correct type and labels.
- Health endpoint returns expected structure within time budget.
- Dashboard/query references valid metric names.
- Alert rules have valid thresholds and conditions.

Controls:
- Prohibit 0/0 pass (specs with zero executable assertions must fail).
- Require at least one executable assertion per spec.
- Classify manual/synthetic checks as `advisory` only.
- When two observability frameworks exist, document explicit authority split.

### 11.4 Benchmark / Performance Regression Tests

**Domain:** Latency, throughput, and load regression detection.

Assertions:
- Percentile latencies (p50, p95, p99) within thresholds.
- Error rate below threshold.
- Throughput above minimum.
- Success rate above minimum.
- p99 degradation within percentage of spec-defined baseline.

Controls:
- Profile support (smoke, load, stress) with different parameters per profile.
- Deterministic baselines: spec thresholds are the baseline, not previous runs.
- Warmup / ramp-up support.
- Seed data setup (if assertions depend on data state).
- CI gate should be `soft` initially — benchmark flakiness is common until specs stabilize.

### 11.5 Security Behavior Tests

**Domain:** Auth boundaries, injection resilience, rate limiting, sensitive data handling.

Assertions:
- Unauthenticated requests to protected endpoints return 401/403.
- SQL/command injection payloads do not alter behavior.
- Rate limiting triggers at configured threshold.
- Sensitive fields are redacted in error responses.
- CORS headers match policy.

Controls:
- Severity classification (critical, high, medium, low) per assertion.
- Critical/high failures block release in `block` mode.
- Test isolation (security tests should not depend on or corrupt other test data).

### 11.6 Hash Integrity (Cross-Framework)

**Domain:** Integrity verification for spec files, fixture files, and other hash-bearing artifacts across all frameworks.

This is typically a single service/scanner that covers all frameworks rather than a standalone test framework. It should:
- Maintain a registry of known hashes with status (verified, mismatch, unknown).
- Scan all declared hash fields across all framework specs.
- Provide verify-now commands and revert-to-previous-hash workflows.
- Maintain audit log of hash updates and reverts.

---

## 12. Governance Artifacts

The agent must produce and maintain these documents:

| Artifact | Purpose | Update Trigger |
|----------|---------|----------------|
| `FRAMEWORK_GOVERNANCE.md` | Policy: ownership, lifecycle rules, authority splits | Framework status change, policy update |
| `UXTS_FRAMEWORK_MATRIX.md` | Inventory: schema/spec/runner/CI paths, counts, parity status | Any spec/runner/CI change |
| `UXTS_FRAMEWORK_GAP_ASSESSMENT_<date>.md` | Point-in-time audit of all frameworks against the canonical contract | Periodic review or before major changes |

Optional machine-readable reports:
- `reports/schema_runner_coverage.json` — field-level parity data per framework.
- `reports/framework_ci_gating.json` — CI gate status per framework.
- `reports/hash_coverage.json` — hash field coverage across all frameworks.

---

## 13. Automated Drift Detection

UxTS governance is only as good as the automation that enforces it. Two guard scripts should run in CI:

### 13.1 Canonical Spec Guard

Validates that `specs/` directories contain only schema-conforming specs. Non-canonical formats must live in `drafts/`.

### 13.2 Framework Drift Checker

Validates that on-disk reality matches documented state:

1. **Spec counts** — actual file count per framework matches count in governance matrix.
2. **Runner existence** — every declared runner path actually exists on disk.
3. **Fixture existence** — every fixture path referenced by a spec actually exists.
4. **Hash coverage** — specs with hash fields contain real hashes (not empty or "PENDING").

Both scripts should be wired into CI. The canonical guard should `block`. The drift checker can start as `soft` and promote to `block` once stable.

---

## 14. Threat Model

For each framework, the agent MUST evaluate and document:

| Risk | Description | Mitigation |
|------|-------------|------------|
| **False pass** | Spec passes with zero real assertions, or runner ignores assertions | Schema-runner parity, 0/0 prohibition |
| **False fail** | Spec fails due to schema drift, environment sensitivity, or flaky assertions | Deterministic baselines, environment-variable resolution, fixture stability |
| **Coverage blind spot** | Domain has verification needs but no specs, or specs exist but no CI gate | Framework discovery audit, CI drift checker |
| **Integrity blind spot** | Hash fields missing or not scanned | Cross-framework hash scanner |
| **Fixture drift** | Fixture modified without updating spec hash; stale fixture invalidates test | Fixture hash verification before execution |
| **Dialect split** | Multiple incompatible spec formats under one framework | Canonical guard, drafts separation |

---

## 15. Acceptance Criteria

A repository is UxTS-governed when:

1. All `active` frameworks satisfy the canonical framework contract (Section 7).
2. Schema-runner parity is documented and current for every framework (Section 8).
3. No `active` framework has silent false-pass behavior (0/0 passes, ignored fields).
4. CI gates are declared and operational per framework status.
5. Hash scanner covers all declared hash-bearing artifacts across all frameworks.
6. Automated drift detection runs in CI and catches documentation/reality divergence.
7. Framework overlap is resolved with documented authority splits.
8. Deprecated frameworks have migration plans.
9. Every schema field in every `active` framework is classified as `enforced`, `advisory`, or hard-fail `unimplemented`.
