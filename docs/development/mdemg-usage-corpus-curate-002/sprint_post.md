# MDEMG-USAGE-CORPUS-CURATE-002 — Sprint Post

**Task**: #148
**Completed**: 2026-09-03 (~1h wall-clock)
**Verdict**: ✅ SHIPPED — exporter WHERE clause + curator gate + retroactive tombstone; live-verified 518 leaks now excluded from a real 1,583-row raw JSONL and the leaked idna LICENSE MemoryNode archived.

Full plan at `sprint_plan.md`. Ship state + verification + one arch rule pinned below.

## What shipped

| Artifact | Notes |
|---|---|
| `scripts/mdemg_usage_export_docs.py` | New `MDEMG_DOCS_PATH_PREFIX` const; WHERE clause rewritten to `n.path STARTS WITH 'mdemg-docs/'` (was 5× `CONTAINS`); `classify_surface` re-anchored to require prefix + subprefix (`features/` / `user/` / `api/` / `cli-help/` / `claude/`) |
| `scripts/mdemg_usage_curate.py` | New `MDEMG_DOCS_PATH_PREFIX` const; new `is_valid_mdemg_docs_path()` predicate gated at entry of `build_qa_row`; new `wrong_prefix` skip reason wired |
| `scripts/test_mdemg_usage_curate.py` | 29 new/updated subcases: `test_classify_surface` (8 positive+negative), `test_is_valid_mdemg_docs_path` (11 subcases), `test_build_query_starts_with_prefix` (6 subcases), 4 new prefix-rejection cases in `test_build_qa_row` |
| Substrate tombstone (live) | 1 MemoryNode archived: `n_fbd44e97d4604cb1067d` at `mdemg-docs/api/license/000__whole-file` — `is_archived=true`, `archive_reason='mdemg_docs_ingest_002_venv_leak_purge'`, `archived_at=2026-09-03T22:07:55.936Z` |
| `docs/development/mdemg-usage-corpus-curate-002/{sprint_plan,sprint_post}.md` | This sprint |
| `CLAUDE.md` — arch note | Prefix predicates on data-pipeline queries MUST use `STARTS WITH`, not `CONTAINS` |
| `CHANGELOG.md` | Unreleased entry |

## Verification

| Check | Result |
|---|---|
| `python3 scripts/test_mdemg_usage_curate.py` | ✅ 67 pass / 0 fail (updated & new pin tests + full pre-existing suite) |
| Exporter WHERE clause via `build_query()` | ✅ contains `STARTS WITH`, no `CONTAINS 'docs/*'` |
| **Tier-3 live smoke — real raw JSONL** (`training_data/mdemg_usage/raw/nodes.jsonl`, 1,583 rows from 2026-08-24 pre-fix export) | ✅ curator emitted **806 out**; skip reasons `wrong_prefix: 518` (matches expected sum: 504 vendored `.venv/**/*.py` + 13 claude-docs/* + 1 empty), `min_words: 236`, `nav_header: 13`, `junk_path: 10` — 1,583 in = 806 + 777 rejected |
| **Live-verify 0 leaks in output** | ✅ per-row scan of `source_path`: 0 rows without `mdemg-docs/` prefix (was hundreds pre-fix) |
| **Tier-3 live substrate — broader sweep** | ✅ only 1 leaked node in scope (`n_fbd44e97d4604cb1067d`); no `.venv`/`__pycache__`/`node_modules`/`.dist-info`/`.egg-info` substring matches in any other `mdemg-docs/*` node |
| **Tier-3 live substrate — tombstone applied** | ✅ post-mutation query confirms `is_archived=true, archive_reason='mdemg_docs_ingest_002_venv_leak_purge'` |
| Production llama-server on port 8102 | ✅ untouched (this sprint has no runtime surface) |

## Live smoke — expected vs actual match

Pre-#148 raw JSONL path-prefix distribution (as measured on 2026-08-24 snapshot):

| Prefix | Count | Post-#148 disposition |
|---|---|---|
| `mdemg-docs/*` | 1,065 | ✅ pass through prefix gate; then min_words/nav_header/junk_path/existing filters → 806 curated |
| (empty from `/docs/…`) | 504 | ❌ `wrong_prefix` skip |
| `claude-docs/*` | 13 | ❌ `wrong_prefix` skip |
| `CLAUDE.md` (code symbol) | 1 | ❌ `wrong_prefix` skip |

**518 rejects = 504 + 13 + 1 (exact match).** Every non-`mdemg-docs/` prefix is now caught.

## Arch rule pinned (proposed for CLAUDE.md next PR)

**Prefix predicates on data-pipeline queries MUST use `STARTS WITH`, not `CONTAINS`.** A Cypher (or SQL) `CONTAINS 'X'` matches ANY occurrence of `X` anywhere in the field — including deeply-nested paths like `/docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/urllib3/**/*.py#Symbol`. When a query is meant to select rows produced by a specific ingest pipeline with a known path prefix (e.g. `mdemg-docs/`, `claude-docs/`, `code-symbol/`, `beta-tester-*`), the WHERE clause MUST be `STARTS WITH '<exact-prefix>'`. This is a data-pipeline invariant: the producer's prefix contract IS the correct selector. Loose `CONTAINS` predicates are almost always a bug waiting to happen — even if today's substrate happens not to contain a colliding path, tomorrow's ingest of a different corpus will. Defense-in-depth: mirror the predicate on both the query side (root cause) AND the downstream consumer's row-filter (regression guard). Task #148 caught 518 leak rows (504 vendored Python symbols + 13 claude-docs + 1 stray CLAUDE.md symbol) — a class that had silently polluted the LoRA training corpus for #145.

## Follow-ups disclosed

1. **Audit `mdemg extract-symbols` for `.venv/` exclusion** — separate sprint; the 504 vendored Python symbol nodes are `SymbolNode`s produced by the code-symbol ingester, not MemoryNodes from `mdemg-docs-ingest`. Different surface, its own vendor-path-guard sprint. Named: `EXTRACT-SYMBOLS-VENV-EXCLUSION-001` (candidate).
2. **Optional retrain on cleaned corpus** — the polluted 949-row training corpus went into #145's LoRA verdict PARITY-WITH-TRADEOFFS / NO-PROMOTE; a re-curate → re-train → benchmark using the 806-row clean corpus is possible but ISN'T needed for production (v1 stays production regardless of #145's adapter). Operator decision if pursued.
3. **Retention/cleanup pattern for future `.md` files in `docs/api/api-spec/uats/.venv/`** — the tombstone is one-shot. If a future `uv sync` (or similar) recreates the venv AND `mdemg mdemg-docs-ingest` is re-run, MDEMG-DOCS-INGEST-002's `.venv` exclusion in the walker prevents re-ingestion. If the walker exclusion is ever disabled via `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-`, the leak class returns. Belt-and-suspenders (not shipped): CLAUDE.md pin explicitly warns against disabling the deny-set in prod. Deferred.

## Documents Accessed

- `docs/development/mdemg-usage-corpus-curate-002/sprint_plan.md` (this sprint)
- `docs/development/mdemg-usage-corpus-curate-001/` — predecessor
- `docs/development/mdemg-docs-ingest-002/` — paired follow-up source
- `docs/development/mdemg-usage-lora-001/` — surfaced the 3-claude-docs leak
- `scripts/mdemg_usage_export_docs.py` — Epic 1 target
- `scripts/mdemg_usage_curate.py` — Epic 2 target
- `scripts/test_mdemg_usage_curate.py` — extended with 29 subcases
- `scripts/mdemg_usage_leak_audit.py`, `mdemg_usage_lora_assemble.py`, `mdemg_usage_manifest.py`, `mdemg_usage_eval.py`, `split_mdemg_usage_v1.py` — audited (no loose path predicates found)
- `internal/cli/mdemg_docs_ingest.go` — established `mdemg-docs/{surface}/…` path shape contract
- `training_data/mdemg_usage/raw/nodes.jsonl` — real 1,583-row Tier-3 smoke input
- Live Neo4j (`docker exec mdemg-neo4j-1 cypher-shell`) — Epic 3 substrate mutation + verification
- CLAUDE.md pins (§3 sprint plan)
- Operator directive: "proceed with #148" (2026-09-03)
