# RETRIEVAL-REVERSE-LOOKUP-001 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #1 (RQA
cluster A — "what consumes X?" queries broken). Q4 follow-up #1.

## 1. Header & Metadata

Address RQA-001 cluster A by injecting **filesystem-grep-matched
candidates** into the RRF pool + top-K quota promoter. The mdemg
workspace filesystem is the source of truth; MDEMG's substrate
indexes function-level abstractions that don't include the symbol
strings from bodies, so any substrate-only intervention fundamentally
CANNOT solve the reverse-lookup class. ~1-1.5d effort. Follows the
just-shipped RETRIEVAL-LAYER-BALANCE-001 shape (fetch extra pool +
inject via quota).

## 2. Problem Statement

**Live q11 on mdemg-dev 2026-07-30**: "what consumes the
constraint_outcomes table?" → top-10 all writers/definers/migrations,
ZERO consumers. Reproduces RQA-001 exactly.

**Substrate diagnosis (sharpened by this sprint's investigation):**

- Consumers of `constraint_outcomes` in the codebase (verified via
  `grep -l "SELECT.*FROM constraint_outcomes"`):
  - `internal/tsdb/dataset_builder.go` (LLMPerformance)
  - `internal/alert/rules.go` (HeuristicShareRule, etc.)
  - `internal/eventgraph/guidance_outcomes.go`
  - `internal/cli/grafana_templates/staged/dashboards/mdemg-jiminy.json`
- Of these, only ONE (`GuidanceOutcomeWithContext`) has
  "constraint_outcomes" in its indexed MemoryNode summary. All others
  are structurally invisible to substrate search — their node
  summaries are function-level abstractions that don't include the
  SQL string literals from function bodies.

**This kills RQA-001's Option A (keyword-index reference column)
FUNDAMENTALLY**: no substrate-side indexing helps when the answer
isn't in the substrate.

**RQA-001's Option B (symbol-references graph edges) is architecturally
correct but has ingest-pipeline scope (~1-2 weeks)**: needs symbol
extraction (regex? tree-sitter?) + new edge type + ingest hook + graph
maintenance on file changes.

**This sprint ships a smaller-scope Option C (live filesystem grep):**
at query time, extract candidate symbols from the query, run a bounded
Go-native filepath.Walk + regex scan against a configured workspace
root, map matching file paths back to MemoryNodes, inject those into
the RRF pool as an "extras" pool via the RETRIEVAL-LAYER-BALANCE-001
quota mechanism.

**Recommend Option C over B for this sprint** because:
1. Immediate win (~1d) vs multi-week ingest-pipeline overhaul
2. Works on ANY substrate age (no re-ingest required for existing
   files)
3. Filesystem-first is philosophically cleaner: `mdemg-dev` claims to
   be a substrate ABOUT `/Users/reh3376/mdemg`, so consulting the
   filesystem directly is correct for reverse-lookup queries
4. The just-shipped RETRIEVAL-LAYER-BALANCE-001 quota mechanism
   already supports injecting an extras pool — reuses the proven
   shape
5. Option B is the RIGHT long-term fix but shouldn't gate this
   sprint (disclosed as follow-up)

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- New helper `fetchGrepReferences(ctx, queryText, workspaceRoot, cfg)`
  in `internal/retrieval/`:
  - Extract candidate symbols from `queryText` via regex
    `[A-Za-z_][A-Za-z0-9_]{4,}` with a stop-word filter (`the`,
    `what`, `table`, `consumes`, etc. via a small hardcoded list)
  - For each symbol, walk `workspaceRoot` filtering to allowlisted
    extensions (`.go`, `.py`, `.sql`, `.md`, `.yml`, `.yaml`,
    `.json`, `.ts`, `.tsx`, `.js`), regex-match content, count hits
  - Rank matching files by hit count DESC; take top-K
  - Look up each matching path as a `MemoryNode` in Neo4j, return
    `[]Candidate` for the RRF/quota injection
- Wire into `service.go::Retrieve` mirroring the concrete-recall
  shape: stash results, inject via `ApplyConcreteQuotaWithExtra` (or
  a new `ApplyReverseRefQuota` if operator prefers separate quota)
- New config knobs (default false; flip after live smoke):
  - `RETRIEVAL_REVERSE_REF_ENABLED` (default false)
  - `RETRIEVAL_REVERSE_REF_WORKSPACE_ROOT` (default `MDEMG_WORKSPACE`
    env → cwd)
  - `RETRIEVAL_REVERSE_REF_TOPK` (default 5)
  - `RETRIEVAL_REVERSE_REF_MIN_SYMBOL_LEN` (default 5)
  - `RETRIEVAL_REVERSE_REF_MAX_FILES_SCANNED` (default 5000 — safety
    cap for large workspaces)
  - `RETRIEVAL_REVERSE_REF_QUOTA_MIN_SLOTS` (default 1)
- Per-request URL override `?reverse_ref=true|false`
- Live smoke on q11 + guards on q10 (must remain 5/5) + q07 (specific
  sprint name — must remain 5/5)
- Flag flipped ON in `.env` after live smoke

**Out of scope:**

- Symbol-references-edge in the graph (RQA Option B) — disclosed as
  follow-up
- Query-time symbol extraction via LLM (extra latency; regex works)
- Multi-workspace support (single workspace root env var is
  sufficient for now)
- Content indexing at ingest time — deferred to Option B follow-up

## 4. Method

**Phase 1 — Symbol extractor + grep helper**
- `extractCandidateSymbols(queryText, minLen, stopWords) []string`
- `grepReferences(workspaceRoot, symbols, extensions, maxFiles,
  topK) []struct{Path, HitCount}` — Go native
  filepath.Walk + regex, NO shell-out (security)

**Phase 2 — Neo4j lookup + injection wire**
- `fetchReverseRefResults(ctx, spaceID, symbols) []Candidate` — grep
  helper + Neo4j path lookup
- Wire in `service.go::Retrieve` right after concrete-recall stash

**Phase 3 — Live A/B + docs + commit**
- q11 baseline vs candidate; target ≥3 consumers in top-10
- Guards on q07 + q10 (5/5 must hold)
- Docs, CHANGELOG, CLAUDE.md pin

## 5. Testing Plan

- **Tier 1 (unit)**: `extractCandidateSymbols` with stop-word filter;
  `grepReferences` against a small fixture workspace; hit-count
  ranking; safety cap (maxFilesScanned)
- **Tier 2 (integration)**: `fetchReverseRefResults` end-to-end against
  a small fixture Neo4j
- **Tier 3 (live)**:
  - q11: candidate surfaces ≥3 consumer files in top-10
  - q07: 5/5 helpful maintained
  - q10: 5/5 helpful maintained
  - New unrelated query (e.g., "what uses the retrieval Cache?") —
    surfaces `cache.go` consumers

## 6. Commit Strategy

Single commit under `RETRIEVAL-REVERSE-LOOKUP-001`.

## 7. Verification Checklist

- [ ] Symbol extractor + stop-words + min-length gate
- [ ] Go-native grep (NO shell-out) with extension allowlist + file
      cap
- [ ] Neo4j path lookup + Candidate conversion
- [ ] Wired via ApplyConcreteQuotaWithExtra or new ReverseRefQuota
- [ ] `?reverse_ref=true|false` URL override
- [ ] CacheKey wired (CACHE-KEY-002 contract)
- [ ] Unit + integration tests green
- [ ] Live q11: ≥3 consumers in top-10
- [ ] Live guards: q07, q10 unchanged
- [ ] Flag flipped ON in `.env`
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

- Set `RETRIEVAL_REVERSE_REF_ENABLED=false` in `.env`
- Revert commit for full removal

## 9. Risks

- **Risk**: workspace path unset OR wrong — sprint plan defaults to
  `MDEMG_WORKSPACE` env; if unset AND cwd doesn't look like a code
  repo, fall through to no-op (fail-open, don't crash)
- **Risk**: grep is slow on large workspaces (>10k files)
  - **Mitigation**: `MAX_FILES_SCANNED` cap (default 5000) with
    exclusions (`node_modules`, `.git`, `dist`, `.venv`, `_backup*`);
    the grep runs concurrent with vector recall so its latency
    is masked
- **Risk**: shell-injection or path-traversal — MITIGATED by using
  Go-native `filepath.Walk` + `regexp` (no shell), only reading files
  under configured workspace root, and refusing symlinks that escape
  the root
- **Risk**: adds noise on non-reverse-lookup queries (e.g. surfaces
  unrelated files that happen to contain a query word)
  - **Mitigation**: min-symbol-length gate (default 5 chars) + stop-
    word filter kills common English words. Ranking by hit-count
    prefers files that MEANINGFULLY reference the symbol. Guards
    on q07/q10 (both 5/5 helpful baseline) validate no regression.

## 10. Documents Accessed

Filled in `post.md`.
