# SCORE-RETRIEVAL-REAL-SIGNALS-001 — E2 Live Verification

**Date:** 2026-07-21

## Live-smoke-caught ordering bug (own fix-commit)
Forced assess after the E1 deploy returned the ENUM value with a nil dataset — `Assess()` scores in section 5 (line 156) but fetches TSDB datasets in section 5f (~line 228). E1's primary path never fired. Fixed: recompute `RetrievalQuality/Confidence` immediately after `RetrievalDataset` lands (gauge publish happens after 5f, so the published value carries the real signal).

## Post-fix gauge
```
mdemg_rsic_health_retrieval
  0.9600  2026-07-21 19:28:57  ← real signal: mean(recall≈1.0, bm25≈1.0, rerank≈0.88), conf 1.0 (279+ events)
  0.9000  2026-07-21 19:27:48  ← boot-time assess (dataset not yet fetched → enum fallback, as designed)
```
The dimension now RESPONDS to real degradation: a broken pipeline stage drops it; the fails-open rerank dips it mildly (~0.96 with rerank at 88%); zero-traffic spaces keep the maturity-prior fallback with weak confidence.
