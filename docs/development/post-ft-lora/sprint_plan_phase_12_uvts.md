# Sprint POST-FT-LORA-PHASE12 — UVTS Activation (Sprint 2)

## Context

Phase 11.6.x + 11.6.2 (commits `002a5f0` + `f81bfd6`, PR #364) closed the production cutover follow-ups: RSIC concurrency-limit semaphore, Jiminy task_name swap fix + V0014 backfill, Grafana LLM-routing dashboard, prompt-cache configuration, conflict-tracker recorder + V0015, plus a 6th cutover-bypass at `jiminy/service.go:102`. With the cutover stable and operational hygiene closed, the post-FT-LORA roadmap (`docs/development/SPRINT_ROADMAP_POST_FT_LORA.md`) lists **Phase 12 — UVTS Activation** as Sprint 2.

**What UVTS is.** Unified Validation Test Suite for **semantic retrieval quality**. Tests query→retrieval accuracy across A/B branches (e.g. "Column-Voting Retrieval vs. legacy ranker"). Distinct from UBENCH (closed-form LLM task rewards) and UATS (API contract). Schema + 70-line baseline spec + 677-line stub runner already exist under `docs/tests/uvts/`. Question corpus (120 stratified questions across 8 categories) lives at `docs/architecture/benchmarks/whk-wms/test_questions_120.json`. Grader (`grader_v4.py`) and answer generator (`answer_generator.py`) live at `docs/architecture/benchmarks/`. The runner is **stub-only for question generation**; it does not yet wire grader output to threshold checks or persist results.

**Why this sprint now (research-eval verbatim):**
- *"UVTS is the bottleneck framework. UVTS is currently spec-only with a stub runner. Notes 02, 03, 04, 05, 06, 07, 08, and 09 all imply UVTS work. Promoting UVTS from pilot to active is implicitly a prerequisite for the entire collection."*
- *"This is the highest-leverage single sprint the project could run right now."*

Without an active UVTS runner, the merge-gate machinery for the eight architectural extensions (Tier 2–5 of the research evaluation) cannot fire — every retrieval-quality A/B comparison would have to be hand-validated, and that doesn't scale across the program.

**Scope option recommended (Option B — UVTS + ConflictTracker tail-cleanup):** Option A (UVTS only) leaves Phase 11.6.x's deferred ConflictTracker production wiring orphaned — the recorder ships but no callers. Option C (UVTS + Notes 02/04/05 spec-skeleton drafting) is too wide; spec authoring for those notes is a per-note exercise and shouldn't be batched. Option B ships UVTS activation + folds in ~1 day of ConflictTracker production wiring (Workstream C item #1) so the 3-month divergence-observation clock starts immediately. Folding it in costs a small slice of sprint capacity but starts longitudinal data flow that future sprints depend on.

**Phase dependency chain:** Phase 11.6.2 (this branch's HEAD) → **Phase 12 (this) — UVTS activation + ConflictTracker production wiring** → Phase 13 (Note 04 Column-Voting Retrieval, first UVTS A/B consumer) → subsequent research-extension notes (Tier 2–5).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE12 |
| Title | UVTS Activation — runner activation + A/B gate + ConflictTracker production wiring |
| Date | 2026-05-01 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 11.6.2 (commit `f81bfd6`); Phase 11.6.x (commit `002a5f0`, PR #364) |
| Successor | Phase 13 — Note 04 Column-Voting Retrieval (first UVTS A/B consumer) |
| Type | Code-medium (~600–900 LOC: runner activation + A/B harness + 2-3 instrumentation sites + new specs); infra-medium (V0016 TSDB migration); compute-light (no model training; grader_v4 + answer_generator may call cloud LLM during e2e) |
| Risk | MEDIUM (decision forks: A/B-gate enforcement mode, V0016-vs-V0012 schema, ConflictTracker scope) |
| Budget | $5–15 OpenAI for Tier-3 e2e if grader_v4 routes to cloud; well under $100 cap. Tier 1+2 = $0 |
| Effort estimate | 10–15 dev-days (per research-evaluation §RD-9) including ~1 day for ConflictTracker wiring tail-cleanup |
| Codebase under test | `/Users/reh3376/whk-wms/` (separate repo, exists locally; spec's `validation.codebase` field) |
| Question corpus | `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (120 stratified, 8 categories of 15 each); spec path field needs path correction (currently points at `docs/benchmarks/whk-wms/`) |
| New TSDB migration | **016_uvts_results.sql** — `uvts_results` (per-question rows: spec_path, branch_label, question_id, category, mean_score, evidence_match, semantic_sim, concept_overlap, file_loc_ok, raw_grade_json, recorded_at) + `uvts_runs` (run_id CUIDv2 PK, spec_sha, branch_label, codebase_sha, started_at, completed_at, gate_verdict, threshold_json) |
| Post-sprint artifacts | activated `docs/tests/uvts/runners/uvts_runner.py` (grader+aggregation+gate wired); `docs/tests/uvts/runners/uvts_ab_compare.py` (new A/B harness); `docs/tests/uvts/specs/{lnl_demo_validation.uvts.json (extended), polysemy_resolution.uvts.json (new)}`; V0016 migration; ConflictTracker production hooks at 3 decision sites (jiminy + ape + consulting); CI workflow extension; sprint docs |

## 2. Problem Statement

Build a reproducible, CI-integrated UVTS runner that, given a UVTS spec and a target codebase, produces a per-question + aggregate validation report and gates merge of any candidate retrieval branch against a baseline. Specifically:

1. **Runner activation** — wire `grader_v4.py` + `answer_generator.py` into `docs/tests/uvts/runners/uvts_runner.py` so the runner produces real per-question scores (evidence match 0.70 + semantic similarity 0.15 + concept overlap 0.15, with 10-line file-location tolerance) rather than just generating questions. Aggregate per-category and overall mean score; emit canonical UxTS report via `uxts_report.build_report()`.
2. **Threshold + merge gate** — apply spec-defined thresholds (`mean_score`, `strong_evidence_pct`, `high_score_rate_pct`, `min_category_score`, `max_token_usage`) to the aggregated result. Emit `gate_verdict ∈ {'pass','fail'}` via the canonical UxTS report.
3. **A/B harness** — new `uvts_ab_compare.py` script: takes two runner reports (branch-A baseline, branch-B candidate) and applies the merge-gate criterion *"B ≥ A AND no individual question regressing more than 10%"* (per the post-FT-LORA roadmap §Phase 12 / RD-9). Emit a structured A/B verdict.
4. **TSDB persistence (V0016)** — `uvts_runs` (run-level metadata) + `uvts_results` (per-question rows, hypertable on `recorded_at`). Additive migration; no ALTER on V0011–V0015.
5. **Spec authoring** — extend `lnl_demo_validation.uvts.json` to include A/B mode threshold extension; add `polysemy_resolution.uvts.json` (Note 05's "+5 to +10% on context-sensitive query strata" hypothesis seed). Fix the spec's corpus path to point at the actual location (`docs/architecture/benchmarks/whk-wms/...`).
6. **CI integration** — add `docs/tests/uvts/**` to `.github/workflows/uxts-canonical-specs.yml` triggers and wire `make test-uvts` (or equivalent) so the runner is exercised on push to dev branches and on PR-to-main.
7. **ConflictTracker production wiring (Workstream C item #1, deferred from 11.6.x)** — instrument 3 decision sites (jiminy guidance issuance, RSIC reflection conclusion, consulting.classify outcome) so divergent recommendations flow to the existing `guidance_conflicts` hypertable. This unlocks the 3-month observation window that the FEP capstone (Note 09) is gated on.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | UVTS runner activation (grader + aggregation + threshold gate) | `docs/tests/uvts/runners/uvts_runner.py` (extend existing 677-line stub) |
| 2 | UVTS A/B comparison harness | `docs/tests/uvts/runners/uvts_ab_compare.py` (new) |
| 3 | Polysemy-resolution spec | `docs/tests/uvts/specs/polysemy_resolution.uvts.json` (new) |
| 4 | Extended demo spec (A/B mode + corpus path fix) | `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` (modify) |
| 5 | TSDB migration 016 | `internal/tsdb/migrations/016_uvts_results.sql` (`uvts_runs` + `uvts_results` hypertable) |
| 6 | ConflictTracker production hooks | `internal/jiminy/{guidance,...}.go` + `internal/ape/{cycle,...}.go` + `internal/consulting/...go` (3 instrumentation sites; pass-through `*ConflictTracker` from API server init) |
| 7 | Tracker holder wiring + dependency injection | `internal/api/server.go` (construct + share `*ConflictTracker`); related Service constructors |
| 8 | CI workflow extension | `.github/workflows/uxts-canonical-specs.yml` — add `docs/tests/uvts/**` triggers; new `make test-uvts` target |
| 9 | Unit + integration + e2e tests | `docs/tests/uvts/runners/test_*.py`; `internal/conversation/conflict_tracker_*_test.go`; `tests/scripts/test_v0016_migration.py` |
| 10 | Sprint docs | `docs/development/post-ft-lora/sprint_plan_phase_12_uvts.md` (this plan, copied + frozen); `phase_12_uvts_post.md` (executed-truth post) |
| 11 | Doc updates | `SPRINT_ROADMAP_POST_FT_LORA.md` Phase 12 → EXECUTED; `AGENT_HANDOFF.md` top entry; `CHANGELOG.md [Unreleased] ### Added`; `CLAUDE.md` — add `make test-uvts` invocation under Testing |

**Out of scope (deferred to Phase 13 or later):**
- Column-Voting Retrieval implementation (Phase 13 — first UVTS A/B *consumer*, not part of this sprint).
- Note 04/05/06 retrieval-cluster spec authoring beyond `polysemy_resolution.uvts.json` seed (per-note specs are per-note exercises).
- UBENCH formalization (Task #215 — different framework, separate sprint).
- Whole-codebase corpus expansion beyond 120 questions (Phase 13+ adds new corpora as A/B targets land).
- `make test-uvts` migration into the canonical pre-merge gate (this sprint adds the trigger; Phase 13 enforces it as a blocking required check after one full A/B cycle has demonstrated stability).
- Note 09 FEP capstone instrumentation beyond ConflictTracker recording (recording starts the 3-month clock; Note 09 itself is a Tier-6 capstone gated on co-implementer recruitment per Action 6).

**Constraints (hard):**
- **MEMORY: no hardcoded values** — all thresholds + corpus paths + grader model selection in spec JSON or runner CLI flags; spec schema enforces required fields.
- **MEMORY: CUIDv2 for run_id** — `cuid2` Python package (already vendored).
- **MEMORY: sequential epics** — no parallel epic execution; docs before implementation within each epic.
- **MEMORY: 3-tier testing** — unit (grader scoring math, A/B compare logic, ConflictTracker hookup unit) / integration (runner against canned answers, V0016 forward+reverse, ConflictTracker live with real subsystem callbacks) / e2e (full UVTS run on whk-wms 16-question quick profile against `/Users/reh3376/whk-wms`; A/B gate fires correctly on synthetic diff).
- **MEMORY: min max_tokens ≥ 3000, min latency_budget_ms ≥ 15000** — applies to any cloud LLM call in grader_v4 (defaults already meet this; CLI flags inherit).
- **MEMORY: no tight budget caps** — target $5–15 OpenAI spend; flag only if >$100.
- **MEMORY: plan-options pattern** — three decision forks (V0016 vs V0012 schema; A/B gate enforcement mode; ConflictTracker scope); recommendations + rationale here; pick at execution; disclose at PR.
- **MEMORY: single batched commit at sprint close**.
- **MEMORY: sprint summary posted to PR comments immediately after push** (not gated on CI).
- **Sprint plans live in `docs/development/<sprint-line>/`** — this is a new sprint line `post-ft-lora/`.
- **TSDB additive** — V0016 creates new tables only; zero ALTER on V0011–V0015.
- **No mdemg API behavior change** for existing endpoints — ConflictTracker hooks fire async (the existing `Track()` method is non-blocking on the caller's critical path) and are gated behind `JIMINY_ENABLED && CONFLICT_TRACKER_ENABLED`.
- **A/B gate blocks merge only when explicitly invoked** — this sprint adds the harness; Phase 13 is when the gate becomes a required CI check. Until then, gate verdict is *advisory* (operator decides). Disclosure at PR.
- **Spec schema versioning** — UVTS schema file already exists; if any new fields land, bump `uvts_version` 1.0.0 → 1.1.0 + add migration note for older specs.

## 4. Dependencies

**Consumed (code, pre-existing):**
- `docs/tests/uvts/runners/uvts_runner.py` — 677-line stub runner; `--base-url` + `--profile` + `--grades` flags already exist; canonical UxTS report machinery already wired via `uxts_report.build_result/build_report`.
- `docs/tests/uvts/schema/uvts.schema.json` — JSON Schema v2020-12 (230 lines); spec-validation foundation.
- `docs/architecture/benchmarks/grader_v4.py` — three-axis scorer (evidence match 0.70 + semantic 0.15 + concept 0.15, 10-line file-loc tolerance). Used as a Python module import.
- `docs/architecture/benchmarks/answer_generator.py` — produces candidate answers from MDEMG retrieve API responses (input to grader). Used as a Python module import.
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — 120-question stratified corpus (8 categories × 15 questions).
- `docs/tests/uxts_report.py` + `docs/tests/uxts_runner_core.py` — canonical UxTS result builders + hash verification (already used by UATS, ULTS, UOTS, UOBS, UDTS).
- `internal/conversation/conflict_tracker.go` — Phase 11.6.x recorder with `Track()` API, per-space rate limiter, nil-pool fail-open. Wiring is what Sprint 2 adds.
- `internal/jiminy/service.go` — issuance + outcome callbacks where ConflictTracker.Track() needs to fire on divergent guidance.
- `internal/ape/cycle.go` — RSIC reflection conclusion site for divergence detection.
- `internal/consulting/...` — consulting.classify outcome site.
- `internal/tsdb/migrations/{014,015}.sql` — must already be live-applied (V0014 + V0015 from Phase 11.6.x); preflight asserts `schema_version=15` before V0016 stacks.

**Consumed (data):**
- `/Users/reh3376/whk-wms/` — separate codebase, exists locally; UVTS validates retrieval against this corpus.
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — corpus.
- `docs/architecture/benchmarks/whk-wms/test_questions_120_agent.json` — agent-formatted version (used by some grader paths).
- TSDB `guidance_conflicts` table (V0015, empty post-11.6.x); ConflictTracker hooks populate this.

**Consumed (compute):**
- Local mlx server on `127.0.0.1:8101` (used when grader_v4 routes to local model; default cloud).
- OpenAI API (or Claude — grader's `--model` flag selects; default sonnet per spec) for grader scoring + answer generation. ~120 questions × $0.04/question = ~$5–15 per full-corpus run.

**External services:**
- mdemg HTTP API (e.g. `localhost:9999`) — grader/answer_generator queries the retrieve endpoint.
- TimescaleDB at `localhost:5433` — schema_version must be ≥ 15 at preflight.
- Neo4j at `localhost:7687` — read-only by mdemg's retrieve endpoint; UVTS doesn't write.

No Neo4j writes. No model training. Grader cloud calls only — no per-step LLM in any tight loop.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean; native binary running on host with `LLM_ENDPOINT=http://127.0.0.1:8101/v1`; mlx alive; TSDB schema_version=15; mdemg health green; Neo4j up; Python venv `mdemg-ft-lora` active for runner work; whk-wms repo present at `/Users/reh3376/whk-wms/`.

### Epic 0 — Preflight + Spec Path Fix + Drafting

1. Verify `schema_version=15` (V0014 + V0015 already applied per Phase 11.6.x).
2. Verify whk-wms target codebase exists; confirm question corpus at `docs/architecture/benchmarks/whk-wms/test_questions_120.json` parses cleanly (assert 120 questions, 8 categories × 15).
3. Confirm `grader_v4.py` + `answer_generator.py` import cleanly; smoke-test the grader on a single question + canned answer pair.
4. Inventory existing runner entry-points (`uvts_runner.py` is 677 lines — read carefully, identify the stub points where grader integration goes; do *not* rewrite from scratch).
5. Fix `lnl_demo_validation.uvts.json` `questions.source_file` field: currently `docs/benchmarks/whk-wms/...`, should be `docs/architecture/benchmarks/whk-wms/...`. **This is the only change to the existing spec in Epic 0**; A/B threshold extension lands in Epic 3.
6. Sketch the full `polysemy_resolution.uvts.json` schema (no questions yet; just the threshold + category structure).

**Gate:** corpus loads; grader smoke-test on one question returns plausible scores; spec path fix is a single-line edit verified by re-loading the spec; runner entry-point inventory documented in a brief note in the plan post.

### Epic 1 — UVTS Runner Activation (the core)

Convert `docs/tests/uvts/runners/uvts_runner.py` from question-generation-only to a full grade-and-gate runner.

1. Wire `grader_v4` + `answer_generator` imports. Add `--grader-model` CLI flag (default per spec); add `--mdemg-base-url` CLI flag (default `http://localhost:9999`); inherit `--profile` from existing.
2. Implement the grading loop: for each question, call `answer_generator.generate(...)` → call `grader_v4.grade(...)` → record per-question score (evidence_match × 0.70 + semantic_sim × 0.15 + concept_overlap × 0.15) + file-loc-tolerance flag.
3. Implement aggregation: per-category mean + overall mean + strong_evidence_pct + high_score_rate_pct + max_token_usage.
4. Implement threshold gate: compare aggregated metrics to spec's `thresholds` block; emit `gate_verdict ∈ {'pass','fail'}` and a per-threshold `triggered_thresholds` list.
5. Emit canonical UxTS report via `uxts_report.build_result/build_report` — same shape as UATS/ULTS/UOTS for CI parsability.
6. Add `--no-tsdb` flag (off by default) for cases where the operator wants a dry-run report without TSDB rows.

**Gate:** runner can grade `lnl_demo_validation.uvts.json` quick profile (16 questions × 8 categories) end-to-end against `/Users/reh3376/whk-wms`; report written; threshold check fires correctly on both pass + fail synthetic inputs (Tier 2 integration test).

### Epic 2 — TSDB V0016 Migration

1. `internal/tsdb/migrations/016_uvts_results.sql`:
   - `uvts_runs` (run_id TEXT CUIDv2 PK, spec_path TEXT, spec_sha TEXT, branch_label TEXT, codebase_root TEXT, codebase_sha TEXT, started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, gate_verdict TEXT CHECK IN ('pending','pass','fail'), threshold_json JSONB, aggregate_score NUMERIC).
   - `uvts_results` (result_id TEXT CUIDv2 PK, run_id TEXT FK, question_id TEXT, category TEXT, mean_score NUMERIC, evidence_match NUMERIC, semantic_sim NUMERIC, concept_overlap NUMERIC, file_loc_ok BOOLEAN, raw_grade JSONB, recorded_at TIMESTAMPTZ).
   - Hypertable on `uvts_results.recorded_at` (7-day chunks).
   - Indexes: `(run_id, recorded_at)`, `(category)`, `(question_id)`.
2. Bump `TSDB_REQUIRED_SCHEMA_VERSION` default 15 → 16 in `internal/config/config.go`.
3. Apply forward + reverse on dev DB to validate rollback. Pre+post DO blocks audit row counts.
4. Wire runner's TSDB writes via `pgxpool` (mirror Conflict Tracker pattern from Phase 11.6.x — direct insert, no batch needed at expected rate).

**Gate:** V0015 → V0016 forward + V0016 → V0015 reverse green; `uvts_runs` + `uvts_results` populated end-to-end during a runner integration test; previous V0011–V0015 row counts unchanged.

### Epic 3 — Spec Authoring (lnl_demo extension + polysemy)

1. Extend `lnl_demo_validation.uvts.json`:
   - Add `ab_mode` block: `enabled: false` (default; enabled at the harness layer), `regression_threshold_per_question: 0.10` (Note 02's "no individual question regressing more than 10%").
   - Add `branches` (advisory metadata): `baseline_label`, `candidate_label`.
   - Bump `uvts_version` 1.0.0 → 1.1.0 if any new required fields added; otherwise keep 1.0.0.
2. Author `polysemy_resolution.uvts.json` (new):
   - 8 polysemy-stratified categories (e.g., `homonym_disambiguation`, `context_carry_over`, `entity_resolution`, etc. — derive concrete category names from research-evaluation §3.4 polysemy taxonomy).
   - 5 questions per category × 8 = 40 questions (matches lnl_demo size).
   - Threshold structure mirrors lnl_demo (mean_score, strong_evidence_pct, high_score_rate_pct, min_category_score, max_token_usage), with values calibrated for polysemy-resolution baseline (TBD by Epic 5 e2e — initial pass uses lnl_demo values as a starting point with a `// CALIBRATION_TODO` note).
   - **Out of scope**: actual question authoring beyond category structure + 1 example question per category; full 40-question authoring deferred to a Note 05 tee-up sprint. The spec ships with `partial_authoring: true` flag.

**Gate:** `uvts_runner.py --validate-spec polysemy_resolution.uvts.json` passes schema validation; lnl_demo_validation extension validates clean; both specs hash-register in UNTS scanner.

### Epic 4 — A/B Compare Harness

1. New `docs/tests/uvts/runners/uvts_ab_compare.py`:
   - CLI: `--baseline <report-A.json> --candidate <report-B.json> --spec <uvts-spec.json> --out <ab-verdict.json>`.
   - Logic: assert both reports cover the same `question_ids` (no question-set drift); compute per-question score deltas; apply *"B mean ≥ A mean AND no individual question regresses more than `ab_mode.regression_threshold_per_question`"* gate.
   - Emit `{verdict: 'pass'|'fail', regressions: [{question_id, a_score, b_score, delta}], baseline_summary, candidate_summary}`.
2. Add a `--persist-tsdb` flag that writes an `uvts_runs` "ab" row with `branch_label='A_vs_B_<short>'` and links to both source `run_id`s.
3. Document the harness as the merge gate for any retrieval-quality A/B (Phase 13+ consumers).

**Gate:** synthetic A/B fixture (canned A + B reports) → pass + fail cases both verified.

### Epic 5 — CI Workflow Integration

1. `.github/workflows/uxts-canonical-specs.yml` — add `docs/tests/uvts/**` to the path triggers (alongside existing UATS/ULTS/UOTS/UOBS/UDTS).
2. New `make test-uvts` target in `Makefile`:
   - Runs `uvts_runner.py` against `lnl_demo_validation.uvts.json` quick profile (~16 questions, ~10 min).
   - Emits canonical report; non-zero exit on `gate_verdict='fail'`.
3. CI executes `make test-uvts` on push/PR; the resulting check is *advisory* this sprint (does not block merge); Phase 13 promotes to required after one full A/B cycle has demonstrated stability.
4. Add `make test-uvts-quick` (16-question quick profile) and `make test-uvts-full` (full 120-question profile) variants.

**Gate:** CI workflow YAML lints clean; `make test-uvts-quick` succeeds locally; check appears (advisory) on the next PR push.

### Epic 6 — ConflictTracker Production Wiring (Workstream C item #1)

1. Construct `*ConflictTracker` once in `internal/api/server.go` (after the TSDB pool is available); pass through to the three Service constructors that need it: jiminy, ape, consulting.
2. Three instrumentation sites (use the existing nil-receiver-safe `Track()` API):
   - **jiminy guidance issuance** (`internal/jiminy/service.go::Guide` post-synthesis): when both consulting + jiminy produce non-empty recommendations and they disagree (e.g., consulting=block but jiminy=warn), call `Track()` with `divergence_kind='ordinal'`.
   - **RSIC reflection conclusion** (`internal/ape/cycle.go::RunCycle` after `reflector.Reflect`): when LLM insights conflict with rule-based insights on the same dimension (e.g., LLM recommends `prune_decayed_edges` while rule-based recommends `graduate_volatile`), call `Track()` with `divergence_kind='textual'`.
   - **consulting.classify outcome** (`internal/consulting/...go`): when consulting classifies an action as constrained but jiminy's most-recent guidance for the same context was advisory-only, call `Track()` with `divergence_kind='binary'`.
3. Add `CONFLICT_TRACKER_ENABLED` config knob (default `true`); gate all three hooks behind it for emergency disable.
4. Each hook is async (the existing `Track()` is non-blocking — wrap callers in `go func() { ... }()` if needed) and never affects the caller's critical path.

**Gate:** unit tests for each hook (mock `*ConflictTracker`, simulate divergent inputs, assert `Track()` was called); live integration test with real TSDB (3 synthetic divergences fire end-to-end and land as 3 rows in `guidance_conflicts`); RSIC + jiminy + consulting existing test suites still green.

### Epic 7 — Testing (3 Tiers)

Covered in §6 below.

### Epic 8 — Documentation (Final Epic — Never Cut)

1. `docs/development/post-ft-lora/sprint_plan_phase_12_uvts.md` — this plan verbatim.
2. `docs/development/post-ft-lora/phase_12_uvts_post.md` — executed-truth doc: lnl_demo aggregate score on whk-wms, A/B harness verdict on a canned diff, V0016 row counts, ConflictTracker hook fire counts, decision-fork outcomes, sprint wall-clock, OpenAI spend actual.
3. `SPRINT_ROADMAP_POST_FT_LORA.md` — mark Phase 12 EXECUTED with commit SHA; flag Phase 13 (Note 04 Column-Voting Retrieval) unblocked.
4. `AGENT_HANDOFF.md` top entry: Phase 12 complete; UVTS active; ConflictTracker live; Phase 13 unblocked.
5. `CHANGELOG.md [Unreleased] ### Added`: UVTS runner activation, A/B harness, polysemy spec, V0016 migration, ConflictTracker production wiring, CI integration.
6. `CLAUDE.md` — add `make test-uvts` invocation under Testing section.

**Gate:** all docs committed; cross-refs valid; `grep -r "Phase 12.*pending\|Phase 12.*planned" docs/development/post-ft-lora/` returns zero hits.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit):**
- `docs/tests/uvts/runners/test_uvts_runner.py`: aggregation math, threshold-gate logic, canonical-report shape.
- `docs/tests/uvts/runners/test_uvts_ab_compare.py`: A/B compare logic against fixture pairs (pass + fail + edge cases like missing-question + tied scores).
- `internal/conversation/conflict_tracker_hooks_test.go` (new): each of the 3 hook functions in isolation — mock `*ConflictTracker`, simulate divergent inputs, assert `Track()` was called with the right `divergence_kind`.

**Tier 2 (Integration):**
- `docs/tests/uvts/runners/test_uvts_runner_integration.py`: real grader_v4 against canned mdemg-API responses → real per-question scores → V0016 TSDB rows.
- `tests/scripts/test_v0016_migration.py`: forward + reverse on a test DB; row counts match expectation; pre+post DO blocks emit expected NOTICEs.
- `internal/jiminy/conflict_tracker_e2e_test.go` + `internal/ape/conflict_tracker_e2e_test.go` + `internal/consulting/conflict_tracker_e2e_test.go`: each Service exercised against a live `pgxpool.Pool`, divergent inputs simulated, rows verified in `guidance_conflicts` then cleaned up.

**Tier 3 (E2E):**
- Full `uvts_runner.py --profile quick` against `/Users/reh3376/whk-wms` codebase, real grader (`--grader-model sonnet`), real mdemg API at `localhost:9999`. Confirms: corpus loads, answer_generator queries succeed, grader returns plausible scores, threshold gate fires, V0016 rows land, canonical UxTS report generates.
- A/B harness e2e: run `lnl_demo_validation.uvts.json` quick profile against the **same** mdemg state on two distinct `branch_label` values (synthetic diff — flip a rerank flag, etc.); assert harness reports `verdict='pass'` since identical state.
- ConflictTracker live observation: post 5 divergent observations through `/v1/conversation/observe` (designed to trigger jiminy-vs-rsic disagreement); assert `guidance_conflicts` has 3+ rows after 60 seconds.
- CI workflow dry-run: `act` or equivalent runs the YAML locally; `make test-uvts-quick` succeeds inside the CI environment.

**State restoration (MEMORY):** all changes additive. Rollback = revert commit; `mdemg tsdb migrate --target V0015` (reverse V0016 — drops `uvts_runs` + `uvts_results` only). ConflictTracker hooks are gated by `CONFLICT_TRACKER_ENABLED`; setting it to `false` disables instrumentation without restart-required code changes (config can hot-reload via SIGHUP if supported, otherwise restart).

**Gate:** all 3 tiers green; T-subset smoke (lnl_demo quick profile e2e) passes before A/B harness e2e launches.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(uvts): Sprint POST-FT-LORA-PHASE12 — UVTS activation + ConflictTracker production wiring`
- Body: scope summary, runner-activation outcome (lnl_demo aggregate score on whk-wms), V0016 migration note, A/B harness verdict on canned diff, polysemy spec status (partial-authoring flag), 3 ConflictTracker hook sites + first-day fire-count, decision-fork outcomes (V0016 vs V0012 schema; A/B gate enforcement mode chosen; ConflictTracker scope chosen), policy compliance checklist (CUIDv2, no hardcoded values, sequential epics, 3-tier testing, single batched commit, sprint summary on PR).
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens (PR #364 update or new PR if rebased) → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: schema_version=15 confirmed; corpus + grader + answer_generator load; spec corpus path fixed
- [ ] Epic 1: runner produces real per-question scores end-to-end; threshold gate fires correctly on synthetic pass + fail; canonical UxTS report shape matches UATS/ULTS
- [ ] Epic 2: V0016 forward + reverse clean; `uvts_runs` + `uvts_results` populated by an integration run
- [ ] Epic 3: lnl_demo_validation extension + polysemy_resolution.uvts.json schema-validate clean; both register in UNTS scanner
- [ ] Epic 4: A/B harness pass + fail cases verified on canned fixture pairs
- [ ] Epic 5: CI workflow extension lints clean; `make test-uvts-quick` succeeds locally
- [ ] Epic 6: 3 ConflictTracker hook sites instrumented; 3+ rows in `guidance_conflicts` after live e2e
- [ ] Epic 7: all 3 test tiers green; T-subset smoke before full e2e
- [ ] Epic 8: sprint plan + post report + ROADMAP "Phase 12 EXECUTED" + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [ ] Commit pushed; auto-PR updated; **sprint summary posted to PR immediately**
- [ ] OpenAI spend logged, under $100 cap (target $5–15)
- [ ] All decision-fork choices disclosed in commit body + PR comment

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 8: `docs/development/post-ft-lora/sprint_plan_phase_12_uvts.md` (this plan, frozen), `docs/development/post-ft-lora/phase_12_uvts_post.md`, `SPRINT_ROADMAP_POST_FT_LORA.md` Phase 12 → EXECUTED with commit SHA, `AGENT_HANDOFF.md` prepended, `CHANGELOG.md [Unreleased] ### Added`, `CLAUDE.md` Testing section adds `make test-uvts-quick` and `make test-uvts-full` invocations.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **whk-wms target codebase has drifted from corpus assumption** (questions reference files no longer at expected paths) | Medium | Epic 0 smoke-test grader on 1 question + canned answer; if grade comes back implausibly low, audit corpus vs codebase before Epic 1 | Update corpus or pin codebase to a known-good SHA; document in post |
| 2 | **grader_v4 cloud API spend exceeds $100 cap** during e2e | Low | quick profile is 16 questions × ~$0.04 ≈ $0.64; full 120-question run ≈ $5; e2e budget bounded by `--profile quick` for routine runs | Switch grader to local mlx via `--mdemg-base-url`; document accuracy delta in post |
| 3 | **Decision fork: V0016 new table vs extend V0012** (`benchmark_results`) — schema choice affects future query joins | Medium | Recommend V0016 (new) — UVTS metrics differ enough from Phase 10 (per-question score vs reward registry) that schema sharing is awkward; Phase 13's column-voting consumer already reads from `uvts_runs.gate_verdict` independently | Option B: extend V0012 with `validation_kind` column + add UVTS-specific columns; document at PR |
| 4 | **Decision fork: A/B gate enforcement mode** — blocking-required-check vs advisory-log vs hybrid | Medium | Recommend advisory this sprint, promote to required in Phase 13 after one full A/B cycle has demonstrated stability; per research-eval *"merge gates"* is the long-term goal but premature blocking on a brand-new runner risks false-fail noise | Option B: hard-required from day 1; risk: false-fail blocks unrelated PRs |
| 5 | **Decision fork: ConflictTracker scope** — fold into this sprint vs separate Workstream-C mini-sprint | Low | Recommend folded (Option B in Context section); ~1 day cost, starts 3-month observation immediately, recorder is ready | Option C: separate sprint; risk: 3-month clock starts later, Note 09 capstone delays |
| 6 | **Polysemy spec authoring is non-trivial** (40 questions designed to stratify polysemy axes) | High | Spec ships with category structure + 1 example per category; flag `partial_authoring: true`; full 40-question authoring deferred to a Note 05 tee-up sprint | Use lnl_demo_validation as the only active spec this sprint; ship polysemy schema-only |
| 7 | **TSDB V0016 migration has unintended interaction with V0014/V0015** | Low | Additive only (new tables); reverse test on dev DB before commit; pre+post DO blocks audit | `mdemg tsdb migrate --target V0015` cleanly rolls back |
| 8 | **ConflictTracker hooks fire too often under normal traffic** (TSDB cost balloon) | Low | Per-space rate limiter (1 row/space/minute) already in `conflict_tracker.go`; 3 hooks share the limiter | Disable hook via `CONFLICT_TRACKER_ENABLED=false`; document threshold tuning |
| 9 | **Existing `uvts_runner.py` has hidden assumptions that block grader integration** | Medium | Epic 0 step 4 inventories runner entry-points carefully; if assumptions block clean integration, write thin wrapper rather than rewriting | Worst case: rewrite the grading loop in a new module `uvts_grader_loop.py` and have `uvts_runner.py` invoke it as a subprocess |
| 10 | **CI workflow extension breaks UATS/ULTS triggers** | Low | Add `docs/tests/uvts/**` as additional path trigger, not replacement; test workflow YAML with `act` locally | Revert workflow change; UVTS runs manually until next sprint |
| 11 | **Mid-sprint discovery of additional cutover-bypass sites** (per Phase 11.6.2 pattern) | Medium | Sweep `cfg.OpenAIEndpoint` direct uses again at Epic 0 (last sprint already swept; this is a re-check) | Patch as Phase 12.1 follow-up commit; mirror 11.6.2 pattern |
| 12 | **Native binary on macOS host needs `LLM_ENDPOINT=127.0.0.1` override** | Certain (known from Phase 11.6.2 live test) | Document in Pre-gate; runbook update in `CLAUDE.md` with explicit native-vs-Docker `LLM_ENDPOINT` guidance | Add fallback in `cfg.EffectiveLLMEndpoint()` if it becomes a recurring issue |

## 11. Documents Accessed (during planning)

**Read during planning (3 parallel Explore agents):**
- `/Users/reh3376/mdemg/docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 12 / RD-9 section, ConflictTracker production wiring (Action 1), Workstream C cross-cutting items
- `/Users/reh3376/Downloads/mdemg-future-sprint-assessments/mdemg-research-evaluation.md` — UVTS gating thesis, "highest-leverage single sprint", Notes 02-09 dependency chain (verbatim quotes captured in Context section)
- `/Users/reh3376/Downloads/mdemg-future-sprint-assessments/mdemg-collaboration-brief.md` — research-collaboration framing
- `/Users/reh3376/mdemg/docs/tests/uvts/runners/uvts_runner.py` — 677-line existing stub runner; canonical UxTS report wiring
- `/Users/reh3376/mdemg/docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — 70-line baseline spec; threshold structure
- `/Users/reh3376/mdemg/docs/tests/uvts/schema/uvts.schema.json` — 230-line JSON Schema v2020-12
- `/Users/reh3376/mdemg/docs/architecture/benchmarks/whk-wms/test_questions_120.json` — 120-question stratified corpus (real path; spec field needs correction)
- `/Users/reh3376/mdemg/docs/architecture/benchmarks/grader_v4.py` + `answer_generator.py` — three-axis grader + answer synthesis modules
- `/Users/reh3376/mdemg/docs/tests/uxts_report.py` + `docs/tests/uxts_runner_core.py` — canonical UxTS result builders
- `/Users/reh3376/mdemg/internal/conversation/conflict_tracker.go` — Phase 11.6.x recorder; production hooks deferred to this sprint
- `/Users/reh3376/mdemg/internal/jiminy/service.go` (line 102 fix from 11.6.2) — wiring pattern reference for ConflictTracker
- `/Users/reh3376/mdemg/internal/ape/cycle.go` — RSIC reflection conclusion site
- `/Users/reh3376/mdemg/internal/unts/{server,registry,scanner}.go` — cross-framework hash governance for UxTS specs
- `/Users/reh3376/mdemg/.github/workflows/uxts-canonical-specs.yml` — CI workflow extension target (path triggers)
- `/Users/reh3376/mdemg/Makefile` — `make test-api` precedent for `make test-uvts`
- `/Users/reh3376/mdemg/scripts/live_validation.py` — 19 hardcoded integration tests (orthogonal to UVTS, no overlap)
- `/Users/reh3376/mdemg/internal/tsdb/migrations/{014,015}.sql` — schema-version 14 + 15 baselines for V0016 stack
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_6_x_post.md` + `phase_11_6_x_hygiene plan` — Phase 11.6.x post-doc (predecessor sprint)
- `/Users/reh3376/mdemg/CLAUDE.md` — testing section, MEMORY references, native-vs-Docker `LLM_ENDPOINT` runbook gap (Phase 11.6.2 finding)
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_sprint_plans_location.md`, `project_mdemg_purpose.md`

## 12. Rollback

All changes additive.

1. `git revert <final commit SHA>`.
2. Reverts: `docs/tests/uvts/runners/uvts_runner.py` to pre-sprint stub; remove `docs/tests/uvts/runners/uvts_ab_compare.py`, `docs/tests/uvts/specs/polysemy_resolution.uvts.json`; restore `lnl_demo_validation.uvts.json` to pre-sprint version; remove ConflictTracker hooks from `internal/jiminy/`, `internal/ape/`, `internal/consulting/`.
3. `mdemg tsdb migrate --target V0015` (reverse V0016; drops `uvts_runs` + `uvts_results` only; Conflict tracker rows in `guidance_conflicts` are preserved since V0015 stays).
4. `CONFLICT_TRACKER_ENABLED=false` for emergency disable without restart (if config hot-reload supported); else config flip + restart.
5. Revert CI workflow YAML (single path-trigger line removal).
6. Revert `CLAUDE.md` Testing-section addition + `Makefile` `test-uvts` targets.

Phase 11.5 + 11.6 + 11.6.x + 11.6.2 artifacts untouched. No Neo4j writes. V0016 rows dropped by reverse migration (auditable beforehand). `guidance_conflicts` rows from production wiring are preserved (V0015 table); they remain valid divergence data even if UVTS is rolled back.

---

## Post-Sprint — Phase 13 (Note 04 Column-Voting Retrieval) Unblocks

On merge, Phase 13 planning begins. Phase 13 consumes:
- `docs/tests/uvts/runners/uvts_runner.py` (active) → first A/B consumer; baseline = current rerank, candidate = column-voting retriever.
- `docs/tests/uvts/runners/uvts_ab_compare.py` → merge gate harness.
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` (extended) → quick-profile baseline.
- `polysemy_resolution.uvts.json` (partial) → secondary measurement axis if column-voting also affects polysemy.
- `uvts_runs` + `uvts_results` TSDB tables → historical baseline rows for trend analysis.
- `guidance_conflicts` 3-month observation → grows during Phase 13; informs Note 09 (FEP capstone) timing.

Phase 12 is intentionally MVP-on-validation: it activates the runner + ships an A/B harness + adds 3 production hooks; it does NOT author the full polysemy spec, does NOT promote A/B gate to required-blocking, and does NOT ship UBENCH formalization. Each of those is a separate scoped sprint.

---

## Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`:

| Fork | Recommended | Alternative | Rationale for recommendation |
|---|---|---|---|
| **TSDB schema** | V0016 new `uvts_runs`+`uvts_results` tables | Extend V0012 `benchmark_results` with `validation_kind` discriminator | UVTS metrics differ structurally from Phase 10 reward registry; clean separation simplifies future joins from Note 04 column-voting consumer. |
| **A/B gate enforcement** | Advisory (CI check non-blocking) | Required from day 1 | Brand-new runner has no proven false-fail rate; advisory gives operators 1 sprint of observation before promotion to required in Phase 13. |
| **ConflictTracker scope** | Folded in (Epic 6 of this sprint) | Separate Workstream-C mini-sprint | ~1 day cost; recorder is already deferred from Phase 11.6.x; folding in starts the 3-month observation clock immediately, which Note 09 capstone planning needs. |
| **Polysemy spec authoring** | Schema + 1 example/category (8 total); flag `partial_authoring: true` | Full 40-question authoring | Question authoring is a per-note exercise; better to ship runner activation now and let Note 05 sprint own polysemy authoring. |
| **CI workflow trigger** | Path-trigger added to existing `uxts-canonical-specs.yml` | New `uvts-only.yml` workflow | Existing pattern (UATS/ULTS/UOTS/UOBS/UDTS share workflow) — consistency matters. |
