# PHASE-4B-GATE-DECISION-001 — Sprint Plan

**Task**: #159
**Type**: gate decision — data-only investigation
**Wall-clock**: ~1h
**Trigger**: JIMINY-METRIC-PARTITION-001 (#158) shipped the honest classifier follow-rate gauge 2026-09-10. Path 4+1 T+168h re-measure window (2026-09-13) not yet reached but honest per-class data available NOW.

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | PHASE-4B-GATE-DECISION-001 |
| Task # | #159 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | ZERO — read-only investigation |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E (gate rule: retrain only if classifier follow rate <70%) |
| Blocks | Phase 4b retrain (large compute commitment) |

## 2. Problem Statement

The Path 3 shipping sprint (#158) reported `mdemg_jiminy_follow_rate_classifier_verifiable = 0.252` at ship time — but that gauge read the shipped column DEFAULT (all pre-#158 rows tagged 'classifier') rather than the honest node-classified rows. Before committing to Phase 4b retrain (30-100h compute), reconcile the honest classifier follow-rate signal — using node-level classification retroactively applied via CASE join to historical rows.

## 3. Scope & Constraints

**In scope**:
- Confirm zero organic outcomes flowed post-#158 (server has been quiet)
- Compute honest per-class rates over 168h and 720h using CASE-based JOIN from the 33-code seed
- Apply the gate math: retrain only if classifier follow rate < 70%
- Recommend Phase 4b decision (proceed / defer / alt-path)

**Out of scope**:
- Substrate mutation (no historical row backfill via UPDATE — the CASE at aggregation is cleaner)
- Kicking off retrain (that's the sprint AFTER this gate decision)
- Retraining scoping details

## 4. Dependencies

- Live TSDB `mdemg_metrics.constraint_outcomes` with V0035 `verifiability_class` column shipped by #158
- Static seed from `internal/cli/jiminy_constraint_class.go::jiminySeedClassMap`

## 5. Implementation Plan

Single-flight investigation:
1. Query row-count split pre vs post-#158 (D1-D5)
2. Apply CASE-mapping to derive honest per-class rates over 168h + 720h (D6-D7)
3. Compare vs gate threshold (70% classifier)
4. Analyze cross-class band to identify whether ceiling is classifier-quality-bound
5. Verdict + recommendation

## 6. Testing Plan

Design-only sprint — reproducibility via verbatim SQL in `verdict.md`.

## 7. Commit Strategy

Single commit with sprint dir + verdict + CHANGELOG entry.

## 8. Verification Checklist

- [x] Row-count split confirmed (pre vs post-#158)
- [x] Per-class rates computed at 168h + 720h
- [x] Gate math applied
- [x] Verdict rendered
- [x] Sprint dir + CHANGELOG

## 9. Documentation Update

- `docs/development/phase-4b-gate-decision-001/{sprint_plan,verdict}.md`
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

Zero — read-only investigation.

## 11. Rollback

N/A.

## 12. Documents Accessed

See verdict.md § Documents Accessed.
