# RETRIEVAL-QUALITY-AUDIT-001 — Sprint Plan (Investigation Only)

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #6.

## 1. Header & Metadata

**Investigation sprint** — no code shipped. Deliverable is a written
report scoring live retrieval quality on a curated real-shaped query
sample. Effort ~half a day (12-15 queries × top-5 scoring + synthesis)
— substantially less than the 5d deep-dive estimate because I'm using
live-issued queries instead of recovering historical ones from
retrieval_audit (which stores hashes, not text).

## 2. Problem Statement

Retrieval is the substrate's most-load-bearing quality signal. **It has
never been formally audited.** The Q4 deep-dive named this as a
frontier-research candidate with the justification: "a low grade here
says 'the substrate remembers wrong things'."

Adjacent evidence exists — the RRF-SCALE-001 arc surfaced multiple
downstream consumers with hardcoded score thresholds calibrated for the
old scorer scale; the SCORE-RETRIEVAL-REAL-SIGNALS-001 sprint replaced
enum-lookup scoring with real-signal reads. But nobody has directly
asked: *when a developer queries this substrate, does the answer help?*

## 3. Scope & Method

**In scope:**
- Curated set of 12-15 realistic operator-shaped queries against
  live `/v1/memory/retrieve` on `mdemg-dev`
- Capture top-5 results per query (node_id, layer, role_type, content
  preview, score)
- Self-grade each result on a 4-value scale:
  - **helpful**: directly answers or usefully informs the query
  - **stale**: technically matches but is superseded knowledge / old
    sprint / historical artifact
  - **wrong-context**: semantically related but wrong context
    (e.g. query about a service, result is about a different service)
  - **redundant**: repeats info already surfaced higher in top-5
    (or in another top-5 result)
- Also flag **missing**: obviously-relevant nodes I know exist in the
  substrate but weren't in top-5 (open-ended, catch by memory)
- Written report at `docs/development/retrieval-quality-audit-001/`
  with per-query scores + failure-mode distribution + concrete lever
  recommendation

**Out of scope:**
- Any code changes (diagnostic only)
- Retraining the reranker
- Changing retrieval config
- Formal HITL platform integration (self-grading is sufficient for a
  first pass; formal HITL follow-up in a separate sprint if warranted)

## 4. Query taxonomy (what to sample)

Real operator work on this substrate touches:
1. **Recent decisions & rules** — "what did we decide about X?"
2. **Bug/error recall** — "did we see this error before?"
3. **Feature/architecture knowledge** — "how does Y work?"
4. **Sprint history** — "what did sprint Z ship?"
5. **Config lookups** — "what's the default for env var W?"
6. **Cross-referencing** — "what depends on component V?"
7. **Anti-patterns / constraints** — "what should I not do?"

Aim: 2-3 queries per bucket = ~15 queries total. Mix of narrow (single
correct answer) and broad (multiple valid answers) shapes.

## 5. Method — 3 sequential steps

**S1 — Query sampling + top-5 capture** (`~30 min`)
- Assemble the query set
- Issue each against `/v1/memory/retrieve` (mdemg-dev)
- Save `{query, top-5-results}` JSON per query

**S2 — Self-grade each top-5 result** (`~2-3 hours`)
- Read each result's content + reason explicitly about its usefulness
  for the query
- Assign one of {helpful, stale, wrong-context, redundant}
- Note any missing-but-should-be-there nodes
- Aggregate per-query "helpful@5" (0-5)

**S3 — Disposition + next-lever recommendation** (`~1 hour`)
- Per-query score table
- Per-mode distribution
- Failure patterns (does staleness cluster on certain query shapes?
  does wrong-context correlate with layer? etc.)
- Concrete lever recommendation with expected-impact reasoning

## 6. Testing / Verification

- Reproducibility: every query + its top-5 saved verbatim so operator
  can independently re-grade
- Self-audit: after grading, re-read grades and flag any that feel
  uncertain; report their share honestly

## 7. Commit strategy

Single commit at end (plan + samples + report).

## 8. Risks

- **Self-grading bias**: I might over-rate helpful because I'm the
  one who authored much of the content. Mitigate by holding a strict
  "would an operator not already knowing this find this useful?" bar.
- **Query-selection bias**: 15 queries can't cover the whole distribution.
  Mitigate by naming the taxonomy explicitly + flagging shapes not
  sampled.
- **Live-state dependence**: findings are a snapshot of mdemg-dev on
  2026-07-29; other spaces or other times may score differently.
  Flag as scope in the report.
