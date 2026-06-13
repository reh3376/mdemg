# CONTEXT-LIVE-001 — UVTS 120q A/B Gate Analysis

Baseline: main @ d3f7920 content (pre-sprint binary). Candidate: all 4
epics. Same live stack, sequential runs, both cold-cache (scorer
namespace v1→v2 flip — which also live-fired the TSDB-CONSUME-001
scorer_version_change tripwire, as designed).

## Strict verdict: FAIL — root-caused to non-retrieval noise

| metric | value |
|---|---|
| baseline mean | 0.4070 |
| candidate mean | 0.4030 (Δ −0.0040) |
| per-question regressions > 0.10 | **0** |
| improvements | 4 |

Forensic: 13/120 questions changed score; **every one** is exactly
±0.100 and **only** in `citation_bonus` (9 lost, 4 gained) — the
stochastic did-the-LLM-emit-citations component of answer synthesis.
The retrieval-quality components (`evidence`, `semantic`, `concept`)
are **bit-identical on all 120 questions** (0 changes).

## Why parity is the expected correct result

- lnl-demo-whk has NO ContextCatalog → query-fingerprint derivation
  returns nil → the context column is not appended → RRF ranking
  unchanged by construction.
- UVTS passes explicit `?category=` → explicit-wins, dispatch inert.
- Version guard / consensus changes don't alter ranking (consensus is
  an output signal; guard only affects spaces WITH fingerprints).

## Disposition

The Note 02 gate's purpose is "no retrieval regression". Demonstrated:
zero retrieval-component changes, zero per-question regressions; the
−0.004 mean is binomial noise in a generation-stochastic bonus. Treated
as PASS-BY-ANALYSIS with full disclosure; the strict verdict artifact
is committed unmodified (uvts_ab_verdict.json). Operator merge gate is
the final arbiter.
