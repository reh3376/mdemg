# Sprint JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — Non-violation credit for must_not in classifier prompt

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 |
| Sprint Name | Route unrelated-context "ignored" verdicts on must_not constraints to "not_applicable" via classifier prompt |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 1-1.5 dev-days (E1-E6 in one pass; A/B window is operator-time not dev-time) |
| Sprint Line | jiminy-actionability-compliance-credit-001 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Fix 2 in JIMINY-ACTIONABILITY-INVERSION-001 fix_spec.md (2026-07-21) |

## 2. Problem Statement

JIMINY-ACTIONABILITY-INVERSION-001 diagnosed the Should-Follow < Follow-Rate inversion as an emergent effect of (constraint semantics) × (Lever C over-surfacing) × (LLM classifier's correct strict-vs-flexible calibration). Root cause is intentional design — but the OUTPUT is misleading: constraint follow rate reads ~10% because Lever C surfaces constraints in unrelated contexts and the LLM correctly classifies non-application-non-violation as `ignored`.

The most defensible fix (Fix 2 in the fix_spec.md, ranked highest leverage / lowest risk of the three) is to extend `classifySystemPrompt` with an explicit rule:

> **Non-violation credit for must_not**: for must_not-type constraints, if the action neither violated the constraint nor plausibly had opportunity to violate it, classify as `not_applicable`, NOT `ignored`. Only use `ignored` when the action clearly could and should have applied the constraint.

Because `constraint_outcomes` already gates out `not_applicable` rows at the writer (verified: `service.go:1730,1762`), routing these to `not_applicable` shrinks the constraint denominator by exactly the class of "surfaced but not-applicable" rows the inversion investigation identified — predicted to lift constraint follow rate from ~10% to ~20%, closing the inversion at the correct root.

Also lands Fix 1 (panel wording reframe: "Actionable Compliance Rate" with description explaining the expected floor).

## 3. Scope & Constraints

**In scope**:
- Extend `classifySystemPrompt` + `classifySystemPromptCompact` in `internal/jiminy/outcome_classifier.go` with the non-violation-credit rule.
- Ship behind default-off config gate `JIMINY_NONVIOLATION_CREDIT_ENABLED` — safe rollout; operator flips after A/B.
- Update ULTS spec `system_prompt_hash` for `jiminy.evaluate_llm` in the same PR (ULTS-CI-001 contract — prompt drift is a merge-blocking check).
- Tier-1 unit tests: canonical "unrelated action + must_not constraint" case classifies as `not_applicable` under the new prompt, `ignored` under the old.
- Live Tier-3 smoke: force-enable the flag, feed 2-3 real actions (an unrelated one + a violating one + a following one), verify verdicts flow correctly.
- Fix 1 panel-wording reframe: rename "Should-Follow" panel to "Actionable Compliance Rate" + updated description.
- Alert rule `guidance_should_follow_rate_low` — either soften threshold to reflect corrected metric OR update its title to match the new panel name.
- Document the 3-day A/B recipe operators run before flipping the default.

**Out of scope**:
- Default-flip. This sprint ships default-off with the flip deferred to a small doc-only sprint after operator runs the 3-day A/B (or the operator can flip in `.env` themselves).
- Fix 3 (reduce Lever C top-K) — plan flagged NOT recommended; still not recommended.
- Constraint classifier's tier-1 short-circuit (embedding-similarity heuristic) — separate concern; this sprint only touches the tier-2 LLM prompt.
- Retraining or LoRA changes — per JIMINY-ACTIONABILITY-INVERSION-001 verdict, LoRA won't help here.

**Constraints**:
- **No hardcoded values.** Config gate + env var.
- **Prompt hash drift = ULTS-CI-001 fail.** MUST update the pinned `system_prompt_hash` in `docs/tests/ults/specs/jiminy_evaluate_llm.ults.json` (or wherever it's pinned) in the same PR.
- **Live Tier-3 required.** Not "the prompt exists in the file"; a real fresh contradicted-inducing call verifies the LLM actually returns `not_applicable` for the unrelated-context case.
- **Default-off ships first.** Never flip default in the same sprint as the code change — that violates the "flag flipped only after live smoke" contract from JIMINY-CONTRADICTED-BRIDGE-001.
- **`constraint_outcomes` gate must stay closed.** Verify the fix DOESN'T add `not_applicable` rows to the table — routing them from the `outcomeWriter` gate check remains the honest behavior.

## 4. Dependencies & Pre-Conditions

- ✅ JIMINY-ACTIONABILITY-INVERSION-001 shipped (fix spec is the input).
- ✅ `classifySystemPrompt` + compact variant exist at `internal/jiminy/outcome_classifier.go:20-58`.
- ✅ `not_applicable` filter at `service.go:1730,1762` verified working (zero `not_applicable` rows in `constraint_outcomes` today).
- ✅ ULTS spec system_prompt_hash pinning is CI-enforced (per HOOKWIRE-001 / ULTS-CI-001).
- ⚠️ Live llama-server must be up + responding for E5 live smoke.

## 5. Implementation Plan

Sequential — never parallelize.

### E0 — Sprint plan
Commit this plan.

### E1 — Baseline stats capture
Record current stats for the A/B baseline (constraint follow-rate + not_applicable-invisible-fraction over the last 7d).

```sql
SELECT guidance_type, outcome_type, COUNT(*)
FROM constraint_outcomes
WHERE space_id='mdemg-dev' AND time > NOW() - INTERVAL '7 days'
  AND guidance_type IN ('constraint','correction')
GROUP BY 1,2 ORDER BY 1,2;
```

Also: count LLM classifier calls with `outcome='not_applicable'` in `llm_interactions` over the same window (this class currently gates OUT of `constraint_outcomes` — measure the pre-fix rate before change).

**Gate**: baseline row counts + rates captured in `docs/development/jiminy-actionability-compliance-credit-001/baseline.md`.

### E2 — Prompt update + config gate
- Add `JIMINY_NONVIOLATION_CREDIT_ENABLED` config (default `false`).
- `classifySystemPrompt` (`internal/jiminy/outcome_classifier.go:20-49`): when the gate is ON, extend the classification rules with the non-violation-credit clause.
- `classifySystemPromptCompact` (line 53-58): parallel extension.
- Gate is checked at prompt-render time (each classify call), NOT at construction — so `mdemg config set JIMINY_NONVIOLATION_CREDIT_ENABLED=true` + restart flips the behavior cleanly.

**Gate**: code changes compile clean; unit test asserts the prompt STRING differs by exactly the new clause when the flag is toggled.

### E3 — Tier-1 unit tests
- `TestClassifySystemPrompt_NonViolationCredit_Disabled`: default-off returns byte-identical HEAD prompt.
- `TestClassifySystemPrompt_NonViolationCredit_Enabled`: flag-on prompt contains the new clause.
- Compact-variant equivalents.
- Optional: an LLM-independent unit test that verifies the prompt renders correctly for both flag states (no mocked LLM — the prompt is a string).

**Gate**: all new tests PASS; existing classifier tests still PASS.

### E4 — ULTS spec update
Update `docs/tests/ults/specs/jiminy_evaluate_llm.ults.json`'s pinned `system_prompt_hash` — the ULTS runner CI check re-computes the hash from `outcome_classifier.go:classifySystemPrompt` (line 20-49) and compares to the pinned value. My E2 change adds text conditionally at prompt-render time, so:

- If the default-off gate is at prompt-render time (recommended): the ULTS spec's default hash is UNCHANGED (the default-off render is byte-identical HEAD).
- If the flag flips the top-level const declaration: the ULTS spec's hash must change.

Recommend the former (default-off render = HEAD) so E4 is a no-op AND the CI drift check doesn't need reworking.

**Gate**: ULTS `--verify-hashes` PASSES; if hash change is needed, the pin is updated in the same commit as E2.

### E5 — Live Tier-3 smoke
- Enable via `launchctl setenv JIMINY_NONVIOLATION_CREDIT_ENABLED true` + `kickstart -k`.
- Manually feed 3 test actions to `/v1/jiminy/feedback`, each targeting a real must_not-type constraint:
  1. Action UNRELATED to the constraint's mechanism (expect `not_applicable`, not `ignored`).
  2. Action that VIOLATES the constraint (expect `contradicted`).
  3. Action that FOLLOWS the constraint (expect `followed`).
- Verify each outcome via `llm_interactions.response` (classifier verdict) and via `constraint_outcomes` (rows land: contradicted/followed rows only; the unrelated-action row correctly filtered by the shipped not_applicable gate).
- Restore default-off.

**Gate**: all 3 verdicts correct; unrelated-action row correctly filtered from `constraint_outcomes`; server logs show the flag toggled between them.

### E6 — Fix 1 panel-wording reframe (bundled)
- Rename `mdemg-jiminy.json` "Should-Follow Follow Rate" panel to "Actionable Compliance Rate".
- Update description: explain that under current Lever C architecture the actionable rate is EXPECTED to be lower than raw follow rate; the >90% target isn't the right frame; use trend-over-14d instead.
- Alert rule `guidance_should_follow_rate_low` — update title to match new panel name; keep threshold semantics.
- 3-day A/B recipe documented for operators.

**Gate**: dashboard JSON validates; alert-rule title update lands.

### E7 — A/B recipe documentation
Write `docs/development/jiminy-actionability-compliance-credit-001/ab_recipe.md` — the exact SQL + CLI operators run to compare pre/post-flag constraint follow rate over a 3-day window.

### E8 — Canonical docs
- CHANGELOG [Unreleased] > Fixed.
- CLAUDE.md: extend the JIMINY-ACTIONABILITY-INVERSION-001 note with the "Fix 2 shipped default-off" clause.
- Feature doc: update `docs/features/jiminy-actionability.md` (if exists) OR fold into observability doc.
- Sprint post.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit**:
- Prompt string equality tests (default-off = HEAD; flag-on = HEAD + new clause).
- Compact variant equivalents.

**Tier 2 — Integration**:
- ULTS `--verify-hashes` passes on the default-off state.
- Full `go test ./internal/jiminy/` clean.

**Tier 3 — Live E2E**:
- E5's 3-action smoke against live llama-server on mdemg-dev.
- 3-day A/B recipe deferred to operator (not dev-time).

## 7. Commit Strategy

1. `docs(jiminy-actionability-compliance-credit-001): E0 — sprint plan`
2. `docs(jiminy-actionability-compliance-credit-001): E1 — baseline stats`
3. `feat(jiminy-actionability-compliance-credit-001): E2+E3 — prompt gate + tier-1 tests`
4. `test(jiminy-actionability-compliance-credit-001): E4 — ULTS hash verified (default-off byte-identical)`
5. `docs(jiminy-actionability-compliance-credit-001): E5 — live Tier-3 smoke`
6. `fix(jiminy-actionability-compliance-credit-001): E6 — Actionable Compliance Rate panel wording`
7. `docs(jiminy-actionability-compliance-credit-001): E7 — 3-day A/B operator recipe`
8. `docs(jiminy-actionability-compliance-credit-001): E8 — CHANGELOG + CLAUDE.md + sprint post`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] `go test ./...` clean (new prompt tests + existing classifier suite)
- [ ] ULTS `--verify-hashes` PASSES
- [ ] Live Tier-3 smoke: 3 verdicts correct under flag-on
- [ ] Dashboard JSON validates
- [ ] CHANGELOG + CLAUDE.md + sprint post committed
- [ ] Pushed; auto-PR created

## 9. Documentation Update (Epic E8 — never cut)

- **CHANGELOG.md** [Unreleased] > Fixed: sprint entry.
- **CLAUDE.md**: extend JIMINY-ACTIONABILITY-INVERSION-001 note in place with "Fix 2 shipped default-off as JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001; operator runs the 3-day A/B recipe then flips `JIMINY_NONVIOLATION_CREDIT_ENABLED=true` in `.env`".
- **Feature doc**: `docs/features/jiminy-actionability.md` (existing — extend) OR fold into observability-dashboards.md.
- **A/B recipe**: `docs/development/jiminy-actionability-compliance-credit-001/ab_recipe.md`.
- **Sprint post**: `docs/development/jiminy-actionability-compliance-credit-001/post.md`.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| LLM interprets the new clause too aggressively → too many `not_applicable` verdicts → constraint follow rate reads misleadingly high | Medium | Default-off ship; operator runs 3-day A/B; if the rate lifts from 10% to >50% (unrealistic-good), that's a signal the classifier is over-crediting non-application — rollback flag |
| Prompt hash change breaks ULTS CI | Low | Default-off render = HEAD byte-identical; hash unchanged (design choice) |
| Compact variant drifts from full variant semantically | Low | Both variants gain the same clause; unit test asserts both strings contain the clause when flag on |
| Operator flips flag without running A/B, then sees weird numbers | Low | Feature doc emphasizes A/B recipe; alert rule threshold reflects new metric |
| Interaction with tier-1 short-circuit (embedding similarity < 0.20 → `ignored` without LLM call) | Medium | E2 change ONLY affects tier-2 LLM path; tier-1 short-circuit unchanged. Tier-1 rows in the constraint_outcomes today ARE the "definitively unrelated" bucket; this sprint doesn't try to reroute them |

## 11. Rollback Procedures

- **Data**: N/A. The gate doesn't backfill; existing rows unchanged.
- **Code**: revert per-commit; the flag itself can be disabled via env without a code revert.
- **Config**: `JIMINY_NONVIOLATION_CREDIT_ENABLED=false` (default) is the rollback state.
- **Panel wording**: revert the mdemg-jiminy.json edit (single-panel rename).

## 12. Documents Accessed

- Parent: `docs/development/jiminy-actionability-inversion-001/{investigation,fix_spec}.md`
- `internal/jiminy/outcome_classifier.go:20-58` (system prompts)
- `internal/jiminy/service.go:1730,1762` (not_applicable gate — must stay closed)
- `docs/tests/ults/specs/jiminy_evaluate_llm.ults.json` (pinned system_prompt_hash)
- `internal/config/config.go` (new env var)
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (Fix 1 panel wording)
- `internal/alert/rules.go::GuidanceShouldFollowRules` (alert rule title match)
- CLAUDE.md § JIMINY-ACTIONABILITY-INVERSION-001, § JIMINY-CORPUS-001
