# JIMINY-OUTCOME-001 Epic 2 — Live Verification

**Date:** 2026-06-08
**Stack:** native `./bin/mdemg serve` (rebuilt from this branch) + Docker (Neo4j + TSDB) + llama-server :8102. Space `mdemg-dev`.

## Tier 3 live e2e — the acceptance bar (PASS)

### Setup
- Rebuilt + restarted server from the Epic 1 commit; warmed the LLM.
- Baseline Neo4j `GUIDANCE_OUTCOME`: **893 edges, last 2026-04-12** (dormant ~8 weeks).

### Step 1 — guidance items now carry constraint codes (was 0)
`POST /v1/jiminy/guide` with context "never commit directly to the main branch…" → **10 items, 6 carrying a `constraint_code`** (before the fix: 0):

```
concept -> no-direct-main-commits
concept -> no-direct-main-commits
pattern -> no-direct-main-commits
pattern -> no-direct-main-commits
pattern -> mandatory-use-cms-every-session
concept -> no-direct-main-commits
```

The matched code `no-direct-main-commits` is **semantically exact** for the "commit to main" context — the embedding matcher correctly links concept-abstracted guidance back to the right raw constraint.

### Step 2 — full loop produces fresh Neo4j edges ON REAL CONSTRAINT NODES
`feedback` (outcome=followed, 10 items processed) → Neo4j edge count **893 → 899** (+6), latest **2026-06-08** (today). The 6 new edges:

```
src_role     | code                            | outcome  | node_name
constraint   | no-direct-main-commits          | followed | CONSTRAINT: NEVER commit directly t…
constraint   | no-direct-main-commits          | followed | CONSTRAINT: NEVER commit directly t…
constraint   | no-direct-main-commits          | followed | CONSTRAINT: NEVER commit directly t…
constraint   | no-direct-main-commits          | followed | CONSTRAINT: NEVER commit directly t…
constraint   | mandatory-use-cms-every-session | followed | # CMS Endpoints (Conversation Memor…
constraint   | no-direct-main-commits          | followed | CONSTRAINT: NEVER commit directly t…
```

**All 6 land on `role_type='constraint'` nodes** (not `emergent_concept`) — the exact gap RRF-SCALE-001 left open. The sink dormant since Apr 12 is observably revived, with edges on the *correct* nodes.

### Step 3 — constraint effectiveness reflects it
`GET /v1/constraints/effectiveness?space_id=mdemg-dev` →

```
CONSTRAINT: NEVER commit directly to main… | surfaced: 30  followed: 28  rate: 0.93
```

The constraint we just gave a "followed" outcome now shows updated graph-aggregated effectiveness. `GetConstraintEffectiveness` (which reads `role_type='constraint'` `GUIDANCE_OUTCOME` edges) is live again.

### Acceptance criteria — all met
1. ✅ Fresh Neo4j `GUIDANCE_OUTCOME` edge on a real `role_type='constraint'` node, dated today.
2. ✅ Matched `constraint_code` (`no-direct-main-commits`) semantically correct for the context.
3. ✅ `/v1/constraints/effectiveness` reflects the new outcome.
4. ✅ Threshold config-driven (`JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD`, default 0.55); validated live — correct matches, no false positives.
5. ✅ Keyword fallback intact (Tier 1); degrades gracefully without an embedder.
6. ✅ **Both sinks now revived** — TSDB (RRF-SCALE-001) + Neo4j (here). The constraint-effectiveness loop is fully restored.

## Threshold grounding (0.55)

The default `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD=0.55` was validated against live behavior: the "commit to main" context matched `no-direct-main-commits` (correct) and a CMS-context item matched `mandatory-use-cms-every-session` (correct), while 4/10 unrelated retrievals matched nothing (correctly rejected). No false positives observed. 0.55 sits sensibly between the Evaluator's `sim > 0.4` floor and the ~0.70 natural-language cosine ceiling. Operators can tune via the env var.

## Tier 2 integration test

`TestJiminyOutcome_GuidanceItemsGetConstraintCodes` (`-tags=integration`, skip-on-empty per the RRF-SCALE-001 CI lesson): against the populated stack with an idle LLM it **PASSES** — "7/10 guidance items carry a constraint_code (was 0 before fix)".

**Known characteristic:** the `/v1/jiminy/guide` path is LLM-latency-dependent — the per-node constraint classifier makes serialized LLM calls (~31s/call) and a guide call fired while the LLM is busy fast-fails with empty guidance. The test therefore **warms via retries and skips (does not fail)** when the LLM path can't produce items in the environment. It is a bonus check; the **Tier 3 live e2e above is the definitive proof**. (This LLM-serialization/synthesis-timeout behavior is RRF-SCALE-001 Follow-up B, tracked separately.)

## Conclusion

JIMINY-OUTCOME-001's acceptance bar is met and live-verified: the embedding-similarity matcher links concept-abstracted guidance to the correct constraint codes, the existing `PersistGuidanceOutcome` machinery creates `GUIDANCE_OUTCOME` edges on real constraint nodes, and constraint effectiveness updates. Combined with RRF-SCALE-001 (TSDB sink), **the guidance→feedback→outcome loop is now fully revived across both sinks.**
