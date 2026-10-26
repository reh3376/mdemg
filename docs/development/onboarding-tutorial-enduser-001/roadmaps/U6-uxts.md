# U6 — UxTS — The Forcing-Function Family (9 slides)

**Intent**: UxTS (Universal x Test Specification) is MDEMG's forcing-function framework pattern — one meta-pattern enforced across 16 shipped families covering APIs, parsers, retrieval quality, LLM tasks, observability, security, benchmarks, etc. Each family is a contract-first schema + specs + runner + CI gate. This section covers the pattern, the full family inventory, how operators interact, and the drift-gate principle.
**Target slides**: 9
**Mermaid**: 1 (U6.2 — forcing-function diamond)
**Natural Q&A anchor**: after U6

---

## U6.1 — Section Landing

**Title**: `U6 · UxTS — Forcing-Functions That Prevent Drift`

**Body**:
- Intent: "A UxTS framework is a contract-first way to make a piece of system behavior impossible to drift without CI noticing. It's spec → fixture → runner → CI gate. There are 16 of these shipped covering APIs, parsers, retrieval quality, LLM tasks, observability, security, benchmarks, training data, auth."
- Preview: forcing-function diamond → full family inventory → how operators invoke → drift-gate principle → trajectory → gotchas
- Points back to U1.5 "How does it continuously improve?" — this is loop #2 (prevents regression as the system evolves)

**Provenance**:
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — the single source of truth
- CLAUDE.md "UATS-GAP-001" + "UXTS-CI-001"

**note**:
- Operator cue: "If a sprint ships a feature + the matching UxTS contract, the feature CAN'T silently regress. If the matching contract is missing, that's the follow-up to open."

---

## U6.2 — Mental Model: The Forcing-Function Diamond

**Title**: `How UxTS Prevents Drift`

**Body**:
- **Mermaid #6** (diamond — the 4-element pattern):
  - **Spec** (JSON Schema) → defines WHAT a valid contract looks like
  - **Fixtures / Specs instances** → concrete examples that MUST pass the schema
  - **Runner** → executes the specs against the live system (or static artifacts) and emits pass/fail
  - **CI gate** → fails merge if any shipped spec regresses
- The flow is uni-directional: a code change that breaks a contract fails CI; a contract change that doesn't update the spec fails CI; a new feature without a contract gets caught by UATS-GAP-001 class rules
- Why it works: the contract is HUMAN-READABLE JSON, maintained in-repo, and the runner is deterministic — operators + reviewers can verify without re-running the whole system

**Provenance**:
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` §1-§3
- CLAUDE.md "Testing — Live System Testing Is Required"

**note**:
- Operator cue: "Every UxTS family is this shape. Learn one, you know them all."

---

## U6.3 — The 16 Shipped Families

**Title**: `The UxTS Family — All 16`

**Body**:
- Full table (grouped by domain):

| Family | Full Name | Scope | Status | Specs |
|---|---|---|---|---:|
| UATS | API Test Specification | HTTP endpoint acceptance | active, CI-gated | 215 |
| UPTS | Parser Test Specification | Language parser conformance | active, CI-gated | 28 |
| UDTS | DevSpace Test Specification | gRPC contract tests | active | 15 |
| UOTS | Observability Test Specification | Observability artifact contracts | active, CI-gated | 11 |
| UOBS | Observability Specification | Runtime observability behavior | active | 11 |
| UVTS | Validation Test Specification | Semantic retrieval quality | active, live-gated | 3 |
| ULTS | LLM Task Specification | LLM task prompt + schema contracts | active, CI-gated hash | 18 |
| UBTS | Benchmark Test Specification | Throughput/latency benchmarks | active | 3 |
| UBENCH | Benchmark Framework (LLM eval) | Aggregate LLM eval (wraps Phase 10) | active, CI-gated | 1 (108 rows / 17 tasks) |
| USTS | Security Test Specification | Security behavior | active, CI-gated | 5 |
| UAMS | Auth Method Specification | Auth method contracts | spec-only | 4 |
| UETS | Emergence Test Specification | LLM emergence concept-naming | active | 8 |
| UITS | Iterative-Improvement Test Specification | T1 encoding comprehension | active | 11 |
| UTDS | Training Data Specification | Training data export manifest | active | 3 |
| UAITS | AI Training Specification | Training data curation | active | 1 (4 datasets) |
| UNTS | Hash Test Specification | Hash integrity registry | active | N/A registry |

- Totals: **16 families · 15 active · 1 spec-only (UAMS)**

**Provenance**:
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` §4 Framework Inventory

**note**:
- Operator cue: "You WILL interact with these as merge-gate failures. The matrix is the map; this slide is the TL;DR."

---

## U6.4 — How To Invoke A Runner

**Title**: `Running A UxTS Suite`

**Body**:
- Convention: `make test-<family>-lint` + `make test-<family>-run` for most families
- Examples:
  ```bash
  $ make test-api BASE_URL=http://localhost:9999   # UATS 215 specs
  $ make test-ults-verify-hashes                   # ULTS prompt drift check
  $ make test-ubench-contract                      # UBENCH dataset-vs-holdout contract
  $ make test-uvts-quick BASE_URL=http://localhost:9999   # UVTS 16q semantic retrieval
  $ python3 docs/api/api-spec/upts/runners/upts_runner.py   # UPTS parser conformance
  ```
- Lint mode: validates spec files against the schema (no live deps; CI-safe)
- Run mode: executes against the live system; some families require specific services up (UATS needs the server, UVTS needs the server + TSDB)

**Provenance**:
- `Makefile` test-* targets
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` §5 Runner Inventory

**note**:
- Operator cue: "If a sprint touches a system behavior, run the matching UxTS family's lint + run BEFORE pushing. CI will catch it anyway, but faster locally."

---

## U6.5 — Drift-Gate Principle

**Title**: `Why UxTS Catches Drift CI Would Otherwise Miss`

**Body**:
- Three drift classes UxTS catches:
  1. **Prompt drift** — ULTS `--verify-hashes` detects when an LLM system prompt changed without pinning the new hash in the task's `.ults.json`
  2. **Spec drift** — UATS runs 215 real HTTP endpoints; a response-shape change that breaks a documented contract fails merge
  3. **Framework drift** — the UxTS drift checker enforces that adding a NEW family or spec updates `UXTS_FRAMEWORK_MATRIX.md` in the same PR (UATS-GAP-001 class)
- UXTS-CI-001 (2026-06-11) un-zombied the gates: UATS, UNTS hash-verify, USTS, UOTS, ULTS `--verify-hashes`, UBENCH contract, neural pytest+ruff, UxTS matrix drift are ALL merge-blocking today
- The forcing-function invariant: don't add `continue-on-error` to a UxTS CI step; every zombie this project un-zombied was hiding real rot

**Provenance**:
- CLAUDE.md "UXTS-CI-001" + "UATS-GAP-001"
- `.github/workflows/ci.yml`

**note**:
- Operator cue: "A failing UxTS gate on your PR is a GIFT. Fix the drift; don't bypass the gate."

---

## U6.6 — Where UxTS Shows Up In The UI

**Title**: `UxTS In /ui/`

**Body**:
- `/ui/` → **Training Data** tab shows UAITS curated datasets (U8)
- `/ui/` → **Config** tab surfaces UxTS-related env vars (`RETRIEVAL_AUDIT_ENABLED`, `UVTS_*`)
- `/ui/` → **Logs** tab filters by `uats:*`, `uvts:*`, etc. runner invocations
- Grafana `mdemg-overview` dashboard: `UVTS Mean Grade` panel (operator-authored UVTS verdicts over time)
- Grafana `mdemg-ft-training` dashboard: UBENCH latest aggregate + per-task pass rate

**Provenance**:
- `internal/api/ui/tabs/training_data.js`
- `deploy/docker/grafana/dashboards/mdemg-overview.json`

**note**:
- Operator cue: "UxTS is mostly a CI story. The UI reflects its OUTCOMES — specs' current state — not the specs themselves (those live in docs/)."

---

## U6.7 — When To Author A New UxTS Framework

**Title**: `Adding A UxTS Family`

**Body**:
- Trigger: a new domain of system behavior that needs contract-level enforcement AND doesn't fit any existing family's scope
- Shape: build the diamond — schema (`<family>/schema/<family>.schema.json`) → specs (`<family>/specs/*.json`) → runner → CI gate
- Governance: UATS-GAP-001 — the new family MUST register in `UXTS_FRAMEWORK_MATRIX.md` + the drift checker; new specs land under `specs/` not `drafts/` once vetted
- Common gotcha: deep-merge semantics in runners — if a variant omits a key, it inherits from base; required-field guards use empty-string overrides, not key omission (caught live in `j17_protocol_learn`)
- Example of successful add: UBENCH (2026-07; wraps Phase 10) + UITS (T1 encoding comprehension) + UOBS (runtime observability)

**Provenance**:
- CLAUDE.md "UXTS-CI-001" + specific family-add sprint records (UATS-GAP-001, UBENCH authoring sprint)

**note**:
- Operator cue: "If you're asking 'should this be a UxTS family?' the answer is usually 'yes, if you want it enforced at merge time.' If it's a one-off check, a unit test is fine."

---

## U6.8 — Current State (Honest Trajectory)

**Title**: `UxTS Today — Honest`

**Body**:
- 16 families · 15 active · ~300+ specs total
- CI merge-blocking: UATS, UPTS, UOTS, ULTS hash-verify, USTS, UBENCH, UxTS matrix drift
- Soft-fail (warnings, not blockers): UBTS, UOBS, UETS, UITS
- No CI gate (operator opt-in): UVTS (stub-embedder grading would measure noise; live-gated via `make test-uvts-quick` + Note 02 A/B merge gate), UTDS, UAITS, UDTS (gRPC contract tests), UNTS (hash registry), UAMS (spec-only, no runner)
- Specs count has been steadily increasing (28 UPTS, 215 UATS, 18 ULTS …)
- Known gap: `UDTS` doesn't have `uxts-canonical-specs.yml` wiring yet (deferred, DORMANT-CENSUS-002 captured)

**Provenance**:
- `docs/development/UXTS_FRAMEWORK_MATRIX.md`
- `.github/workflows/ci.yml`

**note**:
- Operator cue: "Soft-fail families are candidates to merge-block in the future as they stabilize. UVTS is intentionally live-gated — stub-embedder scoring would be dishonest."

---

## U6.9 — Where To Go Deeper

**Title**: `UxTS — More`

**Body**:
- The matrix: `docs/development/UXTS_FRAMEWORK_MATRIX.md` — ALL 16 families with schema path, spec dir, runner, CI gate, implementation status
- Per-family schemas: `docs/api/api-spec/<family>/schema/` + `docs/tests/<family>/schema/`
- Per-family specs: `docs/api/api-spec/<family>/specs/` + `docs/tests/<family>/specs/`
- CLAUDE.md UxTS pins: UATS-GAP-001, UXTS-CI-001, DORMANT-CENSUS-* family (routes + TSDB + metrics)
- Where this feeds into: U7 Review (HITL grades feed UAITS datasets), U8 Learning/Training (UBENCH + ULTS gate the retrain), U13 Grafana (UVTS/UBENCH panels)

**note**:
- Operator cue: "UxTS is the SHAPE. The family is the APPLICATION. Learn the shape; the applications become obvious."
- Transition: "U7 next — Review/HITL, where operators FEED the system by grading what UxTS can't automate."
