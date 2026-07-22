# SCORE-RETRIEVAL-REAL-SIGNALS-001 — Sprint Post

**Shipped:** 2026-07-21 | RSIC Retrieval dimension now reads real pipeline signals (DASHBOARD-TRUTH-002 E1 full fix).

- E1: primary path = mean per-stage fill rates from `report.RetrievalDataset`; conf = min(1, TotalQueries/50) (DH-005-native, knob-free); nil/zero-data → maturity-enum fallback (byte-identical; existing tests unmodified-green). 3 new tests.
- Live-smoke-caught ordering bug: scoring ran before the dataset fetch — recompute wired after the fetch (own fix-commit).
- E2 live: gauge 0.9000 (enum) → **0.9600** (real signal) on the first post-fix assess.
- Rollback: revert commits (fallback = prior behavior).
