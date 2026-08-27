# MDEMG-DOCS-INGEST-001 — Sprint Post

**Task**: #142
**Shipped**: 2026-08-24
**Verdict**: ⚠️ **MIXED** — ingest succeeded end-to-end (1,221 nodes, 0 errors, 60% probes top-3); retrieval-side meta-doc-dominance blocker surfaces on 40% of probes → `RETRIEVAL-META-DOC-SUPPRESSION-001` filed as narrow data-anchored follow-up.

Full per-probe table + failure-pattern analysis + follow-up scoping in `verdict.md`. This sprint post covers what shipped + arch rules pinned.

## What shipped

1. **`internal/cli/mdemg_docs_ingest.go`** — new CLI subcommand `mdemg mdemg-docs-ingest` mirroring shipped `claude-docs-ingest` shape (POST to `/v1/memory/ingest` with path-key + content-hash idempotency; reuses `getNodeContentHash` helper for pre-check dedup).
2. **Chunker for 5 doc surfaces** (all Go, no external deps):
   - `docs/features/*.md` (94 files) — H2 splitter with preamble capture
   - `docs/user/*.md` (10 files) — same
   - `docs/api/*.md` + `docs/api/api-spec/**` — same
   - `CLAUDE.md` — H2 splitter with narrative-junk regex-blacklist (`^(Session|Sprint |Recent |Session-specific)` H2s dropped per JIMINY-CORPUS-003 class)
   - `internal/cli/*.go` — cobra `Long:` string extraction via `go/ast` (one node per command)
3. **`internal/cli/mdemg_docs_ingest_test.go`** — 6 Tier 1 pin tests: H2 split, no-H2 fallback, CLAUDE.md narrative-exclusion, cobra AST extractor, POST payload shape parity with claude-docs-ingest, path-slug non-collision with claude-docs namespace. All green.
4. **Wired into `internal/cli/root.go`** under memory group alongside `claude-docs-ingest`.
5. **1,221 new MemoryNodes in mdemg-dev substrate** (features 867, user 156, cli-help 135, api 42, claude 21). All with `path STARTS WITH 'mdemg-docs/'` + `source='mdemg-docs-ingest'`. Reversible via `is_archived=true`.
6. **Verdict document + probe harness** (`/tmp/mdemg_probe.py`) with 10 hand-authored queries across CLI/API/feature/config axes.

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run ./internal/cli/` | ✅ 0 issues |
| 6 Tier 1 pin tests | ✅ 6/6 pass |
| Dry-run against real repo | ✅ 1,225 chunks across 5 surfaces (matches manifest) |
| Live ingest | ✅ **1,225/1,225 POSTed, 0 errors, ~13 min wall-clock** |
| Neo4j confirmation | ✅ 1,221 nodes with `path STARTS WITH 'mdemg-docs/'` (4 dedup'd server-side on identical content_hash) |
| TSDB confirmation | ✅ 1,203+ `embedding_events` in `call_site='ingest'` window |
| Idempotency proof | ✅ Re-run with `--limit 20` → 20/20 skipped, 0 ingested |
| Cost | ~$0.012 OpenAI embeddings (well under trivial threshold) |
| 10 live-verify probes | ✅ 6/10 top-3, 8/10 top-5, 0/10 not-found |

## Decisions

| Decision | Rationale |
|---|---|
| Sibling command (`mdemg-docs-ingest`) instead of `--source mdemg-docs` on existing command | Path namespace isolation (`mdemg-docs/` vs `claude-docs/`); command Long descriptions can carry architectural framing specific to this sprint; sibling shape matches shipped `mdemg claude-docs-ingest` naming symmetry. Cost: ~15 lines of duplicated wiring. |
| H2 (`## `) chunk boundary, not H3/H4 | Matches shipped `claude-docs-ingest` chunking granularity; H2 sections average ~500 tokens (reasonable for embedder + downstream retrieval); H3-split would fragment coherent sections. Preamble captured so file-level context isn't lost. |
| CLAUDE.md filter via H2-header regex (not content-scanner) | Fast + deterministic + auditable; matches JIMINY-CORPUS-003 reject-pattern shape (regex-list, not LLM); pin-tested with synthetic CLAUDE.md fragment |
| cobra Long strings via `go/ast` (not regex/grep) | Robust to formatting; handles raw + regular strings; test-verifiable; grep would false-positive on struct definitions that mention "Long" |
| Path shape `mdemg-docs/<surface>/<file-stem-slug>/<idx>__<header-slug>` | Filename included so two files with same H2 header ("How it works" is very common) don't collide; surface prefix enables per-surface archive/query; idx preserves in-file ordering |
| Reused `getNodeContentHash` helper unchanged | Path + content-hash contract is shipped-stable (CLAUDE-DOCS-INGEST-001); no reason to fork |
| `source='mdemg-docs-ingest'` tag (not `technical_note` alone) | Enables per-sprint TSDB filtering (`SELECT count FROM ... WHERE source='mdemg-docs-ingest'`) + selective archive rollback |
| 100ms batch delay default | Matches claude-docs-ingest; embedder rate-limit friendly; live run averaged ~600ms per chunk end-to-end so batch delay wasn't the bottleneck |
| Ship as ⚠️ verdict (not defer + iterate) | Sprint plan §Epic 5 verdict rubric is evidence-decided; 60% top-3 is a real data point + hands a data-anchored scope to the follow-up; deferring would violate the "no treadmill" principle Skeptic 2 called out |

## Follow-ups (each disclosed)

### 🔴 RETRIEVAL-META-DOC-SUPPRESSION-001 — narrow retrieval intervention (⚠️ branch)

Scoped by ACTUAL failure pattern:
- Concrete inputs: `.goreleaser.yaml`, `CHANGELOG.md`, `CLAUDE.md` project-instructions preamble score 0.4-0.8 on nearly every MDEMG query while ingested feature docs score 0.005-0.05 (10-100× semantically unjustified gap)
- Candidate interventions (data-decided, pick ONE first): (a) `activation_confidence` cap for meta-doc paths; (b) reranker-side blacklist for known-noisy source_file patterns; (c) archive the 3 files' MemoryNodes (low-signal for MDEMG-usage anyway)
- Recommend #1 (least disruptive; targets actual mechanism)
- Verdict rubric: re-run this sprint's 10 probes after intervention → target ≥8/10 in top-3 (up from 6/10)

### 🟢 MDEMG-USAGE-CORPUS-CURATE-001 — begin curating training corpus (also unblocked)

60% top-3 is a workable RAG baseline; parallel corpus curation is not blocked by RETRIEVAL-META-DOC-SUPPRESSION-001. Operator can start Alt 1 whenever.

### 🟡 Feature doc `docs/features/mdemg-docs-ingest.md`

Per `mandatory-feature-docs` rule. Written as part of this commit for CLI-usage discovery via `docs/features/`.

## Arch rules pinned (proposed for CLAUDE.md via next PR body)

1. **Substrate-ingest sibling commands MUST namespace by path prefix** — `claude-docs/` vs `mdemg-docs/` prevents cross-ingest collisions on identical H2 headers. Pin-tested via `TestPathSlug_MdemgDocs` regression pin. Extends CLAUDE-DOCS-INGEST-001's path convention.

2. **`activation_confidence` accrual from long-time query patterns creates a meta-doc-dominance retrieval blocker** that E2-shipped ingest can't overcome without an activation-side intervention. Evidence: on 10 MDEMG-usage probe queries, 3 files (CHANGELOG.md, CLAUDE.md, .goreleaser.yaml) systematically score 10-100× higher than semantically-relevant feature docs regardless of query content. Confirms deep-dive workflow A2 investigation prediction from `wf_b389463a-61b`.

3. **Chunker H2 preamble capture is load-bearing** — dropping content before the first H2 loses file-level context (title, intro) that RAG downstream may need. Pin-tested via `TestMdemgDocsChunker_H2Split`.

4. **AST extraction of cobra Long strings is a lightweight substrate-population strategy** — no need to fabricate Q&A pairs when the CLI already has authoritative usage descriptions. This sprint's `extractCobraLongs` walker + `stringLiteralValue` decoder is reusable for future ingest patterns.

## Documents Accessed

- `docs/features/` (94 md files — INGESTED to substrate)
- `docs/user/` (10 md files — INGESTED)
- `docs/api/` (6 files — INGESTED)
- `CLAUDE.md` (21 durable H2 sections — INGESTED; narrative junk excluded)
- `internal/cli/*.go` (135 cobra Long strings — extracted via AST + INGESTED)
- `internal/cli/claude_docs_ingest.go` (pattern reused; no code touched)
- `internal/cli/ingest_claude_md.go:503-` (`getNodeContentHash` helper)
- `internal/cli/root.go` (cobra wiring — MODIFIED to register new subcommand)
- `internal/api/handlers.go:705-792` (handleIngest — POST /v1/memory/ingest contract; read-only)
- `internal/models/models.go` (ObserveRequest/IngestRequest payload types; read-only)
- `docs/development/mdemg-docs-ingest-001/{sprint_plan,dry_run_report,verdict}.md` (this sprint's own docs)
- `docs/development/claude-docs-ingest-001/` (reference pattern — CLAUDE-DOCS-INGEST-001)
- `docs/development/phase-e3-retrain-benchmark-001/verdict.md` (forcing function context)
- `docs/development/v2-raw-benchmark-001/quick_result.md` (evidence base swap alone insufficient)
- `/tmp/mdemg_probe.py` (probe harness, 10 queries)
- `/tmp/mdemg_probe_results.json` (probe verdict data)
- Live queries: `/v1/memory/retrieve` (10 probes), Neo4j (`docker exec cypher-shell`), TSDB (`docker exec psql`)
- Deep-dive workflow `wf_b389463a-61b` transcript (10-agent investigation output — foundational context)
- CLAUDE.md pins: CLAUDE-DOCS-INGEST-001, EMBED-CALLSITE-002, JIMINY-CORPUS-003 narrative-exclusion, MDEMG Fine-Tuning shipped state
- Operator ratifications 2026-08-24: (1) "Y, proceed with MDEMG-DOCS-INGEST-001"; (2) freeze `claude_code_knowledge*` + `family_*/tier1` corpus growth
