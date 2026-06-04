# RRF-SCALE-001 Epic 4 — Live Verification

**Date:** 2026-06-03
**Stack:** native `./bin/mdemg serve` (rebuilt from this branch) + Docker (Neo4j + TSDB) + llama-server :8102. Space `mdemg-dev`.

## What the score-gate fix revived (PASS)

### 1. Guidance surfacing: 0 → 10 items

`POST /v1/jiminy/guide` with context "never commit directly to the main branch, always use a dev branch and pull request":

| Metric | Before fix | After fix (model warm) |
|---|---|---|
| guidance items | **0** | **10** |
| `source_counts.constraints` | **0** | **2** |
| `source_counts.patterns` | 0 | 3 |
| `source_counts.suggestions` (debug) | 0 | 10 |
| `source_counts.retrievals` | 0 | 5 |

**Acceptance criterion #1 MET:** `/v1/jiminy/guide` returns non-empty guidance with `source_counts.constraints > 0` on the live stack.

> **Cold-start note:** the *first* guide call after a server restart returned `constraints:0` because the LLM constraint classifier hit a cold-model timeout (`context deadline exceeded` on 8102) and fell back to keyword classification (which doesn't match emergent-concept summaries). After one warm-up call (`/v1/chat/completions` 1.3s), the classifier succeeded and constraints surfaced. This is a model-warmth artifact, not a fix defect — but see Follow-up B (synthesis timeout).

### 2. TSDB `constraint_outcomes` sink: REVIVED (dead since May 1)

Drove the full loop: `warm` (context) → `latest` (guidance_id `jcqegom7auxap0xsd6uay51v`, source_counts `constraints:2, patterns:3`) → `feedback` (outcome=followed, HTTP 200, 10 items processed).

Fresh rows landed (after the buffered-writer flush):

```
        time         |            constraint_id             | outcome_type | guidance_type
---------------------+--------------------------------------+--------------+--------------
 2026-06-03 11:40:44 | dc22b7ff-...                         | followed     | pattern
 2026-06-03 11:40:44 | 7ffb0113-...                         | followed     | concept
 2026-06-03 11:40:44 | 38f3c4a8-...                         | followed     | constraint
 ... (10 rows total: 2 constraint, 3 pattern, 5 concept)
```

Table was dead at 1,139 rows / last `2026-05-01`. Now writing live, dated today. **The constraint-effectiveness TSDB sink (the source for Grafana effectiveness panels) is observably revived.**

## Surfaced follow-ups (distinct root causes, NOT score-scale — documented, not silently dropped)

### Follow-up A — Neo4j `GUIDANCE_OUTCOME` edge sink still dormant

Acceptance criterion #2 wanted a fresh **Neo4j** `GUIDANCE_OUTCOME` edge too. It did **not** appear (still 893 edges / last Apr 12). Root cause, traced:

- The guidance items' `SourceNodes` point at **emergent_concept** nodes (retrieval surfaces L2–L5 concept *abstractions* of constraints, not the raw `role_type='constraint'` nodes — all top-10 retrieved for "commit to main" were `emergent_concept`).
- `PersistGuidanceOutcome` (jiminy/persistence.go) only writes an edge when the target resolves to `obs_type IN ['constraint','correction','pattern','learning'] OR role_type='constraint'`. `emergent_concept` fails that `WHERE`, so the edge is silently skipped.
- Confirmed: the TSDB rows' `constraint_id`s (`f6c27c8d`, `38f3c4a8`, `7ffb0113`) are all `role_type='emergent_concept'`.

**This is a different bug class than RRF-SCALE-001** (node-type targeting + retrieval surfacing concept-abstractions), independent of the score scale — it would occur under the legacy scorer too whenever concepts outrank raw constraints. The TSDB writer has no type filter so it revived; the Neo4j writer's type filter blocks concepts. Deferred to a follow-up sprint (candidate: **JIMINY-OUTCOME-001**) where the architecture decision belongs — should concepts be valid `GUIDANCE_OUTCOME` targets, should retrieval surface raw constraints alongside their concepts, or should concepts resolve to their underlying constraint nodes? Not a rushed bolt-on.

### Follow-up B — LLM guidance synthesis timeout

`debug.synthesis_error: "synthesis failed: ... context deadline exceeded"` appears on guide calls now that synthesis actually runs (it never ran before — no items to synthesize). Guidance items still surface without it (synthesis only composes the optional `prompt_augmentation` polish), so it doesn't block the loop. But the synthesis LLM deadline is being exceeded against the local model under load. Likely the same flavor as DH-004's timeout tuning. Distinct from score-scale; candidate follow-up.

### Follow-up C — `/v1/jiminy/latest` JSON control-character escaping

The `latest` response contains unescaped control characters (raw newlines in guidance content) that break strict JSON parsers (`jq`, `python json`). The hook `prompt-context.sh` parses this with `jq` — so this may *also* impair the real hook's `guidance_id` capture. Worth verifying whether the hook path is affected; if so it compounds the dormancy. Distinct, low-effort follow-up.

## Conclusion

**RRF-SCALE-001's mandate — the RRF score-scale consumer bug — is fixed and live-verified.** The 0.55-gate-class defect that silently zeroed all consulting guidance is resolved: guidance surfacing went 0→10, constraints 0→2, and the TSDB constraint-outcomes sink (dead 5 weeks) is writing live, dated today. The score-gate fix is the correct, sufficient remedy for the score-scale root cause.

Three adjacent issues surfaced during live smoke — the Neo4j-edge node-type gap (A), synthesis timeout (B), and `latest` JSON escaping (C) — are **distinct root causes outside the score-scale mandate**, characterized here and recorded as follow-ups rather than bolted on. Per the live-smoke-surprise discipline, they get honest documentation + their own future fix-commits, not silent inclusion or silent omission.
