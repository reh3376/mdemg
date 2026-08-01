# LEVER-C-TIGHTEN-001 — Sprint Plan

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Precedent:** JIMINY-CORPUS-001 (Lever C on-by-default),
JIMINY-ACTIONABILITY-INVERSION-001 (over-surfacing = dominant driver),
JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (denominator-shrink shape).
**Trigger:** Operator correction 2026-08-01 — 25.4% J17 avg trust +
~14% actionable-compliance are substrate-quality problems, not "harmless-by-design"
calibration artifacts. Trust reflects the receiver's confidence in guidance
producer; low trust = we're producing untrustworthy data → must be fixed.

## 1. Header & Metadata

Line: LEVER-C-TIGHTEN. Sprint: 001. Feature doc target:
`docs/features/jiminy-actionability.md` §Lever-C-Tighten.

## 2. Problem Statement

Live mdemg-dev: J17 avg trust 0.248, actionable-compliance rate ~14%,
Lever C surfaces actionables at 22.7 events/unique constraint_id (vs 9.9
advisory — 2.3× denominator inflation per JIMINY-ACTIONABILITY-INVERSION-001).
Downstream `constraint_outcomes` 7d: 704 actionable rows, 0% follow-rate
below sim 0.40 (257/704 = 37% pure denominator noise). Live knobs
`TOPK=5, SIM_FLOOR=0.70` in `.env`; code defaults `5/0.30`. The `.env`
sim floor is already conservative but the combination causes routine
quota starvation → over-reliance on cooldown-fallback which re-surfaces
recently-ignored items.

## 3. Scope & Constraints

- **In scope**: two config knobs + code defaults + `.env` values + startup log + tests.
- **Out of scope**: Lever A/B tuning; concrete-recall siblings; effectiveness-prior weight; substrate-side classifier rework; URL overrides (deferred as measurement scaffolding).
- **Constraints**: no ULTS hash change (Go control-flow only); no TSDB schema change; no retrieval cache invalidation (audit-confirmed safe — `scorerVersion` excludes JiminyGuidance* knobs).

## 4. Dependencies

Shipped: JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 + DASHBOARD-TRUTH-003.
None blocking.

## 5. Implementation Plan

- **E1** — Config: TOPK default 5→4, SIM_FLOOR default 0.30→0.45. Struct comments reference LEVER-C-TIGHTEN-001 rationale.
- **E2** — `.env`: TOPK 5→4, SIM_FLOOR 0.70→0.55. `.env.example` recommended block updated with data-decided values.
- **E3** — Startup log: `slog.Info("jiminy: lever c actionable bias", "enabled", "topk", "sim_floor")` in `internal/api/server.go` after Lever C boot.
- **E4** — Test: `TestLeverC_ShippedDefaults` pins new defaults; will fail loudly if a future config bump silently regresses them.
- **E5** — Feature doc: append §Lever-C-Tighten to `docs/features/jiminy-actionability.md` (values, A/B protocol, tripwires, load-bearing risk).
- **E6** — CLAUDE.md pin + CHANGELOG entry.

Deferred as follow-up:
- URL overrides (`?leverc_topk`/`?leverc_floor`) — measurement scaffolding, not required for substrate change. Add if passive TSDB A/B proves insufficient.
- CLAUDE.md drift fix: JIMINY-CORPUS-001 note says `TOPK=8/SIM_FLOOR=0.30` but shipped state is TOPK=5/SIM_FLOOR=0.70 → 4/0.55 now. Corrected in this sprint's own CLAUDE.md pin.

## 6. Testing Plan (3 tiers)

- **Tier 1 unit**: `TestLeverC_ShippedDefaults` — pin defaults post-Setenv-clear; `TestFetchActionableCandidates_Guards` (existing) still passes.
- **Tier 2 integration**: build + `go test ./internal/jiminy/` full suite.
- **Tier 3 live e2e**: rebuild binary + kickstart → verify startup log emits `topk=4 sim_floor=0.55` (with `.env` values) or the equivalent from code defaults. Send a sample `/v1/jiminy/warm` request; verify `debug.leverc_actionable_merged` count ≤ 4.

## 7. Commit Strategy

Single commit. All changes cohesive around the two knob tightenings + observability. No cross-cutting refactor.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./internal/jiminy/ ./internal/config/ -count=1` all green
- [ ] `golangci-lint run` clean on modified packages
- [ ] Server restart shows `jiminy: lever c actionable bias enabled=true topk=4 sim_floor=0.55` in the boot log
- [ ] Sample /v1/jiminy/warm call returns `leverc_actionable_merged <= 4`

## 9. Documentation Update

- `docs/features/jiminy-actionability.md` — new §Lever-C-Tighten
- CLAUDE.md — new sprint pin under JIMINY line + rules pinned
- CHANGELOG.md — user-visible tightening entry

## 10. Risks & Mitigations

- **R1**: 0.55 sim floor + TOPK=4 combo starves quota during LLM-saturation. Mitigation: cooldown-fallback path (existing) releases cooled actionables when quota unmet. Tripwire monitors `mdemg_jiminy_surfaced_actionable_fraction < 0.20 for 6h`.
- **R2**: A/B run during drill window contaminates measurement (LLM-HEALTH-CANCELLATION-ALERT-001 precedent). Mitigation: exclude drill-tagged windows in verdict SQL; require quiet-window measurement.
- **R3**: `GetConstraintEffectiveness` (RSIC prune signal) accumulates ~20% slower. Not blocking — signal isn't lost, just slower. Follow-up sprint if RSIC prune velocity degrades over 30d.

## 11. Rollback

Single-commit revert (config defaults + `.env` + `.env.example`). Startup log stays (harmless).

## 12. Documents Accessed

- Workflow output: `wf_e576f7f8-625` (5 investigators + synthesizer)
- `internal/jiminy/service.go` (Lever C dispatch + downstream pipeline)
- `internal/config/config.go` (JIMINY_GUIDANCE_CONSTRAINT_* knobs)
- `internal/jiminy/trust.go` (trust EMA math)
- `internal/ape/task_dispatch.go` (GetConstraintEffectiveness — load-bearing consumer)
- 7d TSDB query on `constraint_outcomes` (mdemg-dev)
- CLAUDE.md sections: JIMINY-CORPUS-001, JIMINY-ACTIONABILITY-INVERSION-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, J17-TIER-GATE-001, DASHBOARD-TRUTH-003
- `.env`, `.env.example`
