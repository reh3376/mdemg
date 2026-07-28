# CLASSIFIER-CONSISTENCY-001 — Sprint Plan (Reduced Scope)

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive Finding 2 (25% heuristic in
`constraint_outcomes` vs 0.03% in `guidance_training_rows`).
**Investigation reframe:** the deep-dive's framing was correct-in-symptom,
wrong-in-cause. Fresh data disproves the "chronic" premise and reduces the
sprint to a durable-prevention alert rule.

## 1. Header & Metadata

Ship a `HeuristicShareRule` alert that fires when
`constraint_outcomes.classifier_source = 'heuristic'` exceeds a threshold
fraction over the last N hours — durable prevention against the LLM-
saturation burst pattern this sprint's investigation surfaced. Effort ~half
a day. Risk low (single rule addition following the shipped ORPHAN-ALERT /
NODE-DROP-CALIBRATION / HITL-CURATION-002-E4 pattern; no substrate touch).

## 2. Problem Statement (revised from investigation)

The Q4 deep-dive found `constraint_outcomes` at 25% heuristic and framed it
as a chronic measurement-integrity issue. **10-day breakdown investigation
reveals a different picture:**

| Day | Total | Heuristic | Heur % |
|---|---|---|---|
| 2026-07-28 | 12 | 0 | 0.00% |
| 2026-07-27 | 305 | 2 | 0.66% |
| 2026-07-24 | 117 | 0 | 0.00% |
| 2026-07-23 | 177 | 20 | 11.30% |
| 2026-07-22 | 87 | 0 | 0.00% |
| **2026-07-21** | **1272** | **451** | **35.46%** |
| 2026-07-20 | 502 | 0 | 0.00% |

The 25% headline was dominated by ONE 2026-07-21 burst (451 of 473 heuristic
rows in the deep-dive's 7d window). That day, 8 sprints shipped
simultaneously (APE-PROMPT-BUDGET, APE-REFLECT-EVAL-REFRESH, J17-TIER-GATE,
FT-BENCH-REFRESH, LLM-HEALTH-CANCELLATION-ALERT, DASHBOARD-TRUTH-002, +2).
LLM saturation → cancellations → jiminy classifier's heuristic fallback
path fired for ~35% of outcomes. Baseline is 0-1%; **the pattern is
saturation-driven and transient**, not chronic.

**Root-cause finding (also from investigation):** BOTH `constraint_outcomes`
and `guidance_training_rows` writers use the SAME `cr.Source` from ONE
`Service.processOutcome()` classifier call. Fresh writes to both tables are
byte-identical on the classifier_source field. The 25% vs 0.03% split
comes ENTIRELY from GUIDANCE-AUDIT-001's asymmetric retroactive relabeling —
audit fires on training_rows only, because the retrain pipeline consumes
training_rows and outcomes is telemetry-only. **The "two paths measuring
different things" framing was wrong; the design was intentional.**

**What the deep-dive was RIGHT about:** the class of failure (a stub
classifier accepting outputs during LLM saturation) is real and will
recur on the next multi-sprint day. A durable-prevention alert is genuine
value even though the current-state measurement issue has resolved.

## 3. Scope & Constraints

**In scope (one epic):**

- `alert.HeuristicShareRule(threshold float64, lookbackHours int)` in
  `internal/alert/rules.go` — fires when
  `count(*) FILTER (WHERE classifier_source='heuristic') / count(*)`
  exceeds `threshold` over `lookbackHours` on `constraint_outcomes`
- Config: `HEURISTIC_SHARE_THRESHOLD` (default 0.05 = 5%),
  `HEURISTIC_SHARE_LOOKBACK_HOURS` (default 24)
- Distinct Service `heuristic-share` (NOSILENT-001 cooldown-key)
- Severity MEDIUM, ForDuration 6h (won't flap on a moderate burst)
- COALESCE(...,0) idle-safe (TSDB-CONSUME-001)
- Wired in `serve.go` alongside the other parameterized rules
- 2 unit tests (defaults + custom params) + auto sweep coverage
- Feature note (short — no separate feature doc; this is one rule)
- CHANGELOG entry + CLAUDE.md architectural note (short — pins the
  investigation-reframe lesson)

**Out of scope:**

- Extending GUIDANCE-AUDIT to `constraint_outcomes` (option (b) in the
  operator's discussion — deferred as marginal-value, deferrable-until-
  triggered)
- Investigating WHY the classifier's heuristic fallback fires so often
  under saturation (root cause is LLM-HEALTH-CANCELLATION-ALERT-001's
  class; this alert is the observability layer for it, not the fix)
- New feature doc file (a one-rule sprint doesn't need its own feature
  page — CLAUDE.md pin + CHANGELOG suffice)

## 4. Dependencies

- ORPHAN-ALERT-001, NODE-DROP-CALIBRATION-001, HITL-CURATION-002-E4:
  the alert-rule pattern being mirrored
- LLM-HEALTH-CANCELLATION-ALERT-001: the shipped fix for the class this
  rule detects (this sprint adds the durable observability)
- TSDB-CONSUME-001: idle-safe alert-SQL contract
- NOSILENT-001: distinct-Service-per-severity cooldown-key contract

## 5. Implementation Plan (single epic)

**E1 — `HeuristicShareRule` + config + serve wire + tests + docs**

- `alert.HeuristicShareRule(threshold, lookbackHours)` factory in
  `internal/alert/rules.go`
- Query: `SELECT COALESCE(count(*) FILTER (WHERE classifier_source='heuristic')::float / NULLIF(count(*), 0), 0) AS heur_share FROM constraint_outcomes WHERE time > now() - interval '${N} hours'`
- Threshold as float, Operator `gt`
- Config fields + FromEnv reads + struct-literal assignment
- Wire in `serve.go` next to `HITLCurationStalledRule` call
- Extend `allRules()` walker (auto-covers `TestAllRules_NoLimitOneAntiPattern`
  + `TestAllRules_DistinctServicePerSeverity`)
- 2 targeted unit tests (defaults + custom params)
- CHANGELOG entry under `### Added`
- CLAUDE.md architectural note: pins the investigation-reframe rule
  ("burst-vs-chronic distinction — a deep-dive can be right about the
  class of failure while wrong about the tempo; check for burst-vs-baseline
  before shipping a chronic-issue fix")

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `TestHeuristicShareRule_Defaults` +
  `TestHeuristicShareRule_CustomParams` — pin service, severity, defaults,
  auto:%-not-needed (no auto/operator distinction here — heuristic is a
  classifier-source label unaffected by autograde), idle-safe SQL contract.
- **Tier 2 (contract):** `TestAllRules_NoLimitOneAntiPattern` +
  `TestAllRules_DistinctServicePerSeverity` sweep the new rule.
  `go build`, `golangci-lint run`, full `go test ./...` green.
- **Tier 3 (live):** direct-query the new rule's SQL against `mdemg-dev`
  `constraint_outcomes` — verify it returns a single non-NULL row + the
  current heuristic share is well below threshold (24h baseline is
  0.63% << 5%). Force-fire by lowering `HEURISTIC_SHARE_THRESHOLD=0.001`
  env override + kickstart; verify rule fires; unset + kickstart to
  restore.

## 7. Commit Strategy

Single commit under the `CLASSIFIER-CONSISTENCY-001` slug (small sprint,
one epic).

## 8. Verification Checklist

- [ ] `HeuristicShareRule` factory + 2 config knobs shipped
- [ ] Distinct Service `heuristic-share` (no collision with existing)
- [ ] SQL uses COALESCE + no LIMIT 1 (TSDB-CONSUME-001 contract)
- [ ] Serve wire alongside other parameterized rules
- [ ] `go build`, `golangci-lint run`, `go test ./...` full green
- [ ] Live query returns single non-NULL row on mdemg-dev
- [ ] Live force-fire test succeeds; env restored
- [ ] CHANGELOG entry + CLAUDE.md architectural note

## 9. Rollback Procedures

Single-rule addition; revert commit fully removes the rule. Config knobs
are default-safe; absent env vars → shipped defaults (0.05, 24). No
schema change, no substrate mutation.

## 10. Risks & Mitigations

- **Risk:** threshold 5% is too tight — flaps during normal moderate
  bursts.
  - **Mitigation:** ForDuration 6h; the 2026-07-21 burst was 35% for the
    whole day, well above any reasonable ForDuration. Real-world bursts
    are heavy-work-days, not brief spikes. Config-tunable if the operator
    experiences flap.
- **Risk:** threshold 5% is too loose — misses moderate bursts.
  - **Mitigation:** operator can lower via env. Current baseline is
    0.63%; a 5x baseline rise IS notable and worth alerting on.
- **Risk:** the rule fires during a legitimate multi-sprint day and
  becomes routine-noise.
  - **Acknowledgment:** correct behavior. A heavy-work day IS worth
    noticing that the classifier's fallback path is being exercised.
    The alert doesn't demand action; it surfaces the fact so the operator
    can decide whether to space work or accept the fallback rate.

## 11. Documents Accessed

Filled in commit message.

## 12. Documentation Update

Final epic — never cut (Sprint Plan Format v1.0). Covered in E1 with
CHANGELOG + CLAUDE.md pin (no separate feature doc for a one-rule sprint).
