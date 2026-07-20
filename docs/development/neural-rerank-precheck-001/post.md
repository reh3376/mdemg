# NEURAL-RERANK-PRECHECK-001 — Sprint Post (2026-07-20)

## Summary
Fixed a lurking calibration bug in the just-shipped LLM-HEALTH-INVESTIGATION-001 E2 pre-check: the single-knob `RerankMinBudgetMs=12000` was correct for the LLM path (p99=11.7s) but over-skipped viable calls on the neural path (p95 ~1s). Split into two provider-aware knobs; live-verified on the neural sidecar.

## What shipped
- **E0** — sprint plan (v1.0 12-section format).
- **E1** — `NeuralRerankMinBudgetMs` config (env `NEURAL_RERANK_MIN_BUDGET_MS`, default 1500ms, floor 500). Pre-check in `Rerank()` selects the appropriate budget based on `RerankProvider`. WARN log gains `provider` field.
- **E2** — 4 provider-aware Tier-1 pins covering the truth table: neural allowed under LLM knob but above neural knob; openai skips at same caller budget; neural skips below neural knob; neural bypass on no-deadline. All existing E2 tests continue to pass (delegated through new `newRerankTestServiceProvider` helper).
- **E3** — live Tier-3 on `mdemg-dev` with neural sidecar UP. Documented in `live_verification.md`:
  - default openai rerank preserved (`rerank_ms=2588`).
  - `RERANK_PROVIDER=neural` completed in **122ms** — 20× faster than the LLM path.
  - forced skip via `NEURAL_RERANK_MIN_BUDGET_MS=30000` fires deterministically with `provider=neural` in the WARN log.
- **E4** — CLAUDE.md architecture note (with the "single-knob gate over switchable dispatch → tune per-branch" pin); CHANGELOG `[Unreleased] > Fixed`; this post.

## Commits (on `reh3376_dev01`)
1. `docs(neural-rerank-precheck-001): E0 — sprint plan` — `e1960e5`
2. `feat(neural-rerank-precheck-001): E1 — provider-aware rerank pre-check budget` — `4f9107f`
3. `test(neural-rerank-precheck-001): E2 — provider-aware pre-check tests` — `23b04ce`
4. `docs(neural-rerank-precheck-001): E3 — live Tier-3 verification` — `a295abb`
5. `docs(neural-rerank-precheck-001): E4 — CLAUDE.md/CHANGELOG/post`

## Live evidence highlights
| Metric | Pre-sprint | Post-sprint |
|---|---|---|
| RerankProvider=neural + tight caller budget | over-skipped (LLM knob) | allowed (neural knob) |
| Neural sidecar latency (live) | 122ms | 122ms (unchanged; neural sidecar not touched) |
| LLM path latency (live) | 2588ms | 2588ms (unchanged) |
| Force-skip WARN log field for grepability | absent | `provider=neural`/`provider=openai` present |
| Fail-open on skip | preserved | preserved |

## Lessons captured
1. **When a global gate sits BEFORE a switchable dispatch path, tune per-branch.** A single knob calibrated for one branch silently misbehaves on the others. The correct shape is either (a) split into per-provider knobs, or (b) derive the threshold from the branch's own parameters (which requires the parameters to be a good proxy — `RerankTimeoutMs` is worst-case, not p95, so derivation was rejected here).
2. **The neural sidecar is measurably 20× faster than the LLM path on this workload** (122ms vs 2588ms same query). Kept out of scope for this sprint but noted as a data-decided default-flip candidate.
3. **`sed -i` on macOS defaults `.bak` file creation** — always clean up (`rm -f *.bak`) or use `-i ''` on macOS. Caught during E3 provider-flip.

## Non-goals (respected)
- Retuning `RerankMinBudgetMs` for the LLM path (still 12000, empirically calibrated).
- Adding per-provider budgets for `ollama`/`jina` (share `RerankMinBudgetMs` — same LLM shape).
- Deriving the budget from `RerankTimeoutMs` (rejected; timeout is worst-case, not p95).
- Modifying the neural sidecar itself.
- Default-flipping `RERANK_PROVIDER` to `neural` (needs a UVTS A/B on quality parity).

## Follow-ups
- **RERANK_PROVIDER=neural default-flip** — a UVTS A/B on the whk-wms corpus would show if the cross-encoder quality is on par with the LLM-judge quality; if yes, the 20× latency win justifies flipping the default.
- **Skip-rate gauge (still open from LLM-HEALTH-INVESTIGATION-001)** — a Prometheus counter tagged by provider would give operator early-warning of budget pressure per provider.
- **Neural sidecar timeout tightening** — 122ms observed vs 1000ms default; `NEURAL_RERANK_TIMEOUT_MS=500` would still comfortably cover p99 and fail faster on true stalls.

## Acceptance criteria — all met
- [x] `NeuralRerankMinBudgetMs` config surfaces with sensible default (1500ms).
- [x] Pre-check uses `NeuralRerankMinBudgetMs` when `RerankProvider=neural`.
- [x] Pre-check uses `RerankMinBudgetMs` for openai/ollama/jina/default (existing behavior preserved).
- [x] WARN log includes provider field.
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated.
