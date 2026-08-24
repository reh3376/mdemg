# MDEMG Docs — Substrate Ingest

**Sprint**: MDEMG-DOCS-INGEST-001 (2026-08-24) · task #142
**Status**: shipped — 1,221 nodes ingested to mdemg-dev; ⚠️ verdict on retrieval-side follow-up
**Feature surface**: `mdemg mdemg-docs-ingest`

## Why

Direct execution of operator directive 2026-08-24: "ingest information into the MDEMG graphDB and only fine-tune on how to use the mdemg framework". The adapter's job is HOW-TO-USE-MDEMG; the substrate's job is FACT-CARRYING. Before this sprint, MDEMG's own docs (`docs/features/`, `docs/user/`, `docs/api/`, CLAUDE.md, CLI help text) were NOT in the mdemg-dev substrate. That meant shipped RAG call sites (`jiminy.synthesize`, `consulting.classify`, etc.) had no substrate content to ground MDEMG-usage questions in — they answered from the LLM's cached general knowledge (stale/wrong).

Companion to `claude-docs-substrate-ingest` (CLAUDE-DOCS-INGEST-001). Same architectural framing: fact-recall tasks are substrate-ingest problems, not model-weight-fine-tune problems.

## Choices

### Sibling command, not extension of `claude-docs-ingest`

- Path namespace isolation (`mdemg-docs/…` vs `claude-docs/…`) prevents collision on identical H2 headers ("How it works" is very common)
- Command Long-descriptions carry architectural framing specific to each ingest source
- Naming symmetry: `mdemg claude-docs-ingest` + `mdemg mdemg-docs-ingest` reads clearly

### H2 (`## `) as the chunk boundary

- Matches shipped `claude-docs-ingest` chunking granularity
- Average chunk ~500 tokens (reasonable for embedder + downstream retrieval)
- H3-split would fragment coherent sections
- Preamble (content before first H2) captured as `(preamble)` section so file-level context (title, intro) isn't lost

### AST-based cobra Long extraction

- Robust to formatting (raw + regular string literals both handled)
- Test-verifiable (`TestMdemgDocsChunker_CobraLongExtractor` pins the extractor)
- Grep would false-positive on struct definitions that mention "Long"

### CLAUDE.md narrative-junk exclusion

- Regex-blacklist H2 headers matching `^(Session|Sprint |Recent |Session-specific)` — same reject-pattern shape as JIMINY-CORPUS-003 constraint promotion gate
- Keeps durable Architecture Notes + Enforced Protocols; drops session/sprint-record narrative
- Pin-tested with synthetic CLAUDE.md fragment

### Path shape: `mdemg-docs/<surface>/<file-stem-slug>/<idx>__<header-slug>`

- Filename included so two features with the same H2 header don't collide
- Surface prefix enables per-surface archive/query
- Idx preserves in-file section ordering
- Slug format: lowercase alphanum + dash, capped at 80 chars

### Reused `getNodeContentHash` helper + `postClaudeDocsIngest` pattern

- Path + content-hash contract is shipped-stable
- No reason to fork the pre-check dedup helper
- POST payload shape mirrors claude-docs-ingest exactly (parity pin-tested via `TestBuildMdemgDocsIngestRequest_ShapeParity`)

## How it works

```
                ┌──────────────────────────────┐
                │  mdemg mdemg-docs-ingest     │
                │  (internal/cli/mdemg_docs_   │
                │   ingest.go)                  │
                └──────────────┬───────────────┘
                               │  collectMdemgDocsChunks
                               ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  5 doc surfaces walked in fixed order:                        │
   │  1. docs/features/*.md   — H2 splitter + preamble             │
   │  2. docs/user/*.md       — H2 splitter                        │
   │  3. docs/api/**/*.md     — H2 splitter                        │
   │  4. CLAUDE.md            — H2 splitter + narrative regex-drop │
   │  5. internal/cli/*.go    — AST cobra Long extraction          │
   └──────────────┬────────────────────────────────────────────────┘
                  │  1,225 chunks (observed on 2026-08-24)
                  ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  For each chunk, buildMdemgDocsIngestRequest → payload:       │
   │    { space_id: "mdemg-dev",                                   │
   │      source: "mdemg-docs-ingest",                             │
   │      content: <full section body>,                            │
   │      path: "mdemg-docs/<surface>/<file>/<idx>__<slug>",       │
   │      name: <H2 header>,                                       │
   │      summary: "<file-stem> — <header>" (cap 500),             │
   │      tags: ["docs:mdemg", "docs:mdemg:<surface>",             │
   │             "obs_type:technical_note"],                       │
   │      content_hash: SHA256(content) }                          │
   └──────────────┬────────────────────────────────────────────────┘
                  │  pre-check dedup: getNodeContentHash → skip if match
                  ▼
        POST /v1/memory/ingest → MemoryNode(role=technical_note, L0)
                  │
                  ▼
    RSIC + consolidation → L1+ abstractions
                  │
                  ▼
     retrievable via /v1/memory/retrieve for downstream RAG
```

Idempotent: re-runs on unchanged docs are no-ops (100% skip verified live).

Reversible: any ingested node can be archived via `is_archived=true` (same tombstone pattern as JIMINY-CORPUS-001).

## How to use

### Quick start

```bash
# Full ingest of MDEMG's own docs into mdemg-dev
./bin/mdemg mdemg-docs-ingest

# Dry-run first to see chunk counts + per-surface breakdown
./bin/mdemg mdemg-docs-ingest --dry-run

# Cap chunks (useful for testing)
./bin/mdemg mdemg-docs-ingest --limit 100

# Different repo root
./bin/mdemg mdemg-docs-ingest --root /path/to/mdemg

# Force re-ingest (bypass content-hash dedup — e.g., if docs edited)
./bin/mdemg mdemg-docs-ingest --force-reingest

# Custom endpoint + space
MDEMG_SPACE_ID=mdemg-scratch MDEMG_DOCS_INGEST_ENDPOINT=http://127.0.0.1:9998 \
  ./bin/mdemg mdemg-docs-ingest
```

### Configuration knobs

| Flag | Env var | Default | Purpose |
|---|---|---|---|
| `--root` | — | `.` | Repository root |
| `--endpoint` | `MDEMG_DOCS_INGEST_ENDPOINT` | `http://127.0.0.1:9999` | MDEMG server |
| `--space-id` | `MDEMG_SPACE_ID` | `mdemg-dev` | Target substrate space |
| `--dry-run` | — | `false` | Print + count without POSTing |
| `--limit` | — | `0` (all) | Cap chunks to ingest |
| `--force-reingest` | — | `false` | Ignore content_hash dedup |
| `--batch-delay-ms` | `MDEMG_DOCS_INGEST_BATCH_DELAY_MS` | `100` | Delay between requests |
| `--verbose` | — | `false` | Per-chunk logging |

### Verification queries

```bash
# Count ingested nodes per surface
docker exec mdemg-neo4j-1 cypher-shell -u neo4j -p testpassword -d neo4j --format plain "
MATCH (n:MemoryNode {space_id: 'mdemg-dev'})
WHERE n.path STARTS WITH 'mdemg-docs/'
RETURN split(n.path, '/')[1] AS surface, count(n) AS n
ORDER BY n DESC;
"

# TSDB embedding events during a recent ingest run
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -tAc "
SELECT count(*) FROM embedding_events
WHERE call_site = 'ingest' AND time > now() - interval '1 hour';
"
```

### Rollback

```bash
# Archive all mdemg-docs-ingest nodes (reversible; sets is_archived=true)
docker exec mdemg-neo4j-1 cypher-shell -u neo4j -p testpassword -d neo4j "
MATCH (n:MemoryNode {space_id: 'mdemg-dev'})
WHERE n.path STARTS WITH 'mdemg-docs/'
SET n.is_archived = true,
    n.archive_reason = 'mdemg_docs_ingest_rollback_YYYYMMDD',
    n.archived_at = datetime();
"

# Un-archive (undo the above)
docker exec mdemg-neo4j-1 cypher-shell -u neo4j -p testpassword -d neo4j "
MATCH (n:MemoryNode {space_id: 'mdemg-dev'})
WHERE n.path STARTS WITH 'mdemg-docs/'
  AND n.archive_reason = 'mdemg_docs_ingest_rollback_YYYYMMDD'
SET n.is_archived = false
REMOVE n.archive_reason, n.archived_at;
"
```

## Live verification (2026-08-24 sprint verdict)

10 hand-authored MDEMG-usage probe queries across CLI/API/feature/config axes:

- **6/10 (60%)** landed the answer-bearing node in **top-3**
- **8/10 (80%)** in **top-5**
- **0/10 not found** — every answer IS in the substrate

Per-probe table + failure-pattern analysis in `docs/development/mdemg-docs-ingest-001/verdict.md`.

**⚠️ MIXED verdict**: substrate coverage is 100%, but retrieval scoring buries some answers behind meta-doc dominance (`CHANGELOG.md`, `CLAUDE.md`, `.goreleaser.yaml` score 0.4-0.8 on nearly every MDEMG query while feature docs score 0.005-0.05).

Follow-up: `RETRIEVAL-META-DOC-SUPPRESSION-001` (data-anchored intervention scoped by the actual failure pattern).

## References

- CLAUDE-DOCS-INGEST-001 (task #124) — companion sprint; pattern reused
- Deep-dive workflow `wf_b389463a-61b` — operator-directive-driven investigation that recommended this sprint
- PHASE-E3-RETRAIN-BENCHMARK-001 (task #138) — forcing function (LoRA retrain FAIL on standalone eval)
- V2-RAW-BENCHMARK-001 (task #141) — evidence that base swap alone doesn't close fact-recall gap
- CLAUDE.md `must-master-data-pipelines` — architectural rule this sprint executes
- JIMINY-CORPUS-003 — narrative-junk exclusion pattern reused for CLAUDE.md filter
