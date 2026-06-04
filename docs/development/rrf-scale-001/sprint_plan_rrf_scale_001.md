# Sprint RRF-SCALE-001 — RRF Score-Scale Consumer Remediation

> **Status:** APPROVED — in execution.
> **Type:** P0 correctness fix (data/cognition pipeline).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | RRF-SCALE-001 |
| **Sprint line** | `docs/development/rrf-scale-001/` |
| **Date opened** | 2026-06-03 |
| **Target version** | v0.11.1 (patch — bugfix, no new feature surface) |
| **Estimated effort** | 1.5–2 dev-days |
| **OpenAI / LLM spend** | $0 (no new LLM call sites; live-verify uses the existing local model) |
| **Risk level** | Medium. Touches the consulting suggestion/constraint path that feeds Jiminy guidance. Changes are gate-threshold + normalization recalibration — behavior-affecting but bounded by feature flags + config. The win is large (revives a 9-week-dormant cognitive loop); the risk is over-correcting the gate so noise surfaces as guidance. |
| **Priority** | P0 — a core cognitive function (constraint-effectiveness learning via the guidance→feedback→outcome loop) has been silently inactive since the Phase 13.1 RRF cutover. |

## 2. Problem Statement

The Jiminy guidance→feedback→outcome loop has been **dormant since ~Phase 13.1 default-on (2026-05-03)**. Root cause, confirmed by live diagnosis (see §11 evidence):

`consulting.Service.findApplicableConstraints` and the surrounding suggestion/conflict generation gate on **hardcoded absolute retrieval-score thresholds** (`r.Score < 0.55`, `> 0.6`, `> 0.65`, `> 0.7`) calibrated for the **legacy linear scorer**. When **Column-Voting RRF became the default scorer (Phase 13.1, 2026-05-03)**, the fused-score scale dropped: strong semantic matches now top out around **0.40–0.53** instead of exceeding the legacy thresholds. Every retrieved node now falls below the gates → `consulting.Suggest` returns **zero constraints, zero suggestions, zero conflicts** → Jiminy synthesizes empty guidance → no `guidance_id` is captured by the hook → `send_jiminy_feedback` never fires → no `GUIDANCE_OUTCOME` edges, no `constraint_outcomes` TSDB rows. The entire constraint-effectiveness learning loop is silently no-op.

This is the **third instance of one bug class**: downstream consumers with hardcoded assumptions about the retrieval-score contract that silently broke when Phase 13.1 changed it.
1. EVENTGRAPH-001 found: the RRF path dropped the `Activation` field → retrieve-time Hebbian learning no-op (~24 days).
2. This sprint's primary: the `0.55` constraint gate → guidance loop no-op (~9 weeks).
3. The audit sweep (Epic 1) will catalog the rest before they're tripped over individually.

Live proof: a retrieve for "committing code directly to the main branch" correctly surfaces `EmergentConcept-L2-git-main` (top score **0.533**) — recall is perfect — but **0 of 10 results clear the 0.55 gate**, so guidance is empty.

## 3. Scope & Constraints

### In scope
1. **Complete audit** (Epic 1) of every downstream consumer of post-RRF `RetrieveResult.Score` / `.Activation` / score-derived confidence. Findings doc with file:line, current threshold, blast radius, remediation.
2. **Fix the consulting score-gate cluster** (Epic 2) — 7 gates in `consulting/service.go` (lines 931, 944, 957, 981, 1005, 1081, 1087). Replace absolute-score gates with the **scale-invariant `NormalizedConfidence` percentile signal** (already computed on the RRF path), config-driven thresholds with RRF-appropriate defaults.
3. **Recalibrate + config-ify the confidence sigmoid** (Epic 2) — `retrievalScoreMidpoint=1.5` / `retrievalScoreSteepness=1.5` are legacy-scale; crush RRF-scored confidence toward zero. Make both config-driven with RRF-calibrated defaults.
4. **Remediate remaining audit findings** (Epic 3) — the `retrieval/jiminy.go` Activation display gates (lines 45, 155, 192) and any other sites Epic 1 surfaces.
5. **3-tier testing** (Epic 4) including live-verify that the **full dormant loop is revived end-to-end**: real guidance surfaces → feedback POSTs → outcome lands in both Neo4j + TSDB.
6. **Documentation** (Epic 5) — fix doc, CHANGELOG, CLAUDE.md, post.md.

### Out of scope
- **Re-tuning the RRF scorer itself** — the scorer is correct; the consumers' assumptions are wrong. We fix the consumers, not the scorer. (A scorer change would re-break everything calibrated to the new scale.)
- **The `intent_translate` 4/997 LLM failures** noticed during diagnosis — unrelated, non-critical, separate follow-up.
- **Backfilling the 9-week gap** of missing guidance outcomes — forward-only; there's no source to reconstruct outcomes that never fired.
- **The guidance synthesis LLM path** — confirmed healthy (LLM endpoint up, 997 calls/7d, 0.4% failure). Synthesis simply never runs because retrieval gates upstream return empty; fixing the gates revives synthesis automatically.
- **Why constraint_outcomes had a 1,135-row Apr 6–12 burst** (likely backfill) — archaeology, no forward value.

### Constraints
- Sequential epics (`feedback_sequential_epics.md`): Epic 1 audit completes before Epic 2/3 fixes; design-from-findings.
- No-hardcoded-values rule (`feedback_no_hardcoded_values.md`) — the bug *is* a hardcoded value; the fix must be config-driven with sensible defaults, not a re-hardcoded constant.
- Tier 3 live testing required (`feedback_live_testing_required.md`) — the acceptance bar is the **revived loop observed end-to-end on the live stack**, not a unit test.
- Thorough verification (`feedback_rigorous_verification.md`) — confirm actual observable output (guidance text, Neo4j edge, TSDB row), not just "gate passes."
- Fix-now / zero-tolerance: this is the fix; do not defer sibling findings as "pre-existing."

## 4. Dependencies

- **Phase 13.1 Column-Voting RRF** (default-on) — the upstream whose score-contract change exposed the consumers.
- **`RetrieveResult.NormalizedConfidence`** (percentile 0–100) — the scale-invariant signal the fix gates on. Confirmed populated on the RRF path via `ApplyNormalizedConfidenceToResults` (`internal/retrieval/service.go:867`).
- **EVENTGRAPH-001 fix-commit `f307f55`** — the prior instance of this class; precedent for the fix shape + the live-smoke discovery discipline.
- **Live stack** — `mdemg` native binary + Docker (Neo4j + TSDB) + llama-server, all currently healthy, for Epic 4 live-verify.
- **111 `role_type='constraint'` nodes + 67 typed correction/pattern/learning observations** in `mdemg-dev` — the material that *should* surface once gates are fixed.

## 5. Implementation Plan

### Epic 0 — Sprint plan (~0.1 day)
Commit this plan. No code.

### Epic 1 — Complete audit sweep (~0.3 day)
Systematic grep + read of every consumer reading post-RRF `Score`/`Activation`/derived confidence. Produce `docs/development/rrf-scale-001/audit_findings.md`: one row per site (file:line, current threshold, what it gates, RRF-scale impact High/Med/Low/None, remediation). Known starting set:
- `consulting/service.go` lines 931, 944, 957, 981, 1005, 1081, 1087 (7 gates) — **High** (the loop killer cluster)
- `consulting/service.go:35-36` sigmoid `retrievalScoreMidpoint/Steepness` — **High** (crushes confidence)
- `retrieval/jiminy.go` lines 45, 155, 192 (Activation display gates) — **triage** (explanation text, likely Low)
- Sweep beyond the known set: rerank input gates, frontier detection, negative-feedback thresholds, any `Score`/`Activation`/`VectorSim`/`Confidence` comparison reading a final retrieval result.
**Gate:** findings doc complete; every High/Med site has a remediation decided.

### Epic 2 — Fix consulting score-gate cluster + sigmoid (~0.5 day)
- Replace the 7 absolute-score gates with `NormalizedConfidence`-percentile gates. **Decision deferred to execution per `feedback_plan_options_pattern.md`:**
  - **Option A — percentile gates** (recommended): `r.Score < 0.55` → `r.NormalizedConfidence < cfg.ConsultingConstraintMinPercentile`. Scale-invariant to any future scorer. Defaults derived empirically from the live `mdemg-dev` score distribution (Epic 1 captures it).
  - **Option B — config-ified absolute thresholds** with RRF-calibrated defaults. Simpler, but re-introduces scale-coupling (would re-break on the next scorer change). Documented as the fallback.
- Recalibrate `retrievalScoreMidpoint/Steepness` for RRF scale + make config-driven (`CONSULTING_SCORE_SIGMOID_MIDPOINT`, `_STEEPNESS`). Default midpoint chosen so a median-strong RRF match (~0.4–0.5) maps to a meaningful confidence (~0.6–0.7), not ~0.
- New config knobs (no-hardcoding): `CONSULTING_CONSTRAINT_MIN_PERCENTILE`, `CONSULTING_SUGGESTION_MIN_PERCENTILE`, `CONSULTING_CONFLICT_MIN_PERCENTILE`, `CONSULTING_SCORE_SIGMOID_MIDPOINT`, `CONSULTING_SCORE_SIGMOID_STEEPNESS` — all with defaults that reproduce sensible behavior on the live distribution.
- Tier 1 unit tests: gates admit RRF-scored results that should pass, reject genuine noise; sigmoid maps RRF scores to sane confidence; config overrides honored.
**Gate:** `consulting.Suggest` (unit-level) returns constraints for RRF-scored inputs; lint clean.

### Epic 3 — Remediate remaining audit findings (~0.25 day)
Apply per-site remediation for Epic 1's other High/Med findings (Activation display gates + any sweep discoveries). Low-impact display-only gates: config-ify or rescale as appropriate; document any intentionally left as-is with rationale.
**Gate:** all High/Med sites remediated; Tier 1 tests green.

### Epic 4 — Tier 2 integration + Tier 3 live e2e (~0.4 day)
- **Tier 2:** integration test — `consulting.Suggest` against real Neo4j returns constraints for a context matching known constraint nodes; assert `suggest_constraints > 0`.
- **Tier 3 live e2e (the real acceptance bar):**
  1. `POST /v1/jiminy/guide` with "commit to main" context → assert non-empty guidance, `source_counts.constraints > 0`.
  2. Drive the **full loop**: warm with a real context → `/v1/jiminy/latest` returns guidance with a `guidance_id` → POST `/v1/jiminy/feedback` with an action summary → verify a new **Neo4j `GUIDANCE_OUTCOME` edge** AND a new **TSDB `constraint_outcomes` row** appear (the two sinks dead since May).
  3. Confirm `jiminy.synthesis`/`consulting.classify` LLM calls now fire (they were absent because synthesis never ran).
- Transcript → `docs/development/rrf-scale-001/verification.md`.
**Gate:** guidance non-empty live; a fresh outcome lands in both Neo4j and TSDB; transcript captured.

### Epic 5 — Documentation (~0.2 day, never cut)
- `docs/features/` — update the relevant guidance/consulting feature doc (or add a "Score-scale contract" note) explaining the percentile-gate design + why absolute score gates are banned downstream.
- `CHANGELOG.md` Unreleased entry.
- `CLAUDE.md` — note under a "Score-scale contract" or the RSIC/Jiminy section: downstream consumers MUST gate on `NormalizedConfidence` percentile, never raw `Score`, because the scorer scale is not a stable contract.
- `docs/development/rrf-scale-001/post.md` — epic-by-epic, acceptance check-off, the bug-class retrospective (3rd instance), forward-looking.

## 6. Testing Plan (3 tiers — required)

**Tier 1 — Unit (target 12–15):**
- `consulting/service_test.go`: percentile gates admit/reject correctly at RRF scale; each of the 7 former-absolute gates exercised; sigmoid recalibration maps representative RRF scores (0.1/0.3/0.5/0.53) to expected confidence band; config overrides honored; empty-results and all-below-threshold edge cases.
- Parser/helper tests for any new config wiring.

**Tier 2 — Integration (`-tags=integration`):**
- `consulting.Suggest` against real Neo4j + the `mdemg-dev` constraint nodes returns `constraints > 0` for a matching context; returns empty for a genuinely irrelevant context (no false-positive flood).

**Tier 3 — Live e2e (Epic 4):**
- Real binary + live stack: `/v1/jiminy/guide` non-empty; full warm→latest→feedback→outcome loop produces a fresh Neo4j edge + TSDB row; synthesis LLM calls fire. Transcript in `verification.md`.

## 7. Commit Strategy
Sequential commits per epic on `reh3376_dev01`; auto-PR. Epic 1 = audit doc. Epic 2 = consulting fix + Tier 1. Epic 3 = remaining sites. Epic 4 = integration tests + verification.md. Epic 5 = docs. Any surprise bug found during Epic 4 live smoke gets its own fix-commit (EVENTGRAPH-001 precedent). Sprint summary on the PR after Epic 5.

## 8. Verification Checklist
- [ ] Audit findings doc enumerates every High/Med RRF-scale consumer with remediation.
- [ ] 7 consulting gates no longer use raw absolute `Score`; use percentile (or config-ified) gates.
- [ ] Confidence sigmoid recalibrated + config-driven; RRF scores map to sane confidence.
- [ ] All new thresholds are config-driven with documented defaults (no new hardcoded magic numbers).
- [ ] Tier 1 unit tests green; `golangci-lint run ./...` clean.
- [ ] Tier 2 integration: `consulting.Suggest` returns constraints for a matching live context; empty for irrelevant context.
- [ ] Tier 3 live: `/v1/jiminy/guide` returns non-empty guidance with `constraints > 0`.
- [ ] Tier 3 live: full loop produces a **fresh** `GUIDANCE_OUTCOME` edge (Neo4j) + `constraint_outcomes` row (TSDB), timestamped today.
- [ ] `jiminy.synthesis`/`consulting.classify` LLM calls observed firing post-fix.
- [ ] No regression in existing consulting/jiminy/retrieval test suites.
- [ ] CHANGELOG, CLAUDE.md, post.md, verification.md, feature doc updated.
- [ ] Sprint summary on PR.

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Over-correcting gates → noise surfaces as guidance (false positives) | Medium | Medium | Gate on percentile derived from the live score distribution (Epic 1 captures it); Tier 2 asserts irrelevant contexts still return empty. Tune defaults conservatively; config-driven so operators can adjust. |
| Percentile signal not meaningful when result set is tiny/uniform | Low | Medium | Keep a low *absolute* floor as a secondary guard (config), so a single high-confidence result still passes; document the hybrid. |
| Recalibrated sigmoid mis-scales confidence elsewhere (constraint authority, effectiveness scoring) | Medium | Medium | Audit all consumers of `normalizeRetrievalConfidence` output in Epic 1; Tier 1 covers the confidence band; config-driven for rollback. |
| Other RRF-scale consumers exist beyond the sweep | Medium | Medium→High | That's exactly what Epic 1's systematic sweep targets; the CLAUDE.md "score-scale contract" note (Epic 5) prevents re-introduction. |
| Reviving the loop floods TSDB/Neo4j with outcomes | Low | Low | Outcomes are gated by actual guidance feedback (hook cooldown + 30-min TTL already throttle); volume is human-paced. |
| Fix changes guidance behavior mid-session for the user | Low | Low | Feature-flag the new gate path if needed; defaults chosen to restore intended pre-13.1 behavior, not invent new. |

## 11. Documents Accessed
- `internal/consulting/service.go` — `Suggest` (551), `findApplicableConstraints` (998), the 7 score gates, `normalizeRetrievalConfidence` (40), sigmoid constants (35–36)
- `internal/jiminy/service.go` — `Guide` (609), `RecordOutcome` (1383), `outcomeWriter` wiring, source_counts assembly (940)
- `internal/jiminy/persistence.go` — `PersistGuidanceOutcome` (GUIDANCE_OUTCOME edge writer)
- `internal/api/handlers_jiminy.go` — `handleJiminyLatest` (cache read), `handleJiminyWarm`, `handleJiminyGuide`
- `internal/retrieval/scoring.go` — `ApplyNormalizedConfidence` (982), `ApplyNormalizedConfidenceToResults` (1032)
- `internal/retrieval/scoring_rrf.go` — RRF result construction (the EVENTGRAPH-001 Activation fix site)
- `internal/tsdb/migrations/011_constraint_outcomes.sql`, `constraint_outcomes_writer.go`, `backfill.go`
- `.claude/hooks/prompt-context.sh` (warm + guidance_id capture), `post-tool-observe.py` (send_jiminy_feedback)
- Live diagnostics: `/v1/jiminy/guide` debug (`retrieval_found:10, suggest_constraints:0`), retrieve scores (top 0.533, 0/10 ≥ 0.55), `llm_interactions` (997 calls/7d healthy), `constraint_outcomes` (1,139 rows, last May 1), Neo4j GUIDANCE_OUTCOME (893 edges, last Apr 12)

## 12. Rollback Procedures
- All new behavior is **config-driven**; restore prior behavior by setting the new threshold/sigmoid env vars back to legacy values (documented in Epic 5).
- Per-epic git revert: Epic 2 (consulting fix) and Epic 3 (remaining sites) are independent commits; revert either without touching the other.
- No schema changes, no migrations, no data mutation — pure logic/threshold changes. Rollback is config or code-revert; no data to restore.
- The revived outcome data is forward-only and additive; rolling back the fix simply re-dormants the loop (returns to the broken-but-stable prior state).

---

## Files to be created/modified (anticipated)

**New:**
- `docs/development/rrf-scale-001/sprint_plan_rrf_scale_001.md` (Epic 0)
- `docs/development/rrf-scale-001/audit_findings.md` (Epic 1)
- `docs/development/rrf-scale-001/verification.md` (Epic 4)
- `docs/development/rrf-scale-001/post.md` (Epic 5)

**Modified:**
- `internal/consulting/service.go` — 7 gates → percentile; sigmoid recalibration + config
- `internal/config/config.go` — new `CONSULTING_*` threshold/sigmoid knobs
- `internal/retrieval/jiminy.go` — Activation display gates (per Epic 1 triage)
- `internal/consulting/service_test.go` — Tier 1
- `tests/integration/` — Tier 2 consulting suggest test
- `CHANGELOG.md`, `CLAUDE.md` — Epic 5
- (possibly) `docs/features/<guidance-or-consulting>.md` — score-scale contract note

## Acceptance Criteria
1. `/v1/jiminy/guide` with a constraint-matching context returns non-empty guidance with `source_counts.constraints > 0` on the live stack.
2. The full guidance→feedback→outcome loop produces a **fresh** Neo4j `GUIDANCE_OUTCOME` edge + TSDB `constraint_outcomes` row dated today — the loop dead since May is observably revived.
3. The audit findings doc catalogs every RRF-scale consumer; all High/Med are remediated.
4. Every new threshold is config-driven with a documented default; no new hardcoded score constant remains in the fixed paths.
5. Irrelevant contexts still return empty guidance (no false-positive flood) — Tier 2 asserts.
6. `CLAUDE.md` records the score-scale contract so this bug class cannot silently recur.
