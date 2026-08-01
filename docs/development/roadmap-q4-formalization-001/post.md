# ROADMAP-Q4-FORMALIZATION-001 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4-frontier deep-dive candidate #10 (the deep-dive
itself named it "meta; write it yourself from these findings + your
priorities" — no code change, doc-only sprint).

## Verdict

**Shipped.** `docs/development/roadmap/ROADMAP_2026Q4.md` (534 lines,
Q3-format-compatible) now exists as the canonical Q4 roadmap doc. Q3
had one; Q4 didn't until now — the deep-dive was scoping, and the arc
was executed against the deep-dive's ranked candidates + their
cascading follow-ups. This doc closes the meta gap by writing it up
formally.

## What shipped

- **`docs/development/roadmap/ROADMAP_2026Q4.md`** (534 lines,
  section-parallel to Q3):
  - §1 State-of-the-System Verdict as of 2026-08-01
  - §2 Phases (retrospective — 5 thematic phases covering all 22
    sprints shipped this quarter, each with per-sprint one-paragraph
    summary + sprint-dir cross-ref)
  - §3 Ranked next-quarter sprints (top-10 with PLUGIN-HYGIENE as
    #1 pending operator disposition, CONTEXT-LIVE-001 and HEBB-ETA-
    001 rollovers now unblocked, plus disclosed-during-Q4 items)
  - §4 Explicitly deferred (updated from Q3 — what closed this
    quarter, what's still deferred, new deferrals from Q4)
  - §5 Post-roadmap follow-ups (5 small items disclosed during Q4
    execution)
  - Annex enumerating 25+ rules pinned to CLAUDE.md this quarter,
    grouped by class (retrieval, guidance, hygiene/drift-check,
    Go-analysis, meta)

## Why this exists

Q3's roadmap doc was a critic-revised deep-dive with sprint-plan
sequencing. Q4's shape was different — the deep-dive was framed
explicitly as "scoping, not commitment" and the operator picked
sprints from it directly, then chained through disclosed follow-ups.
That's a valid execution pattern but leaves no canonical "here's
what shipped and why" record.

This doc IS that record. It's retrospective for §2 and forward-looking
for §3 — the same shape Q3's doc has, but with the phase-plan reversed
(Q3 named the phases up front; Q4 assembled them after the fact).

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q3.md` (format template +
  §4 explicit-deferrals inheritance)
- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (the arc's anchor)
- `CLAUDE.md` (pinned rules — for the Annex)
- `CHANGELOG.md` (shipped-sprint enumeration)
- Every Q4 sprint's `post.md` under `docs/development/` (for the
  per-sprint summaries in §2)
