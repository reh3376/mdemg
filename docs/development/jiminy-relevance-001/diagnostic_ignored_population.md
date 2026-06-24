# JIMINY-RELEVANCE-001 — Step 1 Diagnostic: the ignored-guidance population

**Date:** 2026-06-23 · read-only diagnostic · the Option-B follow-up disclosed in
JIMINY-EFFECTIVENESS-001. Goal context: operator target is **follow rate > 90%**
and **constraint effectiveness > 90%**; current ≈ 27% effectiveness / ≈ 12% pure
follow. This diagnostic asks *why guidance is ignored* and *whether we have the
data to retrain toward the goal* — before committing to a collection/retrain plan.

## TL;DR
1. **We cannot retrain toward the goal today — the training evidence does not
   exist.** `constraint_outcomes` (and the Neo4j `GUIDANCE_OUTCOME` edges) store
   the *verdict* (`outcome_type`, `similarity`, codes, ids) but **not the guidance
   text and not the agent-action text**. No store holds them. Every historical
   interaction's `(context → guidance → did-the-agent-follow)` triple is
   unrecoverable. We persist verdicts everywhere, evidence nowhere.
2. **The root cause is guidance *composition/actionability*, not model fluency.**
   90% of surfaced guidance is abstraction-type (`pattern`/`learning`/`concept`),
   which is ignored 53–65% of the time; the actionable types
   (`constraint`/`correction`, 10% of volume) are ignored half as often. And
   ignored guidance is *topically on-point* (96% at similarity > 0.8) — it's not
   wrong, it's not actionable.
3. **~51% of the labels we do have are non-LLM heuristic defaults** — noise. A
   retrain on them would learn the heuristic, not the goal.
4. **> 90% may be the wrong target as stated.** Some guidance is correctly
   ignored. The meaningful target is ">90% follow on guidance that *should* have
   been followed" — which requires the action evidence (finding 1) to even
   measure.

## Evidence (live `mdemg-dev`, 30-day window, 2,561 outcome rows)

### Finding 1 — the evidence isn't stored (the binding constraint)
- TSDB: no column for guidance content or action summary anywhere
  (`constraint_outcomes` cols: `time, space_id, constraint_id, constraint_code,
  guidance_id, session_id, outcome_type, similarity, guidance_type, instance_id,
  classifier_source`).
- Neo4j: no node carries `guidance_id`; `GUIDANCE_OUTCOME` edge props are
  `outcome_type, guidance_type, similarity, created_at, guidance_id, session_id`.
- The `action_summary` POSTed to `/v1/jiminy/feedback` is classified and
  discarded.
- **Consequence:** the 2,561 labels cannot be assembled into training triples.
  Production emits the perfect signal every prompt and we throw the evidence away.

### Finding 2 — composition is dominated by non-actionable abstractions
| guidance_type | volume | % ignored | % followed |
|---|---|---|---|
| pattern | 1,013 (40%) | 63% | 14% |
| learning | 989 (39%) | 53% | 8% |
| concept | 311 (12%) | 65% | 19% |
| **constraint** | 133 (5%) | **30%** | 16% |
| **correction** | 115 (4%) | **27%** | 2% |

90% of guidance (`pattern`+`learning`+`concept` = 2,313 rows) is the
emergent-principle abstraction class; the actionable `constraint`/`correction`
class is 10% and is followed roughly 2× better / ignored roughly half as often.

### Finding 3 — ignored ≠ off-topic
LLM-classified ignored rows by similarity: **950 of 984 (96%) at sim > 0.8.**
Guidance is semantically adjacent to the action but does not drive a specific
action — the signature of *not-actionable*, not *irrelevant*.

### Finding 4 — label quality is ~half noise
51% of rows have a non-LLM `classifier_source` (blank/heuristic), including 747
`partial_compliance` rows defaulted at sim 0.32. These are heuristic guesses, not
measured outcomes.

### Finding 5 — the structural driver
The graph holds **111 `role_type='constraint'` nodes vs 19,147 abstraction nodes
(layer ≥ 2 / HiddenPattern) — a 172:1 ratio.** Retrieval over this pool
*structurally* surfaces abstractions over actionable constraints. This is the
RRF-SCALE-001 "retrieval surfaces emergent_concept abstractions, not raw
constraint nodes" class, now quantified.

### Live qualitative sample
The Jiminy guidance surfaced to this very session (2026-06-23, hook channel) was
advisory prose instructing adherence to "established conventions for git commit
messages" on a `/tmp/jeff_commit_msg.txt` file, citing two node IDs, with no
specific action — a textbook *not-actionable + mis-contextualized* item, sourced
from `emergent_concept` "Foundational principle: corrections, phase, never, new"
abstractions. Representative of the 90% in Finding 2.

## What this means for the path
- **The lever is collection + composition, not (yet) a model retrain.** Three
  things must precede any retrain: (a) persist the evidence so a corpus can exist;
  (b) raise label quality so the corpus is trustworthy; (c) bias what gets
  surfaced toward actionable constraint/correction guidance (or synthesize
  abstractions into imperative directives).
- **A 3–6 month curated-collection effort is the right call** (operator-decided):
  the corpus has to accumulate trustworthy `(context, surfaced-guidance,
  action, audited-outcome)` rows at production distribution before a retrain can
  move the needle. This is the CLAUDE.md "recursive-retraining loop" (FT Phases
  6/7/9, NOT STARTED) and feeds FT-CLASSIFY-002.
- **Reframe the metric**: instrument "follow rate on should-follow guidance"
  (requires the action evidence) so the > 90% target is measured against a
  reachable ceiling, not against a denominator that includes correctly-ignored
  advisory items.

## Diagnostic queries (reproducible)
TSDB: `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics` then the
`guidance_type × outcome` and `similarity-bucket` aggregates above (`time >
now()-interval '30 days'`). Neo4j: `J17TrustState` / `GUIDANCE_OUTCOME` key
inspection + the constraint-vs-abstraction node counts.

## Documents Accessed
`internal/jiminy/{stats,service,types}.go`; `internal/api/handlers_jiminy.go`;
live TSDB `constraint_outcomes`; live Neo4j `GUIDANCE_OUTCOME` / `MemoryNode`
counts; the live hook-channel guidance sample; CLAUDE.md (recursive-retraining
loop, FT-CLASSIFY-002, RRF-SCALE-001).
