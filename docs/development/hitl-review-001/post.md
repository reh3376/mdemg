# HITL-REVIEW-001 — Sprint Post

**Date:** 2026-06-24 · branch `reh3376_dev01` · v0.11.0 (coordinated pair with
JIMINY-RELEVANCE-001). A general-purpose human-in-the-loop review +
live-reinforcement platform; the guidance corpus + all 16 LLM call sites are its
first tenants.

## What shipped (Epics 1–7)

| Epic | Deliverable | Verification |
|---|---|---|
| **1** registry + persistence | `internal/review` (ReviewableDataset/Registry/Sink/Rubric) + `review_grades` (V0028, schema 27→28) + buffered writer with idempotency/reversal reads | unit ✓ |
| **2** rubric engine | versioned anchored rubric (Rated mean/4 + Ranked DPO); `rubric_v1.md` | unit ✓ |
| **3** sampler | active (uncertain/disagreeing) + stratified, deterministic-by-seed | unit ✓ |
| **4** endpoints | `/v1/review/{datasets,next,grade,reverse}`, admin-gated, idempotent, dry-run | 6 handler tests ✓ |
| **5** sinks (headline) | `GuidanceSink` → live trust EMA + node confidence, **reversible**; NoopSink | **live: trust 0.297→0.367→restored, confidence 0.41→0.46→0.41; idempotency 409** |
| **6** UI + datasets | functional Review tab (vanilla JS) + Playwright; SME enrichment; corrective capture (V0029); the 16 LLM datasets; stub removal | **Playwright 5/5; operator-driven live tests** |
| **7** docs | feature doc + rubric_v1 + CHANGELOG + post + CLAUDE.md | — |

## The arc this sprint proves
The platform was built generic (Epics 1–5) and then **proven generic** by
registering 17 datasets with zero platform changes: the Guidance Corpus
(live-reinforcing) + all 16 MDEMG LLM call sites (gold-only review of
`llm_interactions`). The headline — a human grade reinforcing the live substrate
**reversibly** — was verified end-to-end on the real system.

## Shaped by live SME testing (the high-value part)
Three improvements came directly from the operator driving the UI, not the plan:
1. **SME context enrichment** — the bare item view wasn't enough to judge an
   outcome; added the auto-verdict (+ confidence + how-labeled) + provenance
   panels.
2. **Corrective capture** — the SME asked "what would *better* guidance be?", so a
   "Suggested better guidance" field (`review_grades.suggested_guidance`, V0029)
   now turns each grade into a gold actionable-guidance example — the highest-value
   retrain signal.
3. **16 LLM datasets + ⓘ descriptions** — the operator asked for every LLM call
   site to be reviewable; the generic interface made it a catalog + one dataset
   type. Operator then graded 2 real Guardrail-Evaluate outputs through the
   browser → persisted to `review_grades` with correct rubric math (0.667, 0.833).
4. **Stub removal** — the self-test stub shouldn't appear in a prod UI → moved to
   test-only.

## Decisions disclosed
- `GuidanceSink` ships trust + confidence (both cleanly reversible); the
  `GUIDANCE_OUTCOME` edge sink (risky create-vs-delete) is `hitl-review-002`.
- Guidance items source from `guidance_training_rows`; LLM items from
  `llm_interactions` (`trace_id` as item_id).
- Reinforcement opt-in per submission; idempotency enforced at the endpoint.
- Migrations 028 + 029 (027 was JIMINY-RELEVANCE-001's — the coordinated-pair
  rebase); schema 29.

## Carried forward
- **`hitl-review-002`**: GUIDANCE_OUTCOME edge sink; SFT/DPO sinks; multi-grader
  consensus.
- **`dashboard-fixes-001`** (logged this sprint): the pre-existing dashboard bugs
  the UI audit found (backup field-mismatches + Restore, learning freeze-state,
  rsic badge, plugins/memory) — a focused follow-up, out of this sprint's scope.
- The `suggested_guidance` corpus feeds `jiminy-actionability-001` + the
  recursive-retraining loop.

## Documents Accessed
The JIMINY-RELEVANCE-001 diagnostic + plan; `internal/jiminy/service.go`
(trust/confidence accessors); `internal/tsdb/*writer*`, migrations, `llm_interactions`
schema; `internal/api/ui/*` (vanilla-JS tab pattern); `tests/e2e/browser-ui/`;
the live `:9999` review surface + `review_grades`.
