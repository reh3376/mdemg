> ⚠️ **TRAJECTORY ANNOTATION (added 2026-08-12 by FRAMING-HYGIENE-SWEEP-001):** the framing in this doc calls the current follow-rate "honest steady state" / "not urgent" / "expected". That framing was **superseded** by the operator directive of 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure") — the arc that owns the ≥80% target is [`docs/development/jiminy-ceiling-break-2/`](../jiminy-ceiling-break-2/README.md). Sprint history preserved below for context; do NOT act on the "not urgent" / "by design" conclusions in the body — those conclusions are wrong per the current architectural directive.

---

# JIMINY-HEURISTIC-DEFAULT-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-HEURISTIC-DEFAULT-001 (Lever A of `jiminy-follow-rate-decline-2026-08-10`)
- **Date:** 2026-08-10
- **Branch:** `reh3376_dev01`
- **Effort:** ~1 hour (single-line fix + alert floor + panel description + pin test + live smoke)

## 2. Problem Statement

The `mdemg_jiminy_follow_rate` gauge is inflated whenever the heuristic classifier fires. `outcome_classifier.go:359` defaults the heuristic verdict to `OutcomePartialCompliance` (0.5 credit in the gauge formula) when the LLM returned `OutcomeUnknown` (parse fail, timeout, refusal, circuit-breaker open) — 100% of heuristic verdicts land as partial_compliance, vs <1% for the LLM classifier. When LLM stability improves, heuristic share drops, and the gauge deflates — a signal INVERSELY coupled to classifier health. The 08-06 → 08-10 decline from 24.39% → 16.42% reads to operators as "substrate quality declined" when in fact it's "classifier got healthier and inflation stopped." See INVESTIGATION.md in the sibling sprint dir.

## 3. Scope & Constraints

**In-scope:**
- Change heuristic default from `OutcomePartialCompliance` to `OutcomeIgnored` (single line at `outcome_classifier.go:359`).
- Lower `JiminyFollowRateAlertFloor` default 0.15 → 0.05 to sit below the honest actionable steady state (~11-12%).
- Update `Follow Rate` panel description on `mdemg-jiminy.json` to explain the shift + honest range.
- Sync staged embed via `make sync-grafana-embed`.
- Pin test for the heuristic default.
- Live Tier-3 smoke.
- Sprint dir + post.md + CLAUDE.md pin + CHANGELOG.

**Out-of-scope:**
- Historical rewrite of `constraint_outcomes` rows (they're honest snapshots of past classifier state).
- Lever B: split `heuristic_unparseable` from `heuristic_similarity_only` source labels (defer to a follow-up after post-Lever-A steady state is visible).
- Lever C: model swap (Qwen3-14B → newer). Post-beta-release track.
- Touching the LLM classifier prompt to increase its ~1% partial_compliance rate (separate concern — LLM might be correctly assessing that few cases are truly partial; changing this without data is speculation).

## 4. Dependencies

- The follow-rate decline investigation (`docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md`) — root cause + fix rationale.
- JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — sibling alert-floor recalibration precedent (`GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` = 0.05 was set below the ~13% actionable steady state per the same measurement-first rule).
- FOLLOW-RATE-CALIBRATE-001 — the extraction of `JiminyFollowRateAlertFloor` from a hardcoded value to a configurable knob.
- LLM-HEALTH-CANCELLATION-ALERT-001 + LLM-HEALTH-INVESTIGATION-001 — the LLM stability improvements that turned the heuristic-inflation into an observable measurement bug.

## 5. Implementation Plan (sequential)

**E1 — code change (`internal/jiminy/outcome_classifier.go:359`):**
- Change the final heuristic-fallback branch from `OutcomePartialCompliance` to `OutcomeIgnored`.
- Add an inline comment naming this sprint + explaining WHY: "when the classifier doesn't know the outcome, defaulting to a half-credit verdict inflates the follow-rate gauge; defaulting to ignored is conservative — classifier improvements EARN credit rather than defaulting to it."

**E2 — alert floor (`internal/config/config.go`):**
- Lower `JiminyFollowRateAlertFloor` default from 0.15 to 0.05 to sit below the honest actionable steady state (~11-12%). Same shape as `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR = 0.05` per JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 E6 recalibration.
- Update the field's doc comment to reference this sprint.

**E3 — panel description (`deploy/docker/grafana/dashboards/mdemg-jiminy.json`):**
- `Follow Rate` panel description gains a note: previously inflated by heuristic-fallback's default partial_compliance; post-JIMINY-HEURISTIC-DEFAULT-001 the honest floor is ~11-12% (matches actionable steady state). Green threshold recalibrated.
- Update panel title to `Follow Rate (honest ~12% by design)` to embed the recalibrated target (DASHBOARD-TRUTH-002 title-embedding rule).
- `make sync-grafana-embed` to mirror to `internal/cli/grafana_templates/staged/`.

**E4 — pin test (`internal/jiminy/outcome_classifier_test.go` or a new `_default_test.go`):**
- Pin: with LLM disabled, a moderate-similarity + no-negation item classifies as `OutcomeIgnored` NOT `OutcomePartialCompliance`. Direct regression pin.

**E5 — live Tier-3 smoke on mdemg-dev:**
- Rebuild + restart mdemg via kickstart.
- Boot log confirms `JiminyFollowRateAlertFloor=0.05`.
- Force a heuristic verdict via a synthesized guidance item (or observe naturally by tracking the next `heuristic` verdict in constraint_outcomes) — confirm it lands with `outcome_type=ignored`.
- Wait 90+ seconds for the follow-rate gauge to publish; confirm the value dropped from ~16% into the honest ~11-12% range (may take longer for the full 168h window to shed all prior inflation).

**E6 — docs (this dir + CLAUDE.md pin + CHANGELOG).**

## 6. Testing Plan

**Unit (T1):**
- New pin in `internal/jiminy/outcome_classifier_test.go`: heuristic-fallback with no negation + sub-highThreshold similarity returns `OutcomeIgnored`.

**Integration (T2):**
- Existing `TestClassify_*` tests continue to pass (no LLM enabled + default sim behavior).

**Live (T3):**
- Post-restart boot log confirms alert floor + gauge value publishes.
- 90-min post-restart gauge trend shows deflation into ~11-13% range.
- `mdemg_jiminy_follow_rate` alert `jiminy_follow_rate_drop` does NOT fire (floor 0.05, gauge >> 0.05).
- New `heuristic`-source rows in `constraint_outcomes` land with `outcome_type='ignored'`.

## 7. Commit Strategy

Single commit: `fix(jiminy): heuristic classifier defaults to ignored not partial_compliance (JIMINY-HEURISTIC-DEFAULT-001)`.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/jiminy/...` = 0 issues
- [ ] `go test ./internal/jiminy/...` green including new pin
- [ ] `make verify-grafana-embed` clean (staged mirror in sync)
- [ ] Boot log confirms `JiminyFollowRateAlertFloor=0.05`
- [ ] Live gauge value trends into honest ~11-13% range within 24-48 hours (7d window rolls off)
- [ ] `constraint_outcomes` new heuristic-source rows land as `ignored`
- [ ] `jiminy_follow_rate_drop` alert does not flap
- [ ] Panel description + title updated in both source + staged JSON
- [ ] CHANGELOG entry
- [ ] CLAUDE.md pin

## 9. Risks & Mitigations

**R1: Gauge drop looks like a regression to operators.** Panel title + description embed the design-intent number (per DASHBOARD-TRUTH-002 rule). CHANGELOG names the sprint + INVESTIGATION.md link.

**R2: Alert floor 0.05 too low → misses real collapse.** Steady state on the honest signal is ~11-12%; 0.05 gives ~7pp headroom before an alarm. Matches sibling `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR=0.05` (already validated at that gate).

**R3: Follow-up sprint discovers the LLM under-classifies partial_compliance (i.e. the ~1% rate is wrong).** Separate concern; Lever A stops the CURRENT gauge bias regardless. If LLM prompt-tightening later increases legitimate partial_compliance detection, the gauge will move UP honestly, not INFLATE.

**R4: Change breaks a downstream consumer that reads the raw follow-rate gauge.** Grep-checked: only 4 consumers of `mdemg_jiminy_follow_rate` — the 2 dashboard panels (updated here), the alert rule (floor lowered here), and the RSIC health-formula (which pulls from `stats.FollowRate` — flows through `applyHonestFollowRate` → same TSDB query → same fix applies naturally).

## 10. Rollback Procedures

Single-file revert of `internal/jiminy/outcome_classifier.go` restores the old default. Alert floor + panel description are additional revertible commits. No schema/migration/DB state.

## 11. Documents Accessed

- `docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md`
- `internal/jiminy/outcome_classifier.go` (line 359 — the fix site)
- `internal/config/config.go` (JiminyFollowRateAlertFloor default)
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (Follow Rate panel)
- `internal/cli/grafana_templates/staged/dashboards/mdemg-jiminy.json` (embed mirror)
- CLAUDE.md pins: JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, FOLLOW-RATE-CALIBRATE-001, DASHBOARD-TRUTH-002
