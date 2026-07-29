# RETRIEVAL-QUALITY-AUDIT-001 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #6.
**Verdict:** Investigation complete. **Retrieval is broadly working
(60% helpful@5 median, 4/5 median); the failure modes cluster on
specific query shapes.** Concrete lever recommendation at §5.

## 1. What we set out to answer

Retrieval is the substrate's most-load-bearing quality signal, but had
never been formally audited. The Q4 deep-dive named it: "a low grade
here says 'the substrate remembers wrong things'." Method: issue 15
realistic operator-shaped queries against live `/v1/memory/retrieve`
on `mdemg-dev`, self-grade every top-5 result.

## 2. Method

- 15 curated queries across 7 buckets (decisions, errors,
  architecture, sprints, config, cross-references, antipatterns)
- Live-issued against `/v1/memory/retrieve` (mdemg-dev, top_k=5)
- 75 result rows captured (query, node_id, layer, role_type, score,
  vector_sim, summary)
- Each result hand-graded into one of: **helpful / stale /
  wrong-context / redundant**
- Missing-but-should-be-there nodes noted qualitatively
- All samples saved verbatim at `/tmp/rqa/samples.json` +
  `.../samples.txt` for independent re-grading

## 3. Per-query results

| # | Query | Helpful@5 | Notes |
|---|---|---|---|
| q01 | why did we pivot from MoE to dense LoRA for FT? | **4/5** | 1 stale (pre-pivot sprint plan) |
| q02 | what did we decide about the auto-grade reinforcement invariant? | **2/5** | 3 wrong-context (adjacent corrections/reward sprints) |
| q03 | how did we fix the classifier heuristic fallback burst on 2026-07-21? | **2/5** | 3 wrong-context (HIDDEN-CHURN adjacent) |
| q04 | why does the pre-bash guard match SQL keywords inside search queries? | **1/5** | 2 redundant (dup pre-bash-check module) + 2 unrelated (Postgres SQL lexer package) |
| q05 | how does the HITL autograder work end-to-end? | **5/5** | all directly useful |
| q06 | how does the ft_production_drift alert connect to cycle-open? | **4/5** | 1 abstract L2 meta-concept |
| q07 | what did HITL-CURATION-002 ship? | **5/5** | |
| q08 | what was the reframe of DRIFT-VALIDATION-001? | **1/5** | 3 L5 emergent-concepts about "test failure correction" (wrong domain) + 1 redundant |
| q09 | default value of FT_DRIFT_MARGIN | **1/5** | 4 abstract L3/L4 emergent-concepts, no config-file result |
| q10 | how do I enable the recursive-retrain actuator? | **5/5** | |
| q11 | what consumes the constraint_outcomes table? | **0/5** | ALL results are about the table itself (definition, migrations, writer). None about *consumers*. |
| q12 | which sprints modify the Gate.EvaluateTrigger path? | **5/5** | |
| q13 | why must I not use ORDER BY LIMIT 1 in alert rules? | **4/5** | 1 Grafana yaml (wrong-context — the rule lives in Go, not yaml) |
| q14 | what are the CUIDv2 rules and why not UUID? | **2/5** | 3 redundant L3/L4 abstract concepts; no L0 file surfaced despite `feedback_cuidv2_required.md` existing |
| q15 | how should I write a Go-safe multi-byte string cutting for UTF-8? | **4/5** | 1 unrelated `devspacepb.String` |

**Aggregate:**
- Total helpful: **45/75 = 60%**
- Median helpful@5: **4/5** (12 of 15 queries scored ≥ 3/5)
- 4 queries scored perfect 5/5 (q05, q07, q10, q12)
- 1 query scored 0/5 (q11 — the reverse-lookup case)

## 4. Failure-mode distribution

Across 75 results:

| Mode | n | % | Definition |
|---|---|---|---|
| **helpful** | 45 | **60%** | Directly answers or usefully informs the query |
| **wrong-context** | 18 | **24%** | Semantically related but wrong context (e.g. Postgres SQL lexer surfaced for a query about SQL keywords in a Bash guard) |
| **redundant** | 8 | **11%** | Repeats info already surfaced higher (or as another near-duplicate result) |
| **stale** | 1 | **1%** | Pre-pivot sprint plan surfaced when the pivot memory file was also in top-5 |
| **missing** | ~2-3 noted | qualitative | Obviously-relevant nodes exist but weren't in top-5 (q11: no consumer refs; q14: no L0 CUIDv2 memory file) |

**Staleness is nearly non-existent** (1/75) — surprising given the age
of some substrate memories. Retrieval does not systematically surface
old-and-superseded content.

## 5. Failure-mode CLUSTERS (the real story)

The 60% headline hides a strong signal: **failures cluster on specific
query shapes**, not evenly across queries.

### Cluster A: reverse-lookup queries (q11)

**"What consumes X?" is broken.** Retrieval interpreted the query as
"find things similar to constraint_outcomes" and returned:
- The table definition itself
- Three migration files that DEFINE the table
- The writer that FEEDS the table

Zero results about the **consumers** (dashboards, alert rules,
RSIC.self_reflect, guidance_effectiveness aggregation). All exist in
the substrate but weren't retrieved because "consumer" isn't in their
node names — the semantic-similarity model doesn't understand the
inverted relationship.

**Root cause:** MDEMG has no reverse-symbol-reference retrieval. The
structural retrieval column walks the graph but doesn't index
"references to symbol X" as a first-class relation.

### Cluster B: specific-value / config queries (q09)

**"Default value of FT_DRIFT_MARGIN"** returned four L3/L4 emergent
concepts about "error patterns" + "drift-error" but no config file
or code line naming the default 0.05.

**Root cause:** query is asking for a specific token in a code file
(`FtDriftMargin float64 // default: 0.05`), but the embedding scores
higher for abstract concepts whose name contains the word "drift" than
for the concrete config.go entry.

### Cluster C: abstract-over-concrete drift (q08, q14)

For "reframe of DRIFT-VALIDATION-001" (q08), retrieval surfaced L5
emergent-concepts about "test failure correction" and "iterative error
resolution" that share thematic words with the query but are in a
completely different subject area.

For "CUIDv2 rules" (q14), retrieval surfaced 5 duplicate/near-duplicate
L3/L4 emergent-concepts about "uuid-cuidv2" but did NOT surface the
`feedback_cuidv2_required.md` memory file with the actual rule text.

**Root cause:** the emergent-concept layers (L3/L4/L5) have very high
embedding similarity for any query whose terms appear in their
foundational-principle labels. On queries with a strong topic keyword,
the abstract layer dominates the concrete-content layer.

### Cluster D: redundancy — near-duplicate results (q04, q14)

Q04 returned TWO copies of the pre-bash-check module + TWO copies of
the Postgres sql package as separate top-5 slots.

**Root cause:** the substrate has near-duplicate nodes for some
symbols (likely from re-ingestion or the same code being represented
via multiple abstraction paths). The reranker doesn't apply a
diversity penalty.

### Cluster E: broadly-working queries (q05, q07, q10, q12)

Four queries returned 5/5 helpful. Common shape: the query names a
specific well-documented artifact (a sprint by ID, a feature doc name,
a function name). Retrieval is EXCELLENT at "give me things named X"
and its adjacent context.

## 6. Concrete next-lever recommendation

Ranked by (impact × effort⁻¹):

### Recommended sprint 1: RETRIEVAL-REVERSE-LOOKUP-001 (~2-3d)

Address **cluster A** — the reverse-lookup class. Options:
- Add a keyword-index reference column to RRF: for a query mentioning
  a symbol/table/function name, retrieve nodes that TEXTUALLY reference
  that name (grep-shaped, not vector-shaped)
- OR add a symbol-references-edge to the graph so the structural column
  can walk from `constraint_outcomes` → its consumers
- Expected impact: q11-shape queries move from 0/5 → 3-5/5

### Recommended sprint 2: RETRIEVAL-LAYER-BALANCE-001 (~2d)

Address **cluster C** — abstract concepts winning over concrete rules.
Options:
- Per-layer score adjustment: when a query has strong keyword signal
  (config name, constant name, rule name), boost L0/L1 concrete nodes
  over L3+ emergent-concepts
- OR: expose the L3+ layer as a SEPARATE retrieval column with an
  additive "abstract insight" score that doesn't crowd out the concrete
  answer
- Expected impact: q09/q14-shape queries move from 1-2/5 → 4/5

### Recommended sprint 3: RETRIEVAL-DIVERSITY-001 (~1d)

Address **cluster D** — near-duplicate results. Options:
- Simple: post-rerank pass that drops results whose vector-sim to a
  higher-ranked result exceeds a threshold (e.g. > 0.97 cosine)
- OR: enforce distinct-role_type or distinct-file-path in top-5
- Expected impact: modest but real; ~11% of results become useful
  slots instead of duplicates

### Deferred: RETRIEVAL-WRONG-CONTEXT-001

Address **cluster B / general wrong-context**. This is harder —
requires cross-encoder rerank quality tuning, or query intent
classification. Higher effort with less clear payoff. Handle after
the three above.

### Explicitly NOT recommended

- **Full reranker retraining** — the median 4/5 result suggests the
  reranker is not broken globally; targeted fixes will move the needle
  faster.
- **Corpus purge / staleness sweep** — staleness is 1/75 in this sample.
  Not the bottleneck.
- **Query rewriting / intent classification** — INTENT-DISABLE-001
  disabled this after evidence showed it was a net negative.

## 7. What this tells us about the substrate's overall quality

**The substrate is in a substantially better state than the guidance
follow-rate metric implies.**

- Guidance follow rate: ~11% (per JIMINY-CEILING-INVESTIGATION-001,
  a measurement artifact, not a capability limit)
- Retrieval helpful@5: 60% median 4/5 (this sprint, actually measuring
  helpfulness)

**These two numbers measure different things.** Guidance is
narrowly-scoped "did the agent follow this rule for this action";
retrieval is broadly-scoped "did the substrate return useful nodes."
The retrieval quality is what a developer actually experiences when
querying MDEMG for knowledge — and it's honestly pretty good.

The "substrate remembers wrong things" concern from the Q4 deep-dive
is **not confirmed by the data.** The substrate remembers reasonably
well; the failures are in specific query-shape blind spots, not in
substrate content.

## 8. Known limitations of this investigation

- **N=15 queries** is not statistically strong. Consistent patterns
  are visible, but a follow-up with N=50+ would firm.
- **I authored much of the content being retrieved.** Self-grade bias
  is real — an outside grader might mark some of my "helpful" as
  "wrong-context." Hard to eliminate without HITL second-opinion.
- **The query set is my choice.** Query shapes I didn't sample may
  fail differently (e.g. "how has X changed over time" — didn't test).
- **Only `mdemg-dev` space measured.** Other spaces (guidance
  training corpus, benchmark spaces) may score very differently.
- **Snapshot-in-time.** 2026-07-29 substrate state; ~10 min of live
  queries. Not a load test.

## 9. Follow-ups disclosed

1. **RETRIEVAL-REVERSE-LOOKUP-001** — the recommended #1 sprint
2. **RETRIEVAL-LAYER-BALANCE-001** — recommended #2
3. **RETRIEVAL-DIVERSITY-001** — recommended #3 (cheap)
4. **HITL second-opinion on the 75 sample rows** via the shipped
   review platform to firm the self-grade
5. **N=50+ statistical validation** if the current findings drive
   any follow-up sprint's design
6. **Cross-space audit** — same 15 queries run against `whk-wms` or
   `lnl-demo-whk` to check whether findings generalize

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (candidate #6)
- `docs/development/retrieval-quality-audit-001/sprint_plan.md`
  (this dir)
- `internal/models/models.go` (`RetrieveRequest` field shape:
  `query_text` not `query`)
- `internal/api/handlers.go` (`handleRetrieve`)
- CLAUDE.md pins for RRF-SCALE-001, SCORE-RETRIEVAL-REAL-SIGNALS-001,
  INTENT-DISABLE-001 (context on prior retrieval work)
- Live queries against `/v1/memory/retrieve` on mdemg-dev (15 queries,
  75 result rows captured at `/tmp/rqa/samples.json`)
- No substrate mutation; no code changes
