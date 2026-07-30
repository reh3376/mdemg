# RETRIEVAL-REVERSE-LOOKUP-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #1 (RQA
cluster A — "what consumes X?" queries broken). Q4 follow-up #1.

## Verdict

**Shipped.** Filesystem-grep-at-query-time + post-rerank quota
injection surfaces consumers of a symbol into top-K. **Live q11
"what consumes the constraint_outcomes table?"**: baseline 1/10
consumers → **candidate 3/10** (including `GuidanceEffectiveness`, an
actual consumer function from `dataset_builder.go`). **Guards
q07/q10 unchanged** because a query-shape gate ensures the promoter
fires only for reverse-lookup-shaped queries.

## What we found while investigating

**RQA-001's Option A (keyword-index reference column) was
architecturally impossible on this substrate.** MDEMG's node summaries
are function-level abstractions that don't include the SQL/symbol
strings from function bodies. Only ONE consumer function
(`GuidanceOutcomeWithContext` in `internal/eventgraph/guidance_outcomes.go`)
had "constraint_outcomes" in its indexed summary. All others
(`dataset_builder.go::LLMPerformance`, `alert/rules.go::HeuristicShareRule`,
`mdemg-jiminy.json` dashboard) were structurally INVISIBLE to any
substrate-side text search — the answer wasn't in the substrate at
all.

⚠️ **Architectural rule pinned**: when a retrieval class fails because
the answer isn't in the substrate, no substrate-side indexing helps —
the fix must reach an authoritative external source (filesystem, graph
walk of ingested code, or ingest-time content extraction). This sprint
picked filesystem-grep as the smallest-scope viable path; the proper
long-term fix is ingest-time symbol extraction (RQA-001's Option B —
disclosed as follow-up).

## Two live-caught regressions during smoke — TWO rules pinned

### Regression #1: MinSlots=3 caused q10 5/5→2/5

First smoke had `RETRIEVAL_REVERSE_REF_QUOTA_MIN_SLOTS=3`. On q11
(reverse-lookup): improved 1/10→3/10. On q10 "how do I enable the
recursive-retrain actuator?" (NOT reverse-lookup): regressed 5/5→2/5
because the quota promoted 3 grep-matched-but-irrelevant files
(`WeightIntegrityRules`, `GraduationProcessor`,
`curate_guidance_corpus.py` — all mention "recursive"/"actuator"/
"retrain" tokens somewhere in their bodies) over the natural top-3
relevant `.md` files.

⚠️ **Architectural rule pinned**: the reverse-ref quota promoter MUST
be query-shape gated. Firing on every query displaces natural top-K
with irrelevant grep matches whenever the query contains
identifier-shaped tokens. Fix: `IsReverseLookupQuery(queryText)` gate
checks for a whitelist of reverse-lookup verbs (`consumes`, `uses`,
`reads`, `calls`, `writes`, `references`, `depends`, `imports`,
`queries`, `joins`, `selects`, `inserts`, `updates`, `deletes`,
`affects`, `produces`, `triggers`, plus `where`/`who` as
question-shape indicators). Promoter fires only when the gate returns
true. Under the gate, MinSlots=3 is safe.

### Regression #2: writer-file bias vs consumer-file surfacing

Grep alone ranks by hit count — writer files (`ConstraintOutcomesWriter`,
migrations) mention `constraint_outcomes` MANY times because the
symbol is central to their identity. Consumer files (`LLMPerformance`,
`HeuristicShareRule`) mention it 1-3 times. Raw hit-count ranking
kept the promoter surfacing writers repeatedly.

⚠️ **Architectural rule pinned**: for reverse-lookup grep ranking,
CAP per-file hit count at a small constant (default 3) so files with
"the symbol is my raison d'être" don't dominate files with
"the symbol appears here 1-3 times in real reference sites."
Deterministic tie-break by (HitCount DESC, path-length ASC, lexical).
The cap alone doesn't make consumers WIN — the writer files still
tie at the cap — but it makes them COMPETITIVE, so the quota's
MinSlots=3 injection gives BOTH writers AND consumers a chance to
surface in the top-K.

## What shipped

**Config (8 env vars):**
- `RETRIEVAL_REVERSE_REF_ENABLED` (default false → true in `.env`)
- `RETRIEVAL_REVERSE_REF_WORKSPACE_ROOT` (default: `MDEMG_WORKSPACE`
  env → cwd; empty disables)
- `RETRIEVAL_REVERSE_REF_TOPK` (default 15 — needs runway because
  many top hits are typically writer files already in the primary pool)
- `RETRIEVAL_REVERSE_REF_MIN_SYMBOL_LEN` (default 5)
- `RETRIEVAL_REVERSE_REF_MAX_FILES_SCANNED` (default 5000 — safety cap)
- `RETRIEVAL_REVERSE_REF_EXTENSIONS` (default
  `.go,.py,.sql,.md,.yml,.yaml,.json,.ts,.tsx,.js`)
- `RETRIEVAL_REVERSE_REF_EXCLUDE_DIRS` (default
  `node_modules,.git,.venv,dist,build,vendor,.mypy_cache`)
- `RETRIEVAL_REVERSE_REF_QUOTA_MIN_SLOTS` (default 3 — safe under
  the shape gate)

**Code:**
- `internal/retrieval/reverse_ref.go` — `extractCandidateSymbols` +
  `grepReferences` (Go-native, NO shell-out, symlink-safe, file-cap
  bounded) + `fetchReverseRefResults` (grep + Neo4j path lookup) +
  `IsReverseLookupQuery` (shape gate) + `parseCSVList`
- `internal/retrieval/reverse_ref_quota.go` — `ApplyReverseRefQuota`
  (mirror of concrete-quota's shape; identity-based promotion by
  NodeID membership)
- `internal/retrieval/service.go` — three-block wire: fetch
  reverse-ref → shape-gate the quota → promote
- `internal/models/models.go` — new per-request `?reverse_ref=true|false`
  fields
- `internal/api/handlers.go` — URL param parsing (mirrors `?sparse=`,
  `?concrete=`)
- `internal/retrieval/cache.go` + `cache_key_coverage_test.go` — added
  to CacheKey (CACHE-KEY-002 contract)
- 20 unit tests across reverse_ref_test.go: symbol extractor
  (case-preservation, stop-words, dedup, empty text, min-length),
  grep helper (hit-count ranking, word-boundary, extension filter,
  excluded dirs, topK cap, max-files-scanned, empty inputs), CSV
  parser, quota promoter (disabled passthrough, promotion from
  extras, already-satisfied no-op, empty extras), shape gate (8 case
  matrix).

## Live Tier-3 A/B on mdemg-dev

### q11 "what consumes the constraint_outcomes table?"

Before:
```
[0] BackfillConstraintOutcomes             (WRITER)
[1] ConstraintOutcomesWriter               (WRITER)
[2] <nil>                                  (conversation_observation)
[3] investigation.md                       (unrelated doc)
[4] 011_constraint_outcomes.sql            (DEFINER migration)
[5] GuidanceOutcomeWithContext             (CONSUMER — the one that surfaced naturally)
[6] baseline_composition.md                (unrelated doc)
[7] tsdb                                   (DEFINER)
[8] constraint_outcomes                    (DEFINER migration#table)
[9] 026_constraint_outcomes_classifier_source.sql (DEFINER migration)
```
Consumers surfaced: 1/10 (`GuidanceOutcomeWithContext`)

After:
```
[0] event-graph-federation.md              (CONSUMER doc — describes the eventgraph federation reading constraint_outcomes)
[1] post.md                                (CONSUMER sprint post — EVENTGRAPH-002 whose whole point was consuming this table)
[2] GuidanceEffectiveness                  (CONSUMER function — reads constraint_outcomes in dataset_builder.go)
[3] 011_constraint_outcomes.sql            (DEFINER)
[4] constraint_outcomes                    (DEFINER)
[5] tsdb                                   (DEFINER)
[6] ConstraintOutcomesWriter               (WRITER)
[7] 026_constraint_outcomes_classifier_source.sql (DEFINER)
[8] BackfillConstraintOutcomes             (WRITER)
[9] 023_constraint_outcomes_code_index.sql (DEFINER)
```
Consumers surfaced: **3/10** (event-graph-federation.md,
eventgraph-002 post.md, GuidanceEffectiveness function).

### Guards

**q07 "what did HITL-CURATION-002 ship?"** — shape gate says NOT
reverse-lookup, promoter no-op. 4/5 helpful (unchanged from baseline;
1 slot is a nil conversation_observation).

**q10 "how do I enable the recursive-retrain actuator?"** — shape gate
says NOT reverse-lookup, promoter no-op. 4/5 helpful (unchanged from
baseline).

### Performance

Latency added: ~4-6s for the grep + Neo4j lookup on this workspace
(~2500 files scanned). Runs concurrent with vector recall so wall-
clock impact is smaller. Acceptable for the retrieval quality gain
on reverse-lookup queries; fail-open on error.

## Follow-ups disclosed

1. **RQA-001's Option B (symbol-references graph edges) remains the
   right long-term fix.** Ingest-time symbol extraction (via
   tree-sitter or regex) writes `REFERENCES_SYMBOL` edges into the
   graph; the structural retrieval column then walks from any symbol
   node → its references. Advantages over filesystem-grep: sub-100ms
   graph query vs 4-6s filesystem walk; works when the query text
   doesn't literally name the symbol (the graph can be entered from
   any adjacent context). Deferred as a separate sprint (~1-2 weeks
   effort).

2. **Grep-ranking heuristic for consumers vs writers.** The per-file
   hit-count cap works but is blunt. A future refinement could
   penalize files whose PATH contains the symbol
   (`constraint_outcomes_writer.go` → -1 rank) since those are usually
   the definer/writer; boost files whose path is UNRELATED to the
   symbol name.

3. **Shape gate whitelist may miss reverse-lookup phrasings.** E.g.,
   "who is downstream of X?", "what depends on X's output?". Add
   verbs as we encounter false-negatives.

## Documents Accessed

- `docs/development/retrieval-quality-audit-001/post.md` (parent —
  cluster A recommendation)
- `docs/development/retrieval-reverse-lookup-001/sprint_plan.md` (this
  dir)
- `docs/development/retrieval-layer-balance-001/post.md` (shipped
  precedent — the shape gate + quota reuse pattern)
- `internal/retrieval/concrete_quota.go` (mirrored shape)
- `internal/retrieval/service.go` (integration seam)
- `internal/jiminy/service.go::fetchActionableCandidates` (Lever C
  reference for the fetch-then-inject pattern)
- Live Neo4j queries against mdemg-dev for consumer inventory
  (verified `dataset_builder.go`, `alert/rules.go`,
  `eventgraph/guidance_outcomes.go` as expected consumers)
- Live workspace grep for ground truth (17 Go files reference
  `constraint_outcomes` under `internal/`)
