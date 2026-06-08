# Sprint JIMINY-OUTCOME-001 — Post

**Closed:** 2026-06-08
**Branch:** `reh3376_dev01`
**Plan:** [`sprint_plan_jiminy_outcome_001.md`](sprint_plan_jiminy_outcome_001.md)
**Verification:** [`verification.md`](verification.md)

## Outcome

The Neo4j `GUIDANCE_OUTCOME` edge sink — dormant since 2026-04-12 — is revived, with edges landing on the **correct** `role_type='constraint'` nodes. Combined with RRF-SCALE-001 (which revived the TSDB `constraint_outcomes` sink), **the guidance→feedback→outcome loop is now fully restored across both sinks**, and per-constraint effectiveness graph stats (`GetConstraintEffectiveness`) update again.

## Root cause (Follow-up A from RRF-SCALE-001)

`matchConstraintCode` linked guidance items to constraint codes by keyword overlap (≥3 shared words). Retrieval surfaces `emergent_concept` abstractions whose content rarely shares 3+ literal words with raw constraint text → no `constraint_code` → `PersistGuidanceOutcome` fell back to the concept SourceNode → its `role_type='constraint'` filter rejected `emergent_concept` → no edge. Distinct from the RRF score scale; it would occur under the legacy scorer too.

## Fix (Option 1)

`matchConstraintCodeByEmbedding` queries the constraint vector index (`db.index.vector.queryNodes`, `role_type='constraint'`, `sim ≥ threshold`) and returns the closest constraint's code. `Guide()` tries embedding match first, keyword fallback second — never regresses. The existing `PersistGuidanceOutcome` + `findConstraintNodeID` machinery (unchanged) then create the edge on the real constraint node.

**Implementation refinement vs the plan:** the plan proposed loading constraint embeddings into Go and computing cosine in a loop (extending `constraintCodeEntry`). The implementation instead uses Neo4j's **server-side vector index** (mirroring `Evaluator.findMatchingConstraints`) — cleaner, no in-Go embedding load, no `constraintCodeEntry.Embedding` field. Same Option-1 outcome. Disclosed in the Epic 1 commit.

## Epic-by-epic

| Epic | Status | Notes |
|---|---|---|
| 0 — Sprint plan | ✅ | 12-section; P1; grounded fork (4 options) with Option 1 selected. |
| 1 — Embedding matcher | ✅ | `matchConstraintCodeByEmbedding` + `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` (0.55) + keyword fallback. 4 Tier 1 tests. Vector-index approach (server-side) refined from the plan's in-Go cosine. |
| 2 — Integration + live e2e | ✅ | Tier 3: 0→6 coded items, 6 fresh edges on real constraint nodes, effectiveness 0.93. Tier 2: passes on idle stack, skip-tolerant under LLM contention. |
| 3 — Docs + close | ✅ | CHANGELOG, CLAUDE.md note, post.md. |

## Acceptance criteria — all met

1. ✅ Fresh Neo4j `GUIDANCE_OUTCOME` edge on a real `role_type='constraint'` node, dated today (6 edges).
2. ✅ Matched `constraint_code` (`no-direct-main-commits`) semantically correct.
3. ✅ `/v1/constraints/effectiveness` reflects it (rate 0.93 on the matched constraint).
4. ✅ Threshold config-driven with a documented, live-validated default.
5. ✅ Keyword fallback intact; degrades gracefully without an embedder.
6. ✅ Both sinks revived — the constraint-effectiveness loop is fully restored.

## Discipline notes

- **LLM serialization is the test-flakiness source.** The guide path runs the constraint classifier per node (serialized LLM calls, ~31s/guide call). A guide call fired while the LLM is busy fast-fails with empty guidance. The Tier 2 test warm-retries and **skips** (never false-fails) when items can't be produced; it passes on an idle stack. The Tier 3 live e2e is the definitive proof. This LLM behavior is RRF-SCALE-001 Follow-up B, still open.
- **The vector-index precedent saved work.** `Evaluator.findMatchingConstraints` already did exactly the embed→vector-query→constraint pattern; reusing it made the fix small and proven rather than novel.

## Forward-looking

- **Follow-up B (LLM synthesis timeout / serialization)** — now the most operationally-visible remaining issue: guide calls are slow (~31s) and fast-fail under contention, and `synthesis_error: context deadline exceeded` recurs. Candidate next sprint — tune the per-node classifier (batch? cache? raise the deadline? parallelize?) so guidance is reliably produced under load.
- **Follow-up C (`/v1/jiminy/latest` JSON control-char escaping)** — small; verify whether it impairs the real `prompt-context.sh` hook's `jq` parse of `guidance_id` (if so, it compounds dormancy).
- **13/111 constraints lack embeddings** — they fall back to keyword matching; `mdemg embeddings backfill --space-id mdemg-dev` is the operator remedy to give them embedding coverage.

## Documents Accessed
- `internal/jiminy/service.go` — `Guide` (constraint-code loop), `matchConstraintCode`, `loadSpaceConstraintCodes`, `constraintCodeEntry`, new `matchConstraintCodeByEmbedding`, `embedder` field
- `internal/jiminy/evaluator.go` — `findMatchingConstraints` (the reused vector-index pattern), `asStringVal`/`asFloat64Val`
- `internal/jiminy/persistence.go` — `PersistGuidanceOutcome`, `findConstraintNodeID` (the working downstream)
- `internal/jiminy/outcome_classifier.go` — `cosineSimilarity` + embed-match precedent
- `internal/config/config.go` — `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD`, `VectorIndexName`
- Live diagnostics: 111 coded constraints (98 with embeddings); guidance items 0→6 coded; Neo4j edges 893→899 on `role_type='constraint'`; effectiveness rate 0.93
- RRF-SCALE-001 `verification.md` + `post.md` (Follow-up A definition)
