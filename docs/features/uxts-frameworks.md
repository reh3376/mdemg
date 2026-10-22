# The UxTS Framework Family

**Audience**: engineer or peer AI onboarding to MDEMG's testing + governance stack.
**Scope**: what UxTS is, why it exists, how one works, how the 16 shipped frameworks map to the problem each solves.
**Authoritative source**: `docs/development/UXTS_FRAMEWORK_MATRIX.md` — if any count or disposition here diverges from the matrix, the matrix wins. This doc is the narrative; the matrix is the index.

---

## 1 What UxTS is

UxTS is a **family of versioned JSON spec frameworks** that encode machine-readable contracts for distinct correctness surfaces in MDEMG. Each framework shares a shape:

```
schema.json           ← what a spec MUST look like
specs/*.<family>.json ← one spec per contract
runners/*_runner.py   ← executes specs against the live system
ci.yml step           ← merge-blocking or soft-fail check
UxTS_FRAMEWORK_MATRIX ← inventory + CI disposition entry
```

The `x` is a letter identifying the correctness surface: **A**PI, **P**arser, **B**enchmark, **O**bservability, **V**alidation (retrieval-quality), **L**LM task, **T**raining **D**ata, **A**I **T**raining data (curation), **E**mergence, **S**ecurity, **I**terative-improvement, **D**ev**S**pace (gRPC), **N**umeric (hash-verification), **AM**S (auth-method). UBENCH is the odd one out: it wraps the Phase-10 LLM-eval benchmark in the UxTS pattern but keeps its own name.

**16 shipped** today (per matrix §1). CLAUDE.md's §Testing block cites "15" — pre-UBENCH or pre-UAITS drift that will be corrected in the next doc-currency pass.

## 2 Why it exists

UxTS is a **forcing function**. The problem it solves is this class of failure: a contract exists in operator intent, drifts silently from implementation, and only surfaces as production damage months later.

Four load-bearing examples from the actual incident log:

- **UATS (HTTP endpoints)** — pre-UATS, breaking an endpoint's response shape required reading every caller. Post-UATS, every endpoint has a frozen contract in `*.uats.json` with `sha256`-pinned content; `validate-all` runs in CI merge-blocking.
- **ULTS (LLM prompts)** — pre-ULTS, editing an LLM system prompt silently invalidated every eval against that prompt and every row of training data derived from it. Post-ULTS, each prompt carries a `system_prompt_hash`; CI's `--verify-hashes` step fails any merge that changes a prompt without re-pinning the hash + re-generating training rows in the same PR.
- **UPTS (28 languages)** — pre-UPTS, a tree-sitter query edit silently regressed symbol extraction for a language with no tests. Post-UPTS, every supported language has a fixture + a UPTS spec enumerating expected symbols; CI runs the full suite on every push.
- **UOTS (observability artifacts)** — pre-UOTS, a dashboard JSON edit could remove a required panel and no one would notice until the next incident needed it. Post-UOTS, every operator-facing Grafana dashboard + alert rule file is spec-pinned.

Evidence basis for the forcing-function pattern: CLAUDE.md UXTS-CI-001 (2026-06-11) un-zombied five previously-skipped CI gates and surfaced "stale Grafana spec, 4 prompt drifts, 2 silently-failing neural tests, 51-day-stale matrix" on the first clean run. The hidden-rot-rate was non-zero; the policy pin is "Don't add `continue-on-error` to test steps".

## 3 The 16 shipped frameworks

Grouped by correctness surface (per matrix §1 + §2):

### Endpoint & Protocol Contracts

| Framework | What it pins | Specs | CI |
|---|---|---|---|
| **UATS** | HTTP endpoint acceptance (request shape, status, body assertions, variants, timeout) | 215 canonical + 7 drafts | merge-blocking |
| **UDTS** | gRPC devspace contract (hand-coded per RPC — not JSON-driven) | 15 canonical + 4 drafts | canonical dialect guard |
| **UPTS** | Language parser conformance (28 shipped: c, cpp, csharp, cuda, cypher, dockerfile, go, graphql, ini, java, json, kotlin, lua, makefile, markdown, openapi, php, protobuf, python, rust, scraper_markdown, shell, sql, terraform, toml, typescript, xml, yaml) | 28 | CI-gated in `parser-tests.yml` |

### Behavior Quality & Benchmarking

| Framework | What it pins | Specs | CI |
|---|---|---|---|
| **UVTS** | Semantic retrieval quality — 120q graded corpus with per-category A/B comparison + Note 02 regression gate | 3 canonical + 1 draft | **live-gated by design** (no CI step — stub-embedder grading measures nothing) |
| **UBTS** | Throughput/latency/load thresholds | 3 specs + 3 profiles | soft-fail smoke gate |
| **UBENCH** | Aggregate LLM-eval benchmark wrapping Phase 10 (dataset↔holdout contract: every ULTS spec has ≥ `min_rows_per_task` golden rows) | 1 (108 rows / 17 tasks) | merge-blocking lint + contract |

### LLM & Training Governance

| Framework | What it pins | Specs | CI |
|---|---|---|---|
| **ULTS** | LLM task contracts: prompts (sha256-hashed), schemas, quality metrics, training config | 18 | merge-blocking `--verify-hashes` (prompt-drift tripwire) |
| **UAITS** | Training data curation governance across 4 paradigms (SFT, DPO, RAFT, curriculum) + quality gates + format rules + temporal-split invariant | 1 spec / 4 datasets | drift checker |
| **UTDS** | Training data export manifests, privacy hard gates (violations=0), SHA checksums, archive integrity | 3 | drift checker |
| **UETS** | LLM emergence concept-naming quality (does the model produce useful names for emergent clusters?) | 8 | soft-fail |
| **UITS** | T1 encoding comprehension validation (does the agent correctly parse J17 pipe-coded guidance?) | 11 | manual |

### Observability

Two frameworks with a deliberate authority split (per matrix §4):

| Framework | What it pins | Scope | Specs | CI |
|---|---|---|---|---|
| **UOBS** | Live runtime observability behavior (health probes, dependency checks, tracing headers, metrics presence over wire) | Requires running server | 11 canonical + 1 draft | soft-fail |
| **UOTS** | Artifact-level observability (dashboard JSON shape, alert rule YAML, metric registry definitions, backup manifests) | Static files on disk | 11 | merge-blocking |

### Security & Auth

| Framework | What it pins | Specs | CI |
|---|---|---|---|
| **USTS** | Security behavior & hardening tests (rate-limiting, data-exposure, injection) | 5 canonical + 2 drafts | merge-blocking |
| **UAMS** | Auth method contracts & conformance | 4 (spec-only, no runner) | no CI gate |

### Integrity Infrastructure

| Framework | What it pins | Specs | CI |
|---|---|---|---|
| **UNTS** | Hash integrity registry — scans all 8 primary UxTS frameworks + MDEMG manifest, verifies SHA-256s, drives revert workflow | registry (not spec-shaped) | no dedicated gate (invoked by other verifiers) |

## 4 Anatomy of a UxTS spec

Example (UATS, trimmed from `docs/api/api-spec/uats/specs/node_get.uats.json` shipped 2026-10-14):

```json
{
  "uats_version": "1.0.0",
  "api": {
    "name": "Node Get",
    "base_url": "${MDEMG_BASE_URL}",
    "service": "mdemg",
    "tags": ["memory", "node", "provenance", "j17"]
  },
  "metadata": {
    "author": "reh3376",
    "created": "2026-10-14",
    "description": "...",
    "test_type": "contract",
    "priority": "medium"
  },
  "config": {
    "timeout_ms": 10000,
    "sha256": "b5ad3387c7aaddff1b58f572d6034a627fbd108e7dbbb8d7da6180cc0146c7ea"
  },
  "variables": { "test_space": "mdemg-dev" },
  "request": {
    "method": "GET",
    "path": "/v1/memory/nodes/aaaabbbbccccddddeeee",
    "query": { "space_id": "{{test_space}}" }
  },
  "expected": {
    "status": 404,
    "body_assertions": [
      { "path": "$.error",   "op": "equals", "expected": "node not found" },
      { "path": "$.node_id", "op": "equals", "expected": "aaaabbbbccccddddeeee" }
    ]
  },
  "variants": [
    { "name": "missing_space_id", "request": {...}, "expected": { "status": 400 } },
    { "name": "method_not_allowed_put", "request": { "method": "PUT", ... }, "expected": { "status": 405 } }
  ]
}
```

Four load-bearing invariants:

1. **`<framework>_version`** — spec-schema version. Runner refuses to execute mismatched specs.
2. **`config.sha256`** — SHA-256 of the spec content excluding the sha256 field itself. Added via `<framework>_runner.py add-hashes --spec-dir ...`; verified every run. Catches a hand-edit that forgot to re-hash.
3. **`variants`** — deep-merged over the base body. ⚠️ **UATS-GAP-001 pin**: required-field guards MUST use empty-string overrides, never key omission. An omitted key inherits the base value and the variant silently asserts nothing. Live-caught in `node_get`'s `missing_space_id` variant on 2026-10-14 — the fix is `"query": {"space_id": ""}`.
4. **`metadata.author` + `created`** — provenance. Not load-bearing for correctness, load-bearing for drift triage.

Every framework's schema lives at `<path>/schema/<family>.schema.json`. Every runner has at least four subcommands: `validate` (single spec, live), `validate-all` (whole dir), `add-hashes` (compute + persist sha256), `verify-hashes` (CI gate — refuses merge if any spec was edited without re-hash).

## 5 How a UxTS runner executes

The HTTP-family runner (UATS) illustrates the general shape:

```
load spec → verify sha256 → substitute {{variables}} + ${ENV_VARS}
  → compose base + each variant
    → send HTTP request (per request block)
      → compare response status
        → walk body_assertions (JSONPath against response)
          → emit CheckResult per assertion
            → aggregate into UATSResult (pass/fail/error/skip)
              → write report
```

Variations by framework:
- **UPTS** feeds a fixture file to the parser, expects a `*.golden.json` of extracted symbols, diffs field-by-field (line-tolerance, signature validation, parent-chain checks).
- **UBENCH** calls the production LLM endpoint `--rows-per-spec N` times, scores each row via `neural.training.reward_functions.REWARD_REGISTRY` (deterministic rewards; optional LLM judge via `gpt-5.4-mini` with fixed seed), aggregates per-task + overall.
- **UVTS** sends the 120q corpus against `/v1/memory/retrieve`, scores correct-file + answer-relevance + confidence calibration, computes mean + stddev + improvements/regressions, writes V0016 `uvts_runs` + `uvts_results` TSDB rows, runs the Note-02 A/B gate.
- **UOBS** probes the running server (metrics endpoint shape, health transitions, dependency reachability).
- **UOTS** reads static artifact files from disk — no server needed except for `prometheus_metrics` type.

Each runner writes a report (JSON + stdout summary) and exits non-zero on fail.

## 6 The hash-pin + verify-hashes contract — the drift tripwire

Every UxTS framework that pins content embeds a SHA-256 in `config.sha256`. The hash is computed over the spec content with the `sha256` field itself zeroed/removed (so the hash is a function of what the spec *asserts*, not what it *claims to assert*).

CI's merge gate on hash-verification works as follows:

1. Developer edits a spec (change an assertion, add a variant, bump a threshold).
2. CI runs `<framework>_runner.py verify-hashes --spec-dir ...`.
3. The runner recomputes the hash + compares to the stored one.
4. Mismatch → non-zero exit → merge blocked.
5. Developer runs `add-hashes` locally, commits the regenerated hash in the same PR, pushes.

**Why this matters**: without the hash gate, an operator can edit an assertion in isolation and ship a "contract" that doesn't match any prior PR's accepted shape. The hash makes "I changed the contract" an explicit commit, not an invisible edit. Same forcing-function shape as git signed-off-by.

ULTS applies the same contract to LLM system prompts: `system_prompt_hash` in each `*.ults.json`. If you edit `internal/jiminy/outcome_classifier.go::classifySystemPrompt`, you MUST re-pin the hash in `docs/tests/ults/specs/jiminy_classify_outcome.ults.json` — and because that commit forces re-pinning, every eval + every training-row from the old prompt gets quarantined for the review.

## 7 The DORMANT-CENSUS adjudication registries — sibling, not UxTS

Three JSON audit registries (per matrix §5a) sit alongside UxTS as **build-time drift checks** — they share the forcing-function pattern but validate a different question. UxTS asks "does the current implementation match a frozen contract?" The DORMANT-CENSUS family asks "does every shipped declaration have at least one consumer?"

| Registry | Scope |
|---|---|
| DORMANT-CENSUS-001 | HTTP routes — every registered route adjudicated in `routes_consumer_inventory.json` |
| DORMANT-CENSUS-002 | TSDB tables — every migration-created table adjudicated |
| DORMANT-CENSUS-003 | Metrics registry (counters/gauges/histograms + their 5 histogram derivatives) adjudicated |

Each has a `scripts/verify_*.py` grep-based verifier + a merge-blocking CI step. Adding a new declaration without adjudication fails CI. Pre-commit hook (`.claude/hooks/pre-bash-check.py`) blocks `git commit` locally when a trigger-file is staged without its paired inventory update (PRE-COMMIT-INVENTORY-GATE-001).

Why they're not UxTS: UxTS specs are per-contract (one HTTP endpoint, one LLM task, one dashboard); DORMANT-CENSUS inventories are per-FAMILY aggregates with a different consumer-presence invariant. Shape is different enough that overloading `*.uats.json` to carry "and here's who consumes me" would blur both.

## 8 UOBS vs UOTS — the authority split

A recurring confusion worth pinning (per matrix §4):

- **UOBS** = "does the running system actually emit the right metric names, serve `/healthz` correctly, propagate trace headers?" Needs a live server. Validates *behavior*.
- **UOTS** = "does the Grafana dashboard JSON on disk have the required panels + reference the right datasource UID + does the alert rule YAML parse?" Needs only the filesystem. Validates *artifact structure*.

The practical rule: if the spec can be written without running the server, it's UOTS. If it requires an HTTP call to observe the answer, it's UOBS. The Grafana dashboard JSON that defines a panel is UOTS; the running Grafana instance serving data to that panel is NOT in scope for either (operators verify via `grafana_admin` ad hoc).

## 9 When to add a new framework

Framework-authoring is a last resort. Three cheaper options first:

1. **Add a spec to an existing framework.** If the new contract is "another HTTP endpoint", it's a UATS spec, not a new framework. 215 UATS specs demonstrate the framework scales.
2. **Add a Go pin test.** For single-file content pins (static docs, config constants, embedded JSON), a `internal/<name>pin/*_test.go` matches the shipped `internal/grafanapin` + `internal/j17pin` shape — zero new framework surface, zero runtime dependency. Right-sized when the correctness surface is one file.
3. **Add a DORMANT-CENSUS-style registry.** For "every declaration must have a consumer" invariants on a NEW declaration surface, mint a sibling registry — don't fork UxTS.

Mint a new UxTS framework ONLY when all three apply:
- The contract shape is **generic** (will have ≥3 instances within a quarter)
- The contract has **structural invariants** that benefit from a JSON schema (not just a hash)
- The contract has a **runner** that can execute the spec against the real system (static-only pins belong in a Go pin test)

The matrix §5 shows nine gaps resolved + three still open (UOBS `logging` type unimplemented, K8s/Helm manifests lack UOTS coverage, UITS/UETS/UTDS/UAITS have no CI gate yet). Framework surface is deliberately kept small — 16 is the correct working count, not a target to grow.

## 10 References

- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — canonical inventory (authoritative; this doc is the narrative)
- `docs/guides/UXTS_DEVELOPER_GUIDE.md` — comprehensive methodology guide (how to author, run, hash, CI-wire)
- `docs/specs/FRAMEWORK_GOVERNANCE.md` — governance spine (what counts as "shipped" / "spec-only" / "draft")
- `docs/specs/unts-hash-verification.md` — UNTS hash scanner + revert workflow
- `docs/api/api-spec/uats/README.md` — UATS entry point + runner usage
- `docs/api/api-spec/udts/README.md` — UDTS (gRPC) entry point
- `docs/lang-parser/lang-parse-spec/upts/README.md` — UPTS (28 languages)
- `docs/tests/uobs/README.md` + `docs/api/api-spec/uots/README.md` — the observability pair
- `docs/tests/uams/README.md` — UAMS
- `docs/tests/uits/README.md` — T1 encoding comprehension
- `docs/tests/uaits/README.md` + `docs/features/uaits-framework.md` — UAITS curation governance
- `docs/tests/ubench/specs/mdemg.ubench.json` — the shipped Phase-10 benchmark wrapped as UBENCH
- `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md` — the gap audit that produced the current CI-coverage shape
- `.github/workflows/ci.yml` — merge-blocking step wiring
- `.github/workflows/parser-tests.yml` — UPTS CI
- CLAUDE.md § "Testing" (UXTS-CI-001, UATS-GAP-001, HOOKSYNC-001) — operator-authored pins that explain *why* each gate exists in its current shape
