# Graph Topology Fact Recall

**Sprint**: INGEST-TOPOLOGY-REPAIR-001 (2026-08-18)
**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 Phase A1
**Status**: shipped

## Why

MDEMG's graph topology was designed for fact recall + emergent concept storage. VISION.md line 74 calls L0 the "TapRoot" for concrete SME knowledge; line 434 explicitly designs `GROUNDED_BY` skip-connections so higher-layer abstractions retain a direct path back to raw L0 records. The substrate STORES verbatim content — either directly on the MemoryNode or on separate `:Observation` nodes via `HAS_OBSERVATION`.

But retrieval was structurally blind to that content. All 5 RRF columns (vectorRecall, BM25, graph, structural, concrete_recall) hardcoded `RETURN n.summary` in their Cypher. `RetrieveResult` had no `Content` field. `HAS_OBSERVATION` and `GROUNDED_BY` had zero read consumers. Every consumer of `/v1/memory/retrieve` — Jiminy hooks, MCP `memory_recall`, browser UI, consult synthesis — saw only 65-char summary signposts (e.g., "Use Claude Code with a screen reader — Turn on screen reader mode") while the actual 2244-byte documentation payload sat one edge away, unreachable.

This feature restores end-to-end fact-recall by making the substrate's stored content actually reach the callers who need it.

## What it does

Adds an opt-in `include_content` flag to `/v1/memory/retrieve` that projects verbatim `MemoryNode.content` (or the latest linked `Observation.content` as fallback for legacy nodes) into each result. Consult synthesis with `llm_synthesis:true` auto-enables the flag so `mdemg-llm-v1` gets verbatim reference material to ground on.

Also ships:
- **`/v1/memory/ingest`** now writes `n.content` on the MemoryNode (matches `/v1/conversation/observe`'s modern shape)
- **`mdemg backfill-node-content`** — one-time repair CLI that lifts latest `HAS_OBSERVATION.content` → `n.content` for legacy-ingest nodes never re-ingested after the fix landed

## How it works

### Read path — 3 layers of defense

1. **Ingest layer** (post-fix): `IngestObservation` SETs `n.content = $content` on both path + node_id merge branches. Every future ingest lands content on the MemoryNode directly.

2. **Retrieval column layer** (defense-in-depth): `vectorRecall` Cypher projects `coalesce(n.content, head([(n)-[:HAS_OBSERVATION]->(o) | o.content]))` — surfaces content for candidates surfacing via the vector column even before backfill runs.

3. **Post-rank bulk fetch layer** (column-agnostic): after RRF/rerank/diversity/promoter/truncate produces the final result set, `Service.fetchNodeContents` fires ONE bulk Cypher over the final top-K node_ids. Deterministic Observation fallback via `ORDER BY o.created_at DESC LIMIT 1` (avoids the list-comprehension `[...][0]` nondeterminism when a node has multiple observations). Works regardless of which column surfaced the candidate.

### Content cap

`RETRIEVE_CONTENT_MAX_BYTES` env var (default 8000 ≈ 2000 tokens) caps per-result content. A top-5 with `include_content:true` fits under ~40 KB response — well within synthesis budgets.

### Cache namespace

`IncludeContent` is added to `CacheKey` (separate cache namespace for with-content vs without). Prevents content-leak across requests with different flag values. Classified in `cacheKeyNeutralFields` per CACHE-KEY-002 convention.

## How to use

### Retrieve with content pass-through
```json
POST /v1/memory/retrieve
{
  "space_id": "mdemg-dev",
  "query_text": "How do I configure screen reader mode?",
  "top_k": 5,
  "include_content": true
}
```

Response:
```json
{
  "results": [
    {
      "node_id": "n_...",
      "path": "claude-docs/accessibility/000__turn-on-screen-reader-mode",
      "name": "Turn on screen reader mode",
      "summary": "Use Claude Code with a screen reader — Turn on screen reader mode",
      "content": "Pick the method that matches how often you use a screen reader:\n\n* For one session: run `claude --ax-screen-reader`.\n* ...",
      "score": 0.508,
      ...
    }
  ]
}
```

### Consult synthesis (automatic content pass-through when llm_synthesis=true)
```json
POST /v1/memory/consult
{
  "space_id": "mdemg-dev",
  "context": "Python agent using Claude Agent SDK",
  "question": "What are the exact values of Claude Code SDK EffortLevel Literal type?",
  "llm_synthesis": true
}
```

The consult service internally sets `IncludeContent=true` on its retrieve request because synthesis-with-content strictly dominates synthesis-with-summary-only. Response's `synthesis` field contains the model's narrative grounded on verbatim retrieved content.

### One-time backfill for legacy nodes
```
mdemg backfill-node-content --space-id mdemg-dev --dry-run    # count candidates
mdemg backfill-node-content --space-id mdemg-dev              # actually run
mdemg backfill-node-content --space-id mdemg-dev --batch-size 200 --limit 10000
```

Idempotent + additive-only. Safe to re-run — never overwrites existing `n.content`.

## Live verified (mdemg-dev, 2026-08-18)

Same EffortLevel query that returned *"not explicitly listed in the provided knowledge graph"* pre-fix now returns exact literal values via consult synthesis on `mdemg-llm-v1`:

> 1. **"low"** – Minimal thinking, fastest responses
> 2. **"medium"** – Moderate reasoning
> 3. **"high"** – Thorough analysis
> 4. **"xhigh"** – Extended reasoning depth
> 5. **"max"** – Maximum effort

Backfill run: **55,391 legacy-ingest nodes repaired**; **2191/2191 claude-docs** now have content (was 1/2191).

## Rollback

- Retrieval layer: setting `include_content:false` (default) reproduces byte-identical prior behavior via omitempty JSON tag
- Ingest layer: `git revert` — existing content stays, future ingests stop writing
- Backfill: `MATCH (n:MemoryNode) WHERE n.space_id='mdemg-dev' AND exists { MATCH (n)-[:HAS_OBSERVATION]->() } AND n.updated_at >= <backfill-timestamp> SET n.content = NULL` (scoped to backfill window)

Zero risk to production `:8102` llama-server — no serving symlink flips, no schema migrations.

## Related follow-ups

- **Phase A2 (planned)**: activate `GROUNDED_BY` skip-connection traversal — when retrieval surfaces L≥1 emergent concepts, follow to L0 verbatim evidence
- **Phase A3 (planned)**: `GET /v1/memory/nodes/{id}/content` low-level primitive
- **Phase B (planned)**: substrate-native constraint discovery for Jiminy — replace parallel vector queries with activation spreading over the query's node neighborhood
- **Optional E4 optimization**: extend BM25/graph/structural/concrete_recall/reverse_ref Cypher to project content directly, avoiding the post-rank bulk fetch for single-column responses

## Arch rules pinned (this sprint)

- **Rule K** — MDEMG is infrastructure, Jiminy is the dialogue, LoRA is assistance
- **Rule L** — Jiminy READS from MDEMG's shipped primitives
- **Rule M** — Prompt clauses have a retirement path
- **Rule N** — LoRA adapters do NOT carry facts
- **Rule O** — Both ingest paths converge on `MemoryNode.content`
- **Rule P** — Deterministic Observation fallback via `ORDER BY created_at DESC LIMIT 1`
- **Rule Q** — Content pass-through is opt-in at request layer
