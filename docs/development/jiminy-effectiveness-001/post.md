# JIMINY-EFFECTIVENESS-001 — Sprint Post

**Date:** 2026-06-23 · branch `reh3376_dev01` · the second guidance sprint (after
JIMINY-SIGNAL-001 made the signals honest).

## The premise correction (why recon mattered)
The approved plan assumed the agent was being penalized for ignoring `confidence:0`
junk guidance. **Live recon contradicted that** before any code was written:
surfaced guidance is filtered above `JiminyMinConfidence=0.3` (confidence
0.7–0.9), and the **LLM** classifier (not the heuristic) judges ~79% of it
`Ignored`. So the agent genuinely isn't following confident, on-topic guidance —
and the real J17-T1 blocker is the **trust mechanic**, not junk guidance. The
plan was re-scoped (operator-approved) to **Option A: trust as a recoverable
windowed signal**.

## Outcome
`TrustScorer.RecordOutcome` was a **monotonic ratchet** (Followed +0.05 / Ignored
−0.02 / …). 1,445 mostly-ignored outcomes drove the live session's trust to the
**0.0 floor**, where it was stuck forever — it could never climb back to the 0.75
T1 threshold even if recent guidance became effective. Replaced it with an **EMA**
toward per-outcome effectiveness anchors:

```
trust ← trust + α·(target(outcome) − trust)
target: Followed=1.0, PartialCompliance=0.6, Ignored=0.2, Contradicted=0.0
```

Now trust **tracks recent effectiveness and recovers**: a floored session climbs
back past 0.75 once guidance is followed; a genuinely all-ignored session
converges to ~0.2 (honestly low, correctly < 0.75 — still kept out of T1). This
is the structural T1 unblocker — it does **not** fake promotion: T1 still requires
trust > 0.75 = genuinely-effective guidance, so actual promotion now awaits the
guidance-relevance work (Option B, the disclosed follow-up). What this sprint
fixes is that trust can *honestly reflect and recover with* effectiveness instead
of being permanently pinned by history.

Config: `JIMINY_TRUST_MODE` (`ema` default, `ratchet` rollback) +
`JIMINY_TRUST_EMA_ALPHA` (0.1). Forward-only — existing Neo4j `J17TrustState`
scores self-heal toward their recent regime.

## Testing
- **Tier 1:** 7 EMA unit tests — recovers-off-floor (0.04 → past 0.75 under a
  Followed run), steady-ignored-converges-to-~0.2-not-0, tracks-recent-regime,
  ema-is-default; legacy ratchet tests pinned to `Mode:ratchet` (both modes
  covered). Build + config-scanner (690/690) + lint clean.
- **Tier 3 (live):** restarted the real server with EMA (default mode); the live
  session `46583515-…` — pinned at `score=0.0` for **1,445** feedbacks by the old
  ratchet — was sent **one real `Followed`** through the live `POST
  /v1/jiminy/feedback` wire (fresh guidance_id `o8kxsuin…`, explicit outcome).
  Trust moved **0.0 → 0.10** (`feedback_count` 1445 → 1446), observed in live
  Neo4j `J17TrustState`. That value is the unmistakable EMA signature:
  `0 + α·(target−score) = 0 + 0.1·(1.0−0) = 0.10` — the previously-floored session
  is now **off the floor and recovering** on the real wire. (The old ratchet would
  have applied its fixed `+0.05` boost, or kept a clamped-at-0 session pinned.)

## Carried forward
- **Option B — guidance relevance/actionability** (raising the real ~24%
  effectiveness so trust climbs past 0.75 and T1 actually engages): a retrieval-
  quality effort, its own line.
- doc-currency-001 (trigger-only + dead-path addenda restored).

## Documents Accessed
The two guidance investigations; `internal/jiminy/{trust,service}.go`;
`internal/config/config.go`; `internal/api/server.go`; live `J17TrustState`
(score 0.0 / 1,445 feedbacks) + TSDB `constraint_outcomes` (confidence 0.7–0.9,
LLM-classified, ~79% ignored).
