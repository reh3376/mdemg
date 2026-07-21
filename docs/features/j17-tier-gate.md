# J17 Tier Gate — Comprehension-Keyed T1 Promotion

**Sprint**: J17-TIER-GATE-001 (2026-07-21)
**Status**: shipped; `comprehension` mode live on mdemg-dev (code default `trust`)

## Why

J17's purpose is token-efficient inter-model communication (T1 ≈ 5.2× compression). T1 promotion historically required session trust > 0.75 — but trust is a compliance EMA that converges to ~0.2-0.3 under a relevance-driven-ignore regime, making T1 unreachable for months and capping compression at T2's ~2×. Comprehension — the axis dense encoding actually risks — is measured separately (NLI + heuristic, per-tier + per-code) and already had a T1→T2 demotion gate. This sprint keys PROMOTION on comprehension.

## How it works

`selectTier` (encoder.go) in `comprehension` mode: coded item + overall AvgComprehension ≥ floor → T1; coded → T2; uncoded + above floor → T2; else T3. Falls back to legacy trust logic when in-process samples < MIN_SAMPLES or no provider. Demotion gate unchanged.

## Config

| Env | Default | Meaning |
|---|---|---|
| `J17_TIER_GATE_MODE` | `trust` | `trust` = legacy byte-identical; `comprehension` = new promotion axis |
| `J17_TIER_COMPREHENSION_HIGH` | 0.6 | Promotion floor (aligned with `J17_TIER_INEFFECTIVE_THRESHOLD`) |
| `J17_TIER_COMPREHENSION_MIN_SAMPLES` | 20 | In-process events before the mode activates (cold-start safety) |

## Operational notes

- **Restart behavior**: protocol stats are in-memory; after a restart T1 re-locks until ~an hour of real feedback traffic rebuilds comprehension ≥ floor. Expected, by design.
- **Boundary flap**: steady-state comprehension (~0.61-0.73) sits near the 0.6 floor. Lower the floor to 0.5 if T1/T2 flapping is observed; the demotion gate protects against genuine incomprehension either way.
- **Compression follow-on**: expect `mdemg_j17_compression_ratio` to climb from ~1.7 toward the `J17_COMPRESSION_TARGET_RATIO` (default 2.0; real achievable J17 compression is ~1.8-3×, per DASHBOARD-TRUTH-001) as T1-encoded guidance accumulates in new protocol events.

## Rollback

`.env` `J17_TIER_GATE_MODE=trust` (or remove) + restart. No data changes.
