# Sprint SCORE-RETRIEVAL-REAL-SIGNALS-001 — RSIC Retrieval dimension reads real pipeline signals

## 1. Header & Metadata
| Field | Value |
|---|---|
| Sprint ID | SCORE-RETRIEVAL-REAL-SIGNALS-001 |
| Owner | Roger Henley | Branch | `reh3376_dev01` |
| Format | v1.0 (12-section) | Effort | ~0.25 dev-day |
| Parent | DASHBOARD-TRUTH-002 E1 deferred full fix ("swap the enum-table lookup for real signals") |

## 2. Problem Statement
`scoreRetrieval` is still a LearningPhase enum lookup (cold 0.3 / learning 0.6 / warm 0.9 / saturated 0.9) — a maturity gauge that can NEVER detect real retrieval degradation. Meanwhile the assessment already collects `report.RetrievalDataset` (`tsdb.RetrievalQualitySummary`: per-stage fill rates + TotalQueries over the 24h window, from `retrieval_events`) — and ignores it.

## 3. Scope & Constraints
**In**: score = mean(RecallRate, BM25Rate, RerankRate) when `RetrievalDataset` has data; confidence = min(1, TotalQueries/50) (new `confidenceThresholdRetrievalEvents=50` const, DH-005 family); zero-data/nil → legacy enum fallback (cold spaces keep the maturity prior). Knob-free by design: DH-005's confidence weighting naturally down-weights noisy small-N windows. Tests + live verify + docs.
**Out**: UVTS-mean bonus signal (sparse/stale-prone — documented future); latency-SLO blending (alert rules already cover it).
**Constraints**: fallback path byte-identical to the DASHBOARD-TRUTH-002 E1 enum (existing tests must stay green unmodified); RRF-SCALE-001-safe (fill rates are [0,1] structural signals, not scores).

## 4. Dependencies
✅ `RetrievalDataset` collected at self_assess.go:227 before scoring; ✅ `RetrievalQualitySummary` fields; ✅ DH-005 confidence-exclusion formula.

## 5. Implementation Plan
E0 plan → E1 rewrite + tests → E2 live verify (restart → assess → gauge moves 0.900 → ~0.96 and now tracks real fill rates) → E3 docs.

## 6. Testing Plan
T1: table-driven (dataset present → rate-mean + proportional conf; nil/zero → enum fallback pins). T2: full ape suite (existing enum tests green UNMODIFIED — they pass no dataset). T3: live assess + gauge read.

## 7-12 (Commit / Verification / Docs / Risks / Rollback / Documents Accessed)
Commits: docs(E0) → feat(E1) → docs(E2+E3). Verify: build/lint/tests + live gauge. Docs: CHANGELOG, CLAUDE.md E1-note closure, post. Risk: small-N noise → mitigated by proportional confidence (DH-005 weighting); benchmark-saturation windows depress rerank fill → score honestly dips (that IS the signal; fails-open keeps it mild: live 0.878 → score ~0.96). Rollback: revert commit (fallback = current behavior). Accessed: self_assess.go, dataset_builder.go:242, types_rsic.go, DH-005 feature doc, DASHBOARD-TRUTH-002 E1.
