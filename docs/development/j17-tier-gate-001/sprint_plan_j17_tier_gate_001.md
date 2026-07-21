# Sprint J17-TIER-GATE-001 — Key T1 promotion on measured comprehension, not compliance-trust

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | J17-TIER-GATE-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~1 dev-day |
| Parent | Operator J17 investigation (2026-07-21): "the entire purpose of J17 is to reduce token counts… 40% T3 / 60% T2 gives little benefit" |

## 2. Problem Statement

T1 (dense coded encoding, ~5.2× compression) requires `hasCode && trust > J17_TRUST_HIGH_THRESHOLD (0.75)` in `encoder.go::selectTier`. Trust is a **compliance** EMA (followed→1.0, ignored→0.2); with the honest ~72%-ignored outcome mix it converges to ~0.2-0.3 — T1 is mathematically unreachable, capping compression at T2's ~2×.

But the risk dense encoding actually guards against is **incomprehension**, which is measured separately (live 48h avg **0.732**, n=3592) and already has its own T1→T2 **demotion** gate (`J17_T1_COMPREHENSION_GATE`, service.go:1249). Gating *promotion* on compliance conflates "understood the message" with "obeyed it" — the exact axis confusion DASHBOARD-TRUTH-001 E2 pinned for the NLI bias alert. An agent at 73% comprehension that ignores guidance for relevance reasons has still EARNED dense encoding by the protocol's own purpose (token efficiency).

## 3. Scope & Constraints

**In scope**: new `J17_TIER_GATE_MODE` config (`trust` default = byte-identical legacy | `comprehension`); encoder comprehension provider wired from `ProtocolMetricsCollector` (overall AvgComprehension + TotalEvents); cold-start fallback to trust mode below `J17_TIER_COMPREHENSION_MIN_SAMPLES`; promotion floor `J17_TIER_COMPREHENSION_HIGH` (default 0.6 = the existing `J17_TIER_INEFFECTIVE_THRESHOLD` anchor; live 0.732 clears); truth-table tests; live Tier-3 with `.env` opt-in (operator-authorized by "run A").
**Out of scope**: per-code comprehension promotion (future refinement); any change to the demotion gate; trust scoring itself; prompt changes (no ULTS hash impact).
**Constraints**: default `trust` mode byte-identical (pin-tested); no hardcoded values; the demotion gate stays active in both modes; shadow-compare path (service.go:1200) inherits the mode automatically (same `selectTier`).

## 4. Dependencies

✅ Comprehension telemetry live (`ProtocolMetricsCollector.Snapshot().AvgComprehension/TotalEvents`); ✅ demotion gate shipped (service.go:1249); ✅ trust thresholds config exists (unchanged); ⚠️ in-memory stats reset on restart → cold-start fallback is required, and live smoke uses a temporarily-lowered MIN_SAMPLES.

## 5. Implementation Plan (sequential)

- **E0** plan (this doc).
- **E1** config: `J17_TIER_GATE_MODE` (default `trust`; invalid → `trust` + WARN), `J17_TIER_COMPREHENSION_HIGH` (0.6; clamp (0,1]), `J17_TIER_COMPREHENSION_MIN_SAMPLES` (20; ≥1).
- **E2** encoder: `SetComprehensionGate(mode, high, minSamples)` + `SetComprehensionProvider(func() (score float64, samples int64))`. `selectTier`: comprehension mode with provider + samples ≥ min → `hasCode && score ≥ high → T1; hasCode → T2; !hasCode && score ≥ high → T2; else T3`; otherwise fall through to legacy trust logic. Service wiring: provider from `protocolMetrics.Snapshot()`; init log gains `tier_gate_mode`.
- **E3** tests: truth table both modes; cold-start fallback; provider-nil fallback; default-mode byte-identical pin.
- **E4** live Tier-3: `.env` `J17_TIER_GATE_MODE=comprehension` + temporary `J17_TIER_COMPREHENSION_MIN_SAMPLES=1` → restart → drive warm/feedback traffic → observe T1 items in `/v1/jiminy/latest` + `tier_t1_fraction > 0` + compression ratio rising; restore MIN_SAMPLES default (keep mode).
- **E5** docs: CHANGELOG, CLAUDE.md, `docs/features/j17-tier-gate.md`, post.md.

## 6. Testing Plan

Tier 1: selectTier truth tables + fallbacks + byte-identical default pin. Tier 2: full `go test ./internal/jiminy/`; existing encoder tests untouched-green. Tier 3: E4 live observation (real guidance traffic, real gauge movement).

## 7. Commit Strategy

`docs(E0)` → `feat(E1+E2+E3)` (one cluster commit, same package) → `docs(E4 live evidence)` → `docs(E5)`.

## 8. Verification Checklist

build/lint/test clean · default-mode pin green · live T1 observed · gauges move · docs complete · pushed.

## 9. Documentation Update

CHANGELOG Fixed/Added; CLAUDE.md J17 note (promotion axis rule); feature doc; sprint post.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Comprehension over-grants T1 → agent misreads dense codes | Med | Demotion gate (T1-specific, min 5 T1 outcomes) auto-downgrades; monitor `TierComprehension[0]` |
| Cold-start zero-data grants/denies wrongly | Low | MIN_SAMPLES fallback to legacy trust logic |
| Overall-avg comprehension masks a bad code | Low | Per-code demotion signals exist (`CodeComprehension`); per-code promotion is the documented future refinement |
| Mode typo in .env | Low | invalid → `trust` + WARN |

## 11. Rollback

`.env` remove/`J17_TIER_GATE_MODE=trust` + restart (no data changes); code revert per-commit.

## 12. Documents Accessed

`internal/jiminy/{encoder.go,service.go,protocol_metrics.go}`; CLAUDE.md §DASHBOARD-TRUTH-001 E2, §/strict (J17_T1_COMPREHENSION_GATE); DASHBOARD-TRUTH-002 J17 triage; operator investigation this session (comprehension 0.732 live; trust 0.23; tier mix).
