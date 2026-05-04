# Sprint POST-FT-LORA-PHASE14.1 — Adaptive Per-Category Sparse Gate

> **APPROVED 2026-05-04** as a follow-up to Phase 14 narrow close. Phase 14 shipped Note 06 sparse gate flag-off after 120q full A/B failed per-question on `architecture_structure` (3 of 7 boundary regressions concentrated in one category). This sprint introduces category-aware MIN_ACTIVE / MAX_ACTIVE / percentile and re-runs the A/B with the goal of flipping `SPARSE_RETRIEVAL_ENABLED=true` default-on.

## Context

Phase 14 Epic 2 verdict (from `phase_14_post.md`):
- **16q quick at MIN=10/p95**: PASSED (mean +0.019, 0 regressions, 3 improvements)
- **120q full at MIN=10/p95**: FAILED per-question (mean parity 0.413=0.413, 7 boundary regressions, 7 offsetting improvements)

Per-category 120q breakdown:

| Category | n | Δ | Regressions ≥0.10 | Pattern |
|---|---|---|---|---|
| **architecture_structure** | 20 | **−0.015** | **3** | Concrete struct/file lookups; right citation often at rank 11–20 |
| business_logic_constraints | 20 | +0.010 | 1 | Net win |
| relationship | 6 | +0.017 | 0 | Net win |
| data_flow_integration | 20 | −0.005 | 2 | Cancels |
| cross_cutting_concerns | 20 | 0 | 1 | Cancels |
| (3 unchanged) | varies | 0 | 0 | Unchanged |
| **weighted total** | **120** | **0.000** | **7** | — |

The diagnosis is structurally category-specific. Phase 14 also produced V0019 production telemetry (current count: 198+ rows over the past 24h on `whk-wms`); Epic 0 of this sprint reads that for fresh-data confirmation before designing overrides.

**Phase chain.** Phase 14 → **Phase 14.1 (this)** → Phase 14.2 (Note 05 sparse fingerprints).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE14.1 |
| Title | Adaptive Per-Category Sparse Gate |
| Date | 2026-05-04 (plan), 2026-05-04 (start) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 14 narrow close (commit `e17a2b5`) |
| Successor | Phase 14.2 — Note 05 sparse fingerprints |
| Type | Code-small (~250 LOC production + ~120 LOC tests + ~30 LOC scripts/.env touch) |
| Risk | LOW (extends existing flag-off feature; default-flip only on A/B pass) |
| Budget | $15–25 OpenAI (3-preset 16q quick + 1 full 120q on winner) |
| Effort estimate | 3 dev-days |
| New TSDB migrations | None (V0019 already collects per-category-tunable rows) |
| New Neo4j migrations | None |
| Post-sprint artifacts | `internal/retrieval/gate.go` (extended ~80 LOC), `internal/config/config.go` (~40 LOC), `internal/models/models.go` (~5 LOC), `internal/api/handlers.go` (~10 LOC), `docs/tests/uvts/runners/uvts_ab_compare.py` (eps tolerance fix), `scripts/phase14_1_adaptive_runner.py` (new ~250 LOC), updated `docs/features/sparse-retrieval.md`, `phase_14_1_post.md` |

## 2. Problem Statement

Phase 14 demonstrated the gate produces **structurally category-specific** outcomes, not noise:
- `architecture_structure`: -0.015 net (the worst category, with concrete struct/file lookups whose right answer often sits at rank 11–20)
- `business_logic_constraints`, `relationship`: net wins
- 5 categories unchanged

A static MIN_ACTIVE / MAX_ACTIVE / percentile cannot satisfy both query shapes simultaneously. The gate is correct in concept (50% rerank-input reduction with mean parity is a real win); it's the per-category clamp that fails.

**Hypothesis**: a per-category override map (e.g. `{"architecture_structure": {"min_active": 20}}`) keeps the win on diffuse categories while preserving rank-11–20 citations for concrete categories. Acceptance: 120q A/B pass with mean ≥ baseline AND no per-question regression > 10%.

**Secondary issue**: the A/B comparator at `uvts_ab_compare.py:121` uses strict `<` for `delta < -threshold`. Floating-point boundary cases (`baseline=0.45 - candidate=0.35 = -0.10000000001`) trip as regressions. Phase 14's 7 reported regressions all rounded to display as `-0.10`. Adding an `eps=1e-6` tolerance reflects intent without weakening the bar.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Per-category override config | `internal/config/config.go` — new `SPARSE_GATE_CATEGORY_OVERRIDES` env (JSON map: `{"architecture_structure": {"min_active": 20, "percentile": 0.92}, ...}`) parsed once at startup; Validate() rejects unknown keys |
| 2 | Category resolution at gate site | `internal/retrieval/gate.go` — new `SparseGateOpts.CategoryOverrides` field. `ApplySparseGate` checks category, picks override if present, falls back to global defaults |
| 3 | Hint plumbing (request → gate) | `internal/models/models.go` — new `Category` request field. `internal/api/handlers.go` populates if URL param `?category=N` present (UVTS runner injects it from spec). `internal/retrieval/service.go` passes `req.Category` through to `SparseGateOpts` |
| 4 | UVTS runner injects category | `docs/tests/uvts/runners/uvts_runner.py` — extend the per-question retrieve call to include the spec's per-question `category` field as `?category=...` URL param |
| 5 | A/B comparator `eps` tolerance | `docs/tests/uvts/runners/uvts_ab_compare.py:121` — change `delta < -regression_threshold` to `delta < -(regression_threshold + 1e-6)` (or use `math.fsum`; the former is simpler and intent-preserving) |
| 6 | Adaptive ablation runner | `scripts/phase14_1_adaptive_runner.py` (new ~250 LOC) — extends the Phase 14 Epic 2 runner with category-override presets |
| 7 | Conditional default flip | If A/B passes, flip `SparseRetrievalEnabled=false → true` in `internal/config/config.go` + set the empirically-validated `SPARSE_GATE_CATEGORY_OVERRIDES` default in the same commit |
| 8 | Tier 1 unit tests for category dispatch | `internal/retrieval/gate_test.go` — extend with category override scenarios (override applies only to matching category, falls back when category missing, JSON parse errors) |
| 9 | Sprint plan + post + feature-doc update | `sprint_plan_phase_14_1_*.md` (this) + `phase_14_1_post.md` + update `docs/features/sparse-retrieval.md` |

**Out of scope (deferred):**

- Note 05 sparse fingerprints (Phase 14.2)
- Phase 13.2 per-category column-weight tuning (queued)
- Phase 13.6 backend-agnostic naming (queued)
- Adaptive percentile based on `consensus_strength` (research extension; not in this sprint)
- Per-space override maps (this sprint scopes only per-category; per-space is the natural Phase 14.1.1 if needed)

**Constraints (hard, MEMORY):**

- **Sequential epics** — diagnostic before implementation; A/B before default flip
- **No hardcoded values** — overrides through env JSON, not Go map literal
- **Plan-options pattern** — 4 forks documented in §13 below
- **Single batched commit at sprint close**
- **Sprint summary on PR comment**
- **CUIDv2 for any new IDs** (not adding any in this sprint)
- **`max_tokens ≥ 3000`, `latency_budget_ms ≥ 15000`** — no LLM call sites added
- **Live-testing required** (Tier 3) — A/B against real `whk-wms` + real grader
- **A/B merge gate** — same as Phase 14: B mean ≥ A AND no per-question regression > 10% (with new `eps=1e-6` tolerance)
- **Default flip conditional** on full 120q passing

## 4. Dependencies

**Consumed (code, pre-existing — reuse):**
- `internal/retrieval/gate.go` — Phase 14's gate code (extends, doesn't replace)
- `internal/retrieval/gate_test.go` — Phase 14's tests (extends)
- `internal/retrieval/service.go` — gate wiring (extends to pass category)
- `internal/models/models.go` — RetrieveRequest (adds Category field)
- `internal/api/handlers.go` — URL param parsing (adds `?category=`)
- `internal/config/config.go` — env-loading pattern (adds JSON parse for override map)
- `internal/tsdb/sparse_gate_writer.go` — V0019 writer (no schema change needed; could optionally land category in V0020 in a future sprint)
- `docs/tests/uvts/runners/uvts_runner.py` — per-question retrieve loop (adds `?category=` URL param injection)
- `docs/tests/uvts/runners/uvts_ab_compare.py` — comparator (adds eps tolerance)
- `scripts/phase14_epic2_sparse_ablation.py` — ablation runner (extends to category-override presets)

**Consumed (data):**
- TSDB V0019 `sparse_gate_metrics` rows from Phase 14 (~198 rows on `whk-wms` over past 24h) — Epic 0 input
- TSDB V0017 `retrieval_audit` rows (~280 rows/24h) — Epic 0 cross-check input
- Phase 13.1 candidate baseline grades at `/tmp/phase13_1_full/candidate/grades.json` (current production 120q)
- Phase 14 Epic 2 baseline grades at `/tmp/phase14_epic2/baseline-sparse-off/grades.json` (16q)
- Phase 14 Epic 2 candidate grades at `/tmp/phase14_epic2_full/sparse-p95-min10/grades.json` (the failing 120q)
- UVTS spec per-question `category` field at `docs/tests/uvts/specs/lnl_demo_validation.uvts.json`

**Consumed (compute):**
- mdemg HTTP API at `localhost:9999`
- llama-server at port 8102 (Phase 13.5 substrate)
- TimescaleDB at `localhost:5433`
- OpenAI API for UVTS grader (`gpt-5.4-mini`)

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate**: branch clean post-#366 merge, V0019 collecting, mdemg + llama-server healthy, TSDB schema 19.

### Epic 0 — Preflight + V0019 forensic confirmation

> Phase 14 Epic 2's per-category breakdown was based on grader output. This epic re-confirms the diagnosis from V0019 production rows — independent telemetry input — before designing overrides.

1. Query V0019 for the past 7 days of gate firings on `whk-wms`: per-call active_count, dropped_count, scorer_version distribution.
2. Cross-reference Phase 14 Epic 2 verdict regressions list (7 questions) against UVTS spec to map each regression's category.
3. Audit UVTS spec for category coverage: which categories have ≥10 questions in the 120q profile (statistical floor for per-category A/B verdict)?
4. Compute per-category mean active_count under the failed `MIN=10` config — confirms which categories sit at the clamp floor (gate over-aggressive) vs above it (gate already permissive enough).
5. If the V0019 data confirms the same pattern (architecture_structure dominant + 1-2 marginal categories), proceed with the spec'd override design. If V0019 shows new categories at risk, expand the override scope.

**Output**: `docs/development/post-ft-lora/phase_14_1_forensic.md` (~1 page) with:
- Per-category active_count distribution (V0019 data)
- Mapping of Phase 14 regressions to category coverage
- Recommended override map (likely: `architecture_structure: {min_active: 20}` only; possibly `data_flow_integration: {min_active: 15}` if V0019 supports it)

**Gate**: forensic doc committed; override map defaults backed by data.

### Epic 1 — Per-category override config + gate dispatch

1. `internal/config/config.go` — new `SparseGateCategoryOverrides map[string]SparseGateCategoryOverride` field on `Config`. `SparseGateCategoryOverride` struct: `MinActive *int, MaxActive *int, Percentile *float64` (pointers so missing fields fall back to global defaults). FromEnv parses `SPARSE_GATE_CATEGORY_OVERRIDES` JSON env. Validate() rejects category keys not in a known set + percentile-out-of-range + min>max.
2. `internal/retrieval/gate.go` — extend `SparseGateOpts` with `CategoryOverrides map[string]SparseGateOverride` and `Category string`. `ApplySparseGate` resolves the effective opts from `(opts.CategoryOverrides[opts.Category] ?? opts global defaults)` at the top of the function — minimal code surface change.
3. Tier 1 unit tests: override applies only to matching category; falls back when category missing from map; JSON parse error handling; pointer-nil-means-fallback semantics; multi-category override map.

**Gate**: tests green; lint clean; smoke retrieve with `?category=architecture_structure` shows override applied (different active_count than baseline).

### Epic 2 — Hint plumbing + UVTS runner injection

1. `internal/models/models.go` — add `Category string` to `RetrieveRequest`.
2. `internal/api/handlers.go` — parse `?category=...` URL param, populate `req.Category`.
3. `internal/retrieval/service.go` — `gateOpts.Category = req.Category`.
4. `docs/tests/uvts/runners/uvts_runner.py` — when iterating per-question, append `&category=<spec_category>` to the retrieve URL.
5. `docs/tests/uvts/runners/uvts_ab_compare.py:121` — change `delta < -regression_threshold` to `delta < -(regression_threshold + 1e-6)`.
6. Tier 2 integration test: full retrieve with category param + override config produces correctly-shaped response.

**Gate**: end-to-end UVTS runner injects category; comparator no longer false-positives on -0.10 boundary.

### Epic 3 — A/B sweep + verdict

1. `scripts/phase14_1_adaptive_runner.py` — extends Phase 14 Epic 2 runner with override-preset support. Three primary presets:
   - `baseline-sparse-off` (current production)
   - `adaptive-arch-only` (override: `architecture_structure: {min_active: 20}`)
   - `adaptive-arch-and-data-flow` (override: `architecture_structure: {min_active: 20}, data_flow_integration: {min_active: 15}`) — only if Epic 0 forensic supports it
2. 16q quick sweep: 3 presets vs baseline. Acceptance per preset: B mean ≥ A AND no per-question regression > 10% (with `eps=1e-6`).
3. Winning preset → 120q full A/B against `phase13_1-candidate-120q` baseline.
4. **Decision matrix**:
   - 120q passes → flip `SparseRetrievalEnabled=true` default + set `SPARSE_GATE_CATEGORY_OVERRIDES` default in same commit (matches Phase 13.1 conditional-flip pattern)
   - 120q fails → ship overrides flag-off-but-wired; scope Phase 14.1.1 with broader override scope OR per-space overrides

**Gate**: verdict captured; default flip applied if-and-only-if 120q passed.

### Epic 4 — Documentation (Final Epic — Never Cut)

- `docs/development/post-ft-lora/sprint_plan_phase_14_1_adaptive_per_category_gate.md` — frozen plan (this file)
- `docs/development/post-ft-lora/phase_14_1_post.md` — executed truth: A/B verdicts per preset, default flip rationale, OpenAI spend actual, decision-fork outcomes
- `docs/development/post-ft-lora/phase_14_1_forensic.md` — Epic 0 output
- `docs/features/sparse-retrieval.md` — update with Phase 14.1 outcome (default state, override env var, Phase 14.1.1 if scoped)
- `SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 14.1 EXECUTED with commit SHA
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md` `[Unreleased] ### Added/Changed`
- `CLAUDE.md` — extend the existing "Sparse Retrieval Gate" Architecture-Notes subsection with override env var + Phase 14.1 default state

**Gate**: all docs landed; cross-refs valid; conditional default flip applied.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- `gate_test.go` — per-category override resolution, fallback when category missing, pointer-nil-means-fallback, multi-category map
- `config_phase14_1_test.go` (new) — JSON env parse, Validate() rejects bad shapes, default empty map

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/sparse_gate_category_e2e_test.go` (new) — full `/v1/memory/retrieve?category=X` with override config produces different active_count than baseline

**Tier 3 (Live E2E) — MANDATORY:**
- **Epic 0 forensic** — V0019 production data audit
- **Epic 3 A/B sweep** — UVTS quick × 3 presets + full 120q on winner; real mdemg + real grader
- **Live retrieve smoke** — 5 retrieves with `?category=architecture_structure` (override active) and 5 with `?category=relationship` (override absent), confirm V0019 records correct active_count per category

**State restoration**: all changes additive or feature-flagged. Rollback = `git revert <commit>` + `SPARSE_GATE_CATEGORY_OVERRIDES=` empty in `.env`. Default-on flip is reverted by `SPARSE_RETRIEVAL_ENABLED=false`.

**Gate**: all 3 tiers green; verdict captured.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(retrieval): Sprint POST-FT-LORA-PHASE14.1 — adaptive per-category sparse gate (default-on per A/B verdict)`
- Body: Epic 0 forensic findings; A/B verdicts per preset; eps-tolerance fix; conditional default-flip rationale; OpenAI spend actual; fork outcomes; policy compliance checklist
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`
- Push → auto-PR → sprint summary comment posted

## 8. Verification Checklist

- [ ] Epic 0: forensic doc committed; override defaults data-cited
- [ ] Epic 1: tests green; gate code lint-clean; smoke shows override applied per-category
- [ ] Epic 2: hint plumbing + uvts_runner injection + eps tolerance landed
- [ ] Epic 3: 16q quick × 3 presets done; full 120q on winner done; verdict captured
- [ ] Epic 4: sprint plan + post + forensic + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md + sparse-retrieval.md
- [ ] Commit pushed; auto-PR opens; sprint summary posted
- [ ] Default flip applied if 120q passed (or documented if not)
- [ ] All 4 decision-fork outcomes disclosed
- [ ] `golangci-lint run ./...` clean
- [ ] `go test -race` green across affected packages
- [ ] OpenAI spend logged (target $15–25)
- [ ] CMS observation captured

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 4.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Override design wins quick but loses 120q (same shape as Phase 14) | Medium | Epic 0 forensic re-confirms diagnosis from V0019 before designing; multi-category preset variant if data shows additional risk | Ship flag-off; scope 14.1.1 with per-space scope or LLM-classified categories |
| 2 | UVTS runner can't inject `?category=` cleanly | Low | The spec already has per-question `category` field; runner change is ~5 lines | Inject as JSON body field instead of URL param (both code paths exist) |
| 3 | eps tolerance hides a real regression | Low | 1e-6 is well below the 0.001 grading granularity; only catches strict floating-point boundary | Use `0` and document the boundary issue; rely on operator review |
| 4 | Per-category dispatch adds latency | Low | Map lookup is O(1); single config-load JSON parse at startup | Profile if needed; cache resolved opts per category at gate entry |
| 5 | Categories not in UVTS spec hit the gate via real user requests | Low | `req.Category` empty → falls back to global defaults; no behavior change for un-categorized queries | Document; future sprint can auto-classify via LLM if material |
| 6 | Default flip causes user-visible behavior change in production | Medium | The flip only happens AFTER 120q passes; matches Phase 13.1 conditional-flip pattern | Operator opt-out via `SPARSE_RETRIEVAL_ENABLED=false` in `.env` |

## 11. Documents Accessed (during planning)

- `docs/development/post-ft-lora/phase_14_post.md` (Epic 2 verdict tables — primary input)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` (Epic 0 forensic)
- `docs/development/post-ft-lora/sprint_plan_phase_14_sparse_fingerprints_and_gate.md` (frozen)
- `docs/features/sparse-retrieval.md`
- `internal/retrieval/gate.go`, `gate_test.go`, `service.go`
- `internal/api/handlers.go`
- `internal/config/config.go`
- `internal/models/models.go`
- `docs/tests/uvts/runners/uvts_runner.py` (per-question loop)
- `docs/tests/uvts/runners/uvts_ab_compare.py:121` (regression check)
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` (per-question `category` field)
- `scripts/phase14_epic2_sparse_ablation.py` (extension target)
- `/tmp/phase14_epic2_full/sparse-p95-min10/verdict.json` (the 7-regression list)
- TSDB V0019 `sparse_gate_metrics` rows
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_sequential_epics.md`, `feedback_plan_options_pattern.md`, `feedback_live_testing_required.md`, `feedback_sprint_summary_on_pr.md`, `feedback_per_feature_docs_required.md`, `feedback_data_decides_not_operator.md`, `feedback_no_short_term_mlx_patches.md`

## 12. Rollback

All changes additive or feature-flagged.

1. `git revert <commit>` removes overrides + eps fix + docs
2. **Runtime disable** (no rebuild): `SPARSE_GATE_CATEGORY_OVERRIDES=` (empty) in `.env` + `mdemg restart`. Reverts to Phase 14 global-defaults gate behavior.
3. **Master toggle disable**: `SPARSE_RETRIEVAL_ENABLED=false` reverts to Phase 13.1 production state (gate off entirely).
4. **No schema rollback** (no migrations in this sprint).

Phase 14 + 13.1 + 13.5 + 13 artifacts untouched. V0017 + V0019 data preserved.

---

## 13. Plan-Options (decision forks — pick at execution, disclose in PR)

| # | Fork | Recommendation | Alternative(s) | Decision basis |
|---|---|---|---|---|
| 1 | **Override scope: per-category vs per-space vs both** | **per-category** | per-space (e.g. `whk-wms` vs `mdemg-dev` distinct overrides); both | Phase 14 evidence is category-level. Per-space adds complexity without supporting data. If 14.1 fails on whk-wms but Phase 14.2 finds different category profiles on other spaces, per-space becomes Phase 14.1.1 |
| 2 | **`SPARSE_GATE_CATEGORY_OVERRIDES` shape: JSON env vs YAML file vs DB-driven** | **JSON env** | YAML file (`/etc/mdemg/sparse_gate.yaml`); DB-driven (Neo4j config node) | Matches existing `RETRIEVAL_*` env-var pattern. JSON gets us per-environment control without infra changes. DB-driven is interesting for live tuning but premature given low data on category-stability over time |
| 3 | **`architecture_structure` MIN_ACTIVE override value: 15 vs 20 vs unbounded (no clamp)** | **20** (Phase 14 evidence: rank 11–20 citations need preserving) | 15 (less aggressive) | Phase 14 forensic identified rank 11–20 as the citation-loss zone. 20 = no effective gate for this category; 15 might cut some boundary cases. Default 20; sweep 15 if 20 wastes too much rerank cost |
| 4 | **Default flip when only 16q quick passes (not 120q)** | **NO — 120q required** | flip on quick alone (pragmatic; matches MIN=10 finding) | The 120q full revealed the per-category pattern that quick missed. Trusting 16q quick alone is exactly the false-confidence Phase 14 demonstrated. Stay strict |

If Epic 0 forensic data definitively contradicts a recommendation (e.g. V0019 shows `data_flow_integration` is also material), the data wins.
