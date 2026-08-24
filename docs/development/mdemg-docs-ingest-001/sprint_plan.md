# MDEMG-DOCS-INGEST-001 — Sprint Plan

**Task**: #142
**Origin**: Deep-dive workflow `wf_b389463a-61b` recommended this as Top sprint after operator directive 2026-08-24 ("ingest information into MDEMG graphDB and only fine-tune on how to use the mdemg framework"). Operator ratified 2026-08-24: "Y, proceed" + confirmed freeze on `claude_code_knowledge*` AND `family_*/tier1` corpus growth.

**Category**: dev / substrate ingest + evidence-gathering
**Est. duration**: 1-2 days
**Owner**: `reh3376`

---

## 1. Header & Metadata

- **Sprint**: MDEMG-DOCS-INGEST-001
- **Version**: 1.0
- **Date**: 2026-08-24
- **Author**: Claude (opus 4.7) + operator `reh3376`
- **Predecessors**: CLAUDE-DOCS-INGEST-001 (task #124 — pattern reused verbatim); PHASE-E3 (task #138 — FAIL verdict forcing function); V2-RAW-BENCHMARK-001 (task #141 — evidence that base swap alone doesn't close the fact-recall gap)

## 2. Problem Statement

The operator's directive collapses scope: the adapter's job is HOW-TO-USE-MDEMG; the substrate's job is FACT-CARRYING. Today MDEMG's own docs (`docs/features/*.md`, `docs/user/*.md`, `docs/api/*`, `CLAUDE.md` Architecture Notes, CLI help text) are NOT in the mdemg-dev substrate. So there's no way for the shipped retrieval-augmented call sites (Jiminy synthesis, consulting classification, etc.) to answer MDEMG-usage questions grounded in the actual doc surface.

**Downstream effects if not fixed**:
- Any future MDEMG-usage adapter has no substrate to lean on at inference time
- The "adapter for how-to-use-MDEMG" hypothesis can't be measured (there's no substrate-aware benchmark path for it because the substrate has no MDEMG-docs)
- Every user question about MDEMG behavior is answered from the LLM's cached general knowledge (stale/wrong)

## 3. Scope & Constraints

**In scope**:
- New CLI subcommand `mdemg-docs-ingest --root <path> [--dry-run] [--limit N] [--force-reingest]` mirroring the shipped `claude-docs-ingest` shape
- Chunk strategy: read markdown files, split at H2 boundaries (`## `), one MemoryNode per H2 section; small files (no H2) → one node for whole file
- Ingest 5 doc surfaces via `POST /v1/memory/ingest`:
  1. `docs/features/*.md` (94 files, ~16k lines)
  2. `docs/user/*.md` (10 files, ~14k lines)
  3. `docs/api/*.md` + `docs/api/api-spec/**/*.md` (small)
  4. `CLAUDE.md` — H2 sections in Architecture Notes + Enforced Protocols (EXCLUDE session/sprint-record narrative junk per JIMINY-CORPUS-003 class)
  5. CLI help text extracted from `internal/cli/*.go` cobra `Long:` strings (one node per command)
- Path-keyed + content-hash for idempotent dedup (verbatim CLAUDE-DOCS-INGEST-001 pattern via existing `getNodeContentHash` helper)
- Live-verify probe: 8-12 hand-authored MDEMG-usage retrieval queries across (CLI, API, feature discovery, config) axes
- Honest verdict document per shipped 3-branch rubric

**Out of scope** (each disclosed in §11):
- Any LoRA training or benchmark run
- Rewriting the shipped `claude-docs-ingest` command
- Building a corpus curator (that's Alternative 1 gate on this sprint's ✅/⚠️ verdict)
- Fixing retrieval scoring (that's Alternative 2 gate on ❌ verdict)
- Any change to `training_data/sft/*` families (operator confirmed FREEZE)
- Recursive-retraining loop changes

**Constraints**:
- `space_id=mdemg-dev` per `auto-a4a36173bff8` pin; server-side deletion protection ensures no destructive risk
- CUIDv2 for row_id / any minted identifiers per `must-use-cuid2`
- No hardcoded values — env-driven config + flag overrides per `never-hardcode-config`
- Idempotent: re-runs on unchanged docs are no-ops
- Reversible: any ingested node can be `is_archived=true` (mirrors JIMINY-CORPUS-001 tombstone pattern)
- No mutation of production docs — this sprint READS docs, WRITES to substrate
- MUST include `Documents Accessed` list on all sprint docs per `end-with-docs-accessed`
- MUST run lint before commit per `lint-before-commit`

## 4. Dependencies

- **CLAUDE-DOCS-INGEST-001 SHIPPED** — provides the reusable POST payload builder + `getNodeContentHash` helper + `resolveDefaults` env pattern ✅
- **`/v1/memory/ingest` handler shipped** — `internal/api/handlers.go:705` accepts the same shape ✅
- **mdemg server running on :9999** — verified ✅ (llama-server on :8102 unaffected)
- **mdemg-dev Neo4j substrate accessible** — verified ✅
- **Ingest target files present** — 94 + 10 + 6 + CLAUDE.md + ~50 CLI Long strings ≈ **~160-200 chunkable sections** ✅

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Sibling CLI command `mdemg-docs-ingest`

New file `internal/cli/mdemg_docs_ingest.go` mirroring `claude_docs_ingest.go` structure but with:
- Input: `--root` pointing at repo root (default: current working dir)
- Reader: recursively enumerate the 5 doc surfaces (globs + a narrowly-scoped CLAUDE.md section extractor)
- Chunker: split each markdown file at H2 (`^## `) boundaries; one chunk per section; small files → one whole-file chunk
- Payload builder: reuse the shipped POST shape — `{space_id, timestamp, source: "mdemg-docs-ingest", content, path, name, summary, tags, content_hash}`
- Tags: `docs:mdemg`, `docs:mdemg:<surface>` (features/user/api/claude/cli-help), `obs_type:technical_note`
- Idempotency: reuse `getNodeContentHash` helper
- Dry-run mode; verbose per-row logging; batch delay; force-reingest flag

**Deliverable**: 1 new .go file + wired into cobra root command (via `internal/cli/root.go` if there's a subcommand list)

**Gate**: `go build ./internal/cli/...` clean; `go test ./internal/cli/... -run TestMdemgDocsIngest` green (Tier 1 tests below)

### Epic 2 — Tier 1 unit tests

New `internal/cli/mdemg_docs_ingest_test.go`:
- `TestMdemgDocsChunker_H2Split` — markdown with 3 H2 sections → 3 chunks with correct headers
- `TestMdemgDocsChunker_NoH2` — small file with no H2 → 1 chunk (whole file)
- `TestMdemgDocsChunker_CLAUDEmd_ExcludesNarrative` — sample CLAUDE.md section list; sprint-record sections excluded, Architecture Notes included
- `TestMdemgDocsChunker_CobraLongExtractor` — sample Go source with cobra Long strings → extracted correctly
- `TestBuildMdemgDocsIngestRequest_ShapeParity` — payload shape matches claude-docs-ingest for equivalent input (regression pin against `postClaudeDocsIngest` invariants)
- `TestPathSlug_MdemgDocs` — path format is `mdemg-docs/<surface>/<slug>` and doesn't collide with claude-docs

**Deliverable**: 6 pin tests

**Gate**: all 6 green

### Epic 3 — Dry-run against the real doc tree

`./bin/mdemg mdemg-docs-ingest --root . --dry-run`

Prints: total files scanned, chunks produced per surface, total chunks, sample paths, chunk-size distribution (min/max/median tokens).

**Deliverable**: dry-run report captured to sprint dir as `dry_run_report.md`

**Gate**: chunk count reasonable (~150-300 expected); no oversized chunks (>8k tokens); paths all follow `mdemg-docs/<surface>/<slug>` convention

### Epic 4 — Live ingest

`./bin/mdemg mdemg-docs-ingest --root . --batch-delay-ms 100` (respect embedder rate-limit; ~4-8h wall-clock)

Runs in background; log to sprint dir as `ingest.log`.

**Deliverable**: ingested count + skipped count + error count; TSDB (via `docker exec psql`) confirms new nodes with `space_id=mdemg-dev, source='mdemg-docs-ingest'`

**Gate**: 0 errors on non-network-error paths; skipped count = 0 on first run (fresh ingest); ingested count ≈ chunks produced

### Epic 5 — 8-12 live-verify probes

Hand-author 10 MDEMG-usage retrieval queries across 4 axes:
- **CLI** (3 queries): "how do I run mdemg upgrade", "what does mdemg data export do", "how do I ingest MDEMG docs into the substrate"
- **API** (3 queries): "what does POST /v1/jiminy/classify return", "what fields does GET /v1/jiminy/rules accept", "what's the /v1/memory/ingest payload shape"
- **Feature discovery** (2 queries): "how does the FT recursive loop decide to promote", "how does Jiminy classify guidance outcomes"
- **Config** (2 queries): "what env vars control RSIC alert thresholds", "what does MDEMG_MODEL_RAM_TIERS default to for v2"

For each query: POST to `/v1/memory/retrieve` with `query_text`, `top_k=10`, `candidate_k=200`, `include_content=true`, `space_id=mdemg-dev`. Record top-5 results (score + source_file + section_header).

**Deliverable**: `live_verify_report.md` with per-query table (query / expected-answer-source / top-5 hits / verdict)

**Gate**: at least the raw data captured; no errors on the probe run

### Epic 6 — Verdict + sprint post + commit

Compute the verdict per rubric:
- ✅ **≥70% (≥7 of 10)** of queries surface answer-bearing node in top-3 → path clear; file follow-up MDEMG-USAGE-CORPUS-CURATE-001
- ⚠️ **40-70% (4-6 of 10)** → narrow retrieval intervention scoped by ACTUAL failure pattern observed; file RETRIEVAL-META-DOC-SUPPRESSION-001 with data-anchored scope
- ❌ **<40% (<4 of 10)** → substrate composition needs retrieval work FIRST; file RETRIEVAL-META-DOC-SUPPRESSION-001 as blocker

Write `verdict.md` + `sprint_post.md` + CHANGELOG entry. Commit + push + PR comment + task update.

**Deliverable**: verdict.md + sprint_post.md + CHANGELOG entry + PR comment

**Gate**: verdict is one of {✅ / ⚠️ / ❌}; follow-up sprint filed with concrete inputs; task #142 marked completed

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests (Epic 2)

Six pin tests above. `go test ./internal/cli/... -v` all green.

### Tier 2 — Integration

`./bin/mdemg mdemg-docs-ingest --root . --dry-run` (Epic 3) — validates end-to-end file walk + chunker + payload builder + endpoint resolution without hitting the server.

### Tier 3 — Live e2e (Epic 4 + 5)

- Full ingest against live mdemg-dev substrate
- 10 probe queries against live `/v1/memory/retrieve`
- TSDB confirmation: `SELECT count(*) FROM memorynode_events WHERE space_id='mdemg-dev' AND source='mdemg-docs-ingest'` (or equivalent post-ingest count)
- Neo4j confirmation: `MATCH (n:MemoryNode {space_id: 'mdemg-dev'}) WHERE n.name STARTS WITH 'mdemg-docs' RETURN count(n)`
- Re-run ingest with same corpus → confirm 100% skipped (idempotency proof)

## 7. Commit Strategy

Two commits:
1. **Setup + tests** — new CLI file + Tier 1 tests + sprint plan
2. **Verdict** — live ingest logs + probe report + verdict.md + sprint_post.md + CHANGELOG

## 8. Verification Checklist

- [ ] `scripts/*` / `internal/cli/mdemg_docs_ingest.go` exist
- [ ] Tier 1 tests green (6 pins)
- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/cli/` clean
- [ ] Dry-run report captured with expected chunk count
- [ ] Live ingest completes 0 errors
- [ ] Idempotency proof (second run 100% skipped)
- [ ] TSDB + Neo4j confirmation
- [ ] 10 probe queries executed + logged
- [ ] Verdict = one of {✅/⚠️/❌}; follow-up sprint filed
- [ ] Sprint post + CHANGELOG + PR comment + task #142 completed

## 9. Documentation Update (final epic — never cut)

Epic 6 IS the doc update. Additionally:
- `docs/features/mdemg-docs-ingest.md` (per `mandatory-feature-docs`) with Why / Choices / How it works / How to use sections
- Sprint post + CLAUDE.md pin candidates (via next PR body — 1-2 arch rules from what shipped)

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Meta-doc dominance buries ingested content in retrieval (A2 blocker from deep-dive) | MEDIUM-HIGH | HIGH — invalidates the sprint's premise | Verdict rubric SURFACES this as ❌ branch; hands data-anchored scope to RETRIEVAL-META-DOC-SUPPRESSION-001 |
| CLAUDE.md includes sprint-record narrative junk (JIMINY-CORPUS-003 class) | MEDIUM | MEDIUM — pollutes substrate | Narrow the extractor to Architecture Notes + Enforced Protocols sections ONLY; regex-blacklist "Sprint " prefixed H3s |
| Ingest hits embedder rate limit or times out | LOW | LOW | Batch delay flag (default 100ms); resumable via idempotent dedup |
| CLI help text extractor pulls too little context (just usage lines) | MEDIUM | LOW | Extract `Long:` field, not just `Use:` field; skip commands with empty Long |
| Chunk boundaries mid-code-block leave orphaned fences | LOW | LOW | Simple regex split at H2; code blocks fully inside a section stay intact |
| Content_hash collision on identical section headers across files | LOW | MEDIUM — dedup silently drops content | Path includes filename+section; two files with same section header get different paths |
| Live-verify probes have bias toward what I know is in the docs | HIGH — this is a design signal, not a bug | LOW — the probe is a lower-bound proof-of-concept | Probe queries are hand-picked by task purpose (CLI/API/feature/config axes) so the coverage question is deliberate; a random-sample probe is a follow-up |

## 11. Non-Goals (explicit — deferred to future sprints)

- **MDEMG-USAGE-CORPUS-CURATE-001** — synthesize Q&A training corpus from ingested docs (~2-5k rows via OpenAI teacher, RAFT-shaped, leak-audited). GATED on this sprint's ✅ or ⚠️ verdict.
- **RETRIEVAL-META-DOC-SUPPRESSION-001** — data-anchored retrieval intervention if this sprint's ❌ branch fires
- **Any LoRA retraining** — operator confirmed FREEZE on training corpus growth
- **PHASE-E3-EVAL-SUBSTRATE-AWARE-001** — build substrate-aware benchmark harness. Deferred; the operator's directive means shipping the ingest first is higher-value than measuring the broken path
- **v2-base adapter** — v2-raw already 0.8236 standalone (V2-RAW-BENCHMARK-001); with substrate-augmented eval likely closes the fact-recall gap; adapter deferred until evidence justifies
- **CLI extract of every command** — Epic 1 extracts Long strings; not fabricating usage examples
- **RSS / activation-weight rebalancing** — separate concern; deferred

## 12. Documents Accessed

- `internal/cli/claude_docs_ingest.go` (shipped pattern to mirror — POST payload shape, `getNodeContentHash`, dedup semantics)
- `internal/cli/ingest_claude_md.go:503-` (`getNodeContentHash` helper location)
- `internal/api/handlers.go:705-792` (`handleIngest` — POST /v1/memory/ingest contract)
- `internal/models/models.go` (`ObserveRequest`, `IngestRequest` — payload types)
- `docs/features/` (listed 94 md files — ingest target)
- `docs/user/` (listed 10 md files — ingest target)
- `docs/api/` (listed contents — INGEST_CODEBASE_API.md, inventories, api-spec/)
- `docs/development/claude-docs-ingest-001/{sprint_plan,sprint_post}.md` (reference for pattern)
- `docs/development/phase-e3-retrain-benchmark-001/verdict.md` (forcing function for the operator directive)
- `docs/development/v2-raw-benchmark-001/quick_result.md` (evidence that base swap alone doesn't close fact-recall gap)
- CLAUDE.md — MDEMG Fine-Tuning shipped state, CLAUDE-DOCS-INGEST-001 pin, EMBED-CALLSITE-002 (why ingest payload gets `content_hash` + `space_id`)
- Deep-dive workflow artifact: `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/46583515-3b99-488f-a0fa-057205bd4204/subagents/workflows/wf_b389463a-61b/journal.jsonl` (10-agent investigation output)
- Operator ratification 2026-08-24: "Y, proceed with MDEMG-DOCS-INGEST-001" (freeze `claude_code_knowledge*` + `family_*/tier1` corpus growth confirmed)
