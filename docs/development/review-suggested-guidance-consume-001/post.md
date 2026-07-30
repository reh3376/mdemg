# REVIEW-SUGGESTED-GUIDANCE-CONSUME-001 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #2 (column-
level dormant surface). Original candidate #7 from Q4 deep-dive.

## Verdict

**Shipped.** `scripts/curate_guidance_corpus.py` now consumes the
previously-dormant `review_grades.suggested_guidance` column and emits
its content as synthetic corpus rows. Length-gated (default 40 chars)
to reject operator triage notes. Live-verified on mdemg-dev: 1 genuine
SME rule (~173 chars, config.go three-sites) emitted as a synthetic
row; force-raised gate (500 chars) correctly skips it via the
`sme_suggestions_skipped_short` counter.

## What shipped

- **`scripts/curate_guidance_corpus.py`**
  - Extended `fetch_rows()` SQL: `LEFT JOIN LATERAL review_grades` now
    also selects `suggested_guidance` + `grade_id`. LATERAL filter
    excludes reversed grades (`r.reversed = false`).
  - **Decoupled capability detection**: the pre-existing "have_gold"
    branch referenced a `gold_outcome` scalar column that never
    landed (the shipped schema stores gold verdicts in
    `gold_dimensions` JSONB). The curator was crashing every time
    `review_grades` existed. Split into two capability checks:
    - `have_grades` (table exists → SME suggestions available)
    - `have_gold_outcome` (column exists → gold verdict overrides)
  - New synthetic-row emission block after each primary record:
    - `label_source='sme_suggestion'` (distinct provenance)
    - `outcome='followed'` (SME's suggestion is what SHOULD be followed)
    - `row_id='<orig>::sme_sug'`, plus `sme_source_grade_id` +
      `sme_source_row_id` for traceability
    - Gated on `len(suggested_guidance.strip()) >= --min-suggestion-length`
  - Manifest fields: `sme_suggestions_included`,
    `sme_suggestions_skipped_short`, `min_suggestion_length`,
    `grades_available` (in addition to legacy `gold_available`).
- **`--min-suggestion-length`** flag on both Python + Go CLI, default 40,
  env-tunable via `GUIDANCE_CORPUS_MIN_SUGGESTION_LENGTH`.
- **`internal/cli/data_curate_guidance.go`** — flag pass-through.
- **`internal/cli/data_curate_guidance_test.go`** — flag assertion.

## Live Tier-3 smoke (mdemg-dev)

**Setup:** `review_grades` on mdemg-dev has 4 grades for
`dataset_id='guidance'`; 1 non-reversed row has non-empty
`suggested_guidance` (173 chars — the genuine config.go three-sites
rule).

**Default gate (--min-suggestion-length 40):**
```
sme_suggestions_included: 1
sme_suggestions_skipped_short: 0
```
Synthetic row in corpus:
- `row_id: "gh7weddjslggrxse8xury5wn::sme_sug"`
- `label_source: "sme_suggestion"`
- `outcome: "followed"`
- `guidance_content: "[must] When editing config.go to add a field, wire all three sites..."`
- `sme_source_grade_id: "dly0vltygvmrhxblt13hmugb"`
- `sme_source_row_id: "gh7weddjslggrxse8xury5wn"`

**Raised gate (--min-suggestion-length 500):**
```
sme_suggestions_included: 0
sme_suggestions_skipped_short: 1
```
The same 173-char row now falls below the gate and is counted as
skipped-short. Both branches of the gate proven live.

## Rules pinned

1. **Capability detection at the column level, not the table level.**
   The curator's pre-existing `_table_exists("review_grades")` gate was
   too coarse — the table existed but the referenced column
   (`gold_outcome`) did not, and the curator crashed every run. When a
   SQL branch references specific columns, check them individually via
   `_column_exists` (added helper). This class error is common when a
   schema evolves to store a value in a JSONB blob instead of a scalar
   column.

2. **Dormant-column consumers must be length-gated.** Free-text SME
   fields carry both genuine rules AND operator triage notes ("this was
   a duplicate entry"). Emit blind and the corpus gets polluted. Gate
   on length (default 40) with the skip counter reported in the
   manifest so silent drops are visible.

3. **Synthetic corpus rows must carry provenance.** `label_source`
   distinguishes them from primary rows; `sme_source_grade_id` +
   `sme_source_row_id` let a downstream consumer trace back to the
   grade + evidence that produced them. `row_id` gets a `::sme_sug`
   suffix so uniqueness is preserved even when a single primary row
   spawns a synthetic sibling.

## Follow-ups disclosed

- **Refined outcome inference for `must_not`-shaped SME rules** — the
  synthetic emitter hardcodes `outcome='followed'` under the assumption
  that an SME suggestion is "what should be followed." An SME
  suggestion phrased as a prohibition (e.g. "You must NOT amend pushed
  commits") is really a "what should be avoided" rule and might be
  better as `outcome='contradicted'` (paired with a matching
  action_summary). Not shipping the classification here because
  a) the current data volume is 1 row, b) the safe default is
  `followed` since SME suggestions are almost always positive
  imperatives.

- **Pre-existing `gold_outcome` column mismatch** — the curator has
  legacy code assuming a scalar `gold_outcome` column that never
  landed. This sprint defensively falls through to the no-gold-outcome
  branch when the column is absent (unblocking the SME-suggestion
  path). A future sprint could either (a) migrate `gold_outcome` out of
  `gold_dimensions` JSONB into a scalar column, or (b) rewrite the
  gold-outcome extraction to read from JSONB. Not shipping here — the
  SME-suggestion consumer is higher-value than resurrecting a
  redundant scalar column.

## Documents Accessed

- `docs/development/dormant-census-002/post.md` (parent — disclosed
  the dormant column)
- `docs/development/hitl-review-001/post.md` (writer of the column)
- `docs/development/review-suggested-guidance-consume-001/sprint_plan.md`
  (this dir)
- `scripts/curate_guidance_corpus.py` (target file)
- `internal/cli/data_curate_guidance.go` (Go CLI wrapper)
- `internal/cli/data_curate_guidance_test.go` (test)
- Live TSDB queries against mdemg-dev `review_grades` + Docker
  `psql` for schema introspection
