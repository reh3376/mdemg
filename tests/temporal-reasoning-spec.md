Temporal Reasoning in mdemg: Time-Aware Retrieval Spec
0) Goals
Primary goals

Time-aware query interpretation

Detect temporal intent even when implicit: “latest”, “current”, “as of”, “now”, “since”, “prior to”, “during”, “in 2023”, “Q4”, etc.

Temporal filtering

Apply hard constraints when the user asked for them (e.g., “before 2022-01-01”).

Semantic-temporal hybrid ranking

Score candidates by semantic similarity and temporal relevance separately, then combine.

Temporal consistency across evidence

When retrieving multiple chunks, prefer sets that are time-coherent (no “2026 policy” chunk supporting a “2019 process” answer unless explicitly requested).

Time-decay (freshness bias)

For “latest/current” queries, automatically prefer newer sources; for historic queries, don’t.

Observability

Expose “why was this retrieved” with time factors: timestamp, age, decay applied, constraint satisfaction.

Non-goals (explicit)

Full “reasoning about time” inside the model (e.g., temporal logic proofs). This spec covers retrieval. The LLM can do the reasoning after the correct evidence is retrieved.

1) Terminology

Document: a file/page/thread/commit/issue/etc.

Chunk: a retrieved passage derived from a document (token window).

Event time: time described inside the text (optional/hard).

Source time: time associated with the artifact itself (git commit time, modified time, email date, etc.).

Temporal constraint: explicit bounds (e.g., 2021-05-01 to 2022-01-01).

Temporal intent: user’s implied preference (e.g., “latest” → prefer new).

1) Data Model Changes
2.1 Core timestamps

Every indexed document and chunk must have:

{
  "doc_id": "string",
  "chunk_id": "string",
  "source_type": "repo|docs|issues|notes|obsidian|...",
  "source_uri": "string",
  "created_at": "RFC3339 timestamp | null",
  "updated_at": "RFC3339 timestamp | null",
  "published_at": "RFC3339 timestamp | null",
  "observed_at": "RFC3339 timestamp | null",
  "time_confidence": "high|medium|low",
  "time_basis": "commit|mtime|frontmatter|metadata|inferred"
}

Rule: choose a single canonical time for ranking:

canonical_time = published_at ?? updated_at ?? created_at ?? observed_at

…and store it explicitly:

{
  "canonical_time": "RFC3339 timestamp",
  "canonical_time_basis": "published|updated|created|observed"
}

2.2 Optional “event time” extraction (phase 2)

Later, extract event_time ranges from text when present (“On Jan 2 2024 we changed…”). Store as:

{
  "event_time_start": "RFC3339|null",
  "event_time_end": "RFC3339|null",
  "event_time_confidence": "low|medium|high"
}

This is useful for questions like: “What happened in March 2024?” even if the file was edited later.

1) Query Understanding: Temporal Intent + Constraints
3.1 Temporal parsing output schema

Add a module:

TemporalQueryParser.parse(query: str, now: datetime) -> TemporalQuery

@dataclass
class TemporalConstraint:
    start: datetime | None  # inclusive
    end: datetime | None    # exclusive
    hard: bool              # True if user explicitly constrained

@dataclass
class TemporalIntent:
    mode: Literal["latest", "as_of", "historical", "evergreen", "none"]
    prefer_newer: bool
    prefer_older: bool
    decay_half_life_days: float | None  # for latest/as_of
    anchor_time: datetime | None        # for "as of X"

@dataclass
class TemporalQuery:
    raw_query: str
    cleaned_query: str  # query minus explicit time phrases when appropriate
    constraint: TemporalConstraint
    intent: TemporalIntent
    detected_times: list[datetime | tuple[datetime, datetime]]
    debug: dict

3.2 Parsing rules (must-have)

Explicit dates: YYYY, YYYY-MM, YYYY-MM-DD, month names, “last week/month/year”, “yesterday”, “Q1 2025”.

Keywords:

Latest/current/now/today → intent.mode = latest

As of / at that time → intent.mode = as_of with anchor_time

In 2019 / during 2020–2022 → historical with hard constraint range

“Between X and Y” → hard constraint.

If user explicitly gives a date/range, hard=True.

If user says “recent”, “latest”, “current” without explicit date, hard=False and use decay.

3.3 Output quality gates

If parser detects a date but is ambiguous (e.g., “04/05/06”), mark debug["ambiguous"]=True and default to ISO preference rules or require disambiguation at UX layer.

1) Retrieval Pipeline Changes

Assuming you have something like:

candidate retrieval (BM25 / dense embeddings)

reranking

final selection

We’ll insert temporal logic into filtering and ranking.

4.1 Candidate retrieval (unchanged, but pass cleaned query)

Use TemporalQuery.cleaned_query for semantic search, not the raw query.

4.2 Temporal filtering stage

After initial retrieval (top N), apply:

Hard filter

If constraint.hard=True:

Keep only candidates where canonical_time ∈ [start, end) if bounds exist.

If too few results remain, fall back to:

widen the range slightly only if user didn’t give exact bounds (hard=False). If hard=True, do not widen—return “no results in that time range” gracefully.

Soft filter (as_of)

If intent.mode == "as_of":

Keep candidates with canonical_time <= anchor_time (soft, but strongly preferred).

If none, allow nearest-after but penalize.

4.3 Semantic-temporal hybrid ranking
Compute two scores per candidate

S_sem: semantic score (dense similarity or reranker)

S_time: temporal relevance score (0..1)

Temporal relevance scoring

Define:

age_days = (now - canonical_time).days

anchor = intent.anchor_time or now

For latest queries:

S_time = exp(-ln(2) * age_days / half_life_days)

Choose half_life_days default by source type:

code/commits: 30

design docs: 90

policies/process: 180

notes: 45

For historical with explicit range:

If within range: S_time = 1

Else: S_time = 0 (or filtered out if hard)

For as_of:

If canonical_time <= anchor: S_time = exp(-ln(2) * (anchor - canonical_time)/half_life_days)

Else: penalize heavily, e.g. S_time *= 0.1

For evergreen/none:

S_time = 0.5 constant (or disable temporal weight)

Combine scores

Use a gated mixture:

S_final = w_sem *norm(S_sem) + w_time* S_time + w_src * S_source

Where:

w_sem default 0.75

w_time default:

latest/as_of: 0.25 to 0.45

historical with hard constraint: 0.05 (time mostly enforced via filtering)

none: 0.0

S_source is optional source trust weighting (docs > random notes etc.)

Normalize S_sem per query (min-max or z-score over top N), so time doesn’t get drowned.

Tie-breakers (important!)

If intent.mode in (latest, as_of):

Prefer higher canonical_time among close semantic matches.
If historical:

Prefer closer to midpoint of range (or simply highest semantic).

1) Temporal Consistency Across Multi-hop Evidence

When returning K chunks, you can accidentally select “best chunks” that contradict time.

Add a set selection pass:

5.1 Temporal coherence penalty

For selected set C:

Compute spread_days = max(time) - min(time)

If user asked “latest/current”: penalize large spread unless the question implies history.

If user asked “in 2019”: penalize anything outside.

Add penalty:

Penalty = clamp(spread_days / spread_threshold_days, 0, 1) * lambda

Defaults

spread_threshold_days:

latest: 30–90

as_of: 90

historical: range width (no penalty within)

lambda: 0.1–0.3

5.2 Re-ranking as a graph problem (optional, phase 2)

If you already have entity linking or citations:

Build a small evidence graph and ensure edges don’t violate chronology (e.g., a “policy updated 2026” should not be used to justify “2019 behavior” unless question asks “how has it changed since 2019”).

1) Indexing & Metadata Sources (Git + filesystem + Obsidian)
6.1 Git-backed files

For repo files:

created_at: first commit time for path

updated_at: last commit time for path

chunk inherits doc times

Implementation:

during indexing, run:

git log --follow --format=%aI -- path (take first+last)

6.2 Obsidian vault

Prefer frontmatter date, updated, created

Else fallback to filesystem mtime

Keep basis as frontmatter|mtime

6.3 Issues/PRs

created_at: issue creation

updated_at: last update

For comments, store comment_time and treat each comment as its own chunk-like item.

1) APIs & Integration Points
7.1 Retrieval interface

Add:

class TemporalRetriever:
    def retrieve(self, query: str, k: int = 8, now: datetime | None = None) -> RetrievalResult:
        ...

Return includes debug:

@dataclass
class RetrievedChunk:
    chunk_id: str
    doc_id: str
    text: str
    source_uri: str
    canonical_time: datetime
    time_basis: str
    scores: dict  # semantic, time, final, penalties
    highlights: dict

@dataclass
class RetrievalResult:
    temporal_query: TemporalQuery
    chunks: list[RetrievedChunk]
    diagnostics: dict

7.2 Explainability payload

Include in diagnostics:

detected intent, constraint

weights used

how many candidates filtered by time

top 10 with semantic/time/final scores

This is critical for debugging.

1) Configuration

Create config file (YAML/JSON):

temporal:
  enabled: true
  default_half_life_days:
    code: 30
    docs: 90
    notes: 45
    issues: 14
  weights:
    latest:
      sem: 0.65
      time: 0.35
    as_of:
      sem: 0.7
      time: 0.3
    historical:
      sem: 0.9
      time: 0.1
  spread_threshold_days:
    latest: 60
    as_of: 90
  spread_penalty_lambda: 0.2

1) Testing & Evaluation
9.1 Unit tests

Parse cases:

“latest design doc for X”

“as of 2024-06-01”

“between 2022 and 2023”

“in Q4 2025”

Filtering correctness:

hard constraints never widen

Ranking behavior:

same semantic, newer should win for latest

historical range, in-range should win even if slightly lower semantic

9.2 Retrieval benchmarks (you should build these)

Create a gold dataset of ~100–500 queries with:

expected top docs/chunks

expected time constraints

expected “as of” anchor

expected evidence set coherence

Track:

Recall@K

nDCG@K

Temporal accuracy (constraint satisfaction rate)

Coherence score (spread_days vs intent)

9.3 Regression guards

Non-temporal queries should not regress.

Add A/B toggle: temporal enabled vs disabled.

1) Implementation Plan (phased)
Phase 1 (high leverage, low risk)

Add canonical timestamps to index

TemporalQueryParser (intent + hard constraints)

Temporal filtering

Hybrid scoring + explainability

Config + tests

Phase 2 (better answers)

Event-time extraction from text

Temporal coherence penalty across selected chunks

Better “as_of” handling

Phase 3 (advanced)

Graph-based temporal consistency for multi-hop

Learned temporal reranker (train small model on your benchmark)

Per-source adaptive half-life learned from usage

1) “Two Macs” angle (optional but useful)

If mdemg runs distributed retrieval/indexing:

Keep indexing on Studio (more RAM)

Cache embeddings + time metadata locally

MacBook acts as client; only queries the Studio index

Temporal ranking is cheap; the heavier part is embedding/rerank

What I need from the repo to make this spec implementation-perfect

If you paste:

your current retrieval entrypoint (file/module name)

where embeddings / vector store live

how you store doc metadata today

…I can rewrite this into a repo-accurate spec with exact filenames, class names, and a migration plan (including a schema diff).
