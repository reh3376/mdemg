# REVIEW-SUGGESTED-GUIDANCE-CONSUME-001 — Sprint Plan

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #2 (column-
level dormant surface). Original candidate #7 from Q4 deep-dive
recommendation list.

## 1. Header & Metadata

Extend `scripts/curate_guidance_corpus.py` to emit
`review_grades.suggested_guidance` content as **synthetic corpus rows**
alongside the primary evidence rows. Length-gated to reject triage-note
uses of the field. ~4-6 hours effort (smaller than the 2d estimate
because the curator already has the join infrastructure).

## 2. Problem Statement

DORMANT-CENSUS-002 disclosed a column-level dormant surface:
`review_grades.suggested_guidance` is populated (2/18 = 11% on
mdemg-dev) but has ZERO code readers. HITL-REVIEW-001's CLAUDE.md
pin names the intent: *"SME-authored 'what would have been better
guidance' — the highest-value retrain signal → feeds
jiminy-actionability-001."*

The intent hasn't shifted — the column was aspirational, waiting for
a consumer. This sprint wires the minimum-viable consumer: include
SME suggestions in the training corpus assembled by the shipped
`data curate-guidance` command.

**Data reality on mdemg-dev:**

| Suggestion | Category | Length |
|---|---|---|
| `"[must] When editing config.go to add a field, wire all three sites (struct field, FromEnv parse with a default, struct-literal assignment) or the config scanner fails Build."` | genuine SME rule | ~200 chars |
| `"This was a duplicate entry"` | operator triage note | 22 chars |

Blind emission would pollute the corpus with the triage note. A
length-gate (default 40 chars) filters those out.

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- Modify `scripts/curate_guidance_corpus.py`:
  - Extend the `review_grades` LEFT JOIN to also fetch
    `suggested_guidance`
  - When non-empty AND length ≥ `--min-suggestion-length` (default 40),
    emit a SYNTHETIC corpus row:
    - Same `space_id`, `session_id`, `action_summary` as the original
      row
    - `guidance_content = <the SME's suggested_guidance text>`
    - `outcome_type = 'followed'` (SME says this IS what should be
      followed)
    - `classifier_source = 'sme_suggestion'` (distinct provenance tag)
    - Ties to the source review_grade via a new `sme_source_grade_id`
      field on the corpus row
  - Report SME-suggestion counts in the manifest
    (`sme_suggestions_included` + `sme_suggestions_skipped_short`)
- New CLI flag: `--min-suggestion-length` (default 40)
- Docs: post + CHANGELOG + CLAUDE.md pin

**Out of scope:**

- Any change to `internal/review/` or `handlers_review.go` — the
  field's WRITE path is fine as-is
- Retrain pipeline integration (the consumer flow into the actual
  training call is FT-line work; this sprint feeds the corpus, and
  the shipped retrain trigger already reads the corpus)
- HITL analytics panel (option b from the deep-dive) — a corpus
  consumer is higher-value than a viewer

## 4. Method

**Phase 1 — Extend the curator SQL + emission logic**
- Update `fetch_rows()`'s SQL to also SELECT `rg.suggested_guidance`
- Update the row-writer to emit an additional synthetic row per
  qualifying suggestion
- Add manifest fields for counts

**Phase 2 — CLI flag + integration**
- Add `--min-suggestion-length` flag with default 40
- Verify the Go CLI wrapper passes the flag through

**Phase 3 — Live smoke + docs**
- Run curator on mdemg-dev; verify:
  - The genuine SME rule ("config.go three sites") emits a synthetic row
  - The triage note ("This was a duplicate entry") is SKIPPED
  - The manifest reports the counts
- Docs + commit

## 5. Testing Plan

- **Tier 1**: no new unit tests — the curator is a Python script,
  primary test surface is the live-smoke on real data
- **Tier 2**: `go build`, `golangci-lint 0 issues`, `go test ./...`
  full green (Go CLI wrapper unchanged aside from adding a flag)
- **Tier 3 (live)**: curator run against mdemg-dev; inspect corpus
  output for exactly 1 synthetic SME row (from the config.go
  suggestion) + confirm the "duplicate entry" is not emitted

## 6. Commit Strategy

Single commit under `REVIEW-SUGGESTED-GUIDANCE-CONSUME-001`.

## 7. Verification Checklist

- [ ] Curator SQL fetches `suggested_guidance`
- [ ] Synthetic row emission gated on length ≥ threshold
- [ ] `classifier_source='sme_suggestion'` on synthetic rows
- [ ] `sme_source_grade_id` populated for traceability
- [ ] Manifest reports `sme_suggestions_included` +
      `sme_suggestions_skipped_short`
- [ ] `--min-suggestion-length` flag on both Python + Go CLI
- [ ] Live smoke: 1 synthetic row emitted, triage note skipped
- [ ] CHANGELOG + CLAUDE.md pin
- [ ] Sprint post with data

## 8. Rollback

- Revert commit. The curator falls back to the pre-sprint behavior
  (LEFT JOIN gold_outcome only). No substrate mutation, no schema
  change.

## 9. Risks

- **Risk**: an operator-authored SME suggestion is a genuine rule but
  shorter than the 40-char default → gets skipped as a triage note.
  - **Mitigation**: `--min-suggestion-length` is config-tunable;
    operator can lower it. Also the SKIPPED count is reported in the
    manifest so silent drops are visible.
- **Risk**: the `outcome_type='followed'` synthetic assignment is
  wrong — an SME suggestion might be phrased as a "what to NOT do"
  rule, in which case `contradicted` would be more accurate.
  - **Acknowledgment**: the field's freeform nature means this
    heuristic is a first-cut; a future sprint could add lightweight
    LLM classification of the suggestion's intent (must/should/
    must_not) and pick the appropriate synthetic outcome. Not shipping
    that machinery here.

## 10. Documents Accessed

Filled in `post.md`.
