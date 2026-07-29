# JIMINY-CEILING-INVESTIGATION-001 — Sprint Plan (Investigation Only)

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #4.

## 1. Header & Metadata

**Investigation sprint** — no code shipped. Deliverable is a written
analysis + data-decided disposition table + concrete next-lever
recommendation. Effort ~1 day (research passes, live TSDB queries,
sampled reads, synthesis).

## 2. Problem Statement

The Jiminy actionable-guidance follow rate has converged on **~11%**
across three consecutive sprint arcs designed to move it:

| Sprint | Ship date | Focus | Post-ship follow rate |
|---|---|---|---|
| JIMINY-ACTIONABILITY-001 | 2026-06-25 | 3 surface levers (A/B/C) | ~11% |
| JIMINY-CORPUS-001 | 2026-07-03 | constraint corpus cleanup + repetition control | ~11-13% |
| JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 | 2026-07-24 | non-violation credit for must_not | 12.97% (KEEP-ON verdict) |

The operator's stated goal is `>90%` follow rate on actionable guidance
(per JIMINY-RELEVANCE-001). Gap is ~8×. Continued sprint-lever-tuning
has diminishing returns; the honest recalibration in
JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 named ~13% as the realistic
steady state.

**Before spending more sprint capacity on levers, we need to know WHY
this is the ceiling.** The Q4 deep-dive named four candidate failure
modes; this sprint measures which ones actually apply and in what
proportion.

## 3. Scope & Method

**In scope:**
- Live TSDB queries against `constraint_outcomes` (7d window)
- Cross-references to Neo4j L1 `role_type='constraint'` node contents
- Sampled reading of `(constraint, action_summary, outcome_type)`
  tuples with manual + heuristic categorization
- Written report at `docs/development/jiminy-ceiling-investigation-001/`
  with per-failure-mode percentages + next-lever recommendation

**Out of scope:**
- Any code changes (this is diagnostic, not therapeutic)
- Enabling/disabling any current levers (the shipped state is the
  system we're measuring)
- Sampling from `guidance_training_rows` (that table is post-audit
  relabeled; we want the RAW outcome signal from `constraint_outcomes`)

## 4. Method — 4 sequential steps

**S1 — Cohort identification** (`~30 min`)
- Query TSDB `constraint_outcomes` grouped by `constraint_code` over
  last 7d, sorted by count desc
- Take top-10 by surface volume
- Cross-reference each `constraint_code` to its L1 constraint node in
  Neo4j to read the actual rule text
- Deliverable: table of `(constraint_code, rule text, 7d count)`

**S2 — Per-constraint outcome distribution** (`~1 hour`)
- For each of the top-10, break down outcomes:
  - `followed / ignored / contradicted / partial_compliance /
    not_applicable`
  - Split by `classifier_source` (llm / tier1 / heuristic)
  - Split by similarity bucket (>0.55, 0.30-0.55, 0.10-0.30, <0.10)
- Identifies which constraints have systematic failure patterns vs
  distributed noise
- Deliverable: per-constraint outcome breakdown tables

**S3 — Sampled failure-mode categorization** (`~3-4 hours`)
- For 2-3 highest-ignore-rate constraints from S2, pull 20-30
  `(constraint, action_summary, outcome_type, similarity)` tuples from
  `constraint_outcomes`
- Read each tuple + categorize into one of the 4 failure modes:
  - **surface mismatch**: constraint was surfaced but wasn't relevant
    to the action (retrieval / Lever C over-surfaced it)
  - **context mismatch**: constraint IS relevant but the action was in
    a different context where the rule doesn't apply (e.g. rule about
    schema migrations, action was a config change)
  - **genuine ignore**: constraint was relevant AND applied AND agent
    should have followed but didn't
  - **classifier misclassification**: constraint was ACTUALLY FOLLOWED
    but the classifier labeled it "ignored"
- Deliverable: per-failure-mode counts with representative examples
  cited

**S4 — Disposition + next-lever recommendation** (`~1-2 hours`)
- Synthesize: what proportion of the ~89% ignore rate is each failure
  mode?
- For each failure mode: what LEVER would move it? What's the plausible
  ceiling if that lever hits perfectly?
- Rank levers by (expected impact × effort⁻¹)
- Deliverable: `post.md` with the disposition table + concrete lever
  recommendation

## 5. Testing / Verification

- Per-query cross-checks: constraint counts from `constraint_outcomes`
  should match Neo4j edge counts on `GUIDANCE_OUTCOME` (RRF-SCALE-001
  invariant); if they diverge, the constraint_code join is broken and
  the whole analysis is invalidated
- Sampled categorization: every categorization must cite the exact
  outcome_row so the reader can independently verify

## 6. Commit strategy

Single commit at the end (report + sprint plan). Investigation sprints
don't produce sequential code artifacts.

## 7. Risks

- **Sample size too small** for statistical claims — mitigated by
  clearly framing findings as "sampled evidence" not "population
  statistics" in the report
- **Sampling bias** if I only look at top-10 volume — high-volume
  constraints may exhibit different failure modes than long-tail ones;
  flag this in the report + name it as a follow-up investigation
- **The 4 failure modes may not exhaust the space** — if I find a
  significant fifth mode during sampling, add it and note the discovery
  in the report

## 8. Documents Accessed

Filled in the final post.md.
