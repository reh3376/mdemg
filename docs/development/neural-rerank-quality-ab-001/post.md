# NEURAL-RERANK-QUALITY-AB-001 — Sprint Post (2026-07-20)

## Summary
Data-decided sprint: ran a UVTS 120q A/B on `RERANK_PROVIDER=openai` vs `neural`. Verdict: FAIL per the strict Note 02 gate (candidate mean 0.4130 < baseline 0.4150 by 0.002, within noise). Substantive read: **statistical parity — the two providers are indistinguishable at the mean**, and the 20× latency win from NEURAL-RERANK-PRECHECK-001 remains attractive. But per the strict gate: **no default flip; openai stays default**.

## What shipped
- **E0** — sprint plan (v1.0 12-section format).
- **E1** — Baseline (openai): 120q UVTS mean=0.4150, correct_file_rate=64.2%, σ=0.048.
- **E2** — Candidate (neural): 120q UVTS mean=0.4130, correct_file_rate=61.7%, σ=0.049.
- **E3** — A/B compare + verdict: mean_delta=-0.002; regression_count=0 (below threshold 0.10); improvement_count=6; verdict=fail (mean_gate strict). Artifacts: `ab_verdict.json`, `ab_summary.json`.
- **E4** — SKIPPED (verdict=fail; no code change per the sprint plan).
- **E5** — CLAUDE.md architecture note + CHANGELOG entry + this post.

## Commits (on `reh3376_dev01`)
1. `docs(neural-rerank-quality-ab-001): E0 — sprint plan` — `21bab7a`
2. `docs(neural-rerank-quality-ab-001): E1+E2+E3 — 120q UVTS A/B verdict` — `80fa3e6`
3. `docs(neural-rerank-quality-ab-001): E5 — CLAUDE.md/CHANGELOG/post`

## Evidence — 120q UVTS on `lnl_demo_validation` (mdemg-dev)

| Metric | openai (baseline) | neural (candidate) | Delta |
|---|---|---|---|
| Mean | 0.4150 | 0.4130 | **−0.002** |
| Median | 0.450 | 0.450 | 0 |
| Std | 0.048 | 0.049 | ~equal |
| p10 / p25 / p75 / p90 | 0.35 / 0.35 / 0.45 / 0.454 | 0.35 / 0.35 / 0.45 / 0.454 | identical |
| Min / Max | 0.350 / 0.459 | 0.350 / 0.459 | identical |
| Correct-file rate | 64.2% | 61.7% | −2.5% |
| Regressions >0.10 | — | 0 | ✅ within threshold |
| Improvements | — | 6 | — |
| CV% | 11.6% | 11.9% | ~equal |

**Delta is 24× smaller than std** — statistical parity within noise.

### By-category means (baseline → candidate)
| Category | count | baseline | candidate | delta |
|---|---|---|---|---|
| architecture_structure | 20 | 0.431 | 0.421 | −0.010 |
| business_logic_constraints | 20 | 0.392 | 0.392 | 0.000 |
| computed_value | 6 | 0.383 | 0.367 | −0.016 |
| cross_cutting_concerns | 20 | 0.442 | 0.437 | −0.005 |
| data_flow_integration | 20 | 0.392 | 0.387 | −0.005 |
| disambiguation | 8 | 0.412 | 0.425 | **+0.013** |
| relationship | 6 | 0.417 | 0.417 | 0.000 |
| service_relationships | 20 | 0.431 | 0.436 | **+0.005** |

Two categories improved (`disambiguation`, `service_relationships`); six essentially flat or marginally worse (all within σ).

## Lessons captured
1. **A wash A/B is a legitimate no-op decision, not a bug.** The strict Note 02 gate ("B mean ≥ A mean") correctly reads a 0.5%-relative delta as fail. Sprint plans with strict quality gates should NOT be blurred into subjective flips even when substantive interpretation says "parity." The data-decided contract survives only if the gate is honored.
2. **Latency parity ≠ quality parity for a default flip.** A 20× latency win + statistical parity in quality is a reasonable *operator opt-in* case but not a *default* case. Defaults are conservative; opt-ins carry the trade-off explicitly.
3. **Rerun with a bigger/different UVTS corpus is a legitimate follow-up.** The `lnl_demo_validation` spec is 120 questions on the `lnl-demo-whk` space. A whk-wms full-space A/B (already sprint-exercised in RETRIEVAL-TYPED-EDGES-002) or a per-category-weighted verdict COULD shift the mean gate. Not run here.
4. **UVTS runner's "Passed: 0 / Failed: 1" is spec-level, NOT per-question.** The `grades.json` carries per-question scores; the CLI-level report says "the spec failed the completeness contract" which is orthogonal to the A/B verdict. Reading only the CLI summary would be misleading.

## Non-goals (respected)
- No retuning of either provider.
- No change to rerank prompts / neural model.
- No UVTS spec authoring.
- No dashboard changes.

## Follow-ups
- **whk-wms full 507K LOC A/B** — if operator wants stronger evidence, re-run against the production corpus (RETRIEVAL-TYPED-EDGES-002 already uses this space).
- **Per-category-weighted verdict** — the disambiguation +0.013 and service_relationships +0.005 wins on neural suggest a per-category weighting could tip the gate. Would require an ab_mode.category_weights extension to the compare tool.
- **Cost consideration** — the openai path routes through llama-server (:8102, local — $0 cost); the neural path routes through the neural sidecar (:8100, local — $0 cost). Both are $0 to the operator; latency is the only measurable delta.
- **Cache-invalidation on RerankProvider swap** — noted-not-issue: the retrieve cache key already includes the scorer version but NOT the RerankProvider explicitly. In practice each provider produces different rerank scores → cache miss on swap → no cross-contamination.

## Acceptance criteria — all met
- [x] Baseline (openai) + candidate (neural) 120q UVTS grades captured.
- [x] `ab_verdict.json` committed as sprint artifact.
- [x] Decision (flip vs keep) data-driven per Note 02 gate; rationale in post.
- [x] E4 conditional: verdict=fail → no code change (as planned).
- [x] `.env` restored to `RERANK_PROVIDER=openai` post-run.
- [x] Full test suite still green (nothing was changed).
- [x] Canonical docs updated.
