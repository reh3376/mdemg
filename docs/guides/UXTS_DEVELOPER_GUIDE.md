# UxTS Developer Guide

**Universal-x Test Specification — A Comprehensive Reference for Declarative Verification Governance**

Version: 1.0.0
Date: 2026-03-10
Audience: Software engineers building and maintaining modular systems, and AI coding agents operating in governed codebases.

---

## Table of Contents

- [1. Executive Summary](#1-executive-summary)
- [2. The Problem: Why Ad-Hoc Testing Fails at Scale](#2-the-problem-why-ad-hoc-testing-fails-at-scale)
- [3. Goals: What UxTS Achieves](#3-goals-what-uxts-achieves)
- [4. The Need: Why Large Modular Projects Require This](#4-the-need-why-large-modular-projects-require-this)
- [5. Architecture: The Four-Layer Model](#5-architecture-the-four-layer-model)
- [6. The Framework Inventory](#6-the-framework-inventory)
- [7. Schema-Runner Parity: The Critical Invariant](#7-schema-runner-parity-the-critical-invariant)
- [8. Hash Integrity: Change Detection](#8-hash-integrity-change-detection)
- [9. Writing Specs: A Practical Guide](#9-writing-specs-a-practical-guide)
- [10. Running Specs: The Runner Contract](#10-running-specs-the-runner-contract)
- [11. CI Integration](#11-ci-integration)
- [12. Anti-Patterns: Lessons Learned](#12-anti-patterns-lessons-learned)
- [13. Creating a New Framework](#13-creating-a-new-framework)
- [14. Framework Lifecycle Management](#14-framework-lifecycle-management)
- [15. Governance Artifacts](#15-governance-artifacts)
- [16. Brownfield Adoption](#16-brownfield-adoption)
- [17. Threat Model](#17-threat-model)
- [18. Acceptance Criteria](#18-acceptance-criteria)
- [Appendix A: UATS Schema Reference](#appendix-a-uats-schema-reference)
- [Appendix B: UPTS Schema Reference](#appendix-b-upts-schema-reference)
- [Appendix C: Quick Reference Card](#appendix-c-quick-reference-card)
- [Appendix D: Glossary](#appendix-d-glossary)
- [Appendix E: Reference Documents](#appendix-e-reference-documents)

---

# Layer 1: Vision & Motivation

---

## 1. Executive Summary

UxTS (Universal-x Test Specification) is a methodology for organizing programmatic verification into domain-specific frameworks that share a common architecture. The "x" is a wildcard — each framework replaces it with a letter representing its concern domain: UATS for API contracts, UPTS for parser conformance, UBTS for benchmarks, and so on.

UxTS solves three compounding problems that emerge in every non-trivial codebase: **silent drift** (behavior changes that slip past hand-written tests because there is no diffable contract), **false confidence** (test suites that report "all passing" while silently ignoring half their assertions), and the **reinvention tax** (each new verification concern spawning its own ad-hoc format, runner, and CI approach).

The solution is a single repeating pattern — **declarative specs validated by executable runners governed by explicit schemas** — applied uniformly across every concern domain. Each domain gets its own framework with its own schema and runner, but all frameworks share the same structural contract, lifecycle, governance rules, CI integration pattern, and canonical report format.

This guide is the authoritative reference for understanding, adopting, and extending UxTS. It starts from high-level vision and progressively deepens into technical specifics. Every concept is illustrated with real examples from the MDEMG codebase, where 11 UxTS frameworks govern API contracts, parser conformance, benchmarks, security, observability, authentication, hash integrity, gRPC contracts, semantic validation, and LLM emergence quality.

After reading this guide, you will understand: why declarative test specifications outperform imperative test scripts at scale; how the four-layer architecture (schema, specs, runner, CI gate) works; how to write, run, and govern specs; how to create new frameworks; and how to adopt UxTS in an existing codebase.

---

## 2. The Problem: Why Ad-Hoc Testing Fails at Scale

Every non-trivial codebase accumulates recurring verification needs: API endpoints need contract tests, parsers need conformance checks, observability configs need structure validation, benchmarks need regression gates. Without a unifying methodology, each concern spawns its own ad-hoc test system — different formats, different runners, different CI wiring, different standards for what "passing" means.

This creates three compounding failures.

### 2.1 Silent Drift

When tests are hand-written scripts rather than declarative contracts, behavior changes slip through because the test and the code can diverge without anyone noticing. There is no single artifact that says "this endpoint MUST return this shape" that can be diffed, reviewed, and machine-validated.

Consider an imperative test:

```python
# Imperative: test logic buried in code
def test_health_endpoint():
    resp = requests.get(f"{BASE_URL}/healthz")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
```

If someone adds a new required field to the health response, this test still passes — it only checks two things. The test and the implementation have silently drifted apart. No reviewer can see the gap by looking at either artifact alone.

Now consider the declarative equivalent:

```json
{
  "uats_version": "1.0.0",
  "api": { "name": "Health Check", "base_url": "${MDEMG_BASE_URL}" },
  "request": { "method": "GET", "path": "/healthz" },
  "expected": {
    "status": 200,
    "body_assertions": [
      { "path": "$.status", "op": "equals", "expected": "ok" }
    ]
  }
}
```

This is a **contract** — a diffable artifact that explicitly declares what the endpoint must do. When behavior changes, the spec diff shows exactly what changed. A PR that modifies the endpoint and updates the spec makes the behavioral change visible in code review.

### 2.2 False Confidence: The 0/0 Problem

A test suite that reports "all passing" but silently ignores half its assertions is worse than no tests at all. It provides the illusion of coverage.

This happens when runners don't enforce every field in the spec, when schemas define capabilities runners don't implement, or when specs are written but never wired into CI.

**Real example from the MDEMG codebase:** An observability test framework (UOBS) reported 100% pass rate on specs with zero executable assertions. The runner implemented `health` and `metrics` check types but not `logging`. Specs that only defined logging checks had zero assertions to evaluate — so they passed with 0 failures out of 0 checks. The formula `0/0 = pass` is mathematically undefined but practically catastrophic: it told the team everything was verified when nothing was.

This was discovered during the UxTS Framework Gap Assessment (February 2026) and classified as a **Critical** finding. The fix was simple but fundamental: runners must require at least one executable assertion per spec. A spec with zero executable assertions must **fail**, not pass.

### 2.3 The Reinvention Tax

When each concern domain invents its own format, runner, and CI approach, adding a new domain requires solving all the same problems again: how to define specs, how to validate them against a schema, how to run them, how to gate CI, how to manage integrity. The overhead compounds with every new concern.

In the MDEMG project, before UxTS standardization, adding a new verification domain meant:

1. Inventing a spec format (JSON? YAML? Custom?)
2. Writing a runner from scratch
3. Figuring out CI integration independently
4. Defining what "passing" means differently each time
5. Building report infrastructure for that specific domain
6. Creating governance documentation in a different structure

After UxTS, adding a new domain means instantiating the same pattern with a new schema and runner. The spec format is JSON. The runner produces the canonical report. CI integration follows the Makefile pattern. "Passing" means the same thing everywhere. Report infrastructure is shared (`uxts_report.py`, `uxts_runner_core.py`).

But the deeper payoff is **uniformity within each framework**. Every UATS spec — all 129 of them — has the same structure: `api`, `request`, `expected`, `body_assertions`. A script that processes one UATS spec can process every UATS spec. A tool that generates one UPTS spec from parser output can generate all 27. This uniformity is what makes the framework programmatically tractable: you write the tooling once, and it works across every endpoint, every parser, every benchmark. Exceptions exist (variants, optional fields, tag-filtered execution), but the structural contract holds. Without uniformity, each spec is a snowflake that requires custom handling — with it, specs are interchangeable units that tooling can create, validate, diff, and aggregate at scale.

### 2.4 The Compound Effect

These three failures are not independent — they compound. Silent drift means you don't know what's tested. False confidence means you think you know. Reinvention tax means fixing the problem in one domain doesn't help the others.

The result is a codebase where:

- Nobody can answer "what exactly do our tests verify?" with confidence
- Adding verification for a new concern takes hours instead of minutes
- Every developer's tests work differently, and new team members must learn each approach
- AI coding agents cannot systematically contribute to test coverage because there is no discoverable, machine-readable standard

UxTS solves all three by defining a single pattern that applies uniformly across every concern domain. Each domain gets its own framework, but all frameworks share the same structural contract.

---

## 3. Goals: What UxTS Achieves

### 3.1 Declarative Contracts as Source of Truth

Every verification need is expressed as a declarative JSON spec — data, not code. Specs can be:

- **Diffed** in code review, making behavioral changes visible
- **Generated** by tools or AI agents from live system behavior
- **Validated** against schemas to catch structural errors before execution
- **Versioned** alongside the code they verify
- **Queried** programmatically (e.g., "how many specs cover the /v1/conversation/* endpoints?")

The spec is the contract. If the spec says `GET /healthz` returns `200` with `{"status": "ok"}`, that is the verifiable truth. The runner's job is to confirm or deny it. The CI gate's job is to enforce it.

### 3.2 Schema-Governed Correctness

Every spec is validated against a JSON Schema before execution. The schema defines:

- What fields are legal in a spec
- What values each field can take
- What fields are required vs. optional
- What assertion types are supported

This prevents spec rot — the gradual accumulation of fields that nothing validates. Without a schema, specs diverge from what runners actually check, creating the illusion of thorough coverage. With a schema, every field is intentional.

More importantly, the schema-runner parity invariant (Section 7) ensures that every field defined in the schema is either **enforced** by the runner, classified as **advisory**, or causes a **hard failure** if used but unimplemented. There are no silently ignored fields.

### 3.3 Reproducible Verification

Specs are deterministic inputs. Given the same spec and the same system state, the runner produces the same result. This means:

- Local runs match CI runs (no "works on my machine" for test definitions)
- Results are reproducible across environments (base URL is parameterized, not hardcoded)
- Debugging a failure starts with reading the spec, not reverse-engineering test code

The canonical report format (Section 10) ensures that results from any framework are machine-readable in the same structure, enabling cross-framework dashboards and trend analysis.

### 3.4 Horizontal Governance Scaling

Once you have the pattern (schema + spec + runner + CI) for one domain, adding a new domain is mechanical. You don't reinvent test infrastructure — you instantiate the same pattern with a new schema and runner.

This is how a single codebase can govern 11 different verification domains without 11 different test philosophies. The governance rules are the same everywhere: schema-runner parity, hash integrity, canonical reports, CI gates, framework lifecycle management.

### 3.5 Hash-Backed Change Detection

Spec files and fixture files carry SHA256 hashes. If a spec is modified — intentional edit, merge conflict resolution, tooling side-effect — hash verification detects the change. The hash answers "was this file modified since last review?" not "is this file correct?" Correctness is the assertions' job.

Hash integrity matters most for high-stakes specs (security, benchmark baselines) where an unreviewed modification could mask a regression. The runner always verifies hashes and always executes assertions — these are independent operations reported separately in the canonical report.

This becomes especially critical when AI coding agents are modifying the codebase. An agent operating without full project context may change a spec or fixture as a side-effect of a broader task — adjusting a response field, reformatting JSON, resolving a merge conflict — without understanding that the change invalidates a verification contract. Without hash detection, that modification silently alters what the spec verifies, and nobody notices until a regression slips through. With hashes, the next runner execution flags the mismatch immediately: "this file changed since it was last reviewed." The human (or a supervising agent) can then inspect the diff and decide whether the change was intentional and correct, or an uninformed modification that needs to be reverted. In a workflow where agents routinely touch dozens of files per task, hash-backed change detection is the safety net that catches the changes you didn't know to look for.

### 3.6 Programmatic Uniformity

This is the benefit that compounds all the others. Within a framework, every spec has the same structure. Every UATS spec has `api`, `request`, `expected`. Every UPTS spec has `language`, `fixture`, `expected.symbols`. The structural contract is identical whether you are testing a health endpoint or a complex multi-variant conversation API.

This uniformity makes specs **programmatically tractable** in ways that ad-hoc tests never are:

- **Bulk generation**: A script that generates one UATS spec from an OpenAPI definition can generate specs for every endpoint. A script that captures parser output and encodes it as a UPTS spec can do so for every language. The generation logic is write-once because the output format is uniform.
- **Bulk validation**: `validate-all --spec-dir` works because every spec in the directory conforms to the same schema. No per-spec dispatch logic, no format sniffing, no special cases.
- **Bulk analysis**: "Which endpoints lack body assertions?" is a one-liner JSONPath query across all UATS specs. "Which parsers don't validate parent relationships?" is a one-liner across UPTS specs. Ad-hoc tests make such queries impossible because each test has a different shape.
- **Bulk transformation**: Migrating all specs from legacy dialect to canonical dialect is a mechanical transformation — the input and output structures are known. Migrating ad-hoc tests requires understanding each test's bespoke logic.
- **Tooling reuse**: The UATS runner, report builder, hash verifier, drift checker, and CI integration all work for any UATS spec — whether it tests `/healthz` or `/v1/memory/meta-learn`. You build the tooling once and it scales to every spec in the framework.

Exceptions exist — variants add complexity, optional fields create structural variation, tag filtering means not every spec runs in every context. But the structural contract holds: if you can process one spec in a framework, you can process all of them. This is the property that transforms a collection of tests into a **governed system**.

---

## 4. The Need: Why Large Modular Projects Require This

### 4.1 Multiple Contributors with No Shared Standard

In any project with more than one contributor, ad-hoc testing creates fragmentation. Developer A writes pytest assertions. Developer B writes shell scripts. Developer C writes Go table-driven tests. Each approach works in isolation, but the aggregate test suite has no discoverability, no shared reporting, and no way to answer "what's actually verified?"

UxTS provides the shared standard: every verification concern is expressed as a JSON spec conforming to a schema, executed by a runner that produces a canonical report, gated in CI through a Makefile target. Any developer (or agent) who understands this pattern can contribute to any framework.

### 4.2 AI Agents as First-Class Contributors

AI coding agents — Claude Code, Copilot, Cursor, custom agents — are increasingly responsible for writing and maintaining tests. Without a machine-readable standard, agents must reverse-engineer each project's testing conventions from context clues.

UxTS gives agents a discoverable, deterministic interface:

- **Schemas** tell the agent what fields are valid
- **Existing specs** provide examples of the expected format
- **Uniformity** means a single example is sufficient — every spec in the framework has the same structure, so the agent doesn't need to learn 129 different patterns for 129 endpoints
- **Runners** validate the agent's output immediately
- **CI gates** catch mistakes before they reach production

The uniformity point deserves emphasis. An agent asked to "add a UATS spec for the new `/v1/jobs/status` endpoint" can read any one existing UATS spec, understand the structure, and produce a correct spec for the new endpoint. It doesn't need to study a codebase-specific test framework, find the right test file to modify, or understand imperative test helper functions. The spec format is the same whether the endpoint returns a simple status string or a complex nested object — only the `request` and `expected` fields differ.

The UxTS Portable Agent Specification (`docs/specs/UXTS_PORTABLE_AGENT_SPEC.md`) codifies this explicitly: it tells agents how to discover frameworks, write specs, validate them, and contribute to governance — all from the same source of truth that human developers use.

### 4.3 The Cost of Not Knowing What's Tested

When you cannot answer "what percentage of our API endpoints have contract tests?" with a precise number, you are operating on faith. UxTS makes this answerable:

- Count spec files in `docs/api/api-spec/uats/specs/`: that's your API contract coverage
- Read the runner report: that's your pass rate
- Check the governance matrix: that's which frameworks are active and which are gaps

In the MDEMG project, the answer is precise: 129 UATS specs covering API endpoints, 27 UPTS specs covering language parsers, 8 UETS specs covering emergence quality, 5 UOTS specs covering observability artifacts, and so on. Each number is verifiable by counting files on disk.

### 4.4 Cross-Concern Verification Needs

Modern systems have verification needs that span multiple concerns:

- An API endpoint that must return correct data (UATS) **and** expose Prometheus metrics (UOTS) **and** respect auth boundaries (USTS)
- A parser that must extract correct symbols (UPTS) **and** meet latency budgets (UBTS)
- A service that must pass health checks (UOBS) **and** have correct dashboard definitions (UOTS)

Without a shared methodology, these cross-concern verifications live in different test systems with different formats, different pass criteria, and different CI gates. UxTS unifies them under one governance structure while keeping each concern in its own framework with its own schema.

### 4.5 The Complete Framework Inventory

UxTS currently defines 11 frameworks, each owning one verification concern:

| Acronym | Full Name | Domain | Status | Specs |
|---------|-----------|--------|--------|-------|
| **UATS** | Universal API Test Specification | HTTP endpoint acceptance contracts | active | 129 |
| **UPTS** | Universal Parser Test Specification | Language parser conformance | active | 27 |
| **UBTS** | Universal Benchmark Test Specification | Throughput/latency/load regression | active | 3 |
| **UETS** | Universal Emergence Test Specification | LLM concept-naming quality | active | 8 |
| **UOBS** | Universal Observability Specification | Runtime observability behavior | active | 3 |
| **UOTS** | Universal Observability Test Specification | Artifact-level observability contracts | active | 5 |
| **UDTS** | Universal DevSpace Test Specification | gRPC contract tests | active | 7 |
| **UNTS** | Universal Hash Test Specification | Hash integrity registry | active | N/A (registry) |
| **USTS** | Universal Security Test Specification | Security behavior and hardening | pilot | 3 |
| **UAMS** | Universal Auth Method Specification | Auth method contracts | spec-only | 4 |
| **UVTS** | Universal Validation Test Specification | Semantic retrieval quality | spec-only | 1 |

The naming convention is `U<X>TS` where `<X>` identifies the domain. You are not limited to these letters — any recurring verification pattern can become a framework. The methodology is the pattern, not the specific letters.

---

# Layer 2: Architecture

---

## 5. Architecture: The Four-Layer Model

### 5.1 Overview

Every UxTS framework has exactly four layers. Data flows upward: the schema constrains what specs can say, the runner interprets what specs say, and CI decides when the runner executes.

```
┌──────────────────────────────────────────────────────────────┐
│  Layer 4: CI Gate                                            │
│  Automation: when and how tests run                          │
│  (GitHub Actions workflow, Makefile target)                   │
│                                                              │
│  Examples:                                                   │
│    ci.yml → "Run UATS Contract Tests"                        │
│    parser-tests.yml → "Run UPTS validation"                  │
│    Makefile → test-api, test-parsers, test-ubts-smoke        │
├──────────────────────────────────────────────────────────────┤
│  Layer 3: Runner                                             │
│  Execution: reads specs, reports results                     │
│  (Python script, Go binary, shell script)                    │
│                                                              │
│  Examples:                                                   │
│    uats_runner.py → validates API endpoints                  │
│    upts_runner.py → validates parser output                  │
│    ubts_runner.py → validates benchmark thresholds           │
├──────────────────────────────────────────────────────────────┤
│  Layer 2: Specs                                              │
│  Contracts: declarative test definitions                     │
│  (*.uats.json, *.upts.json, *.ubts.json, etc.)              │
│                                                              │
│  Examples:                                                   │
│    health.uats.json → "GET /healthz returns 200"             │
│    python.upts.json → "Python parser extracts 31 symbols"   │
│    retrieve_latency.ubts.json → "p99 < 500ms"               │
├──────────────────────────────────────────────────────────────┤
│  Layer 1: Schema                                             │
│  Authority: defines valid spec structure                     │
│  (*.schema.json — JSON Schema files)                         │
│                                                              │
│  Examples:                                                   │
│    uats.schema.json → defines valid UATS spec fields         │
│    upts.schema.json → defines valid UPTS spec fields         │
│    ubts.schema.json → defines valid UBTS spec fields         │
└──────────────────────────────────────────────────────────────┘
```

Each layer is independently versionable and auditable. You can update a schema without touching specs, update specs without touching the runner, or update CI without touching anything else. This separation of concerns is what makes the methodology scalable.

### 5.2 Schema Layer (Layer 1)

The schema is a JSON Schema file that defines the canonical structure of specs within a framework. It is the **source of truth** for what fields are legal, what types they accept, and what values are valid.

**UATS Schema excerpt** (from `docs/api/api-spec/uats/schema/uats.schema.json`):

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://mdemg.dev/schemas/uats/v1.0.0",
  "title": "Universal API Test Schema (UATS)",
  "description": "Language-agnostic test specification for API endpoint validation",
  "type": "object",
  "required": ["uats_version", "api", "request", "expected"],

  "properties": {
    "uats_version": {
      "type": "string",
      "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+$"
    },
    "api": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": { "type": "string" },
        "base_url": { "type": "string" },
        "version": { "type": "string" },
        "service": { "type": "string" },
        "tags": { "type": "array", "items": { "type": "string" } }
      }
    },
    "request": {
      "type": "object",
      "required": ["method", "path"],
      "properties": {
        "method": { "type": "string", "enum": ["GET","POST","PUT","PATCH","DELETE","HEAD","OPTIONS"] },
        "path": { "type": "string" }
      }
    },
    "expected": {
      "type": "object",
      "required": ["status"],
      "properties": {
        "status": { ... },
        "body_assertions": { "type": "array", "items": { "$ref": "#/$defs/BodyAssertion" } }
      }
    }
  }
}
```

Key observations:

- **`required` fields** define the minimum viable spec. A UATS spec must have `uats_version`, `api`, `request`, and `expected`. Everything else is optional.
- **Enum constraints** restrict values to known-good sets. The `method` field only allows standard HTTP methods.
- **Environment variable syntax** (`${MDEMG_BASE_URL}`) is supported in string fields — the runner resolves these at execution time.
- **Nested `$defs`** define reusable types like `BodyAssertion`, `Matcher`, `Step`, and `Variant`.

**UPTS Schema excerpt** (from `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json`):

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://mdemg.dev/schemas/upts/v1.0.0",
  "title": "Universal Parser Test Specification",
  "type": "object",
  "required": ["upts_version", "language", "fixture", "expected"],

  "properties": {
    "upts_version": { "type": "string", "const": "1.0.0" },
    "language": {
      "type": "string",
      "enum": ["c","cpp","csharp","cuda","cypher","dockerfile","dotenv",
               "go","graphql","ini","java","javascript","json","jsonc",
               "kotlin","lua","makefile","markdown","openapi","protobuf",
               "python","rust","scraper-markdown","shell","sql",
               "terraform","toml","typescript","xml","yaml"]
    },
    "fixture": {
      "type": "object",
      "oneOf": [
        { "properties": { "type": {"const":"file"}, "path": {"type":"string"}, "sha256": {"type":"string"} }, "required": ["type","path"] },
        { "properties": { "type": {"const":"inline"}, "content": {"type":"string"} }, "required": ["type","content"] }
      ]
    },
    "expected": {
      "type": "object",
      "required": ["symbols"],
      "properties": {
        "symbol_count": { ... },
        "symbols": { "type": "array", "items": { "$ref": "#/$defs/Symbol" } }
      }
    }
  }
}
```

Notice the structural parallel: both schemas follow the same pattern of `version` + `identity` + `input` + `expected output`. UATS uses `api` + `request` + `expected`. UPTS uses `language` + `fixture` + `expected`. The pattern is the same; the domain-specific details differ.

### 5.3 Specs Layer (Layer 2)

Specs are declarative JSON files — each one defines a single verifiable contract. They are data, not code.

#### Annotated UATS Spec: `health.uats.json`

```json
{
  "uats_version": "1.0.0",                          // Schema version
  "api": {
    "name": "Health Check",                          // Human-readable endpoint name
    "base_url": "${MDEMG_BASE_URL}",                 // Resolved from environment at runtime
    "version": "v1",                                 // API version
    "service": "mdemg",                              // Service identifier
    "tags": ["health", "smoke"]                      // Used for filtering (--include-tag smoke)
  },
  "metadata": {
    "author": "reh3376",                             // Spec author
    "created": "2026-01-29",                         // Creation date
    "description": "Validates Health Check endpoint", // What this spec tests
    "test_type": "contract",                         // contract | integration | smoke | regression
    "priority": "high"                               // critical | high | medium | low
  },
  "config": {
    "timeout_ms": 15000,                             // Request timeout
    "sha256": "5f6e872d...b4b2185"                   // Integrity hash of this spec file
  },
  "variables": {
    "test_space": "demo"                             // Variables available via {{test_space}}
  },
  "request": {
    "method": "GET",                                 // HTTP method
    "path": "/healthz"                               // Endpoint path (appended to base_url)
  },
  "expected": {
    "status": 200,                                   // Expected HTTP status code
    "body_assertions": [                             // Assertions against response body
      {
        "path": "$.status",                          // JSONPath into response
        "op": "equals",                              // Assertion operator (canonical dialect)
        "expected": "ok"                             // Expected value
      }
    ]
  }
}
```

This spec says: "Send `GET /healthz` and verify it returns HTTP 200 with `$.status` equal to `"ok"`." That is the complete contract. The runner's job is to execute it and report the result.

#### Annotated UPTS Spec: `python.upts.json` (abbreviated)

```json
{
  "upts_version": "1.0.0",
  "language": "python",                              // Which parser to invoke
  "variants": [".py", ".pyi"],                       // File extensions this parser handles
  "metadata": {
    "author": "reh3376",
    "description": "Python parser spec - updated to match actual parser output"
  },
  "config": {
    "line_tolerance": 2,                             // Lines can be off by ±2
    "require_all_symbols": false,                    // Missing symbols don't fail
    "allow_extra_symbols": true,                     // Extra symbols are OK
    "validate_parent": false                         // Don't check parent relationships
  },
  "fixture": {
    "type": "file",                                  // Test input is a file on disk
    "path": "../fixtures/python_test_fixture.py",    // Relative path to fixture
    "sha256": "8975b14f...4b11f5"                    // Hash of the fixture file
  },
  "expected": {
    "symbol_count": { "min": 30, "max": 45 },       // Expected range of symbols
    "symbols": [                                     // Specific symbols to verify
      { "name": "MAX_RETRIES", "type": "constant", "line": 12, "exported": true, "value": "3" },
      { "name": "calculate_total", "type": "function", "line": 23, "exported": true },
      { "name": "Status", "type": "enum", "line": 42, "exported": true, "parent": "Enum" },
      { "name": "UserRepository", "type": "interface", "line": 57, "exported": true, "parent": "Protocol" },
      { "name": "UserService", "type": "class", "line": 95, "exported": true },
      { "name": "__init__", "type": "method", "line": 98, "exported": false, "parent": "UserService" },
      { "name": "_private_helper", "type": "function", "line": 153, "exported": false }
      // ... 31 symbols total
    ]
  }
}
```

This spec says: "Parse `python_test_fixture.py` and verify it produces 30-45 symbols, including `MAX_RETRIES` as a constant on line 12 (±2), `Status` as an enum on line 42, and `UserService` as a class on line 95." The fixture hash ensures the input file hasn't been modified since the spec was last reviewed.

### 5.4 Runner Layer (Layer 3)

The runner is the only component that actually **does** anything. Specs and schemas are pure data; the runner reads specs, performs verification, and reports results.

Every runner must support:

| Command | Purpose |
|---------|---------|
| `validate --spec <path>` | Validate a single spec |
| `validate-all --spec-dir <dir>` | Validate all specs in a directory |
| `--base-url <url>` | Override the target URL (UATS) or parser command (UPTS) |
| `--report <path>` | Write structured JSON report to file |
| `--include-tag <tag>` | Only run specs with this tag |
| `--exclude-tag <tag>` | Skip specs with this tag |

**Execution flow** (common to all runners):

```
1. Load spec file from disk
2. Verify spec hash (if config.sha256 present)
   → Record hash_verified: true/false/null
3. Validate spec against JSON Schema
   → Reject malformed specs with status: error
4. Check schema-runner parity
   → Hard fail on unimplemented fields
5. Resolve variables and environment references
   → ${ENV_VAR} → actual values
   → {{variable}} → spec-defined values
6. Execute the verification
   → UATS: send HTTP request, check response
   → UPTS: run parser on fixture, check symbols
7. Evaluate assertions
   → Count assertions_evaluated and assertions_passed
   → If 0 assertions evaluated → FAIL (0/0 protection)
8. Build result using canonical report format
9. Print human-readable summary to stdout
10. Write JSON report to --report path
```

### 5.5 CI Gate Layer (Layer 4)

The CI gate determines **when** and **how** runners execute, and **what happens** when they fail.

**Real CI integration: UATS in `ci.yml`**

```yaml
# From .github/workflows/ci.yml (lines 136-154)

- name: Setup Python for UATS
  uses: actions/setup-python@v6
  with:
    python-version: '3.12'

- name: Install UATS dependencies
  run: pip install requests jsonpath-ng

- name: Create UATS test fixtures
  run: |
    mkdir -p /tmp/uats-test-codebase
    echo 'package main' > /tmp/uats-test-codebase/main.go

- name: Run UATS Contract Tests
  run: |
    python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
      --spec-dir docs/api/api-spec/uats/specs/ \
      --base-url http://localhost:${{ env.MDEMG_PORT }} \
      --exclude-tag unts,llm_required
```

Key points:
- **Tag filtering**: `--exclude-tag unts,llm_required` skips specs that need UNTS infrastructure or LLM access (unavailable in CI)
- **Gate mode: block** — if UATS fails, the CI pipeline fails
- **Environment**: the server is started in a previous step with `EMBEDDING_PROVIDER=stub` to work without a real embedding service

**Real CI integration: UPTS in `parser-tests.yml`**

```yaml
# From .github/workflows/parser-tests.yml (lines 41-74)

- name: Verify UPTS schema parity
  run: python3 scripts/verify_upts_schema_parity.py

- name: Verify fixtures integrity
  run: |
    cd docs/lang-parser/lang-parse-spec/upts/fixtures
    echo "8975b14f...4b11f5  python_test_fixture.py" | shasum -a 256 -c
    echo "a95030aa...8e1e98  go_test_fixture.go" | shasum -a 256 -c
    echo "76778e09...9f49d6  typescript_test_fixture.ts" | shasum -a 256 -c

- name: Run UPTS validation
  run: |
    python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
      --spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
      --parser "./bin/extract-symbols --json" \
      --report parser-report.json
```

Key points:
- **Schema parity check** runs first — catches schema/spec drift before validation
- **Fixture integrity** verified independently with `shasum` before the runner executes
- **Path-triggered**: this workflow only runs when parser-related files change (`paths:` filter)
- **Report artifact** uploaded for post-mortem analysis

**Real CI integration: UBTS (soft-fail)**

```yaml
# From .github/workflows/ci.yml (lines 156-163)

- name: Run UBTS Smoke Benchmark (soft-fail)
  continue-on-error: true
  run: |
    python3 docs/tests/ubts/runners/ubts_runner.py \
      --spec docs/tests/ubts/specs/retrieve_latency.ubts.json \
      --profile docs/tests/ubts/profiles/smoke.profile.json \
      --base-url http://localhost:${{ env.MDEMG_PORT }} \
      --output /tmp/ubts-results/
```

Key point: `continue-on-error: true` implements the **soft-fail** gate mode — benchmark results are visible but don't block the pipeline. This is appropriate for benchmarks where CI environments have different performance characteristics than production.

---

## 6. The Framework Inventory

### 6.1 Naming Convention

Framework names follow the pattern `U<X>TS`:

- **U** = Universal (the methodology prefix)
- **X** = Domain identifier (one or two letters)
- **TS** = Test Specification (the methodology suffix)

| Letter(s) | Domain | Framework |
|-----------|--------|-----------|
| A | API contracts | UATS |
| P | Parser conformance | UPTS |
| B | Benchmarks | UBTS |
| S | Security | USTS |
| O | Observability (artifacts) | UOTS |
| OB | Observability (runtime) | UOBS |
| D | DevSpace / gRPC | UDTS |
| V | Validation quality | UVTS |
| E | Emergence quality | UETS |
| AM | Auth methods | UAMS |
| N | Hash integrity | UNTS |

Spec files use the convention `<name>.<framework>.json`. For example: `health.uats.json`, `python.upts.json`, `retrieve_latency.ubts.json`.

### 6.2 Status Definitions

| Status | Meaning | Requirements |
|--------|---------|-------------|
| **active** | Full verification capability, CI-gated | Schema + specs + runner + CI gate + schema-runner parity |
| **pilot** | Functional but not yet proven at scale | Schema + specs + runner (CI may be observe/soft) |
| **spec-only** | Intent documented, no execution capability | Schema + specs only (no runner, no CI) |
| **deprecated** | Superseded or no longer relevant | Migration plan documented |

### 6.3 Current Inventory

The source of truth for this table is `docs/development/UXTS_FRAMEWORK_MATRIX.md`.

| Framework | Schema | Specs Dir | Runner | CI Gate | Status |
|-----------|--------|-----------|--------|---------|--------|
| **UATS** | `docs/api/api-spec/uats/schema/uats.schema.json` | `docs/api/api-spec/uats/specs/` (129) | `uats_runner.py` v1.1.0 | `ci.yml` (block) | active |
| **UPTS** | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` | `docs/lang-parser/lang-parse-spec/upts/specs/` (27) | `upts_runner.py` | `parser-tests.yml` (block) | active |
| **UBTS** | `docs/tests/ubts/schema/ubts.schema.json` | `docs/tests/ubts/specs/` (3) | `ubts_runner.py` v1.1.0 | `ci.yml` (soft-fail) | active |
| **UETS** | `docs/tests/uets/schema/uets.schema.json` | `docs/tests/uets/specs/` (8) | `uets_runner.py` | No CI gate | active |
| **UOBS** | `docs/tests/uobs/schema/uobs.schema.json` | `docs/tests/uobs/specs/` (3) | `uobs_runner.py` | No CI gate | active |
| **UOTS** | `docs/api/api-spec/uots/schema/uots.schema.json` | `docs/api/api-spec/uots/specs/` (5) | `uots_runner.py` | Makefile target | active |
| **UDTS** | `docs/api/api-spec/udts/schema/udts.schema.json` | `docs/api/api-spec/udts/specs/` (7) | `contract_test.go` | Canonical guard | active |
| **UNTS** | N/A (registry format) | `docs/specs/unts-registry.json` | `internal/unts/` (Go) | No CI gate | active |
| **USTS** | `docs/tests/usts/schema/usts.schema.json` | `docs/tests/usts/specs/` (3) | `usts_runner.py` | No CI gate | pilot |
| **UAMS** | `docs/tests/uams/schema/uams.schema.json` | `docs/tests/uams/specs/` (4) | None | No CI gate | spec-only |
| **UVTS** | `docs/tests/uvts/schema/uvts.schema.json` | `docs/tests/uvts/specs/` (1) | None (stub only) | Canonical guard | spec-only |

### 6.4 Case Study: The UOBS/UOTS Authority Split

A common governance challenge is when two frameworks appear to cover the same domain. In the MDEMG project, both UOBS and UOTS claimed "observability." This caused confusion about where specs should live and which runner should validate them.

The resolution was an explicit **authority split** based on what each framework validates:

| Dimension | UOBS | UOTS |
|-----------|------|------|
| **What it validates** | Live runtime behavior | Static artifact structure |
| **Requires running server** | Yes | No (except `prometheus_metrics`) |
| **Examples** | Health probes, dependency checks, tracing headers | Dashboard JSON, alert rule YAML, metric definitions |
| **Runner approach** | Sends requests to live service | Reads files from disk |

This split is documented in both `FRAMEWORK_GOVERNANCE.md` and `UXTS_FRAMEWORK_MATRIX.md`. When a new observability spec is needed, the author checks: "Does this require a running service?" If yes → UOBS. If no → UOTS.

**Lesson**: Framework overlap must be resolved by defining exactly what each framework owns. Ambiguity causes specs to land in the wrong framework and get validated by the wrong runner.

---

## 7. Schema-Runner Parity: The Critical Invariant

This is the single most important governance concept in UxTS. It is also the most frequently violated.

### 7.1 Why It Matters Most

A schema defines fields like `setup`, `teardown`, `body_schema`, `retry_policy`. A spec uses some of these fields. The runner reads the spec — but if the runner doesn't implement `setup`, it silently ignores the field. The spec "passes" even though the setup step never ran. This is a **false pass**.

False passes are worse than test failures because they create the belief that something is verified when it is not. Every UxTS framework that matured through real use discovered this problem.

### 7.2 The Classification Rule

Every schema field MUST be classified in the runner:

| Classification | Runner Behavior | Allowed in `active` Status? |
|---------------|----------------|---------------------------|
| **enforced** | Field affects pass/fail logic | Yes |
| **advisory** | Field produces a warning but does not affect pass/fail | Yes (for supplementary data) |
| **unimplemented** | Runner detects the field and **hard fails** with explicit error | Only if field is deprecated |

The critical constraint: **unimplemented fields must cause hard failures, not silent skips.** A runner that encounters a field it doesn't implement must refuse to execute the spec, not quietly ignore the field and report "pass."

### 7.3 Implementation Pattern

In the runner, maintain a set of known/handled fields. Before executing any spec, scan for unknown fields:

```python
_KNOWN_FIELDS = {
    "uats_version", "api", "metadata", "config",
    "request", "expected", "variables", "variants", "captures"
}

_UNIMPLEMENTED_FIELDS = {
    "setup", "teardown", "chain", "body_file", "body_schema", "oauth2"
}

def validate_supported_features(spec: dict) -> list[str]:
    """Detect spec fields the runner does not implement."""
    errors = []

    # Check top-level fields
    for key in spec:
        if key in _UNIMPLEMENTED_FIELDS:
            errors.append(
                f"Unimplemented field '{key}': runner cannot validate this spec"
            )
        elif key not in _KNOWN_FIELDS:
            errors.append(
                f"Unknown field '{key}': not in schema or runner"
            )

    # Check nested fields (e.g., auth.oauth2, config sub-keys)
    if "auth" in spec and spec["auth"].get("type") == "oauth2":
        errors.append("Unimplemented auth type 'oauth2'")

    return errors
```

If `validate_supported_features` returns errors, the runner must report the spec as **FAIL** (not SKIP, not WARN), with the unimplemented fields listed in the failure message.

### 7.4 Current Parity Status

This table is the primary audit artifact for schema-runner alignment across all frameworks:

| Framework | Enforced Fields | Unimplemented Fields | Fail-Fast Detection |
|-----------|----------------|---------------------|-------------------|
| **UATS** | Most core fields | `setup`, `teardown`, `chain`, `body_file`, `body_schema`, `oauth2`, several config keys | Yes (hard fail) |
| **UPTS** | `line_tolerance`, `validate_signature`, `validate_value`, `validate_parent`, `require_all_symbols`, `allow_extra_symbols` | `relationships` | Yes (hard fail) |
| **UBTS** | All threshold fields, `min_success_rate`, `max_p99_degradation_pct` | `setup.seed_data`, `ramp_up_seconds`, `duration_seconds` | Yes (warnings) |
| **UETS** | E1-E5 all enforced (including E4 description quality) | None | N/A |
| **UOBS** | `metrics`, `health`, `dependency` | `logging` (draft), `tracing` | Yes (hard fail) |
| **UOTS** | `prometheus_metrics`, `grafana_dashboard`, `alert_rules` | `log_format`, `trace_propagation` | Yes (hard fail) |
| **UDTS** | Hand-coded per RPC | N/A (not spec-driven) | N/A |
| **USTS** | `rate_limiting`, `data_exposure`, `injection` | `authentication` (draft), `test_cases` format | Yes (hard fail) |
| **UAMS** | N/A (no runner) | All | N/A |
| **UVTS** | N/A (no functional runner) | All | N/A |

**Reading this table**: UATS has `setup` in its schema but the runner doesn't implement it. If a spec uses `setup`, the runner detects it and hard-fails. This is correct behavior — better to fail explicitly than to silently skip the setup and report a false pass.

---

## 8. Hash Integrity: Change Detection

### 8.1 What Hashes Solve

Hashes answer one question: **"Was this file modified since it was last reviewed?"**

They do NOT answer "Is this file correct?" — that's the assertions' job. Hashes are a change-detection mechanism. They matter most for:

- **High-stakes specs** (security, benchmark baselines) where an unreviewed modification could mask a regression
- **Fixture files** where a modified input could silently invalidate test results
- **Cross-team collaboration** where one team's tooling might modify another team's specs during merge conflict resolution
- **AI coding agent workflows** where an agent may modify specs or fixtures as a side-effect of a broader task without the context to understand the verification implications

The last point deserves emphasis. AI coding agents operate on files programmatically and at scale. An agent refactoring an API handler might update the corresponding UATS spec to match — but without full project context, it might change the spec in ways that weaken the contract (removing an assertion, broadening a type check, altering a fixture). Without hashes, these changes are invisible until a regression slips through. With hashes, every modification is flagged on the next runner execution, creating a mandatory review checkpoint. Hashes function as a **constraint on agent autonomy**: agents can modify whatever they need to, but they cannot make those modifications invisible.

### 8.2 Hash Computation Procedure

**Spec file hashes:**

1. The hash field location is framework-specific (e.g., `config.sha256` for UATS, `fixture.sha256` for UPTS fixture files)
2. To compute: read the spec file → parse as JSON → **remove the hash field** → re-serialize to canonical JSON (sorted keys, compact form, UTF-8) → SHA256 the bytes
3. The hash field is excluded from its own input to avoid circular dependency

The shared implementation lives in `docs/tests/uxts_runner_core.py`:

```python
def canonical_json_bytes(obj):
    """Serialize JSON data into deterministic bytes (sorted keys, compact form)."""
    return json.dumps(
        obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False
    ).encode("utf-8")

def sha256_hex_obj(obj):
    return hashlib.sha256(canonical_json_bytes(obj)).hexdigest()

def sha256_spec_without_field(spec, field_path):
    """Compute SHA256 excluding a nested field (e.g., ('config', 'sha256'))."""
    spec_copy = json.loads(json.dumps(spec))
    # Remove the hash field from the copy
    current = spec_copy
    for key in field_path[:-1]:
        current = current.get(key, {})
    current.pop(field_path[-1], None)
    return sha256_hex_obj(spec_copy)
```

**Fixture file hashes:**

Fixture hashes are simpler — read the raw bytes, SHA256 them. No JSON parsing needed because fixtures can be any format (source code, YAML, binary).

```python
def sha256_file(path, chunk_size=8192):
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(chunk_size), b""):
            hasher.update(chunk)
    return hasher.hexdigest()
```

### 8.3 Always Execute, Always Report

Hash verification and assertion evaluation are **independent operations**. The runner always does both and reports the results separately.

A spec can have four outcome combinations:

| Assertions | Hash | Meaning |
|-----------|------|---------|
| pass | verified | Spec correct, file unchanged — healthy state |
| pass | mismatch | Spec correct, but file was modified since last hash — developer should review and recompute |
| fail | verified | Spec incorrect, file unchanged — real regression |
| fail | mismatch | Spec incorrect AND file was modified — investigate whether the modification caused the failure |

The per-spec `status` field in the report reflects assertion results **only**. The `hash_verified` and `hash_mismatches` fields reflect integrity results **only**. These are never conflated.

### 8.4 Shared Infrastructure

Two shared Python modules provide hash and reporting infrastructure for all framework runners:

**`docs/tests/uxts_runner_core.py`** — Low-level primitives:
- `canonical_json_bytes()` — Deterministic JSON serialization
- `sha256_hex_obj()` — Hash a JSON object
- `sha256_file()` — Hash a file on disk
- `sha256_spec_without_field()` — Hash a spec excluding its own hash field
- `canonical_result_status()` — Normalize status values (e.g., legacy `warn` → `fail`)

**`docs/tests/uxts_report.py`** — Report construction:
- `build_result()` — Build a single spec result (with 0/0 false-pass protection)
- `build_report()` — Build the full canonical report with summary and integrity blocks
- `print_summary()` — Print human-readable output to stdout
- `save_report()` — Write JSON report to disk

The 0/0 false-pass protection is built directly into `build_result()`:

```python
def build_result(spec_path, status, assertions_evaluated=0, ...):
    if status == "pass" and assertions_evaluated == 0:
        status = "fail"
        failures.append("0/0 false pass: no assertions evaluated")
    return { "spec_path": str(spec_path), "status": status, ... }
```

This ensures that no runner in any framework can accidentally report a pass with zero assertions.

---

# Layer 3: Hands-On Technical

---

## 9. Writing Specs: A Practical Guide

This section covers everything you need to write well-formed specs for UATS and UPTS — the two most mature frameworks. The patterns shown here apply to all frameworks.

### 9.1 UATS Spec Anatomy

A UATS spec has this structure:

```
┌─ uats_version        Version lock
├─ api                  Endpoint identity + tags
├─ metadata             Author, description, priority (optional)
├─ config               Timeout, hash, behavioral flags (optional)
├─ variables            Template variables (optional)
├─ auth                 Authentication config (optional)
├─ request              HTTP method + path + body + headers
├─ expected             Status + body_assertions + headers
├─ variants             Parameterized test variations (optional)
├─ captures             Extract values from response (optional)
├─ setup / teardown     Pre/post steps (schema-defined, not yet implemented)
└─ chain                Dependent request sequences (schema-defined, not yet implemented)
```

**Minimal valid UATS spec** (4 required fields only):

```json
{
  "uats_version": "1.0.0",
  "api": { "name": "My Endpoint" },
  "request": { "method": "GET", "path": "/my-endpoint" },
  "expected": { "status": 200 }
}
```

This is a valid spec — it verifies that `GET /my-endpoint` returns HTTP 200. No assertions on the body, no metadata, no config. You can start here and add detail incrementally.

**Full-featured UATS spec** (from `conversation_observe.uats.json`):

```json
{
  "uats_version": "1.0.0",
  "api": {
    "name": "Record Observation",
    "base_url": "${MDEMG_BASE_URL}",
    "version": "v1",
    "service": "mdemg",
    "tags": ["conversation", "observe", "write"]
  },
  "metadata": {
    "author": "reh3376",
    "created": "2026-01-29",
    "description": "Validates Record Observation endpoint",
    "test_type": "contract",
    "priority": "medium"
  },
  "config": {
    "timeout_ms": 15000,
    "sha256": "e7a0875c...caae14",
    "tags": ["embedding_required"]
  },
  "variables": {
    "test_space": "uats-observe-test"
  },
  "request": {
    "method": "POST",
    "path": "/v1/conversation/observe",
    "body": {
      "space_id": "{{test_space}}",
      "session_id": "uats-session",
      "content": "Test observation from UATS",
      "obs_type": "learning"
    },
    "headers": {
      "Content-Type": "application/json"
    }
  },
  "expected": {
    "status": 200,
    "body_assertions": [
      { "path": "$.obs_id", "type": "string" },
      { "path": "$.node_id", "type": "string" },
      { "path": "$.surprise_score", "type": "number" },
      { "path": "$.surprise_factors", "type": "object" },
      { "path": "$.summary", "type": "string" }
    ]
  }
}
```

Key patterns demonstrated:

- **`config.tags`** vs **`api.tags`**: `api.tags` categorize the endpoint; `config.tags` control runner behavior (e.g., `embedding_required` tells CI to skip this spec when no embedding provider is available)
- **`variables`**: Define `test_space` once, reference it as `{{test_space}}` in the request body. This keeps test data isolated per spec.
- **Type assertions**: `{ "path": "$.obs_id", "type": "string" }` checks that the field exists and is a string, without asserting a specific value. Use this for dynamic fields like IDs and timestamps.

### 9.2 Body Assertions: The Full Operator Vocabulary

UATS supports two assertion dialects. The **canonical dialect** (preferred) uses `path` + `op` + `expected`. The **legacy dialect** uses `path` + specific matcher keys.

#### Canonical Dialect (Recommended)

```json
{ "path": "$.status", "op": "equals", "expected": "ok" }
```

The canonical dialect has three required fields:
- `path` — JSONPath into the response body
- `op` — Operation to perform
- `expected` — Value to compare against (not required for `exists`/`not_exists`)

**Complete operator enum:**

| Operator | Description | Example |
|----------|-------------|---------|
| `equals` | Exact value match | `{"path":"$.status","op":"equals","expected":"ok"}` |
| `not_equals` | Value must not equal | `{"path":"$.type","op":"not_equals","expected":"error"}` |
| `contains` | String contains substring | `{"path":"$.message","op":"contains","expected":"success"}` |
| `not_contains` | String must not contain | `{"path":"$.error","op":"not_contains","expected":"panic"}` |
| `matches` | Regex match | `{"path":"$.id","op":"matches","expected":"^[a-f0-9]{8}$"}` |
| `type_is` | Type check | `{"path":"$.count","op":"type_is","expected":"number"}` |
| `greater_than` | Numeric comparison | `{"path":"$.score","op":"greater_than","expected":0.5}` |
| `less_than` | Numeric comparison | `{"path":"$.latency","op":"less_than","expected":100}` |
| `greater_than_or_equal` | Numeric comparison | `{"path":"$.version","op":"greater_than_or_equal","expected":1}` |
| `less_than_or_equal` | Numeric comparison | `{"path":"$.retries","op":"less_than_or_equal","expected":3}` |
| `exists` | Field must be present | `{"path":"$.token","op":"exists","expected":true}` |
| `not_exists` | Field must be absent | `{"path":"$.password","op":"not_exists","expected":true}` |
| `one_of` | Value in list | `{"path":"$.status","op":"one_of","expected":["ok","degraded"]}` |
| `length` | Array/string length | `{"path":"$.items","op":"length","expected":5}` |
| `length_greater_than` | Length comparison | `{"path":"$.results","op":"length_greater_than","expected":0}` |
| `length_less_than` | Length comparison | `{"path":"$.errors","op":"length_less_than","expected":10}` |
| `items_all_match` | All array items match | `{"path":"$.items","op":"items_all_match","expected":{"type":"object"}}` |
| `schema_match` | JSON Schema validation | `{"path":"$.data","op":"schema_match","expected":{"type":"object","required":["id"]}}` |

#### Legacy Dialect (Backward Compatible)

```json
{ "path": "$.status", "equals": "ok" }
{ "path": "$.obs_id", "type": "string" }
{ "path": "$.score", "range": { "min": 0, "max": 1 } }
```

The legacy dialect uses the matcher key directly on the assertion object. Both dialects are supported simultaneously — the schema defines `BodyAssertion` as `oneOf [CanonicalBodyAssertion, LegacyBodyAssertion]`.

**When to use which:**
- **New specs**: Always use canonical dialect (`op` + `expected`)
- **Existing specs**: Legacy dialect is fine — no need to migrate working specs
- **Mixed**: A single spec can use both dialects in its `body_assertions` array

#### Optional Assertion Fields

Both dialects support these optional fields:

| Field | Type | Purpose |
|-------|------|---------|
| `name` | string | Human-readable assertion name (appears in failure messages) |
| `message` | string | Custom failure message |
| `optional` | boolean | If `true`, failure produces a warning instead of hard fail |
| `capture_as` | string | Save the matched value to a variable for use in later assertions |

### 9.3 UPTS Spec Anatomy

A UPTS spec has this structure:

```
┌─ upts_version        Version lock
├─ language             Target language (enum of 30 languages)
├─ variants             File extensions [".py", ".pyi"]
├─ metadata             Author, description, parser_status
├─ config               Tolerances and validation flags
├─ fixture              Test input (file path or inline content)
└─ expected
   ├─ symbol_count      Expected count or {min, max} range
   ├─ symbols[]         Array of expected symbols
   ├─ excluded[]        Symbols that must NOT appear
   └─ relationships[]   Inter-symbol relationships (not yet enforced)
```

**Symbol object fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | Yes | string | Symbol name |
| `type` | Yes | enum | One of: `class`, `function`, `method`, `field`, `constant`, `enum`, `interface`, `struct`, `type`, `variable`, `module`, `namespace`, etc. (31 types total) |
| `line` | Yes | integer | Line number in fixture (validated with `line_tolerance`) |
| `line_end` | No | integer | End line number |
| `exported` | No | boolean | Whether symbol is publicly visible |
| `parent` | No | string | Containing symbol name (e.g., `"UserService"` for a method) |
| `signature` | No | string | Full signature text |
| `value` | No | string | Literal value (for constants) |
| `doc_comment` | No | string | Documentation comment |
| `decorators` | No | string[] | Applied decorators/annotations |
| `pattern` | No | string | Pattern code (e.g., `P1_CONSTANT`, `P2_FUNCTION`) |
| `optional` | No | boolean | If `true`, missing symbol produces warning not failure |
| `tags` | No | string[] | Categorization tags |

### 9.4 Variables and Environment Resolution

UATS supports two variable resolution mechanisms:

**Environment variables** — `${ENV_VAR}` syntax in string fields:

```json
{
  "api": {
    "base_url": "${MDEMG_BASE_URL}"
  }
}
```

The runner resolves `${MDEMG_BASE_URL}` from the process environment at execution time. If the variable is not set and no `--base-url` flag is provided, the runner reports an error. The `--base-url` CLI flag overrides the spec's `base_url` value.

**Spec-defined variables** — `{{variable}}` syntax:

```json
{
  "variables": {
    "test_space": "uats-observe-test",
    "session_id": { "generator": "uuid" }
  },
  "request": {
    "body": {
      "space_id": "{{test_space}}",
      "session_id": "{{session_id}}"
    }
  }
}
```

Variable values can be:
- Static strings or numbers
- Environment variable references: `{ "env": "MY_VAR", "default": "fallback" }`
- Generated values: `{ "generator": "uuid" }`, `{ "generator": "timestamp" }`, `{ "generator": "random_int" }`

### 9.5 Variants: Parameterized Test Variations

Variants let you define multiple test cases from a single spec by overriding specific fields:

```json
{
  "uats_version": "1.0.0",
  "api": { "name": "Observe Endpoint", "base_url": "${MDEMG_BASE_URL}" },
  "request": {
    "method": "POST",
    "path": "/v1/conversation/observe",
    "body": {
      "space_id": "test-space",
      "content": "test content",
      "obs_type": "learning"
    }
  },
  "expected": { "status": 200 },
  "variants": [
    {
      "name": "missing_content",
      "description": "Should fail when content is empty",
      "request": { "body": { "content": "" } },
      "expected": { "status": 400 }
    },
    {
      "name": "invalid_obs_type",
      "description": "Should fail with unknown observation type",
      "request": { "body": { "obs_type": "nonexistent" } },
      "expected": { "status": 400 }
    },
    {
      "name": "missing_space_id",
      "description": "Should fail when space_id is missing",
      "request": { "body": { "space_id": "" } },
      "expected": { "status": 400 }
    }
  ]
}
```

**Deep merge semantics**: Variant overrides are **deep-merged** with the base spec. In the example above, the `missing_content` variant merges its `request.body` with the base `request.body`:

```
Base:    { "space_id": "test-space", "content": "test content", "obs_type": "learning" }
Variant: { "content": "" }
Result:  { "space_id": "test-space", "content": "", "obs_type": "learning" }
```

**Gotcha: Deep merge can surprise you.** If the base body has `"obs_type": "learning"` and the variant only overrides `"content"`, the variant inherits `obs_type` from the base. To test a missing field, you must explicitly set it to `""` or `null` in the variant — simply omitting it means it inherits the base value.

**Variant-specific fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `name` | string (required) | Unique variant identifier |
| `description` | string | What this variant tests |
| `tags` | string[] | Additional tags for this variant |
| `skip` | boolean | Skip this variant (with `skip_reason`) |
| `skip_reason` | string | Why this variant is skipped |
| `variables` | object | Variable overrides |
| `request` | object | Request field overrides (deep-merged) |
| `expected` | object | Expected field overrides (deep-merged) |

### 9.6 Tags and Selective Execution

Tags control which specs execute in different contexts.

**Where tags live:**

| Location | Purpose | Example |
|----------|---------|---------|
| `api.tags` | Categorize the endpoint | `["conversation", "observe", "write"]` |
| `config.tags` | Control runner behavior | `["embedding_required", "llm_required"]` |
| `variants[].tags` | Tag individual variants | `["error_case", "negative_test"]` |

**CLI tag filtering:**

```bash
# Only run specs tagged "smoke"
python3 uats_runner.py validate-all --include-tag smoke

# Skip specs that need embedding or LLM
python3 uats_runner.py validate-all --exclude-tag embedding_required,llm_required

# Run only RSIC-related specs
python3 uats_runner.py validate-all --include-tag rsic
```

**Common tag conventions:**

| Tag | Meaning |
|-----|---------|
| `smoke` | Fast, fundamental checks (health, readiness) |
| `embedding_required` | Needs embedding provider (skip in CI with `EMBEDDING_PROVIDER=stub`) |
| `llm_required` | Needs LLM provider (skip in CI) |
| `unts` | UNTS-specific tests (may need UNTS infrastructure) |
| `rsic` | RSIC self-improvement tests |
| `write` | Test modifies data (may need cleanup) |

---

## 10. Running Specs: The Runner Contract

### 10.1 CLI Interface Standard

All UxTS runners follow the same CLI interface pattern:

```
<runner> validate --spec <spec-path> [options]
<runner> validate-all --spec-dir <directory> [options]
<runner> add-hashes --spec-dir <directory>
<runner> verify-hashes --spec-dir <directory>
```

**UATS runner CLI:**

```bash
# Validate single spec
python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
  --spec docs/api/api-spec/uats/specs/health.uats.json \
  --base-url http://localhost:9999

# Validate all specs
python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --report /tmp/api-report.json \
  --exclude-tag embedding_required

# Add integrity hashes to all specs
python3 docs/api/api-spec/uats/runners/uats_runner.py add-hashes \
  --spec-dir docs/api/api-spec/uats/specs/

# Verify existing hashes
python3 docs/api/api-spec/uats/runners/uats_runner.py verify-hashes \
  --spec-dir docs/api/api-spec/uats/specs/
```

**UPTS runner CLI:**

```bash
# Validate single language
python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
  --spec docs/lang-parser/lang-parse-spec/upts/specs/python.upts.json \
  --parser "./bin/extract-symbols --json"

# Validate all languages
python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
  --spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
  --parser "./bin/extract-symbols --json" \
  --report /tmp/parser-report.json
```

### 10.2 Execution Flow

```
                    ┌─────────────┐
                    │  Load Spec  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Verify Hash │──→ hash_verified: true/false/null
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Validate   │──→ status: error (if malformed)
                    │  vs Schema  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Parity    │──→ status: fail (if unimplemented fields)
                    │   Check     │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Resolve    │
                    │  Variables  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Execute    │
                    │  Request    │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Evaluate   │──→ assertions_evaluated / assertions_passed
                    │ Assertions  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  0/0 Check  │──→ FAIL if 0 assertions evaluated
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Build Result│──→ canonical report entry
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Report    │──→ JSON file + stdout summary
                    └─────────────┘
```

### 10.3 Canonical Report Format

All runners produce JSON reports in the same structure. This enables cross-framework tooling to consume results from any framework without framework-specific parsing.

**Report structure:**

```json
{
  "timestamp": "2026-02-27T14:30:00Z",
  "framework": "uats",
  "framework_version": "1.1.0",
  "summary": {
    "total_specs": 129,
    "passed": 127,
    "failed": 1,
    "skipped": 1,
    "errors": 0,
    "pass_rate": 98.45,
    "duration_ms": 4521.00
  },
  "integrity": {
    "total_hashed": 128,
    "verified": 126,
    "mismatched": 2,
    "no_hash": 1
  },
  "results": [
    {
      "spec_path": "specs/health.uats.json",
      "status": "pass",
      "duration_ms": 42.00,
      "hash_verified": true,
      "hash_mismatches": [],
      "assertions_evaluated": 1,
      "assertions_passed": 1,
      "failures": [],
      "warnings": [],
      "error": null
    }
  ]
}
```

**Summary fields:**

| Field | Type | Description |
|-------|------|-------------|
| `total_specs` | integer | Total specs processed |
| `passed` | integer | Specs with status `pass` |
| `failed` | integer | Specs with status `fail` |
| `skipped` | integer | Specs excluded by tag filter |
| `errors` | integer | Specs that could not execute (parse error, runner crash) |
| `pass_rate` | float | `(passed / total_specs) * 100` |
| `duration_ms` | float | Total wall-clock time in milliseconds |

**Integrity fields** (independent from assertion results):

| Field | Type | Description |
|-------|------|-------------|
| `total_hashed` | integer | Specs that have a hash field |
| `verified` | integer | Hash matches |
| `mismatched` | integer | Hash mismatches (files were modified) |
| `no_hash` | integer | Specs with no hash field |

**Status semantics:**

| Status | Meaning |
|--------|---------|
| `pass` | All assertions passed AND `assertions_evaluated >= 1` |
| `fail` | Assertion failure OR parity failure OR 0/0 false pass detected |
| `skip` | Spec excluded by tag filter or explicit `--exclude` |
| `error` | Runner could not execute spec (parse failure, missing fixture, crash) |

### 10.4 Shared Runner Infrastructure

Two Python modules provide shared infrastructure that all framework runners import:

**`docs/tests/uxts_runner_core.py`** — Cryptographic primitives:

| Function | Purpose |
|----------|---------|
| `canonical_json_bytes(obj)` | Deterministic JSON serialization for hash computation |
| `sha256_hex_obj(obj)` | SHA256 of a JSON object |
| `sha256_file(path)` | SHA256 of a file on disk |
| `sha256_spec_without_field(spec, field_path)` | Hash a spec excluding its own hash field |
| `canonical_result_status(status)` | Normalize status values (`warn` → `fail`) |

**`docs/tests/uxts_report.py`** — Report construction:

| Function | Purpose |
|----------|---------|
| `build_result(spec_path, status, ...)` | Build a single spec result with 0/0 protection |
| `build_integrity(results)` | Compute integrity summary from result list |
| `build_report(framework, version, results)` | Build full canonical report |
| `print_summary(report)` | Print human-readable summary to stdout |
| `save_report(report, path)` | Write JSON report to disk |

When building a new framework runner, import these modules instead of reimplementing hash computation and report formatting. This ensures all frameworks produce identical report structures.

---

## 11. CI Integration

### 11.1 Makefile as Portable Interface

The Makefile is the **portable CI interface**. Every framework has a Makefile target that runs the same command locally and in CI. This means switching CI providers (GitHub Actions → GitLab CI → Jenkins) requires changing the workflow file but not the runners, specs, or Makefile.

**Key Makefile targets** (from the MDEMG `Makefile`):

```makefile
# Dynamic port discovery
BASE_URL ?= http://localhost:$(shell cat .mdemg.port 2>/dev/null || echo 9999)
export MDEMG_BASE_URL ?= $(BASE_URL)

# Run all UATS API validation tests
test-api:
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--exclude-tag unts \
		--report /tmp/api-report.json

# Run UPTS parser validation tests
test-parsers: build-parser verify-upts-schema
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
		--spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
		--parser "./bin/extract-symbols --json" \
		--report /tmp/parser-report.json

# Run UBTS smoke benchmark
test-ubts-smoke:
	python3 docs/tests/ubts/runners/ubts_runner.py \
		--spec "docs/tests/ubts/specs/retrieve_latency.ubts.json" \
		--profile docs/tests/ubts/profiles/smoke.profile.json \
		--base-url $(BASE_URL) \
		--report /tmp/ubts-report.json

# Run all UOTS observability contract tests
test-uots:
	python3 docs/api/api-spec/uots/runners/uots_runner.py \
		--spec-dir docs/api/api-spec/uots/specs/ \
		--base-url $(BASE_URL) \
		--report /tmp/uots-report.json

# Validate single API endpoint: make test-api-health
test-api-%:
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
		--spec docs/api/api-spec/uats/specs/$*.uats.json \
		--base-url $(BASE_URL)

# Validate single parser: make test-parser-python
test-parser-%: build-parser
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
		--spec docs/lang-parser/lang-parse-spec/upts/specs/$*.upts.json \
		--parser "./bin/extract-symbols --json"

# Smoke tests (health + readiness only)
test-smoke:
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
		--spec docs/api/api-spec/uats/specs/health.uats.json \
		--base-url $(BASE_URL)

# Run all tests
test-all: test-parsers test-api

# Schema/drift verification
verify-upts-schema:
	python3 scripts/verify_upts_schema_parity.py

verify-uxts-canonical:
	python3 scripts/verify_uxts_canonical_specs.py

verify-uxts-drift:
	python3 scripts/verify_uxts_drift.py
```

**Pattern**: `make test-<framework>` runs the framework. `make test-<framework>-<name>` runs a single spec. `make verify-<check>` runs governance verification. Every target is portable — it works identically on a developer laptop and in CI.

### 11.2 GitHub Actions Integration

The MDEMG project uses two GitHub Actions workflows for UxTS:

**`ci.yml`** — Main CI pipeline (runs on every push/PR):

```yaml
name: CI
on:
  push:
    branches: [main, mdemg-dev01]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      neo4j:
        image: neo4j:5
        env:
          NEO4J_AUTH: neo4j/testpassword
        ports:
          - 7687:7687
    steps:
      # ... build, start server ...

      - name: Run UATS Contract Tests
        run: |
          python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
            --spec-dir docs/api/api-spec/uats/specs/ \
            --base-url http://localhost:${{ env.MDEMG_PORT }} \
            --exclude-tag unts,llm_required

      - name: Run UBTS Smoke Benchmark (soft-fail)
        continue-on-error: true
        run: |
          python3 docs/tests/ubts/runners/ubts_runner.py \
            --spec docs/tests/ubts/specs/retrieve_latency.ubts.json \
            --profile docs/tests/ubts/profiles/smoke.profile.json \
            --base-url http://localhost:${{ env.MDEMG_PORT }}
```

**`parser-tests.yml`** — Parser validation (runs on parser-related changes):

```yaml
name: Parser Tests
on:
  push:
    branches: [main]
    paths:
      - 'cmd/ingest-codebase/**'
      - 'cmd/extract-symbols/**'
      - 'docs/lang-parser/**'

jobs:
  parser-validation:
    steps:
      - name: Verify UPTS schema parity
        run: python3 scripts/verify_upts_schema_parity.py

      - name: Verify fixtures integrity
        run: |
          cd docs/lang-parser/lang-parse-spec/upts/fixtures
          echo "8975b14f...4b11f5  python_test_fixture.py" | shasum -a 256 -c
          echo "a95030aa...8e1e98  go_test_fixture.go" | shasum -a 256 -c

      - name: Run UPTS validation
        run: |
          python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
            --spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
            --parser "./bin/extract-symbols --json" \
            --report parser-report.json

      - name: Upload parser report
        uses: actions/upload-artifact@v6
        if: always()
        with:
          name: parser-validation-report
          path: parser-report.json
```

Key CI patterns:

- **Path triggers**: `parser-tests.yml` only runs when parser-related files change, avoiding unnecessary work
- **Service containers**: Neo4j runs as a service container for integration tests
- **Tag filtering**: `--exclude-tag unts,llm_required` skips specs that need infrastructure unavailable in CI
- **Artifact upload**: Parser reports are uploaded as CI artifacts for post-mortem analysis
- **Soft-fail**: UBTS benchmarks use `continue-on-error: true` so performance variations don't block the pipeline

### 11.3 Tag Filtering in CI

Tags provide fine-grained control over which specs execute in different environments:

| Environment | Include Tags | Exclude Tags | Rationale |
|-------------|-------------|-------------|-----------|
| Local dev | (none — run all) | (none) | Developers have full infrastructure |
| CI (main) | (none) | `unts`, `llm_required` | CI has no UNTS infra or LLM access |
| CI (RSIC) | `rsic` | (none) | Run only RSIC-related specs |
| Smoke | `smoke` | (none) | Fastest possible validation |
| Staging | (none) | (none) | Full suite against staging environment |

### 11.4 Gate Modes

Gate modes determine what happens when a framework's runner reports failures:

| Gate Mode | CI Behavior | When to Use |
|-----------|-------------|-------------|
| **block** | Failures fail the pipeline | Stable frameworks with proven spec sets (UATS, UPTS) |
| **soft** | Failures are visible but don't block | Newly promoted frameworks or flaky domains (UBTS) |
| **observe** | Results recorded for metrics only | Frameworks in early pilot where false-fails are expected |

Gate mode is **independent of framework status**. An `active` framework can be `soft`-gated during a stabilization period. A `pilot` framework can be `block`-gated if confidence is high. See Section 14 for the full lifecycle.

### 11.5 Canonical Spec Guard

The canonical spec guard ensures that `specs/` directories contain only schema-conforming specs. Non-canonical formats must live in `drafts/`.

```makefile
# From Makefile
verify-uxts-canonical:
	python3 scripts/verify_uxts_canonical_specs.py
```

This script:
1. Scans all `specs/` directories across all frameworks
2. Validates each spec against its framework's JSON Schema
3. Rejects specs with non-canonical formats
4. Reports specs that should be in `drafts/` instead

The guard runs in CI to prevent non-canonical specs from accumulating in the main spec directories.

---

# Layer 4: Operational Knowledge

---

## 12. Anti-Patterns: Lessons Learned

These are failure modes discovered through real implementation in the MDEMG project. They are sourced from the UxTS Framework Gap Assessment (`docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md`) and represent hard-won lessons from governing 11 frameworks across 105 development phases.

### 12.1 The 0/0 False Pass

**What happened:** The UOBS (runtime observability) framework reported 100% pass rate on specs that defined `logging` checks. The runner implemented `health`, `metrics`, and `dependency` check types but not `logging`. Specs with only logging checks had zero assertions to evaluate — so they passed with 0 failures out of 0 checks.

**Why it's dangerous:** The pass rate is 100% and the coverage is zero. The team believed logging observability was verified when nothing was checked. The Gap Assessment classified this as **Critical** severity.

**The fix:** Runners must require at least one executable assertion per spec. This is enforced in the shared `build_result()` function:

```python
def build_result(spec_path, status, assertions_evaluated=0, ...):
    if status == "pass" and assertions_evaluated == 0:
        status = "fail"
        failures.append("0/0 false pass: no assertions evaluated")
```

**Prevention:** Every new runner must import `uxts_report.py` and use `build_result()` — the protection is automatic.

### 12.2 The Silent Schema Ignore

**What happened:** The UATS schema defined `setup` and `teardown` blocks for pre/post request steps. Spec authors used these fields to seed test data. The runner read the spec, ignored the `setup`/`teardown` fields entirely, executed the core assertions, and reported "pass." The setup that was supposed to seed test data never ran. Assertions passed against whatever data happened to exist.

**Why it's dangerous:** The spec expressed clear intent ("seed this data before testing"), but the runner silently ignored it. The spec appeared to verify a specific scenario but actually verified an accidental state.

**The fix:** Schema-runner parity enforcement (Section 7). The runner now maintains a set of known fields and hard-fails when it encounters fields it doesn't implement:

```
FAIL: Unimplemented field 'setup': runner cannot validate this spec
```

**Prevention:** Every runner must implement field-detection logic. Unimplemented fields cause hard failures, not silent skips.

### 12.3 The Dialect Split

**What happened:** The UDTS (gRPC) framework had 11 specs, but they used two incompatible formats. Seven specs used the canonical `{ "service": "...", "method": "...", "request": {...}, "expected": {...} }` structure. Four specs used a different structure: `{ "api": "...", "test_cases": [...] }`. The runner handled the canonical format; the non-canonical specs were read but not properly validated.

**Why it's dangerous:** Non-canonical specs inflate the spec count and pass rate without contributing real verification. The team sees "11 specs, 100% pass" when only 7 are actually validated.

**The fix:** Canonical/draft separation. Non-canonical specs were moved from `specs/` to `drafts/`. A canonical guard script (`verify_uxts_canonical_specs.py`) runs in CI to reject non-conforming specs in the `specs/` directory.

**Prevention:** One schema per framework. Specs that don't conform go to `drafts/`, not `specs/`. The canonical guard enforces this on every push.

### 12.4 The Phantom Directory

**What happened:** The UAMS (auth method) framework's documentation and governance matrix claimed it had `fixtures/` and `runners/` directories with a functional runner and credential fixtures. Neither actually existed on disk. The framework appeared complete in the matrix but had no executable verification.

**Why it's dangerous:** Governance documentation says "UAMS: active, runner exists, fixtures exist" when none of this is true. Decisions about auth coverage are made based on false inventory data.

**The fix:** The framework was reclassified from "active" to **spec-only** and phantom claims were removed. A drift checker (`verify_uxts_drift.py`) was created to validate that every path declared in the governance matrix actually exists on disk.

**Prevention:** Run `make verify-uxts-drift` in CI. It validates:
- Every declared runner path exists
- Every declared spec directory exists and contains the documented number of specs
- Every fixture path referenced by specs exists

### 12.5 The Ambiguous Baseline

**What happened:** The UBTS (benchmark) framework defined `max_p99_degradation_pct` to catch performance regressions. But degradation compared to what? The initial implementation compared against "the previous run." This meant:
- Different CI environments produced different baselines
- The first run in a new environment had no baseline
- A slow run followed by a normal run could appear as "improvement" rather than "return to normal"

**Why it's dangerous:** Non-deterministic baselines make benchmark results unreliable and non-reproducible. The same code can "pass" or "fail" depending on what was run before.

**The fix:** Use the spec's own threshold as the fixed baseline. If the spec says `p99_ms: 500`, then `max_p99_degradation_pct: 10` means "fail if actual p99 exceeds 550ms." The baseline is declarative, deterministic, and visible in the spec diff.

**Prevention:** Benchmark specs must define their own thresholds. Runners must never compare against external state (previous runs, databases, files) for pass/fail decisions.

### 12.6 The SKIP/WARN False Confidence

**What happened:** A runner encountered a field it couldn't validate and reported SKIP. The overall spec result was "pass with 1 warning." Over time, teams stopped reading warnings. A spec had three SKIP'd assertions and one passing trivial assertion — it reported "pass."

**Why it's dangerous:** SKIP accumulates silently. After a month, 40% of assertions in a framework are SKIP'd but the pass rate is still "100%." Nobody notices because the summary line says "PASS" and the warnings are buried in detail output.

**The fix:** In `active` frameworks, unimplemented fields cause hard **FAIL**, not SKIP. The `canonical_result_status()` function in `uxts_runner_core.py` normalizes legacy `warn` to `fail`:

```python
def canonical_result_status(status):
    value = str(getattr(status, "value", status)).strip().lower()
    if value == "warn":
        return "fail"
    if value in {"pass", "fail", "skip", "error"}:
        return value
    return "error"
```

**Prevention:** Reserve SKIP for intentional exclusion (tag filtering). Never use SKIP for "couldn't validate this assertion."

### 12.7 The Stale Documentation

**What happened:** The governance matrix said "124 UATS specs." The disk had 89 at the time. The README said "45 specs." Three different documents claimed three different numbers. Nobody knew the real count. When counts diverge, every statement about coverage is suspect.

**Why it's dangerous:** Stale documentation undermines trust in the entire governance system. If the spec count is wrong, what else is wrong? Are the status classifications current? Are the runner paths correct?

**The fix:** Automated drift checker that compares on-disk spec counts to documented counts. The checker runs in CI and produces loud warnings (or blocks the pipeline) when discrepancies are detected.

**Prevention:** Never manually maintain spec counts in documentation. Use `verify_uxts_drift.py` to auto-detect the real state and flag discrepancies. Update governance docs as part of the implementation workflow (see MEMORY.md "Documents Accessed" list pattern).

---

## 13. Creating a New Framework

### 13.1 When to Create vs. Extend

**Default: extend an existing framework.** Creating a new framework introduces governance overhead: a new schema, a new runner, a new CI gate, new documentation. Only create a new framework when:

1. No existing framework can represent the construct without **semantic overlap**
2. Schema extension would **degrade clarity** or ownership boundaries
3. The new domain has fundamentally **different assertion semantics** (e.g., "parse this file" vs. "send this HTTP request")

**Decision rule:** If the new specs would use the same runner logic as an existing framework, extend rather than create. If they need a fundamentally different runner (different input type, different output format, different pass/fail criteria), create.

### 13.2 Step-by-Step Bootstrap

#### Step 1: Define the Schema

Create a JSON Schema that defines the valid structure for specs in your domain. Start minimal — you can extend later.

**Required sections:**
- Version field (`<framework>_version`)
- Identity section (what is being tested)
- Input section (what to send/parse/check)
- Expected section (what the result should be)

**Template:**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://your-org.dev/schemas/<framework>/v1.0.0",
  "title": "Universal <Domain> Test Specification",
  "type": "object",
  "required": ["<framework>_version", "<identity>", "<input>", "expected"],
  "properties": {
    "<framework>_version": { "type": "string", "const": "1.0.0" },
    "metadata": {
      "type": "object",
      "properties": {
        "author": { "type": "string" },
        "description": { "type": "string" }
      }
    },
    "config": {
      "type": "object",
      "properties": {
        "sha256": { "type": "string" }
      }
    }
  }
}
```

#### Step 2: Write Baseline Specs from Live Code

**Critical:** Don't write specs from documentation or assumptions. Run the actual code, capture the actual behavior, and encode that behavior as a spec. Specs generated from assumed behavior are the leading cause of false-fail on first run.

For parser specs, this lesson was learned the hard way — 5 tree-sitter languages (C, C++, CUDA, Java, Rust) had specs written from assumed parser output. All 5 failed on first run. The fix was a `generate_spec_from_output.py` script that captures actual parser output and generates the spec from it.

```bash
# Good: generate spec from actual behavior
./bin/extract-symbols --json fixture.py > actual_output.json
python3 generate_spec_from_output.py actual_output.json > python.upts.json

# Bad: guess what the output should look like
# (This spec WILL fail when it encounters real parser behavior)
```

#### Step 3: Build the Runner

The runner reads specs, performs verification, and reports results. Import the shared infrastructure:

```python
#!/usr/bin/env python3
"""Minimal framework runner template."""
import sys
sys.path.insert(0, "docs/tests")  # For shared modules

from uxts_report import build_result, build_report, print_summary, save_report
from uxts_runner_core import sha256_spec_without_field

def run_spec(spec_path, spec):
    """Execute one spec and return a result dict."""
    # 1. Verify hash
    hash_verified = None
    hash_mismatches = []
    stored_hash = spec.get("config", {}).get("sha256")
    if stored_hash:
        computed = sha256_spec_without_field(spec, ("config", "sha256"))
        hash_verified = (stored_hash == computed)
        if not hash_verified:
            hash_mismatches.append(f"config.sha256: expected {stored_hash}, got {computed}")

    # 2. Check parity (detect unimplemented fields)
    parity_errors = validate_supported_features(spec)
    if parity_errors:
        return build_result(spec_path, "fail", hash_verified=hash_verified,
                           hash_mismatches=hash_mismatches, failures=parity_errors)

    # 3. Execute verification
    failures = []
    assertions_evaluated = 0
    assertions_passed = 0

    # ... domain-specific verification logic ...

    # 4. Build result (0/0 protection is automatic)
    status = "pass" if not failures else "fail"
    return build_result(spec_path, status,
                       hash_verified=hash_verified,
                       hash_mismatches=hash_mismatches,
                       assertions_evaluated=assertions_evaluated,
                       assertions_passed=assertions_passed,
                       failures=failures)
```

#### Step 4: Add Schema Validation

Before executing any spec, validate it against the framework's JSON Schema:

```python
import jsonschema

def validate_spec_schema(spec, schema):
    try:
        jsonschema.validate(spec, schema)
    except jsonschema.ValidationError as e:
        return f"Schema validation failed: {e.message}"
    return None
```

#### Step 5: Wire into Local Automation

Add a Makefile target:

```makefile
test-<framework>:
	python3 docs/tests/<framework>/runners/<framework>_runner.py validate-all \
		--spec-dir docs/tests/<framework>/specs/ \
		--report /tmp/<framework>-report.json
```

#### Step 6: Wire into CI

Add a CI step that invokes the Makefile target:

```yaml
- name: Run <Framework> Tests
  run: make test-<framework>
```

Start with `soft` gating (`continue-on-error: true`) until the spec set stabilizes.

#### Step 7: Add Hash Integrity

Define the framework's hash field path and compute hashes for all specs:

```bash
python3 <runner>.py add-hashes --spec-dir docs/tests/<framework>/specs/
```

#### Step 8: Document Governance

Add an entry to `FRAMEWORK_GOVERNANCE.md` and `UXTS_FRAMEWORK_MATRIX.md` with:
- Schema path
- Spec directory
- Runner command
- CI gate mode
- Hash field convention
- Current status (`pilot` for new frameworks)

### 13.3 Repository Layout Convention

```
docs/tests/<framework>/
├── schema/
│   └── <framework>.schema.json    # JSON Schema
├── specs/
│   └── *.u<x>ts.json              # Canonical specs
├── drafts/
│   └── *.u<x>ts.json              # Non-canonical / in-progress specs
├── fixtures/                       # Static test inputs (if applicable)
│   └── <fixture-files>
├── runners/
│   └── <framework>_runner.py      # Executable runner
└── README.md                      # Framework-specific documentation
```

For frameworks closely tied to API specs, the convention uses `docs/api/api-spec/<framework>/` instead of `docs/tests/<framework>/`. See the UATS and UOTS locations in the inventory table (Section 6.3).

### 13.4 Worked Example: Creating UETS

The UETS (Universal Emergence Test Specification) framework was created to verify LLM concept-naming quality for the dynamic emergence system (Phase 103). Here is how it was bootstrapped:

1. **Schema**: Defined quality dimensions E1-E5 (lexical validity, semantic relevance, hierarchical coherence, description quality, distinctiveness) as the assertion vocabulary
2. **Specs from live output**: Ran the emergence namer against real concept clusters, captured actual output, encoded quality thresholds as specs
3. **Runner**: Built `uets_runner.py` importing `uxts_report.py` for canonical reports
4. **Parity**: All 5 quality dimensions (E1-E5) are enforced — no unimplemented fields
5. **Makefile**: Added `test-uets` target
6. **CI**: Not yet gated (no CI gate) but functional locally
7. **Hashes**: All 8 specs have integrity hashes
8. **Governance**: Added to both `FRAMEWORK_GOVERNANCE.md` and `UXTS_FRAMEWORK_MATRIX.md`

Status: `active` — all schema fields enforced, 8 specs, functional runner.

---

## 14. Framework Lifecycle Management

### 14.1 Status Transitions

```
                    ┌───────────┐
                    │ spec-only │  Schema + specs exist
                    │           │  No runner, no CI
                    └─────┬─────┘
                          │  Functional runner + 1 passing spec
                          ▼
                    ┌───────────┐
                    │   pilot   │  Schema + specs + runner
                    │           │  CI may be observe/soft
                    └─────┬─────┘
                          │  100% parity + CI gate + meaningful coverage
                          ▼
                    ┌───────────┐
                    │  active   │  Full verification capability
                    │           │  CI-gated (soft or block)
                    └─────┬─────┘
                          │  Superseded or no longer relevant
                          ▼
                    ┌───────────┐
                    │deprecated │  Migration plan documented
                    │           │  Specs archived
                    └───────────┘
```

### 14.2 Promotion Criteria

**spec-only → pilot:**
- [ ] Runner exists and is executable
- [ ] At least one spec passes when executed by the runner
- [ ] Runner produces canonical report format (Section 10.3)

**pilot → active:**
- [ ] 100% schema-runner parity — every schema field classified as `enforced`, `advisory`, or hard-fail `unimplemented`
- [ ] CI gate enabled at minimum `soft` mode
- [ ] Documented authority scope (what domain this framework owns)
- [ ] No critical false-pass paths (0/0 protection, parity detection)
- [ ] Meaningful spec coverage (not just one trivial spec)

**active (soft) → active (block):**
- [ ] Demonstrated stability — no flaky false-fails over a meaningful window
- [ ] Low false-pass rate
- [ ] Sufficient spec coverage for the domain

### 14.3 Deprecation

When a framework is superseded or no longer relevant:

1. Change status to `deprecated` in governance docs
2. Document the migration path to the successor framework
3. Archive existing specs (move to an `archived/` directory, not delete)
4. Remove or disable the CI gate
5. Keep the runner available for reference but remove from CI

### 14.4 Status vs. Gate Mode: Independent Axes

A common mistake is conflating framework status with CI gate mode. They are independent:

| Axis | Values | What It Governs |
|------|--------|-----------------|
| **Status** | `spec-only`, `pilot`, `active`, `deprecated` | Framework maturity — does it have a schema? runner? parity? |
| **Gate mode** | `observe`, `soft`, `block` | CI behavior — does failure stop the pipeline? |

Valid combinations:

| Status | Gate Mode | Meaning |
|--------|-----------|---------|
| spec-only | (none) | Intent documented, no execution |
| pilot | observe | Runner works, results tracked, no CI impact |
| pilot | soft | Runner works, failures visible, don't block |
| active | soft | Full verification, stabilization period |
| active | block | Full verification, failures stop pipeline |
| deprecated | (none) | Framework retiring, CI removed |

An `active` framework starts with `soft` gate mode on promotion and graduates to `block` after demonstrated stability.

---

## 15. Governance Artifacts

### 15.1 Required Documents

Every UxTS-governed repository must maintain these documents:

| Artifact | Path (MDEMG) | Purpose | Update Trigger |
|----------|------|---------|----------------|
| Framework Governance | `docs/specs/FRAMEWORK_GOVERNANCE.md` | Policy: ownership, lifecycle rules, authority splits | Framework status change, policy update |
| Framework Matrix | `docs/development/UXTS_FRAMEWORK_MATRIX.md` | Inventory: schema/spec/runner/CI paths, counts, parity | Any spec/runner/CI change |
| Gap Assessment | `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_<date>.md` | Point-in-time audit against canonical contract | Periodic review or before major changes |

The **Governance** document defines the rules. The **Matrix** documents the current state. The **Gap Assessment** audits whether the state conforms to the rules.

### 15.2 Machine-Readable Reports

Runners produce JSON reports per the canonical format (Section 10.3). These reports enable:

- **Trend analysis**: Track pass rates over time
- **Cross-framework dashboards**: Single view of all framework health
- **Automated alerting**: Detect regressions before humans review reports
- **CI gating**: Programmatic pass/fail decisions

Reports are written to paths specified by `--report <path>` flags:

| Framework | Report Path (Convention) |
|-----------|------------------------|
| UATS | `/tmp/api-report.json` |
| UPTS | `/tmp/parser-report.json` |
| UBTS | `/tmp/ubts-report.json` |
| UOTS | `/tmp/uots-report.json` |
| UDTS | `/tmp/udts-report.json` |
| UNTS | `/tmp/unts-report.json` |

### 15.3 Keeping Docs in Sync: Drift Checkers

Three automated scripts prevent documentation/reality divergence:

**1. Schema parity checker** (`scripts/verify_upts_schema_parity.py`):
- Verifies that every language in UPTS specs is in the schema's `language` enum
- Catches schema drift when new languages are added to specs but not the schema

**2. Canonical spec guard** (`scripts/verify_uxts_canonical_specs.py`):
- Validates that `specs/` directories contain only schema-conforming files
- Ensures non-canonical formats live in `drafts/`

**3. Framework drift checker** (`scripts/verify_uxts_drift.py`):
- Compares on-disk spec counts to documented counts
- Validates that every declared runner path exists
- Checks fixture existence for every fixture reference
- Verifies hash coverage (no empty or "PENDING" hashes)

All three are wired into the Makefile:

```makefile
test: test-parsers verify-uxts-canonical verify-uxts-drift
```

---

## 16. Brownfield Adoption

### 16.1 Discovery Protocol

When entering an existing codebase, the first step is systematic discovery of what verification needs exist and what infrastructure already exists.

**Operating mode determination:**

- `greenfield` only if ALL are true:
  - No `*.u?ts.json` files exist
  - No governance artifacts exist (`FRAMEWORK_GOVERNANCE.md`, `UXTS_FRAMEWORK_MATRIX.md`)
  - No CI jobs reference UxTS-like contract execution
- Otherwise → `brownfield`

**Deterministic discovery commands** (run in this order):

```bash
# 1. Baseline file inventory
rg --files > reports/uxts_discovery/01_files.txt

# 2. Existing UxTS/governance inventory
rg -n -S "UATS|UPTS|UBTS|USTS|UOTS|UOBS|UDTS|UVTS|UETS|UAMS|UNTS" . \
  > reports/uxts_discovery/02_uxts_inventory.txt

# 3. API/gRPC surface signals
rg -n -S "router\.|@app\.route|http\.HandleFunc|FastAPI\(|\.proto|grpc" . \
  > reports/uxts_discovery/03_interface_signals.txt

# 4. Parser/transformation signals
rg -n -S "parser|parse\(|AST|tree-sitter|symbol extractor" . \
  > reports/uxts_discovery/04_parser_signals.txt

# 5. Security/benchmark/quality signals
rg -n -S "auth|rbac|jwt|oauth|rate limit|latency|p95|p99|throughput|LLM|embedding" . \
  > reports/uxts_discovery/05_risk_signals.txt
```

Two agents running this procedure against the same codebase should produce the same framework candidates.

### 16.2 Opportunity Scoring

For each discovered recurring construct, score using this rubric (1-5 each):

| Dimension | Description | Score |
|-----------|-------------|-------|
| **Recurrence** | How many concrete instances exist | 1 (few) → 5 (many) |
| **Blast radius** | Impact if drift escapes | 1 (low) → 5 (catastrophic) |
| **Change frequency** | How often this area changes | 1 (stable) → 5 (every sprint) |
| **Compliance/security risk** | Regulatory or security consequence | 1 (none) → 5 (critical) |
| **Detection gap** | How weak current verification is | 1 (well-tested) → 5 (no tests) |

`priority_score = recurrence + blast_radius + change_frequency + compliance_risk + detection_gap`

Maximum: 25. High priority: ≥ 18.

**Tie-breaker order:** higher blast radius → higher compliance/security risk → higher recurrence.

### 16.3 Remediation Planning

After scoring, prioritize and plan remediation:

1. **Rank by score** — highest priority first
2. **Map to existing frameworks** — extend before creating
3. **Execute top-priority opportunities** — at least one high-priority (≥ 18) to `pilot` status in the current engagement
4. **Document waivers** for opportunities that can't be addressed (with blocker evidence, owner, due date, interim mitigation)

### 16.4 MDEMG Case Study

The MDEMG project was a brownfield adoption. The Gap Assessment (`docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md`) discovered 12 issues across Critical/High/Medium severity:

**Critical findings:**
- UPTS schema out of sync — 9 active languages not in schema enum
- UDTS had incompatible spec dialects (4/11 non-canonical)
- UVTS had split formats and runner crashes

**High findings:**
- UOBS logging checks were non-validating (0/0 false positives)
- UATS runner didn't enforce schema-runner parity
- UOBS/UOTS overlap unresolved
- UAMS fixture/runner packaging incomplete (phantom directory)

**Remediation results:**
- UPTS schema parity fixed, drift checker added
- Non-canonical UDTS specs moved to `drafts/`
- UOBS/UOTS authority split documented (runtime vs. artifact)
- UATS schema-runner parity enforcement added (fail-fast detection)
- UAMS reclassified from "active" to "spec-only"
- UVTS reclassified from "active" to "spec-only"
- Pass rate improved from 38.7% to 48.0% (36/75 variants passing) during remediation

---

## 17. Threat Model

For each framework, evaluate and document these risk categories:

### 17.1 Risk Categories

| Risk | Description | Mitigation |
|------|-------------|------------|
| **False pass** | Spec passes with zero real assertions, or runner ignores assertions | Schema-runner parity (Section 7), 0/0 prohibition in `build_result()` |
| **False fail** | Spec fails due to schema drift, environment sensitivity, or flaky assertions | Deterministic baselines, `${ENV_VAR}` resolution, fixture hash stability |
| **Coverage blind spot** | Domain has verification needs but no specs, or specs exist but no CI gate | Framework discovery audit (Section 16.1), CI drift checker |
| **Undetected changes** | Hash fields missing or not scanned; file modifications go unnoticed | Cross-framework hash scanner (UNTS), `integrity` summary in reports |
| **Fixture drift** | Fixture modified without developer review; test validates unreviewed inputs | Fixture hash in spec, `hash_mismatches` in report, CI integrity gate |
| **Dialect split** | Multiple incompatible spec formats under one framework | Canonical guard, `drafts/` separation (Section 12.3) |

### 17.2 Framework-Specific Risks

| Framework | Primary Risk | Specific Concern |
|-----------|-------------|-----------------|
| **UATS** | False pass from unimplemented fields | `setup`, `teardown`, `chain` in schema but not in runner |
| **UPTS** | Fixture drift | Parser output changes silently if fixture file is modified |
| **UBTS** | Ambiguous baseline | Benchmark thresholds must be spec-defined, not run-relative |
| **UOBS** | 0/0 false pass | Unimplemented check types (logging, tracing) must fail, not skip |
| **UOTS** | Coverage blind spot | Some check types (`log_format`, `trace_propagation`) unimplemented |
| **USTS** | False confidence | Pilot status means not all security assertions are enforced |
| **UAMS** | Phantom capability | Spec-only status means no verification actually occurs |
| **UDTS** | Dialect split | Hand-coded per-RPC tests, not fully spec-driven |
| **UVTS** | No verification | Spec-only with non-functional runner stub |

---

## 18. Acceptance Criteria

A repository is UxTS-governed when all 14 criteria are met:

### The Checklist

1. **All `active` frameworks satisfy the canonical framework contract** — schema exists, spec directory exists, runner exists and is executable, CI gate exists, ownership is defined, hash strategy is defined, schema-runner parity is documented.

2. **Schema-runner parity is documented and current for every framework** — every schema field classified as `enforced`, `advisory`, or hard-fail `unimplemented`.

3. **No `active` framework has silent false-pass behavior** — 0/0 passes are prohibited, ignored fields are prohibited.

4. **CI gates are declared and operational per framework status** — `active` frameworks have `soft` or `block` gates; `pilot` frameworks have at minimum `observe` gates.

5. **Hash scanner covers all declared hash-bearing artifacts** — spec hashes and fixture hashes are verified by runners.

6. **Automated drift detection runs in CI** — catches documentation/reality divergence (spec counts, runner existence, fixture existence).

7. **Framework overlap is resolved with documented authority splits** — no two frameworks claim the same domain without explicit ownership boundaries.

8. **Deprecated frameworks have migration plans** — successor framework identified, migration path documented.

9. **Every schema field in every `active` framework is classified** — `enforced`, `advisory`, or hard-fail `unimplemented`. No unclassified fields.

10. **Operating mode is explicitly declared** — `greenfield` or `brownfield` with evidence.

11. **In `brownfield` mode, deterministic discovery outputs are captured** — the five discovery commands have been run and outputs stored.

12. **In `brownfield` mode, a scored opportunity backlog exists and is current** — recurring constructs are scored and prioritized.

13. **In `brownfield` mode, at least one high-priority opportunity is implemented** — to `pilot` or higher, or an explicit waiver exists with owner and due date.

14. **Any new framework created in `brownfield` mode includes extension-first justification** — documented reason why extending an existing framework was insufficient.

---

# Appendices

---

## Appendix A: UATS Schema Reference

Complete field reference for the Universal API Test Schema v1.0.0.

Schema file: `docs/api/api-spec/uats/schema/uats.schema.json`

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `uats_version` | string | **Yes** | Schema version (semver pattern: `^[0-9]+\.[0-9]+\.[0-9]+$`) |
| `api` | object | **Yes** | API endpoint identification |
| `metadata` | object | No | Authorship, priority, and classification |
| `config` | object | No | Validation behavior (timeout, strictness, hash) |
| `auth` | object | No | Authentication configuration |
| `variables` | object | No | Template variables for request/response |
| `request` | object | **Yes** | HTTP request definition |
| `expected` | object | **Yes** | Expected response definition |
| `captures` | object | No | Value extraction from response |
| `setup` | array | No | Pre-request steps (schema-defined, not yet implemented) |
| `teardown` | array | No | Post-request steps (schema-defined, not yet implemented) |
| `variants` | array | No | Parameterized test variations |
| `chain` | array | No | Dependent request sequences (schema-defined, not yet implemented) |

### `api` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Endpoint or operation name |
| `version` | string | No | API version (e.g., `v1`, `2024-01`) |
| `base_url` | string | No | Base URL with `${ENV_VAR}` resolution. Overridden by `--base-url` CLI flag. |
| `service` | string | No | Service or microservice name |
| `operation_id` | string | No | OpenAPI operationId |
| `tags` | string[] | No | Categorization tags |

### `metadata` Object

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `author` | string | — | Spec author |
| `created` | string (date) | — | Creation date |
| `updated` | string (date) | — | Last update date |
| `description` | string | — | What this spec tests |
| `status` | enum | `active` | `draft`, `active`, `deprecated`, `disabled` |
| `priority` | enum | `medium` | `critical`, `high`, `medium`, `low` |
| `test_type` | enum | `contract` | `contract`, `integration`, `smoke`, `regression`, `load`, `security` |
| `requires` | string[] | — | Other UATS specs that must pass first |

### `config` Object

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `timeout_ms` | integer | 30000 | Request timeout in milliseconds |
| `response_time_max_ms` | integer | — | Fail if response exceeds this |
| `retry_count` | integer | 0 | Number of retry attempts |
| `retry_delay_ms` | integer | 1000 | Delay between retries |
| `strict_headers` | boolean | false | Fail on unexpected response headers |
| `strict_body` | boolean | false | Fail on unexpected response body fields |
| `ignore_fields` | string[] | — | JSONPaths to ignore (e.g., `$.timestamp`) |
| `ignore_headers` | string[] | — | Headers to ignore in validation |
| `validate_schema` | boolean | true | Validate spec against JSON Schema before execution |
| `validate_values` | boolean | false | Check exact values, not just types |
| `follow_redirects` | boolean | false | Follow HTTP redirects |
| `verify_ssl` | boolean | true | Verify SSL certificates |
| `sha256` | string | — | Integrity hash of this spec file |
| `tags` | string[] | — | Runner behavior tags (e.g., `embedding_required`) |

### `auth` Object

| Field | Type | Description |
|-------|------|-------------|
| `type` | enum | `none`, `bearer`, `basic`, `api_key`, `oauth2`, `custom` |
| `bearer.token` | string | Token value or `${ENV_VAR}` |
| `bearer.prefix` | string | Auth header prefix (default: `Bearer`) |
| `basic.username` | string | Basic auth username |
| `basic.password` | string | Basic auth password |
| `api_key.name` | string | Header or query parameter name |
| `api_key.value` | string | API key value |
| `api_key.in` | enum | `header` or `query` (default: `header`) |
| `oauth2.*` | object | OAuth2 configuration (not yet implemented in runner) |
| `custom.headers` | object | Custom authentication headers |

### `variables` Object

Variable values can be:

| Type | Example | Resolution |
|------|---------|------------|
| Static string | `"test_space": "demo"` | Direct substitution |
| Static number | `"count": 5` | Direct substitution |
| Static boolean | `"verbose": true` | Direct substitution |
| Env reference | `"token": { "env": "API_TOKEN", "default": "none" }` | Read from environment |
| Generator | `"id": { "generator": "uuid" }` | Generate at runtime |

Available generators: `uuid`, `timestamp`, `timestamp_ms`, `random_int`, `random_string`.

### `request` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `method` | enum | **Yes** | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS` |
| `path` | string | **Yes** | Endpoint path with optional `{param}` placeholders |
| `path_params` | object | No | Values for `{param}` in path |
| `query` | object | No | Query string parameters |
| `headers` | object | No | Request headers |
| `body` | any | No | Request body (object, array, or string) |
| `body_file` | object | No | External file for request body (not yet implemented) |
| `content_type` | string | `application/json` | Content-Type header |
| `sha256` | string | No | Hash of canonical request for integrity check |

### `expected` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status` | int or int[] or range | **Yes** | Expected HTTP status code(s) |
| `headers` | object | No | Expected response headers (string or Matcher) |
| `body` | any | No | Expected body (exact match after `ignore_fields`) |
| `body_file` | object | No | External file for expected body (not yet implemented) |
| `body_schema` | object | No | JSON Schema for response body (not yet implemented) |
| `body_assertions` | array | No | Fine-grained body validations (BodyAssertion[]) |
| `response_time` | object | No | `max_ms`, `p95_ms`, `p99_ms` thresholds |
| `sha256` | string | No | Hash of expected response for regression lock |

**Status formats:**

```json
// Single status code
"status": 200

// Multiple acceptable codes
"status": [200, 201]

// Status code range
"status": { "min": 200, "max": 299 }
```

### Body Assertion Operators (Canonical Dialect)

| Operator | Expected Type | Description |
|----------|--------------|-------------|
| `equals` | any | Exact value match |
| `not_equals` | any | Value must not equal |
| `contains` | string | String contains substring |
| `not_contains` | string | String must not contain |
| `matches` | string | Regex pattern match |
| `type_is` | string | Type check (`string`, `number`, `integer`, `boolean`, `array`, `object`, `null`) |
| `greater_than` | number | Numeric `>` comparison |
| `less_than` | number | Numeric `<` comparison |
| `greater_than_or_equal` | number | Numeric `>=` comparison |
| `less_than_or_equal` | number | Numeric `<=` comparison |
| `exists` | boolean | Field presence check |
| `not_exists` | boolean | Field absence check |
| `one_of` | array | Value must be in list |
| `length` | integer | Exact length (array or string) |
| `length_greater_than` | integer | Length `>` comparison |
| `length_less_than` | integer | Length `<` comparison |
| `items_all_match` | object | All array items must match nested matcher |
| `schema_match` | object | JSON Schema validation on path value |

### Legacy Dialect Matchers

| Matcher Key | Type | Description |
|-------------|------|-------------|
| `equals` | any | Exact value match |
| `not_equals` | any | Value must not equal |
| `contains` | string | Substring match |
| `not_contains` | string | Substring absence |
| `regex` | string | Regex pattern match |
| `type` | enum | Type check |
| `in` | array | Value in list |
| `range` | object | `{ "min": N, "max": N }` — numeric range |
| `length` | object | `{ "min": N, "max": N, "equals": N }` — length constraints |
| `exists` | boolean | Field presence |
| `schema` | object | JSON Schema validation |
| `schema_match` | object | JSON Schema validation (alias) |
| `items_all_match` | object | Array item matcher |

### `captures` Object

Extract values from responses for use in later assertions or variants:

```json
{
  "captures": {
    "created_id": {
      "from": "body",
      "path": "$.id"
    },
    "auth_token": {
      "from": "header",
      "path": "Authorization",
      "regex": "Bearer (.+)"
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `from` | enum | `body`, `header`, `status`, `response_time` (default: `body`) |
| `path` | string | JSONPath (for body) or header name |
| `regex` | string | Regex with capture group |

### `variants` Array

Each variant object supports:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Unique variant identifier |
| `description` | string | No | What this variant tests |
| `tags` | string[] | No | Additional tags |
| `skip` | boolean | No | Skip this variant (default: false) |
| `skip_reason` | string | No | Why skipped |
| `variables` | object | No | Variable overrides |
| `request` | object | No | Request overrides (deep-merged with base) |
| `expected` | object | No | Expected overrides (deep-merged with base) |

### `setup` / `teardown` Arrays (Not Yet Implemented)

Each Step object supports:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Step name |
| `type` | enum | `request`, `sql`, `command`, `wait`, `set` |
| `spec` | string | Path to another `.uats.json` |
| `request` | object | Inline HTTP request |
| `sql` | string | SQL statement |
| `command` | string | Shell command |
| `wait_ms` | integer | Wait duration |
| `set` | object | Set variables |
| `capture` | object | Extract values from step result |
| `condition` | string | Conditional execution expression |

> **Note:** `setup`, `teardown`, and `chain` are defined in the schema but not yet implemented in the runner. Specs that use these fields will fail with a parity error (Section 7).

---

## Appendix B: UPTS Schema Reference

Complete field reference for the Universal Parser Test Specification v1.0.0.

Schema file: `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json`

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `upts_version` | string | **Yes** | Schema version (const: `1.0.0`) |
| `language` | enum | **Yes** | Target language (30 values) |
| `variants` | string[] | No | File extensions (e.g., `[".py", ".pyi"]`) |
| `metadata` | object | No | Authorship and status |
| `config` | object | No | Tolerances and validation flags |
| `fixture` | object | **Yes** | Test input (file or inline) |
| `expected` | object | **Yes** | Expected parser output |
| `patterns_covered` | string[] | No | Pattern codes covered by this spec |

### Supported Languages

```
c, cpp, csharp, cuda, cypher, dockerfile, dotenv, go, graphql, ini,
java, javascript, json, jsonc, kotlin, lua, makefile, markdown, openapi,
protobuf, python, rust, scraper-markdown, shell, sql, terraform, toml,
typescript, xml, yaml
```

### `config` Object

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `line_tolerance` | integer | 2 | Accepted line number deviation (±N) |
| `require_all_symbols` | boolean | true | Fail if any expected symbol is missing |
| `allow_extra_symbols` | boolean | true | Whether parser output may contain symbols not in spec |
| `validate_signature` | boolean | false | Check symbol signature text |
| `validate_value` | boolean | false | Check constant literal values |
| `validate_parent` | boolean | true | Check parent/container relationships |

### `fixture` Object

Two modes:

**File-based** (references external fixture file):

```json
{
  "type": "file",
  "path": "../fixtures/python_test_fixture.py",
  "sha256": "8975b14f..."
}
```

**Inline** (fixture content embedded in spec):

```json
{
  "type": "inline",
  "content": "package main\n\nfunc Hello() string { return \"hello\" }",
  "filename": "test.go"
}
```

### `expected` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `symbol_count` | int or range | No | Expected symbol count or `{ "min": N, "max": N }` |
| `symbols` | Symbol[] | **Yes** | Array of expected symbols |
| `excluded` | object[] | No | Symbols that must NOT appear |
| `relationships` | Relationship[] | No | Inter-symbol relationships (not yet enforced) |

### Symbol Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Symbol name |
| `type` | enum | **Yes** | Symbol type (see below) |
| `line` | integer | **Yes** | Line number in fixture (minimum: 1) |
| `line_end` | integer | No | End line number |
| `exported` | boolean | No | Whether symbol is publicly visible |
| `parent` | string | No | Containing symbol name |
| `signature` | string | No | Full signature text |
| `signature_contains` | string[] | No | Substrings that must appear in signature |
| `value` | string | No | Literal value (for constants) |
| `doc_comment` | string | No | Documentation comment text |
| `decorators` | string[] | No | Applied decorators/annotations |
| `pattern` | string | No | Pattern code (e.g., `P1_CONSTANT`) |
| `optional` | boolean | No | If true, missing symbol is a warning not failure |
| `tags` | string[] | No | Categorization tags |

### Symbol Types

```
class, code_block, column, constant, constraint, device_function, endpoint,
enum, enum_value, field, function, heading, index, interface, kernel, label,
link, macro, method, module, namespace, package-reference, parameter,
relationship_type, section, struct, table, target, trait, trigger, type,
variable, view
```

### Relationship Object (Not Yet Enforced)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | **Yes** | Source symbol name |
| `relation` | enum | **Yes** | `DEFINES_METHOD`, `EXTENDS`, `IMPLEMENTS`, `IMPORTS`, `CONTAINS` |
| `target` | string | **Yes** | Target symbol name |

> **Note:** The `relationships` field is defined in the schema but not yet enforced by the runner. Specs that use it will fail with a parity error.

### Pattern Codes

```
P10_SECTION, P1_CONSTANT, P2_FUNCTION, P3_CLASS_STRUCT, P4_INTERFACE_TRAIT,
P5_ENUM, P5_ENUM_VALUE, P5_NAMESPACE, P6_METHOD, P7_TYPE_ALIAS, P8_ENUM,
P8_ENUM_VALUE, P9_FIELD, P_CODE_BLOCK, P_DIRECTIVE, P_EXTEND_TYPE, P_FIELD,
P_HEADING_H1, P_HEADING_H2, P_HEADING_H3, P_INPUT, P_LINK, P_ROOT_OPERATION,
P_SCALAR, P_UNION
```

---

## Appendix C: Quick Reference Card

### Daily Commands

```bash
# Run all UATS API tests
make test-api

# Run single API endpoint test
make test-api-health
make test-api-retrieve

# Run all UPTS parser tests
make test-parsers

# Run single parser test
make test-parser-python
make test-parser-go

# Run smoke tests only
make test-smoke

# Run all tests (UATS + UPTS)
make test-all

# Run UBTS benchmark (smoke)
make test-ubts-smoke

# Run UOTS observability tests
make test-uots

# Run UDTS gRPC tests
make test-udts

# Verify governance consistency
make verify-uxts-canonical
make verify-uxts-drift
make verify-upts-schema
```

### Key Paths

| What | Path |
|------|------|
| UATS specs | `docs/api/api-spec/uats/specs/*.uats.json` |
| UATS schema | `docs/api/api-spec/uats/schema/uats.schema.json` |
| UATS runner | `docs/api/api-spec/uats/runners/uats_runner.py` |
| UPTS specs | `docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json` |
| UPTS schema | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` |
| UPTS runner | `docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py` |
| UPTS fixtures | `docs/lang-parser/lang-parse-spec/upts/fixtures/` |
| Shared report lib | `docs/tests/uxts_report.py` |
| Shared core lib | `docs/tests/uxts_runner_core.py` |
| Governance policy | `docs/specs/FRAMEWORK_GOVERNANCE.md` |
| Framework inventory | `docs/development/UXTS_FRAMEWORK_MATRIX.md` |
| Gap assessment | `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md` |
| Portable agent spec | `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` |
| CI workflow (main) | `.github/workflows/ci.yml` |
| CI workflow (parsers) | `.github/workflows/parser-tests.yml` |
| Makefile | `Makefile` |
| Schema parity checker | `scripts/verify_upts_schema_parity.py` |
| Canonical spec guard | `scripts/verify_uxts_canonical_specs.py` |
| Drift checker | `scripts/verify_uxts_drift.py` |

### Naming Conventions

| Convention | Pattern | Example |
|------------|---------|---------|
| Spec files | `<name>.<framework>.json` | `health.uats.json` |
| Schema files | `<framework>.schema.json` | `uats.schema.json` |
| Runner files | `<framework>_runner.py` | `uats_runner.py` |
| Report files | `<framework>-report.json` | `api-report.json` |
| Makefile targets | `test-<framework>` | `test-api`, `test-parsers` |
| CI workflows | `<domain>-tests.yml` | `parser-tests.yml` |

### Report Format Quick Reference

```json
{
  "timestamp": "ISO 8601",
  "framework": "lowercase framework name",
  "framework_version": "semver",
  "summary": {
    "total_specs": 0,
    "passed": 0, "failed": 0, "skipped": 0, "errors": 0,
    "pass_rate": 0.0,
    "duration_ms": 0.0
  },
  "integrity": {
    "total_hashed": 0, "verified": 0, "mismatched": 0, "no_hash": 0
  },
  "results": [{
    "spec_path": "relative/path",
    "status": "pass|fail|skip|error",
    "duration_ms": 0.0,
    "hash_verified": true|false|null,
    "hash_mismatches": [],
    "assertions_evaluated": 0,
    "assertions_passed": 0,
    "failures": [],
    "warnings": [],
    "error": null
  }]
}
```

---

## Appendix D: Glossary

| Term | Definition |
|------|------------|
| **UxTS** | Universal-x Test Specification — the methodology for declarative verification governance |
| **Framework** | The combination of schema + specs + runner + CI wiring for one concern domain |
| **Spec** | A declarative JSON file defining a single verifiable contract |
| **Schema** | A JSON Schema defining valid spec structure for a framework |
| **Runner** | An executable program that reads specs and reports pass/fail results |
| **Fixture** | A static input file referenced by a spec for testing |
| **Gate mode** | CI behavior when runner reports failures: `block`, `soft`, or `observe` |
| **Framework status** | Maturity level: `spec-only`, `pilot`, `active`, `deprecated` |
| **Schema-runner parity** | The invariant that every schema field is classified as enforced, advisory, or hard-fail unimplemented |
| **Canonical dialect** | The preferred assertion format using `path` + `op` + `expected` |
| **Legacy dialect** | The backward-compatible assertion format using `path` + specific matcher keys |
| **0/0 false pass** | A spec that passes with zero assertions evaluated — prohibited by `build_result()` |
| **Parity failure** | A spec failure caused by using fields the runner doesn't implement |
| **Hash verification** | SHA256 integrity check on spec and fixture files — independent from assertion results |
| **Deep merge** | Variant override semantics where variant fields are recursively merged with base spec |
| **Canonical report** | The standardized JSON report format all runners must produce (Section 10.3) |
| **Drift checker** | Automated script that validates on-disk reality matches documented state |
| **Canonical guard** | Automated script that validates `specs/` directories contain only schema-conforming files |
| **Brownfield** | Adoption mode for codebases with existing code and partial UxTS artifacts |
| **Greenfield** | Adoption mode for codebases with no existing UxTS artifacts |
| **Authority split** | When two frameworks cover adjacent domains, the explicit documentation of what each owns |
| **Tag filtering** | Runner CLI flags (`--include-tag`, `--exclude-tag`) that control which specs execute |
| **Portable agent spec** | The specification that tells AI coding agents how to implement UxTS governance |

---

## Appendix E: Reference Documents

### Primary Specifications

| Document | Path | Description |
|----------|------|-------------|
| UxTS Portable Agent Spec | `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` | Comprehensive specification for implementing UxTS governance in any codebase. Covers architecture, bootstrap, parity, lifecycle, anti-patterns, and acceptance criteria. Version 2.3.0-draft. |
| Framework Governance | `docs/specs/FRAMEWORK_GOVERNANCE.md` | Policy document governing all 11 frameworks. Defines ownership, lifecycle rules, per-framework policies, and authority splits. |
| Framework Matrix | `docs/development/UXTS_FRAMEWORK_MATRIX.md` | Operational inventory mapping each framework to its schema, specs, runner, CI, and parity status. The source of truth for "what exists." |

### Gap Analysis and Assessment

| Document | Path | Description |
|----------|------|-------------|
| Framework Gap Assessment | `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md` | Point-in-time audit (Feb 26, 2026) identifying 12 issues across Critical/High/Medium severity. Source material for the anti-patterns section. |

### Schemas

| Framework | Schema Path |
|-----------|-------------|
| UATS | `docs/api/api-spec/uats/schema/uats.schema.json` |
| UPTS | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` |
| UBTS | `docs/tests/ubts/schema/ubts.schema.json` |
| UETS | `docs/tests/uets/schema/uets.schema.json` |
| UOBS | `docs/tests/uobs/schema/uobs.schema.json` |
| UOTS | `docs/api/api-spec/uots/schema/uots.schema.json` |
| UDTS | `docs/api/api-spec/udts/schema/udts.schema.json` |
| USTS | `docs/tests/usts/schema/usts.schema.json` |
| UAMS | `docs/tests/uams/schema/uams.schema.json` |
| UVTS | `docs/tests/uvts/schema/uvts.schema.json` |

### Runners

| Framework | Runner Path |
|-----------|-------------|
| UATS | `docs/api/api-spec/uats/runners/uats_runner.py` |
| UPTS | `docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py` |
| UBTS | `docs/tests/ubts/runners/ubts_runner.py` |
| UETS | `docs/tests/uets/runners/uets_runner.py` |
| UOBS | `docs/tests/uobs/runners/uobs_runner.py` |
| UOTS | `docs/api/api-spec/uots/runners/uots_runner.py` |
| USTS | `docs/tests/usts/runners/usts_runner.py` |
| UDTS | `docs/api/api-spec/udts/runners/udts_runner.py` |

### Shared Infrastructure

| File | Path | Description |
|------|------|-------------|
| Report builder | `docs/tests/uxts_report.py` | Canonical report construction with 0/0 protection |
| Runner core | `docs/tests/uxts_runner_core.py` | Hash computation, JSON serialization, status normalization |

### CI Workflows

| Workflow | Path | Frameworks Covered |
|----------|------|--------------------|
| Main CI | `.github/workflows/ci.yml` | UATS (block), UBTS (soft-fail) |
| Parser Tests | `.github/workflows/parser-tests.yml` | UPTS (block) |
| Canonical Guard | `.github/workflows/uxts-canonical-specs.yml` | UDTS, UVTS dialect enforcement |

### Automation Scripts

| Script | Path | Purpose |
|--------|------|---------|
| Schema parity | `scripts/verify_upts_schema_parity.py` | UPTS schema/spec language enum sync |
| Canonical guard | `scripts/verify_uxts_canonical_specs.py` | Reject non-canonical specs in `specs/` dirs |
| Drift checker | `scripts/verify_uxts_drift.py` | Validate on-disk reality vs. documented state |
| Sidecar schemas | `scripts/verify_sidecar_schemas.py` | Validate sidecar JSON fixtures against schemas |
| UNTS report | `scripts/unts_report_adapter.py` | Generate UNTS Section 8A report from registry |

### Project Context

| Document | Path | Description |
|----------|------|-------------|
| Project Vision | `VISION.md` | MDEMG architectural philosophy, design principles, and success metrics |
| Agent Handoff | `AGENT_HANDOFF.md` | Phase artifact index linking every development phase to its docs and specs |

---

*This guide is the authoritative reference for UxTS in the MDEMG project. For the specification that AI coding agents follow when implementing UxTS governance, see `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md`. For the current framework state, see `docs/development/UXTS_FRAMEWORK_MATRIX.md`.*
