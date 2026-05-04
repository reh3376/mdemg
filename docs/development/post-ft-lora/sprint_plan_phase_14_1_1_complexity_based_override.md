# Sprint POST-FT-LORA-PHASE14.1.1 — Complexity-Based Sparse Gate Override

> **STUB — scoped 2026-05-04** as a follow-up to Phase 14.1's failed 120q full A/B. Phase 14.1's per-category override produced 2 catastrophic regressions (q119 -0.45 in `service_relationships`, q333 -0.35 in `data_flow_integration`) — both questions had 3 required_files, in categories Phase 14.1's design didn't override. The signal is not category but **multi-file complexity**. This sprint replaces the per-category override with a complexity-driven one.

## Context

Phase 14.1 Epic 3 verdict (from `phase_14_1_post.md`):

| | Phase 14 (no override) | Phase 14.1 (arch-only override) |
|---|---|---|
| 120q mean Δ | 0.000 | -0.009 |
| Regressions ≥0.10 | 7 (boundary) | 2 (catastrophic) |
| Improvements ≥0.10 | 7 | 6 |
| Verdict | fail (per-question) | fail (mean + per-question) |

The override **slightly improved** the target category (`architecture_structure` -0.015 → -0.010) but exposed catastrophic regressions in 2 categories where Phase 14 had zero. Both new regressions:
- q119 [`service_relationships`]: 3 required_files
- q333 [`data_flow_integration`]: 3 required_files

Phase 14's regressions were predominantly 2-file (5 of 7) with one 3-file. Phase 14.1's regressions are 100% 3-file. The pattern: gate at MIN=10 loses cited files when ≥3 required files spread across rank 1–N+M with small M.

## Hypothesis

**Per-question multi-file complexity** drives the gate's cut-loss probability, not category. Override design:
- Spec-time tag (already present): `complexity ∈ {multi-file, cross-module, system-wide, computed_value, disambiguation, relationship}`
- Or computed: `len(required_files)` as a pre-trip signal
- Override: when `complexity` is `multi-file` or `cross-module` AND `required_files ≥ 3`, use MIN_ACTIVE=20 (effectively no-op for the gate). Otherwise, use global MIN=10.

This subsumes Phase 14.1's per-category dispatch with a more principled cut-line.

## Scope

| # | Deliverable | Path |
|---|---|---|
| 1 | New env knob `SPARSE_GATE_COMPLEXITY_OVERRIDE_THRESHOLD` (default `3` — required_files count above which to apply override) | `internal/config/config.go` |
| 2 | New env knob `SPARSE_GATE_COMPLEXITY_MIN_ACTIVE` (default `20`) | `internal/config/config.go` |
| 3 | New request field `RequiredFilesCount` on `RetrieveRequest` (or `ComplexityTag string`) | `internal/models/models.go` |
| 4 | URL param `?required_files_count=N` parsed in handler | `internal/api/handlers.go` |
| 5 | UVTS runner injects per-question `len(q['required_files'])` | `docs/tests/uvts/runners/uvts_runner.py` |
| 6 | Gate dispatch reads complexity hint, applies override before per-category lookup | `internal/retrieval/gate.go` |
| 7 | Tier 1 unit tests for complexity-driven dispatch | `internal/retrieval/gate_test.go` |
| 8 | A/B sweep with 3 presets (threshold=2, =3, =4) × 16q quick + 120q full on winner | `scripts/phase14_1_1_complexity_runner.py` |
| 9 | Conditional default flip (gate default-on with complexity override) | `internal/config/config.go` |
| 10 | Sprint plan + post + feature doc update | standard 4-doc trail |

## Alternative design (consider during Epic 0)

Instead of a new knob, **raise the global `SPARSE_MIN_ACTIVE` from 3 to 15**:
- Phase 14 16q quick at MIN=10/p95 PASSED
- Phase 14.1 16q quick at MIN=10/p95 + arch-override-to-MIN=20 also PASSED
- A simpler MIN=15 globally might catch all 3-file cases without per-question dispatch

Risk: MIN=15 is less aggressive on rerank-input reduction (~25% vs ~50%). May fail 120q if 4-file questions exist (unlikely; benchmark max is 3).

Decision in Epic 0: ablation runner sweeps both designs (complexity-tagged + global MIN=15) and 120q goes to the winner.

## Estimate

~3 dev-days (same shape as 14.1 — add request field + URL param + gate dispatch + ablation runner + A/B).

## Budget

~$15–25 OpenAI (3 quick presets + 1 full).

## Pre-gate

Phase 14.1 commit landed; V0019 has rows from this sprint's testing for diagnostic baseline.

## Acceptance

120q full A/B vs Phase 13.1 production: B mean ≥ A AND no per-question regression > 10% (with eps tolerance).

If passes → flip `SPARSE_RETRIEVAL_ENABLED=true` default + set complexity-override default in same commit.
If fails → ship complexity-override flag-off-but-wired; re-evaluate gate concept (may not be a fit for this corpus).

## Risks

1. **Both designs fail 120q**: implies the gate concept itself is wrong for the lnl_demo corpus — Phase 14 + 14.1 + 14.1.1 all reach the same conclusion. Decision: keep gate code in tree as opt-in for spaces where it works (e.g. very-large candidate sets); document the lnl_demo non-fit; close Note 06 work.
2. **`required_files` is benchmark-only metadata**: production retrieve calls don't have it. The override only fires during UVTS runs. That's actually fine — the override is for A/B verification; production callers can opt out via empty hint.

## Documents Accessed (during planning)

- `docs/development/post-ft-lora/phase_14_1_post.md` (verdict + diagnosis)
- `docs/development/post-ft-lora/phase_14_1_forensic.md` (Epic 0 forensic)
- `/tmp/phase14_1_full/adaptive-arch-only/verdict.json` (the 2 catastrophic regressions)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (required_files data)
- `docs/features/sparse-retrieval.md`
- `internal/retrieval/gate.go`, `gate_test.go`
