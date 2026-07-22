# Guardrail Producer — live training rows for guardrail.evaluate

**Sprint**: GUARDRAIL-PRODUCER-001 (2026-07-22)
**Status**: shipped; code default OFF (`GUARDRAIL_PRODUCER_ENABLED`), enabled in the dev `.env` after live smoke
**Related**: FT-RECURSIVE loop (this unblocks the 16th call site's retraining), JIMINY-ACTIONABILITY-001 (the Lever-C retrieval pattern reused here)

## Why

`guardrail.evaluate` is a fully shipped LLM call site (service, endpoint, MCP
tool, ULTS/UBENCH contracts) that nothing invoked in normal operation — ~3
`llm_interactions` rows total, all hand-made in April. A call site with no
production rows can never be retrained. Meanwhile every Claude Write/Edit
already flows through the PostToolUse hook, which was discarding exactly the
evidence guardrail needs: real code changes to evaluate against the real
constraint corpus.

## How it works

1. **Hook** (`post-tool-observe.py`, template + live copy): on successful
   Write/Edit, synthesize a minimal diff (Write → `+` content lines; Edit →
   `-` old / `+` new), capped at `MDEMG_GUARDRAIL_DIFF_MAX_BYTES` (8000),
   and POST `/v1/memory/guardrail/validate` with `async:true,
   agent_trust_level:"standard"` via detached `Popen(curl)` — the hook never
   waits or fails on this.
2. **Server async path**: gate on `GUARDRAIL_PRODUCER_ENABLED` → semaphore
   try-acquire (`GUARDRAIL_PRODUCER_MAX_CONCURRENT`, default 1; busy =
   **drop, not queue** — the producer is opportunistic sampling, and
   unbounded backlog is llama-server saturation) → evaluation detached from
   the request context (fresh Background + `GUARDRAIL_TIMEOUT_MS` bound; a
   hook curl exiting must never caller-cancel the eval mid-LLM) →
   **202 {status:"queued"}** in ~0ms.
3. **The product** is the ordinary `llm_interactions` row the detached
   `Validate` records (task `guardrail.evaluate`, the hook's space_id via
   `llmclient.WithSpaceID`). No new tables. HITL's gold-only
   Guardrail-Evaluate dataset reviews these rows; the FT-RECURSIVE pipeline
   retrains from them.

Responses on the async path carry no verdict: `queued` (202) / `disabled` /
`dropped` (200). The synchronous path (MCP `validate_changes`) is unchanged.

## Config

| Env var | Default | Notes |
|---|---|---|
| `GUARDRAIL_PRODUCER_ENABLED` | `false` | Flip after smoke (dev `.env` runs `true`) |
| `GUARDRAIL_PRODUCER_MAX_CONCURRENT` | `1` (floor 1) | Detached evaluations in flight; excess dropped |
| `MDEMG_GUARDRAIL_DIFF_MAX_BYTES` | `8000` | Hook-side diff cap (hook process env) |

Existing knobs that shape the producer: `GUARDRAIL_TIMEOUT_MS` (detached
bound), `GUARDRAIL_CONSTRAINT_SIM_FLOOR` (0.3), `GUARDRAIL_MAX_CONSTRAINTS`.

Observability: `mdemg_guardrail_producer_total{status=queued|dropped|disabled}`
counters + a completion log line (`guardrail producer: evaluation complete`
with status/violations/warnings).

## The retrieval fix this smoke surfaced

First live runs returned instant Pass with **zero constraints matched at any
floor** — `semanticSearch` used `db.index.vector.queryNodes` top-200 then
filtered `role_type='constraint'`: the JIMINY-ACTIONABILITY-001 Lever-C
class. Constraints are ~0.1% of a production-scale space and never appear in
a global top-200 (tiny UATS spaces fit inside 200, which is why contracts
stayed green). **Guardrail semantic retrieval had been structurally blind on
real spaces since Phase 104.** Fixed with the shipped Lever-C pattern:
role-filtered `vector.similarity.cosine` over only the constraint partition
(63 live nodes on mdemg-dev — O(n) trivial). Own fix-commit.

## Live evidence (2026-07-22, mdemg-dev)

- Hook-shaped stdin through the real `post-tool-observe.py` → 202 in 0ms →
  detached eval 1.7s → **first production row ever**: `tokens_in=1489`
  (constraints in prompt), verdict recorded.
- Cost reality at floor 0.3 in a 63-constraint space: ~every sampled edit
  matches ≥1 constraint (even a cooking-recipe edit) → expect ~1 LLM call
  per non-dropped edit (~1.5–2.7s each). Cost control is the concurrency
  bound + drop policy, NOT the floor. Broad matching is fine for training
  data — Pass rows are negatives the model must learn.
- Drop path live: concurrent second call → `dropped`; counters
  `queued=3, dropped=1`.
- UATS `guardrail_validate` 5/5 incl. the new env-robust
  `async_producer_accepted` variant (accepts queued/disabled/dropped).

## Corrections in retrieval (GUARDRAIL-CORRECTIONS-001 — shipped 2026-07-22)

`GUARDRAIL_INCLUDE_CORRECTIONS` (code default false; dev `.env` true after
smoke) unions `role_type='correction'` into BOTH retrieval phases
(semantic partition-cosine + keyword). Corrections carry no
`constraint_type`, so the Cypher coalesces their type to `'correction'` —
rendered in the prompt and capped at **Warning** tier
(`isBlockingType` is must/must_not only): learned lessons advise, hard
constraints block. Live: the `mdemg db start` fixture returned
**Warning (warnings=1)** with 5 correction items in the prompt; flag-off
(process-env-verified) is constraints-only. Applies identically to the
sync MCP `validate_changes` path (one retrieval function — disclosed).

## Follow-ups (disclosed, not built)
- Producer-specific sim floor if row noise ever outweighs the negative-
  example value (data-decide from HITL grades, not up front).
- Sampling/throttle knobs if the concurrency-bound-plus-drop policy proves
  too coarse.
