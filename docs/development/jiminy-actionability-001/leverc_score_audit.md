# Lever C — RRF-SCALE-001 Score Audit

Mandatory per the sprint plan: Lever C touches the guidance candidate pool, which is RRF-SCALE-001 territory. This audits every score/confidence comparison Lever C introduces or touches.

## RRF-SCALE-001 rule (recap)
Downstream consumers MUST NOT hardcode absolute thresholds against `RetrieveResult.Score` (the RRF scale is not a stable contract). Gate via config with RRF-calibrated defaults, or via a **scale-invariant** signal. When changing the scorer, re-audit every `.Score`/`.Activation` comparison.

## What Lever C adds

Lever C is a **separate Neo4j vector query** (`fetchActionableCandidates`), not a change to the RRF retrieval/scorer path. It introduces exactly one numeric comparison:

| Comparison | Scale | Source | Verdict |
|---|---|---|---|
| `sim >= $simFloor` (Cypher) | **vector-index cosine [0,1]** (`db.index.vector.queryNodes` `score`) | `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` config (default 0.30) | ✅ **Safe** — cosine is a stable absolute scale (identical=1, orthogonal≈0), NOT the RRF `Score`. Threshold is config-driven, not a code literal. Mirrors the existing `matchConstraintCodeByEmbedding` (JIMINY-OUTCOME-001) pattern. |

- **No `RetrieveResult.Score` comparison** is added — the merged candidates never carry an RRF score; they come from the vector index, not the RRF aggregator.
- **No hardcoded score literal** — the only threshold is `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` (config, RRF-calibration-irrelevant because it's cosine, not RRF).
- The vector-index scan window (`actionableScanWindow = 50`) is an internal impl constant (mirrors the existing constraint-code matcher), not an operator score threshold.

## What Lever C touches downstream

Merged `GuidanceItem`s carry `Confidence = sim` (cosine [0,1], clamped to `maxConfidence` 0.95). They flow into:

| Downstream consumer | Field used | Verdict |
|---|---|---|
| `guidanceSortKey` (Lever A sort) | `Confidence` | ✅ Safe — operates on `Confidence` ([0,1] confidence semantics), not raw `Score`. Merged items' cosine-confidence and retrieval items' sigmoid-of-RRF confidence are both [0,1] confidence values — comparable for *ranking*; neither is a raw RRF score. |
| `JiminyMinConfidence` gate (0.3) | `Confidence` | ✅ Safe — a config gate on the [0,1] confidence; a cosine-sim ≥0.3 actionable item clearing it is correct (already-relevant constraint). |
| `applyActionableComposition` (quota/cap) | `Type` partition | ✅ Safe — partitions by `Type` (role), no score. |
| consulting `CONSULTING_*_SCORE_FLOOR` gates | — | ✅ **Untouched** — Lever C does not route through the consulting path; those RRF-calibrated gates are unchanged. |

## Note on the cross-derivation
Merged items derive `Confidence` from cosine sim; retrieval items derive it from `NormalizeScore(RRF Score, sigmoid)`. Both are [0,1] **confidence** values (not raw scores), so blending them in the sort is sound. This is disclosed, not a scale break — neither path compares an RRF `Score` against an absolute literal.

## Conclusion
**Clean.** Lever C is scale-invariant (cosine-gated, config-driven, no RRF `Score` comparison, no hardcoded literal) and does not regress any RRF-SCALE-001-calibrated gate. Default-off ships safe.
