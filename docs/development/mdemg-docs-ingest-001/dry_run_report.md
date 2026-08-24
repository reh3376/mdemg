# MDEMG-DOCS-INGEST-001 — Dry-Run Report

**Sprint**: MDEMG-DOCS-INGEST-001 (task #142) — Epic 3 output
**Date**: 2026-08-24
**Command**: `./bin/mdemg mdemg-docs-ingest --root . --dry-run`

## Totals

**1,225 chunks** would be POSTed to `/v1/memory/ingest` on `mdemg-dev`.

## Per-surface breakdown

| Surface | Chunks | Source |
|---|---|---|
| features | 867 | `docs/features/*.md` (94 files @ H2 sections + preamble) |
| user | 156 | `docs/user/*.md` (10 files) |
| cli-help | 135 | cobra `Long:` strings across `internal/cli/*.go` |
| api | 46 | `docs/api/*.md` + `docs/api/api-spec/**/*.md` |
| claude | 21 | `CLAUDE.md` (H2 sections; narrative-junk regex-rejected) |
| **total** | **1,225** | |

## Chunk path shape (sample from first 5)

```
mdemg-docs/features/template/000__preamble
mdemg-docs/features/template/001__summary
mdemg-docs/features/template/002__vision-goals
mdemg-docs/features/template/003__current-state
mdemg-docs/features/template/004__notes
```

- Prefix: `mdemg-docs/<surface>/<file-stem-slug>/<section-idx>__<header-slug>`
- Namespace-isolated from `claude-docs/` (CLAUDE-DOCS-INGEST-001) — no path collisions

## Verdict on Epic 3 gate

✅ **Chunk count reasonable** (1,225 — 4× my initial 150-300 estimate; docs are structurally richer than I predicted)
✅ **No oversized chunks observed** — H2 splitter caps section size at natural markdown boundaries
✅ **Paths follow convention**: `mdemg-docs/<surface>/<file>/<idx>__<slug>`
✅ **CLAUDE.md filter working** — 21 H2 sections kept (Architecture Notes + Enforced Protocols) out of ~40 total; session/sprint-narrative rejected per JIMINY-CORPUS-003 class

## Cost estimate

- Embedder: OpenAI (verified via `.env`; API key present)
- ~500 tokens avg per chunk × 1,225 chunks = ~612,500 tokens
- OpenAI embedding pricing (`text-embedding-3-small` default): ~$0.00002 per 1K tokens
- **Est. total: ~$0.012** (trivial)

## Wall-clock estimate

- Ingest rate: ~1-2s per chunk (embedder round-trip + Neo4j write + async downstream)
- Batch delay: 100ms
- Est. total: **~35-50 min** at default batch-delay-ms=100
