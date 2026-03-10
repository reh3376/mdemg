# UxTS Portable Agent Specification

Version: 3.0.0
Date: 2026-02-27
Audience: Coding agents implementing Universal-x Test Specification governance in arbitrary codebases.

**Changelog from 2.3.0-draft:**
- Added schema evolution and spec migration strategy (Section 7A).
- Added canonical assertion grammar for cross-framework interoperability (Section 5.2).
- Added environment variable resolution standard (Section 5.3).
- Added spec composition and defaults mechanism (Section 5.4).
- Added secret handling rules (Section 5.5).
- Added governance tiers — lite and full (Section 9.1).
- Added parallel execution semantics (Section 8B).
- Added error taxonomy and retry semantics to report schema (Section 8A).
- Added tag filtering standard (Section 8C).
- Added aggregate report schema (Section 12).
- Added tooling ecosystem and adoption pathway (Section 16).
- Extended anti-patterns with The Environment Leak (10.8) and The Duplication Explosion (10.9).
- Extended acceptance criteria for new normative requirements.

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

**Fixture** — A static input file that a spec references for testing. For example, a parser spec might reference a `.go` source file as its fixture. Fixtures have their own integrity controls (hashes) because a modified fixture can silently invalidate test results without anyone realizing the test inputs changed.

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

### Step 4: Wire into local automation and CI

Add a Makefile target (or equivalent task runner — `justfile`, `Taskfile.yml`, `package.json` scripts, etc.):

```makefile
test-api:
	python3 runners/uats_runner.py validate-all \
		--spec-dir specs/ --base-url $(BASE_URL) --report /tmp/api-report.json
```

Then wire the target into your CI system. The CI integration is a thin wrapper around the same command:

**GitHub Actions:**
```yaml
- name: Run API contract tests
  run: make test-api BASE_URL=http://localhost:9999
```

**GitLab CI:**
```yaml
api-contracts:
  script: make test-api BASE_URL=http://localhost:9999
```

**Jenkins (declarative):**
```groovy
stage('API Contracts') {
    steps { sh 'make test-api BASE_URL=http://localhost:9999' }
}
```

**Azure Pipelines:**
```yaml
- script: make test-api BASE_URL=http://localhost:9999
  displayName: 'Run API contract tests'
```

The pattern is always the same: the Makefile target is the portable interface, and the CI system invokes it. This means switching CI providers requires changing the workflow file but not the runners, specs, or Makefile.

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

### 4.5 Hash integrity detects unreviewed changes

Specs and fixtures can include SHA256 hashes. If a spec file is modified — intentional edit, merge conflict resolution, tooling side-effect — hash verification alerts the developer that the file changed. The hash is a change-detection mechanism: it answers "was this file modified since last review?" not "is this file correct?" Correctness is the assertions' job. The hash ensures no change goes unnoticed, which matters most for high-stakes specs (security, benchmark baselines) where an unreviewed modification could mask a regression.

---

## 5. Core Principles

1. **One concern domain per framework.** Mixing API tests and parser tests in one framework guarantees confusion about ownership, schema design, and failure semantics.

2. **Declarative specs, executable runners, CI linkage are mandatory.** A framework with specs but no runner is documentation, not verification. A framework with a runner but no CI is verification that nobody runs.

3. **Schema and runner must be explicitly aligned.** Every schema field must be classified as `enforced` (affects pass/fail), `advisory` (warning only), or `unimplemented` (not yet handled). An `active` framework must not have unimplemented fields that are silently ignored — this is the single largest source of false confidence.

4. **Hash integrity is first-class.** Spec files and fixture files should carry SHA256 hashes computed using the canonical procedure defined in Section 5.1. Hashes are a change-detection mechanism — they alert developers that a file was modified, not that a file is incorrect. Runners always verify hashes and always execute assertions. These are independent signals reported separately (see Section 5.1 for runner behavior, Section 8A for report structure).

5. **Fixtures are first-class test inputs.** When a spec references a static file (source code for parsing, config file for validation), that file has its own change-detection hash. A modified fixture without a corresponding spec update means the test is validating inputs the developer may not have reviewed.

6. **Framework overlap must be resolved by canonical ownership.** If two frameworks both claim "observability," define exactly what each owns (e.g., one owns runtime behavior, the other owns artifact structure). Document the split. Ambiguity here causes specs to land in the wrong framework and get validated by the wrong runner.

7. **Expand before creating.** In existing codebases, agents MUST attempt to extend an existing framework before introducing a new `U<X>TS` framework. Creating a new framework requires explicit justification that extension would cause semantic overlap, schema distortion, or ownership ambiguity.

### 5.1 Hash Computation Procedure

All UxTS implementations MUST use this procedure to ensure portable, deterministic hash values.

**Spec file hashes** (integrity of the spec itself):

1. The hash field location is defined per framework (e.g., `config.sha256` for UATS, `fixture.sha256` for UPTS). This path is recorded in the governance matrix under `hash_field_convention`.
2. To compute the hash: read the spec file as raw bytes, parse it as JSON, remove the hash field from the parsed structure, re-serialize to canonical JSON (sorted keys, no trailing whitespace, UTF-8, `\n` line endings), and compute SHA256 over the resulting bytes.
3. To verify: read the stored hash value, recompute using step 2, and compare. Mismatch = integrity failure.

The critical rule is **the hash field itself is excluded from the hash input**. Without this rule, embedding the hash changes the file, which changes the hash, creating an unsolvable circular dependency.

**Canonical JSON serialization** for hash computation:
- Keys sorted lexicographically at every nesting level.
- No trailing commas, no comments.
- Indent: 2 spaces (or 0 for compact — choose one per codebase and document it).
- Encoding: UTF-8 without BOM.
- Line endings: `\n` (Unix-style).

**Fixture file hashes** (integrity of test input files):

1. Fixture hashes are stored in the spec that references the fixture (e.g., `fixture.sha256`).
2. To compute: read the fixture file as raw bytes, compute SHA256 over the raw bytes. No JSON parsing or re-serialization — fixtures may be any format (source code, YAML, binary).
3. To verify: recompute and compare before executing the spec that references the fixture.

**Runner behavior — always execute, always report:**

Hash verification and assertion evaluation are **independent operations**. The runner always does both, and reports the results separately.

1. **Verify hash.** If the spec has a hash field: compare stored hash to computed hash. Record `hash_verified: true` (match) or `hash_verified: false` (mismatch), and record mismatch details in `hash_mismatches[]`. If the spec has no hash field: record `hash_verified: null` (not applicable).
2. **Execute assertions.** Run all spec assertions regardless of hash result. Record `pass` or `fail` based on assertion outcomes.
3. **Report both.** The per-spec `status` field reflects assertion results only. The `hash_verified` and `hash_mismatches` fields reflect integrity results only. These are never conflated.

A spec can have four outcome combinations:

| Assertions | Hash | Meaning |
|-----------|------|---------|
| pass | verified | Spec correct, file unchanged — healthy state |
| pass | mismatch | Spec correct, but file was modified since last hash — developer should review and recompute hash |
| fail | verified | Spec incorrect, file unchanged — real regression |
| fail | mismatch | Spec incorrect AND file was modified — investigate whether the modification caused the failure |

**CI integrity policy:**

Whether hash mismatches block the pipeline is a gate-mode decision, not a runner decision. The runner always reports hash status; CI decides what to do with it.

- `block` gate mode: CI MUST treat hash mismatches as pipeline failures (separate from assertion failures). This ensures no unreviewed spec/fixture changes reach production.
- `soft` gate mode: CI reports hash mismatches visibly but does not block. This is appropriate during active development.
- `observe` gate mode: Hash status is recorded in the report for metrics only.

The runner itself does not have a `--skip-hash` flag. Hash verification is always performed when a hash field is present. If no hash field exists in a spec, `hash_verified` is reported as `null` (not applicable) rather than `true` or `false`.

### 5.2 Canonical Assertion Grammar

All UxTS runners SHOULD support this minimal assertion grammar to ensure cross-framework interoperability. Frameworks MAY extend this grammar with domain-specific operators, but the canonical operators provide a portable baseline that tooling can rely on.

**Path resolution syntax:** JSONPath (`$.field.nested`) is the canonical path syntax for all UxTS assertions. Runners that support alternative syntaxes (JMESPath, jq-style) MUST also accept JSONPath as the default.

**Canonical assertion operators:**

| Operator | Meaning | Example |
|----------|---------|---------|
| `equals` | Exact match (type-sensitive) | `{"path": "$.status", "equals": "ok"}` |
| `not_equals` | Negated exact match | `{"path": "$.status", "not_equals": "error"}` |
| `contains` | Substring (strings) or subset (arrays/objects) | `{"path": "$.message", "contains": "success"}` |
| `not_contains` | Negated containment | `{"path": "$.tags", "not_contains": "deprecated"}` |
| `matches` | Regex match (PCRE or ECMAScript subset) | `{"path": "$.id", "matches": "^[a-f0-9]{8}$"}` |
| `type_is` | Type check | `{"path": "$.count", "type_is": "integer"}` |
| `greater_than` | Numeric comparison (strict) | `{"path": "$.latency_ms", "greater_than": 0}` |
| `less_than` | Numeric comparison (strict) | `{"path": "$.latency_ms", "less_than": 500}` |
| `greater_than_or_equal` | Numeric comparison (inclusive) | `{"path": "$.count", "greater_than_or_equal": 1}` |
| `less_than_or_equal` | Numeric comparison (inclusive) | `{"path": "$.error_rate", "less_than_or_equal": 0.01}` |
| `exists` | Path resolves to a non-null value | `{"path": "$.created_at", "exists": true}` |
| `not_exists` | Path does not resolve or is null | `{"path": "$.deleted_at", "not_exists": true}` |
| `one_of` | Value is one of a set | `{"path": "$.status", "one_of": ["ok", "degraded"]}` |
| `length` | Array/string length equals | `{"path": "$.items", "length": 10}` |
| `length_greater_than` | Array/string length exceeds | `{"path": "$.items", "length_greater_than": 0}` |

**Type semantics for `type_is`:** Valid values are `string`, `integer`, `number`, `boolean`, `array`, `object`, `null`. These match JSON Schema type keywords.

**Operator precedence:** Each assertion object MUST contain exactly one operator key (plus `path`). Multiple operators in a single assertion object are invalid — use separate assertion entries.

**Runner implementation requirement:** A runner that encounters an assertion operator it does not implement MUST report a parity failure (not silently skip the assertion). This is a specific application of the schema-runner parity rule (Section 8).

### 5.3 Environment Variable Resolution

Specs frequently reference deployment-specific values (base URLs, port numbers, auth tokens) that vary between local, CI, staging, and production environments. All UxTS runners MUST resolve these values using a canonical syntax and resolution order.

**Canonical syntax:**

| Pattern | Meaning |
|---------|---------|
| `${VAR_NAME}` | Resolve from environment; hard-fail if unset |
| `${VAR_NAME:-default_value}` | Resolve from environment; use `default_value` if unset |

**Resolution order** (first match wins):

1. CLI flags passed to the runner (e.g., `--base-url http://localhost:9999`)
2. Environment variables (e.g., `BASE_URL=http://localhost:9999`)
3. Spec-level default (the `:-default` syntax)
4. Hard failure with explicit error message naming the unresolved variable

**Normative rules:**

- Runners MUST fail with a clear error if a `${VAR_NAME}` reference (no default) cannot be resolved. The error must name the variable and the spec file that references it.
- Runners MUST NOT pass the literal string `${VAR_NAME}` to the target system. This is a common source of confusing test failures where an HTTP request is sent to `${BASE_URL}/healthz` as a literal URL.
- Variable resolution happens before schema validation. The schema validates the resolved spec, not the template.
- Variable references are valid in any string-typed field. They are not valid in numeric, boolean, or structural fields.

### 5.4 Spec Composition and Defaults

When a framework has many specs sharing common configuration (same base URL, same auth headers, same timeout), duplicating those values in every spec creates maintenance burden and divergence risk. UxTS supports an optional defaults mechanism to reduce duplication.

**The `_defaults.json` file:**

A file named `_defaults.json` in a spec directory provides base values that individual specs inherit. The runner merges defaults with each spec before schema validation and execution.

```json
{
  "_uxts_defaults": true,
  "api": {
    "base_url": "${BASE_URL:-http://localhost:9999}"
  },
  "config": {
    "timeout_ms": 15000,
    "retry": { "max_attempts": 1, "backoff_ms": 0 }
  }
}
```

**Merge rules:**

1. Individual spec values override defaults at the same path. Deep merge — not shallow replacement.
2. The `_uxts_defaults` marker field is stripped before schema validation (it is not a spec field).
3. Schema validation runs on the merged result, not on individual files.
4. The `_defaults.json` file itself is NOT a spec and is NOT executed by the runner. It has no `status`, no assertions, and produces no report entry.
5. Hash computation for a spec includes only the spec file itself, not the merged result. The `_defaults.json` file SHOULD have its own hash tracked in the governance matrix.

**Nesting:** Subdirectories may have their own `_defaults.json` that overrides the parent directory's defaults. Merge order: root defaults → subdirectory defaults → individual spec.

**When NOT to use defaults:** Defaults are for shared infrastructure config (URLs, timeouts, auth patterns). They are NOT for shared assertions or shared expected values — those belong in the spec because they are the contract.

### 5.5 Secret Handling

Specs MUST NOT contain plaintext secrets (API keys, auth tokens, passwords, certificates). Sensitive values MUST use one of the following patterns:

**Environment variable reference** (preferred for most cases):
```json
{ "auth_header": "Bearer ${API_TOKEN}" }
```

**Secret store reference** (for vault-backed workflows):
```json
{ "auth_header": "Bearer ${SECRET:vault/path/to/token}" }
```

The `${SECRET:path}` syntax signals that the runner must resolve the value from a configured secret backend (HashiCorp Vault, AWS Secrets Manager, 1Password CLI, etc.). The backend is configured per-runner, not per-spec.

**Normative rules:**

- Runners MUST NOT log or include resolved secret values in report output.
- The canonical guard script (Section 13.1) SHOULD scan spec files for patterns that look like plaintext secrets (high-entropy strings, `Bearer ey...` patterns, `-----BEGIN` blocks) and flag them as warnings.
- Fixture files that contain secrets MUST be listed in `.gitignore` and referenced via environment-specific paths.

---

## 6. Framework Discovery and Bootstrap

When entering a new codebase, the agent MUST systematically discover what verification needs exist, map them to framework candidates, and in brownfield repositories propose and execute prioritized remediations.

### 6.0 Operating Mode (Mandatory)

The agent MUST explicitly declare one operating mode at the start of work:

- `greenfield`: No existing UxTS artifacts are present.
- `brownfield`: Existing codebase already contains production constructs and/or partial UxTS artifacts.

Use this decision rule:

- `greenfield` only if ALL are true:
  - No `*.u?ts.json` files exist.
  - No framework governance artifacts exist (`FRAMEWORK_GOVERNANCE.md`, `UXTS_FRAMEWORK_MATRIX.md`, equivalent).
  - No existing runner commands or CI jobs reference UxTS-like contract execution.
- Otherwise use `brownfield`.

In `brownfield` mode, Sections 6.1 through 6.4 are all mandatory and the agent MUST produce brownfield deliverables listed in Section 12.

### 6.1 Discovery Phase

Discovery must be systematic and reproducible. Two agents running this procedure against the same codebase should produce the same framework candidates.

**Recurring construct definition (normative):**

A construct qualifies as recurring if ANY condition is met:

- Pattern appears in `>= 3` concrete implementation instances.
- Pattern appears in `>= 2` modules/services with similar verification semantics.
- Pattern appears once but is high-risk (public API surface, auth/security boundary, data integrity path, compliance-critical flow, or SLO-governed path).

Only recurring constructs are eligible for frameworkization candidates.

#### 6.1.1 Deterministic Command Protocol (Required)

Agents MUST run discovery commands in this exact order and capture raw output under `reports/uxts_discovery/` (or equivalent path documented in the discovery artifact).

1. Baseline file inventory:
   - `rg --files > reports/uxts_discovery/01_files.txt`
2. Existing UxTS/governance inventory:
   - `rg -n --hidden -S "UATS|UPTS|UBTS|USTS|UOTS|UOBS|UDTS|UVTS|UETS|UAMS|UNTS|FRAMEWORK_GOVERNANCE|UXTS_FRAMEWORK_MATRIX|\\.u[a-z]+ts\\.json" . > reports/uxts_discovery/02_uxts_inventory.txt`
3. API/gRPC/runtime surface signals:
   - `rg -n --hidden -S "router\\.|@app\\.route|http\\.HandleFunc|gin\\.|FastAPI\\(|\\.proto|grpc|/healthz|/readyz|/metrics" . > reports/uxts_discovery/03_interface_signals.txt`
4. Parser/transformation signals:
   - `rg -n --hidden -S "parser|parse\\(|AST|tree-sitter|symbol extractor|tokenizer|grammar" . > reports/uxts_discovery/04_parser_signals.txt`
5. Security/benchmark/quality signals:
   - `rg -n --hidden -S "auth|rbac|jwt|oauth|rate limit|latency|p95|p99|throughput|load test|SLO|LLM|embedding|retrieval|evaluation" . > reports/uxts_discovery/05_risk_signals.txt`

If `rg` is unavailable, use an equivalent search tool and document the substitution in the discovery artifact.

**Step 1: Enumerate concrete artifacts.** Search the codebase for the following file patterns, in this order. Each match is a signal for a candidate framework.

| Priority | Search pattern | What it signals | Candidate framework |
|----------|---------------|-----------------|-------------------|
| 1 | Route/handler registrations (e.g., `router.GET`, `@app.route`, `http.HandleFunc`) | HTTP API surface exists | API contract tests |
| 2 | `.proto` files or gRPC generated code | gRPC service surface exists | gRPC contract tests |
| 3 | Parser grammars, AST definitions, tree-sitter configs, symbol extractors | Parsing/transformation pipeline exists | Parser conformance tests |
| 4 | Prometheus client registrations, `/metrics` endpoints, StatsD calls | Observable metrics exist | Observability artifact tests |
| 5 | `/healthz` or `/readyz` handlers, dependency check functions | Health probes exist | Runtime observability tests |
| 6 | Auth middleware, token validation, RBAC decorators | Auth boundary exists | Security / auth tests |
| 7 | Latency budgets in configs, existing load test scripts, SLO definitions | Performance contracts exist | Benchmark regression tests |
| 8 | LLM API calls, model scoring functions, generation pipelines | AI quality surface exists | Quality evaluation tests |

**Step 2: Map to existing frameworks first (mandatory).** Before proposing a new framework, map each candidate to existing frameworks already present in the repo. The default action is extension, not creation.

**New framework gate:** a new framework MAY be created only if all are documented:

- Why no existing framework can represent the construct without semantic overlap.
- Why schema extension would degrade clarity or ownership boundaries.
- Proposed ownership and CI integration for the new framework.

**Step 3: Deduplicate and prioritize.** If multiple signals map to the same candidate, merge them. If a signal is ambiguous (e.g., an endpoint that is both an API and a health probe), assign it to the more specific framework (health probe -> observability, not API contracts).

**Step 4: Rank by drift risk.** For each candidate, estimate the cost of undetected drift. Rank higher:
- Frameworks covering the primary external interface (APIs, gRPC) — highest drift risk.
- Frameworks covering the core processing pipeline (parsers, transformers) — data quality risk.
- Frameworks covering security boundaries — compliance risk.
- Frameworks covering observability and benchmarks — operational risk.

**Step 5: Bootstrap in rank order.** Start with the highest-risk candidate and complete one full framework (schema -> spec -> runner -> CI) before starting the next. Attempting to bootstrap all frameworks in parallel leads to many incomplete frameworks instead of a few solid ones.

**Decision rule:** If a candidate has fewer than 3 concrete artifacts in the codebase, defer it to `spec-only` status. If it has 3+, bootstrap to `pilot`.

### 6.2 Bootstrap Procedure

For each identified domain:

1. **Define the schema.** What fields does a spec in this domain need? Start minimal — you can extend the schema later. Required sections should be: version, metadata, the domain-specific input/config, and the domain-specific expected output/assertions.

2. **Write baseline specs from live code.** Don't write specs from documentation or assumptions. Run the actual code, capture the actual behavior, and encode that behavior as a spec. Specs generated from assumed behavior are the leading cause of false-fail on first run.

3. **Build the runner.** The runner reads a spec, performs the verification, and reports results in the canonical report format (Section 8A). Start with a `validate` command for one spec and a `validate-all` command for a directory. The runner must support `--report <path>` to write structured JSON output.

4. **Add schema validation to the runner.** Before executing any spec, the runner should validate it against the framework's JSON schema. This catches structural errors early.

5. **Wire into local automation.** Add a Makefile target (or equivalent) so developers can run the framework locally with one command.

6. **Wire into CI.** Add a CI step that invokes the Makefile target. The Makefile is the portable interface — the CI step is a thin wrapper (see Section 3, Step 4 for examples across GitHub Actions, GitLab CI, Jenkins, and Azure Pipelines). Start with `soft` gating (report but don't block) until you have confidence in the spec set.

7. **Add hash integrity.** Define the framework's hash field path (e.g., `config.sha256`, `fixture.sha256`) and record it in the governance matrix under `hash_field_convention`. Compute SHA256 for each spec file using the canonical procedure (Section 5.1) and embed the hash at the defined path. Have the runner verify hashes on load.

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
<ci-config>/                              # CI pipelines (platform-dependent location)
└── *                                     #   GitHub: .github/workflows/*.yml
                                          #   GitLab: .gitlab-ci.yml
                                          #   Jenkins: Jenkinsfile
                                          #   Azure: azure-pipelines.yml
Makefile                                 # Local automation targets (portable CI interface)
```

The `specs/` vs `drafts/` split is important: specs in `specs/` are canonical and subject to CI gating. Specs in `drafts/` are work-in-progress and excluded from automated runs. This prevents draft specs with incomplete structure from crashing runners or inflating pass counts.

### 6.4 Brownfield Opportunity Backlog and Remediation Loop (Mandatory in Brownfield Mode)

In `brownfield` mode, discovery is not complete until remediation opportunities are prioritized and acted on.

1. Build an opportunity backlog for recurring constructs.
2. Score each opportunity with the rubric below.
3. Implement the top-priority opportunities.
4. Re-run discovery and update backlog status after implementation.

**Opportunity record (required fields):**

| Field | Description |
|-------|-------------|
| `id` | Stable identifier (e.g., `OPP-001`) |
| `construct` | Recurring construct name |
| `evidence_paths` | File paths proving recurrence |
| `current_state` | Current verification state (none/manual/partial/framework) |
| `target_framework` | Existing framework to extend or proposed new framework |
| `action_type` | `extend-existing` or `create-new` |
| `priority_score` | Numeric score from rubric |
| `status` | `planned`, `in_progress`, `implemented`, `waived` |

**Scoring rubric (1-5 each, higher = more urgent):**

| Dimension | Description |
|-----------|-------------|
| Recurrence | How many concrete instances exist |
| Blast radius | Impact if drift escapes |
| Change frequency | How often this area changes |
| Compliance/security risk | Regulatory or security consequence |
| Detection gap | How weak current verification is |

`priority_score = recurrence + blast_radius + change_frequency + compliance_security_risk + detection_gap`

Tie-breaker order: higher blast radius, then higher compliance/security risk, then higher recurrence.

**Execution requirement (normative):**

- Agent MUST implement at least one high-priority (`priority_score >= 18`) opportunity to `pilot` status in the same engagement.
- If implementation is blocked, agent MUST produce a waiver entry with:
  - blocker evidence,
  - explicit owner,
  - due date,
  - interim risk mitigation.
- A brownfield review without either implemented high-priority remediation or a documented waiver is incomplete.

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
| `gate_mode` | `block` (failures fail CI), `soft` (report only), `observe` (metrics only). See Section 9 for the distinction between status and gate mode. |
| `hash_field_convention` | JSON path to hash field in specs (e.g., `config.sha256`). See Section 5.1 for computation procedure. |
| `status` | `spec-only`, `pilot`, `active`, `deprecated`. Status governs maturity; gate mode governs CI behavior. These are independent axes (Section 9). |
| `schema_version` | Current schema semver (e.g., `1.2.0`). See Section 7A for versioning rules. |
| `defaults_path` | Path to `_defaults.json` if spec composition is used (Section 5.4), or `null`. |

---

## 7A. Schema Evolution and Spec Migration

Schemas evolve as frameworks mature. New assertion types get added, deprecated fields get removed, and structural changes improve clarity. Without a versioning strategy, schema changes silently break existing specs or — worse — existing specs pass validation against a schema they weren't written for.

### 7A.1 Schema Versioning Rules

Every framework schema MUST include a version, and every spec MUST declare which schema version it targets.

**Schema version** is embedded in the schema file:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "_uxts_schema_version": "1.2.0",
  "type": "object",
  ...
}
```

**Spec version reference** uses the framework version field (e.g., `uats_version`):
```json
{
  "uats_version": "1.2.0",
  ...
}
```

**Semantic versioning for schemas:**

| Change type | Version bump | Examples |
|-------------|-------------|---------|
| **MAJOR** (breaking) | `1.x.x` → `2.0.0` | Required field added, field renamed, field type changed, field removed |
| **MINOR** (additive) | `1.1.x` → `1.2.0` | Optional field added, new enum value added, new assertion operator |
| **PATCH** (non-structural) | `1.1.0` → `1.1.1` | Description updated, examples changed, documentation-only changes |

### 7A.2 Runner Compatibility Behavior

When a runner loads a spec, it MUST compare the spec's declared version against the schema version the runner implements:

| Spec version vs Runner version | Runner behavior |
|-------------------------------|-----------------|
| Exact match | Normal execution |
| Spec MINOR < Runner MINOR (same MAJOR) | Normal execution (backward compatible) |
| Spec MINOR > Runner MINOR (same MAJOR) | Warn: spec may use fields the runner doesn't know. Apply parity rules — unknown fields cause hard fail. |
| MAJOR mismatch | Hard fail with migration guidance: "Spec targets schema v2.x but runner implements v1.x. Run migration." |

### 7A.3 Migration Procedure

When a schema MAJOR version changes, existing specs must be migrated. The framework SHOULD provide a migration script or the migration MUST be documented as a manual procedure.

**Migration script pattern:**

```python
#!/usr/bin/env python3
"""Migrate UATS specs from schema v1.x to v2.0."""
import json, sys
from pathlib import Path

def migrate_v1_to_v2(spec: dict) -> dict:
    """Transform a v1.x spec to v2.0 format."""
    migrated = dict(spec)
    migrated["uats_version"] = "2.0.0"
    # Example: field renamed
    if "test_cases" in migrated:
        migrated["variants"] = migrated.pop("test_cases")
    # Example: field restructured
    if "auth" in migrated.get("config", {}):
        migrated.setdefault("security", {})["auth"] = migrated["config"].pop("auth")
    return migrated

for path in Path(sys.argv[1]).glob("*.uats.json"):
    spec = json.loads(path.read_text())
    if spec.get("uats_version", "").startswith("1."):
        migrated = migrate_v1_to_v2(spec)
        path.write_text(json.dumps(migrated, indent=2, sort_keys=True) + "\n")
        print(f"Migrated: {path}")
```

**Migration governance:**

- Migration scripts MUST be stored alongside the schema (e.g., `schema/migrate_v1_to_v2.py`).
- After migration, all specs MUST pass schema validation against the new version.
- Hash values MUST be recomputed after migration (the file content has changed).
- The governance matrix MUST be updated to reflect the new schema version.

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

## 8A. Canonical Runner Report Schema

All runners MUST produce structured output in a common format so that cross-framework governance tooling (drift checkers, CI aggregators, dashboards) can consume results from any framework without framework-specific parsing.

### Report Structure

```json
{
  "timestamp": "2026-02-27T14:30:00Z",
  "framework": "uats",
  "framework_version": "1.1.0",
  "summary": {
    "total_specs": 124,
    "passed": 122,
    "failed": 1,
    "skipped": 1,
    "errors": 0,
    "pass_rate": 98.4,
    "duration_ms": 4521
  },
  "integrity": {
    "total_hashed": 123,
    "verified": 121,
    "mismatched": 2,
    "no_hash": 1
  },
  "results": [
    {
      "spec_path": "specs/health.uats.json",
      "status": "pass",
      "duration_ms": 42,
      "hash_verified": true,
      "hash_mismatches": [],
      "assertions_evaluated": 3,
      "assertions_passed": 3,
      "failures": [],
      "warnings": [],
      "error": null
    },
    {
      "spec_path": "specs/ingest.uats.json",
      "status": "fail",
      "duration_ms": 187,
      "hash_verified": false,
      "hash_mismatches": [
        "config.sha256: expected abc123..., got def456..."
      ],
      "assertions_evaluated": 8,
      "assertions_passed": 6,
      "failures": [
        "expected status 201, got 500",
        "$.node_id: expected string, got null"
      ],
      "warnings": [],
      "error": null
    }
  ]
}
```

### Required Fields

**Top-level and assertion summary:**

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | ISO 8601 string | When the run started |
| `framework` | string | Framework acronym (lowercase) |
| `framework_version` | string | Runner/spec version |
| `summary.total_specs` | integer | Total specs processed |
| `summary.passed` | integer | Specs with status `pass` |
| `summary.failed` | integer | Specs with status `fail` |
| `summary.skipped` | integer | Specs with status `skip` (excluded by tag filter) |
| `summary.errors` | integer | Specs that could not be executed (parse error, runner crash) |
| `summary.pass_rate` | float | `(passed / total_specs) * 100` |
| `summary.duration_ms` | float | Total wall-clock time |

**Integrity summary** (reported independently from assertion results):

| Field | Type | Description |
|-------|------|-------------|
| `integrity.total_hashed` | integer | Specs that have a hash field defined |
| `integrity.verified` | integer | Specs where stored hash matches computed hash |
| `integrity.mismatched` | integer | Specs where stored hash does not match — files were modified |
| `integrity.no_hash` | integer | Specs with no hash field (hash verification not applicable) |

**Per-spec results:**

| Field | Type | Description |
|-------|------|-------------|
| `results[].spec_path` | string | Relative path to spec file |
| `results[].status` | enum | `pass`, `fail`, `skip`, `error` — based on **spec verification only** (assertions + parity), never hash results |
| `results[].duration_ms` | float | Execution time for this spec |
| `results[].hash_verified` | boolean or null | `true` = hash match, `false` = hash mismatch, `null` = no hash field in spec |
| `results[].hash_mismatches` | string[] | Details of each hash mismatch (empty if verified or no hash) |
| `results[].assertions_evaluated` | integer | Number of assertions actually checked |
| `results[].assertions_passed` | integer | Number of assertions that passed |
| `results[].failures` | string[] | Human-readable assertion failure descriptions |
| `results[].warnings` | string[] | Advisory messages (do not affect status) |
| `results[].error` | string or null | Error message if status is `error` |

### Status Semantics

The `status` field reflects **spec verification results**. Hash verification results are reported separately via `hash_verified` and `hash_mismatches`. These are independent signals — a spec can pass verification but have a hash mismatch, or fail verification with a verified hash.

- `pass`: All evaluated assertions passed. `assertions_evaluated` must be >= 1 (0/0 is not a pass).
- `fail`: Two fail classes exist:
  - **Assertion failure:** One or more assertions evaluated and did not pass.
  - **Parity failure:** The spec uses a field that the runner does not implement (unimplemented-field detection per Section 8). This is a fail even though no assertion was evaluated — the runner cannot verify what the spec asks for.
- `skip`: Spec was intentionally not executed (excluded by tag filter or explicit `--exclude` flag).
- `error`: Runner could not execute the spec (parse failure, missing fixture, runner crash).

### Output Conventions

- Runners MUST write the report to the path specified by `--report <path>` flag.
- Runners MUST print a human-readable summary to stdout that includes both assertion results (total/passed/failed/error) and integrity results (verified/mismatched). These are separate lines — do not merge them.
- Report files use `.json` extension and UTF-8 encoding.

### Error Categories

When a spec has `status: "error"`, the `error` field SHOULD include a category prefix to enable programmatic triage:

| Category | Prefix | Meaning |
|----------|--------|---------|
| Parse error | `PARSE:` | Spec JSON is malformed or fails schema validation |
| Fixture missing | `FIXTURE:` | Referenced fixture file does not exist on disk |
| Connection error | `CONNECTION:` | Target service is unreachable |
| Timeout | `TIMEOUT:` | Execution exceeded configured timeout |
| Runner error | `RUNNER:` | Internal runner failure (crash, OOM, unhandled exception) |
| Dependency error | `DEPENDENCY:` | Required external tool or library unavailable |
| Version mismatch | `VERSION:` | Spec schema version incompatible with runner (Section 7A) |

Example: `"error": "CONNECTION: Failed to connect to http://localhost:9999 — connection refused"`

### Retry Reporting

When a runner supports retries (via spec-level or defaults-level `retry` config), retried specs MUST be flagged in the report:

```json
{
  "spec_path": "specs/flaky-endpoint.uats.json",
  "status": "pass",
  "retried": true,
  "attempts": 3,
  "duration_ms": 892,
  ...
}
```

**Additional per-spec fields when retries are enabled:**

| Field | Type | Description |
|-------|------|-------------|
| `results[].retried` | boolean | `true` if the spec was executed more than once. Default `false`. |
| `results[].attempts` | integer | Total execution attempts (1 = no retry). Default `1`. |

**Summary-level flaky tracking:**

| Field | Type | Description |
|-------|------|-------------|
| `summary.flaky_passes` | integer | Specs that passed only after retry. These are `pass` in status but warrant investigation. |

A spec that passes only on retry is a `pass` in the `status` field (it did ultimately pass), but `retried: true` signals that it is flaky. CI tooling SHOULD track flaky pass rates over time and flag specs that consistently require retries.

---

## 8B. Parallel Execution Semantics

Specs within a framework SHOULD be independently executable with no implicit ordering dependencies. This enables parallel execution for faster CI feedback.

### Default: Independent Specs

Unless a framework explicitly declares otherwise, runners SHOULD treat all specs as independent and support parallel execution:

- `--parallel <N>` — execute up to N specs concurrently (default: 1 = sequential).
- Each spec gets its own execution context. No shared state between specs.
- Report output is collected from all parallel executions and merged into a single report.

### Sequential Execution Override

If a framework genuinely requires ordered execution (e.g., a spec that creates a resource must run before a spec that reads it), the framework schema MUST declare:

```json
{ "execution_mode": "sequential" }
```

This is an explicit opt-out from parallelism. The governance matrix SHOULD record `execution_mode` for each framework. Frameworks that require sequential execution SHOULD document why and SHOULD work toward removing the ordering dependency.

### Timing Data

Runners SHOULD include per-spec timing in the report (the `duration_ms` field) to enable parallelism analysis. A spec that takes 10x longer than peers is a candidate for optimization or splitting.

---

## 8C. Tag Filtering Standard

All UxTS runners MUST support tag-based spec filtering using a consistent CLI syntax. Tags enable selective execution — running only smoke tests in a pre-commit hook, or only regression tests in nightly CI.

**Tag declaration in specs:**

Tags are an array of case-insensitive strings in the spec's metadata or API section:
```json
{ "tags": ["smoke", "health", "p0"] }
```

**CLI syntax:**

| Flag | Behavior |
|------|----------|
| `--tags smoke` | Run only specs that have the `smoke` tag |
| `--tags smoke,regression` | Run specs that have `smoke` OR `regression` (union) |
| `--exclude-tags slow,flaky` | Run all specs EXCEPT those tagged `slow` or `flaky` |
| `--tags smoke --exclude-tags flaky` | Run `smoke` specs, but exclude any also tagged `flaky` |

**Semantics:**

- Tags are case-insensitive: `Smoke`, `SMOKE`, and `smoke` are equivalent.
- `--tags` is inclusive (union): a spec matches if it has ANY of the specified tags.
- `--exclude-tags` is exclusive: a spec is excluded if it has ANY of the specified tags.
- When both are specified: include filter applies first, then exclude filter.
- A spec with no tags matches `--tags` only if no `--tags` filter is specified. It never matches `--exclude-tags`.
- Filtered-out specs are reported with `status: "skip"` in the report.

---

## 9. Framework Maturity Lifecycle

Frameworks progress through three statuses. Each transition has explicit criteria. Gate mode (`soft`, `block`, `observe`) is a separate axis that governs CI behavior within a status — it is not a status itself.

```
spec-only ──→ pilot ──→ active
                          │
                          ├── gate_mode: soft (default on promotion)
                          └── gate_mode: block (after demonstrated stability)
```

### spec-only
- Schema and specs exist.
- No runner, no CI, no automation.
- Specs are documentation of intent, not executable verification.
- **Transition to pilot requires:** functional runner, at least one passing spec.

### pilot
- Schema, specs, and runner all exist and function.
- CI may exist but is not gating (`observe` or `soft` mode).
- Schema-runner parity may be incomplete.
- **Transition to active requires:** 100% schema-runner parity (every schema field classified as `enforced`, `advisory`, or hard-fail `unimplemented`), CI gate enabled at minimum `soft`, documented authority scope, no critical false-pass paths.

### active
- Full schema-runner parity. Every schema field is enforced or detected-and-failed.
- CI gate is operational.
- Spec set has meaningful coverage (not just one trivial spec).
- Newly promoted frameworks enter `active` with `gate_mode: soft`. This allows a stabilization period where failures are visible but do not block the pipeline.
- **Gate mode promotion to `block` requires:** demonstrated stability (no flaky false-fails over a meaningful window), low false-pass rate, sufficient spec coverage for the domain. Once promoted to `block`, failures stop the pipeline.

### deprecated
- Framework is superseded or no longer relevant.
- Existing specs are archived. Runner may be removed.
- Migration path to successor framework must be documented.

### Status vs Gate Mode

These are independent axes. Do not conflate them.

| Axis | Values | What it governs |
|------|--------|-----------------|
| **Status** | `spec-only`, `pilot`, `active`, `deprecated` | Framework maturity (does it have a schema? runner? parity?) |
| **Gate mode** | `observe`, `soft`, `block` | CI behavior (does failure stop the pipeline?) |

A framework's status determines what infrastructure must exist. Its gate mode determines how CI responds to failures. An `active` framework can be `soft`-gated (reporting failures without blocking) or `block`-gated (failures stop the pipeline). A `pilot` framework can be `observe`-gated (metrics only) or `soft`-gated.

### 9.1 Governance Tiers

The full UxTS governance artifact set (Section 12) is appropriate for codebases with 3 or more frameworks. For smaller projects, the overhead can deter adoption. UxTS defines two governance tiers to match overhead to scale.

**Lite Governance** (1-2 frameworks):

Required artifacts:
- Schema, specs, runner, Makefile target, CI step — per the canonical framework contract (Section 7).
- A single `UXTS_README.md` that combines framework inventory, parity status, and authority scope.
- Per-framework run reports (the canonical report schema, Section 8A).

NOT required:
- `FRAMEWORK_GOVERNANCE.md` (combined into README).
- `UXTS_FRAMEWORK_MATRIX.md` (combined into README).
- Gap assessments, discovery artifacts, opportunity backlogs, remediation plans, waivers.
- Aggregate reports.

Transition trigger: When the third framework reaches `pilot` status, the project MUST transition to full governance. The agent SHOULD proactively suggest this transition when the second framework is created.

**Full Governance** (3+ frameworks):

All artifacts in Section 12 are required. All normative requirements in this specification apply without exception.

**Decision rule for agents:**

1. Count frameworks at `pilot` or higher status.
2. If count <= 2: apply lite governance.
3. If count >= 3: apply full governance.
4. If operating in `brownfield` mode with existing frameworks that collectively span 3+ domains: apply full governance regardless of current UxTS framework count.

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

### 10.8 The Environment Leak

A spec hardcodes `"base_url": "http://localhost:9999"`. It passes locally and in CI (where port 9999 is mapped). A developer runs it against staging, which uses port 443. The spec fails — not because the API is broken, but because the URL is wrong. Worse: a spec hardcodes a staging URL and accidentally runs production traffic during tests.

**Fix:** Environment variable resolution (Section 5.3). All deployment-specific values use `${VAR}` syntax. Runners hard-fail on unresolved variables rather than passing literal `${VAR}` strings. The canonical guard script flags specs with hardcoded URLs, ports, or hostnames.

### 10.9 The Duplication Explosion

A framework has 80 API specs. Each one repeats the same `base_url`, `timeout_ms`, auth header, and retry config. A developer needs to change the timeout from 15000 to 30000 — they update 60 specs and miss 20. The 20 unchanged specs now have a different timeout, and nobody notices because they still pass.

**Fix:** Spec composition via `_defaults.json` (Section 5.4). Shared configuration lives in one file. Individual specs contain only their unique contract (path, method, assertions). Changes to shared config happen once and propagate to all specs.

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
| `UXTS_DISCOVERY_<date>.md` | Discovery evidence, operating mode declaration, recurring construct inventory | Every brownfield review and initial greenfield bootstrap |
| `UXTS_OPPORTUNITY_BACKLOG_<date>.md` | Ranked recurring-construct opportunities with scoring rubric | After discovery; update when scores/status change |
| `UXTS_REMEDIATION_PLAN_<date>.md` | Prioritized implementation plan from backlog to framework changes | After backlog creation; update as work completes |
| `UXTS_REMEDIATION_WAIVERS_<date>.md` | Approved blockers for unimplemented high-priority opportunities | Only when execution requirement cannot be met |

Machine-readable reports (produced by runners per the canonical report schema in Section 8A):
- Per-framework run reports (e.g., `/tmp/api-report.json`) — produced by `--report` flag on every runner invocation.

Optional aggregated reports:
- `reports/schema_runner_coverage.json` — field-level parity data per framework.
- `reports/framework_ci_gating.json` — CI gate status per framework.
- `reports/hash_coverage.json` — hash field coverage across all frameworks.
- `reports/uxts_aggregate_report.json` — cross-framework aggregate (see below).

### 12.1 Aggregate Report Schema (Full Governance)

In full governance mode (3+ frameworks), an aggregate report SHOULD be produced after all per-framework runners complete. This enables repository-level governance decisions.

```json
{
  "timestamp": "2026-02-27T15:00:00Z",
  "governance_tier": "full",
  "operating_mode": "brownfield",
  "frameworks": [
    {
      "acronym": "uats",
      "status": "active",
      "gate_mode": "block",
      "schema_version": "1.2.0",
      "summary": {
        "total_specs": 124, "passed": 122, "failed": 1,
        "skipped": 1, "errors": 0, "flaky_passes": 2
      },
      "integrity": {
        "total_hashed": 123, "verified": 121,
        "mismatched": 2, "no_hash": 1
      },
      "parity": {
        "enforced": 14, "advisory": 3, "unimplemented": 0
      }
    }
  ],
  "governance_health": {
    "total_frameworks": 5,
    "active_frameworks": 3,
    "pilot_frameworks": 1,
    "deprecated_frameworks": 1,
    "overall_pass_rate": 97.2,
    "overall_integrity_rate": 98.1,
    "parity_complete": true,
    "drift_clean": true
  },
  "cross_framework_issues": [
    "Fixture tests/fixtures/sample.go referenced by both UPTS and USTS with different hashes"
  ]
}
```

The `governance_health` section provides a single-glance status. CI can use `parity_complete` and `drift_clean` as repository-level gates.

### 12.2 Fixture Governance

Fixtures are test inputs that have their own lifecycle. For frameworks that use fixtures:

- Fixtures SHOULD include creation metadata as a sidecar file (`<fixture>.meta.json`) containing:
  - `source`: How the fixture was generated (manual, extracted from production, generated by tool).
  - `created_at`: ISO 8601 timestamp.
  - `refresh_command`: Command to regenerate the fixture (if applicable).
  - `stale_after_days`: Optional threshold for staleness warnings.

- The drift checker (Section 13) SHOULD flag fixtures older than `stale_after_days` as warnings.
- Fixture refresh commands SHOULD be documented in the governance matrix or framework README.

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
5. **Brownfield artifacts** — when operating mode is `brownfield`, required artifacts (`UXTS_DISCOVERY_*`, `UXTS_OPPORTUNITY_BACKLOG_*`, `UXTS_REMEDIATION_PLAN_*`) exist and are not stale.
6. **Schema version consistency** — every spec's declared version is compatible with the framework's current schema version (Section 7A).
7. **Defaults file validity** — if a `_defaults.json` exists, it parses as valid JSON and contains the `_uxts_defaults` marker (Section 5.4).
8. **Secret leak scan** — no spec file in `specs/` contains patterns matching plaintext secrets (high-entropy strings, JWT patterns, PEM blocks). Report as warning (Section 5.5).
9. **Fixture staleness** — fixtures with `stale_after_days` metadata that have exceeded their threshold. Report as warning (Section 12.2).
10. **Environment variable completeness** — all `${VAR}` references in specs either have defaults (`:-`) or are documented in the framework README as required environment variables.

Both scripts should be wired into CI via Makefile targets (the same portable pattern used for framework runners). The canonical guard should use `block` gate mode. The drift checker can start as `soft` and promote to `block` once stable.

---

## 14. Threat Model

For each framework, the agent MUST evaluate and document:

| Risk | Description | Mitigation |
|------|-------------|------------|
| **False pass** | Spec passes with zero real assertions, or runner ignores assertions | Schema-runner parity, 0/0 prohibition |
| **False fail** | Spec fails due to schema drift, environment sensitivity, or flaky assertions | Deterministic baselines, environment-variable resolution, fixture stability |
| **Coverage blind spot** | Domain has verification needs but no specs, or specs exist but no CI gate | Framework discovery audit, CI drift checker |
| **Undetected changes** | Hash fields missing or not scanned; file modifications go unnoticed | Cross-framework hash scanner, `integrity` summary in reports |
| **Fixture drift** | Fixture modified without developer review; test validates unreviewed inputs | Fixture hash in spec, `hash_mismatches` in report, CI integrity gate |
| **Dialect split** | Multiple incompatible spec formats under one framework | Canonical guard, drafts separation |
| **Environment leak** | Hardcoded URLs/ports cause failures in different environments or accidental production traffic | Environment variable resolution (Section 5.3), canonical guard scan |
| **Schema migration gap** | Schema version changes but existing specs are not migrated, causing silent validation failures | Schema versioning (Section 7A), runner version compatibility check |
| **Secret exposure** | Plaintext secrets committed in spec files | Secret handling rules (Section 5.5), canonical guard secret scan |

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
10. Operating mode is explicitly declared (`greenfield` or `brownfield`) with evidence (Section 6.0).
11. In `brownfield` mode, deterministic discovery command outputs are captured and referenced (Section 6.1.1).
12. In `brownfield` mode, a scored opportunity backlog exists and is current (Section 6.4).
13. In `brownfield` mode, at least one high-priority opportunity is implemented to `pilot` (or higher) in the current engagement, or an explicit waiver exists with owner and due date (Section 6.4).
14. Any new framework created in `brownfield` mode includes documented extension-first justification (Section 6.1, Step 2).
15. Every framework schema has a declared version, and every spec declares which schema version it targets (Section 7A).
16. Environment variable references in specs use canonical `${VAR}` syntax and are not hardcoded deployment-specific values (Section 5.3).
17. No spec file contains plaintext secrets (Section 5.5).
18. Governance tier is appropriate to framework count — lite for 1-2 frameworks, full for 3+ (Section 9.1).
19. Runners support canonical assertion operators for all assertion types used in specs, or report parity failures for unsupported operators (Section 5.2).
20. Tag filtering is available on all runners with `--tags` and `--exclude-tags` flags (Section 8C).

---

## 16. Tooling Ecosystem and Adoption Pathway

UxTS governance is only as useful as the tooling that makes it practical. This section defines the recommended tooling ecosystem and adoption pathway for new projects.

### 16.1 Reference CLI Tools

The following CLI tools SHOULD be provided as part of a UxTS tooling distribution. All tools SHOULD be written in Python (compatible with `uv` and `ruff` toolchains) and distributed as a single installable package.

| Tool | Purpose | Priority |
|------|---------|----------|
| `uxts validate` | Validate all specs against their schemas across all frameworks | Critical |
| `uxts drift` | Run the drift checker (Section 13.2) — compare on-disk reality to governance docs | Critical |
| `uxts hash` | Compute/verify hashes using the canonical procedure (Section 5.1) | Critical |
| `uxts report` | Aggregate per-framework reports into a governance dashboard (Section 12.1) | High |
| `uxts init` | Scaffold a new framework (schema template, example spec, runner skeleton, Makefile target) | High |
| `uxts migrate` | Run schema migration scripts for a framework (Section 7A) | Medium |
| `uxts parity` | Scan a runner's source code for field handling and generate a parity table draft | Medium |
| `uxts secret-scan` | Scan spec files for potential plaintext secrets (Section 5.5) | Medium |

**CLI design conventions:**
- All tools use the `uxts` namespace: `uxts <subcommand> [options]`.
- All tools support `--help`, `--verbose`, and `--json` (machine-readable output) flags.
- All tools return exit code 0 on success, 1 on failure, 2 on usage error.
- All tools are idempotent — running them twice with the same inputs produces the same results.

### 16.2 Quick-Start Bootstrap

For a developer starting from zero, the recommended bootstrap sequence is:

```bash
# 1. Initialize a single framework (creates schema, example spec, runner, Makefile target)
uxts init --framework api --lang python --ci github-actions

# 2. Write baseline specs from live code (Section 6.2, Step 2)
#    The init command generates a runner with a `generate` subcommand:
python runners/uats_runner.py generate --base-url http://localhost:9999 \
    --endpoints /healthz /api/v1/status --output specs/

# 3. Validate specs against schema
uxts validate

# 4. Run specs locally
make test-api BASE_URL=http://localhost:9999

# 5. Commit and push — CI runs automatically
git add . && git commit -m "feat: add UATS framework" && git push
```

Time to first governed framework: under 10 minutes.

### 16.3 OpenAPI Integration

For codebases with existing OpenAPI (Swagger) specifications, UATS specs can be generated from OAS definitions:

```bash
# Generate UATS specs from an OpenAPI spec
uxts init --from-openapi openapi.yaml --framework api
```

This reads the OAS paths, methods, and response schemas, and generates one UATS spec per endpoint with:
- `request.method` and `request.path` from the OAS path item.
- `expected.status` from the OAS response codes.
- `expected.body_assertions` from the OAS response schema (structural type checks).

The generated specs are a starting point — developers should add semantic assertions (specific values, business logic checks) that OAS structural schemas cannot express.

UxTS consumes OpenAPI; it does not compete with it. OpenAPI describes what an API *can* do. UxTS specs describe what an API *must* do in specific scenarios. Both are valuable, and they are complementary.

### 16.4 Integration with Existing Test Frameworks

UxTS is not a replacement for unit tests, integration tests, or end-to-end tests. It governs a specific class of verification: **declarative contract tests that can be expressed as data**. The relationship to existing test infrastructure:

| Test type | Owned by | Relationship to UxTS |
|-----------|----------|---------------------|
| Unit tests | Language-native test framework (pytest, Jest, Go test) | Independent — UxTS does not govern unit tests |
| Integration tests | Custom scripts or test frameworks | May overlap — consider converting recurring patterns to UxTS specs |
| E2E tests | Playwright, Cypress, Selenium | Independent — UxTS does not govern UI tests |
| Contract tests (API) | UxTS (UATS) | Governed by UxTS |
| Contract tests (parser) | UxTS (UPTS) | Governed by UxTS |
| Benchmark tests | UxTS (UBTS) or dedicated tools (k6, locust) | UxTS governs the contract; the benchmark tool is the runner |
| Security tests | UxTS (USTS) or dedicated scanners | UxTS governs behavioral contracts; scanners are complementary |

The key question: **is this verification need recurring and expressible as a declarative contract?** If yes, it's a UxTS candidate. If it requires imperative logic, state management, or user interaction, it belongs in a traditional test framework.
