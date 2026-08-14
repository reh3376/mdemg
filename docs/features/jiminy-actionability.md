# Jiminy Actionability — Surface Bias + Directive Synthesis (JIMINY-ACTIONABILITY-001)

## Why

Diagnostics (JIMINY-RELEVANCE-001) showed guidance is ignored not because it is *off-topic* but because it is *not actionable*: the abstraction class (`pattern`/`learning`/`concept`) is ~90% of surfaced/outcome guidance and is ignored 54–66%, while the actionable class (`constraint`/`correction`) is ~10% and ignored only 27–35%. This sprint is the near-term surfacing lever — bias what `Guide()` *surfaces*, and how it is *phrased*, toward the actionable.

## Choices

Two independent, default-off, config-driven levers:

- **Lever A — surface-composition reweighting.** A per-type sort weight (`JIMINY_SURFACE_ACTIONABLE_WEIGHT`), a min-actionable quota (`JIMINY_SURFACE_MIN_ACTIONABLE` / `_FRACTION`) reserving surfaced slots before the cut to `max_items`, and an abstraction cap (`JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION`) dropping the abstraction tail first. All default to no-ops, so at defaults the surfaced set is byte-identical to the prior plain truncation; actionable items are never dropped to satisfy the cap.
- **Lever B — directive synthesis.** When `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`, the synthesis system prompt is augmented to render abstraction-type evidence as **imperative, task-specific directives** ("Do X", "Before Y, do Z") instead of advisory prose, bounded by `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS`. Reuses the existing `jiminy.synthesize` call — no new LLM call on the hot path.

A surfaced-composition gauge (`mdemg_jiminy_surfaced_actionable_fraction` / `_abstraction_fraction`) measures the surfaced side directly.

## How it works

- `internal/jiminy/service.go`: `isActionableType` partitions types; `guidanceTypeWeight` applies Lever A's sort multiplier; `applyActionableComposition` enforces the quota + abstraction cap on the already-sorted list before truncation (default-preserving fast path when no quota + no cap). `Guide()` emits the surfaced-composition gauge from the final `filtered` set.
- `internal/jiminy/synthesizer.go` + `guidance_prompt.go`: in directive mode the system prompt gets `directiveSynthesisInstruction`, and `boundDirectivePrompt` keeps the user prompt within the token budget.

## How to use

| Env var | Default | Meaning |
|---|---|---|
| `JIMINY_SURFACE_ACTIONABLE_WEIGHT` | 1.0 (no-op) | Sort-key multiplier for actionable types |
| `JIMINY_SURFACE_MIN_ACTIONABLE` | 0 | Absolute min actionable items reserved |
| `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION` | 0.0 | Min actionable as a fraction of `max_items` |
| `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` | 1.0 (no cap) | Max abstraction items as a fraction of `max_items` |
| `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` | false (code; set `true` in the production `.env` since JIMINY-CORPUS-001 E5) | Render abstractions as imperative directives |
| `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS` | 3500 | Directive-mode prompt token bound |

## Live A/B result (Epic 4)

Levers off vs on, same 6 contexts against mdemg-dev (`docs/development/jiminy-actionability-001/epic4_live_ab.md`):

- **Lever B works** — directive synthesis produces imperative narratives ("it is imperative... You MUST clean up... You MUST NOT delete..."). This is the higher-impact lever: it makes guidance actionable in *phrasing* regardless of *type*.
- **Lever A is mechanically correct but modest** — surfaced actionable fraction 6.7% → 10.5%. It pulls actionables up *where they exist in the candidate pool* but cannot manufacture them; for most contexts retrieval surfaces no actionable candidates.
- **Finding:** the binding constraint is **upstream retrieval candidate composition**, not the surfacing cut.

## Lever C — constraint-inclusion (Epic 5; the lever that works)

Lever C addresses the Epic-4 finding directly: when `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=true`, `Guide()` runs a targeted query (`fetchActionableCandidates`) for the top-K `constraint`/`correction` nodes by **embedding cosine similarity** to the context (`vector.similarity.cosine` over the role-filtered set — *not* the RRF score; RRF-SCALE-001-safe) and merges them (dedup by node_id) into the candidate pool. The merged nodes are already correctly typed (the query filters role_type). The disclosed "adapter drops role_type" classification gap is **closed by JIMINY-ROLETYPE-ADAPTER-001** (2026-07-17); Lever C's role-filtered fetch and the general retrieval path now BOTH yield correctly-typed items.

**Config:** `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED` (false), `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK` (5), `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` (0.30 cosine — raise for tighter relevance).

**Live A/B (controlled, `ab_results.md`):** surfaced **actionable fraction 11.1% → 47.7%** (4.3×), abstraction **88.9% → 52.3%** — **clears the ≤60%-abstraction milestone** Lever A couldn't move. Surfaced constraints are query-relevant. ⚠️ A live-smoke fix was needed mid-sprint: the initial index-scan query (top-50 then role-filter) returned 0 because actionables are ~0.1% of nodes; the role-filtered cosine query fixed it.

## Shipping
All three levers ship **default-off in code**; operationally the production `.env` enables Lever B and Lever C (plus Lever A quota/weight settings), and the JIMINY-CORPUS-001 surface cooldown is default-on in code. **Operator recommendation: enable Lever C** (the actionable-composition mover) and Lever B (imperative phrasing). Lever A's quota/cap then shapes the now-actionable-rich pool. See `docs/development/jiminy-actionability-001/`.

## Follow-up — corpus cleanup + repetition control (JIMINY-CORPUS-001, 2026-07-03)
Enabling Lever C exposed that the `role_type='constraint'` partition it surfaces from was ~half junk and over-repeated. JIMINY-CORPUS-001 addressed the corpus itself:
- **Promotion gate** (`internal/hidden/constraint_gate.go`) stops junk observations (build/test status, bash errors, PR/sprint/phase-completion notes, doc dumps) from becoming constraint nodes — provenance obs_type deny-set + content patterns, config-driven, default-on.
- **Purge:** 140→61 live constraint nodes (tombstone-only, reversible), removing ~58% of the constraint surfacing noise.
- **Repetition control** (`internal/jiminy/surface_cooldown.go`): per-session cooldown on repeatedly-ignored nodes + an effectiveness-prior soft re-rank (both default-on, RRF-SCALE-001-safe).
- **Relevance gate** (`internal/jiminy/outcome_classifier.go`): a precise 4-band classifier so unrelated-domain surfacings are `not_applicable`, near-LOW real ignores are `ignored`.
- **Lever B enabled** (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED`) + exposed in compose + the UI config tab.

Follow-rate lift is forward-looking (baseline 0.165; re-measure ~1 week out). See `docs/development/jiminy-corpus-001/`.


## Follow-up — role_type adapter gap closed (JIMINY-ROLETYPE-ADAPTER-001, 2026-07-17)
The disclosed follow-up ("retrieval adapter drops role_type → retrieval-sourced items typed `learning`") is **shipped and live-verified**. Additive `RoleType`/`ObsType` fields flow from Neo4j `role_type`/`obs_type` through `retrieval.Candidate` → `models.RetrieveResult` → `jiminy.RetrievalResult`; `classifyRetrievalItem` prefers `role_type` before the Layer≥2 concept short-circuit and the `ObsType` switch. Live smoke on `mdemg-dev`: the L1 UxTS constraint node surfaced with `role_type='constraint'`, Jiminy `latest` returned **4/5 items typed `constraint`** (all 5 would have been `learning` pre-fix), and `constraint_outcomes` now carries `guidance_type='constraint'` rows with matched `constraint_code`. The BM25 sink also picked up `role_type` at source (BM25 runs its own Cypher, not a virtual view over `cands`), and the reasoning-module rerank preserves ontology labels through the proto boundary via the existing `originalByID` restore hook. See `docs/development/jiminy-roletype-adapter-001/`.


## Follow-up — L1 correction producer (JIMINY-CORRECTION-PRODUCER-001, 2026-07-20)
JIMINY-ROLETYPE-ADAPTER-001 wired retrieval + classifier to carry `role_type='correction'` end-to-end, but the correction slot in `constraint_outcomes` still read zero because **no L1 correction nodes existed anywhere**. `CreateConstraintNodes` had a producer since inception; the correction side had none — 32 L0 `obs_type='correction'` observations sat in `mdemg-dev` unpromoted.

JIMINY-CORRECTION-PRODUCER-001 mints the missing L1 layer:
- **`internal/hidden/correction_nodes.go::CreateCorrectionNodes`** mirrors the constraint producer, keyed by `obs_type='correction'` (semantic label, 1:1 with the obs — no type-grouping); idempotent via the `IMPLEMENTS_CORRECTION` guard.
- **`internal/hidden/correction_gate.go::CorrectionPromotionGate`** — content-length + config-driven regex deny-set (reuses the JIMINY-CORPUS-001 junk-class defaults). No obs_type deny-set (the predicate already gates on `obs_type='correction'`).
- Wired as `correctionStep` in the consolidation pipeline (phase 20) alongside `constraintStep`.
- **Live Tier-3:** 32 L1 correction nodes minted; `/v1/memory/retrieve` returns `role_type='correction'` on relevant queries; Jiminy `latest` surfaces `type='correction'`; `constraint_outcomes.guidance_type='correction'` gains its first ever row for `mdemg-dev` (`followed`).

New config: `CORRECTION_PROMOTION_ENABLED` (default true), `CORRECTION_PROMOTION_MIN_CONTENT_LEN` (default 20), `CORRECTION_PROMOTION_REJECT_PATTERNS` (JSON regex array; defaults reuse the constraint gate's junk-class set). Rollback tombstone-only via `is_archived=true`. See `docs/development/jiminy-correction-producer-001/`.

## Follow-up — structured correction propagation (JIMINY-STRUCTURED-CORRECTION-001, 2026-07-20)
The `Incorrect`/`Correct`/`Context` triple that `POST /v1/conversation/correct` receives no longer dead-ends in the free-text join. It now flows end-to-end:
- **L0 obs**: `structured_data.correction = {incorrect, correct, context}` — additive to any constraint-detector fields already present.
- **L1 correction node**: `correction_incorrect` / `correction_correct` / `correction_context` as top-level graph properties (populated by `CreateCorrectionNodes` from L0 structured_data).
- **GuidanceItem** (via Lever C fetch): three optional fields carried through to the synthesizer.
- **Lever B synthesis prompt**: renders "Do <correct> — not <incorrect>. (Context: <ctx>)" when structured; falls back to raw `Content` when not. `directiveSynthesisInstruction` preserves both sides of the contrast — never drops the anti-pattern half, since the contrast is what teaches.
- **Backfill**: `mdemg corrections rehydrate-structured --space-id <id> [--dry-run] [--batch-size 100]` walks L0 corrections missing structured, parses the joined content via the template regex, merges (preserves other keys), and propagates to linked L1 via IMPLEMENTS_CORRECTION. Idempotent. WARN-skips unparseable free-form corrections (~97% of mdemg-dev's L0 corrections — they were captured via `/v1/conversation/observe` and never followed the template shape). This is by design: downstream prefers structured when present, falls back to Content otherwise.

**Retired dead code:** the `metadata_*` param flatten in `createObservationNode` was never referenced by the CREATE cypher — no `metadata_incorrect` / `metadata_correct` property has ever landed on any MemoryNode (verified live). Removed with a comment explaining prior intent; if a future feature needs graph-persisted Metadata it must add BOTH the flatten AND matching cypher SET clauses.

See `docs/development/jiminy-structured-correction-001/`.

## Follow-up — Lever C tightening (LEVER-C-TIGHTEN-001, 2026-08-01)

The operator flagged J17 avg trust at 25.4% + Jiminy actionable-compliance at ~14% as SUBSTRATE-QUALITY problems, not calibration artifacts (correction: `[correction]` 2026-08-01). Trust is the receiver-side view of guidance quality; a compliance rate of ~14% means most surfaced actionable guidance is not worth acting on. A 7-agent read-only deep-dive workflow (`wf_e576f7f8-625`) investigated Lever C's role in denominator inflation.

**Data-decided values** (from 7d TSDB analysis on mdemg-dev):
- **`JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK`**: code default **5 → 4** (`.env` **5 → 4**). Live actionable-fraction gauge mean is 0.342 (vs 0.30 quota) — TOPK=5 was over-supplying with headroom; TOPK=4 gives ~2.7 survivors after 32% cooldown/prior attrition, still quota-safe via the shipped cooldown-fallback path.
- **`JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR`**: code default **0.30 → 0.45** (`.env` **0.70 → 0.55**). `constraint_outcomes` distribution over 7d: 257 rows below downstream-sim 0.40, 0 followed (37% pure denominator noise). All 78 followed events sit at sim ≥ 0.50 (77 ≥ 0.60). Surface-time sim runs systematically lower than downstream sim → 0.45 code default kills the noise tail with margin; `.env` relaxes 0.70 → 0.55 to restore Lever C supply headroom (0.70 was starving the quota, forcing over-reliance on cooldown-fallback which re-surfaces recently-ignored items).

**Startup log**: `slog.Info("jiminy: lever c actionable bias", "enabled", "topk", "sim_floor")` emitted at boot when Lever C is enabled — grepable, no hidden state.

**A/B measurement** — passive over 7d:
- Expect: actionable-compliance rate **~14% → ~17-19%** (denominator-shrink mechanism, same shape as JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001).
- Expect: J17 avg trust EMA positive 7d slope (steady state fixed point: 14% follow → 0.312; 30% follow → 0.44).
- Actionable volume/day: expect **~101 → ~75-80**, ~20-25% cut.

**Revert tripwires** (if any tripped, revert single commit):
- Actionable-compliance rate **falls** below 0.10
- `mdemg_jiminy_surfaced_actionable_fraction` **falls below 0.20** for 6h continuously (quota starving)
- Surface-cooldown fallback rate > 30% of surfaces (TOPK too tight)

**Load-bearing downstream — watch-item**: `GetConstraintEffectiveness` (RSIC per-constraint prune signal, `ape/task_dispatch.go:736,764`) accumulates ~20% slower per constraint (from ~100 GUIDANCE_OUTCOME edges/day/space). Not blocking — the signal isn't lost, just slower. If RSIC prune velocity measurably degrades over 30d, follow-up sprint bumps `RSIC_GUIDANCE_MIN_SURFACES` or similar.

⚠️ **Architectural rule pinned**: when a config knob has a "shrink the denominator" shape (Lever C top-K + sim floor), the load-bearing downstream isn't the surface volume itself but the signal-per-time density feeding secondary consumers. Enumerate consumers of `constraint_outcomes` / `GUIDANCE_OUTCOME` edges BEFORE tightening; disclose slower-accumulation risk even when it's not blocking.

⚠️ **Also pinned**: trust score is a receiver-side quality signal, NOT an observability-only gauge. Even when `J17_TIER_GATE_MODE=comprehension` (J17-TIER-GATE-001) bypasses trust for tier selection, a low trust EMA still means the substrate is producing untrustworthy guidance. Don't frame low trust as "harmless-by-design." The operator directive from 2026-08-01 stands: raise the trust signal, don't calibrate away the alarming number.

See `docs/development/lever-c-tighten-001/`.

## Follow-up — Mention-vs-Perform (JIMINY-CLASSIFIER-META-SCOPE-001, 2026-08-14)

Phase 3.5 arc-adjacent to JIMINY-CEILING-BREAK-2. Extends the shipped CONTEXT-002 `mechanismScopeCreditClause` with a mention-vs-perform disambiguation clause: when the action-text contains the constraint's mechanism-verb ONLY as prose CONTENT (a doc edit, a sprint plan describing an approach, a CLAUDE.md pin quoting the rule) rather than as an EXECUTED mechanism (Bash invocation, runtime code call, executed migration), route to `not_applicable` UNCONDITIONALLY.

**Motivation.** CONTEXT-002's mechanism-scope gate treats "action-text contains mechanism-verb" as "maybe the action performs it → proceed to determine follow/ignore." Operator observation post-Phase-3: the LLM systematically applies "contains verb → assume performing" → routes legitimate documentation edits to `ignored`. Adding a mention-vs-perform disambiguation should catch this class.

**Live smoke result (2026-08-14 initial, 2 runs against local mdemg-llm-v1).** The clause SHIPS as DORMANT capability. `.env` flag `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED` STAYS at code default `false`. Reasoning:
- CONTEXT-002's shipped 3-credit baseline (non-violation + context-mismatch + mechanism-scope) already routes most fixtures to defensible verdicts — the mention-vs-perform clause showed no reliable marginal delta across two runs against the same 6-fixture set.
- Two runs against identical inputs produced inconsistent LLM verdicts on the ambiguous doc-edit cases (F1-F3) — this is LLM variance on marginal cases, not a defect in the clause.
- Counter-fixtures (F4 authored a mermaid diagram; F6 ran a real `git commit`) stayed stable in both runs — the safety envelope is intact. 0 regressions on either run.
- Unambiguous mention-only cases (F1 pin quoting rule; F5 prose quoting rule) DO flip cleanly (`ignored`→`not_applicable`) when the LLM's variance lands on the mention interpretation.

**Ship-dormant decision.** Rather than force a fixture-based lift that isn't there, the code + tests + docs + config knob ship as dormant capability. The 6 unit pin tests regression-lock the wiring at build time. The `.env` flag flip is deferred to a future measurement window — specifically, if the JIMINY-CEILING-BREAK-2 T+168h passive re-check on 2026-08-19 shows CONTEXT-002 alone underdelivers on the actionable-follow-rate target.

**Byte-identical default-off.** The `resolveClassifySystemPrompt()` render is byte-identical to the CONTEXT-002 output when `MentionVsPerformCredit=false`. ULTS-CI-001 `system_prompt_hash` pin for the flag-off path is preserved; no ULTS hash bump required to ship dormant.

**Composition.** With all four credit flags ON, the render order is exactly: base → non-violation → context-mismatch → mechanism-scope → mention-vs-perform (strongest-gate-last; recency-weighted LLM attention). Pin `TestResolveClassifySystemPrompt_AllFourCredits_Ordering` locks this.

⚠️ **Architectural rule pinned**: a live-smoke that reveals "no measurable delta" is a legitimate ship-dormant outcome, not a failure. Mirror NEURAL-RERANK-QUALITY-AB-001's data-decided no-op verdict. Ship the code as regression-insured capability; leave the flag off until evidence justifies flipping it. Do not force a fixture set to produce a lift the LLM isn't organically providing.

⚠️ **Also pinned**: LLM-fixture-based smokes on ambiguous prompt-tuning changes are HIGH-VARIANCE. Two runs against identical inputs can produce different verdicts. When a smoke gates a decision, either (a) accept 3+ runs of the same fixture and report the modal verdict, or (b) accept "informational smoke" mode where variance-dependent outcomes don't fail the build but regressions on counter-fixtures do (the shape used here — `regressions>0` fails, `flips` informational only).

See `docs/development/jiminy-classifier-meta-scope-001/`.
