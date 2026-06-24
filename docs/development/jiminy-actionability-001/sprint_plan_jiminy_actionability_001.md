# Sprint Plan — JIMINY-ACTIONABILITY-001: Bias What We SURFACE Toward Actionable Guidance (the fast near-term follow-rate lever)

## 1. Header & Metadata

- **Sprint ID:** JIMINY-ACTIONABILITY-001
- **Line dir:** `docs/development/jiminy-actionability-001/`
- **Date:** 2026-06-24
- **Branch:** `reh3376_dev01` (PR to `main`; never commit to `main`)
- **Target version:** **v0.11.0** (minor bump — additive surfacing-logic + config change; no schema change expected). **Shares the v0.11.0 collection-infrastructure arc with JIMINY-RELEVANCE-001 and HITL-REVIEW-001** — the three CHANGELOG entries read as one capability ("we persist + human-certify the evidence AND we bias what we surface toward the actionable class"). If this lands *after* a v0.11.0 cut, take **v0.12.0**. Decided at execution from the actual merge order (current released tag v0.9.0, CHANGELOG `[Unreleased]`).
- **TSDB schema:** **NO migration expected.** This sprint changes the **surfacing/selection logic + config** and *reads* the existing `constraint_outcomes` sink (migration 011, written by `/v1/jiminy/feedback`) to measure the win. If — and only if — execution finds a migration genuinely required (it should not), it coordinates the migration number with the two siblings (whichever of `027`/`028` is free) and bumps `TSDB_REQUIRED_SCHEMA_VERSION` in the same PR; **the strong default is zero migrations** (see §3 Constraints + §10 Risks).
- **Effort:** ~2–3d (Lever A is a self-contained selection-step change; Lever B reuses the existing synthesizer + warm-compute budget; Lever C is a flagged stretch only if A+B under-deliver).
- **Risk:** medium — the change is on the **per-prompt guidance hot path** (`Guide()` in `internal/jiminy/service.go`), and a wrong reweighting could *starve* genuinely-useful constraints or surface low-value actionables. Mitigations: every lever is config-gated + default-preserving until measured, and the binding gate is an **A/B (before/after) live comparison observed in TSDB** — composition shift AND follow-rate movement.
- **Lineage:** the **near-term composition lever explicitly held out of JIMINY-RELEVANCE-001** (its §3 Out-of-scope names this sprint: *"a strong candidate for a PARALLEL near-term sprint — proposed name `jiminy-actionability-001`"*). Scoped by the same **Step-1 diagnostic** `docs/development/jiminy-relevance-001/diagnostic_ignored_population.md` — specifically its **Findings 2, 3, and 5**. This sprint is **deliberately disjoint** from its two siblings so each validates independently: **JIMINY-RELEVANCE-001 changes what we STORE/MEASURE** (persist the evidence corpus + the should-follow metric), **HITL-REVIEW-001 is the human review/reinforcement PLATFORM**, and **this sprint changes what we SURFACE** (the guidance composition at `Guide()` time). One sprint touches the writer plane, one the review plane, one the surfacing plane — coupling them would make each impossible to validate.

## 2. Problem Statement

The Step-1 diagnostic (live `mdemg-dev`, 30-day window, 2,561 outcome rows) established that **guidance is ignored because it is not actionable, not because it is off-topic** — and that the root cause is structural, in *what gets surfaced*:

- **Finding 2 — composition is dominated by non-actionable abstractions.** 90% of surfaced guidance is the emergent-principle abstraction class (`pattern` 40% / `learning` 39% / `concept` 12% = 2,313 rows), ignored **53–65%** of the time. The actionable class (`constraint` 5% / `correction` 4% = 248 rows, 10% of volume) is followed **~2× better** / ignored roughly half as often (`constraint` 30% ignored, `correction` 27% ignored).
- **Finding 3 — ignored ≠ off-topic.** **950 of 984 (96%) of LLM-classified ignored rows are at similarity > 0.8.** Guidance is semantically adjacent to the action but does not drive a specific action — the signature of *not-actionable*, not *irrelevant*. Retrieving "more relevant" abstractions will not help; the abstractions are *already* relevant. They need to be **actionable**.
- **Finding 5 — the structural driver.** The graph holds **111 `role_type='constraint'` nodes vs 19,147 abstraction nodes (layer ≥ 2 / HiddenPattern) — a 172:1 ratio.** Retrieval over this pool *structurally* surfaces abstractions over actionable constraints. This is the RRF-SCALE-001 *"retrieval surfaces emergent_concept abstractions, not raw constraint nodes"* class, now quantified.

The two siblings address the **long arc**: JIMINY-RELEVANCE-001 starts a 3–6-month corpus so a future retrain has trustworthy evidence; HITL-REVIEW-001 builds the human-certification platform that grades it. **Neither moves follow-rate this month** — the corpus has to accumulate, and a retrain is a future-triggered sprint.

**This sprint is the fastest independent lever to raise guidance follow-rate NOW** — without waiting on the corpus or a retrain. It **biases what gets SURFACED toward the actionable class** (and/or **makes abstraction-type guidance actionable at surface time**), entirely inside the existing `Guide()` assembly + the existing synthesizer, gated by config, measured against the existing `constraint_outcomes` follow-rate sink. It does **not** persist a new corpus, build a review tool, change retrieval scoring globally, or retrain a model (the first two are the siblings; the last is the future-triggered retrain).

## 3. Scope & Constraints

**The design space — PROPOSED OPTIONS (propose-in-plan / decide-at-execution / disclose-in-PR).** Recommend a **phased combination**, do not over-commit:

- **Lever A — surface-composition reweighting (RECOMMENDED, phase 1; cheapest, no LLM, biggest structural lever; attacks Finding 2).** Bias guidance-item *selection* toward actionable `role_type`s (`constraint`/`correction`) over the abstraction class (`pattern`/`learning`/`concept`/`suggestion`). Plugs into the selection step of `Guide()` (`internal/jiminy/service.go`, between dedup ~L941 and truncate-to-`maxItems` ~L955). Three composable mechanisms, all config-driven, all default-preserving until measured:
  1. **Per-type surfacing weight** — a configurable multiplier applied to each item's sort key (`guidanceSortKey`) so actionable types out-rank abstractions of equal priority/confidence. `JIMINY_SURFACE_ACTIONABLE_WEIGHT` (default `1.0` = no-op; bump at execution after the A/B baseline).
  2. **Minimum-actionable quota** — guarantee at least N (or a fraction) of the surfaced `maxItems` are actionable when actionable candidates exist (reserve slots before truncation). `JIMINY_SURFACE_MIN_ACTIONABLE` (count) / `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION`.
  3. **Abstraction cap** — cap the fraction of the surfaced set that may be the abstraction class, so the 90% can't re-form. `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` (default `1.0` = no cap).
- **Lever B — abstraction→directive synthesis (RECOMMENDED, phase 2; makes the REMAINING abstractions actionable; attacks Finding 3).** When an abstraction-type item IS surfaced, rewrite it into an **imperative, specific directive** at synthesis time ("you MUST do X") instead of advisory prose ("Foundational principle: corrections, phase, never"). **Reuses the existing `GuidanceSynthesizer.Synthesize` (`internal/jiminy/synthesizer.go`) invoked inside `Guide()` (~L1139) within the GUIDANCE-SYNTH-001 warm-compute budget** — a directive-mode prompt variant / instruction that biases the synthesized narrative toward imperative phrasing for abstraction items. **HARD CONSTRAINTS:** respect the warm-compute budget (`JIMINY_WARM_COMPUTE_TIMEOUT_MS`, default 90000); bound the prompt (no growth-with-state); **fire-and-forget on the warm path** — do **NOT** add a new blocking LLM call to the prompt hot path; `max_tokens ≥ 3000`; `latency_budget_ms ≥ 15000` (house rules). `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` (default `false` until measured).
- **Lever C — retrieval-side bias for the guidance path (FLAGGED STRETCH / contingency only; attacks Finding 5; HIGHER RISK).** Bias the *guidance retrieval* toward `role_type='constraint'` nodes to counter the 172:1 surfacing imbalance — e.g. a per-role-type retrieval-score boost or a constraint-node inclusion floor on the consulting path (`internal/consulting/service.go` `findApplicableConstraints` / the `CONSULTING_*_SCORE_FLOOR` family). ⚠️ **This is RRF-SCALE-001 territory.** It MUST gate via config with RRF-calibrated defaults / scale-invariant signals, **NEVER a hardcoded score threshold**, and MUST **re-audit every `RetrieveResult.Score`/`.Activation` comparison it touches**. `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED` (default `false`). **Recommended: do NOT build C unless A+B under-deliver** the follow-rate movement at the Epic-5 gate — it is a contingency, not a planned epic.

**Recommended phasing:** **A first** (cheap, no LLM, biggest structural lever — directly attacks the 90:10 composition), **then B** (makes the abstractions that still surface actionable), **C held as a flagged stretch/contingency** behind a default-off flag, built only if the A/B at Epic 5 shows A+B insufficient. Each lever is independently flagged so the A/B can attribute the win.

**In scope (sequential epics — do NOT parallelize; docs-before-implementation within reason):**

1. **Baseline + A/B measurement harness (Epic 1)** — establish the before-state from the existing `constraint_outcomes` sink (composition % by `guidance_type`, follow-rate, ignore-rate) and stand up the A/B/before-after comparison that every later epic is gated on. **No surfacing change yet** — measurement first so the win is provable.
2. **Lever A — surface-composition reweighting (Epic 2)** — the per-type weight + min-actionable quota + abstraction cap in the `Guide()` selection step; config-driven, default-preserving.
3. **Lever B — abstraction→directive synthesis (Epic 3)** — directive-mode synthesis for abstraction items, within the warm-compute budget, fire-and-forget, default-off.
4. **Live A/B + tune (Epic 4)** — flip the levers on the live stack, run the before/after comparison, tune the weights/quota from the observed composition shift + follow-rate movement.
5. **Lever C — retrieval-side constraint bias (Epic 5, CONTINGENT)** — built **only if** Epic 4's A/B shows A+B insufficient; RRF-SCALE-001-safe, default-off, score-comparison re-audit.
6. **Documentation (Epic 6 — final, never cut)** — `docs/features/guidance-actionability.md`, CHANGELOG, `post.md` stub, CLAUDE.md Architecture-Notes entry.

**Out of scope (documented, with forward references):**

- **Persisting a guidance-training corpus / the should-follow metric.** That is **JIMINY-RELEVANCE-001** (changes what we STORE/MEASURE). This sprint *reads* the existing `constraint_outcomes` sink to measure; it persists nothing new. It can **upgrade its measurement to the should-follow metric when JIMINY-RELEVANCE-001 lands** (see §6) but does **NOT hard-depend** on it.
- **The human review / live-reinforcement platform.** That is **HITL-REVIEW-001**.
- **A model retrain.** Future-triggered (CLAUDE.md "recursive-retraining loop, FT Phases 6/7/9 — NOT STARTED" + FT-CLASSIFY-002), fed by JIMINY-RELEVANCE-001's corpus. This sprint is the *behavioral* lever that buys follow-rate **now** while that corpus accumulates.
- **Changing the substrate's 172:1 node ratio** (creating more constraint nodes / fewer abstractions). That is a data-hygiene / emergence-tuning concern, not a surfacing concern. Lever C *biases around* the ratio at surface time; it does not fix the ratio.

**Constraints:**

- **No-hardcoding:** every new value is an env var / config field with a sensible default (concrete names in §5). Defaults are **no-op / default-preserving** until the A/B measures the right setting (`*_WEIGHT=1.0`, `*_MAX_ABSTRACTION_FRACTION=1.0`, `*_DIRECTIVE_SYNTHESIS_ENABLED=false`, Lever C off) — the sprint ships *safe*, the operator/A-B turns the lever.
- **No new TSDB migration expected** — this sprint changes surfacing logic + config and READS existing outcome data. If execution finds a migration genuinely needed, **flag it + the `TSDB_REQUIRED_SCHEMA_VERSION` bump + the `027`/`028` coordination with the two siblings** (whichever number is free); strong preference: **none**.
- **CUIDv2** for any new identifier (`github.com/nrednav/cuid2`; never UUID) — though this sprint introduces **no new persisted identifier** (it adds no rows).
- **Respect the synthesis + latency budgets** (GUIDANCE-SYNTH-001): Lever B stays inside `JIMINY_WARM_COMPUTE_TIMEOUT_MS`, bounds its prompt, is fire-and-forget on the warm path; `max_tokens ≥ 3000`; `latency_budget_ms ≥ 15000`. **Never block the prompt hot path on a new LLM call.**
- **Respect RRF-SCALE-001** if Lever C is built: config-gated, RRF-calibrated defaults, re-audit every `Score`/`.Activation` comparison touched; never a hardcoded score literal.
- **Sequential epics**, gates between each.
- **Live Tier-3 required** (real `bin/mdemg` + real Neo4j + TSDB + llama-server, observable output), with a **binding A/B / before-after design** (§6).
- The surfacing change must **not regress** the existing `/v1/jiminy/guide` + `/v1/jiminy/warm` + `/v1/jiminy/feedback` UATS contracts (`make test-api`).

## 4. Dependencies & Pre-Conditions

- **Code touch-points:**
  - `internal/jiminy/service.go` — `Guide()`; the **selection step** between `deduplicateItems` (~L941) and the `len(filtered) > maxItems` truncation (~L955) is where **Lever A** plugs in (per-type weight into `guidanceSortKey` / `sort.Slice` ~L946, quota-reserve + abstraction-cap before truncate); the `SourceCounts` loop (~L989) is the natural in-process composition observation point; the synthesizer invocation (~L1139, `s.synthesizer.Synthesize(ctx, filtered, req.Context, req.AgentOutput)`) is where **Lever B**'s directive mode is selected.
  - `internal/jiminy/synthesizer.go` — `GuidanceSynthesizer.Synthesize(ctx, items, agentContext, agentOutput)` (~L59) + `NewGuidanceSynthesizer` (~L37); **Lever B** adds a directive-mode prompt variant here (bounded prompt, within budget).
  - `internal/jiminy/types.go` — `GuidanceType` constants (`constraint`/`correction` = actionable; `pattern`/`learning`/`concept`/`suggestion` = abstraction class); `GuidanceItem{Type, Content, Confidence, Priority, SourceNodes, ConstraintCode}`.
  - `internal/consulting/service.go` — `findApplicableConstraints` (~L1076) + `effective*ScoreFloor` helpers (~L61–78) + the `CONSULTING_*_SCORE_FLOOR` family — **only touched by the CONTINGENT Lever C.**
  - `internal/config/config.go` — the Jiminy config block (`JiminyMinConfidence` ~L288, `JiminyMaxItems` ~L287, `JiminyWarmComputeTimeoutMs` ~L312, the `JiminyConstraintCodeSimThreshold` family) + the `FromEnv()` no-hardcoding pattern; new `JIMINY_SURFACE_*` + `JIMINY_DIRECTIVE_SYNTHESIS_*` (+ contingent `JIMINY_GUIDANCE_CONSTRAINT_BIAS_*`) knobs.
  - Existing `constraint_outcomes` sink (TSDB, migration 011; cols incl. `guidance_type`, `outcome_type`, `similarity`, `classifier_source`, `time`) — **read-only**, the measurement basis.
- **Pre-conditions:** `JIMINY_ENABLED=true` on the live stack; the live hook channel surfacing guidance per prompt (HOOKWIRE-001 path); llama-server :8102 up (for Lever B synthesis); TSDB reachable with `constraint_outcomes` populated (the diagnostic's 2,561 rows confirm it is). **No HITL-REVIEW-001 / JIMINY-RELEVANCE-001 merge dependency** — this sprint is independent (it *optionally upgrades* its metric when JIMINY-RELEVANCE-001's should-follow gauge exists, but does not block on it).
- **Data dependency:** the actionable-vs-abstraction partition is a pure function of `GuidanceItem.Type` (`constraint`/`correction` vs `pattern`/`learning`/`concept`/`suggestion`) — resolvable in-process at surface time, **no Neo4j lookup on the hot path** (the type is already on the item). The A/B reads `constraint_outcomes.guidance_type` for the same partition.

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Plan (this document).** Gate: plan written, v1.0 format-conformant, scope reconciled with the diagnostic + held disjoint from the two siblings.

---

**Epic 1 — Baseline + A/B measurement harness (measure first, so the win is provable).**

- **Establish the before-state** from the live `constraint_outcomes` sink: composition % by `guidance_type` (expect ≈ 90% abstraction / 10% actionable per Finding 2), follow-rate + ignore-rate overall and **broken out `guidance_type × outcome`** (expect abstraction ignored 53–65%, actionable ~½ that). Capture as a committed baseline artifact `docs/development/jiminy-actionability-001/baseline_composition.md` (the SQL + the numbers + the window).
- **A/B / before-after harness:** a small reproducible comparison — a script or a documented query pair that computes `{abstraction_fraction_surfaced, follow_rate, ignore_rate}` over a window, runnable **before** (levers off) and **after** (levers on), with an explicit verdict rule (composition shift toward actionable AND follow-rate non-decreasing / ignore-rate decreasing on the actionable+abstraction sets). Reuse the existing `scripts/jiminy_effectiveness_report.py` (`--space-id mdemg-dev --days 7`) as the follow-rate basis where it already computes this; extend only if it lacks the `guidance_type` breakdown.
- **In-process composition observation:** add a Prometheus gauge family `mdemg_jiminy_surfaced_actionable_fraction{space_id}` (+ `_abstraction_fraction`) emitted from the `SourceCounts` computation in `Guide()` so the **surfaced** composition (not just the outcome-side composition) is observable live on Grafana — this is what Lever A directly moves, and the A/B reads it.
- **Gate G1:** baseline artifact committed with real numbers from the live sink; the A/B harness runs and emits the three quantities for the current (levers-off) state; the surfaced-composition gauge emits on a live retrieve. **No surfacing behavior changed yet** (defaults are no-ops).

---

**Epic 2 — Lever A: surface-composition reweighting.**

- **Per-type surfacing weight** into the existing sort: extend `guidanceSortKey` (or the `sort.Slice` comparator ~L946) so an item's effective rank within equal priority is multiplied by a per-type weight — actionable types (`constraint`/`correction`) get `JIMINY_SURFACE_ACTIONABLE_WEIGHT` (default `1.0`), abstraction types get `1.0` baseline; the weight is the tunable that the A/B sets. **Ordering only** — it does not change the `JiminyMinConfidence` selection floor (selection is still confidence-gated; this re-orders within the selected set, like DORMANT-CENSUS-001's signal sort).
- **Minimum-actionable quota** before truncation: when the selected set has actionable candidates, reserve up to `JIMINY_SURFACE_MIN_ACTIONABLE` (count, default `0` = off) or `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION` (default `0.0` = off) of the `maxItems` slots for actionable items, so a high-confidence abstraction wall can't crowd every actionable item out of the truncation window.
- **Abstraction cap** before truncation: `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` (default `1.0` = no cap) caps the abstraction share of the surfaced set; overflow abstraction items are dropped from the tail (lowest sort key first), never the actionable items.
- **Config block** (`internal/config/config.go`, no-hardcoding, all default-preserving): `JIMINY_SURFACE_ACTIONABLE_WEIGHT` (default `1.0`, floor `>0`), `JIMINY_SURFACE_MIN_ACTIONABLE` (default `0`), `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION` (default `0.0`, range `[0,1]`), `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` (default `1.0`, range `(0,1]`). A single helper `isActionableType(GuidanceType) bool` is the one place the actionable partition is defined (re-used by Epic 1's gauge, Epic 3's directive selection, and the A/B).
- **Gate G2:** `go build ./...` green; unit tests (below) pass; with all four knobs at default, surfaced output is **byte-identical** to pre-sprint (default-preserving proof); with a non-default weight/quota/cap on a synthetic item set, actionable items move up / the abstraction cap holds / the quota reserves slots; UATS `/v1/jiminy/guide` + `/warm` still green.

---

**Epic 3 — Lever B: abstraction→directive synthesis.**

- **Directive-mode synthesis:** when `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`, the synthesizer renders abstraction-type items as **imperative, specific directives** ("you MUST do X", "before Y, do Z") rather than advisory prose. Implemented as a **prompt variant** inside `GuidanceSynthesizer.Synthesize` (`internal/jiminy/synthesizer.go`) — a directive instruction block prepended to the existing synthesis prompt, biasing the LLM to convert principle-statements into concrete actions, applied to the abstraction subset (`!isActionableType`) while passing actionable items (`constraint`/`correction`) through as-is (they are already imperative).
- **Budget discipline (HARD):** the directive instruction is a **fixed, bounded** addition (no growth with state — APE-PROMPT-BUDGET-001 class); the call stays inside the existing `JIMINY_WARM_COMPUTE_TIMEOUT_MS` budget and the synthesizer's circuit breaker; it is **fire-and-forget on the warm path** (the synthesizer already runs inside `Guide()` under the warm budget — Lever B does not add a *new* call site, it changes the *prompt* of the existing one). `max_tokens` for the synthesis call stays `≥ 3000`; the effective latency budget stays `≥ 15000ms`. **No new blocking LLM call on the prompt hot path.**
- **Config** (no-hardcoding): `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` (default `false`), `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS` (default `3500`, the bounded-prompt guard mirroring `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS`), and — if the directive prompt is a template — `JIMINY_DIRECTIVE_SYNTHESIS_INSTRUCTION` overridable (default the in-repo instruction).
- **Fallback:** when the synthesizer is unavailable / synthesis errors (the existing `synthesis_error` path), fall back to the static formatting exactly as today — directive mode never makes guidance *worse* than the current advisory prose; it is strictly additive on the synthesis success path.
- **Gate G3:** `go build ./...` green; unit test asserts the directive instruction is added only when enabled + only bounds the abstraction subset; the synthesis call respects the prompt-token bound (`MAX_PROMPT_TOKENS`); with the flag off, synthesis output is unchanged (default-preserving); a live `jiminy.synthesize` with directive mode on produces an imperative narrative within the warm budget (no timeout, `synthesis_used=true`).

---

**Epic 4 — Live A/B + tune (the binding gate).**

- **On the live stack**, run the Epic 1 A/B harness in two arms:
  - **Arm A (before):** all levers at default (no-op) — record `{abstraction_fraction_surfaced, follow_rate, ignore_rate}` over the window.
  - **Arm B (after):** enable Lever A (set `JIMINY_SURFACE_ACTIONABLE_WEIGHT` + `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` + a min-actionable quota to a measured-reasonable first setting) and Lever B (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`) — record the same three quantities.
  - Drive **N real prompts** through the live hook channel so real guidance is surfaced + fed back via `/v1/jiminy/feedback`, populating `constraint_outcomes` for both arms (windowed so the arms are separable — e.g. by toggling at a recorded timestamp and comparing pre/post windows, or by running the levers-on arm long enough to accumulate a comparable sample).
- **Verdict rule (binding):** Arm B's **surfaced abstraction fraction drops from ≈90% to ≤ a target `X%`** (target set at execution from the gauge, e.g. ≤ 60% as a first milestone) **AND** the **7-day follow-rate rises / ignore-rate falls** in `constraint_outcomes` (observed in TSDB / on the Grafana panel), with **no collapse of genuinely-useful constraint surfacing** (the actionable set's own follow-rate does not regress). **Tune** the weight/quota/cap from the observed shift — over-aggressive caps that starve good abstractions or surface low-value actionables are walked back.
- **Measurement upgrade (optional, non-blocking):** if JIMINY-RELEVANCE-001 has landed, prefer its **"should-follow follow rate"** gauge (`GUIDANCE_SHOULD_FOLLOW_*`) as the numerator — it excludes correctly-ignored advisory items, a cleaner denominator for this lever's win. The sprint **ships on the existing `constraint_outcomes` follow-rate (i)** and **upgrades to should-follow (ii)** when available — no hard dependency.
- **Gate G4:** the live A/B is run + recorded in `docs/development/jiminy-actionability-001/ab_results.md`; the surfaced abstraction fraction measurably dropped (gauge before/after); the follow-rate/ignore-rate moved in the right direction in TSDB without an actionable-set regression; the tuned config values are recorded for the PR disclosure.

---

**Epic 5 — Lever C: retrieval-side constraint bias (CONTINGENT — built ONLY if Epic 4 shows A+B insufficient).**

- **Trigger condition (state it; build only on trigger):** Epic 4's A/B shows the surfaced abstraction fraction did **not** reach the target OR the follow-rate did **not** move, because the *candidate pool itself* is too abstraction-dominated for A's reweighting to find enough actionable items to surface (the 172:1 ratio bottleneck, Finding 5). Only then build C.
- **Mechanism:** bias the guidance retrieval toward `role_type='constraint'` candidates — a per-role-type retrieval-score *boost* (not a hard floor) or a constraint-node inclusion guarantee on the consulting path (`findApplicableConstraints`), so more actionable candidates enter the pool A re-orders. `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED` (default `false`), `JIMINY_GUIDANCE_CONSTRAINT_BIAS_WEIGHT` (RRF-calibrated default, e.g. a modest multiplicative boost; **NOT a hardcoded absolute score threshold**).
- ⚠️ **RRF-SCALE-001 safety (HARD):** config-gated; RRF-calibrated default; **re-audit every `RetrieveResult.Score`/`.Activation` comparison the bias touches** (the consulting `CONSULTING_*_SCORE_FLOOR` gates are already RRF-calibrated — do not regress them); never introduce a hardcoded score literal; prefer a scale-invariant signal (role-type partition) over a score threshold. Document the audit in `docs/development/jiminy-actionability-001/leverc_score_audit.md`.
- **Gate G5 (only if built):** with the flag off, retrieval is byte-identical (default-preserving); with it on, the candidate pool carries more `role_type='constraint'` nodes; the A/B re-run shows the abstraction fraction reaching the target; the score-comparison audit is clean (no hardcoded literal, no RRF-SCALE-001 regression).

---

**Epic 6 — Documentation (final, never cut).**

- `docs/features/guidance-actionability.md` (Why / Choices / How it works / How to use).
- CHANGELOG `[Unreleased] → Added` under v0.11.0 (coordinated with the two siblings).
- `docs/development/jiminy-actionability-001/post.md` stub.
- CLAUDE.md Architecture-Notes entry (the JIMINY-ACTIONABILITY-001 paragraph: the surfacing levers, the `JIMINY_SURFACE_*` + `JIMINY_DIRECTIVE_SYNTHESIS_*` config, the surfaced-composition gauge, the A/B result, that this is the *surface* plane disjoint from JIMINY-RELEVANCE-001's *store/measure* plane + HITL-REVIEW-001's *review* plane, and the contingent Lever C / RRF-SCALE-001 note).
- **Gate G6:** docs present; CHANGELOG + CLAUDE.md updated; tree clean.

## 6. Testing Plan (3 tiers — unit + integration + live Tier-3)

**Tier 1 (unit):**
- **`isActionableType`** partition: `constraint`/`correction` → actionable; `pattern`/`learning`/`concept`/`suggestion` → abstraction; the one definition used everywhere (regression-pins the partition).
- **Lever A selection:** with default knobs, the selected+ordered set is **identical** to the pre-sprint sort (default-preserving); with a non-default `ACTIONABLE_WEIGHT`, actionable items out-rank equal-priority/confidence abstractions; `MIN_ACTIONABLE` reserves slots (actionable items survive truncation even when out-scored); `MAX_ABSTRACTION_FRACTION` caps the abstraction share and drops abstraction-tail first (never actionable); quota+cap interaction is consistent.
- **Lever B directive synthesis:** the directive instruction is added only when `DIRECTIVE_SYNTHESIS_ENABLED`; it applies to the abstraction subset only; the prompt respects `MAX_PROMPT_TOKENS` (bounded — drop-oldest/gating, no unbounded growth); with the flag off, the synthesis prompt is unchanged.
- **Config:** every new env var parses, defaults are the no-op/default-preserving values, floors/ranges enforced (`WEIGHT > 0`, fractions `[0,1]`/`(0,1]`, `MAX_PROMPT_TOKENS` ≥ a floor; the synthesis call's effective `max_tokens ≥ 3000` and `latency_budget_ms ≥ 15000` honored); config scanner sees every new knob (no-hardcoding); `TSDB_REQUIRED_SCHEMA_VERSION` **unchanged** (no migration).
- **Surfaced-composition gauge:** computes the actionable/abstraction fractions from a synthetic surfaced set correctly.

**Tier 2 (integration):** `go test ./internal/jiminy/... ./internal/consulting/... ./internal/config/...`; `golangci-lint run ./...` (0 issues); **UATS contract** for `/v1/jiminy/guide`, `/v1/jiminy/warm`, `/v1/jiminy/feedback` still green (`make test-api`) — the surfacing change re-orders/caps items + may render directives, but the response *contract* (shape) is unchanged. No migration to apply (assert schema version unchanged in CI).

**Tier 3 (LIVE — required; the binding A/B gate):** on the running stack (real `bin/mdemg` + real Neo4j + TSDB + llama-server):
- **A/B before/after (the binding deliverable):** with levers **off**, drive N real prompts → observe the surfaced abstraction fraction ≈ 90% (gauge) + the `constraint_outcomes` follow/ignore rates (baseline). Then enable Lever A + Lever B → drive N real prompts → **observe in TSDB/Grafana** that **the surfaced guidance set shifts from ~90% abstraction to ≤ X% abstraction** (the surfaced-composition gauge moves) **AND** the **7-day follow-rate rises / ignore-rate falls** for the surfaced sets, **without an actionable-set follow-rate regression**. Record both arms in `ab_results.md`.
- **Lever B live narrative:** a real `/v1/jiminy/warm` (or `/guide`) with directive mode on produces an **imperative, specific** synthesized narrative (`synthesis_used=true`) within the warm-compute budget (no timeout) — qualitatively distinct from the advisory-prose baseline the diagnostic's live sample showed ("Foundational principle: …" → "you MUST …").
- **Hot-path latency unaffected:** confirm the prompt hot path latency is unchanged (Lever A is in-process ordering; Lever B is the existing fire-and-forget warm synthesis — no new blocking call).
- **(Contingent) Lever C live:** if built, re-run the A/B and confirm the candidate pool carries more constraint nodes + the abstraction fraction reaches target, with the score-audit clean.
- **Restore state** after the A/B (revert the levers to the tuned/agreed config; confirm `mdemg-dev` outcome data is intact — this sprint only *reads* `constraint_outcomes`, it writes none).

## 7. Commit Strategy

Conventional commits, one per logical unit / epic:
- `feat(jiminy-actionability-001): baseline + surfaced-composition gauge + A/B harness`
- `feat(jiminy-actionability-001): Lever A surface-composition reweighting (weight + quota + cap)`
- `feat(jiminy-actionability-001): Lever B abstraction→directive synthesis (warm-budget, default-off)`
- `feat(jiminy-actionability-001): live A/B results + tuned config` (config/doc only — the code landed in Epics 2/3)
- `feat(jiminy-actionability-001): Lever C retrieval-side constraint bias` *(ONLY if the Epic-4 A/B triggers it — RRF-SCALE-001-audited)*
- `docs(jiminy-actionability-001): feature doc + CHANGELOG + post + CLAUDE.md`

gofmt/vet + lint each; push once at the end (auto-PR fires — do **NOT** manually create the PR); add the sprint summary to the PR comments. **Propose options in the plan, decide at execution, disclose in the PR** (the lever-phasing decision + the tuned weights + whether Lever C was built). **Live-surprise fixes get their own fix-commit** (Phase 11.6.2 precedent) — never fold them silently into an epic commit.

## 8. Verification Checklist

- [ ] `go build ./...` green; `golangci-lint run ./...` 0 issues
- [ ] **No TSDB migration** (`TSDB_REQUIRED_SCHEMA_VERSION` unchanged); if one was genuinely unavoidable, the `027`/`028` sibling coordination + version bump are in the same PR (strong preference: none)
- [ ] All new values are env vars / config fields with **default-preserving** defaults; config scanner clean (no-hardcoding)
- [ ] No new persisted identifier (sprint adds no rows); any incidental id is CUIDv2 (not UUID)
- [ ] **Default-preserving proof:** with every new knob at default, surfaced guidance + synthesis output are byte-identical to pre-sprint
- [ ] Lever A: per-type weight re-orders within priority; min-actionable quota reserves slots; abstraction cap holds + drops abstraction-tail first; `isActionableType` is the single partition definition
- [ ] Lever B: directive mode applies to the abstraction subset only, bounded prompt (`MAX_PROMPT_TOKENS`), inside `JIMINY_WARM_COMPUTE_TIMEOUT_MS`, fire-and-forget, `max_tokens ≥ 3000`, `latency_budget_ms ≥ 15000`, no new blocking hot-path LLM call; static fallback on synthesis error
- [ ] Surfaced-composition gauge (`mdemg_jiminy_surfaced_actionable_fraction` / `_abstraction_fraction`) emits live
- [ ] Tier 1 unit + Tier 2 integration + UATS (`/guide` + `/warm` + `/feedback`) green
- [ ] **LIVE A/B:** surfaced abstraction fraction drops from ~90% to ≤ target X% (gauge before/after) AND follow-rate rises / ignore-rate falls in `constraint_outcomes` (TSDB/Grafana) AND no actionable-set follow-rate regression; both arms recorded in `ab_results.md`
- [ ] **LIVE:** Lever B produces an imperative synthesized narrative within budget (`synthesis_used=true`, no timeout)
- [ ] Prompt hot-path latency unaffected
- [ ] **(Contingent) Lever C:** built only if the Epic-4 A/B triggered it; RRF-SCALE-001-safe (config-gated, RRF-calibrated default, score-comparison audit clean — `leverc_score_audit.md`); default-off byte-identical
- [ ] Measurement upgrade to JIMINY-RELEVANCE-001's should-follow metric noted (optional, non-blocking)
- [ ] `docs/features/guidance-actionability.md` + CHANGELOG (v0.11.0) + `post.md` + CLAUDE.md Architecture-Notes entry
- [ ] Disjointness from the two siblings documented (surface plane vs store/measure plane vs review plane)
- [ ] Working tree clean; pushed; auto-PR created; sprint summary on PR; epics executed sequentially with gates

## 9. Documentation Update (final epic — never cut)

**Epic 6** delivers `docs/features/guidance-actionability.md` with the four mandatory sections:
- **Why:** the diagnostic's Findings 2/3/5 — 90% of surfaced guidance is the abstraction class ignored 53–65%; ignored ≠ off-topic (96% sim > 0.8 — not-actionable, not irrelevant); the 172:1 abstraction:constraint substrate that structurally surfaces abstractions. The fast lever: bias *what we surface* toward the actionable class / make abstractions actionable at surface time — **now**, without waiting on the 3–6-month corpus or a retrain.
- **Choices:** Lever A (surface-composition reweighting — cheapest, no LLM, biggest structural lever) phased first; Lever B (abstraction→directive synthesis — reuse the existing synthesizer + warm budget) second; Lever C (retrieval-side constraint bias — RRF-SCALE-001 territory) held as a default-off contingency built only if A+B under-deliver; **default-preserving** ship (the operator/A-B turns the lever, the sprint ships safe); read the existing `constraint_outcomes` sink for measurement vs persist a new corpus (the latter is JIMINY-RELEVANCE-001's job).
- **How it works:** the `Guide()` selection step (weight → quota → cap, between dedup and truncate), the `isActionableType` partition, the directive-mode synthesis prompt inside the warm-compute budget (bounded, fire-and-forget), the surfaced-composition gauge, the A/B verdict rule, the contingent Lever C + its score-audit.
- **How to use:** the `JIMINY_SURFACE_*` + `JIMINY_DIRECTIVE_SYNTHESIS_*` (+ contingent `JIMINY_GUIDANCE_CONSTRAINT_BIAS_*`) env vars + their defaults, how to read the surfaced-composition gauge + the follow-rate panel, how to run the A/B before/after, and how to upgrade the measurement to the should-follow metric once JIMINY-RELEVANCE-001 lands.

Plus CHANGELOG (v0.11.0 Added — coordinated with the two siblings), `post.md` stub, and the CLAUDE.md Architecture-Notes paragraph.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Reweighting **starves genuinely-useful constraints** or surfaces low-value actionables (over-correction) | **High** | Default-preserving ship (knobs default to no-op); the binding gate is a **live A/B** that watches the *actionable-set's own follow-rate* for regression, not just the aggregate; tune from the gauge; quota/cap walk-back disclosed in the PR. |
| The change is on the **per-prompt guidance hot path** | High | Lever A is in-process re-ordering (no I/O); Lever B reuses the existing fire-and-forget warm synthesizer (no new blocking call); hot-path latency unchanged is an explicit live check. |
| Lever B blows the synthesis/latency budget (GUIDANCE-SYNTH-001 / APE-PROMPT-BUDGET-001 class) | High | Bounded directive prompt (`MAX_PROMPT_TOKENS`, no growth-with-state); stays inside `JIMINY_WARM_COMPUTE_TIMEOUT_MS` + the synthesizer circuit breaker; `max_tokens ≥ 3000`, `latency_budget_ms ≥ 15000`; static-format fallback on synthesis error; live "no timeout" check. |
| Lever C reintroduces an **RRF-SCALE-001 hardcoded-threshold** regression | **High** | Lever C is a **default-off contingency** built only on the Epic-4 trigger; config-gated with an RRF-calibrated default (never a hardcoded score literal); a committed score-comparison audit (`leverc_score_audit.md`) re-audits every `Score`/`.Activation` comparison touched; prefer the scale-invariant role-type partition over a score threshold. |
| The win isn't real (composition shifts but follow-rate doesn't move) | **High** | The verdict rule requires **both** the composition shift AND the follow-rate movement in TSDB; if composition moves but follow-rate doesn't, that's the trigger to escalate to Lever B / Lever C, disclosed honestly — the sprint does not claim a win on composition alone. |
| Entangling with the two siblings (scope creep into store/measure or review) | Med | Explicit disjointness: this sprint changes **surfacing logic + config only**, persists **nothing new**, builds **no review tool**; reads the existing `constraint_outcomes` sink for measurement; the should-follow upgrade is optional + non-blocking. |
| An unexpected migration need surfaces mid-sprint | Low | Strong default is none; if genuinely required, coordinate the free `027`/`028` number + bump `TSDB_REQUIRED_SCHEMA_VERSION` in the same PR, flagged in the PR — not discovered at merge time. |

## 11. Documents Accessed

- `docs/development/jiminy-relevance-001/diagnostic_ignored_population.md` (Step-1 diagnostic — factual basis; Findings 2/3/5 are this sprint's target; the live advisory-prose guidance sample)
- `docs/development/jiminy-relevance-001/sprint_plan_jiminy_relevance_001.md` (sibling — house style/voice; its §3 names this sprint as the held-out composition lever; the store/measure plane this sprint is disjoint from; the should-follow metric this sprint can optionally upgrade to)
- `docs/development/hitl-review-001/sprint_plan_hitl_review_001.md` (sibling — house style/voice; the review/reinforcement plane this sprint is disjoint from; the shared-v0.11.0 recommendation; the `027`/`028` migration coordination)
- `internal/jiminy/service.go` (`Guide()` ~L645+; the selection step — dedup ~L941, sort ~L946, truncate ~L955 — where Lever A plugs in; the `SourceCounts` composition point ~L989; the synthesizer invocation ~L1139 where Lever B's directive mode is selected; `JiminyMinConfidence` filter ~L932)
- `internal/jiminy/synthesizer.go` (`GuidanceSynthesizer.Synthesize` ~L59, `NewGuidanceSynthesizer` ~L37 — Lever B's directive-mode prompt site)
- `internal/jiminy/types.go` (`GuidanceType` constants — the actionable/abstraction partition: `constraint`/`correction` vs `pattern`/`learning`/`concept`/`suggestion`; `GuidanceItem`)
- `internal/consulting/service.go` (`findApplicableConstraints` ~L1076; `effective*ScoreFloor` ~L61–78; the `CONSULTING_*_SCORE_FLOOR` family — Lever C's contingent touch-point)
- `internal/config/config.go` (`JiminyMinConfidence` ~L288, `JiminyMaxItems` ~L287, `JiminyWarmComputeTimeoutMs` ~L312 + `JiminyWarmComputeTimeout()` ~L1195; the `FromEnv()` + no-hardcoding pattern; `CONSULTING_*_SCORE_FLOOR` defaults ~L1175)
- `scripts/jiminy_effectiveness_report.py` (the existing follow-rate report — the A/B follow-rate basis); live TSDB `constraint_outcomes` (migration 011 — the read-only measurement sink)
- CHANGELOG.md / git tags (current v0.9.0 → next minor v0.11.0, shared with the two siblings)
- CLAUDE.md / MEMORY.md (RRF-SCALE-001 score-scale contract; GUIDANCE-SYNTH-001 warm-compute budget + bounded concurrency; APE-PROMPT-BUDGET-001 bounded prompt; JIMINY-OUTCOME-001 constraint-code matching; no-hardcoding / CUIDv2 rules; min `max_tokens` 3000 / min `latency_budget` 15000; mandatory 3-tier + live testing; sprint-plan v1.0 format)

## 12. Rollback Procedures

- **Feature flags (the primary rollback — no code change):** set every new knob to its default to fully neutralize the sprint — `JIMINY_SURFACE_ACTIONABLE_WEIGHT=1.0`, `JIMINY_SURFACE_MIN_ACTIONABLE=0`, `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION=0.0`, `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION=1.0`, `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=false` (and `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=false` if Lever C was built). At defaults the surfacing + synthesis are byte-identical to pre-sprint.
- **No data to roll back:** this sprint **persists nothing** (it reads the existing `constraint_outcomes` sink for measurement). There is no migration, no new table, no new rows — disabling the levers simply restores the prior surfacing behavior; already-fed-back outcome rows are inert and unaffected.
- **Code revert:** reverting the sprint commits removes the selection-step reweighting + the directive-synthesis prompt variant + the surfaced-composition gauge; `Guide()` returns to its pre-sprint sort/truncate path. **No destructive operation on any existing data at any point** — this sprint only *re-orders/caps in-process* and *reads* outcomes.
- **No schema rollback** — no `TSDB_REQUIRED_SCHEMA_VERSION` change to revert (no migration). (If execution had been forced to add one, its additive table would be dropped manually + the version reverted, per the sibling rollback pattern — but the planned path is none.)
