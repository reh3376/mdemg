# GUIDANCE-SYNTH-001 Epic 3 — Live Verification

**Date:** 2026-06-08
**Stack:** native `./bin/mdemg serve` (rebuilt from this branch) + Docker (Neo4j + TSDB) + llama-server :8102. Space `mdemg-dev`.

## Acceptance bar (PASS): synthesis succeeds on the production warm path

### Before (baseline)
`jiminy.synthesize` was failing on every warm call — the 6 most recent calls were all `context deadline exceeded` (the warm-path background `Guide()` had a hardcoded 30s budget; serial per-node classifier ~15s + synthesis 8–27s > 30s).

### After
Triggered the production path `POST /v1/jiminy/warm` (real context) → `GET /v1/jiminy/latest`:

```
synthesis_used:      true
synthesis_error:     (absent)
prompt_augmentation: 1892 chars, "═══ JIMINY GUIDANCE ═══ …" (real synthesized narrative)
```

Fresh `jiminy.synthesize` row in `llm_interactions` (post-restart):

```
time (UTC)            | latency_ms | status
2026-06-08 14:43:48   | 50719      | OK ✓
```

**The synthesis took 50.7 seconds and SUCCEEDED** — it completed only because the budget is now 90s (`JIMINY_WARM_COMPUTE_TIMEOUT_MS`). At the old hardcoded 30s it would have deadline-exceeded; even a 60s budget would have been risky for this call. This both fixes the bug and validates the 90s default choice.

### Classifier parallelization
`consulting.classify` in the window showed few calls (LRU cache warm after prior calls), each ~2.5s. The Tier 1 `ParallelIsFaster` test confirms the concurrency overlaps per-node latency (8 nodes × 20ms: serial ~160ms → parallel(4) ~40ms); in production the cold-cache path (~10 nodes × 1.5s serial = ~15s) now runs ~4-wide (~4s), freeing budget for synthesis.

## Acceptance criteria — met
1. ✅ Warm path produces a synthesized narrative — `synthesis_used=true`, no `synthesis_error`, 1892-char JIMINY GUIDANCE augmentation.
2. ✅ Fresh `jiminy.synthesize` succeeds (`OK`, 50.7s) where it was erroring before.
3. ✅ Per-node classifier runs with bounded concurrency (Tier 1 `ParallelIsFaster` + determinism).
4. ✅ No behavior regression — guidance still surfaces constraints + codes still attach (the warm result carried coded items; JIMINY-OUTCOME-001 + RRF-SCALE-001 intact).
5. ✅ Both knobs config-driven (`CONSULTING_CLASSIFY_CONCURRENCY` 4, `JIMINY_WARM_COMPUTE_TIMEOUT_MS` 90000); Tier 1 `-race` clean.
6. ✅ Rollback is a config flip (`CONSULTING_CLASSIFY_CONCURRENCY=1`, `JIMINY_WARM_COMPUTE_TIMEOUT_MS=30000`).

## Tier 2 integration test

`TestGuidanceSynth_WarmPathSynthesisSucceeds` (`-tags=integration`, skip-on-empty + LLM-tolerant): **PASS** — "warm-path guidance produced without synthesis_error (synthesis_used=true)". Skips on an empty/LLM-unavailable environment (CI) where synthesis isn't exercisable.

## Note: synthesis latency is high (50s)

The local model takes up to ~50s for the ~1900-char guidance narrative. The 90s budget accommodates it, but this latency is itself worth watching — if it grows, the budget (config-tunable) or the synthesis prompt size (`JIMINY_GUIDANCE_OUTPUT_MAX_CHARS`) may need attention. Not in scope here; the warm path is fire-and-forget background compute, so the latency doesn't block the hook (which reads the cached `/latest`).

## Conclusion

GUIDANCE-SYNTH-001's acceptance bar is met and live-verified: the warm-path guidance synthesis, which failed on every production call, now succeeds. The two fixes — parallelizing the per-node constraint classifier and config-driving the warm-compute timeout (30s → 90s) — together give synthesis the budget it needs. **This closes Follow-up B**; the guidance pipeline (surfacing + codes + synthesis) is now fully functional end-to-end.
