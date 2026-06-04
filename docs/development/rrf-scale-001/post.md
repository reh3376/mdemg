# Sprint RRF-SCALE-001 — Post

**Closed:** 2026-06-03
**Branch:** `reh3376_dev01`
**Plan:** [`sprint_plan_rrf_scale_001.md`](sprint_plan_rrf_scale_001.md)
**Audit:** [`audit_findings.md`](audit_findings.md)
**Verification:** [`verification.md`](verification.md)

## Outcome

The Jiminy guidance→feedback→outcome loop — silently dormant ~9 weeks — is revived for its primary path. Root cause was the **third instance of one bug class**: downstream consumers hardcoding the retrieval-score contract that Phase 13.1 RRF default-on silently changed. The `consulting/service.go` `0.55` constraint gate (and 6 sibling gates + the confidence sigmoid) were calibrated for the legacy linear scorer; RRF strong matches top out ~0.49–0.58, so the gates rejected everything → empty guidance → dead loop.

Fixed by making the gates + sigmoid config-driven and RRF-calibrated. Live-verified: guidance items 0→10, constraints 0→2, and the TSDB `constraint_outcomes` sink (dead since May 1) writing fresh rows dated today.

## Epic-by-epic

| Epic | Status | Commit | Notes |
|---|---|---|---|
| 0 — Sprint plan | ✅ | (plan) | 12-section; P0 correctness. |
| 1 — Audit sweep | ✅ | (audit) | 12 sites cataloged. Live distribution captured (strong matches 0.49–0.58; 0.55 gate sits mid-band). Percentile (Option A) ruled out — positional rank. |
| 2 — Fix gates + sigmoid | ✅ | (consulting fix) | 7 gates + sigmoid (both copies) → 5 config knobs, RRF-calibrated defaults (0.45 / 0.45 / 0.45 / 0.45 / 8.0). Discovered the inner authority gate binds tighter than the outer constraint gate → all floors default 0.45. 4 new/updated Tier 1 tests. |
| 3 — Remaining findings | ✅ | (audit Epic-3) | 3 Low Activation *display* gates traced + intentionally left with rationale (explainability path, always-additive at RRF scale, no guidance gating). |
| 4 — Integration + live e2e | ✅ | (verification) | Live: guide 0→10 items, constraints 0→2; TSDB outcome sink revived (fresh rows today). Tier 2: 2 integration tests. |
| 5 — Docs + close | ✅ | (this) | CHANGELOG, CLAUDE.md score-scale contract, post.md. |

## Acceptance criteria (from plan)

1. ✅ `/v1/jiminy/guide` with a constraint-matching context returns non-empty guidance with `source_counts.constraints > 0` (live: 10 items, 2 constraints).
2. ⚠️ **Partial.** TSDB `constraint_outcomes` sink revived with a fresh row dated today (✅). The Neo4j `GUIDANCE_OUTCOME` edge did **not** appear — a *distinct* root cause (node-type targeting; retrieval surfaces `emergent_concept` abstractions, not raw constraint nodes) outside the score-scale mandate. Documented as Follow-up A, not silently dropped.
3. ✅ Audit findings doc catalogs every RRF-scale consumer; all High/Med remediated, all Low decided.
4. ✅ Every new threshold config-driven with documented default; no new hardcoded score constant in the fixed paths.
5. ✅ Irrelevant contexts don't flood (Tier 2 `SuggestRejectsNoise`).
6. ✅ `CLAUDE.md` records the score-scale contract.

## Honest scope note

The sprint's mandate — the **RRF score-scale consumer bug** — is fixed, live-verified, and structurally defended (CLAUDE.md contract). The score-gate fix is the correct and *sufficient* remedy for the score-scale root cause: it revived guidance surfacing and the TSDB outcome sink.

Live smoke then surfaced three *adjacent* issues with **distinct root causes outside the score-scale mandate**. Per the discovery discipline (trace the whole pipeline; don't overclaim; surprise bugs get honest documentation), they are characterized, not bolted on:

- **Follow-up A — Neo4j `GUIDANCE_OUTCOME` edge sink dormant.** Retrieval surfaces `emergent_concept` (L2–L5) abstractions of constraints, not the raw `role_type='constraint'` nodes. `PersistGuidanceOutcome` only writes an edge when the target is `obs_type IN [constraint,correction,pattern,learning] OR role_type='constraint'`; `emergent_concept` fails that filter (the TSDB writer has no such filter, so it revived). This is a node-type-targeting + retrieval-ranking issue independent of the scorer — it would occur under the legacy scorer too. It needs an architecture decision (should concepts be valid outcome targets? should retrieval surface raw constraints alongside concepts? should concepts resolve to underlying constraints?), so it warrants its own sprint — **JIMINY-OUTCOME-001** — not a rushed bolt-on.
- **Follow-up B — LLM guidance synthesis timeout.** `synthesis_error: context deadline exceeded` now that synthesis actually runs (it never ran while guidance was empty). Doesn't block the loop (synthesis only composes the optional `prompt_augmentation`). Timeout-tuning flavor à la DH-004.
- **Follow-up C — `/v1/jiminy/latest` JSON control-char escaping.** The response carries unescaped control characters that break strict JSON parsers. `prompt-context.sh` parses it with `jq` — so this may *also* impair the real hook's `guidance_id` capture and compound the dormancy. Low-effort; verify the hook path.

## Surprises / discipline notes

- **Cold-start LLM-classifier timeout masked the fix on the first call.** Immediately post-restart, `/v1/jiminy/guide` returned `constraints:0` because the LLM constraint classifier hit a cold-model deadline and fell back to keyword classification (which doesn't match concept summaries). One warm-up call fixed it. A model-warmth artifact, not a fix defect — but it's why "trace the whole pipeline / don't stop at the first result" mattered: the first call looked like the fix had failed.
- **The inner authority gate.** `keywordClassifyConstraint`'s internal `Score > authorityFloor` binds *tighter* than the outer constraint gate; had I left authority at 0.50 it would have re-rejected the strong-match band and the loop would have stayed dormant despite the outer fix. Caught by the failing Tier 1 boundary test.

## Forward-looking

- **JIMINY-OUTCOME-001** (Follow-up A) — the highest-value next step: revive the Neo4j `GUIDANCE_OUTCOME` edge sink so per-constraint effectiveness graph stats update again. Requires the architecture decision above.
- Follow-ups B (synthesis timeout) and C (latest JSON escaping) — smaller, can be folded into a maintenance pass.
- Re-audit every `RetrieveResult.Score`/`.Activation` comparison on any future scorer change (the CLAUDE.md contract is the reminder).

## Documents Accessed
- `internal/consulting/service.go`, `service_test.go` — gates, sigmoid, `findApplicableConstraints`, `keywordClassifyConstraint`
- `internal/jiminy/{service.go,retrieval_source.go,persistence.go,types.go}` — Guide, mapRetrievalToGuidance, PersistGuidanceOutcome, feedback
- `internal/api/handlers_jiminy.go`, `server.go` — guide/warm/latest/feedback handlers, writer wiring
- `internal/config/config.go` — new `CONSULTING_*` / `RETRIEVAL_CONFIDENCE_SIGMOID_*` knobs
- `internal/mathutil/normalize.go` — Sigmoid/NormalizeScore
- `internal/retrieval/scoring.go` — ApplyNormalizedConfidence (percentile signal)
- `internal/tsdb/{constraint_outcomes_writer.go,backfill.go}`, `migrations/011_constraint_outcomes.sql`
- `.claude/hooks/{prompt-context.sh,post-tool-observe.py}` — the warm + guidance_id + feedback hook chain
- Live diagnostics: retrieve score distribution, `/v1/jiminy/guide` debug, `llm_interactions`, `constraint_outcomes`, Neo4j `GUIDANCE_OUTCOME` + node role_types
