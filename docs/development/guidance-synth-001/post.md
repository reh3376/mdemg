# Sprint GUIDANCE-SYNTH-001 — Post

**Closed:** 2026-06-08
**Branch:** `reh3376_dev01`
**Plan:** [`sprint_plan_guidance_synth_001.md`](sprint_plan_guidance_synth_001.md)
**Verification:** [`verification.md`](verification.md)

## Outcome

Guidance synthesis — which failed on **every** production warm call — now succeeds. **Closes Follow-up B.** The guidance pipeline (surfacing from RRF-SCALE-001, constraint-code attachment from JIMINY-OUTCOME-001, and synthesis here) is now fully functional end-to-end.

## Root cause (measured)

The hook's `/v1/jiminy/warm` background `Guide()` had a hardcoded 30s timeout. Inside it, the per-node LLM constraint classifier ran serially (`consulting.classify` ~1.55s avg × ~10 nodes ≈ 15s on cache-miss), leaving ~15s for synthesis which needs 8–27s (observed up to 50s) → deadline-exceeded. `JIMINY_TIMEOUT_MS=240s` was configured but the 30s hardcode capped it.

## Fix

1. **Parallelize the per-node classifier** (`CONSULTING_CLASSIFY_CONCURRENCY`, default 4): bounded worker pool, position-indexed slots, collect-in-order + dedup → identical output to serial.
2. **Config-drive the warm-compute timeout** (`JIMINY_WARM_COMPUTE_TIMEOUT_MS`, default 90000): replaced the hardcoded 30s.

Both were needed: a single warm synthesis was measured at **50.7s** — parallelizing alone wouldn't survive that in a 30s budget; raising the budget alone leaves guide calls slow.

## Epic-by-epic

| Epic | Status | Notes |
|---|---|---|
| 0 — Plan | ✅ | Grounded in `llm_interactions` latency data (classify 1.55s avg, synthesize 8s avg/27s max/6-of-6 errored). |
| 1 — Parallel classifier | ✅ | Bounded concurrency + `constraintClassifierIface` extraction for testability. 5 Tier 1 tests, `-race` clean. Determinism (parallel==serial) asserted. |
| 2 — Warm timeout config | ✅ | 30s hardcode → `JIMINY_WARM_COMPUTE_TIMEOUT_MS` (90s). |
| 3 — Live e2e + docs | ✅ | `synthesis_used=true`, fresh synthesize OK at 50.7s, Tier 2 pass. |

## Acceptance criteria — all met

1. ✅ Warm path produces a synthesized narrative (`synthesis_used=true`, no `synthesis_error`).
2. ✅ Fresh `jiminy.synthesize` succeeds (50.7s, fit the 90s budget) where it was 6/6 errored.
3. ✅ Classifier runs with bounded concurrency (Tier 1 determinism + parallel-faster).
4. ✅ No behavior regression (constraints surface + codes attach).
5. ✅ Both knobs config-driven; `-race` clean.
6. ✅ Rollback is a config flip.

## Discipline notes

- **The data drove the diagnosis.** The `llm_interactions` table gave exact latencies (classify 1.55s, synthesize up to 27s, 6/6 errored) that pinned the budget arithmetic (15s classifier + 27s synthesis > 30s). Without it, "synthesis times out" could have been mis-attributed to the synthesizer config (which was actually correct at 180s — the *parent* warm ctx was the cap).
- **50.7s synthesis validated the 90s default.** The live call came in well above the 27s max seen in the baseline window — a 60s budget would have been risky. The generous 90s default is justified, not arbitrary.

## Forward-looking

- **Follow-up C — `/v1/jiminy/latest` JSON control-char escaping** is now the last open item from the original RRF-SCALE-001 triage. Small: verify whether unescaped control chars in the `latest` response break the `jq` parse in `prompt-context.sh` (which would impair the hook's `guidance_id` capture). If so, escape at the writer; if not, close it.
- **Synthesis latency (~50s)** is high for the local model on a ~1900-char narrative. Not a bug (fire-and-forget on the warm path), but worth watching — tunable via the budget or `JIMINY_GUIDANCE_OUTPUT_MAX_CHARS`. If it grows, consider a smaller synthesis target or a faster path.
- **13/111 constraints still lack embeddings** (JIMINY-OUTCOME-001 carry-over) — `mdemg embeddings backfill --space-id mdemg-dev` is the operator remedy.

## Documents Accessed
- `internal/consulting/service.go` — `findApplicableConstraints` (parallelized), `constraintClassifierIface`, `SetConstraintClassifier`
- `internal/consulting/llm_classifier.go` — `ConstraintClassifier` (LRU cache + mutex), `ConstraintClassification`
- `internal/api/handlers_jiminy.go` — `handleJiminyWarm` (the 30s hardcode → config)
- `internal/jiminy/service.go` / `synthesizer.go` — `Guide` ctx, synthesis call + its (correct) 180s timeout
- `internal/config/config.go` — `CONSULTING_CLASSIFY_CONCURRENCY`, `JIMINY_WARM_COMPUTE_TIMEOUT_MS`
- `~/Library/LaunchAgents/com.mdemg.llama-server.plist` — `--parallel 4`
- Live `llm_interactions`: baseline (synthesize 6/6 errored) → post-fix (synthesize OK at 50.7s)
