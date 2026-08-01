# LEVER-C-TIGHTEN-001 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Trigger:** Operator correction 2026-08-01 — trust score 25.4% is NOT
harmless-by-design. Trust is the receiver-side view of substrate quality;
low trust means the substrate is producing untrustworthy guidance. Reframe:
J17 avg trust 25.4% and Jiminy actionable-compliance ~14% are the same
substrate-quality signal seen from producer + receiver sides. Both must be
addressed by raising the surface's signal-to-noise, not by calibrating
dashboards to render the low numbers as acceptable.

## Verdict

**Shipped.** Lever C tightened from live `TOPK=5, SIM_FLOOR=0.70` (`.env`
values; code defaults were `5, 0.30`) to code defaults `4, 0.45` + `.env`
values `4, 0.55`. Live Tier-3 immediately post-restart: `leverc_actionable_merged=4`
(exactly the new TOPK, was 5 pre-sprint), 4 of 6 surfaced items typed
actionable → 67% actionable share on first sample (vs 30% quota + 34%
7d-baseline mean).

## Method

7-agent read-only deep-dive workflow (`wf_e576f7f8-625`, 6 agents, 234s,
778k tokens) produced a shipping-grade sprint plan with concrete data-decided
values, testing plan, downstream mitigations, and loose-ends checklist.

## What shipped

### Config (E1)
- `internal/config/config.go`: `JiminyGuidanceConstraintIncludeTopK` default **5→4**; `JiminyGuidanceConstraintSimFloor` default **0.30→0.45**. Struct comments cite the sprint + 7d TSDB reasoning.
- `.env`: `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK=5→4`; `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR=0.70→0.55` (relaxed from over-tight 0.70 back toward the code default direction — the 0.70 was starving Lever C, forcing over-reliance on cooldown-fallback which re-surfaces recently-ignored items).
- `.env.example`: recommended block updated with data-decided values + rationale comment.

### Observability (E3)
- `internal/api/server.go`: startup log `slog.Info("jiminy: lever c actionable bias", "enabled", "topk", "sim_floor")` fires at boot when Lever C enabled. Grepable, no hidden state.
- Live boot log line verified: `time=2026-08-01T11:36:30.437-04:00 level=INFO msg="jiminy: lever c actionable bias" enabled=true topk=4 sim_floor=0.55`

### Tests (E4)
- New pin `TestLeverC_ShippedDefaults` in `internal/jiminy/actionability_test.go` — Setenv-clears the two knobs to force code-default read, asserts the shipped 4/0.45. Any future silent regression fails loudly.
- Existing `TestFetchActionableCandidates_Guards` still passes (no coupling to the defaults).

### Live Tier-3 verification
- Restart mdemg via `launchctl kickstart` → `/healthz` green in ~4s.
- Startup log confirmed above.
- `POST /v1/jiminy/warm` → wait ~60s for warm compute → `GET /v1/jiminy/latest` returned `debug.leverc_actionable_merged=4` and `guidance_count=6` (4 actionable + 2 abstraction). Was 5-item merge pre-sprint.

## Data-decided values (from the deep-dive)

| Knob | Before (.env) | After (.env) | Code default before → after | Basis |
|------|---------------|--------------|-----------------------------|-------|
| TOPK | 5 | **4** | 5 → **4** | topk-sensitivity investigator: live actionable_fraction mean 0.342 vs 0.30 quota → TOPK=5 over-supplying with ~13% headroom; TOPK=4 gives ~2.7 expected survivors at 32% attrition, quota-safe via cooldown-fallback |
| SIM_FLOOR | 0.70 | **0.55** | 0.30 → **0.45** | sim-distribution investigator: 7d TSDB, 0/257 followed below downstream-sim 0.40; all 78 followed events ≥0.50 (77 ≥0.60). Surface-time sim runs systematically lower → 0.45 code default kills noise tail with margin; `.env` 0.70→0.55 restores Lever C supply headroom (was quota-starving) |

## Deferred (intentional)

- **URL overrides** (`?leverc_topk`/`?leverc_floor`): measurement scaffolding, not required for the substrate change. Adds handler-parse + ctx-thread + Guide-read glue. Defer unless passive TSDB A/B proves insufficient at T+7d verdict.

## Load-bearing downstream — watch-item

`GetConstraintEffectiveness` (`internal/ape/task_dispatch.go:736,764`) is the RSIC per-constraint prune signal that decides which constraints to tombstone. Actionable `GUIDANCE_OUTCOME` edges are the majority producer (~100/day at baseline). A ~20% surface volume cut = ~20% slower samples-to-confidence per constraint. **The signal isn't lost; it just accumulates slower.** If RSIC prune velocity measurably degrades in 30d, follow-up sprint bumps `RSIC_GUIDANCE_MIN_SURFACES` or similar.

Disclosed in this sprint's docs; not blocking.

## Revert tripwires (7d verdict window)

Single-commit revert if any tripped:
- Actionable-compliance rate **falls** below 0.10 (was ~14%)
- `mdemg_jiminy_surfaced_actionable_fraction < 0.20` for 6h continuously (quota starving)
- Surface-cooldown fallback rate > 30% of surfaces (indicates TOPK too tight)

## Rules pinned

⚠️ **Trust score is a receiver-side quality signal, NOT observability-only.** Even when `J17_TIER_GATE_MODE=comprehension` (J17-TIER-GATE-001) bypasses trust for tier selection, a low trust EMA still means the substrate is producing untrustworthy guidance. **Do not frame low trust as "harmless-by-design."** The operator correction of 2026-08-01 is durable — dashboard-honesty work must not conflate "the number is accurate" with "the situation is fine." Both must be addressed.

⚠️ **When tightening a "shrink the denominator"-shape knob, enumerate consumers of the downstream signal density, not just the volume itself.** Lever C's tightening reduces surface volume by ~20%; the load-bearing consumer isn't surface count but `GUIDANCE_OUTCOME` edge accumulation rate feeding `GetConstraintEffectiveness`. This class of risk must be disclosed even when not blocking.

⚠️ **CLAUDE.md drift caught + corrected inline**: the JIMINY-CORPUS-001 note claimed shipped `TOPK=8/SIM_FLOOR=0.30` but actual shipped state was `5/0.70`. Note is now corrected via this sprint's pin. General rule: when a sprint changes a config value that a prior sprint's CLAUDE.md pin references, verify the pin against the actual live state — pins drift silently when subsequent sprints tune the knob without updating the reference.

## Follow-ups disclosed

1. **T+7d passive A/B verdict** — measure actionable-compliance-rate delta + trust EMA slope + surface volume. Success criterion: any positive lift + non-negative trust slope. Revert if any tripwire above trips.
2. **URL overrides** (deferred) — add if the T+7d passive measurement is inconclusive and paired A/B is needed.
3. **`GetConstraintEffectiveness` samples-to-confidence latency check** — 30d observability review of RSIC prune velocity vs pre-sprint baseline.
4. **Corpus-quality substrate work** — if trust EMA does NOT recover to the 0.40+ range even after Lever C tightening, the residual gap is corpus-quality (the actual content of surfaced items). That's the JIMINY-CORPUS-003 / HITL-CURATION-ACCEL lever, not this sprint's scope.

## Documents Accessed

- Deep-dive workflow output: `wf_e576f7f8-625` (6 agents, 234s)
- `internal/jiminy/service.go` (Lever C dispatch, downstream pipeline)
- `internal/jiminy/trust.go` (EMA math)
- `internal/config/config.go` (JIMINY_GUIDANCE_CONSTRAINT_* knobs)
- `internal/api/server.go` (server init, startup log)
- `internal/ape/task_dispatch.go` (GetConstraintEffectiveness — load-bearing consumer)
- `internal/jiminy/actionability_test.go` (pin test addition)
- Live TSDB queries on `mdemg_metrics.constraint_outcomes` (mdemg-dev, 7d + 30d windows)
- CLAUDE.md: JIMINY-CORPUS-001, JIMINY-ACTIONABILITY-INVERSION-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, J17-TIER-GATE-001, DASHBOARD-TRUTH-003
- `.env`, `.env.example`, `docs/features/jiminy-actionability.md`
