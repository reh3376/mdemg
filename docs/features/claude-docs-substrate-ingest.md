# Claude Code Docs — Substrate Ingest

**Sprint**: CLAUDE-DOCS-INGEST-001 (2026-08-17)
**Status**: shipped

## Why

MDEMG's `mdemg-llm-v1` (Phase-5 SFT of Qwen3-14B-4bit) has no Claude Code CLI / Agent SDK documentation in its training corpus. When queried about Claude Code specifics — slash commands, settings keys, hook events, SDK classes, EffortLevel enum, McpServerStatusConfig type — the raw model hallucinates.

Sprints CLAUDE-DOCS-TRAINING-001..004 tried the LoRA path (bake facts into model weights). Sprint 004 verdict: DO NOT PROMOTE — adapter regressed −0.13 vs baseline. Rule F pinned: **fact-recall tasks are substrate-ingest problems in MDEMG's architecture, not model-weight-fine-tune problems.** MDEMG is an improved RAG (RRF over 4-5 columns + rerank + Hebbian reinforcement + consolidation hierarchy + RSIC self-improvement). The correct fact-acquisition path is ingest into substrate, let retrieval + consulting surface at inference.

## What it does

`mdemg claude-docs-ingest` reads the curated Claude Code docs Q&A JSONL corpus (`training_data/claude-docs/curated/qa.jsonl`, 2191 rows produced by `neural/training/curate_claude_docs.py`) and POSTs each row to `/v1/memory/ingest`. Once ingested:

- **Retrieval surface** — `/v1/memory/retrieve` on Claude Code queries returns matched docs in top-5 with vector_sim 0.68-0.84 (10/10 hit rate on canned validation queries)
- **Consulting surface** — `/v1/memory/consult` routes retrieved context into LLM synthesis via `mdemg-llm-v1`
- **RSIC observability** — RSIC assess reflects the new nodes; health dimensions unchanged from pre-ingest baseline
- **Consolidation** — scheduled weekly LaunchAgent (CONSOLIDATE-PERF-002) will abstract L0 docs into L1+ emergent concepts on next cycle

## How it works

### Ingest schema (per row)
- **Path** (unique, deterministic): `claude-docs/<source_slug>/<3-digit-section-index>__<slug-of-section-header>`
  - e.g., `claude-docs/agent-sdk--python/023__effortlevel`
  - human-readable, safe for filesystem-style keying
- **Content**: `<prompt>\n\n<completion>` (Q+A both, so retrieval finds by keyword + embedding regardless of query shape)
- **Tags**: `[docs:claude-code, docs:<source_slug>, docs:concept:<type>, obs_type:technical_note]`
  - `docs:claude-code` for corpus-wide filtering
  - `docs:<slug>` for per-source-doc filtering
  - `docs:concept:<h2|h3|etc.>` for structural filtering
- **Name**: `<section_header>` (verbatim, so retrieval by exact-name match works)
- **Summary**: `<doc_title> — <section_header>` (capped 500 chars)
- **content_hash**: SHA256 of content (dedup gate)
- **space_id**: `mdemg-dev` (default; overridable via `MDEMG_SPACE_ID`)
- **obs_type**: `technical_note` (semantically closest to docs; NOT `constraint`/`correction` which are for durable rules)

### Idempotency
Path-keyed pre-check via `/v1/memory/node/meta?space_id=X&path=Y`:
- If node exists AND `content_hash` matches → **skip** (silent)
- Otherwise → POST (updates node in place via path-key merge)

⚠️ **Arch rule G**: `/v1/memory/ingest` does NOT dedup server-side. The server writes a new node on every POST + reports duplicates via `anomalies:[{type:"duplicate"}]` array. Client-side dedup pre-check is REQUIRED for idempotent batch ingest. Any future batch-ingest CLI MUST mirror this pattern.

## How to use

### Full corpus ingest
```
mdemg claude-docs-ingest
```
Reads `training_data/claude-docs/curated/qa.jsonl`, POSTs 2191 rows over ~10 min (50ms delay + ~200ms embed+neo4j per row). Idempotent — re-runs skip unchanged rows silently.

### Staged rollout
```
mdemg claude-docs-ingest --limit 10 --dry-run    # inspect what would happen
mdemg claude-docs-ingest --limit 10               # ingest 10 rows
mdemg claude-docs-ingest --limit 100              # ingest 100
mdemg claude-docs-ingest                          # full corpus
```

### Force re-ingest (bypass dedup)
```
mdemg claude-docs-ingest --force-reingest
```
Use if you've re-cured the corpus and want to overwrite existing nodes regardless of content_hash.

### Alternate corpus or space
```
mdemg claude-docs-ingest \
    --corpus training_data/claude-docs/curated/qa_v2.jsonl \
    --space-id my-scratch-space
```

### Env config
- `CLAUDE_DOCS_INGEST_ENDPOINT` — MDEMG server URL (default: `http://127.0.0.1:9999`)
- `CLAUDE_DOCS_INGEST_BATCH_DELAY_MS` — inter-request delay (default: `50`)
- `MDEMG_SPACE_ID` — target space (default: `mdemg-dev`)

## Rollback

```cypher
MATCH (n:MemoryNode)
WHERE n.space_id = 'mdemg-dev' AND n.path STARTS WITH 'claude-docs/'
SET n.is_archived = true,
    n.archive_reason = 'claude_docs_ingest_001_rollback',
    n.archived_at = datetime()
RETURN count(n) AS archived
```

Tombstone-only rollback (preserves audit trail); reversible via `SET n.is_archived = false`.

## Known limitations + follow-ups

**CLAUDE-DOCS-CONSULT-PASSTHROUGH-001 (filed)**: `/v1/memory/consult` LLM synthesis returned "not explicitly listed" for a verbatim-fact query even though the correct doc surfaced in retrieval with sim 0.832. Evidence the synthesis prompt sees node metadata (name + summary + node_id + rationale), NOT full content. For verbatim-recall workflows, either extend consult with a `content_passthrough` mode OR use a retrieve→fetch-content→inject-as-system-prompt pattern.

**UBENCH retrieval integration (deferred)**: `neural.benchmarks.run_benchmark` bypasses MDEMG's consulting layer, sending bare prompts to the LLM endpoint. Sprint 004's 50-row `claude.code_knowledge` eval would score identically to baseline (0.379) even after ingest because the raw `mdemg-llm-v1` doesn't know about the ingested substrate. Extending UBENCH to route through `/v1/memory/consult` (or inject retrieved context) is a separate sprint.

**Rule G/H/I pinned in sprint post** — see `docs/development/claude-docs-ingest-001/sprint_post.md`.
