# Sprint GUIDANCE-SYNTH-001 — Fix Guidance Synthesis Timeout (Follow-up B)

> **Status:** DRAFT — awaiting user approval before implementation.
> **Type:** P1 performance + correctness fix (guidance pipeline reliability).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | GUIDANCE-SYNTH-001 |
| **Sprint line** | `docs/development/guidance-synth-001/` |
| **Date opened** | 2026-06-08 |
| **Target version** | v0.11.3 (patch) |
| **Estimated effort** | 1–1.5 dev-days |
| **OpenAI / LLM spend** | $0 (local model; this *reduces* LLM wall-clock by parallelizing) |
| **Risk level** | Medium. Touches the guidance hot path (`Guide()` + the consulting classifier) with concurrency. Bounded: concurrency is capped + config-gated, falls back to serial at cap=1; the timeout change is a config-driven bound. |
| **Priority** | P1 — Follow-up B from RRF-SCALE-001 / JIMINY-OUTCOME-001, now the most operationally-visible remaining guidance issue: synthesis fails on **every** production warm call, so the hook never delivers a synthesized guidance narrative. |

## 2. Problem Statement

LLM guidance **synthesis fails on the production path every time** (`synthesis_error: context deadline exceeded`; live data: 6/6 `jiminy.synthesize` calls errored). Root cause, measured (§11):

The hook's production path is `POST /v1/jiminy/warm` → a **background `Guide()` with a hardcoded 30-second timeout** (`internal/api/handlers_jiminy.go:302`), even though `JIMINY_TIMEOUT_MS=240000` is configured. Inside that 30s budget:

1. **The per-node constraint classifier runs serially.** `consulting.findApplicableConstraints` calls the LLM classifier once per retrieved node in a serial loop. Measured `consulting.classify`: **~1.55s avg, 7.6s max**. ~10 retrieved nodes on cache-miss ⇒ **~15s** consumed before synthesis starts. (An LRU cache keyed by node_id helps in steady state, but each prompt retrieves different nodes, so cache-misses are common.)
2. **Synthesis needs 8–27s** (measured `jiminy.synthesize`: 8.0s avg, **27s max**) but inherits only the remaining ~15s of the 30s budget.
3. ⇒ **15s classifier + 8–27s synthesis = 23–42s needed, 30s available ⇒ synthesis deadline-exceeds.**

Net production effect: the guidance loop surfaces items (post-RRF-SCALE-001 + JIMINY-OUTCOME-001) but the **synthesized narrative** (`prompt_augmentation`) is never produced on the warm path — the hook shows static formatting instead of the intended LLM narrative. It also makes guide calls slow (~31s observed) and prone to fast-fail under LLM contention (the test-flakiness seen in the prior two sprints).

## 3. Scope & Constraints

### In scope
1. **Parallelize the per-node constraint classifier** (Epic 1): `findApplicableConstraints` classifies nodes with **bounded concurrency** (config-driven cap, default 4 to match llama-server `--parallel 4`). Falls back to serial at cap=1. Preserves dedup + result determinism. Cuts the classifier from ~sum(N) to ~ceil(N/cap)·avg.
2. **Config-drive the warm-compute timeout** (Epic 2): replace the hardcoded 30s at `handlers_jiminy.go:302` with `JIMINY_WARM_COMPUTE_TIMEOUT_MS` (default 90000 — accommodates parallel-classifier + a slow 27s synthesis with headroom). No-hardcoding rule (the bug *is* a hardcoded 30s).
3. **3-tier testing** — acceptance bar: **synthesis succeeds (no `synthesis_error`) on the live warm path**, plus a measured guide-latency reduction.
4. **Documentation** — CHANGELOG, CLAUDE.md, post.md.

### Out of scope
- **Replacing the synthesizer / changing the LLM model or runtime** — the model + 8–27s synthesis latency are accepted; we fit the budget to it, not the reverse.
- **Caching synthesis output** — possible future optimization; synthesis is context-specific so cache-hit rate is low.
- **Follow-up C** (`/v1/jiminy/latest` JSON control-char escaping) — separate, small; tracked.
- **Reworking the constraint classifier's LLM prompt or accuracy** — only its *concurrency* changes here.
- **The direct `/v1/jiminy/guide` handler timeout** — the warm path is the production path; `/guide` is a debug/test surface. We'll verify `/guide` isn't *further* capped, but the fix targets warm.

### Constraints
- Sequential epics; no-hardcoding rule; Tier 3 live testing required; rigorous verification (observe `synthesis_used=true` / absence of `synthesis_error` live).
- Concurrency must respect the LLM endpoint's capacity (llama-server `--parallel 4`) — the default cap matches it; the cap is config-driven so it tracks the runtime.
- Never deadlock / leak goroutines; bounded by a semaphore + the parent ctx.
- Parallelization must not change *which* constraints surface (only the speed) — Tier 1 asserts identical output vs serial.

## 4. Dependencies

- **JIMINY-OUTCOME-001 + RRF-SCALE-001** (merged) — guidance now surfaces items + assigns codes; this makes synthesis *reachable*, exposing the timeout.
- **`consulting.findApplicableConstraints`** + the LLM `ConstraintClassifier` (LRU-cached, per-node) — the serial loop to parallelize.
- **`handlers_jiminy.go` warm handler** — the hardcoded-30s site.
- **llama-server `--parallel 4`** — the concurrency the endpoint supports; basis for the default cap.
- **Live stack** (Neo4j + TSDB + llama-server) + the `llm_interactions` table for latency measurement.

## 5. Implementation Plan

### Epic 0 — Sprint plan (~0.1 day)
Commit this plan. No code.

### Epic 1 — Parallelize the per-node constraint classifier (~0.5 day)
- In `findApplicableConstraints`, replace the serial `for _, r := range results { classify }` with bounded-concurrency classification: a worker pool / semaphore of `cfg.ConsultingClassifyConcurrency` (default 4), each goroutine classifying one node, results collected and then deduped/ordered exactly as today.
- Preserve: the `r.Score < constraintFloor` pre-gate (RRF-SCALE-001), the keyword fallback on classifier error, the dedup-by-name, and output ordering (sort/stable to keep determinism).
- Respect the parent ctx (cancel on deadline); bound goroutines (no unbounded fan-out); the LRU cache remains (concurrent-safe — verify the classifier's cache mutex).
- New config: `CONSULTING_CLASSIFY_CONCURRENCY` (int, default 4, floor 1 = serial).
- Tier 1 unit tests: parallel path yields the **same constraints** as serial for the same inputs (determinism); cap=1 == serial; classifier-error → keyword fallback still works per node; cancellation/ctx-deadline returns promptly without leaking.
**Gate:** unit tests green; `go build`/lint clean; the classifier cache remains race-free (`go test -race`).

### Epic 2 — Config-drive the warm-compute timeout (~0.25 day)
- Replace `context.WithTimeout(context.Background(), 30*time.Second)` at `handlers_jiminy.go:302` with `time.Duration(s.cfg.JiminyWarmComputeTimeoutMs) * time.Millisecond` (zero-value fallback to 90000).
- New config: `JIMINY_WARM_COMPUTE_TIMEOUT_MS` (int, default 90000). Document the relationship: must exceed parallel-classifier time + `JIMINY_SYNTHESIS_TIMEOUT_MS`-bounded synthesis.
- Tier 1: config default + zero-value fallback.
**Gate:** unit tests green; lint clean.

### Epic 3 — Tier 2/3 live e2e + docs + close (~0.4 day)
- **Tier 2 integration** (skip-on-empty + LLM-tolerant): a warm→latest cycle yields guidance with `synthesis_used` true (or at least no `synthesis_error`) on a populated+idle stack.
- **Tier 3 live e2e (acceptance bar):**
  1. Restart from the build; baseline a warm call's latency + `synthesis_error` presence.
  2. Trigger `/v1/jiminy/warm` (production path) on a real context; read `/v1/jiminy/latest`.
  3. Confirm **no `synthesis_error`** and `synthesis_used=true` (or a non-empty synthesized `prompt_augmentation`).
  4. Measure: `consulting.classify` wall-clock for the guide call drops (parallel vs serial); `jiminy.synthesize` now succeeds (check `llm_interactions`: error column empty for new synth calls).
  5. Confirm guidance still surfaces the same constraints (no behavior regression) and codes still attach (JIMINY-OUTCOME-001 intact).
  - Transcript → `docs/development/guidance-synth-001/verification.md`.
- **Docs:** CHANGELOG, CLAUDE.md note (warm-compute timeout + classifier concurrency are config-driven; the 30s hardcode was starving synthesis), post.md.
**Gate:** synthesis succeeds live (no error); latency measurably reduced; no behavior regression; docs done.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit:** parallel-vs-serial determinism (same constraints out); cap=1 == serial; per-node classifier-error → keyword fallback; ctx-cancel returns promptly, no goroutine leak; `-race` clean on the classifier cache; config defaults + zero-value fallbacks (concurrency, warm timeout).

**Tier 2 — Integration (`-tags=integration`, skip-on-empty + LLM-tolerant):** warm→latest on a populated stack yields guidance without `synthesis_error`.

**Tier 3 — Live e2e:** the warm production path produces a synthesized narrative (no `synthesis_error`, `synthesis_used=true`); measured classifier-latency reduction + synthesis success in `llm_interactions`; no constraint-surfacing regression. Transcript in `verification.md`.

## 7. Commit Strategy
Sequential commits per epic on `reh3376_dev01`; auto-PR. Epic 1 = parallel classifier + Tier 1 + `-race`. Epic 2 = warm timeout config. Epic 3 = integration test + verification.md + docs. Surprise bugs get their own fix-commit. Sprint summary on PR after Epic 3.

## 8. Verification Checklist
- [ ] `findApplicableConstraints` classifies with bounded concurrency (`CONSULTING_CLASSIFY_CONCURRENCY`, default 4, floor 1).
- [ ] Parallel output identical to serial for the same inputs (Tier 1 determinism); cap=1 == serial.
- [ ] Per-node classifier-error still falls back to keyword; ctx-cancel prompt; no goroutine leak; `-race` clean.
- [ ] Warm-compute timeout config-driven (`JIMINY_WARM_COMPUTE_TIMEOUT_MS`, default 90000); zero-value fallback.
- [ ] `go build ./...` + `golangci-lint` clean; Tier 1 + Tier 2 green.
- [ ] **Live: warm path produces a synthesized narrative — no `synthesis_error`, `synthesis_used=true`.**
- [ ] Live: `jiminy.synthesize` rows in `llm_interactions` now succeed (error column empty for new calls).
- [ ] Live: measurable guide-latency reduction (parallel classifier); constraints still surface + codes still attach (no regression).
- [ ] CHANGELOG, CLAUDE.md, post.md, verification.md updated.
- [ ] Sprint summary on PR.

## 9. Documentation Update — Epic 3 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Concurrency overruns llama-server `--parallel 4` → queueing/latency | Low | Low | Default cap = 4 (matches); config-driven so it tracks the runtime; excess requests queue, don't fail. |
| Parallelization changes which constraints surface (nondeterminism) | Low | Medium | Collect then sort/dedup exactly as serial; Tier 1 asserts identical output; ordering made deterministic. |
| Classifier LRU cache race under concurrency | Low | Medium | Cache already has a mutex (verify); `go test -race` gate. |
| 90s warm timeout lets a hung call linger in the background | Low | Low | It's a fire-and-forget background goroutine, bounded by the ctx; 90s is the ceiling, not the norm. Config-tunable down. |
| Synthesis still occasionally > budget (27s max + classifier) | Low | Low | 90s default leaves wide headroom (parallel classifier ~7.5s + 27s synthesis = ~35s ≪ 90s). |
| Goroutine leak / deadlock in the worker pool | Low | High | Bounded semaphore + WaitGroup + parent-ctx cancel; Tier 1 leak/cancel test; `-race`. |

## 11. Documents Accessed
- `internal/consulting/service.go` — `findApplicableConstraints` (the serial per-node classifier loop), `constraintFloor` gate
- `internal/consulting/llm_classifier.go` — `ConstraintClassifier` (LRU cache + mutex + `TimeoutMs`)
- `internal/jiminy/service.go` — `Guide` (642–646 ctx/timeout), synthesis region (1094–1118)
- `internal/jiminy/synthesizer.go` — `Synthesize` (73 timeout wrap)
- `internal/api/handlers_jiminy.go` — `handleJiminyWarm` background Guide (**302: hardcoded 30s**), `handleJiminyGuide`
- `internal/api/server.go` — synthesizer wiring (538–555; `TimeoutMs: cfg.JiminySynthesisTimeoutMs`)
- `internal/config/config.go` — `JiminyTimeoutMs` (240000), `JiminySynthesisTimeoutMs` (180000 wired correctly), `ConsultingClassifyTimeoutMs`
- `~/Library/LaunchAgents/com.mdemg.llama-server.plist` — `--parallel` (4)
- Live `llm_interactions` latencies: `consulting.classify` 1.55s avg/7.6s max (22 calls, 4 err); `jiminy.synthesize` 8.0s avg/27s max (6 calls, **6 err**)

## 12. Rollback Procedures
- **Config:** set `CONSULTING_CLASSIFY_CONCURRENCY=1` (serial, today's behavior) and/or `JIMINY_WARM_COMPUTE_TIMEOUT_MS=30000` (today's hardcoded value). Instant, no redeploy logic change.
- **Code revert:** Epic 1 (parallel classifier) and Epic 2 (warm timeout) are independent commits; revert either. No schema/data changes.
- The change is pure control-flow + a timeout bound; rollback restores prior behavior exactly.

---

## Files to be created/modified (anticipated)

**New:**
- `docs/development/guidance-synth-001/sprint_plan_guidance_synth_001.md` (Epic 0)
- `docs/development/guidance-synth-001/verification.md` (Epic 3)
- `docs/development/guidance-synth-001/post.md` (Epic 3)
- `tests/integration/guidance_synth_test.go` (Epic 3, skip-on-empty)

**Modified:**
- `internal/consulting/service.go` — `findApplicableConstraints` bounded-concurrency classification
- `internal/consulting/service_test.go` — Tier 1 (determinism, fallback, cancel)
- `internal/api/handlers_jiminy.go` — warm-compute timeout config-driven
- `internal/config/config.go` — `CONSULTING_CLASSIFY_CONCURRENCY`, `JIMINY_WARM_COMPUTE_TIMEOUT_MS`
- `CHANGELOG.md`, `CLAUDE.md` — Epic 3

## Acceptance Criteria
1. On the live **warm production path**, guidance synthesis **succeeds** — no `synthesis_error`, a synthesized narrative is produced (`synthesis_used=true`).
2. `jiminy.synthesize` calls in `llm_interactions` succeed post-fix (error column empty) where they were 6/6 errored before.
3. The per-node constraint classifier runs with bounded concurrency; measured guide-call classifier wall-clock is reduced vs serial.
4. Guidance still surfaces the same constraints + codes still attach — no behavior regression (RRF-SCALE-001 + JIMINY-OUTCOME-001 intact).
5. Both new knobs are config-driven with documented defaults; `-race` clean.
6. Rollback is a single config flip (`CONSULTING_CLASSIFY_CONCURRENCY=1`, `JIMINY_WARM_COMPUTE_TIMEOUT_MS=30000`).
