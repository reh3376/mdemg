# Sprint JIMINY-ACTIONABILITY-INVERSION-001 — Investigate why advisory guidance is followed more than actionable

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | JIMINY-ACTIONABILITY-INVERSION-001 |
| Sprint Name | Investigate why advisory guidance is followed more than actionable guidance |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 0.5–1 dev-day (investigation-heavy; fix scope decided from findings) |
| Sprint Line | jiminy-actionability-inversion-001 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Suspicious finding from DASHBOARD-TRUTH-002 triage (2026-07-20) |

## 2. Problem Statement

Should-Follow Follow Rate on the Jiminy dashboard is supposed to be **≥** the raw Follow Rate: it excludes correctly-ignored advisory types (`pattern|learning|concept`) and counts only actionable types (`constraint|correction`). The premise is "actionable is what SHOULD be followed."

Live evidence (2026-07-20, `mdemg-dev`, 7-day window over `constraint_outcomes`):
- Raw Follow Rate (all types): **0.146**
- Should-Follow (actionable-only): **0.125**
- Gap: should-follow is **-0.021 LOWER** than actual — **sign is inverted**.

Per-type breakdown (7d):

| type | followed | total | rate |
|---|---|---|---|
| constraint | 16 | 172 | **9.3%** |
| correction | 8 | 29 | 27.6% |
| **actionable combined** | 25 | 200 | **12.5%** |
| pattern | 8 | 49 | 16.3% |
| learning | 9 | 40 | 22.5% |
| risk | 5+0.5p | 29 | 19.0% |
| concept | 2 | 16 | 12.5% |
| **advisory combined** | 24+ | 134 | **~18%** |

**Advisory types are followed at 1.4×–2.4× the rate of actionable types.** The correction rate (27.6%) is the only actionable that beats advisory — and it's brand-new corpus from JIMINY-CORRECTION-PRODUCER-001 (2026-07-20 today), so its statistics are unreliable. Constraints (the dominant actionable, 172/200 rows) are the ones being ignored the most.

This is neither an artifact (measurement is honest) nor a known open sprint's target. It's a new finding requiring investigation to decide the remediation shape.

## 3. Scope & Constraints

**In scope**:
- Reproduce the inversion against fresh data.
- Classify the root cause among candidate hypotheses:
  1. **Classifier calibration asymmetry** — the outcome classifier under-credits actionable follows because their text is short/imperative and less overlappy with the action's paraphrase, while advisory text is longer and gets fuzzy credit.
  2. **JIMINY-CORPUS-001 E4 4-band restoration bias** — the precise `not_applicable` restoration moved actionable rows into `ignored` more aggressively than advisory rows (advisory being less easy to classify as "not applicable").
  3. **Lever C surface bias** — constraint-partition biasing surfaces constraints that don't apply to the specific action → high ignored rate for constraints in particular; advisory items surface only via base retrieval and are more relevant-when-surfaced.
  4. **Duplication after purge** — post-purge constraints (140→61) are repeatedly surfaced under session cooldown pressure; many are still redundant/adjacent → ignored.
  5. **The classifier prompt itself** — is the `jiminy.evaluate` LLM system prompt biased toward crediting narrative "followed" for softer guidance?
- Choose ONE root cause (or a small subset) based on evidence, and document.
- Produce a follow-up sprint spec if a fix is warranted.

**Out of scope**:
- Any code change beyond adding lightweight diagnostic queries/scripts.
- Building the fix (its own sprint, spec'd here).
- Fixing the Should-Follow panel wording (that's DASHBOARD-TRUTH-002 A7 territory if we conclude the panel description needs update).

**Constraints**:
- **Read-only investigation** — no substrate mutations.
- **Cross-check LLM decisions**: for a sample of `ignored` actionable rows, hand-inspect whether the classifier was actually correct.
- **Report writes to** `docs/development/jiminy-actionability-inversion-001/investigation.md`.

## 4. Dependencies & Pre-Conditions

- ✅ `mdemg-dev` has recent LLM-classified outcomes.
- ✅ JIMINY-CORPUS-001 shipped (defines the purge/cooldown/effectiveness levers baseline).
- ✅ JIMINY-ROLETYPE-ADAPTER-001 shipped (retrieval carries role_type correctly now).
- ✅ JIMINY-CORRECTION-PRODUCER-001 shipped (first L1 corrections exist).
- ⚠️ Correction sample size is tiny (n=29 in 7d, mostly today) — will need to weight cautiously.

## 5. Implementation Plan

### E0 — Sprint plan
Commit this plan.

### E1 — Reproduce the inversion
Direct SQL queries against `constraint_outcomes`:
- Per-type follow rates over 1d / 7d / 30d windows.
- Per-classifier-source (`llm` vs `tier1` vs `explicit` vs `heuristic`) split by type.
- Trend: is the inversion stable, widening, or recent?
Capture in `investigation.md`.

### E2 — Test hypothesis 1: classifier calibration asymmetry
Sample 30-50 `ignored` actionable rows + 30-50 `ignored` advisory rows. For each, retrieve the original `(guidance_content, action_summary, similarity, classifier verdict, reasoning)` tuple from `llm_interactions` where task=`jiminy.evaluate*` or the tier-2 classifier. Hand-classify: was the LLM verdict correct?
If actionable "ignored" rows are systematically false-negatives (actually followed in the action text) → hypothesis 1 CONFIRMED.

### E3 — Test hypothesis 2: 4-band restoration bias
Compare the same actionable/advisory populations by the observed similarity bands:
- What % of actionable-ignored rows fall in `[NA(0.10), LOW(0.20))` vs `< NA(0.10)`?
- What % of advisory-ignored rows fall in the same bands?
If actionable is over-represented in the just-restored `ignored` band while advisory sits below NA → hypothesis 2 CONFIRMED.

### E4 — Test hypothesis 3: Lever C surface bias
Query the surfaced-but-ignored rate for each type — is constraint surfaced-per-action higher than advisory types? If Lever C is over-surfacing constraints, the ignore rate is arithmetic (denominators inflated by irrelevant-to-context surfacings, not by classifier).

### E5 — Test hypothesis 4: duplication after purge
For the top-N most-ignored constraints, inspect: are they semantically-close to each other? Is session cooldown insufficient?

### E6 — Test hypothesis 5: prompt bias
Read the current `jiminy.evaluate` system prompt. Look for asymmetric language (e.g. "did the action reflect the pattern/learning?" is easier to answer "yes" than "did the action strictly follow the constraint?"). If prompt-level bias suspected, propose a controlled A/B (matched prompts, same corpus, same-day).

### E7 — Decide root cause + write fix spec
Rank hypotheses by evidence. Write `docs/development/jiminy-actionability-inversion-001/fix_spec.md` with:
- Root cause verdict
- Recommended remediation (sprint spec)
- Confidence level + open questions.

### E8 — Documentation
- CHANGELOG entry (this is an INVESTIGATION sprint — no code change, but the finding matters).
- CLAUDE.md: add a Jiminy calibration architectural note reflecting the finding.
- Sprint post with all evidence + root-cause verdict.

## 6. Testing Plan (3 tiers)

This is an investigation sprint — Tiers reframed:

**Tier 1 (Unit-equivalent)**: reproducibility of the SQL queries — same query, same window, same numbers.
**Tier 2 (Integration-equivalent)**: cross-check between TSDB `constraint_outcomes` and `llm_interactions` — do the classifier's reasoning strings match the recorded outcome?
**Tier 3 (Live E2E)**: hand-inspection of the 30-50 sample verdicts against source guidance + source action content — is the LLM verdict manifestly correct or manifestly wrong?

## 7. Commit Strategy

1. `docs(jiminy-actionability-inversion-001): E0 — sprint plan`
2. `docs(jiminy-actionability-inversion-001): E1 — inversion reproduced`
3. `docs(jiminy-actionability-inversion-001): E2 — classifier asymmetry hypothesis tested`
4. `docs(jiminy-actionability-inversion-001): E3 — 4-band restoration bias hypothesis tested`
5. `docs(jiminy-actionability-inversion-001): E4 — Lever C surface bias hypothesis tested`
6. `docs(jiminy-actionability-inversion-001): E5 — duplication-after-purge hypothesis tested`
7. `docs(jiminy-actionability-inversion-001): E6 — prompt bias hypothesis tested`
8. `docs(jiminy-actionability-inversion-001): E7 — root cause verdict + fix spec`
9. `docs(jiminy-actionability-inversion-001): E8 — CHANGELOG + CLAUDE.md + sprint post`

May collapse to fewer commits if some hypotheses are quickly refuted.

## 8. Verification Checklist

- [ ] All hypotheses have a documented test + verdict (PASS/REFUTED/INCONCLUSIVE)
- [ ] Root cause identified OR "insufficient data — need N weeks more" documented
- [ ] Fix spec written (may be "no fix needed, just update panel description")
- [ ] CHANGELOG + CLAUDE.md + sprint post committed
- [ ] Pushed; auto-PR created

## 9. Documentation Update (Epic E8 — never cut)

- **CHANGELOG.md** [Unreleased] > **Investigation** (new subsection if none): sprint findings summary.
- **CLAUDE.md**: extend JIMINY-CORPUS-001 architecture note with the discovered inversion + root cause; anchor future work.
- **Sprint post**: comprehensive findings + fix spec.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Sample size too small for confident verdict (correction n=29) | Medium | Explicitly weight actionable = constraint (n=172) as primary; correction treated as "too fresh" and noted |
| Inversion turns out to be measurement noise (7d window not enough) | Medium | Cross-check against 30d window; if noisy, document "need N weeks" and pause fix |
| Multiple hypotheses partially support the data | Medium | Rank by evidence strength; ship a MULTI-fix spec if warranted |
| Fix spec creates scope inflation for follow-up sprint | Low | Explicitly bound the fix spec to ONE root cause and one epic; deferred issues listed separately |

## 11. Rollback Procedures

- N/A (no code/data changes).

## 12. Documents Accessed

- DASHBOARD-TRUTH-002 triage report (this session)
- `docs/development/jiminy-corpus-001/` (parent — corpus + Lever C + 4-band gate)
- `docs/development/jiminy-outcome-002/` (4-band restoration)
- `docs/development/jiminy-roletype-adapter-001/` (role_type propagation)
- `docs/development/jiminy-correction-producer-001/` (fresh corrections)
- CLAUDE.md § JIMINY-CORPUS-001, § JIMINY-OUTCOME-002, § JIMINY-ROLETYPE-ADAPTER-001, § JIMINY-CORRECTION-PRODUCER-001
- `internal/jiminy/outcome_classifier.go`
- `internal/jiminy/stats.go`
- `internal/tsdb/dataset_builder.go::GuidanceEffectiveness`
- TSDB tables `constraint_outcomes`, `llm_interactions`, `guidance_training_rows`
